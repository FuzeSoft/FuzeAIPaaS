package llmgw

import (
	"testing"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/ports"
)

func TestRegistryMemoryBackend(t *testing.T) {
	r := NewVectorStoreRegistry(VectorStoreConfig{Backend: BackendMemory, EmbedDim: 4}, nil)
	if r.ActiveBackend() != BackendMemory {
		t.Fatalf("backend = %s, want memory", r.ActiveBackend())
	}
	if _, ok := r.Store().(*MemoryVectorStore); !ok {
		t.Fatalf("store type = %T, want *MemoryVectorStore", r.Store())
	}
}

func TestRegistryPGVectorNoDB(t *testing.T) {
	r := NewVectorStoreRegistry(VectorStoreConfig{Backend: BackendPGVector, EmbedDim: 4}, nil)
	if r.ActiveBackend() != BackendMemory {
		t.Fatalf("degraded backend = %s, want memory", r.ActiveBackend())
	}
}

func TestRegistryAutoDegradesOnSQLite(t *testing.T) {
	
	r := NewVectorStoreRegistry(VectorStoreConfig{Backend: BackendAuto, EmbedDim: 4}, nil)
	if r.ActiveBackend() != BackendMemory {
		t.Fatalf("auto backend = %s, want memory (degraded)", r.ActiveBackend())
	}
}

func TestRegistryStoreUsable(t *testing.T) {
	r := NewVectorStoreRegistry(VectorStoreConfig{Backend: BackendMemory, EmbedDim: 2}, nil)
	s := r.Store()
	ctx := t.Context()
	items := []ports.VectorItem{
		{ID: "1", DocumentID: "d1", Title: "T", Segment: llm.Segment{Content: "alpha"}, Vector: []float32{1, 0}},
		{ID: "2", DocumentID: "d2", Title: "T", Segment: llm.Segment{Content: "beta"}, Vector: []float32{0, 1}},
	}
	if err := s.Upsert(ctx, "kb", items); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	hits, err := s.Search(ctx, "kb", []float32{0.9, 0.1}, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 || hits[0].Item.ID != "1" {
		t.Fatalf("top hit unexpected: %+v", hits)
	}
	
	if err := s.Delete(ctx, "kb", "d1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	hits, _ = s.Search(ctx, "kb", []float32{0.9, 0.1}, 5)
	for _, h := range hits {
		if h.Item.DocumentID == "d1" {
			t.Fatalf("deleted document d1 still present in results")
		}
	}
}