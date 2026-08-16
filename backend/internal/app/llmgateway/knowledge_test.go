package llmgateway

import (
	"context"
	"strings"
	"sync"
	"testing"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/llmgw"
	"fuze-ai-paas/backend/internal/ports"
)

type memKnowledgeRepo struct {
	mu    sync.Mutex
	bases map[string]*ports.KnowledgeBase
	docs  map[string]*ports.KnowledgeDocument
}

func newMemKnowledgeRepo() *memKnowledgeRepo {
	return &memKnowledgeRepo{
		bases: map[string]*ports.KnowledgeBase{},
		docs:  map[string]*ports.KnowledgeDocument{},
	}
}

func (r *memKnowledgeRepo) CreateBase(_ context.Context, kb *ports.KnowledgeBase) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bases[kb.ID] = kb
	return nil
}
func (r *memKnowledgeRepo) GetBase(_ context.Context, id string) (*ports.KnowledgeBase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bases[id], nil
}
func (r *memKnowledgeRepo) ListBases(_ context.Context, tenantID string) ([]*ports.KnowledgeBase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*ports.KnowledgeBase
	for _, kb := range r.bases {
		if tenantID == "" || kb.TenantID == tenantID {
			out = append(out, kb)
		}
	}
	return out, nil
}
func (r *memKnowledgeRepo) DeleteBase(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.bases, id)
	return nil
}
func (r *memKnowledgeRepo) AddDocument(_ context.Context, doc *ports.KnowledgeDocument) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.docs[doc.ID] = doc
	return nil
}
func (r *memKnowledgeRepo) GetDocument(_ context.Context, id string) (*ports.KnowledgeDocument, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.docs[id], nil
}
func (r *memKnowledgeRepo) ListDocuments(_ context.Context, baseID string) ([]*ports.KnowledgeDocument, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*ports.KnowledgeDocument
	for _, d := range r.docs {
		if d.BaseID == baseID {
			out = append(out, d)
		}
	}
	return out, nil
}
func (r *memKnowledgeRepo) DeleteDocument(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.docs, id)
	return nil
}

func newKnowledgeSvc(t *testing.T) (*KnowledgeService, *memKnowledgeRepo, *llmgw.MemoryVectorStore) {
	t.Helper()
	repo := newMemKnowledgeRepo()
	store := llmgw.NewMemoryVectorStore()
	embedder := llmgw.NewHashEmbedder(256)
	var seq int
	idgen := func() string { seq++; return "id-" + string(rune('0'+seq)) }

	svc, err := NewKnowledgeService(repo, store, embedder, llmgw.KeywordReranker{Weight: 0.3}, "", idgen)
	if err != nil {
		t.Fatalf("NewKnowledgeService: %v", err)
	}
	return svc, repo, store
}

func TestKnowledgeServiceRequiresDeps(t *testing.T) {
	store := llmgw.NewMemoryVectorStore()
	if _, err := NewKnowledgeService(nil, store, nil, nil, "", nil); err == nil {
		t.Fatal("expected error without repo")
	}
	if _, err := NewKnowledgeService(newMemKnowledgeRepo(), nil, nil, nil, "", nil); err == nil {
		t.Fatal("expected error without store")
	}
}

func TestCreateBaseValidation(t *testing.T) {
	svc, _, _ := newKnowledgeSvc(t)
	if _, err := svc.CreateBase(context.Background(), CreateBaseInput{}); err != ErrEmptyBaseName {
		t.Fatalf("want ErrEmptyBaseName, got %v", err)
	}
	
	if _, err := svc.CreateBase(context.Background(), CreateBaseInput{
		Name: "kb", ChunkSize: 10, Overlap: 20,
	}); err != llm.ErrInvalidOverlap {
		t.Fatalf("want ErrInvalidOverlap, got %v", err)
	}
}

func TestCreateBaseDefaults(t *testing.T) {
	svc, _, _ := newKnowledgeSvc(t)
	kb, err := svc.CreateBase(context.Background(), CreateBaseInput{Name: "kb", TenantID: "t1"})
	if err != nil {
		t.Fatalf("CreateBase: %v", err)
	}
	if kb.ChunkSize == 0 {
		t.Fatal("chunk size not defaulted")
	}
}

func TestGetBaseNotFound(t *testing.T) {
	svc, _, _ := newKnowledgeSvc(t)
	if _, err := svc.GetBase(context.Background(), "ghost"); err != ErrKnowledgeBaseNotFound {
		t.Fatalf("want ErrKnowledgeBaseNotFound, got %v", err)
	}
}

