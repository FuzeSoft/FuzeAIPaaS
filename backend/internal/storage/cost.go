package storage

import (
	"context"
	"errors"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type costRepo struct {
	*llmQuotaRepo
}

func NewCostRepository(db *gorm.DB) ports.CostRepository {
	return &costRepo{llmQuotaRepo: &llmQuotaRepo{db: db}}
}

func (r *costRepo) RecordGPUCost(ctx context.Context, rec *models.GPUUsageRecord) error {
	if rec.ID == "" {
		rec.ID = genID()
	}
	if rec.CreatedAt == 0 {
		rec.CreatedAt = now().Unix()
	}
	
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "job_id"}},
			DoNothing: true,
		}).Create(rec)
		if res.Error != nil {
			return res.Error
		}
		
		if res.RowsAffected == 0 {
			return nil
		}
		
		return consumeCostInTx(tx, rec.TenantID, 0, rec.Cost)
	})
}

func consumeCostInTx(tx *gorm.DB, tenantID string, tokens int64, cost float64) error {
	var q models.LLMTokenQuota
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&q, "tenant_id = ?", tenantID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	
	q.UsedTokens += int64(tokens)
	q.UsedCost += cost
	q.UpdatedAt = now()
	return tx.Save(&q).Error
}

func (r *costRepo) SumGPUUsage(ctx context.Context, tenantID string, since, until int64) (float64, float64, error) {
	type agg struct {
		Hours float64
		Cost  float64
	}
	var row agg
	q := r.llmQuotaRepo.db.WithContext(ctx).Model(&models.GPUUsageRecord{}).
		Select("COALESCE(SUM(hours),0) AS hours, COALESCE(SUM(cost),0) AS cost")
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if since > 0 {
		q = q.Where("created_at >= ?", since)
	}
	if until > 0 {
		q = q.Where("created_at <= ?", until)
	}
	if err := q.Scan(&row).Error; err != nil {
		return 0, 0, err
	}
	return row.Hours, row.Cost, nil
}