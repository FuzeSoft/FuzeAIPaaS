package storage

import (
	"context"
	"errors"
	"time"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
)

type tokenRepo struct {
	db *gorm.DB
}

func NewTokenRepository(db *gorm.DB) ports.TokenRepository {
	return &tokenRepo{db: db}
}

func (s *Storage) Token() ports.TokenRepository {
	return NewTokenRepository(s.db)
}

func isNotFoundErr(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func (r *tokenRepo) Create(ctx context.Context, tok *models.PersonalAccessToken) error {
	if tok.CreatedAt.IsZero() {
		tok.CreatedAt = now()
	}
	return r.db.WithContext(ctx).Create(tok).Error
}

func (r *tokenRepo) ListByUser(ctx context.Context, userID string) ([]models.PersonalAccessToken, error) {
	var list []models.PersonalAccessToken
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (r *tokenRepo) Get(ctx context.Context, id string) (*models.PersonalAccessToken, error) {
	var tok models.PersonalAccessToken
	err := r.db.WithContext(ctx).First(&tok, "id = ?", id).Error
	if isNotFoundErr(err) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tok, nil
}

func (r *tokenRepo) GetByJTI(ctx context.Context, jti string) (*models.PersonalAccessToken, error) {
	var tok models.PersonalAccessToken
	err := r.db.WithContext(ctx).First(&tok, "jti = ?", jti).Error
	if isNotFoundErr(err) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tok, nil
}

func (r *tokenRepo) Update(ctx context.Context, tok *models.PersonalAccessToken) error {
	return r.db.WithContext(ctx).
		Model(&models.PersonalAccessToken{}).
		Where("id = ?", tok.ID).
		Updates(map[string]interface{}{
			"rotated_at":         tok.RotatedAt,
			"rotated_from":       tok.RotatedFrom,
			"rotate_before_days": tok.RotateBeforeDays,
		}).Error
}

func (r *tokenRepo) ListDueRotation(ctx context.Context, before time.Time) ([]models.PersonalAccessToken, error) {
	var list []models.PersonalAccessToken
	err := r.db.WithContext(ctx).
		Where("rotate_before_days > 0").
		Where("expires_at IS NOT NULL AND expires_at <= ?", before).
		Where("rotated_at IS NULL").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (r *tokenRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.PersonalAccessToken{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (r *tokenRepo) UpdateLastUsed(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&models.PersonalAccessToken{}).
		Where("id = ?", id).
		Update("last_used_at", &at).Error
}

func (r *tokenRepo) AddToBlacklist(ctx context.Context, entry *models.TokenBlacklist) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now()
	}
	
	return r.db.WithContext(ctx).
		Where(models.TokenBlacklist{JTI: entry.JTI}).
		Assign(*entry).
		FirstOrCreate(entry).Error
}

func (r *tokenRepo) BlacklistJTI(ctx context.Context, jti, reason string) error {
	return r.AddToBlacklist(ctx, &models.TokenBlacklist{
		JTI:    jti,
		Reason: reason,
	})
}

func (r *tokenRepo) IsBlacklisted(ctx context.Context, jti string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.TokenBlacklist{}).
		Where("jti = ?", jti).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}