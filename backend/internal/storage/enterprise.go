package storage

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func isTransientLockError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "database is locked"),
		strings.Contains(msg, "sqlite_busy"),
		strings.Contains(msg, "locked"),
		strings.Contains(msg, "serialization failure"),
		strings.Contains(msg, "40p01"),
		strings.Contains(msg, "40001"):
		return true
	}
	return false
}

func (s *Storage) retryTx(fn func(*gorm.DB) error) error {
	const (
		maxAttempts    = 10
		baseBackoff    = 5 * time.Millisecond
		maxBackoffStep = 800 * time.Millisecond
	)
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		lastErr = s.db.Transaction(fn)
		if lastErr == nil {
			return nil
		}
		
		if errors.Is(lastErr, ports.ErrQuotaExceeded) || errors.Is(lastErr, gorm.ErrRecordNotFound) {
			return lastErr
		}
		if !isTransientLockError(lastErr) {
			return lastErr
		}
		if attempt == maxAttempts-1 {
			break 
		}
		
		backoff := baseBackoff << attempt
		if backoff > maxBackoffStep || backoff <= 0 { 
			backoff = maxBackoffStep
		}
		time.Sleep(backoff/2 + time.Duration(rand.Int63n(int64(backoff/2)+1)))
	}
	return fmt.Errorf("transaction still conflicting after %d attempts: %w", maxAttempts, lastErr)
}

func (s *Storage) GetUserByUsername(username string) (*models.User, error) {
	var u models.User
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Storage) GetUser(id string) (*models.User, error) {
	var u models.User
	if err := s.db.Where("id = ?", id).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

const defaultListLimit = 1000

func (s *Storage) ListUsers() ([]models.User, error) {
	var users []models.User
	err := s.db.Order("created_at ASC").Find(&users).Error
	return users, err
}

func (s *Storage) CreateUser(u *models.User) error {
	return s.db.Create(u).Error
}

func (s *Storage) UpdateUser(u *models.User) error {
	return s.db.Save(u).Error
}

func (s *Storage) UpdateUserMFARecovery(userID, recoveryEnc string) error {
	return s.db.Model(&models.User{}).
		Where("id = ?", userID).
		Update("mfa_recovery_enc", recoveryEnc).Error
}

func (s *Storage) GetUserByID(id string) (*models.User, error) {
	return s.GetUser(id)
}

func (s *Storage) RecordLoginFailure(username string, maxFails, lockSec int) (*models.User, bool, error) {
	var locked bool
	
	var result models.User
	err := s.retryTx(func(tx *gorm.DB) error {
		var u models.User
		if err := tx.Where("username = ?", username).First(&u).Error; err != nil {
			return err
		}
		u.LoginFails++
		if maxFails > 0 && u.LoginFails >= maxFails {
			t := now().Add(time.Duration(lockSec) * time.Second)
			u.LockedUntil = &t
			u.LoginFails = 0 
			locked = true
		}
		if err := tx.Model(&models.User{}).
			Where("id = ?", u.ID).
			Updates(map[string]interface{}{
				"login_fails":  u.LoginFails,
				"locked_until": u.LockedUntil,
			}).Error; err != nil {
			return err
		}
		result = u
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &result, locked, nil
}

func (s *Storage) ClearLoginFailures(username string) error {
	return s.db.Model(&models.User{}).
		Where("username = ?", username).
		Updates(map[string]interface{}{
			"login_fails":  0,
			"locked_until": nil,
		}).Error
}

func (s *Storage) UpsertSSOUser(u *models.User) (*models.User, error) {
	var result *models.User
	err := s.retryTx(func(tx *gorm.DB) error {
		var existing models.User
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("username = ?", u.Username).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if e := tx.Create(u).Error; e != nil {
				return e
			}
			result = u
			return nil
		}
		if err != nil {
			return err
		}
		existing.SSOProvider = u.SSOProvider
		existing.Email = u.Email
		existing.DisplayName = u.DisplayName
		existing.Enabled = true
		if e := tx.Save(&existing).Error; e != nil {
			return e
		}
		result = &existing
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Storage) GetTenant(id string) (*models.Tenant, error) {
	var t models.Tenant
	if err := s.db.Where("id = ?", id).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Storage) ListTenants() ([]models.Tenant, error) {
	var ts []models.Tenant
	err := s.db.Order("created_at ASC").Find(&ts).Error
	return ts, err
}

func (s *Storage) CreateTenant(t *models.Tenant) error {
	return s.db.Create(t).Error
}

func (s *Storage) UpdateTenant(t *models.Tenant) error {
	t.UpdatedAt = now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		var existing models.Tenant
		if err := tx.Select("id").First(&existing, "id = ?", t.ID).Error; err != nil {
			return err
		}
		return tx.Model(&models.Tenant{}).
			Where("id = ?", t.ID).
			Select("Name", "Description", "UpdatedAt").
			Updates(t).Error
	})
}

func (s *Storage) DeleteTenant(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ?", id).Delete(&models.Quota{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", id).Delete(&models.Tenant{}).Error; err != nil {
			return err
		}
		return nil
	})
}

