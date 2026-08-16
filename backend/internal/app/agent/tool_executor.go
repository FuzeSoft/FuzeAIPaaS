package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/domain/agent"
)

type RegistryToolExecutor struct {
	
	Tools ToolRepo
	
	Retriever Retriever
	
	http *http.Client
}

type ToolRepo interface {
	GetByName(ctx context.Context, tenantID, name string) (*agent.Tool, error)
}

func NewToolExecutor(tools ToolRepo, retriever Retriever) *RegistryToolExecutor {
	return &RegistryToolExecutor{
		Tools:     tools,
		Retriever: retriever,
		http: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: noRedirectToPrivate,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
			},
		},
	}
}

func (e *RegistryToolExecutor) Execute(t agent.ToolCall) (agent.ToolResult, error) {
	switch t.Tool {
	case "rag":
		return e.execRAG(t)
	default:
		return e.execHTTP(t)
	}
}

func (e *RegistryToolExecutor) execRAG(t agent.ToolCall) (agent.ToolResult, error) {
	if e.Retriever == nil {
		return agent.ToolResult{}, errors.New("tool rag: retriever not configured")
	}
	q := t.Args["query"]
	query, _ := q.(string)
	if query == "" {
		return agent.ToolResult{}, errors.New("tool rag: missing args.query")
	}
	topK := 4
	if v, ok := t.Args["top_k"].(float64); ok && v > 0 {
		topK = int(v)
	}
	hits, err := e.Retriever.Retrieve(context.Background(), "", query, topK)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("tool rag: %w", err)
	}
	return agent.ToolResult{OK: true, Data: formatSegments(hits)}, nil
}

func (e *RegistryToolExecutor) execHTTP(t agent.ToolCall) (agent.ToolResult, error) {
	if e.Tools == nil {
		return agent.ToolResult{}, fmt.Errorf("tool %s: no tool registry configured", t.Tool)
	}
	def, err := e.Tools.GetByName(context.Background(), t.TenantID, t.Tool)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("tool %s: not found: %w", t.Tool, err)
	}
	if def.Kind != agent.ToolKindHTTP || def.HTTP == nil {
		return agent.ToolResult{}, fmt.Errorf("tool %s: not an http tool", t.Tool)
	}
	if def.Sensitive {
		return agent.ToolResult{}, fmt.Errorf("tool %s: sensitive tools require human approval, unsupported in DAG run", t.Tool)
	}

	raw := def.HTTP.URL
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return agent.ToolResult{}, fmt.Errorf("tool %s: only http/https urls allowed", t.Tool)
	}
	if err := ssrfGuard(raw); err != nil {
		return agent.ToolResult{}, fmt.Errorf("tool %s: %w", t.Tool, err)
	}

	method := def.HTTP.Method
	if method == "" {
		method = http.MethodPost
	}
	timeout := time.Duration(def.HTTP.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var body io.Reader
	if b, err := json.Marshal(t.Args); err == nil {
		body = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, raw, body)
	if err != nil {
		return agent.ToolResult{}, err
	}
	for k, v := range def.HTTP.Headers {
		req.Header.Set(k, v)
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := e.http.Do(req)
	if err != nil {
		return agent.ToolResult{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) 
	if err != nil {
		return agent.ToolResult{}, err
	}
	if resp.StatusCode >= 400 {
		return agent.ToolResult{}, fmt.Errorf("tool %s: http %d: %s", t.Tool, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return agent.ToolResult{OK: true, Data: string(data)}, nil
}

var ssrfGuard = guardSSRF

func guardSSRF(rawURL string) error {
	host := rawURL
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.IndexAny(host, "/:#?"); i >= 0 {
		host = host[:i]
	}
	host = strings.TrimSuffix(host, "]")
	if i := strings.LastIndex(host, ":"); i >= 0 && strings.Contains(host, "]") == false {
		host = host[:i]
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("dns resolve failed: %w", err)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("refusing private address %s", ip)
		}
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 10,
			v4[0] == 127,
			v4[0] == 169 && v4[1] == 254,
			v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31,
			v4[0] == 192 && v4[1] == 168:
			return true
		}
	}
	return false
}

func noRedirectToPrivate(req *http.Request, via []*http.Request) error {
	return http.ErrUseLastResponse
}