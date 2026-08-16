package models

import "time"

type PipelineStatus string

const (
	PipelineStatusDraft     PipelineStatus = "draft"     
	PipelineStatusPending   PipelineStatus = "pending"   
	PipelineStatusRunning   PipelineStatus = "running"   
	PipelineStatusCompleted PipelineStatus = "completed" 
	PipelineStatusFailed    PipelineStatus = "failed"    
	PipelineStatusCancelled PipelineStatus = "cancelled"
)

func (s PipelineStatus) IsTerminal() bool {
	return s == PipelineStatusCompleted || s == PipelineStatusFailed || s == PipelineStatusCancelled
}

type StepKind string

const (
	StepKindClean      StepKind = "clean"
	StepKindAugment    StepKind = "augment"
	StepKindETL        StepKind = "etl"
	StepKindAnnotation StepKind = "annotation"
)

func (k StepKind) ToJobType() JobType {
	switch k {
	case StepKindClean:
		return JobTypeDataClean
	case StepKindAugment:
		return JobTypeDataAugment
	case StepKindETL:
		return JobTypeDataETL
	case StepKindAnnotation:
		return JobTypeDataAnnotation
	default:
		return JobTypeDataETL
	}
}

type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusSucceeded StepStatus = "succeeded"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
)

func (s StepStatus) IsTerminal() bool {
	return s == StepStatusSucceeded || s == StepStatusFailed || s == StepStatusSkipped
}

type DataPipeline struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	TenantID    string         `gorm:"index" json:"tenant_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	DatasetName string         `json:"dataset_name"` 
	MountPath   string         `json:"mount_path"`   
	Status      PipelineStatus `json:"status"`
	Priority    int            `json:"priority"`
	QueueName   string         `json:"queue_name"` 
	ClusterID   string         `json:"cluster_id"` 
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type PipelineStep struct {
	ID        string     `gorm:"primaryKey" json:"id"`
	PipelineID string    `gorm:"index" json:"pipeline_id"`
	Name      string     `json:"name"`
	Kind      StepKind   `json:"kind"`
	
	Operator string `json:"operator"`
	
	DependsOn string `json:"depends_on"`
	
	InputPath  string `json:"input_path"`
	OutputPath string `json:"output_path"`
	
	Params string `json:"params"`
	
	Image   string `json:"image"`
	Command string `json:"command"`
	
	GPUs   int `json:"gpus"`
	Memory int `json:"memory"`
	
	Status         StepStatus `json:"status"`
	JobID          string     `json:"job_id"` 
	FailureReason  string     `json:"failure_reason,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type PipelineStepRun struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	StepID        string    `gorm:"index" json:"step_id"`
	PipelineID    string    `gorm:"index" json:"pipeline_id"`
	VolcanoJobName string   `json:"volcano_job_name,omitempty"`
	Status        StepStatus `json:"status"`
	Attempts      int       `json:"attempts"`
	LogRef        string    `json:"log_ref,omitempty"` 
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type AnnotationStatus string

const (
	AnnotationStatusOpen      AnnotationStatus = "open"      
	AnnotationStatusInProgress AnnotationStatus = "in_progress"
	AnnotationStatusReviewing AnnotationStatus = "reviewing"
	AnnotationStatusCompleted AnnotationStatus = "completed"
	AnnotationStatusExported  AnnotationStatus = "exported"
)

type AnnotationTask struct {
	ID           string           `gorm:"primaryKey" json:"id"`
	TenantID     string           `gorm:"index" json:"tenant_id"`
	Name         string           `json:"name"`
	DatasetName  string           `json:"dataset_name"` 
	DataGlob     string           `json:"data_glob"`    
	TaskType     string           `json:"task_type"`    
	Categories   string           `json:"categories"`   
	Assignee     string           `json:"assignee"`     
	Status       AnnotationStatus `json:"status"`
	Progress     int              `json:"progress"`     
	OutputFormat string           `json:"output_format"` 
	ExportedURI  string           `json:"exported_uri,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}