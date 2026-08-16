package models

import (
	"time"
)

type WorkspaceKind string

const (
	
	WorkspaceKindNotebook WorkspaceKind = "notebook"
)

type WorkspaceStatus string

const (
	WorkspaceStatusPending  WorkspaceStatus = "pending"
	WorkspaceStatusStarting WorkspaceStatus = "starting"
	WorkspaceStatusRunning  WorkspaceStatus = "running"
	WorkspaceStatusStopping WorkspaceStatus = "stopping"
	WorkspaceStatusStopped  WorkspaceStatus = "stopped"
	WorkspaceStatusFailed   WorkspaceStatus = "failed"
	WorkspaceStatusDeleting WorkspaceStatus = "deleting"
)

type Workspace struct {
	ID        string          `gorm:"primaryKey" json:"id"`
	TenantID  string          `gorm:"index:idx_ws_tenant_status;column:tenant_id"`
	Name      string          `gorm:"size:128;not null;index:idx_ws_tenant_name,unique"`
	Kind      WorkspaceKind   `gorm:"size:32;not null;default:notebook"`
	OwnerID   string          `gorm:"size:64;not null"`
	Image     string          `gorm:"size:255;not null"`
	Status    WorkspaceStatus `gorm:"index:idx_ws_tenant_status;size:32;not null;default:pending"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	
	CPURequest    string `gorm:"size:32"`
	MemoryRequest string `gorm:"size:32"`
	GPUCount      int    `gorm:"default:0"`
	GPUModel      string `gorm:"size:64"`
	
	IdleTimeout time.Duration `gorm:"default:0"`
	
	LastActiveAt *time.Time `gorm:"index"`
	
	StartedAt  *time.Time
	StoppedAt  *time.Time
	FailureMsg string `gorm:"size:512"`
	
	Mounts string `gorm:"type:text"`
	
	RuntimeName string `gorm:"size:128"`
	NodeName    string `gorm:"size:128"`
}