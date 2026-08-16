package models

import "time"

type LineageNodeType string

const (
	LineageCode         LineageNodeType = "code"
	LineageData         LineageNodeType = "data"
	LineageHyperparam   LineageNodeType = "hyperparam"
	LineageJob          LineageNodeType = "job"
	LineageRun          LineageNodeType = "run"
	LineageModelVersion LineageNodeType = "model-version"
)

const (
	
	RelUses string = "uses"
	
	RelProduced string = "produced"
	
	RelDerived string = "derived"
	
	RelServedBy string = "served_by"
)

type LineageEdge struct {
	ID        string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID  string          `gorm:"index;size:64" json:"tenant_id"`
	FromType  LineageNodeType `gorm:"index;size:32" json:"from_type"`
	FromID    string          `gorm:"index;size:128" json:"from_id"`
	ToType    LineageNodeType `gorm:"index;size:32" json:"to_type"`
	ToID      string          `gorm:"index;size:128" json:"to_id"`
	Relation  string          `gorm:"index;size:32" json:"relation"`
	CreatedAt time.Time       `json:"created_at"`
}

func (LineageEdge) TableName() string { return "model_lineage_edges" }