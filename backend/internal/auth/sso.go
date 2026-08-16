package auth

import (
	"errors"
	"strings"

	"fuze-ai-paas/backend/internal/models"
)

type SSOUserInfo struct {
	Provider    string   
	Subject     string   
	Username    string   
	Email       string   
	DisplayName string   
	Groups      []string 
	
	AMR []string
}

type SSOUserProvisioner interface {
	UpsertSSOUser(u *models.User) (*models.User, error)
}

type SSORoleConfig struct {
	DefaultRole models.Role 
	AdminGroups []string    
	AdminRole   models.Role 
}

func (cfg SSORoleConfig) resolveRole(groups []string) models.Role {
	role := cfg.DefaultRole
	if role == "" {
		role = models.RoleDeveloper
	}
	admin := cfg.AdminRole
	if admin == "" {
		admin = models.RoleTenantAdmin
	}
	for _, g := range groups {
		for _, ag := range cfg.AdminGroups {
			if strings.EqualFold(strings.TrimSpace(g), strings.TrimSpace(ag)) {
				return admin
			}
		}
	}
	return role
}

func (m *Manager) SSOLogin(repo SSOUserProvisioner, info SSOUserInfo, cfg SSORoleConfig, defTenant string) (string, *models.User, error) {
	if info.Username == "" {
		return "", nil, errors.New("sso: empty username")
	}
	if defTenant == "" {
		defTenant = "default"
	}
	u := &models.User{
		ID:          "sso:" + info.Provider + ":" + info.Subject,
		Username:    info.Username,
		DisplayName: info.DisplayName,
		Email:       info.Email,
		Role:        cfg.resolveRole(info.Groups),
		TenantID:    defTenant,
		SSOProvider: info.Provider,
		Enabled:     true,
	}
	persisted, err := repo.UpsertSSOUser(u)
	if err != nil {
		return "", nil, err
	}
	
	if !amrHasMFA(info.AMR) && persisted.MFARequired() {
		temp, err := m.signMFATempToken(persisted)
		if err != nil {
			return "", nil, err
		}
		return temp, persisted, nil
	}
	token, err := m.Sign(&Claims{
		UserID:   persisted.ID,
		Username: persisted.Username,
		Role:     persisted.Role,
		TenantID: persisted.TenantID,
	})
	if err != nil {
		return "", nil, err
	}
	return token, persisted, nil
}

func amrHasMFA(amr []string) bool {
	for _, a := range amr {
		if strings.EqualFold(strings.TrimSpace(a), "mfa") {
			return true
		}
	}
	return false
}