package llmgw

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/domain/llm"
)

func chatReq() llm.ChatRequest {
	return llm.ChatRequest{
		Model:    "qwen",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
	}
}

func TestOpenAIClientComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization = %q", got)
		}
		fmt.Fprint(w, `{"id":"c1","model":"qwen","created":1,
			"choices":[{"index":0,"message":{"role":"assistant","content":"hi there"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`)
	}))
	defer srv.Close()

	c := NewOpenAIClient(time.Second, "sk-test")
	resp, err := c.Complete(context.Background(), srv.URL, chatReq())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text() != "hi there" {
		t.Fatalf("Text() = %q", resp.Text())
	}
	if resp.Usage.TotalTokens != 8 {
		t.Fatalf("TotalTokens = %d, want 8", resp.Usage.TotalTokens)
	}
}

func TestOpenAIClientEstimatesMissingUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"c1","model":"qwen",
			"choices":[{"index":0,"message":{"role":"assistant","content":"a fairly long answer here"}}]}`)
	}))
	defer srv.Close()

	c := NewOpenAIClient(time.Second, "")
	resp, err := c.Complete(context.Background(), srv.URL, chatReq())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Usage.TotalTokens == 0 {
		t.Fatal("usage was not estimated when upstream omitted it")
	}
	if resp.Usage.CompletionTokens == 0 {
		t.Fatal("completion tokens not estimated")
	}
}

func TestOpenAIClientNormalizesUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"c1","model":"qwen",
			"choices":[{"message":{"role":"assistant","content":"x"}}],
			"usage":{"prompt_tokens":7,"completion_tokens":3}}`)
	}))
	defer srv.Close()

	resp, err := NewOpenAIClient(time.Second, "").Complete(context.Background(), srv.URL, chatReq())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Usage.TotalTokens != 10 {
		t.Fatalf("TotalTokens = %d, want 10", resp.Usage.TotalTokens)
	}
}

func TestOpenAIClientUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "engine overloaded")
	}))
	defer srv.Close()

	_, err := NewOpenAIClient(time.Second, "").Complete(context.Background(), srv.URL, chatReq())
	if err == nil {
		t.Fatal("expected error on 503")
	}
	
	if !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "engine overloaded") {
		t.Fatalf("error lacks diagnostics: %v", err)
	}
}

func TestOpenAIClientStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"model\":\"qwen\",\"choices\":[{\"delta\":{\"content\":\"He\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"model\":\"qwen\",\"choices\":[{\"delta\":{\"content\":\"llo\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":2,\"total_tokens\":4}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	var got strings.Builder
	usage, err := NewOpenAIClient(time.Second, "").Stream(
		context.Background(), srv.URL, chatReq(),
		func(ch llm.Chunk) error { got.WriteString(ch.Delta); return nil },
	)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got.String() != "Hello" {
		t.Fatalf("streamed text = %q, want %q", got.String(), "Hello")
	}
	if usage.TotalTokens != 4 {
		t.Fatalf("TotalTokens = %d, want 4", usage.TotalTokens)
	}
}

func TestOpenAIClientStreamSkipsMalformedFrames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, ": heartbeat\n\n")
		fmt.Fprint(w, "data: {not json}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	var got strings.Builder
	_, err := NewOpenAIClient(time.Second, "").Stream(
		context.Background(), srv.URL, chatReq(),
		func(ch llm.Chunk) error { got.WriteString(ch.Delta); return nil },
	)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got.String() != "ok" {
		t.Fatalf("streamed text = %q, want %q", got.String(), "ok")
	}
}

func TestOpenAIClientStreamAbortsOnCallbackError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for i := 0; i < 100; i++ {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	sentinel := errors.New("client gone")
	count := 0
	_, err := NewOpenAIClient(time.Second, "").Stream(
		context.Background(), srv.URL, chatReq(),
		func(llm.Chunk) error { count++; return sentinel },
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
	if count != 1 {
		t.Fatalf("callback invoked %d times, want 1 (must abort immediately)", count)
	}
}

func TestOpenAIClientStreamEstimatesUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello world answer\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	usage, err := NewOpenAIClient(time.Second, "").Stream(
		context.Background(), srv.URL, chatReq(), func(llm.Chunk) error { return nil },
	)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if usage.CompletionTokens == 0 {
		t.Fatal("stream usage not estimated")
	}
}

func TestOpenAIClientEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path = %q, want /embeddings", r.URL.Path)
		}
		fmt.Fprint(w, `{"model":"bge","data":[{"index":0,"embedding":[0.1,0.2]}],
			"usage":{"prompt_tokens":3,"total_tokens":3}}`)
	}))
	defer srv.Close()

	resp, err := NewOpenAIClient(time.Second, "").Embed(context.Background(), srv.URL,
		llm.EmbeddingRequest{Model: "bge", Input: []string{"hi"}})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(resp.Data) != 1 || len(resp.Data[0].Vector) != 2 {
		t.Fatalf("unexpected data: %+v", resp.Data)
	}
}

func TestOpenAIClientEmbedValidates(t *testing.T) {
	_, err := NewOpenAIClient(time.Second, "").Embed(context.Background(), "http://unused",
		llm.EmbeddingRequest{Input: []string{"hi"}})
	if err != llm.ErrEmptyModel {
		t.Fatalf("want ErrEmptyModel, got %v", err)
	}
}

func TestOpenAIClientStream200ErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"error":{"message":"context length exceeded","type":"invalid_request_error"}}`)
	}))
	defer srv.Close()

	_, err := NewOpenAIClient(time.Second, "").Stream(
		context.Background(), srv.URL, chatReq(), func(llm.Chunk) error { return nil },
	)
	if err == nil {
		t.Fatal("Stream: want error for 200+error body, got nil")
	}
}

func TestOpenAIClientStreamEmbeddedErrorChunk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"error\":{\"message\":\"upstream overload\",\"type\":\"server_error\"}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	_, err := NewOpenAIClient(time.Second, "").Stream(
		context.Background(), srv.URL, chatReq(), func(llm.Chunk) error { return nil },
	)
	if err == nil {
		t.Fatal("Stream: want error for embedded error chunk, got nil")
	}
}

func TestOpenAIClientStreamReturnsUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":1,\"total_tokens\":8}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	usage, err := NewOpenAIClient(time.Second, "").Stream(
		context.Background(), srv.URL, chatReq(), func(llm.Chunk) error { return nil },
	)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if usage.PromptTokens != 7 || usage.CompletionTokens != 1 || usage.TotalTokens != 8 {
		t.Fatalf("usage = %+v, want prompt=7 completion=1 total=8", usage)
	}
}

func TestOpenAIClientRespectsContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := NewOpenAIClient(5*time.Second, "").Complete(ctx, srv.URL, chatReq())
	if err == nil {
		t.Fatal("expected context deadline error")
	}
}

func TestOpenAIClientTrimsTrailingSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"x"}}]}`)
	}))
	defer srv.Close()

	_, _ = NewOpenAIClient(time.Second, "").Complete(context.Background(), srv.URL+"/", chatReq())
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", gotPath)
	}
}