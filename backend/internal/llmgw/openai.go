package llmgw

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/domain/llm"
)

type OpenAIClient struct {
	http *http.Client
	
	httpStream *http.Client
	
	APIKey string
}

func NewOpenAIClient(timeout time.Duration, apiKey string) *OpenAIClient {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &OpenAIClient{
		http:      &http.Client{Timeout: timeout},
		httpStream: &http.Client{},
		APIKey:    apiKey,
	}
}

type wireRequest struct {
	Model    string        `json:"model"`
	Messages []llm.Message `json:"messages"`
	
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Stream      bool     `json:"stream,omitempty"`
	
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
	Stop          []string       `json:"stop,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type wireResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Created int64  `json:"created"`
	Choices []struct {
		Index        int         `json:"index"`
		Message      llm.Message `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type wireChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *OpenAIClient) Complete(ctx context.Context, endpoint string, req llm.ChatRequest) (llm.ChatResponse, error) {
	body, err := json.Marshal(wireRequest{
		Model:       req.Model,
		Messages:    req.Messages,
		Temperature: floatPtr(req.Temperature),
		TopP:        floatPtr(req.TopP),
		MaxTokens:   req.MaxTokens,
		Stop:        req.Stop,
	})
	if err != nil {
		return llm.ChatResponse{}, err
	}

	httpReq, err := c.newRequest(ctx, endpoint, "/chat/completions", body)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("llmgw: upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return llm.ChatResponse{}, upstreamError(resp)
	}

	if errResp, ok := detectErrorBody(resp); ok {
		return llm.ChatResponse{}, fmt.Errorf("llmgw: upstream error (http 200): %s", errResp)
	}

	var wire wireResponse
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return llm.ChatResponse{}, fmt.Errorf("llmgw: decode upstream response: %w", err)
	}

	out := llm.ChatResponse{
		ID: wire.ID, Model: wire.Model, Created: wire.Created,
		Usage: llm.Usage{
			PromptTokens:     wire.Usage.PromptTokens,
			CompletionTokens: wire.Usage.CompletionTokens,
			TotalTokens:      wire.Usage.TotalTokens,
		}.Normalize(),
	}
	for _, ch := range wire.Choices {
		out.Choices = append(out.Choices, llm.Choice{
			Index: ch.Index, Message: ch.Message, FinishReason: ch.FinishReason,
		})
	}
	
	if out.Usage.TotalTokens == 0 {
		out.Usage = estimateUsage(req, out.Text())
	}
	return out, nil
}

func (c *OpenAIClient) Stream(ctx context.Context, endpoint string, req llm.ChatRequest, onChunk func(llm.Chunk) error) (llm.Usage, error) {
	body, err := json.Marshal(wireRequest{
		Model:        req.Model,
		Messages:     req.Messages,
		Temperature:  floatPtr(req.Temperature),
		TopP:         floatPtr(req.TopP),
		MaxTokens:    req.MaxTokens,
		Stop:         req.Stop,
		Stream:       true,
		
		StreamOptions: &streamOptions{IncludeUsage: req.StreamOptions == nil || req.StreamOptions.IncludeUsage},
	})
	if err != nil {
		return llm.Usage{}, err
	}

	httpReq, err := c.newRequest(ctx, endpoint, "/chat/completions", body)
	if err != nil {
		return llm.Usage{}, err
	}
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpStream.Do(httpReq)
	if err != nil {
		return llm.Usage{}, fmt.Errorf("llmgw: upstream stream failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return llm.Usage{}, upstreamError(resp)
	}

	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "json") && !strings.Contains(ct, "text/event-stream") {
		if msg, ok := detectErrorBody(resp); ok {
			return llm.Usage{}, fmt.Errorf("llmgw: upstream returned 200 with error body: %s", msg)
		}
		return llm.Usage{}, nil
	}

	var (
		usage   llm.Usage
		text    strings.Builder
		gotDone bool
		emitted int
	)
	scanner := bufio.NewScanner(resp.Body)
	
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			gotDone = true
			break
		}

		if msg, ok := detectErrorBodyFromText(payload); ok {
			return usage, fmt.Errorf("llmgw: upstream returned error chunk: %s", msg)
		}

		var frame wireChunk
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			
			continue
		}
		if frame.Usage != nil {
			usage = llm.Usage{
				PromptTokens:     frame.Usage.PromptTokens,
				CompletionTokens: frame.Usage.CompletionTokens,
				TotalTokens:      frame.Usage.TotalTokens,
			}.Normalize()
		}
		for _, ch := range frame.Choices {
			chunk := llm.Chunk{
				ID: frame.ID, Model: frame.Model,
				Delta: ch.Delta.Content, FinishReason: ch.FinishReason,
			}
			if chunk.Delta == "" && chunk.FinishReason == "" {
				continue
			}
			text.WriteString(chunk.Delta)
			emitted++
			if err := onChunk(chunk); err != nil {
				return usage, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return usage, fmt.Errorf("llmgw: read stream: %w", err)
	}

	if !gotDone && emitted > 0 {
		return usage, fmt.Errorf("llmgw: upstream stream terminated without [DONE] after %d chunks", emitted)
	}

	if usage.TotalTokens == 0 {
		usage = estimateUsage(req, text.String())
	}

	return usage, nil
}

