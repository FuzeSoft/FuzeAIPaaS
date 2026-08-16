package storage

import (
	"errors"

	"fuze-ai-paas/backend/internal/crypto/aes"
	"fuze-ai-paas/backend/internal/models"
	"gorm.io/gorm"
)

var ErrRotationKeyMismatch = errors.New("key rotation: some kubeconfig ciphertext cannot be decrypted with the old key")

func (s *Storage) RotateKubeConfigKeys(oldC, newC *aes.Cipher) (int, error) {
	if oldC == nil || newC == nil {
		return 0, errors.New("key rotation: both old and new ciphers are required")
	}

	var rotated int
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var clusters []models.Cluster
		if err := tx.Where("kubeconfig_enc <> ?", "").Find(&clusters).Error; err != nil {
			return err
		}
		for i := range clusters {
			c := &clusters[i]
			plain, derr := oldC.DecryptString(c.KubeConfigEnc)
			if derr != nil {
				
				return ErrRotationKeyMismatch
			}
			enc, eerr := newC.EncryptString(plain)
			if eerr != nil {
				return eerr
			}
			if err := tx.Model(&models.Cluster{}).
				Where("id = ?", c.ID).
				Update("kubeconfig_enc", enc).Error; err != nil {
				return err
			}
			rotated++
		}
		
		if err := tx.Create(&models.AuditLog{
			Actor:        "system",
			Action:       models.ActionKeyRotation,
			ResourceType: models.ResCluster,
			ResourceID:   "kubeconfig_enc",
			Detail:       "rotated kubeconfig encryption key",
			CreatedAt:    now(),
		}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return rotated, nil
}

func (s *Storage) CountKubeConfigEnc() (int64, error) {
	var n int64
	if err := s.db.Model(&models.Cluster{}).Where("kubeconfig_enc <> ?", "").Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}