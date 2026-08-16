package ports

import (
	"context"
	"io"
	"time"

	"fuze-ai-paas/backend/internal/models"
)

type ArtifactInfo struct {
	Key          string    `json:"key"`
	URI          string    `json:"uri"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

type MetricSample struct {
	
	Timestamp int64 `json:"timestamp"`
	
	Value float64 `json:"value"`
}

type MetricSeries struct {
	
	Labels map[string]string `json:"labels"`
	
	Samples []MetricSample `json:"samples"`
}

type MetricQuery struct {
	
	Query string `json:"query"`
	
	JobID string `json:"job_id,omitempty"`
	
	Labels map[string]string `json:"labels,omitempty"`
	
	Start int64 `json:"start,omitempty"`
	End   int64 `json:"end,omitempty"`
	
	Step int `json:"step,omitempty"`
}

type MetricsQuery interface {
	
	QueryRange(q MetricQuery) ([]MetricSeries, error)
	
	QueryLatest(q MetricQuery) (*MetricSample, error)
	
	Alerts() ([]ActiveAlert, error)
}

type ActiveAlert struct {
	
	Fingerprint string `json:"fingerprint"`
	
	RuleName string `json:"rule_name"`
	
	State string `json:"state"`
	
	Severity string `json:"severity"`
	
	Labels map[string]string `json:"labels"`
	
	Annotations map[string]string `json:"annotations"`
	
	ActiveAt int64 `json:"active_at"`
	
	Value string `json:"value"`
}

type AlertRepository interface {
	
	CreateRule(r *models.AlertRule) error
	
	UpdateRule(r *models.AlertRule) error
	
	GetRule(tenantID, id string) (*models.AlertRule, error)
	
	ListRules(tenantID string) ([]models.AlertRule, error)
	
	DeleteRule(tenantID, id string) error

	CreateSilence(s *models.AlertSilence) error
	
	ListSilences(tenantID string) ([]models.AlertSilence, error)
	
	DeleteSilence(tenantID, id string) error
}

type ExperimentRepository interface {
	
	GetExperiments(tenantID string) ([]models.Experiment, error)
	GetExperiment(id string) (*models.Experiment, error)
	CreateExperiment(e *models.Experiment) error
	UpdateExperiment(e *models.Experiment) error
	DeleteExperiment(id string) error

	GetRuns(experimentID string) ([]models.Run, error)
	GetRun(id string) (*models.Run, error)
	
	GetRunByJobID(jobID string) (*models.Run, error)
	CreateRun(r *models.Run) error
	UpdateRun(r *models.Run) error
	DeleteRun(id string) error
}

type ArtifactStore interface {
	Put(ctx context.Context, key string, r io.Reader) (uri string, err error)
	Get(ctx context.Context, uri string) (io.ReadCloser, error)
	List(ctx context.Context, prefix string) ([]ArtifactInfo, error)
	Delete(ctx context.Context, uri string) error
	
	Presign(ctx context.Context, uri string, ttl time.Duration) (string, error)
}