func TestAddDocumentIndexesSegments(t *testing.T) {
	ctx := context.Background()
	svc, _, store := newKnowledgeSvc(t)
	kb, _ := svc.CreateBase(ctx, CreateBaseInput{Name: "kb", ChunkSize: 30, Overlap: 5})

	content := strings.Repeat("推理网关负责统一流量入口。", 20)
	doc, err := svc.AddDocument(ctx, AddDocumentInput{BaseID: kb.ID, Title: "手册", Content: content})
	if err != nil {
		t.Fatalf("AddDocument: %v", err)
	}
	if doc.Segments < 2 {
		t.Fatalf("segments = %d, want >= 2", doc.Segments)
	}
	if store.Count(kb.ID) != doc.Segments {
		t.Fatalf("indexed %d vectors, want %d", store.Count(kb.ID), doc.Segments)
	}
}

func TestRetrieveFindsRelevantContent(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newKnowledgeSvc(t)
	kb, _ := svc.CreateBase(ctx, CreateBaseInput{Name: "kb", EmbeddingModel: "stub"})

	_, _ = svc.AddDocument(ctx, AddDocumentInput{
		BaseID: kb.ID, Title: "网关", Content: "推理网关负责多模型路由与故障转移和统一鉴权",
	})
	_, _ = svc.AddDocument(ctx, AddDocumentInput{
		BaseID: kb.ID, Title: "天气", Content: "今天天气晴朗适合出门散步游玩",
	})

	hits, err := svc.Retrieve(ctx, kb.ID, "推理网关路由", 1)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("no results retrieved")
	}
	if hits[0].Title != "网关" {
		t.Fatalf("top hit = %q, want 网关", hits[0].Title)
	}
}

func TestRetrieveFallsBackToKeyword(t *testing.T) {
	ctx := context.Background()
	repo := newMemKnowledgeRepo()
	store := llmgw.NewMemoryVectorStore()
	
	svc, err := NewKnowledgeService(repo, store, nil, nil, "", func() string { return "d1" })
	if err != nil {
		t.Fatalf("NewKnowledgeService: %v", err)
	}

	kb, _ := svc.CreateBase(ctx, CreateBaseInput{Name: "kb"})
	_, _ = svc.AddDocument(ctx, AddDocumentInput{
		BaseID: kb.ID, Title: "网关", Content: "推理网关支持多模型路由",
	})

	hits, err := svc.Retrieve(ctx, kb.ID, "推理网关路由", 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("keyword fallback returned nothing")
	}
}

func TestRetrieveEmptyQuery(t *testing.T) {
	svc, _, _ := newKnowledgeSvc(t)
	hits, err := svc.Retrieve(context.Background(), "kb", "", 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if hits != nil {
		t.Fatal("empty query should return nil")
	}
}

func TestDeleteBaseRemovesVectors(t *testing.T) {
	ctx := context.Background()
	svc, repo, store := newKnowledgeSvc(t)
	kb, _ := svc.CreateBase(ctx, CreateBaseInput{Name: "kb", EmbeddingModel: "stub"})
	_, _ = svc.AddDocument(ctx, AddDocumentInput{BaseID: kb.ID, Content: "some content here"})

	if err := svc.DeleteBase(ctx, kb.ID); err != nil {
		t.Fatalf("DeleteBase: %v", err)
	}
	if store.Count(kb.ID) != 0 {
		t.Fatal("vectors not removed on base deletion")
	}
	repo.mu.Lock()
	_, exists := repo.bases[kb.ID]
	repo.mu.Unlock()
	if exists {
		t.Fatal("base metadata not removed")
	}
}

func TestDeleteDocumentRemovesVectors(t *testing.T) {
	ctx := context.Background()
	svc, _, store := newKnowledgeSvc(t)
	kb, _ := svc.CreateBase(ctx, CreateBaseInput{Name: "kb", EmbeddingModel: "stub"})
	doc, _ := svc.AddDocument(ctx, AddDocumentInput{BaseID: kb.ID, Content: "content to be deleted later"})

	if err := svc.DeleteDocument(ctx, kb.ID, doc.ID); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	if store.Count(kb.ID) != 0 {
		t.Fatal("document vectors not removed")
	}
}

var _ Retriever = (*KnowledgeService)(nil)