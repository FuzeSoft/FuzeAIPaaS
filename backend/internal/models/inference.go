package models

import "time"

type InferenceStatus string

const (
	InferenceStatusPending InferenceStatus = "pending"
	InferenceStatusReady   InferenceStatus = "ready"
	InferenceStatusFailed  InferenceStatus = "failed"
	InferenceStatusUnknown InferenceStatus = "unknown"
)

type InferenceFramework string

const (
	FrameworkTensorflow InferenceFramework = "tensorflow"
	FrameworkPyTorch    InferenceFramework = "pytorch"
	FrameworkTriton     InferenceFramework = "triton"
	FrameworkSKLearn    InferenceFramework = "sklearn"
	FrameworkXGBoost    InferenceFramework = "xgboost"
	FrameworkONNX       InferenceFramework = "onnx"
	FrameworkCustom     InferenceFramework = "custom"
)

type InferenceService struct {
	ID         string             `gorm:"primaryKey" json:"id"`
	ClusterID  string             `gorm:"index" json:"cluster_id"` 
	Name       string             `json:"name"`
	Framework  InferenceFramework `json:"framework"`   
	StorageURI string             `json:"storage_uri"` 
	Image      string             `json:"image"`       
	RuntimeVer string             `json:"runtime_version,omitempty"`

	MinReplicas int `json:"min_replicas"` 
	MaxReplicas int `json:"max_replicas"` 

	CPU       string `json:"cpu"`        
	Memory    string `json:"memory"`     
	GPUs      int    `json:"gpus"`       
	GPUMemory int    `json:"gpu_memory"` 
	GPUCores  int    `json:"gpu_cores"`  

	Chip string `json:"chip,omitempty"`

	Runtime        string `json:"runtime,omitempty"`         
	TargetReplicas int    `json:"target_replicas,omitempty"` 
	CanaryWeight   int    `json:"canary_weight,omitempty"`   

	Status        InferenceStatus `json:"status"`
	URL           string          `json:"url,omitempty"`            
	KServeName    string          `json:"kserve_name,omitempty"`    
	ReadyReplicas int             `json:"ready_replicas,omitempty"` 

	FailureReason string `json:"failure_reason,omitempty"` 

	TenantID string `json:"tenant_id,omitempty"` 

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func KServeStateToStatus(ready bool, found bool, failed bool) InferenceStatus {
	if failed {
		return InferenceStatusFailed
	}
	if !found {
		return InferenceStatusPending
	}
	if ready {
		return InferenceStatusReady
	}
	return InferenceStatusPending
}