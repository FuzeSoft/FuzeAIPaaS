package ports

import (
	"context"

	"fuze-ai-paas/backend/internal/models"
)

type EvaluationRepository interface {
	Create(ctx context.Context, e *models.Evaluation) error
	Get(ctx context.Context, id string) (*models.Evaluation, error)
	List(ctx context.Context, tenantID string) ([]models.Evaluation, error)
	ListByExperiment(ctx context.Context, experimentID string) ([]models.Evaluation, error)
	ListByModel(ctx context.Context, modelID string) ([]models.Evaluation, error)
	Update(ctx context.Context, e *models.Evaluation) error
	Delete(ctx context.Context, id string) error

	CreateReview(ctx context.Context, r *models.EvaluationReview) error
	
	ListReviews(ctx context.Context, evaluationID string) ([]models.EvaluationReview, error)
}