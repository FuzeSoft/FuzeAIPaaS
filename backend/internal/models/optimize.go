package models

import "time"

type CompressionTask struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	TenantID       string    `gorm:"index" json:"tenantId"`
	Name           string    `json:"name"`
	Type           string    `gorm:"index" json:"type"`       
	Backend        string    `json:"backend"`                 
	ConfigJSON     string    `gorm:"type:text" json:"config"` 
	ModelVersionID string    `gorm:"index" json:"modelVersionId"`
	Status         string    `gorm:"index" json:"status"` 
	JobID          string    `json:"jobId,omitempty"`     

	CompressedSizeBytes int64   `json:"compressedSizeBytes"`
	LatencyMs           float64 `json:"latencyMs"`
	Accuracy            float64 `json:"accuracy"`
	ArtifactURI         string  `json:"artifactUri,omitempty"`
	CompressionRatio    float64 `json:"compressionRatio"`
	Speedup             float64 `json:"speedup"`
	OrigAccuracy        float64 `json:"origAccuracy"`  
	GateThreshold       float64 `json:"gateThreshold"`  
	GatePass            bool    `json:"gatePass"`       
	FailReason          string  `json:"failReason,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (CompressionTask) TableName() string { return "compression_tasks" }