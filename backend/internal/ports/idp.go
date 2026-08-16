package ports

import (
	"context"

	"fuze-ai-paas/backend/internal/models"
)

type IdPRegistry interface {
	
	List(ctx context.Context) ([]models.IdPConfig, error)
	
	ListEnabled(ctx context.Context) ([]models.IdPConfig, error)
	
	Get(ctx context.Context, providerID string) (*models.IdPConfig, error)
	
	Create(ctx context.Context, cfg *models.IdPConfig) error
	
	Update(ctx context.Context, cfg *models.IdPConfig) error
	
	Delete(ctx context.Context, providerID string) error
}