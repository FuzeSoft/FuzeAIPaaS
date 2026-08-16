package models

import "time"

type IdPType string

const (
	
	IdPOIDC IdPType = "oidc"
	
	IdPLDAP IdPType = "ldap"
)

type IdPConfig struct {
	ProviderID string  `json:"provider_id" gorm:"primaryKey"` 
	Type       IdPType `json:"type"`                          
	Name       string  `json:"name"`                          
	Enabled    bool    `json:"enabled"`

	Issuer       string `json:"issuer,omitempty"`       
	ClientID     string `json:"client_id,omitempty"`    
	ClientSecret string `json:"-"`                      
	
	ClientSecretEnc string `json:"-" gorm:"column:client_secret_enc"`
	RedirectURI     string `json:"redirect_uri,omitempty"` 
	Scopes          string `json:"scopes,omitempty"`       

	LDAPAddr         string `json:"ldap_addr,omitempty"`           
	LDAPUseTLS       bool   `json:"ldap_use_tls,omitempty"`        
	LDAPSkipVerify   bool   `json:"ldap_skip_verify,omitempty"`    
	LDAPUserDNFormat string `json:"ldap_user_dn_format,omitempty"` 

	DefaultRole   Role     `json:"default_role,omitempty"`
	AdminGroups   []string `json:"admin_groups,omitempty" gorm:"serializer:json"`
	AdminRole     Role     `json:"admin_role,omitempty"`
	DefaultTenant string   `json:"default_tenant,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (IdPConfig) TableName() string { return "idp_configs" }

func (c IdPConfig) IsValid() bool {
	switch c.Type {
	case IdPOIDC:
		return c.Issuer != "" && c.ClientID != ""
	case IdPLDAP:
		return c.LDAPAddr != "" && c.LDAPUserDNFormat != ""
	default:
		return false
	}
}