package storage

import (
	"context"
	"errors"
	"strings"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
)

type llmAdapterMountRepo struct{ db *gorm.DB }

func NewAdapterMountRepository(db *gorm.DB) ports.AdapterMountRepository {
	return &llmAdapterMountRepo{db: db}
}

func (s *Storage) AdapterMounts() ports.AdapterMountRepository {
	return NewAdapterMountRepository(s.db)
}

const defaultMountListLimit = 200

func toPortMount(row models.LLMAdapterMount) *ports.AdapterMount {
	return &ports.AdapterMount{
		ID:         row.ID,
		AdapterID:  row.AdapterID,
		ServiceID:  row.ServiceID,
		ServedName: row.ServedName,
		BaseModel:  row.BaseModel,
		TenantID:   row.TenantID,
		CreatedBy:  row.CreatedBy,
		CreatedAt:  row.CreatedAt.Unix(),
	}
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate")
}

func (r *llmAdapterMountRepo) Mount(ctx context.Context, m *ports.AdapterMount) error {
	if m == nil {
		return ports.ErrAdapterInvalid
	}
	m.Normalize()
	if err := m.Validate(); err != nil {
		return err
	}

	now := now()
	row := models.LLMAdapterMount{
		ID:         generateID(),
		AdapterID:  m.AdapterID,
		ServiceID:  m.ServiceID,
		ServedName: m.ServedName,
		BaseModel:  m.BaseModel,
		TenantID:   m.TenantID,
		CreatedBy:  m.CreatedBy,
		CreatedAt:  now,
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.LLMAdapterMount{}).
			Where("tenant_id = ? AND served_name = ?", m.TenantID, m.ServedName).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ports.ErrMountConflict
		}
		if err := tx.Create(&row).Error; err != nil {
			if isUniqueViolation(err) {
				return ports.ErrMountConflict
			}
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	m.ID = row.ID
	m.CreatedAt = now.Unix()
	return nil
}

func (r *llmAdapterMountRepo) Unmount(ctx context.Context, tenantID, adapterID, serviceID string) error {
	q := scopeTenant(r.db.WithContext(ctx), tenantID)
	res := q.Where("adapter_id = ? AND service_id = ?", adapterID, serviceID).
		Delete(&models.LLMAdapterMount{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrAdapterNotMounted
	}
	return nil
}

func (r *llmAdapterMountRepo) listMounts(ctx context.Context, tenantID, column, value string) ([]*ports.AdapterMount, error) {
	q := scopeTenant(r.db.WithContext(ctx), tenantID)

	var rows []models.LLMAdapterMount
	if err := q.Where(column+" = ?", value).
		Order("created_at DESC").
		Limit(defaultMountListLimit).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]*ports.AdapterMount, 0, len(rows))
	for i := range rows {
		out = append(out, toPortMount(rows[i]))
	}
	return out, nil
}

func (r *llmAdapterMountRepo) ListByAdapter(ctx context.Context, tenantID, adapterID string) ([]*ports.AdapterMount, error) {
	return r.listMounts(ctx, tenantID, "adapter_id", adapterID)
}

func (r *llmAdapterMountRepo) ListByService(ctx context.Context, tenantID, serviceID string) ([]*ports.AdapterMount, error) {
	return r.listMounts(ctx, tenantID, "service_id", serviceID)
}

func (r *llmAdapterMountRepo) ResolveServedName(ctx context.Context, tenantID, servedName string) (*ports.AdapterMount, error) {
	var row models.LLMAdapterMount
	q := scopeTenant(r.db.WithContext(ctx), tenantID)
	err := q.First(&row, "served_name = ?", servedName).Error
	if isNotFoundErr(err) {
		
		return nil, ports.ErrAdapterNotMounted
	}
	if err != nil {
		return nil, err
	}
	return toPortMount(row), nil
}