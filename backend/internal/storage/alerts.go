package storage

import (
	"context"
	"fmt"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
)

type AlertRepo struct {
	db *gorm.DB
}

func NewAlertRepository(db *gorm.DB) *AlertRepo {
	return &AlertRepo{db: db}
}

func (r *AlertRepo) CreateRule(rule *models.AlertRule) error {
	if rule.ID == "" || rule.Name == "" || rule.Expr == "" {
		return fmt.Errorf("%w: id, name, expr 不能为空", ports.ErrAlertRuleInvalid)
	}
	if rule.CreatedAt.IsZero() {
		now := now().UTC()
		rule.CreatedAt, rule.UpdatedAt = now, now
	}
	if err := r.db.WithContext(context.Background()).Create(rule).Error; err != nil {
		return fmt.Errorf("create alert rule: %w", err)
	}
	return nil
}

func (r *AlertRepo) UpdateRule(rule *models.AlertRule) error {
	if rule.ID == "" || rule.TenantID == "" {
		return fmt.Errorf("%w: id 与 tenant_id 不能为空", ports.ErrAlertRuleInvalid)
	}
	rule.UpdatedAt = now().UTC()
	res := r.db.WithContext(context.Background()).
		Model(&models.AlertRule{}).
		Where("id = ? AND tenant_id = ?", rule.ID, rule.TenantID).
		Select("*").
		Updates(rule)
	if res.Error != nil {
		return fmt.Errorf("update alert rule: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ports.ErrAlertRuleNotFound
	}
	return nil
}

func (r *AlertRepo) GetRule(tenantID, id string) (*models.AlertRule, error) {
	var rule models.AlertRule
	err := r.db.WithContext(context.Background()).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		First(&rule).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ports.ErrAlertRuleNotFound
		}
		return nil, fmt.Errorf("get alert rule: %w", err)
	}
	return &rule, nil
}

func (r *AlertRepo) ListRules(tenantID string) ([]models.AlertRule, error) {
	var out []models.AlertRule
	err := r.db.WithContext(context.Background()).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("list alert rules: %w", err)
	}
	return out, nil
}

func (r *AlertRepo) DeleteRule(tenantID, id string) error {
	res := r.db.WithContext(context.Background()).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&models.AlertRule{})
	if res.Error != nil {
		return fmt.Errorf("delete alert rule: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ports.ErrAlertRuleNotFound
	}
	return nil
}

func (r *AlertRepo) CreateSilence(s *models.AlertSilence) error {
	if s.ID == "" || s.TenantID == "" {
		return fmt.Errorf("%w: id 与 tenant_id 不能为空", ports.ErrAlertSilenceInvalid)
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now().UTC()
	}
	if err := r.db.WithContext(context.Background()).Create(s).Error; err != nil {
		return fmt.Errorf("create silence: %w", err)
	}
	return nil
}

func (r *AlertRepo) ListSilences(tenantID string) ([]models.AlertSilence, error) {
	var out []models.AlertSilence
	err := r.db.WithContext(context.Background()).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("list silences: %w", err)
	}
	return out, nil
}

func (r *AlertRepo) DeleteSilence(tenantID, id string) error {
	res := r.db.WithContext(context.Background()).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&models.AlertSilence{})
	if res.Error != nil {
		return fmt.Errorf("delete silence: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ports.ErrAlertSilenceNotFound
	}
	return nil
}