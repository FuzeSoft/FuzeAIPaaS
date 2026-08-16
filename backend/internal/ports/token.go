package ports

import (
	"context"
	"time"

	"fuze-ai-paas/backend/internal/models"
)

type TokenRepository interface {
	
	Create(ctx context.Context, tok *models.PersonalAccessToken) error
	
	ListByUser(ctx context.Context, userID string) ([]models.PersonalAccessToken, error)
	
	Get(ctx context.Context, id string) (*models.PersonalAccessToken, error)
	
	GetByJTI(ctx context.Context, jti string) (*models.PersonalAccessToken, error)
	
	Update(ctx context.Context, tok *models.PersonalAccessToken) error
	
	ListDueRotation(ctx context.Context, before time.Time) ([]models.PersonalAccessToken, error)
	
	Delete(ctx context.Context, id string) error
	
	UpdateLastUsed(ctx context.Context, id string, at time.Time) error
	
	AddToBlacklist(ctx context.Context, entry *models.TokenBlacklist) error
	
	IsBlacklisted(ctx context.Context, jti string) (bool, error)
	
	BlacklistJTI(ctx context.Context, jti, reason string) error
}