
package llmgw

import (
	"context"
	"sort"
	"strings"
	"sync"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/ports"
)

type MemoryVectorStore struct {
	mu sync.RWMutex
	
	collections map[string]map[string]ports.VectorItem
}

func NewMemoryVectorStore() *MemoryVectorStore {
	return &MemoryVectorStore{collections: make(map[string]map[string]ports.VectorItem)}
}

func (s *MemoryVectorStore) Upsert(_ context.Context, collection string, items []ports.VectorItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.collections[collection]
	if !ok {
		c = make(map[string]ports.VectorItem)
		s.collections[collection] = c
	}
	for _, it := range items {
		
		if it.Vector != nil {
			v := make([]float32, len(it.Vector))
			copy(v, it.Vector)
			it.Vector = v
		}
		c[it.ID] = it
	}
	return nil
}

func (s *MemoryVectorStore) Search(_ context.Context, collection string, vector []float32, topK int) ([]ports.VectorHit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.collections[collection]
	if !ok {
		
		return nil, nil
	}

	hits := make([]ports.VectorHit, 0, len(c))
	for _, it := range c {
		if len(vector) == 0 || len(it.Vector) == 0 {
			continue
		}
		score, err := llm.CosineSimilarity(vector, it.Vector)
		if err != nil {
			
			continue
		}
		hits = append(hits, ports.VectorHit{Item: it, Score: score})
	}

	sortHits(hits)
	if topK > 0 && len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, nil
}

func (s *MemoryVectorStore) SearchKeyword(_ context.Context, collection, query string, topK int) ([]ports.VectorHit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.collections[collection]
	if !ok {
		return nil, nil
	}
	hits := make([]ports.VectorHit, 0, len(c))
	for _, it := range c {
		score := llm.KeywordScore(query, it.Segment.Content)
		if score <= 0 {
			continue
		}
		hits = append(hits, ports.VectorHit{Item: it, Score: score})
	}
	sortHits(hits)
	if topK > 0 && len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, nil
}

func (s *MemoryVectorStore) Delete(_ context.Context, collection, documentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.collections[collection]
	if !ok {
		return nil
	}
	for id, it := range c {
		if it.DocumentID == documentID {
			delete(c, id)
		}
	}
	return nil
}

func (s *MemoryVectorStore) DropCollection(_ context.Context, collection string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.collections, collection)
	return nil
}

func (s *MemoryVectorStore) Count(collection string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.collections[collection])
}

func sortHits(hits []ports.VectorHit) {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Item.ID < hits[j].Item.ID
	})
}

type HashEmbedder struct {
	
	Dim int
}

func NewHashEmbedder(dim int) *HashEmbedder {
	if dim <= 0 {
		dim = 256
	}
	return &HashEmbedder{Dim: dim}
}

func (e *HashEmbedder) Embed(_ context.Context, _ string, req llm.EmbeddingRequest) (llm.EmbeddingResponse, error) {
	if err := req.Validate(); err != nil {
		return llm.EmbeddingResponse{}, err
	}
	resp := llm.EmbeddingResponse{Model: req.Model, Data: make([]llm.Embedding, 0, len(req.Input))}
	total := 0
	for i, text := range req.Input {
		resp.Data = append(resp.Data, llm.Embedding{Index: i, Vector: e.vector(text)})
		total += llm.EstimateTokens(text)
	}
	resp.Usage = llm.Usage{PromptTokens: total, TotalTokens: total}
	return resp, nil
}

func (e *HashEmbedder) Vector(text string) []float32 { return e.vector(text) }

func (e *HashEmbedder) vector(text string) []float32 {
	v := make([]float32, e.Dim)
	for _, gram := range grams(strings.ToLower(text)) {
		v[llm.HashSeed(gram)%uint64(e.Dim)]++
	}
	
	var norm float32
	for _, x := range v {
		norm += x * x
	}
	if norm == 0 {
		return v
	}
	inv := 1 / sqrt32(norm)
	for i := range v {
		v[i] *= inv
	}
	return v
}

func grams(s string) []string {
	var out []string
	var latin strings.Builder
	var cjk []rune

	flushLatin := func() {
		if latin.Len() > 0 {
			out = append(out, latin.String())
			latin.Reset()
		}
	}
	flushCJK := func() {
		if len(cjk) == 1 {
			out = append(out, string(cjk))
		}
		for i := 0; i+1 < len(cjk); i++ {
			out = append(out, string(cjk[i:i+2]))
		}
		cjk = cjk[:0]
	}

	for _, r := range s {
		switch {
		case isCJKRune(r):
			flushLatin()
			cjk = append(cjk, r)
		case isWordRune(r):
			flushCJK()
			latin.WriteRune(r)
		default:
			flushLatin()
			flushCJK()
		}
	}
	flushLatin()
	flushCJK()
	return out
}

func isCJKRune(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x3040 && r <= 0x30FF) || (r >= 0xAC00 && r <= 0xD7AF)
}

func isWordRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func sqrt32(x float32) float32 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 12; i++ {
		z = (z + x/z) / 2
	}
	return z
}

type PassthroughReranker struct{}

func (PassthroughReranker) Rerank(_ context.Context, _ string, hits []llm.ScoredSegment) ([]llm.ScoredSegment, error) {
	return hits, nil
}

type KeywordReranker struct {
	
	Weight float64
}

func (r KeywordReranker) Rerank(_ context.Context, query string, hits []llm.ScoredSegment) ([]llm.ScoredSegment, error) {
	w := r.Weight
	if w <= 0 || w > 1 {
		w = 0.3
	}
	out := make([]llm.ScoredSegment, len(hits))
	copy(out, hits)
	for i := range out {
		kw := llm.KeywordScore(query, out[i].Segment.Content)
		out[i].Score = out[i].Score*(1-w) + kw*w
	}
	llm.RankByScore(out)
	return out, nil
}