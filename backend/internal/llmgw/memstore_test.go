package llmgw

import (
	"context"
	"sync"
	"testing"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/ports"
)

func item(id, doc string, vec []float32, content string) ports.VectorItem {
	return ports.VectorItem{
		ID: id, DocumentID: doc, Title: "T-" + doc,
		Segment: llm.Segment{Content: content}, Vector: vec,
	}
}

func TestMemoryVectorStoreUpsertAndSearch(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryVectorStore()

	err := s.Upsert(ctx, "kb1", []ports.VectorItem{
		item("1", "d1", []float32{1, 0}, "alpha"),
		item("2", "d2", []float32{0, 1}, "beta"),
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	hits, err := s.Search(ctx, "kb1", []float32{1, 0}, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(hits))
	}
	if hits[0].Item.ID != "1" {
		t.Fatalf("top hit = %q, want 1", hits[0].Item.ID)
	}
}

func TestMemoryVectorStoreSearchMissingCollection(t *testing.T) {
	hits, err := NewMemoryVectorStore().Search(context.Background(), "ghost", []float32{1}, 5)
	if err != nil {
		t.Fatalf("Search on missing collection: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits = %d, want 0", len(hits))
	}
}

func TestMemoryVectorStoreTopK(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryVectorStore()
	_ = s.Upsert(ctx, "kb", []ports.VectorItem{
		item("1", "d", []float32{1, 0}, "a"),
		item("2", "d", []float32{0.9, 0.1}, "b"),
		item("3", "d", []float32{0.8, 0.2}, "c"),
	})
	hits, _ := s.Search(ctx, "kb", []float32{1, 0}, 2)
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(hits))
	}
}

func TestMemoryVectorStoreSkipsDimensionMismatch(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryVectorStore()
	_ = s.Upsert(ctx, "kb", []ports.VectorItem{
		item("good", "d", []float32{1, 0}, "a"),
		item("bad", "d", []float32{1, 0, 0}, "b"),
	})
	hits, err := s.Search(ctx, "kb", []float32{1, 0}, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].Item.ID != "good" {
		t.Fatalf("hits = %+v, want only 'good'", hits)
	}
}

func TestMemoryVectorStoreCopiesVector(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryVectorStore()
	vec := []float32{1, 0}
	_ = s.Upsert(ctx, "kb", []ports.VectorItem{item("1", "d", vec, "a")})

	vec[0] = 99
	hits, _ := s.Search(ctx, "kb", []float32{1, 0}, 1)
	if len(hits) != 1 {
		t.Fatal("item missing")
	}
	if hits[0].Item.Vector[0] != 1 {
		t.Fatalf("store aliased caller slice: %v", hits[0].Item.Vector)
	}
}

func TestMemoryVectorStoreUpsertOverwrites(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryVectorStore()
	_ = s.Upsert(ctx, "kb", []ports.VectorItem{item("1", "d", []float32{1, 0}, "old")})
	_ = s.Upsert(ctx, "kb", []ports.VectorItem{item("1", "d", []float32{1, 0}, "new")})

	if got := s.Count("kb"); got != 1 {
		t.Fatalf("Count = %d, want 1 (upsert must overwrite)", got)
	}
	hits, _ := s.Search(ctx, "kb", []float32{1, 0}, 1)
	if hits[0].Item.Segment.Content != "new" {
		t.Fatalf("content = %q, want new", hits[0].Item.Segment.Content)
	}
}

func TestMemoryVectorStoreDelete(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryVectorStore()
	_ = s.Upsert(ctx, "kb", []ports.VectorItem{
		item("1", "d1", []float32{1, 0}, "a"),
		item("2", "d1", []float32{1, 0}, "b"),
		item("3", "d2", []float32{1, 0}, "c"),
	})

	if err := s.Delete(ctx, "kb", "d1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := s.Count("kb"); got != 1 {
		t.Fatalf("Count = %d, want 1", got)
	}
	
	if err := s.Delete(ctx, "ghost", "d1"); err != nil {
		t.Fatalf("Delete on missing collection: %v", err)
	}
}

func TestMemoryVectorStoreDropCollection(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryVectorStore()
	_ = s.Upsert(ctx, "kb", []ports.VectorItem{item("1", "d", []float32{1}, "a")})
	_ = s.DropCollection(ctx, "kb")
	if got := s.Count("kb"); got != 0 {
		t.Fatalf("Count = %d, want 0", got)
	}
}

func TestMemoryVectorStoreKeywordFallback(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryVectorStore()
	_ = s.Upsert(ctx, "kb", []ports.VectorItem{
		item("1", "d1", nil, "平台支持知识库检索"),
		item("2", "d2", nil, "今天天气不错"),
	})

	hits, err := s.SearchKeyword(ctx, "kb", "知识库检索", 10)
	if err != nil {
		t.Fatalf("SearchKeyword: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("keyword fallback returned nothing")
	}
	if hits[0].Item.ID != "1" {
		t.Fatalf("top hit = %q, want 1", hits[0].Item.ID)
	}
}

func TestMemoryVectorStoreConcurrent(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryVectorStore()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			_ = s.Upsert(ctx, "kb", []ports.VectorItem{item(string(rune('a'+n%26)), "d", []float32{1, 0}, "x")})
		}(i)
		go func() {
			defer wg.Done()
			_, _ = s.Search(ctx, "kb", []float32{1, 0}, 5)
		}()
	}
	wg.Wait()
}

