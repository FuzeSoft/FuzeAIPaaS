package models

import "time"

type Model struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	TenantID    string    `gorm:"index" json:"tenantId"`
	Name        string    `gorm:"index" json:"name"`
	Description string    `json:"description"`
	Framework   string    `json:"framework"`
	Owner       string    `json:"owner"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ModelVersion struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	TenantID   string    `gorm:"index" json:"tenantId"`
	ModelID    string    `gorm:"uniqueIndex:idx_model_version;index" json:"modelId"`
	Version    string    `gorm:"uniqueIndex:idx_model_version" json:"version"`
	StorageURI string    `json:"storageUri"`
	Image      string    `json:"image"`
	SizeBytes  int64     `json:"sizeBytes"`
	Hash       string    `json:"hash"`
	CreatedAt  time.Time `json:"createdAt"`

	SourceJobID string `gorm:"index" json:"sourceJobId,omitempty"`
	
	SourceRunID string `gorm:"index" json:"sourceRunId,omitempty"`

	CodeRepo   string `json:"codeRepo,omitempty"`
	CodeCommit string `json:"codeCommit,omitempty"`
	TemplateID string `json:"templateId,omitempty"`
	
	DatasetID      string `json:"datasetId,omitempty"`
	DatasetName    string `json:"datasetName,omitempty"`
	DatasetVersion string `json:"datasetVersion,omitempty"`
	
	Hyperparameters string `json:"hyperparameters,omitempty"`

	Files string `gorm:"type:text" json:"files,omitempty"`
}

func (Model) TableName() string        { return "models" }
func (ModelVersion) TableName() string { return "model_versions" }