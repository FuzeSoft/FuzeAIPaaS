package llmgateway

import (
	"context"
	"errors"
	"fmt"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/ports"
)

type KeywordSearcher interface {
	SearchKeyword(ctx context.Context, collection, query string, topK int) ([]ports.VectorHit, error)
}

type KnowledgeService struct {
	repo     ports.KnowledgeRepository
	store    ports.VectorStore
	embedder ports.Embedder
	reranker ports.Reranker
	
	embedEndpoint string
	newID         IDGen
}

func NewKnowledgeService(repo ports.KnowledgeRepository, store ports.VectorStore, embedder ports.Embedder, reranker ports.Reranker, embedEndpoint string, newID IDGen) (*KnowledgeService, error) {
	if repo == nil {
		return nil, errors.New("llmgateway: knowledge repository is required")
	}
	if store == nil {
		return nil, errors.New("llmgateway: vector store is required")
	}
	if reranker == nil {
		reranker = llmPassthrough{}
	}
	if newID == nil {
		newID = defaultIDGen()
	}
	return &KnowledgeService{
		repo: repo, store: store, embedder: embedder,
		reranker: reranker, embedEndpoint: embedEndpoint, newID: newID,
	}, nil
}

type llmPassthrough struct{}

func (llmPassthrough) Rerank(_ context.Context, _ string, hits []llm.ScoredSegment) ([]llm.ScoredSegment, error) {
	return hits, nil
}

var (
	
	ErrKnowledgeBaseNotFound = errors.New("llmgateway: knowledge base not found")
	
	ErrEmptyBaseName = errors.New("llmgateway: knowledge base name must not be empty")
)

type CreateBaseInput struct {
	Name           string
	TenantID       string
	CreatedBy      string
	EmbeddingModel string
	Dimension      int
	ChunkSize      int
	Overlap        int
}

func (s *KnowledgeService) CreateBase(ctx context.Context, in CreateBaseInput) (*ports.KnowledgeBase, error) {
	if in.Name == "" {
		return nil, ErrEmptyBaseName
	}
	opt := llm.DefaultChunkOptions()
	if in.ChunkSize > 0 {
		opt.Size = in.ChunkSize
	}
	if in.Overlap >= 0 {
		opt.Overlap = in.Overlap
	}
	
	if err := opt.Validate(); err != nil {
		return nil, err
	}

	kb := &ports.KnowledgeBase{
		ID: s.newID(), Name: in.Name, TenantID: in.TenantID,
		EmbeddingModel: in.EmbeddingModel, Dimension: in.Dimension,
		ChunkSize: opt.Size, Overlap: opt.Overlap, CreatedBy: in.CreatedBy,
	}
	if err := s.repo.CreateBase(ctx, kb); err != nil {
		return nil, err
	}
	return kb, nil
}

func (s *KnowledgeService) GetBase(ctx context.Context, id string) (*ports.KnowledgeBase, error) {
	kb, err := s.repo.GetBase(ctx, id)
	if err != nil {
		return nil, err
	}
	if kb == nil {
		return nil, ErrKnowledgeBaseNotFound
	}
	return kb, nil
}

func (s *KnowledgeService) ListBases(ctx context.Context, tenantID string) ([]*ports.KnowledgeBase, error) {
	return s.repo.ListBases(ctx, tenantID)
}

func (s *KnowledgeService) DeleteBase(ctx context.Context, id string) error {
	if _, err := s.GetBase(ctx, id); err != nil {
		return err
	}
	
	if err := s.store.DropCollection(ctx, id); err != nil {
		return fmt.Errorf("llmgateway: drop vectors: %w", err)
	}
	return s.repo.DeleteBase(ctx, id)
}

type AddDocumentInput struct {
	BaseID  string
	Title   string
	Source  string
	Content string
}

