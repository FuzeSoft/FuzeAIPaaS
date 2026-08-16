package token

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type Service struct {
	auth  *auth.Manager
	repos ports.TokenRepository
}

func NewService(auth *auth.Manager, repos ports.TokenRepository) *Service {
	return &Service{auth: auth, repos: repos}
}

type IssuedToken struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Token     string     `json:"token"` 
	Prefix    string     `json:"prefix"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func (s *Service) Issue(ctx context.Context, user *models.User, name string, ttl time.Duration) (*IssuedToken, error) {
	if name == "" {
		return nil, errors.New("token name is required")
	}
	if ttl <= 0 {
		ttl = 365 * 24 * time.Hour
	}

	now := time.Now()
	expires := now.Add(ttl)
	jti := generateJTI()
	rec := &models.PersonalAccessToken{
		ID:        generateID(),
		Name:      name,
		Prefix:    prefixOf(jti),
		UserID:    user.ID,
		TenantID:  user.TenantID,
		JTI:       jti,
		CreatedAt: now,
		ExpiresAt: &expires,
	}
	if err := s.repos.Create(ctx, rec); err != nil {
		return nil, err
	}

	jwt, err := s.auth.Sign(&auth.Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		TenantID: user.TenantID,
		JTI:      jti,
		Scope:    auth.ScopePAT,
		IssuedAt: now.Unix(),
		Expires:  expires.Unix(),
	})
	if err != nil {
		return nil, err
	}
	return &IssuedToken{
		ID:        rec.ID,
		Name:      rec.Name,
		Token:     jwt,
		Prefix:    rec.Prefix,
		ExpiresAt: rec.ExpiresAt,
	}, nil
}

func (s *Service) List(ctx context.Context, userID string) ([]models.PersonalAccessToken, error) {
	return s.repos.ListByUser(ctx, userID)
}

func (s *Service) Get(ctx context.Context, id string) (*models.PersonalAccessToken, error) {
	return s.repos.Get(ctx, id)
}

func (s *Service) Revoke(ctx context.Context, id string) error {
	rec, err := s.repos.Get(ctx, id)
	if err != nil {
		return err
	}
	
	if rec.JTI != "" && (rec.ExpiresAt == nil || rec.ExpiresAt.After(time.Now())) {
		exp := int64(0)
		if rec.ExpiresAt != nil {
			exp = rec.ExpiresAt.Unix()
		}
		if err := s.repos.AddToBlacklist(ctx, &models.TokenBlacklist{
			JTI:       rec.JTI,
			Subject:   rec.UserID,
			ExpiresAt: exp,
			Reason:    "revoked",
		}); err != nil {
			return err
		}
	}
	return s.repos.Delete(ctx, id)
}

func (s *Service) Rotate(ctx context.Context, id, userID string) (*IssuedToken, error) {
	old, err := s.repos.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if old.UserID != userID {
		return nil, ports.ErrNotFound
	}

	ttl := 365 * 24 * time.Hour
	if old.ExpiresAt != nil && old.ExpiresAt.After(old.CreatedAt) {
		ttl = old.ExpiresAt.Sub(old.CreatedAt)
	}

	now := time.Now()
	expires := now.Add(ttl)
	jti := generateJTI()
	newRec := &models.PersonalAccessToken{
		ID:               generateID(),
		Name:             old.Name,
		Prefix:           prefixOf(jti),
		UserID:           old.UserID,
		TenantID:         old.TenantID,
		JTI:              jti,
		CreatedAt:        now,
		ExpiresAt:        &expires,
		RotateBeforeDays: old.RotateBeforeDays,
		RotatedFrom:      old.ID,
	}
	if err := s.repos.Create(ctx, newRec); err != nil {
		return nil, err
	}

	if old.JTI != "" {
		exp := int64(0)
		if old.ExpiresAt != nil {
			exp = old.ExpiresAt.Unix()
		}
		if err := s.repos.AddToBlacklist(ctx, &models.TokenBlacklist{
			JTI:       old.JTI,
			Subject:   old.UserID,
			ExpiresAt: exp,
			Reason:    "rotated",
		}); err != nil {
			return nil, err
		}
	}

	rotatedAt := now
	if err := s.repos.Update(ctx, &models.PersonalAccessToken{
		ID:        old.ID,
		RotatedAt: &rotatedAt,
	}); err != nil {
		return nil, err
	}

	jwt, err := s.auth.Sign(&auth.Claims{
		UserID:   old.UserID,
		Username: "", 
		Role:     "",
		TenantID: old.TenantID,
		JTI:      jti,
		Scope:    auth.ScopePAT,
		IssuedAt: now.Unix(),
		Expires:  expires.Unix(),
	})
	if err != nil {
		return nil, err
	}
	return &IssuedToken{
		ID:        newRec.ID,
		Name:      newRec.Name,
		Token:     jwt,
		Prefix:    newRec.Prefix,
		ExpiresAt: newRec.ExpiresAt,
	}, nil
}

func (s *Service) ListDueRotation(ctx context.Context, before time.Time) ([]models.PersonalAccessToken, error) {
	return s.repos.ListDueRotation(ctx, before)
}

func (s *Service) Touch(ctx context.Context, jti string) error {
	rec, err := s.repos.GetByJTI(ctx, jti)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil
		}
		return err
	}
	return s.repos.UpdateLastUsed(ctx, rec.ID, time.Now())
}

func prefixOf(jti string) string {
	if len(jti) < 8 {
		return jti
	}
	return jti[:8]
}

func generateID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return time.Now().Format("20060102150405") + "-tok-fallback"
	}
	return time.Now().Format("20060102150405") + "-" + hex.EncodeToString(b)[:8]
}

func generateJTI() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b)
}