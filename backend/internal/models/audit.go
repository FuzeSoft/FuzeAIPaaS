package models

import "time"

const (
	ActionLogin      = "login"
	ActionLogout     = "logout"
	ActionCreate     = "create"
	ActionUpdate     = "update"
	ActionDelete     = "delete"
	ActionScale      = "scale"
	ActionCanary     = "canary"
	ActionStatusSync = "status_sync"
	
	ActionMount   = "mount"
	ActionUnmount = "unmount"
	
	ActionTokenRotate = "token_rotate"
	
	ActionIdPCreate = "idp_create"
	ActionIdPUpdate = "idp_update"
	ActionIdPDelete = "idp_delete"
	
	ActionLoginFailed = "login_failed"
	
	ActionKeyRotation = "key_rotation"
	
	ActionPasskeyRegister = "passkey_register"
	
	ActionMFASetup = "mfa_setup"
	
	ActionMFADisable = "mfa_disable"
	
	ActionRunFinish = "agent_run_finish"

	ResModel     = "model"
	ResModelVer  = "model_version"
	ResInference = "inference_service"
	ResCluster   = "cluster"
	ResJob       = "job"
	ResTenant    = "tenant"
	ResQuota     = "quota"
	ResUser      = "user"
	ResAuth      = "auth"
	ResWorkspace = "workspace"
	ResAdapter   = "llm_adapter"
	ResAgent     = "agent"
)

type AuditLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Actor        string    `gorm:"size:128;index" json:"actor"`    
	ActorID      string    `gorm:"size:64;index" json:"actor_id"`  
	ActorRole    Role      `gorm:"size:32" json:"actor_role"`      
	TenantID     string    `gorm:"size:64;index" json:"tenant_id"` 
	Action       string    `gorm:"size:32;index" json:"action"`    
	ResourceType string    `gorm:"size:32;index" json:"resource_type"`
	ResourceID   string    `gorm:"size:64;index" json:"resource_id"`
	Detail       string    `gorm:"type:text" json:"detail"`
	ClientIP     string    `gorm:"size:64" json:"client_ip"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}