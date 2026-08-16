package token

import (
	"context"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"fuze-ai-paas/backend/internal/storage"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newRotateTestDB(t *testing.T) (*gorm.DB, ports.TokenRepository) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.PersonalAccessToken{}, &models.TokenBlacklist{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db, storage.NewTokenRepository(db)
}

func TestRotateOldTokenRevokedNewUsable(t *testing.T) {
	ctx := context.Background()
	_, repo := newRotateTestDB(t)
	authMgr := auth.NewManager()

	expires := time.Now().Add(30 * 24 * time.Hour)
	old := &models.PersonalAccessToken{
		ID:        "tok-old",
		Name:      "ci",
		Prefix:    "fuze_old1",
		UserID:    "u-1",
		TenantID:  "t-1",
		JTI:       "jti-old",
		ExpiresAt: &expires,
		CreatedAt: time.Now(),
	}
	if err := repo.Create(ctx, old); err != nil {
		t.Fatalf("create old: %v", err)
	}

	svc := NewService(authMgr, repo)
	issued, err := svc.Rotate(ctx, "tok-old", "u-1")
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if issued.Token == "" {
		t.Fatal("new token must be returned")
	}
	if issued.ID == old.ID {
		t.Fatal("new token must have a fresh ID")
	}

	gotOld, err := repo.Get(ctx, "tok-old")
	if err != nil {
		t.Fatalf("get old: %v", err)
	}
	if gotOld.RotatedAt == nil {
		t.Fatal("old record must have RotatedAt set")
	}

	newRec, err := repo.Get(ctx, issued.ID)
	if err != nil {
		t.Fatalf("get new: %v", err)
	}
	if newRec.RotatedFrom != "tok-old" {
		t.Fatalf("new RotatedFrom=%q, want tok-old", newRec.RotatedFrom)
	}

	authMgr.SetRevokedCheck(func(jti string) bool {
		listed, _ := repo.IsBlacklisted(ctx, jti)
		return listed
	})
	if _, err := authMgr.Validate(issued.Token); err != nil {
		t.Fatalf("new token must validate: %v", err)
	}
	
	oldJWT, _ := authMgr.Sign(&auth.Claims{UserID: "u-1", TenantID: "t-1", JTI: "jti-old", Expires: expires.Unix()})
	if _, err := authMgr.Validate(oldJWT); err == nil {
		t.Fatal("old token (blacklisted jti) must fail validation")
	}
}

func TestRotateWrongOwnerReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	_, repo := newRotateTestDB(t)
	authMgr := auth.NewManager()

	old := &models.PersonalAccessToken{
		ID:        "tok-old",
		Name:      "ci",
		Prefix:    "fuze_old2",
		UserID:    "u-1",
		TenantID:  "t-1",
		JTI:       "jti-old2",
		CreatedAt: time.Now(),
	}
	if err := repo.Create(ctx, old); err != nil {
		t.Fatalf("create old: %v", err)
	}
	svc := NewService(authMgr, repo)
	
	if _, err := svc.Rotate(ctx, "tok-old", "u-other"); err != ports.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListDueRotationHitsBeforeExpiry(t *testing.T) {
	ctx := context.Background()
	_, repo := newRotateTestDB(t)
	authMgr := auth.NewManager()
	svc := NewService(authMgr, repo)

	soon := time.Now().Add(3 * 24 * time.Hour) 
	due := &models.PersonalAccessToken{
		ID:              "tok-due",
		Name:            "ci",
		Prefix:          "fuze_due1",
		UserID:          "u-1",
		TenantID:        "t-1",
		JTI:             "jti-due",
		ExpiresAt:       &soon,
		RotateBeforeDays: 7, 
		CreatedAt:       time.Now(),
	}
	if err := repo.Create(ctx, due); err != nil {
		t.Fatalf("create: %v", err)
	}
	
	noRemind := &models.PersonalAccessToken{
		ID:        "tok-norem",
		Name:      "manual",
		Prefix:    "fuze_nr01",
		UserID:    "u-1",
		TenantID:  "t-1",
		JTI:       "jti-nr",
		ExpiresAt: &soon,
		CreatedAt: time.Now(),
	}
	if err := repo.Create(ctx, noRemind); err != nil {
		t.Fatalf("create: %v", err)
	}

	list, err := svc.ListDueRotation(ctx, time.Now().Add(5*24*time.Hour))
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(list) != 1 || list[0].ID != "tok-due" {
		t.Fatalf("expected only tok-due, got %+v", list)
	}
}