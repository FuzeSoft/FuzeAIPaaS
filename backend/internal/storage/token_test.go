package storage

import (
	"context"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"gorm.io/gorm"
)

func newTokenTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testDB(t)
	return db
}

func TestTokenRepoCRUD(t *testing.T) {
	ctx := context.Background()
	repo := NewTokenRepository(newTokenTestDB(t))

	rec := &models.PersonalAccessToken{
		ID:       "tok-1",
		Name:     "ci",
		Prefix:   "abcdef12",
		UserID:   "u-1",
		TenantID: "t-1",
		JTI:      "abcdef1234567890",
	}
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.Get(ctx, "tok-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "ci" || got.Prefix != "abcdef12" {
		t.Fatalf("unexpected record: %+v", got)
	}

	byJ, err := repo.GetByJTI(ctx, "abcdef1234567890")
	if err != nil || byJ.ID != "tok-1" {
		t.Fatalf("get by jti: %v %+v", err, byJ)
	}

	list, err := repo.ListByUser(ctx, "u-1")
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}

	now := time.Now()
	if err := repo.UpdateLastUsed(ctx, "tok-1", now); err != nil {
		t.Fatalf("update last used: %v", err)
	}
	got, _ = repo.Get(ctx, "tok-1")
	if got.LastUsedAt == nil {
		t.Fatalf("expected last_used_at set")
	}

	if err := repo.Delete(ctx, "tok-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = repo.Get(ctx, "tok-1")
	if err != ports.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTokenRepoGetMissing(t *testing.T) {
	repo := NewTokenRepository(newTokenTestDB(t))
	_, err := repo.Get(context.Background(), "nope")
	if err != ports.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}