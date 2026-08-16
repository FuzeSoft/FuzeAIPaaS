package llmgw

import (
	"log"
	"os"

	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
)

type VectorBackend string

const (
	
	BackendMemory VectorBackend = "memory"
	
	BackendPGVector VectorBackend = "pgvector"
	
	BackendAuto VectorBackend = "auto"
)

type VectorStoreConfig struct {
	
	Backend VectorBackend
	
	EmbedDim int
}

func (c VectorStoreConfig) ResolveBackend() VectorBackend {
	if c.Backend != "" {
		return c.Backend
	}
	if v := os.Getenv("VECTOR_BACKEND"); v != "" {
		return VectorBackend(v)
	}
	return BackendAuto
}

type VectorStoreRegistry struct {
	active VectorBackend
	store  ports.VectorStore
}

func NewVectorStoreRegistry(cfg VectorStoreConfig, db *gorm.DB) *VectorStoreRegistry {
	backend := cfg.ResolveBackend()
	want := backend
	if want == BackendAuto {
		want = BackendPGVector 
	}

	var (
		store  ports.VectorStore
		active VectorBackend
	)
	switch want {
	case BackendPGVector:
		if db == nil {
			log.Printf("[vector] pgvector requested but no DB; falling back to memory")
			store, active = NewMemoryVectorStore(), BackendMemory
			break
		}
		pg, err := NewPGVectorStore(db, cfg.EmbedDim)
		if err != nil {
			log.Printf("[vector] pgvector unavailable (%v); falling back to memory", err)
			store, active = NewMemoryVectorStore(), BackendMemory
			break
		}
		store, active = pg, BackendPGVector
	default:
		store, active = NewMemoryVectorStore(), BackendMemory
	}

	if backend == BackendAuto {
		log.Printf("[vector] auto-selected backend=%s", active)
	} else {
		log.Printf("[vector] vector backend=%s", active)
	}
	return &VectorStoreRegistry{active: active, store: store}
}

func (r *VectorStoreRegistry) Store() ports.VectorStore { return r.store }

func (r *VectorStoreRegistry) ActiveBackend() VectorBackend { return r.active }