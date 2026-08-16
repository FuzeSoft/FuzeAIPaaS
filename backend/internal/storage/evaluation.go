package storage

import (
	"context"
	"errors"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
)

type evaluationRepo struct {
	db *gorm.DB
}

func NewEvaluationRepository(db *gorm.DB) ports.EvaluationRepository {
	return &evaluationRepo{db: db}
}

func (s *Storage) Evaluation() ports.EvaluationRepository {
	return NewEvaluationRepository(s.db)
}

func isEvalNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func (r *evaluationRepo) Create(ctx context.Context, e *models.Evaluation) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *evaluationRepo) Get(ctx context.Context, id string) (*models.Evaluation, error) {
	var e models.Evaluation
	if err := r.db.WithContext(ctx).First(&e, "id = ?", id).Error; err != nil {
		if isEvalNotFound(err) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return &e, nil
}

func (r *evaluationRepo) List(ctx context.Context, tenantID string) ([]models.Evaluation, error) {
	var list []models.Evaluation
	q := r.db.WithContext(ctx)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *evaluationRepo) ListByExperiment(ctx context.Context, experimentID string) ([]models.Evaluation, error) {
	var list []models.Evaluation
	if err := r.db.WithContext(ctx).Where("experiment_id = ?", experimentID).Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *evaluationRepo) ListByModel(ctx context.Context, modelID string) ([]models.Evaluation, error) {
	var list []models.Evaluation
	if err := r.db.WithContext(ctx).Where("model_id = ?", modelID).Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *evaluationRepo) Update(ctx context.Context, e *models.Evaluation) error {
	return r.db.WithContext(ctx).Save(e).Error
}

func (r *evaluationRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Evaluation{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (r *evaluationRepo) CreateReview(ctx context.Context, rv *models.EvaluationReview) error {
	return r.db.WithContext(ctx).Create(rv).Error
}

func (r *evaluationRepo) ListReviews(ctx context.Context, evaluationID string) ([]models.EvaluationReview, error) {
	var list []models.EvaluationReview
	if err := r.db.WithContext(ctx).Where("evaluation_id = ?", evaluationID).
		Order("created_at ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}