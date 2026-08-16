package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"fuze-ai-paas/backend/internal/domain/agent"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
)

type toolRepo struct{ db *gorm.DB }

func NewToolRepository(db *gorm.DB) ports.ToolRepository {
	return &toolRepo{db: db}
}

type ToolRow struct {
	ID          string `gorm:"primaryKey;size:64"`
	TenantID    string `gorm:"index;size:64"`
	Name        string `gorm:"index;size:256"`
	Description string `gorm:"type:text"`
	Kind        string `gorm:"size:32"`
	HTTPSpec    string `gorm:"column:http_spec;type:text"`
	Sensitive   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (ToolRow) TableName() string { return "agent_tools" }

func rowFromTool(t *agent.Tool) (ToolRow, error) {
	b, err := json.Marshal(t.HTTP)
	if err != nil {
		return ToolRow{}, err
	}
	return ToolRow{
		ID:          t.ID,
		TenantID:    t.TenantID,
		Name:        t.Name,
		Description: t.Description,
		Kind:        string(t.Kind),
		HTTPSpec:    string(b),
		Sensitive:   t.Sensitive,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}, nil
}

func (r ToolRow) toDomain() (*agent.Tool, error) {
	t := &agent.Tool{
		ID:          r.ID,
		TenantID:    r.TenantID,
		Name:        r.Name,
		Description: r.Description,
		Kind:        agent.ToolKind(r.Kind),
		Sensitive:   r.Sensitive,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
	if r.HTTPSpec != "" {
		var spec agent.HTTPToolSpec
		if err := json.Unmarshal([]byte(r.HTTPSpec), &spec); err != nil {
			return nil, err
		}
		t.HTTP = &spec
	}
	return t, nil
}

func (r *toolRepo) Create(ctx context.Context, t *agent.Tool) error {
	if t == nil {
		return errors.New("storage: tool is nil")
	}
	row, err := rowFromTool(t)
	if err != nil {
		return err
	}
	now := now()
	row.CreatedAt, row.UpdatedAt = now, now
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&ToolRow{}).
			Where("tenant_id = ? AND name = ?", t.TenantID, t.Name).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ports.ErrToolConflict
		}
		return tx.Create(&row).Error
	})
}

func (r *toolRepo) Get(ctx context.Context, tenantID, id string) (*agent.Tool, error) {
	var row ToolRow
	if err := scopeTenant(r.db.WithContext(ctx), tenantID).
		First(&row, "id = ?", id).Error; err != nil {
		if isNotFoundErr(err) {
			return nil, ports.ErrToolNotFound
		}
		return nil, err
	}
	return row.toDomain()
}

func (r *toolRepo) GetByName(ctx context.Context, tenantID, name string) (*agent.Tool, error) {
	var row ToolRow
	if err := scopeTenant(r.db.WithContext(ctx), tenantID).
		First(&row, "name = ?", name).Error; err != nil {
		if isNotFoundErr(err) {
			return nil, ports.ErrToolNotFound
		}
		return nil, err
	}
	return row.toDomain()
}

func (r *toolRepo) List(ctx context.Context, tenantID string) ([]*agent.Tool, error) {
	var rows []ToolRow
	if err := scopeTenant(r.db.WithContext(ctx), tenantID).
		Order("created_at DESC").Limit(defaultListLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*agent.Tool, 0, len(rows))
	for i := range rows {
		t, err := rows[i].toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *toolRepo) Update(ctx context.Context, t *agent.Tool) error {
	if t == nil {
		return errors.New("storage: tool is nil")
	}
	row, err := rowFromTool(t)
	if err != nil {
		return err
	}
	row.UpdatedAt = now()
	res := scopeTenant(r.db.WithContext(ctx), t.TenantID).
		Model(&ToolRow{}).
		Where("id = ?", t.ID).
		Updates(map[string]interface{}{
			"name":        row.Name,
			"description": row.Description,
			"kind":        row.Kind,
			"http_spec":   row.HTTPSpec,
			"sensitive":   row.Sensitive,
			"updated_at":  row.UpdatedAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrToolNotFound
	}
	return nil
}

func (r *toolRepo) Delete(ctx context.Context, tenantID, id string) error {
	res := scopeTenant(r.db.WithContext(ctx), tenantID).
		Where("id = ?", id).
		Delete(&ToolRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrToolNotFound
	}
	return nil
}

var _ ports.ToolRepository = (*toolRepo)(nil)