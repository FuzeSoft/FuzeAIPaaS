package llm

import "testing"

func TestChatRequestValidate(t *testing.T) {
	valid := []Message{{Role: RoleUser, Content: "hi"}}

	cases := []struct {
		name string
		req  ChatRequest
		want error
	}{
		{"ok", ChatRequest{Model: "m", Messages: valid}, nil},
		{"ok with params", ChatRequest{Model: "m", Messages: valid, Temperature: 0.7, TopP: 0.9, MaxTokens: 128}, nil},
		{"empty model", ChatRequest{Messages: valid}, ErrEmptyModel},
		{"blank model", ChatRequest{Model: "   ", Messages: valid}, ErrEmptyModel},
		{"no messages", ChatRequest{Model: "m"}, ErrNoMessages},
		{"bad role", ChatRequest{Model: "m", Messages: []Message{{Role: "root", Content: "x"}}}, ErrInvalidRole},
		{"temp too high", ChatRequest{Model: "m", Messages: valid, Temperature: 2.5}, ErrInvalidTemperature},
		{"temp negative", ChatRequest{Model: "m", Messages: valid, Temperature: -1}, ErrInvalidTemperature},
		{"topp too high", ChatRequest{Model: "m", Messages: valid, TopP: 1.5}, ErrInvalidTopP},
		{"topp negative", ChatRequest{Model: "m", Messages: valid, TopP: -0.1}, ErrInvalidTopP},
		{"negative max tokens", ChatRequest{Model: "m", Messages: valid, MaxTokens: -1}, ErrInvalidMaxTokens},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.req.Validate(); got != tc.want {
				t.Fatalf("Validate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestChatRequestValidateTreatsZeroAsUnset(t *testing.T) {
	req := ChatRequest{
		Model:       "m",
		Messages:    []Message{{Role: RoleUser, Content: "hi"}},
		Temperature: 0,
		TopP:        0,
		MaxTokens:   0,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("zero-valued sampling params should be accepted, got %v", err)
	}
}

func TestValidRole(t *testing.T) {
	for _, r := range []string{RoleSystem, RoleUser, RoleAssistant, RoleTool} {
		if !ValidRole(r) {
			t.Errorf("ValidRole(%q) = false, want true", r)
		}
	}
	for _, r := range []string{"", "admin", "System"} {
		if ValidRole(r) {
			t.Errorf("ValidRole(%q) = true, want false", r)
		}
	}
}

func TestChatRequestPrompt(t *testing.T) {
	req := ChatRequest{Messages: []Message{
		{Role: RoleSystem, Content: "be brief"},
		{Role: RoleUser, Content: "hello"},
	}}
	want := "system: be brief\nuser: hello"
	if got := req.Prompt(); got != want {
		t.Fatalf("Prompt() = %q, want %q", got, want)
	}
}

func TestUsageAdd(t *testing.T) {
	a := Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}
	b := Usage{PromptTokens: 3, CompletionTokens: 7, TotalTokens: 10}
	got := a.Add(b)
	want := Usage{PromptTokens: 13, CompletionTokens: 12, TotalTokens: 25}
	if got != want {
		t.Fatalf("Add() = %+v, want %+v", got, want)
	}
	
	if a.TotalTokens != 15 {
		t.Fatalf("Add mutated receiver: %+v", a)
	}
}

func TestUsageNormalize(t *testing.T) {
	got := Usage{PromptTokens: 10, CompletionTokens: 5}.Normalize()
	if got.TotalTokens != 15 {
		t.Fatalf("TotalTokens = %d, want 15", got.TotalTokens)
	}
	
	got = Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 99}.Normalize()
	if got.TotalTokens != 99 {
		t.Fatalf("TotalTokens = %d, want 99", got.TotalTokens)
	}
}

func TestChatResponseText(t *testing.T) {
	empty := ChatResponse{}
	if got := empty.Text(); got != "" {
		t.Fatalf("Text() on empty choices = %q, want empty", got)
	}
	resp := ChatResponse{Choices: []Choice{{Message: Message{Content: "answer"}}}}
	if got := resp.Text(); got != "answer" {
		t.Fatalf("Text() = %q, want %q", got, "answer")
	}
}

func TestEmbeddingRequestValidate(t *testing.T) {
	if err := (EmbeddingRequest{Model: "e", Input: []string{"x"}}).Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if err := (EmbeddingRequest{Input: []string{"x"}}).Validate(); err != ErrEmptyModel {
		t.Fatalf("want ErrEmptyModel, got %v", err)
	}
	if err := (EmbeddingRequest{Model: "e"}).Validate(); err != ErrNoMessages {
		t.Fatalf("want ErrNoMessages, got %v", err)
	}
}

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Fatalf("EstimateTokens(\"\") = %d, want 0", got)
	}
	
	if got := EstimateTokens("a"); got != 1 {
		t.Fatalf("EstimateTokens(\"a\") = %d, want 1", got)
	}
	
	const cjk = "你好世界"
	if got := EstimateTokens(cjk); got < 4 {
		t.Fatalf("EstimateTokens(%q) = %d, want >= 4", cjk, got)
	}
	
	if got := EstimateTokens("hello world this is a test"); got < 4 || got > 10 {
		t.Fatalf("EstimateTokens(english) = %d, want within [4,10]", got)
	}
}