package llmgateway

import (
	"context"
	"sync"
	"time"

	"fuze-ai-paas/backend/internal/domain/llm"
)

const guardTTL = 5 * time.Minute

type GuardProvider interface {
	
	GuardFor(ctx context.Context, tenantID string) (*llm.Guard, error)
}

type RuleResolver interface {
	Resolve(ctx context.Context, tenantID string) ([]llm.Rule, error)
}

type GuardCache struct {
	resolver RuleResolver

	mu     sync.RWMutex
	guards map[string]cachedGuard
}

type cachedGuard struct {
	guard *llm.Guard
	at    time.Time
}

func NewGuardCache(resolver RuleResolver) *GuardCache {
	return &GuardCache{
		resolver: resolver,
		guards:   make(map[string]cachedGuard),
	}
}

func (c *GuardCache) GuardFor(ctx context.Context, tenantID string) (*llm.Guard, error) {
	c.mu.RLock()
	if item, ok := c.guards[tenantID]; ok && time.Since(item.at) < guardTTL {
		c.mu.RUnlock()
		return item.guard, nil
	}
	c.mu.RUnlock()

	rules, err := c.resolver.Resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	guard := llm.NewGuard(rules...)

	c.mu.Lock()
	defer c.mu.Unlock()
	
	if existing, ok := c.guards[tenantID]; ok && time.Since(existing.at) < guardTTL {
		return existing.guard, nil
	}
	c.guards[tenantID] = cachedGuard{guard: guard, at: time.Now()}
	return guard, nil
}

func (c *GuardCache) Invalidate(tenantID string) {
	c.mu.Lock()
	delete(c.guards, tenantID)
	c.mu.Unlock()
}

func (c *GuardCache) InvalidateAll() {
	c.mu.Lock()
	c.guards = make(map[string]cachedGuard)
	c.mu.Unlock()
}

type staticGuardProvider struct {
	guard *llm.Guard
}

func (p staticGuardProvider) GuardFor(context.Context, string) (*llm.Guard, error) {
	return p.guard, nil
}