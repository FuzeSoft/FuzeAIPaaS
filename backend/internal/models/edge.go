package models

import "time"

type EdgeNodeRow struct {
	ID            string `gorm:"primaryKey" json:"id"`
	TenantID      string `gorm:"index" json:"tenantId"`
	Name          string `gorm:"index" json:"name"`
	Mode          string `json:"mode"`
	Status        string `json:"status"`
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region"`
	Labels        string `gorm:"type:text" json:"labels"` 
	CACertPEM     string `gorm:"type:text" json:"caCertPem,omitempty"`
	ClientCertPEM string `gorm:"type:text" json:"clientCertPem,omitempty"`
	ClientKeyPEM  string `gorm:"type:text" json:"clientKeyPem,omitempty"`
	HeartbeatAt   time.Time `json:"heartbeatAt"`
	LastSeenAt    time.Time `json:"lastSeenAt"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (EdgeNodeRow) TableName() string { return "edge_nodes" }

type EdgeDeploymentRow struct {
	ID               string `gorm:"primaryKey" json:"id"`
	TenantID         string `gorm:"index" json:"tenantId"`
	NodeID           string `gorm:"index" json:"nodeId"`
	ModelID          string `json:"modelId"`
	Version          string `json:"version"`
	DesiredSpec      string `gorm:"type:text" json:"desiredSpec"` 
	CurrentVersion   string `json:"currentVersion"`
	ActiveVersion    string `json:"activeVersion"`
	CanaryVersion    string `json:"canaryVersion"`
	CanaryWeight     int    `json:"canaryWeight"`
	Status           string `json:"status"`
	AutoRollback     bool   `json:"autoRollback"`
	DriftGuardEnabled bool  `json:"driftGuardEnabled"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func (EdgeDeploymentRow) TableName() string { return "edge_deployments" }

type DriftReportRow struct {
	ID               string `gorm:"primaryKey" json:"id"`
	TenantID         string `gorm:"index" json:"tenantId"`
	DeploymentID     string `gorm:"index" json:"deploymentId"`
	NodeID           string `json:"nodeId"`
	EvaluatedAt      time.Time `json:"evaluatedAt"`
	DataDrift        string `gorm:"type:text" json:"dataDrift"`        
	PredictionDrift  string `gorm:"type:text" json:"predictionDrift"`  
	PerformanceDrift string `gorm:"type:text" json:"performanceDrift"` 
	ConceptDrift     string `gorm:"type:text" json:"conceptDrift"`     
	OverallSeverity  string `json:"overallSeverity"`
	TriggeredRollback bool  `json:"triggeredRollback"`
	Recommendation   string `gorm:"type:text" json:"recommendation"`
}

func (DriftReportRow) TableName() string { return "drift_reports" }

type DriftBaselineRow struct {
	DeploymentID     string `gorm:"primaryKey" json:"deploymentId"`
	ReferenceWindow  string `json:"referenceWindow"`
	NumericFeatures  string `gorm:"type:text" json:"numericFeatures"`   
	CategoricalFeatures string `gorm:"type:text" json:"categoricalFeatures"` 
	PredictionDist   string `gorm:"type:text" json:"predictionDist"`    
	Performance      string `gorm:"type:text" json:"performance"`       
	ConceptLabels    string `gorm:"type:text" json:"conceptLabels"`     
}

func (DriftBaselineRow) TableName() string { return "drift_baselines" }

type EdgeLabelFeedbackRow struct {
	ID           string `gorm:"primaryKey" json:"id"`
	TenantID     string `gorm:"index" json:"tenantId"`
	DeploymentID string `gorm:"index" json:"deploymentId"`
	Label        string `gorm:"index" json:"label"`
	RequestID    string `json:"requestId,omitempty"`
	FeedbackAt   time.Time `gorm:"index" json:"feedbackAt"`
}

func (EdgeLabelFeedbackRow) TableName() string { return "edge_label_feedback" }