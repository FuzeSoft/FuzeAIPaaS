package optimize

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("compression task not found")

type CompressionRepository interface {
	Create(ctx context.Context, t *CompressionTask) error
	Get(ctx context.Context, id string) (*CompressionTask, error)
	ListByTenant(ctx context.Context, tenantID string) ([]*CompressionTask, error)
	Update(ctx context.Context, t *CompressionTask) error
	Delete(ctx context.Context, id string) error
}

type CompressionResult struct {
	JobID              string  `json:"job_id"`
	CompressedSizeBytes int64 `json:"compressed_size_bytes"`
	LatencyMs          float64 `json:"latency_ms"`
	Accuracy           float64 `json:"accuracy"`
	ArtifactURI        string  `json:"artifact_uri"`
	CompressionRatio   float64 `json:"compression_ratio"`
	Speedup            float64 `json:"speedup"`
}

type CompressionExecutor interface {
	
	Submit(*CompressionTask) (jobID string, err error)
	
	Cancel(jobID string) error
	
	GetResult(jobID string) (*CompressionResult, error)
}