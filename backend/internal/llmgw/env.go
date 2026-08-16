package llmgw

import (
	"os"
	"time"

	"fuze-ai-paas/backend/internal/ports"
)

func NewCompleterFromEnv(getenv func(string) string) ports.ChatCompleter {
	baseURL := getenv("LLM_BASE_URL")
	apiKey := getenv("LLM_API_KEY")
	if baseURL == "" || apiKey == "" {
		return nil
	}
	
	return NewOpenAIClient(60*time.Second, apiKey)
}

func NewCompleterFromOS() ports.ChatCompleter { return NewCompleterFromEnv(os.Getenv) }