func (s *Storage) GetQuota(tenantID string) (*models.Quota, error) {
	var q models.Quota
	if err := s.db.Where("tenant_id = ?", tenantID).First(&q).Error; err != nil {
		return nil, err
	}
	return &q, nil
}

func (s *Storage) ListQuotas() ([]models.Quota, error) {
	var qs []models.Quota
	err := s.db.Order("tenant_id ASC").Find(&qs).Error
	return qs, err
}

func (s *Storage) UpsertQuota(q *models.Quota) error {
	if q.ID == "" {
		q.ID = q.TenantID
	}
	q.UpdatedAt = now()
	
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"gpu_quota", "memory_quota_gb", "job_quota",
			"gpu_used", "memory_used_gb", "job_used", "updated_at",
		}),
	}).Create(q).Error
}

func (s *Storage) CheckAndReserve(tenantID string, gpus, memGB, jobs int) error {
	return s.retryTx(func(tx *gorm.DB) error {
		var q models.Quota
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ?", tenantID).First(&q).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("quota not configured for tenant %s", tenantID)
			}
			return err
		}
		
		if q.GPUUsed+gpus > q.GPUQuota {
			return fmt.Errorf("%w: gpu quota exceeded: used %d + requested %d > limit %d", ports.ErrQuotaExceeded, q.GPUUsed, gpus, q.GPUQuota)
		}
		if q.MemoryUsedGB+memGB > q.MemoryQuotaGB {
			return fmt.Errorf("%w: memory quota exceeded: used %d + requested %d > limit %d", ports.ErrQuotaExceeded, q.MemoryUsedGB, memGB, q.MemoryQuotaGB)
		}
		if q.JobUsed+jobs > q.JobQuota {
			return fmt.Errorf("%w: job quota exceeded: used %d + requested %d > limit %d", ports.ErrQuotaExceeded, q.JobUsed, jobs, q.JobQuota)
		}
		q.GPUUsed += gpus
		q.MemoryUsedGB += memGB
		q.JobUsed += jobs
		return tx.Save(&q).Error
	})
}

func (s *Storage) Release(tenantID string, gpus, memGB, jobs int) error {
	return s.retryTx(func(tx *gorm.DB) error {
		var q models.Quota
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ?", tenantID).First(&q).Error; err != nil {
			return err
		}
		q.GPUUsed = max(0, q.GPUUsed-gpus)
		q.MemoryUsedGB = max(0, q.MemoryUsedGB-memGB)
		q.JobUsed = max(0, q.JobUsed-jobs)
		return tx.Save(&q).Error
	})
}

func adjustReservationTx(tx *gorm.DB, tenantID string, oldGPUs, oldMemGB, newGPUs, newMemGB int) error {
	if oldGPUs == newGPUs && oldMemGB == newMemGB {
		return nil
	}
	var q models.Quota
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ?", tenantID).First(&q).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("quota not configured for tenant %s", tenantID)
		}
		return err
	}
	
	q.GPUUsed = max(0, q.GPUUsed-oldGPUs)
	q.MemoryUsedGB = max(0, q.MemoryUsedGB-oldMemGB)
	
	if q.GPUUsed+newGPUs > q.GPUQuota {
		return fmt.Errorf("%w: gpu quota exceeded: used %d + requested %d > limit %d", ports.ErrQuotaExceeded, q.GPUUsed, newGPUs, q.GPUQuota)
	}
	if q.MemoryUsedGB+newMemGB > q.MemoryQuotaGB {
		return fmt.Errorf("%w: memory quota exceeded: used %d + requested %d > limit %d", ports.ErrQuotaExceeded, q.MemoryUsedGB, newMemGB, q.MemoryQuotaGB)
	}
	q.GPUUsed += newGPUs
	q.MemoryUsedGB += newMemGB
	return tx.Save(&q).Error
}

func (s *Storage) AdjustReservation(tenantID string, oldGPUs, oldMemGB, newGPUs, newMemGB int) error {
	return adjustReservationTx(s.db, tenantID, oldGPUs, oldMemGB, newGPUs, newMemGB)
}

func (s *Storage) Record(e *models.AuditLog) error {
	return s.db.Create(e).Error
}

func (s *Storage) ListAudit(opts AuditQuery) ([]models.AuditLog, error) {
	q := s.db.Model(&models.AuditLog{})
	if opts.Actor != "" {
		q = q.Where("actor = ?", opts.Actor)
	}
	if opts.TenantID != "" {
		q = q.Where("tenant_id = ?", opts.TenantID)
	}
	if opts.Action != "" {
		q = q.Where("action = ?", opts.Action)
	}
	if opts.ResourceType != "" {
		q = q.Where("resource_type = ?", opts.ResourceType)
	}
	
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var logs []models.AuditLog
	err := q.Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}