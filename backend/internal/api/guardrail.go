package api

import "fuze-ai-paas/backend/internal/domain/llm"

func defaultGuard() *llm.Guard {
	return llm.NewDefaultGuard()
}