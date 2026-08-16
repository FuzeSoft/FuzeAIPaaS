package storage

import (
	"errors"
	"time"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Storage) CreateWorkspace(ws *models.Workspace) error {
	if ws.ID == "" {
		ws.ID = generateID()
	}
	now := now()
	if ws.CreatedAt.IsZero() {
		ws.CreatedAt = now
	}
	if ws.UpdatedAt.IsZero() {
		ws.UpdatedAt = now
	}
	if ws.Status == "" {
		ws.Status = models.WorkspaceStatusPending
	}
	return s.db.Create(ws).Error
}

func (s *Storage) GetWorkspaceForTenant(tenantID, id string) (*models.Workspace, error) {
	var ws models.Workspace
	err := s.db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&ws).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return &ws, nil
}

func (s *Storage) GetWorkspaceByName(tenantID, name string) (*models.Workspace, error) {
	var ws models.Workspace
	err := s.db.Where("tenant_id = ? AND name = ?", tenantID, name).First(&ws).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return &ws, nil
}

func (s *Storage) ListWorkspaces(tenantID string, filter ports.WorkspaceFilter) ([]models.Workspace, error) {
	tx := s.db.Model(&models.Workspace{})
	if tenantID != "" {
		tx = tx.Where("tenant_id = ?", tenantID)
	}
	if filter.Status != "" {
		tx = tx.Where("status = ?", filter.Status)
	}
	if filter.OwnerID != "" {
		tx = tx.Where("owner_id = ?", filter.OwnerID)
	}
	var list []models.Workspace
	if err := tx.Order("created_at DESC").Limit(defaultListLimit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Storage) UpdateWorkspace(ws *models.Workspace) error {
	return s.db.Save(ws).Error
}

func (s *Storage) DeleteWorkspace(id string) error {
	return s.db.Delete(&models.Workspace{}, "id = ?", id).Error
}

func (s *Storage) DeleteWorkspaceAndReleaseQuota(id, tenantID string, gpus, memGB int) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var q models.Quota
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ?", tenantID).First(&q).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				
				return tx.Delete(&models.Workspace{}, "id = ?", id).Error
			}
			return err
		}
		q.GPUUsed = max(0, q.GPUUsed-gpus)
		q.MemoryUsedGB = max(0, q.MemoryUsedGB-memGB)
		q.JobUsed = max(0, q.JobUsed-1)
		if err := tx.Save(&q).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Workspace{}, "id = ?", id).Error
	})
}

func (s *Storage) GetWorkspaceByID(id string) (*models.Workspace, error) {
	var ws models.Workspace
	if err := s.db.Where("id = ?", id).First(&ws).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return &ws, nil
}

func (s *Storage) TouchWorkspace(id string, at time.Time) error {
	res := s.db.Model(&models.Workspace{}).
		Where("id = ? AND (last_active_at IS NULL OR last_active_at <= ?)", id, at).
		Update("last_active_at", at)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		
		var n int64
		if err := s.db.Model(&models.Workspace{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return ports.ErrNotFound
		}
		
	}
	return nil
}

func (s *Storage) ListReclaimable(now time.Time) ([]models.Workspace, error) {
	var candidates []models.Workspace
	if err := s.db.
		Where("status = ?", models.WorkspaceStatusRunning).
		Where("idle_timeout > 0").
		Where("last_active_at IS NOT NULL").
		Find(&candidates).Error; err != nil {
		return nil, err
	}
	out := make([]models.Workspace, 0, len(candidates))
	for _, ws := range candidates {
		cutoff := now.Add(-ws.IdleTimeout)
		if ws.LastActiveAt != nil && ws.LastActiveAt.Before(cutoff) {
			out = append(out, ws)
		}
	}
	return out, nil
}