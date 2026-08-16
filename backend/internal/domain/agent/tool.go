package agent

import "time"

type ToolKind string

const (
	
	ToolKindHTTP ToolKind = "http"
	
	ToolKindBuiltin ToolKind = "builtin"
)

type Tool struct {
	ID          string   `json:"id"`
	TenantID    string   `json:"tenant_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Kind        ToolKind `json:"kind"`
	
	HTTP *HTTPToolSpec `json:"http,omitempty"`
	
	Sensitive bool `json:"sensitive"`
	
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type HTTPToolSpec struct {
	URL    string            `json:"url"`
	Method string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
	
	TimeoutMs int `json:"timeout_ms"`
}

type ToolCall struct {
	
	Tool string `json:"tool"`
	
	Args map[string]any `json:"args"`
	
	TenantID string `json:"-"`
}

type ToolResult struct {
	
	OK bool `json:"ok"`
	
	Data string `json:"data,omitempty"`
	
	Error string `json:"error,omitempty"`
}

type ToolExecutor interface {
	
	Execute(tool ToolCall) (ToolResult, error)
}

type RagQuery struct {
	
	KnowledgeBaseID string
	
	Query string
	
	TopK int
}

func ExtractRagQuery(nodeRef string, args map[string]any) RagQuery {
	q := RagQuery{KnowledgeBaseID: nodeRef, TopK: 4}
	if v, ok := args["query"].(string); ok {
		q.Query = v
	}
	if v, ok := args["top_k"].(float64); ok {
		q.TopK = int(v)
	}
	return q
}

type LLMCallSpec struct {
	
	Model string
	
	System string
	
	Prompt string
	
	KnowledgeBaseID string
}

func ExtractLLMCall(ref string, cfg map[string]any) LLMCallSpec {
	sp := LLMCallSpec{Model: ref}
	if v, ok := cfg["system"].(string); ok {
		sp.System = v
	}
	if v, ok := cfg["prompt"].(string); ok {
		sp.Prompt = v
	}
	if v, ok := cfg["knowledge_base_id"].(string); ok {
		sp.KnowledgeBaseID = v
	}
	return sp
}

func RenderPrompt(template string, upstream func(nodeID string) string) string {
	if template == "" {
		return template
	}
	
	out := template
	for i := 0; i < len(out); i++ {
		if out[i] != '{' {
			continue
		}
		end := indexByte(out[i:], '}')
		if end < 0 {
			continue
		}
		name := out[i+1 : i+end]
		if name != "" {
			if val := upstream(name); val != "" {
				out = out[:i] + val + out[i+end+1:]
			}
		}
	}
	return out
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}