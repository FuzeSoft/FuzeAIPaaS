package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newLLMTestStorage(t *testing.T) *Storage {
	t.Helper()
	
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&models.LLMRoute{}, &models.LLMPrice{}, &models.LLMTokenQuota{},
		&models.LLMUsageRecord{}, &models.LLMTrace{}, &models.LLMPrompt{},
		&models.LLMKnowledgeBase{}, &models.LLMDocument{}, &models.LLMAdapter{},
		&models.LLMAdapterMount{},
	); err != nil {
		t.Fatalf("migrate llm models: %v", err)
	}
	return &Storage{db: db}
}

func TestRouteSaveListDelete(t *testing.T) {
	s := newLLMTestStorage(t)
	ctx := context.Background()
	rt := llm.Route{
		Model:    "gpt-4o",
		Strategy: "priority",
		Backends: []llm.Backend{
			{Name: "us-east", Endpoint: "https://us", Weight: 10, Healthy: true},
			{Name: "eu-west", Endpoint: "https://eu", Weight: 5, Healthy: true},
		},
	}
	if err := s.Route().Save(ctx, "t1", rt); err != nil {
		t.Fatalf("save route: %v", err)
	}
	
	if err := s.Route().Save(ctx, "t1", rt); err != nil {
		t.Fatalf("upsert route: %v", err)
	}
	got, err := s.Route().List(ctx, "t1")
	if err != nil || len(got) != 1 {
		t.Fatalf("list route: %v len=%d", err, len(got))
	}
	if len(got[0].Backends) != 2 {
		t.Fatalf("expected 2 backends, got %d", len(got[0].Backends))
	}
	if err := s.Route().Delete(ctx, "t1", "gpt-4o"); err != nil {
		t.Fatalf("delete route: %v", err)
	}
	if _, err := s.Route().List(ctx, "t1"); err != nil {
		t.Fatalf("list after delete: %v", err)
	}
}

func TestQuotaAtomicConsume(t *testing.T) {
	s := newLLMTestStorage(t)
	ctx := context.Background()
	if err := s.TokenQuota().SetQuota(ctx, llm.TokenQuota{
		TenantID: "tq", LimitTokens: 100, UsedTokens: 0, LimitCost: 10, UsedCost: 0,
	}); err != nil {
		t.Fatalf("set quota: %v", err)
	}
	
	if err := s.TokenQuota().CheckAndConsume(ctx, "tq", 150, 0); err == nil {
		t.Fatal("expected quota exceeded error")
	}
	
	var wg sync.WaitGroup
	ok := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.TokenQuota().CheckAndConsume(ctx, "tq", 10, 1); err == nil {
				ok <- struct{}{}
			}
		}()
	}
	wg.Wait()
	close(ok)
	if len(ok) != 10 {
		t.Fatalf("expected 10 successful consumes, got %d", len(ok))
	}
	q, err := s.TokenQuota().GetQuota(ctx, "tq")
	if err != nil {
		t.Fatalf("get quota: %v", err)
	}
	if q.UsedTokens != 100 {
		t.Fatalf("expected 100 used tokens, got %d", q.UsedTokens)
	}
	if q.UsedCost != 10 {
		t.Fatalf("expected 10 used cost, got %f", q.UsedCost)
	}
}

func TestPromptVersionLifecycle(t *testing.T) {
	s := newLLMTestStorage(t)
	ctx := context.Background()
	p := &llm.Prompt{Name: "summarize", Versions: []llm.PromptVersion{{Version: 1, Content: "v1"}}, ActiveVersion: 1}
	if err := s.Prompt().Create(ctx, p, "tp", "u1"); err != nil {
		t.Fatalf("create prompt: %v", err)
	}
	got, err := s.Prompt().Get(ctx, "tp", "summarize")
	if err != nil || len(got.Versions) != 1 {
		t.Fatalf("get prompt: %v versions=%d", err, len(got.Versions))
	}
	
	got.Versions = append(got.Versions, llm.PromptVersion{Version: 2, Content: "v2"})
	got.ActiveVersion = 2
	if err := s.Prompt().Update(ctx, "tp", got); err != nil {
		t.Fatalf("update prompt: %v", err)
	}
	got2, _ := s.Prompt().Get(ctx, "tp", "summarize")
	if len(got2.Versions) != 2 || got2.ActiveVersion != 2 {
		t.Fatalf("expected 2 versions active=2, got %d active=%d", len(got2.Versions), got2.ActiveVersion)
	}
	if err := s.Prompt().Delete(ctx, "tp", "summarize"); err != nil {
		t.Fatalf("delete prompt: %v", err)
	}
}

func TestKnowledgeBaseAndDocument(t *testing.T) {
	s := newLLMTestStorage(t)
	ctx := context.Background()
	kb := &ports.KnowledgeBase{ID: "kb1", Name: "docs", TenantID: "t-kb", ChunkSize: 256, Overlap: 32}
	if err := s.Knowledge().CreateBase(ctx, kb); err != nil {
		t.Fatalf("create base: %v", err)
	}
	if kb.ID == "" {
		t.Fatal("knowledge base id not generated")
	}
	list, err := s.Knowledge().ListBases(ctx, "t-kb")
	if err != nil || len(list) != 1 {
		t.Fatalf("list bases: %v len=%d", err, len(list))
	}
	doc := &ports.KnowledgeDocument{BaseID: kb.ID, Title: "intro", Content: "hello world", Segments: 1, Status: "indexed"}
	if err := s.Knowledge().AddDocument(ctx, doc); err != nil {
		t.Fatalf("add document: %v", err)
	}
	docs, err := s.Knowledge().ListDocuments(ctx, kb.ID)
	if err != nil || len(docs) != 1 {
		t.Fatalf("list documents: %v len=%d", err, len(docs))
	}
	if err := s.Knowledge().DeleteBase(ctx, kb.ID); err != nil {
		t.Fatalf("delete base: %v", err)
	}
}

func TestTraceSaveGet(t *testing.T) {
	s := newLLMTestStorage(t)
	ctx := context.Background()
	tr := &llm.Trace{
		ID:         "trace-1",
		TenantID:   "t-tr",
		Model:      "gpt-4o",
		Spans:      []llm.Span{{Name: "complete", Elapsed: 100 * 1000 * 1000}},
		Usage:      llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		Cost:       0.01,
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
	}
	if err := s.Trace().Save(ctx, tr); err != nil {
		t.Fatalf("save trace: %v", err)
	}
	got, err := s.Trace().Get(ctx, "trace-1")
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if len(got.Spans) != 1 || got.Usage.TotalTokens != 15 {
		t.Fatalf("decode mismatch spans=%d total=%d", len(got.Spans), got.Usage.TotalTokens)
	}
}

func TestUsageRecordAndSum(t *testing.T) {
	s := newLLMTestStorage(t)
	ctx := context.Background()
	rec := &ports.TokenUsageRecord{TenantID: "t-u", Model: "gpt-4o", TotalTokens: 120, Cost: 2.5, Success: true}
	if err := s.TokenUsage().RecordUsage(ctx, rec); err != nil {
		t.Fatalf("record usage: %v", err)
	}
	usage, cost, err := s.TokenUsage().SumUsage(ctx, "t-u", 0, 0)
	if err != nil {
		t.Fatalf("sum usage: %v", err)
	}
	if usage.TotalTokens != 120 {
		t.Fatalf("expected 120 tokens, got %d", usage.TotalTokens)
	}
	if cost != 2.5 {
		t.Fatalf("expected 2.5 cost, got %f", cost)
	}
}