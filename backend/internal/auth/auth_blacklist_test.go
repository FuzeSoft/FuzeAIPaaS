package auth

import (
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/models"
)

func TestValidateRevokedTokenFails(t *testing.T) {
	m := NewManager()
	revoked := map[string]bool{}
	m.SetRevokedCheck(func(jti string) bool { return revoked[jti] })

	tok, err := m.Sign(&Claims{
		UserID:   "u1",
		Username: "alice",
		Role:     models.RoleDeveloper,
		TenantID: "t1",
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	c, err := m.Validate(tok)
	if err != nil {
		t.Fatalf("valid before revoke: %v", err)
	}
	if c.JTI == "" {
		t.Fatal("issued token must carry a jti")
	}

	revoked[c.JTI] = true
	if _, err := m.Validate(tok); err != ErrTokenRevoked {
		t.Fatalf("revoked token must fail with ErrTokenRevoked, got %v", err)
	}
}

func TestValidateNoRevokedCheckStillWorks(t *testing.T) {
	
	m := NewManager()
	tok, _ := m.Sign(&Claims{UserID: "u1", Role: models.RoleDeveloper, TenantID: "t1"})
	if _, err := m.Validate(tok); err != nil {
		t.Fatalf("valid token without revoked check must pass: %v", err)
	}
}

func TestValidateExpiredTokenFailsBeforeRevokeCheck(t *testing.T) {
	m := NewManager()
	revoked := map[string]bool{}
	m.SetRevokedCheck(func(jti string) bool { return revoked[jti] })
	tok, _ := m.Sign(&Claims{
		UserID:   "u1",
		Role:     models.RoleDeveloper,
		TenantID: "t1",
		Expires:  time.Now().Add(-time.Hour).Unix(),
	})
	if _, err := m.Validate(tok); err == nil {
		t.Fatal("expired token must fail")
	}
}