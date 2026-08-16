package models

import "time"

type Role string

const (
	RolePlatformAdmin Role = "platform_admin" 
	RoleTenantAdmin   Role = "tenant_admin"   
	RoleDeveloper     Role = "developer"      
	RoleViewer        Role = "viewer"         
)

func AllRoles() []Role {
	return []Role{RolePlatformAdmin, RoleTenantAdmin, RoleDeveloper, RoleViewer}
}

func ValidRole(r Role) bool {
	for _, x := range AllRoles() {
		if x == r {
			return true
		}
	}
	return false
}

type User struct {
	ID          string `gorm:"primaryKey;size:64" json:"id"`
	Username    string `gorm:"uniqueIndex;size:128" json:"username"`
	Password    string `gorm:"size:255" json:"-"` 
	DisplayName string `gorm:"size:128" json:"display_name"`
	Role        Role   `gorm:"size:32;default:developer" json:"role"`
	TenantID    string `gorm:"size:64;index" json:"tenant_id"`
	Email       string `gorm:"size:255" json:"email"`
	SSOProvider string `gorm:"size:64" json:"sso_provider"` 
	Enabled     bool   `gorm:"default:true" json:"enabled"`

	MFAEnabled     bool   `gorm:"default:false" json:"mfa_enabled"`  
	MFAEnforced    bool   `gorm:"default:false" json:"mfa_enforced"` 
	TOTPSecretEnc  string `gorm:"type:text" json:"-"`                
	MFARecoveryEnc string `gorm:"type:text" json:"-"`                
	PendingTOTPEnc  string `gorm:"type:text" json:"-"`               
	MFARecoveryPendingEnc string `gorm:"type:text" json:"-"`         

	LoginFails  int        `gorm:"default:0" json:"-"`    
	LockedUntil *time.Time `gorm:"default:null" json:"-"` 

	PasskeyEnabled bool   `gorm:"default:false" json:"passkey_enabled"` 
	Passkeys       string `gorm:"type:text" json:"-"`                   

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (u *User) MFARequired() bool {
	return u.MFAEnabled || u.MFAEnforced
}

type Tenant struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"`
	Name        string    `gorm:"uniqueIndex;size:128" json:"name"`
	Description string    `gorm:"size:255" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Quota struct {
	ID            string    `gorm:"primaryKey;size:64" json:"id"`
	TenantID      string    `gorm:"uniqueIndex;size:64" json:"tenant_id"`
	Tenant        *Tenant   `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	GPUQuota      int       `gorm:"default:0" json:"gpu_quota"`       
	GPUUsed       int       `gorm:"default:0" json:"gpu_used"`        
	MemoryQuotaGB int       `gorm:"default:0" json:"memory_quota_gb"` 
	MemoryUsedGB  int       `gorm:"default:0" json:"memory_used_gb"`  
	JobQuota      int       `gorm:"default:0" json:"job_quota"`       
	JobUsed       int       `gorm:"default:0" json:"job_used"`        
	UpdatedAt     time.Time `json:"updated_at"`
}