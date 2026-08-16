package storage

import (
	"context"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/models"
)

func TestTokenBlacklistAddAndCheck(t *testing.T) {
	db := openSQLite(t)
	s := NewStorage(db)
	r := s.Token()
	ctx := context.Background()
	jti := "revoked-jti-123"

	listed, err := r.IsBlacklisted(ctx, jti)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if listed {
		t.Fatal("fresh jti must not be blacklisted")
	}

	entry := &models.TokenBlacklist{
		JTI:       jti,
		Subject:   "u1",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		Reason:    "user logout",
	}
	if err := r.AddToBlacklist(ctx, entry); err != nil {
		t.Fatalf("add: %v", err)
	}
	
	if err := r.AddToBlacklist(ctx, entry); err != nil {
		t.Fatalf("idempotent add: %v", err)
	}

	listed, err = r.IsBlacklisted(ctx, jti)
	if err != nil {
		t.Fatalf("check after add: %v", err)
	}
	if !listed {
		t.Fatal("jti must be blacklisted after add")
	}
}