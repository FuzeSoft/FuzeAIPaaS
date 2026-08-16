package llmgateway

import (
	"context"
	"strings"
	"sync"
	"testing"

	"fuze-ai-paas/backend/internal/domain/llm"
)

type recordingCompleter struct {
	mu   sync.Mutex
	sent []llm.ChatRequest
	text string
}

func (c *recordingCompleter) Complete(_ context.Context, _ string, req llm.ChatRequest) (llm.ChatResponse, error) {
	c.mu.Lock()
	c.sent = append(c.sent, req)
	c.mu.Unlock()
	return llm.ChatResponse{
		ID:      "x",
		Model:   req.Model,
		Choices: []llm.Choice{{Message: llm.Message{Role: llm.RoleAssistant, Content: c.text}}},
	}, nil
}

func (c *recordingCompleter) Stream(_ context.Context, _ string, req llm.ChatRequest, onChunk func(llm.Chunk) error) (llm.Usage, error) {
	c.mu.Lock()
	c.sent = append(c.sent, req)
	text := c.text
	c.mu.Unlock()
	if err := onChunk(llm.Chunk{Delta: text}); err != nil {
		return llm.Usage{}, err
	}
	return llm.Usage{}, nil
}

func (c *recordingCompleter) last() llm.ChatRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sent) == 0 {
		return llm.ChatRequest{}
	}
	return c.sent[len(c.sent)-1]
}

type stubResolver struct {
	mu    sync.Mutex
	rules map[string][]llm.Rule
	calls int
}

func (s *stubResolver) Resolve(_ context.Context, tenantID string) ([]llm.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if r, ok := s.rules[tenantID]; ok {
		return r, nil
	}
	return llm.DefaultRules(), nil
}

func (s *stubResolver) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func testRoutes(t *testing.T) RouteTable {
	t.Helper()
	rt := llm.NewRouteTable()
	if err := rt.Upsert(llm.Route{
		Model:    "m1",
		Backends: []llm.Backend{{Name: "http://b", Endpoint: "http://b", Weight: 1, Healthy: true}},
	}); err != nil {
		t.Fatalf("upsert route: %v", err)
	}
	return rt
}

func chatReq(tenant, content string) Request {
	return Request{
		TenantID: tenant,
		Chat: llm.ChatRequest{
			Model:    "m1",
			Messages: []llm.Message{{Role: llm.RoleUser, Content: content}},
		},
	}
}

func TestGuardProviderPerTenantRules(t *testing.T) {
	resolver := &stubResolver{rules: map[string][]llm.Rule{
		"t-emp": {{
			Name: "emp_id", Category: llm.CategoryPII, Direction: llm.DirectionBoth,
			Action: llm.ActionRedact, Pattern: `EMP-\d{6}`, Replacement: "[EMP]",
		}},
	}}
	comp := &recordingCompleter{text: "ok"}
	svc, err := NewService(Deps{
		Completer: comp,
		Routes:    testRoutes(t),
		Guards:    NewGuardCache(resolver),
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	t.Run("租户自定义规则生效", func(t *testing.T) {
		if _, err := svc.Complete(context.Background(), chatReq("t-emp", "工号 EMP-123456")); err != nil {
			t.Fatalf("complete: %v", err)
		}
		got := comp.last().Messages[0].Content
		if strings.Contains(got, "EMP-123456") {
			t.Fatalf("租户自定义脱敏未生效: %q", got)
		}
	})

	t.Run("其他租户回退内建规则", func(t *testing.T) {
		if _, err := svc.Complete(context.Background(), chatReq("t-other", "手机 13812345678")); err != nil {
			t.Fatalf("complete: %v", err)
		}
		got := comp.last().Messages[0].Content
		if strings.Contains(got, "13812345678") {
			t.Fatalf("内建兜底规则未生效: %q", got)
		}
	})

	t.Run("租户规则不串用", func(t *testing.T) {
		
		if _, err := svc.Complete(context.Background(), chatReq("t-emp", "手机 13800001111")); err != nil {
			t.Fatalf("complete: %v", err)
		}
		got := comp.last().Messages[0].Content
		if !strings.Contains(got, "13800001111") {
			t.Fatalf("t-emp 未配置手机号规则，不应被脱敏: %q", got)
		}
	})
}

func TestGuardCacheAvoidsRecompile(t *testing.T) {
	resolver := &stubResolver{}
	cache := NewGuardCache(resolver)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := cache.GuardFor(ctx, "t-1"); err != nil {
			t.Fatalf("guard for: %v", err)
		}
	}
	if n := resolver.callCount(); n != 1 {
		t.Fatalf("规则应只解析一次，实际 %d 次", n)
	}

	if _, err := cache.GuardFor(ctx, "t-2"); err != nil {
		t.Fatalf("guard for: %v", err)
	}
	if n := resolver.callCount(); n != 2 {
		t.Fatalf("不同租户应各自解析，实际 %d 次", n)
	}
}

func TestGuardCacheInvalidate(t *testing.T) {
	resolver := &stubResolver{rules: map[string][]llm.Rule{"t-1": {}}}
	cache := NewGuardCache(resolver)
	ctx := context.Background()

	if _, err := cache.GuardFor(ctx, "t-1"); err != nil {
		t.Fatalf("guard for: %v", err)
	}

	resolver.mu.Lock()
	resolver.rules["t-1"] = []llm.Rule{{
		Name: "secret", Category: llm.CategorySensitive, Direction: llm.DirectionBoth,
		Action: llm.ActionBlock, Keywords: []string{"绝密"},
	}}
	resolver.mu.Unlock()

	cache.Invalidate("t-1")

	g, err := cache.GuardFor(ctx, "t-1")
	if err != nil {
		t.Fatalf("guard for: %v", err)
	}
	if !g.Check("这是绝密内容", llm.DirectionInput).Blocked() {
		t.Fatal("缓存失效后新规则未生效")
	}
}

func TestGuardCacheConcurrent(t *testing.T) {
	cache := NewGuardCache(&stubResolver{})
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := cache.GuardFor(ctx, "shared"); err != nil {
				t.Errorf("guard for: %v", err)
			}
			cache.Invalidate("shared")
		}()
	}
	wg.Wait()
}