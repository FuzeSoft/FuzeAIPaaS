package ports

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrAdapterNotFound = errors.New("adapter not found")

var ErrAdapterInvalid = errors.New("invalid adapter")

var ErrAdapterConflict = errors.New("adapter name already exists")

const (
	MethodLoRA  = "lora"
	MethodQLoRA = "qlora"
)

const maxAdapterRank = 256

type FineTuneAdapter struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	
	BaseModel string `json:"base_model"`
	
	Path string `json:"path"`
	
	Rank int `json:"rank"`
	
	Method string `json:"method"`
	
	SourceJobID string `json:"source_job_id,omitempty"`
	TenantID    string `json:"tenant_id"`
	CreatedBy   string `json:"created_by"`
	
	CreatedAt int64 `json:"created_at"`
}

func (a *FineTuneAdapter) Normalize() {
	a.Name = strings.TrimSpace(a.Name)
	a.BaseModel = strings.TrimSpace(a.BaseModel)
	a.Path = strings.TrimSpace(a.Path)
	a.SourceJobID = strings.TrimSpace(a.SourceJobID)
	a.Method = strings.ToLower(strings.TrimSpace(a.Method))
	if a.Method == "" {
		
		a.Method = MethodLoRA
	}
	if a.Rank == 0 {
		
		a.Rank = 8
	}
}

func (a FineTuneAdapter) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("%w: name required", ErrAdapterInvalid)
	}
	if a.BaseModel == "" {
		return fmt.Errorf("%w: base_model required", ErrAdapterInvalid)
	}
	if a.Path == "" {
		return fmt.Errorf("%w: path required", ErrAdapterInvalid)
	}
	if a.Rank <= 0 || a.Rank > maxAdapterRank {
		return fmt.Errorf("%w: rank must be within [1,%d], got %d", ErrAdapterInvalid, maxAdapterRank, a.Rank)
	}
	if a.Method != MethodLoRA && a.Method != MethodQLoRA {
		return fmt.Errorf("%w: unknown method %q (want %s or %s)", ErrAdapterInvalid, a.Method, MethodLoRA, MethodQLoRA)
	}
	return nil
}

type FineTuneFilter struct {
	
	TenantID string
	
	BaseModel string
	
	Limit int
}

type FineTuneRepository interface {
	
	Create(ctx context.Context, a *FineTuneAdapter) error
	
	Get(ctx context.Context, tenantID, id string) (*FineTuneAdapter, error)
	
	List(ctx context.Context, f FineTuneFilter) ([]*FineTuneAdapter, error)
	
	Delete(ctx context.Context, tenantID, id string) error
}