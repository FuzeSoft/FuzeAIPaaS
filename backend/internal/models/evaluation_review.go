package models

import "time"

type EvaluationReview struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	EvaluationID string    `gorm:"index" json:"evaluation_id"`
	TenantID     string    `json:"tenant_id"`
	
	JudgeType string `json:"judge_type"`
	
	JudgeID string `json:"judge_id"`
	
	Scores string `json:"scores"`
	
	Comment string `json:"comment,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

func (EvaluationReview) TableName() string { return "evaluation_reviews" }