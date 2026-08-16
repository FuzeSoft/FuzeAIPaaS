package models

import "time"

type Evaluation struct {
	ID           string  `gorm:"primaryKey" json:"id"`
	Name         string  `json:"name"`
	Task         string  `json:"task"`
	Dataset      string  `json:"dataset"`
	ExperimentID string  `gorm:"index" json:"experiment_id,omitempty"`
	RunID        string  `json:"run_id,omitempty"`
	ModelID      string  `gorm:"index" json:"model_id,omitempty"`
	
	Criteria string `json:"criteria"`
	
	Metrics    string `json:"metrics"`
	Score      float64 `json:"score"`
	Passed     bool    `json:"passed"`
	Status     string  `json:"status"` 
	FailReason string  `json:"fail_reason,omitempty"`
	TenantID   string  `json:"tenant_id"`
	CreatedBy  string  `json:"created_by"`

	JudgeMode string `json:"judge_mode,omitempty"`
	
	Dimensions string `json:"dimensions,omitempty"`
	
	Report string `json:"report,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

func (Evaluation) TableName() string { return "evaluations" }