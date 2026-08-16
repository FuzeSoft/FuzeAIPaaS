package llmjudge

import (
	"os"

	"fuze-ai-paas/backend/internal/ports"
)

func NewFromEnv(getenv func(string) string) ports.JudgeLLM {
	baseURL := getenv("LLM_BASE_URL")
	apiKey := getenv("LLM_API_KEY")
	if baseURL != "" && apiKey != "" {
		if c, err := New(Config{
			BaseURL: baseURL,
			APIKey:  apiKey,
			Model:   getenv("LLM_MODEL"),
		}); err == nil {
			return c
		}
	}
	return NewStub("stub")
}

func NewFromOS() ports.JudgeLLM { return NewFromEnv(os.Getenv) }