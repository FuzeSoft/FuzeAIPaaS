package tests

import (
	"testing"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
)

type mockUserStore struct {
	users map[string]*models.User
}

func (m *mockUserStore) GetUserByUsername(username string) (*models.User, error) {
	if u, ok := m.users[username]; ok {
		return u, nil
	}
	return nil, nil
}

func (m *mockUserStore) GetUserByID(id string) (*models.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, nil
}

func (m *mockUserStore) UpdateUser(u *models.User) error {
	if u != nil {
		m.users[u.Username] = u
	}
	return nil
}

func (m *mockUserStore) UpdateUserMFARecovery(userID, recoveryEnc string) error {
	return nil
}

func (m *mockUserStore) RecordLoginFailure(username string, maxFails, lockSec int) (*models.User, bool, error) {
	return m.users[username], false, nil
}

func (m *mockUserStore) ClearLoginFailures(username string) error {
	return nil
}

func TestAuthLogin(t *testing.T) {
	mgr := auth.NewManager()

	hashed, _ := auth.HashPassword("testpass123")

	store := &mockUserStore{
		users: map[string]*models.User{
			"testuser": {
				ID:       "u-1",
				Username: "testuser",
				Password: hashed,
				Role:     models.RoleDeveloper,
				TenantID: "default",
				Enabled:  true,
			},
			"disabled-user": {
				ID:       "u-2",
				Username: "disabled-user",
				Password: hashed,
				Role:     models.RoleDeveloper,
				TenantID: "default",
				Enabled:  false,
			},
			"sso-user": {
				ID:       "u-3",
				Username: "sso-user",
				Password: "", 
				Role:     models.RoleDeveloper,
				TenantID: "default",
				Enabled:  true,
			},
		},
	}

	t.Run("login with valid credentials", func(t *testing.T) {
		token, user, err := mgr.Login(store, "testuser", "testpass123")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if token == "" {
			t.Error("expected non-empty token")
		}
		if user == nil || user.Username != "testuser" {
			t.Error("expected user testuser")
		}
	})

	t.Run("login with invalid password", func(t *testing.T) {
		_, _, err := mgr.Login(store, "testuser", "wrongpass")
		if err == nil {
			t.Error("expected error for invalid password")
		}
	})

	t.Run("login with non-existent user", func(t *testing.T) {
		_, _, err := mgr.Login(store, "nonexistent", "pass")
		if err == nil {
			t.Error("expected error for non-existent user")
		}
	})

	t.Run("login with disabled user", func(t *testing.T) {
		_, _, err := mgr.Login(store, "disabled-user", "testpass123")
		if err == nil {
			t.Error("expected error for disabled user")
		}
	})

	t.Run("login with SSO user (no password)", func(t *testing.T) {
		_, _, err := mgr.Login(store, "sso-user", "pass")
		if err == nil {
			t.Error("expected error for SSO user without password")
		}
	})

	t.Run("login and validate token roundtrip", func(t *testing.T) {
		token, user, err := mgr.Login(store, "testuser", "testpass123")
		if err != nil {
			t.Fatal(err)
		}
		claims, err := mgr.Validate(token)
		if err != nil {
			t.Fatalf("expected no error validating token, got %v", err)
		}
		if claims.Username != user.Username {
			t.Errorf("expected username %s, got %s", user.Username, claims.Username)
		}
		if claims.Role != user.Role {
			t.Errorf("expected role %s, got %s", user.Role, claims.Role)
		}
		if claims.TenantID != user.TenantID {
			t.Errorf("expected tenant %s, got %s", user.TenantID, claims.TenantID)
		}
	})
}

func TestAuthSignWithCustomExpiry(t *testing.T) {
	mgr := auth.NewManager()

	t.Run("preserves custom issued at", func(t *testing.T) {
		token, err := mgr.Sign(&auth.Claims{
			UserID:   "u1",
			Username: "custom-user",
			Role:     models.RolePlatformAdmin,
			TenantID: "default",
			IssuedAt: 1000000,
		})
		if err != nil {
			t.Fatal(err)
		}
		claims, err := mgr.Validate(token)
		if err != nil {
			t.Fatal(err)
		}
		if claims.IssuedAt != 1000000 {
			t.Errorf("expected issued at 1000000, got %d", claims.IssuedAt)
		}
	})

	t.Run("preserves custom expiry", func(t *testing.T) {
		token, err := mgr.Sign(&auth.Claims{
			UserID:   "u1",
			Username: "custom-expiry-user",
			Role:     models.RolePlatformAdmin,
			TenantID: "default",
			Expires:  9999999999,
		})
		if err != nil {
			t.Fatal(err)
		}
		claims, err := mgr.Validate(token)
		if err != nil {
			t.Fatal(err)
		}
		if claims.Expires != 9999999999 {
			t.Errorf("expected expires 9999999999, got %d", claims.Expires)
		}
	})
}