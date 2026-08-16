package storage

import (
	"context"
	"fmt"

	"fuze-ai-paas/backend/internal/crypto/aes"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"gorm.io/gorm"
)

type idpRepo struct {
	db *gorm.DB
	
	cipher *aes.Cipher
}

func NewIdPRegistry(db *gorm.DB, cipher *aes.Cipher) ports.IdPRegistry {
	return &idpRepo{db: db, cipher: cipher}
}

func (r *idpRepo) encryptSecret(cfg *models.IdPConfig) error {
	if cfg.ClientSecret == "" {
		cfg.ClientSecretEnc = ""
		return nil
	}
	if r.cipher == nil {
		cfg.ClientSecretEnc = ""
		return nil
	}
	enc, err := r.cipher.EncryptString(cfg.ClientSecret)
	if err != nil {
		return err
	}
	cfg.ClientSecretEnc = enc
	cfg.ClientSecret = "" 
	return nil
}

func (r *idpRepo) decryptSecret(cfg *models.IdPConfig) error {
	if cfg.ClientSecretEnc == "" {
		return nil 
	}
	if r.cipher == nil {
		return fmt.Errorf("storage: idp %s has encrypted client_secret but no cipher configured", cfg.ProviderID)
	}
	plain, err := r.cipher.DecryptString(cfg.ClientSecretEnc)
	if err != nil {
		return fmt.Errorf("storage: decrypt client_secret for idp %s: %w", cfg.ProviderID, err)
	}
	cfg.ClientSecret = plain
	cfg.ClientSecretEnc = "" 
	return nil
}

func (r *idpRepo) List(ctx context.Context) ([]models.IdPConfig, error) {
	var list []models.IdPConfig
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		if err := r.decryptSecret(&list[i]); err != nil {
			return nil, err
		}
	}
	return list, nil
}

func (r *idpRepo) ListEnabled(ctx context.Context) ([]models.IdPConfig, error) {
	var list []models.IdPConfig
	if err := r.db.WithContext(ctx).Where("enabled = ?", true).Order("created_at ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		if err := r.decryptSecret(&list[i]); err != nil {
			return nil, err
		}
	}
	return list, nil
}

func (r *idpRepo) Get(ctx context.Context, providerID string) (*models.IdPConfig, error) {
	var cfg models.IdPConfig
	err := r.db.WithContext(ctx).First(&cfg, "provider_id = ?", providerID).Error
	if isNotFoundErr(err) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.decryptSecret(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *idpRepo) Create(ctx context.Context, cfg *models.IdPConfig) error {
	var cnt int64
	if err := r.db.WithContext(ctx).Model(&models.IdPConfig{}).Where("provider_id = ?", cfg.ProviderID).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return ports.ErrConflict
	}
	if err := r.encryptSecret(cfg); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(cfg).Error
}

func (r *idpRepo) Update(ctx context.Context, cfg *models.IdPConfig) error {
	existing, err := r.Get(ctx, cfg.ProviderID)
	if err != nil {
		return err
	}
	
	secret := cfg.ClientSecret
	if secret == "" {
		secret = existing.ClientSecret
	}
	cfg.ClientSecret = secret
	if err := r.encryptSecret(cfg); err != nil {
		return err
	}
	
	res := r.db.WithContext(ctx).Model(existing).Where("provider_id = ?", cfg.ProviderID).Updates(map[string]interface{}{
		"type":                cfg.Type,
		"name":                cfg.Name,
		"enabled":             cfg.Enabled,
		"issuer":              cfg.Issuer,
		"client_id":           cfg.ClientID,
		"client_secret":       cfg.ClientSecret,
		"client_secret_enc":   cfg.ClientSecretEnc,
		"redirect_uri":        cfg.RedirectURI,
		"scopes":              cfg.Scopes,
		"ldap_addr":           cfg.LDAPAddr,
		"ldap_use_tls":        cfg.LDAPUseTLS,
		"ldap_skip_verify":    cfg.LDAPSkipVerify,
		"ldap_user_dn_format": cfg.LDAPUserDNFormat,
		"default_role":        cfg.DefaultRole,
		"admin_groups":        cfg.AdminGroups,
		"admin_role":          cfg.AdminRole,
		"default_tenant":      cfg.DefaultTenant,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (r *idpRepo) Delete(ctx context.Context, providerID string) error {
	res := r.db.WithContext(ctx).Where("provider_id = ?", providerID).Delete(&models.IdPConfig{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}