func (s *KnowledgeService) AddDocument(ctx context.Context, in AddDocumentInput) (*ports.KnowledgeDocument, error) {
	kb, err := s.GetBase(ctx, in.BaseID)
	if err != nil {
		return nil, err
	}

	opt := llm.DefaultChunkOptions()
	if kb.ChunkSize > 0 {
		opt.Size = kb.ChunkSize
		opt.Overlap = kb.Overlap
	}
	segments, err := llm.SplitText(in.Content, opt)
	if err != nil {
		return nil, err
	}

	doc := &ports.KnowledgeDocument{
		ID: s.newID(), BaseID: in.BaseID, Title: in.Title, Source: in.Source,
		Content: in.Content, Segments: len(segments), Status: "indexed",
	}

	items := make([]ports.VectorItem, 0, len(segments))
	for _, seg := range segments {
		items = append(items, ports.VectorItem{
			ID:         fmt.Sprintf("%s#%d", doc.ID, seg.Index),
			DocumentID: doc.ID, Title: in.Title, Segment: seg,
		})
	}

	if s.embedder != nil {
		texts := make([]string, len(segments))
		for i, seg := range segments {
			texts[i] = seg.Content
		}
		resp, eerr := s.embedder.Embed(ctx, s.embedEndpoint, llm.EmbeddingRequest{
			Model: kb.EmbeddingModel, Input: texts,
		})
		if eerr != nil {
			doc.Status = "indexed_without_vectors"
		} else {
			for _, e := range resp.Data {
				if e.Index >= 0 && e.Index < len(items) {
					items[e.Index].Vector = e.Vector
				}
			}
		}
	}

	if err := s.store.Upsert(ctx, in.BaseID, items); err != nil {
		return nil, fmt.Errorf("llmgateway: upsert vectors: %w", err)
	}
	if err := s.repo.AddDocument(ctx, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *KnowledgeService) ListDocuments(ctx context.Context, baseID string) ([]*ports.KnowledgeDocument, error) {
	if _, err := s.GetBase(ctx, baseID); err != nil {
		return nil, err
	}
	return s.repo.ListDocuments(ctx, baseID)
}

func (s *KnowledgeService) DeleteDocument(ctx context.Context, baseID, docID string) error {
	if err := s.store.Delete(ctx, baseID, docID); err != nil {
		return fmt.Errorf("llmgateway: delete vectors: %w", err)
	}
	return s.repo.DeleteDocument(ctx, docID)
}

func (s *KnowledgeService) Retrieve(ctx context.Context, baseID, query string, topK int) ([]llm.ScoredSegment, error) {
	if query == "" {
		return nil, nil
	}
	kb, err := s.GetBase(ctx, baseID)
	if err != nil {
		return nil, err
	}
	if topK <= 0 {
		topK = defaultTopK
	}

	var hits []ports.VectorHit
	if s.embedder != nil {
		resp, eerr := s.embedder.Embed(ctx, s.embedEndpoint, llm.EmbeddingRequest{
			Model: kb.EmbeddingModel, Input: []string{query},
		})
		if eerr == nil && len(resp.Data) > 0 {
			
			hits, err = s.store.Search(ctx, baseID, resp.Data[0].Vector, topK*3)
			if err != nil {
				return nil, err
			}
		}
	}

	if len(hits) == 0 {
		if ks, ok := s.store.(KeywordSearcher); ok {
			hits, err = ks.SearchKeyword(ctx, baseID, query, topK*3)
			if err != nil {
				return nil, err
			}
		}
	}

	scored := make([]llm.ScoredSegment, 0, len(hits))
	for _, h := range hits {
		scored = append(scored, llm.ScoredSegment{
			DocumentID: h.Item.DocumentID, Title: h.Item.Title,
			Segment: h.Item.Segment, Score: h.Score,
		})
	}

	reranked, err := s.reranker.Rerank(ctx, query, scored)
	if err != nil {
		
		reranked = scored
	}
	llm.RankByScore(reranked)
	return llm.TopK(reranked, topK), nil
}