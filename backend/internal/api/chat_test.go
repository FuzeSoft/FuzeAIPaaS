package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fuze-ai-paas/backend/internal/app/llmgateway"
	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/ports"
	"github.com/gin-gonic/gin"
)

type fakeCompleter struct {
	calledWith []llm.ChatRequest
	reply      string
}

func (f *fakeCompleter) Complete(ctx context.Context, endpoint string, req llm.ChatRequest) (llm.ChatResponse, error) {
	f.calledWith = append(f.calledWith, req)
	return llm.ChatResponse{
		ID:    "fake-id",
		Model: req.Model,
		Choices: []llm.Choice{{
			Index:   0,
			Message: llm.Message{Role: llm.RoleAssistant, Content: f.reply},
		}},
		Usage: llm.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
	}, nil
}

func (f *fakeCompleter) Stream(ctx context.Context, endpoint string, req llm.ChatRequest, onChunk func(llm.Chunk) error) (llm.Usage, error) {
	f.calledWith = append(f.calledWith, req)
	_ = onChunk(llm.Chunk{Delta: f.reply, FinishReason: "stop"})
	return llm.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, nil
}

func newChatTestHandler(t *testing.T, completer ports.ChatCompleter) *Handler {
	t.Helper()
	h, store := newTestHandler(t)
	h.llmRoute = store.Route()
	h.llmQuota = store.TokenQuota()
	h.llmUsage = store.TokenUsage()
	h.llmTrace = store.Trace()

	gw, err := llmgateway.NewService(llmgateway.Deps{
		Routes:    llmgateway.NewRepoRouteTable(h.llmRoute),
		Completer: completer,
		Traces:    h.llmTrace,
		Usage:     h.llmUsage,
		Quota:     h.llmQuota,
	})
	if err != nil {
		t.Fatalf("构建网关服务: %v", err)
	}
	h.llmgw = gw
	return h
}

func chatRouter(h *Handler) http.Handler {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(auth.PassthroughMiddleware())
	r.POST("/v1/chat/completions", h.OpenAIChat)
	return r
}

func TestOpenAIChatDegradedWhenBackendMissing(t *testing.T) {
	h, _ := newTestHandler(t)
	h.llmgw = nil 

	body, _ := json.Marshal(map[string]any{
		"model":    "gpt-4o-mini",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	chatRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("期望 503 降级，实际 %d: %s", w.Code, w.Body.String())
	}
}

func TestOpenAIChatEndToEnd(t *testing.T) {
	completer := &fakeCompleter{reply: "hello from fake backend"}
	h := newChatTestHandler(t, completer)

	ctx := context.Background()
	if err := h.llmRoute.Save(ctx, "default", llm.Route{
		Model:    "gpt-4o-mini",
		Strategy: llm.StrategyPriority,
		Backends: []llm.Backend{{Name: "fake", Endpoint: "http://fake.local/v1", Weight: 1, Healthy: true}},
	}); err != nil {
		t.Fatalf("save route: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"model":    "gpt-4o-mini",
		"messages": []map[string]string{{"role": "user", "content": "ping"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	chatRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
	}
	var resp llm.ChatResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Text() != completer.reply {
		t.Fatalf("回复内容不符: %q", resp.Text())
	}
	if len(completer.calledWith) == 0 {
		t.Fatal("推理后端未被调用")
	}

	recs, err := h.llmUsage.ListUsage(ctx, "default", 10)
	if err != nil {
		t.Fatalf("读取用量失败: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("未记录用量流水，计量链路未打通")
	}
	if recs[0].Model != "gpt-4o-mini" || recs[0].Backend != "fake" {
		t.Fatalf("用量记录模型/后端不符: %+v", recs[0])
	}
}

func TestOpenAIChatUnknownModel(t *testing.T) {
	completer := &fakeCompleter{reply: "x"}
	h := newChatTestHandler(t, completer)

	body, _ := json.Marshal(map[string]any{
		"model":    "does-not-exist",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	chatRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("期望 404（模型未注册），实际 %d: %s", w.Code, w.Body.String())
	}
}

func TestOpenAIChatStreamReturnsUsage(t *testing.T) {
	completer := &fakeCompleter{reply: "hello"}
	h := newChatTestHandler(t, completer)

	ctx := context.Background()
	if err := h.llmRoute.Save(ctx, "default", llm.Route{
		Model:    "gpt-4o-mini",
		Strategy: llm.StrategyPriority,
		Backends: []llm.Backend{{Name: "fake", Endpoint: "http://fake.local/v1", Weight: 1, Healthy: true}},
	}); err != nil {
		t.Fatalf("save route: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"model":          "gpt-4o-mini",
		"messages":       []map[string]string{{"role": "user", "content": "ping"}},
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	chatRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
	}

	var sawDelta, sawUsage bool
	for _, line := range bytes.Split(w.Body.Bytes(), []byte("\n")) {
		s := strings.TrimSpace(string(line))
		if !strings.HasPrefix(s, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(s, "data:"))
		if payload == "[DONE]" {
			continue
		}
		var ch llm.Chunk
		if err := json.Unmarshal([]byte(payload), &ch); err != nil {
			t.Fatalf("无法解析 SSE 帧 %q: %v", payload, err)
		}
		if ch.Delta != "" {
			sawDelta = true
		}
		if ch.Usage != nil && ch.Usage.TotalTokens != 0 {
			sawUsage = true
		}
	}
	if !sawDelta {
		t.Fatal("流式响应未包含文本增量")
	}
	if !sawUsage {
		t.Fatal("#4: 流式响应未回传 usage 字段")
	}
}

func TestOpenAIChatStreamDefaultIncludeUsage(t *testing.T) {
	completer := &fakeCompleter{reply: "hi"}
	h := newChatTestHandler(t, completer)

	ctx := context.Background()
	if err := h.llmRoute.Save(ctx, "default", llm.Route{
		Model:    "gpt-4o-mini",
		Strategy: llm.StrategyPriority,
		Backends: []llm.Backend{{Name: "fake", Endpoint: "http://fake.local/v1", Weight: 1, Healthy: true}},
	}); err != nil {
		t.Fatalf("save route: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"model":    "gpt-4o-mini",
		"messages": []map[string]string{{"role": "user", "content": "ping"}},
		"stream":   true,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	chatRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
	}
	var sawUsage bool
	for _, line := range bytes.Split(w.Body.Bytes(), []byte("\n")) {
		s := strings.TrimSpace(string(line))
		if !strings.HasPrefix(s, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(s, "data:"))
		if payload == "[DONE]" {
			continue
		}
		var ch llm.Chunk
		if json.Unmarshal([]byte(payload), &ch) == nil && ch.Usage != nil && ch.Usage.TotalTokens != 0 {
			sawUsage = true
		}
	}
	if !sawUsage {
		t.Fatal("#4: 默认流式响应未回传 usage")
	}
}