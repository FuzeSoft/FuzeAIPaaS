package storage

import (
	"errors"
	"fmt"

	"fuze-ai-paas/backend/internal/models"

	"gorm.io/gorm"
)

var ErrModelCredentialNotFound = errors.New("storage: model credential not found")

func (s *Storage) ListModelCredentials(tenantID string) ([]models.ModelCredential, error) {
	var creds []models.ModelCredential
	if err := s.db.Where("tenant_id = ?", tenantID).Find(&creds).Error; err != nil {
		return nil, err
	}
	for i := range creds {
		if err := s.decryptModelCredential(&creds[i]); err != nil {
			return nil, err
		}
	}
	return creds, nil
}

func (s *Storage) UpsertModelCredential(cred *models.ModelCredential) error {
	if cred.ID == "" {
		var existing models.ModelCredential
		err := s.db.Where("tenant_id = ? AND backend = ? AND name = ?",
			cred.TenantID, cred.Backend, cred.Name).First(&existing).Error
		if err == nil {
			cred.ID = existing.ID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	cred.UpdatedAt = now()
	if cred.ID == "" {
		cred.ID = generateID()
		cred.CreatedAt = now()
	}
	if err := s.encryptModelCredential(cred); err != nil {
		return err
	}
	return s.db.Save(cred).Error
}

func (s *Storage) DeleteModelCredential(tenantID, id string) error {
	var cred models.ModelCredential
	err := s.db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&cred).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrModelCredentialNotFound
	}
	if err != nil {
		return err
	}
	return s.db.Delete(&cred).Error
}

func (s *Storage) encryptModelCredential(cred *models.ModelCredential) error {
	if cred.APIKey == "" {
		cred.APIKeyEnc = ""
		return nil
	}
	if s.cipher == nil {
		
		cred.APIKeyEnc = ""
		return nil
	}
	enc, err := s.cipher.EncryptString(cred.APIKey)
	if err != nil {
		return fmt.Errorf("storage: encrypt model credential: %w", err)
	}
	cred.APIKeyEnc = enc
	cred.APIKey = ""
	return nil
}

func (s *Storage) decryptModelCredential(cred *models.ModelCredential) error {
	if cred.APIKeyEnc == "" {
		return nil
	}
	if s.cipher == nil {
		return fmt.Errorf("storage: model credential %s has encrypted api_key but no cipher configured", cred.ID)
	}
	plain, err := s.cipher.DecryptString(cred.APIKeyEnc)
	if err != nil {
		return fmt.Errorf("storage: decrypt model credential %s: %w", cred.ID, err)
	}
	cred.APIKey = plain
	cred.APIKeyEnc = ""
	return nil
}