func (c *OpenAIClient) Embed(ctx context.Context, endpoint string, req llm.EmbeddingRequest) (llm.EmbeddingResponse, error) {
	if err := req.Validate(); err != nil {
		return llm.EmbeddingResponse{}, err
	}
	body, err := json.Marshal(map[string]any{"model": req.Model, "input": req.Input})
	if err != nil {
		return llm.EmbeddingResponse{}, err
	}
	httpReq, err := c.newRequest(ctx, endpoint, "/embeddings", body)
	if err != nil {
		return llm.EmbeddingResponse{}, err
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return llm.EmbeddingResponse{}, fmt.Errorf("llmgw: embedding request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return llm.EmbeddingResponse{}, upstreamError(resp)
	}

	var wire struct {
		Model string `json:"model"`
		Data  []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return llm.EmbeddingResponse{}, fmt.Errorf("llmgw: decode embedding response: %w", err)
	}

	out := llm.EmbeddingResponse{
		Model: wire.Model,
		Usage: llm.Usage{PromptTokens: wire.Usage.PromptTokens, TotalTokens: wire.Usage.TotalTokens}.Normalize(),
	}
	for _, d := range wire.Data {
		out.Data = append(out.Data, llm.Embedding{Index: d.Index, Vector: d.Embedding})
	}
	return out, nil
}

func (c *OpenAIClient) newRequest(ctx context.Context, endpoint, path string, body []byte) (*http.Request, error) {
	url := strings.TrimSuffix(endpoint, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	return req, nil
}

func upstreamError(resp *http.Response) error {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("llmgw: upstream returned %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
}

func detectErrorBody(resp *http.Response) (string, bool) {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false
	}
	
	resp.Body = io.NopCloser(bytes.NewReader(raw))

	var probe struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &probe) != nil || probe.Error == nil {
		return "", false
	}
	msg := probe.Error.Message
	if msg == "" {
		msg = probe.Error.Type
	}
	if msg == "" {
		msg = probe.Error.Code
	}
	if msg == "" {
		msg = strings.TrimSpace(string(raw))
	}
	return msg, true
}

func detectErrorBodyFromText(text string) (string, bool) {
	var probe struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(text), &probe) != nil || probe.Error == nil {
		return "", false
	}
	msg := probe.Error.Message
	if msg == "" {
		msg = probe.Error.Type
	}
	if msg == "" {
		msg = probe.Error.Code
	}
	if msg == "" {
		msg = strings.TrimSpace(text)
	}
	return msg, true
}

func floatPtr(f float64) *float64 {
	return &f
}

func estimateUsage(req llm.ChatRequest, output string) llm.Usage {
	in := llm.EstimateTokens(req.Prompt())
	out := llm.EstimateTokens(output)
	return llm.Usage{PromptTokens: in, CompletionTokens: out, TotalTokens: in + out}
}