package storage

import (
	"context"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
)

type llmAdapterRepo struct{ db *gorm.DB }

func NewFineTuneRepository(db *gorm.DB) ports.FineTuneRepository {
	return &llmAdapterRepo{db: db}
}

func (s *Storage) FineTune() ports.FineTuneRepository { return NewFineTuneRepository(s.db) }

const defaultAdapterListLimit = 200

func scopeTenant(q *gorm.DB, tenantID string) *gorm.DB {
	if tenantID == "" {
		return q
	}
	return q.Where("tenant_id = ?", tenantID)
}

func toPortAdapter(row models.LLMAdapter) *ports.FineTuneAdapter {
	return &ports.FineTuneAdapter{
		ID:          row.ID,
		Name:        row.Name,
		BaseModel:   row.BaseModel,
		Path:        row.Path,
		Rank:        row.Rank,
		Method:      row.Method,
		SourceJobID: row.SourceJobID,
		TenantID:    row.TenantID,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt.Unix(),
	}
}

func (r *llmAdapterRepo) Create(ctx context.Context, a *ports.FineTuneAdapter) error {
	if a == nil {
		return ports.ErrAdapterInvalid
	}
	now := now()
	row := models.LLMAdapter{
		ID:          generateID(),
		Name:        a.Name,
		BaseModel:   a.BaseModel,
		Path:        a.Path,
		Rank:        a.Rank,
		Method:      a.Method,
		SourceJobID: a.SourceJobID,
		TenantID:    a.TenantID,
		CreatedBy:   a.CreatedBy,
		CreatedAt:   now,
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.LLMAdapter{}).
			Where("tenant_id = ? AND name = ?", a.TenantID, a.Name).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ports.ErrAdapterConflict
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return err
	}

	a.ID = row.ID
	a.CreatedAt = now.Unix()
	return nil
}

func (r *llmAdapterRepo) Get(ctx context.Context, tenantID, id string) (*ports.FineTuneAdapter, error) {
	var row models.LLMAdapter
	q := scopeTenant(r.db.WithContext(ctx), tenantID)
	err := q.First(&row, "id = ?", id).Error
	if isNotFoundErr(err) {
		
		return nil, ports.ErrAdapterNotFound
	}
	if err != nil {
		return nil, err
	}
	return toPortAdapter(row), nil
}

func (r *llmAdapterRepo) List(ctx context.Context, f ports.FineTuneFilter) ([]*ports.FineTuneAdapter, error) {
	limit := f.Limit
	if limit <= 0 || limit > defaultAdapterListLimit {
		limit = defaultAdapterListLimit
	}

	q := scopeTenant(r.db.WithContext(ctx), f.TenantID)
	if f.BaseModel != "" {
		q = q.Where("base_model = ?", f.BaseModel)
	}

	var rows []models.LLMAdapter
	if err := q.Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]*ports.FineTuneAdapter, 0, len(rows))
	for i := range rows {
		out = append(out, toPortAdapter(rows[i]))
	}
	return out, nil
}

func (r *llmAdapterRepo) Delete(ctx context.Context, tenantID, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var mounted int64
		if err := scopeTenant(tx.Model(&models.LLMAdapterMount{}), tenantID).
			Where("adapter_id = ?", id).
			Count(&mounted).Error; err != nil {
			return err
		}
		if mounted > 0 {
			return ports.ErrAdapterMounted
		}

		res := scopeTenant(tx, tenantID).Where("id = ?", id).Delete(&models.LLMAdapter{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ports.ErrAdapterNotFound
		}
		return nil
	})
}