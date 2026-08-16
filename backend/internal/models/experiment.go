package models

import "time"

const (
	ExperimentStatusActive  = "active"
	ExperimentStatusArchived = "archived"
)

const (
	RunStatusPending   = "pending"
	RunStatusRunning   = "running"
	RunStatusCompleted = "completed"
	RunStatusFailed    = "failed"
	RunStatusCancelled = "cancelled"
)

type Experiment struct {
	ID          string `gorm:"primaryKey" json:"id"`
	TenantID    string `gorm:"index" json:"tenant_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	
	Objective string `json:"objective"` 
	
	MetricName string `json:"metric_name"`
	Status     string `json:"status"` 
	
	Tags string `json:"tags,omitempty"`
	
	BestRunID string `json:"best_run_id,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Run struct {
	ID           string `gorm:"primaryKey" json:"id"`
	ExperimentID string `gorm:"index" json:"experiment_id"`
	TenantID     string `gorm:"index" json:"tenant_id"`
	Name         string `json:"name"`
	
	Hyperparameters string `json:"hyperparameters,omitempty"`
	
	MetricValue *float64 `json:"metric_value,omitempty"`
	
	Metrics string `json:"metrics,omitempty"`
	
	ArtifactURI string `json:"artifact_uri,omitempty"`
	
	SourceJobID string `gorm:"index" json:"source_job_id,omitempty"`
	
	CodeRepo       string `json:"code_repo,omitempty"`
	CodeCommit     string `json:"code_commit,omitempty"`
	Image          string `json:"image,omitempty"`
	TemplateID     string `json:"template_id,omitempty"`
	DatasetID      string `json:"dataset_id,omitempty"`
	DatasetName    string `json:"dataset_name,omitempty"`
	DatasetVersion string `json:"dataset_version,omitempty"`
	Status         string `json:"status"` 
	
	IsBest    bool      `json:"is_best"`
	
	ParentRunID string `json:"parent_run_id,omitempty" gorm:"index"`
	
	ReproductionState string `json:"reproduction_state,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

func (Experiment) TableName() string { return "experiments" }
func (Run) TableName() string        { return "runs" }