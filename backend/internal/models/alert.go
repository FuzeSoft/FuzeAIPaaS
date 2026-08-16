
package models

import "time"

type AlertSeverity string

const (
	
	SeverityInfo AlertSeverity = "info"
	
	SeverityWarning AlertSeverity = "warning"
	
	SeverityCritical AlertSeverity = "critical"
)

type AlertRule struct {
	ID          string        `gorm:"primaryKey;type:text" json:"id"`
	TenantID    string        `gorm:"index;type:text" json:"tenant_id"`
	Name        string        `gorm:"type:text;not null" json:"name"`
	Expr        string        `gorm:"type:text;not null" json:"expr"`
	For         string        `gorm:"type:text" json:"for"` 
	Severity    AlertSeverity `gorm:"type:text" json:"severity"`
	Summary     string        `gorm:"type:text" json:"summary"`
	Description string        `gorm:"type:text" json:"description"`
	
	Labels map[string]string `gorm:"serializer:json" json:"labels"`
	
	Channels []string `gorm:"serializer:json" json:"channels"`
	Enabled  bool     `gorm:"default:true" json:"enabled"`
	CreatedBy string  `gorm:"type:text" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (AlertRule) TableName() string { return "alert_rules" }

type AlertSilence struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	TenantID  string    `gorm:"index;type:text" json:"tenant_id"`
	RuleID    string    `gorm:"index;type:text" json:"rule_id"` 
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Comment   string    `gorm:"type:text" json:"comment"`
	CreatedBy string    `gorm:"type:text" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (AlertSilence) TableName() string { return "alert_silences" }