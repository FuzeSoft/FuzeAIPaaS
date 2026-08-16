
package llmjudge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"fuze-ai-paas/backend/internal/ports"
)

type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		return nil, fmt.Errorf("llmjudge: LLM_BASE_URL and LLM_API_KEY are required")
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		model:   model,
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
			},
		},
	}, nil
}

func (c *Client) Model() string { return c.model }

func (c *Client) Judge(ctx context.Context, req ports.JudgeRequest) (ports.JudgeResponse, error) {
	if len(req.Dimensions) == 0 {
		return ports.JudgeResponse{}, fmt.Errorf("llmjudge: no dimensions to judge")
	}
	system, user := buildPrompt(req)
	body := map[string]interface{}{
		"model":       c.model,
		"temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"response_format": map[string]string{"type": "json_object"},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ports.JudgeResponse{}, fmt.Errorf("llmjudge: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return ports.JudgeResponse{}, fmt.Errorf("llmjudge: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return ports.JudgeResponse{}, fmt.Errorf("llmjudge: call llm: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ports.JudgeResponse{}, fmt.Errorf("llmjudge: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ports.JudgeResponse{}, fmt.Errorf("llmjudge: llm returned %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}

	content, err := extractContent(respBody)
	if err != nil {
		return ports.JudgeResponse{}, fmt.Errorf("llmjudge: parse response: %w", err)
	}
	return parseJudgeResponse(content, req.Dimensions), nil
}

func buildPrompt(req ports.JudgeRequest) (system, user string) {
	system = "You are a strict evaluation judge. Score the model output on each dimension with a float in [0,1]. Respond with a single JSON object only, no markdown, no prose."
	payload := map[string]interface{}{
		"task":        req.Task,
		"dataset":     req.Dataset,
		"dimensions":  req.Dimensions,
		"model_output": req.ModelOutput,
		"reference":    req.Reference,
	}
	b, _ := json.Marshal(payload)
	user = string(b) + "\n\nReturn JSON shape: {\"scores\":{\"<dim>\":<0..1>},\"overall\":<0..1>,\"comment\":\"<short>\"}"
	return system, user
}

func extractContent(respBody []byte) (string, error) {
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("unmarshal choices: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return out.Choices[0].Message.Content, nil
}

var jsonObjectRe = regexp.MustCompile(`(?s)\{.*\}`)

func parseJudgeResponse(content string, dims []ports.JudgeDimension) ports.JudgeResponse {
	var jr ports.JudgeResponse
	if err := json.Unmarshal([]byte(content), &jr); err != nil {
		if m := jsonObjectRe.FindString(content); m != "" {
			if err2 := json.Unmarshal([]byte(m), &jr); err2 != nil {
				return ports.JudgeResponse{Comment: "unparseable: " + truncate(content, 200)}
			}
		} else {
			return ports.JudgeResponse{Comment: "unparseable: " + truncate(content, 200)}
		}
	}
	if jr.Scores == nil {
		jr.Scores = map[string]float64{}
	}
	var weightSum, weighted float64
	for _, d := range dims {
		v := jr.Scores[d.Name]
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		jr.Scores[d.Name] = v
		weighted += v * d.Weight
		weightSum += d.Weight
	}
	if jr.Overall <= 0 && weightSum > 0 {
		jr.Overall = weighted / weightSum
	}
	if jr.Overall < 0 {
		jr.Overall = 0
	}
	if jr.Overall > 1 {
		jr.Overall = 1
	}
	return jr
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}