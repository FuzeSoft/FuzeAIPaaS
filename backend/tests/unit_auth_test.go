package tests

import (
	"testing"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
)

func TestTokenSignAndValidate(t *testing.T) {
	mgr := auth.NewManager()

	t.Run("sign and validate a valid token", func(t *testing.T) {
		claims := &auth.Claims{
			UserID:   "user-001",
			Username: "testuser",
			Role:     models.RoleDeveloper,
			TenantID: "tenant-001",
		}
		token, err := mgr.Sign(claims)
		if err != nil {
			t.Fatalf("Sign failed: %v", err)
		}
		if token == "" {
			t.Fatal("expected non-empty token")
		}

		decoded, err := mgr.Validate(token)
		if err != nil {
			t.Fatalf("Validate failed: %v", err)
		}
		if decoded.UserID != "user-001" {
			t.Errorf("expected user-001, got %s", decoded.UserID)
		}
		if decoded.Username != "testuser" {
			t.Errorf("expected testuser, got %s", decoded.Username)
		}
		if decoded.Role != models.RoleDeveloper {
			t.Errorf("expected developer, got %s", decoded.Role)
		}
	})

	t.Run("validate invalid token format", func(t *testing.T) {
		_, err := mgr.Validate("invalid-token")
		if err == nil {
			t.Error("expected error for invalid token")
		}
	})

	t.Run("validate token with wrong signature", func(t *testing.T) {
		claims := &auth.Claims{UserID: "user-001", Username: "testuser"}
		token, _ := mgr.Sign(claims)
		
		tampered := token[:len(token)-1] + "X"
		_, err := mgr.Validate(tampered)
		if err == nil {
			t.Error("expected error for tampered token")
		}
	})
}

func TestPasswordHashing(t *testing.T) {
	t.Run("hash and verify password", func(t *testing.T) {
		hash, err := auth.HashPassword("mypassword")
		if err != nil {
			t.Fatalf("HashPassword failed: %v", err)
		}
		if hash == "" {
			t.Fatal("expected non-empty hash")
		}
		if hash == "mypassword" {
			t.Error("hash should not equal plaintext")
		}
	})

	t.Run("different passwords produce different hashes", func(t *testing.T) {
		hash1, _ := auth.HashPassword("password1")
		hash2, _ := auth.HashPassword("password2")
		if hash1 == hash2 {
			t.Error("different passwords should produce different hashes")
		}
	})
}

func TestRoleValidation(t *testing.T) {
	t.Run("valid roles pass validation", func(t *testing.T) {
		roles := models.AllRoles()
		for _, r := range roles {
			if !models.ValidRole(r) {
				t.Errorf("role %s should be valid", r)
			}
		}
	})

	t.Run("invalid role fails validation", func(t *testing.T) {
		if models.ValidRole("invalid_role") {
			t.Error("invalid_role should not be valid")
		}
	})
}

func TestSyntheticAdmin(t *testing.T) {
	claims := auth.SyntheticAdmin()
	if claims.UserID != "system" {
		t.Errorf("expected system, got %s", claims.UserID)
	}
	if claims.Username != "system-admin" {
		t.Errorf("expected system-admin, got %s", claims.Username)
	}
	if claims.Role != models.RolePlatformAdmin {
		t.Errorf("expected platform_admin, got %s", claims.Role)
	}
	if claims.TenantID != "default" {
		t.Errorf("expected default, got %s", claims.TenantID)
	}
}