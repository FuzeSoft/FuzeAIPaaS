package api

import (
	"context"

	agentapp "fuze-ai-paas/backend/internal/app/agent"
	"fuze-ai-paas/backend/internal/app/llmgateway"
	"fuze-ai-paas/backend/internal/domain/event"
	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/events"
)

type llmGatewayCaller struct {
	gw *llmgateway.Service
}

func (c *llmGatewayCaller) Complete(ctx context.Context, req llm.ChatRequest, tenantID, userID, kbID string, topK int) (string, error) {
	res, err := c.gw.Complete(ctx, llmgateway.Request{
		Chat:            req,
		TenantID:        tenantID,
		UserID:          userID,
		KnowledgeBaseID: kbID,
		TopK:            topK,
	})
	if err != nil {
		return "", err
	}
	return res.Response.Text(), nil
}

var _ agentapp.LLMCaller = (*llmGatewayCaller)(nil)

type llmGatewayRetriever struct {
	retriever llmgateway.Retriever
}

func (r *llmGatewayRetriever) Retrieve(ctx context.Context, kbID, query string, topK int) ([]llm.ScoredSegment, error) {
	return r.retriever.Retrieve(ctx, kbID, query, topK)
}

var _ agentapp.Retriever = (*llmGatewayRetriever)(nil)

type eventSinkAdapter struct {
	bus *events.Bus
}

func (a *eventSinkAdapter) Notify(_ context.Context, e event.Event) error {
	if a.bus == nil {
		return nil
	}
	a.bus.Publish(e)
	return nil
}

var _ agentapp.EventPublisher = (*eventSinkAdapter)(nil)