func TestHashEmbedderIsDeterministic(t *testing.T) {
	e := NewHashEmbedder(64)
	a := e.Vector("知识库检索能力")
	b := e.Vector("知识库检索能力")
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("vectors differ at %d: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestHashEmbedderSimilarityOrdering(t *testing.T) {
	e := NewHashEmbedder(512)
	q := e.Vector("知识库检索")
	near := e.Vector("平台支持知识库检索功能")
	far := e.Vector("今天天气很好适合出门散步")

	sNear, err := llm.CosineSimilarity(q, near)
	if err != nil {
		t.Fatalf("CosineSimilarity: %v", err)
	}
	sFar, _ := llm.CosineSimilarity(q, far)
	if sNear <= sFar {
		t.Fatalf("similar text scored %v <= unrelated %v", sNear, sFar)
	}
}

func TestHashEmbedderNormalized(t *testing.T) {
	e := NewHashEmbedder(128)
	v := e.Vector("some text here")
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if norm < 0.99 || norm > 1.01 {
		t.Fatalf("vector norm = %v, want ~1", norm)
	}
}

func TestHashEmbedderEmbed(t *testing.T) {
	e := NewHashEmbedder(32)
	resp, err := e.Embed(context.Background(), "", llm.EmbeddingRequest{
		Model: "stub", Input: []string{"a", "b"},
	})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("data len = %d, want 2", len(resp.Data))
	}
	if len(resp.Data[0].Vector) != 32 {
		t.Fatalf("dim = %d, want 32", len(resp.Data[0].Vector))
	}
	if resp.Usage.TotalTokens == 0 {
		t.Fatal("usage not accounted")
	}
}

func TestHashEmbedderValidates(t *testing.T) {
	_, err := NewHashEmbedder(16).Embed(context.Background(), "", llm.EmbeddingRequest{Input: []string{"x"}})
	if err != llm.ErrEmptyModel {
		t.Fatalf("want ErrEmptyModel, got %v", err)
	}
}

func TestHashEmbedderDefaultDim(t *testing.T) {
	if got := NewHashEmbedder(0).Dim; got != 256 {
		t.Fatalf("default Dim = %d, want 256", got)
	}
}

func TestHashEmbedderEmptyText(t *testing.T) {
	v := NewHashEmbedder(8).Vector("")
	for i, x := range v {
		if x != 0 {
			t.Fatalf("v[%d] = %v, want 0", i, x)
		}
	}
}

func TestPassthroughReranker(t *testing.T) {
	in := []llm.ScoredSegment{{DocumentID: "a", Score: 0.1}, {DocumentID: "b", Score: 0.9}}
	got, err := PassthroughReranker{}.Rerank(context.Background(), "q", in)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if got[0].DocumentID != "a" {
		t.Fatalf("passthrough changed order: %+v", got)
	}
}

func TestKeywordRerankerPromotesLexicalMatch(t *testing.T) {
	in := []llm.ScoredSegment{
		{DocumentID: "irrelevant", Score: 0.60, Segment: llm.Segment{Content: "完全无关的内容"}},
		{DocumentID: "relevant", Score: 0.55, Segment: llm.Segment{Content: "知识库检索的实现方式"}},
	}
	got, err := KeywordReranker{Weight: 0.6}.Rerank(context.Background(), "知识库检索", in)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if got[0].DocumentID != "relevant" {
		t.Fatalf("lexical match not promoted: %+v", got)
	}
}

func TestKeywordRerankerDefaultWeight(t *testing.T) {
	in := []llm.ScoredSegment{{DocumentID: "a", Score: 1, Segment: llm.Segment{Content: "x"}}}
	got, err := KeywordReranker{Weight: 5}.Rerank(context.Background(), "q", in)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if got[0].Score < 0 || got[0].Score > 1 {
		t.Fatalf("score out of range: %v", got[0].Score)
	}
}

func TestKeywordRerankerDoesNotMutateInput(t *testing.T) {
	in := []llm.ScoredSegment{{DocumentID: "a", Score: 0.5, Segment: llm.Segment{Content: "hello"}}}
	_, _ = KeywordReranker{}.Rerank(context.Background(), "hello", in)
	if in[0].Score != 0.5 {
		t.Fatalf("input mutated: %v", in[0].Score)
	}
}

var (
	_ ports.VectorStore = (*MemoryVectorStore)(nil)
	_ ports.Embedder    = (*HashEmbedder)(nil)
	_ ports.Reranker    = PassthroughReranker{}
	_ ports.Reranker    = KeywordReranker{}
	_ ports.ChatCompleter = (*OpenAIClient)(nil)
	_ ports.Embedder      = (*OpenAIClient)(nil)
)