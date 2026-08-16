package models

import "time"

type PersonalAccessToken struct {
	ID         string `gorm:"primaryKey" json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"` 
	UserID     string `gorm:"index" json:"user_id"`
	TenantID   string `json:"tenant_id"`
	
	JTI        string     `json:"jti,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	
	RotatedAt *time.Time `json:"rotated_at,omitempty"`
	
	RotatedFrom string `json:"rotated_from,omitempty" gorm:"column:rotated_from"`
	
	RotateBeforeDays int `json:"rotate_before_days,omitempty" gorm:"column:rotate_before_days"`
}

func (PersonalAccessToken) TableName() string { return "personal_access_tokens" }

type TokenBlacklist struct {
	JTI       string    `gorm:"primaryKey" json:"jti"`
	Subject   string    `gorm:"index" json:"subject"`    
	ExpiresAt int64     `gorm:"index" json:"expires_at"` 
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (TokenBlacklist) TableName() string { return "token_blacklist" }