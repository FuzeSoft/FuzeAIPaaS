package llmjudge

import (
	"context"
	"testing"

	"fuze-ai-paas/backend/internal/ports"
)

func TestStubJudgeReturnsAllDimensions(t *testing.T) {
	s := NewStub("test-stub")
	req := ports.JudgeRequest{
		Task: "classification",
		Dimensions: []ports.JudgeDimension{
			{Name: "accuracy", Weight: 0.5},
			{Name: "fluency", Weight: 0.5},
		},
	}
	resp, err := s.Judge(context.Background(), req)
	if err != nil {
		t.Fatalf("stub judge: %v", err)
	}
	
	if len(resp.Scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(resp.Scores))
	}
	for _, d := range req.Dimensions {
		v, ok := resp.Scores[d.Name]
		if !ok {
			t.Fatalf("missing score for dimension %q", d.Name)
		}
		
		if v < 0.4 || v >= 0.6 {
			t.Fatalf("stub score %v for %q out of [0.4,0.6) range", v, d.Name)
		}
	}
}

func TestStubDeterministicScores(t *testing.T) {
	s := NewStub("test-stub")
	req := ports.JudgeRequest{
		Task: "fixed-task",
		Dimensions: []ports.JudgeDimension{
			{Name: "accuracy", Weight: 1},
		},
	}
	r1, _ := s.Judge(context.Background(), req)
	r2, _ := s.Judge(context.Background(), req)
	
	if r1.Scores["accuracy"] != r2.Scores["accuracy"] {
		t.Fatalf("stub scores not deterministic: %v vs %v", r1.Scores["accuracy"], r2.Scores["accuracy"])
	}
}

func TestStubModelName(t *testing.T) {
	s := NewStub("my-stub")
	if s.Model() != "my-stub" {
		t.Fatalf("expected model name 'my-stub', got %q", s.Model())
	}
	
	s2 := NewStub("")
	if s2.Model() != "stub" {
		t.Fatalf("expected default 'stub', got %q", s2.Model())
	}
}

func TestStubOverallIsWeightedAverage(t *testing.T) {
	s := NewStub("stub")
	req := ports.JudgeRequest{
		Task: "test",
		Dimensions: []ports.JudgeDimension{
			{Name: "a", Weight: 0.6},
			{Name: "b", Weight: 0.4},
		},
	}
	resp, err := s.Judge(context.Background(), req)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	
	expected := resp.Scores["a"]*0.6 + resp.Scores["b"]*0.4
	if abs(resp.Overall-expected) > 0.001 {
		t.Fatalf("expected overall %.4f, got %.4f", expected, resp.Overall)
	}
}

func TestNewFromEnvStubWhenUnconfigured(t *testing.T) {
	
	j := NewFromEnv(func(key string) string { return "" })
	if _, ok := j.(*Stub); !ok {
		t.Fatalf("expected *Stub when unconfigured, got %T", j)
	}
}

func TestNewFromEnvClientWhenConfigured(t *testing.T) {
	env := map[string]string{
		"LLM_BASE_URL": "https://api.openai.com/v1",
		"LLM_API_KEY":  "sk-test",
		"LLM_MODEL":    "gpt-4o-mini",
	}
	j := NewFromEnv(func(key string) string { return env[key] })
	c, ok := j.(*Client)
	if !ok {
		t.Fatalf("expected *Client when configured, got %T", j)
	}
	if c.Model() != "gpt-4o-mini" {
		t.Fatalf("expected model gpt-4o-mini, got %q", c.Model())
	}
}

func TestNewFromEnvStubWhenOnlyBaseURL(t *testing.T) {
	
	env := map[string]string{"LLM_BASE_URL": "https://api.openai.com/v1"}
	j := NewFromEnv(func(key string) string { return env[key] })
	if _, ok := j.(*Stub); !ok {
		t.Fatalf("expected *Stub when API_KEY missing, got %T", j)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}