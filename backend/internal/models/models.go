package models

import (
	"encoding/json"
	"time"
)

type ResourceType string

const (
	ResourceTypeGPU ResourceType = "GPU"
	ResourceTypeNPU ResourceType = "NPU"
	ResourceTypeCPU ResourceType = "CPU"
)

type ResourceStatus string

const (
	ResourceStatusAvailable   ResourceStatus = "available"
	ResourceStatusAllocated   ResourceStatus = "allocated"
	ResourceStatusMaintenance ResourceStatus = "maintenance"
	ResourceStatusError       ResourceStatus = "error"
)

type Resource struct {
	ID              string         `gorm:"primaryKey" json:"id"`
	ClusterID       string         `gorm:"index" json:"cluster_id"` 
	Name            string         `json:"name"`
	Type            ResourceType   `json:"type"`
	Vendor          string         `json:"vendor"`
	Model           string         `json:"model"`
	TotalGPUs       int            `json:"total_gpus"`   
	UsedGPUs        int            `json:"used_gpus"`    
	TotalMemory     int            `json:"total_memory"` 
	AvailableMemory int            `json:"available_memory"`
	Status          ResourceStatus `json:"status"`
	NodeName        string         `json:"node_name"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type JobType string

const (
	JobTypeTraining  JobType = "training"
	JobTypeInference JobType = "inference"
	JobTypeBatch     JobType = "batch"
	
	JobTypeDataClean      JobType = "data-clean"
	JobTypeDataAugment    JobType = "data-augment"
	JobTypeDataETL        JobType = "data-etl"
	JobTypeDataAnnotation JobType = "data-annotation"
)

type JobStatus string

const (
	JobStatusPending JobStatus = "pending"
	JobStatusRunning JobStatus = "running"
	JobStatusPaused  JobStatus = "paused"
	
	JobStatusRetrying  JobStatus = "retrying"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

type Job struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	ClusterID string    `gorm:"index" json:"cluster_id"` 
	Name      string    `json:"name"`
	Type      JobType   `json:"type"`
	Status    JobStatus `json:"status"`
	Priority  int       `json:"priority"`
	Image     string    `json:"image"`
	Command   string    `json:"command"`
	GPUs      int       `json:"gpus"`
	Memory    int       `json:"memory"`

	GPUMemory int `json:"gpu_memory"` 
	GPUCores  int `json:"gpu_cores"`  

	Distributed  bool   `json:"distributed"`   
	Framework    string `json:"framework"`     
	Replicas     int    `json:"replicas"`      
	MinAvailable int    `json:"min_available"` 

	DatasetName string `json:"dataset_name,omitempty"` 
	MountPath   string `json:"mount_path,omitempty"`   

	TenantID string `json:"tenant_id,omitempty"` 

	VolcanoJobName string    `json:"volcano_job_name,omitempty"` 
	QueueName      string    `json:"queue_name,omitempty"`       
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	FailureReason  string `json:"failure_reason,omitempty"` 
	SubmitAttempts int    `json:"submit_attempts"`          

	LastSubmitAt time.Time `json:"last_submit_at,omitempty"`

	MaxRuntime int `json:"max_runtime"`
	
	StartedAt *time.Time `json:"started_at,omitempty"`

	CheckpointEnabled    bool `json:"checkpoint_enabled"`
	CheckpointInterval   int  `json:"checkpoint_interval"`    
	CheckpointMaxRetries int  `json:"checkpoint_max_retries"` 
	
	LatestCheckpointURI      string     `json:"latest_checkpoint_uri,omitempty"`
	LatestCheckpointStep     int        `json:"latest_checkpoint_step"`
	LatestCheckpointAt       *time.Time `json:"latest_checkpoint_at,omitempty"`
	LatestCheckpointHash     string     `json:"latest_checkpoint_hash,omitempty"`
	LatestCheckpointSizeBytes int64     `json:"latest_checkpoint_size_bytes,omitempty"`
	
	ResumeFrom string `json:"resume_from,omitempty"`
	
	RetryAttempts int `json:"retry_attempts"`

	RegisterModelEnabled bool   `json:"register_model_enabled"`
	RegisterModelID      string `json:"register_model_id,omitempty"`
	RegisterVersionTag   string `json:"register_version_tag,omitempty"`
	
	RegisteredVersionID string `json:"registered_version_id,omitempty"`

	RegisterAdapterEnabled bool   `json:"register_adapter_enabled"`
	
	AdapterBaseModel string `json:"adapter_base_model,omitempty"`
	
	AdapterMethod string `json:"adapter_method,omitempty"`
	
	AdapterRank int `json:"adapter_rank,omitempty"`
	
	RegisteredAdapterID string `json:"registered_adapter_id,omitempty"`

	CodeCommit string `json:"code_commit,omitempty"`
	
	CodeRepo string `json:"code_repo,omitempty"`
	TemplateID string `json:"template_id,omitempty"`

	DatasetID      string `json:"dataset_id,omitempty"`      
	DatasetVersion string `json:"dataset_version,omitempty"` 

	DataSpecJSON string `gorm:"type:text" json:"data_spec_json,omitempty"`
}

func (j *Job) EffectiveReplicas() int {
	if !j.Distributed || j.Replicas <= 0 {
		return 1
	}
	return 1 + j.Replicas
}

func (j *Job) TotalGPUs() int {
	if j.GPUs <= 0 {
		return 0
	}
	return j.GPUs * j.EffectiveReplicas()
}

func (j *Job) TotalMemory() int {
	if j.Memory <= 0 {
		return 0
	}
	return j.Memory * j.EffectiveReplicas()
}

func (j *Job) IsTerminal() bool {
	return j.Status.IsTerminal()
}

func (s JobStatus) IsTerminal() bool {
	switch s {
	case JobStatusCompleted, JobStatusFailed, JobStatusCancelled:
		return true
	default:
		return false
	}
}

const (
	ClusterStatusRegistered = "registered" 
	ClusterStatusHealthy    = "healthy"    
	ClusterStatusUnhealthy  = "unhealthy"  
)

type Cluster struct {
	ID          string `gorm:"primaryKey" json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Region      string `json:"region"`
	Provider    string `json:"provider"`  
	Endpoint    string `json:"endpoint"`  
	Version     string `json:"version"`   
	Namespace   string `json:"namespace"` 

	KubeConfig string `gorm:"type:text" json:"kube_config,omitempty"`

	KubeConfigEnc string `gorm:"column:kubeconfig_enc;type:text" json:"-"`

	Enabled bool `json:"enabled"`

	NodeCount int `json:"node_count"`
	GPUCount  int `json:"gpu_count"` 
	TotalGPUs int `json:"total_gpus"`
	UsedGPUs  int `json:"used_gpus"`

	Status     string    `json:"status"`
	LastSyncAt time.Time `json:"last_sync_at"`
	SyncError  string    `json:"sync_error,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c Cluster) MarshalJSON() ([]byte, error) {
	type Alias Cluster 
	return json.Marshal(struct {
		Alias
		
		KubeConfig string `json:"kube_config,omitempty"`
	}{
		Alias:      Alias(c),
		KubeConfig: "",
	})
}

type Queue struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Priority    int       `json:"priority"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Metrics struct {
	TotalGPUs         int     `json:"total_gpus"`
	UsedGPUs          int     `json:"used_gpus"`
	AvailableGPUs     int     `json:"available_gpus"`
	GPUUtilization    float64 `json:"gpu_utilization"`
	TotalJobs         int     `json:"total_jobs"`
	RunningJobs       int     `json:"running_jobs"`
	PendingJobs       int     `json:"pending_jobs"`
	CompletedJobs     int     `json:"completed_jobs"`
	TotalMemory       int     `json:"total_memory"`
	UsedMemory        int     `json:"used_memory"`
	MemoryUtilization float64 `json:"memory_utilization"`
}