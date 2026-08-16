package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestLoginSuccess(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("login with valid admin credentials", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/auth/login", map[string]string{
			"username": "admin",
			"password": "admin123",
		})
		AssertStatus(t, w, http.StatusOK)
		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if _, ok := resp["token"]; !ok {
			t.Error("expected token in response")
		}
		user, ok := resp["user"].(map[string]interface{})
		if !ok {
			t.Fatal("expected user in response")
		}
		if user["username"] != "admin" {
			t.Errorf("expected username admin, got %v", user["username"])
		}
	})

	t.Run("login with invalid password", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/auth/login", map[string]string{
			"username": "admin",
			"password": "wrongpass",
		})
		AssertStatus(t, w, http.StatusUnauthorized)
	})

	t.Run("login with non-existent user", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/auth/login", map[string]string{
			"username": "nonexistent",
			"password": "password",
		})
		AssertStatus(t, w, http.StatusUnauthorized)
	})

	t.Run("login with empty body returns 401", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/auth/login", map[string]string{})
		AssertStatus(t, w, http.StatusUnauthorized)
	})
}

func TestMeEndpoint(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("me without authentication in dev mode returns system admin", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/auth/me")
		AssertStatus(t, w, http.StatusOK)
		body := ParseJSON[map[string]interface{}](t, w)
		if body["username"] != "system-admin" {
			t.Errorf("expected system-admin, got %v", body["username"])
		}
	})

	t.Run("me with auth enabled but no token returns 401", func(t *testing.T) {
		envAuth := NewTestEnvWithAuth(t)
		w := envAuth.DoGET(http.MethodGet, "/api/v1/auth/me")
		AssertStatus(t, w, http.StatusUnauthorized)
	})

	t.Run("me with valid token returns user info", func(t *testing.T) {
		envAuth := NewTestEnvWithAuth(t)
		token := envAuth.LoginAndGetToken(t, "admin", "admin123")
		w := envAuth.DoAuthGET(http.MethodGet, "/api/v1/auth/me", token)
		AssertStatus(t, w, http.StatusOK)
		body := ParseJSON[map[string]interface{}](t, w)
		if body["username"] != "admin" {
			t.Errorf("expected admin, got %v", body["username"])
		}
	})
}

func TestSSOProviders(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("list SSO providers returns empty when not configured", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/auth/sso")
		AssertStatus(t, w, http.StatusOK)
		body := ParseJSON[map[string]interface{}](t, w)
		providers, ok := body["providers"].([]interface{})
		if !ok {
			t.Fatal("expected providers array")
		}
		if len(providers) != 0 {
			t.Errorf("expected 0 providers, got %d", len(providers))
		}
	})
}

func TestRoleAuthorization(t *testing.T) {
	env := NewTestEnvWithAuth(t)

	t.Run("admin can access tenant management", func(t *testing.T) {
		token := env.LoginAndGetToken(t, "admin", "admin123")
		w := env.DoAuthGET(http.MethodGet, "/api/v1/tenants", token)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("admin can update quota", func(t *testing.T) {
		token := env.LoginAndGetToken(t, "admin", "admin123")
		w := env.DoAuthJSON(http.MethodPut, "/api/v1/quotas/default", token, map[string]interface{}{
			"gpu_quota":       128,
			"memory_quota_gb": 2048,
			"job_quota":       20,
		})
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("admin can delete tenant except default", func(t *testing.T) {
		token := env.LoginAndGetToken(t, "admin", "admin123")
		
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/tenants", token, map[string]string{
			"name": "test-tenant",
		})
		AssertStatus(t, w, http.StatusCreated)
		body := ParseJSON[map[string]interface{}](t, w)
		tenantID := body["id"].(string)

		w = env.DoAuthJSON(http.MethodDelete, "/api/v1/tenants/"+tenantID, token, nil)
		AssertStatus(t, w, http.StatusNoContent)
	})

	t.Run("cannot delete default tenant", func(t *testing.T) {
		token := env.LoginAndGetToken(t, "admin", "admin123")
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/tenants/default", token, nil)
		AssertStatus(t, w, http.StatusBadRequest)
	})
}

func TestRegisterTenantWithRoleRestriction(t *testing.T) {
	env := NewTestEnvWithAuth(t)

	err := env.Store.CreateUser(&models.User{
		ID:        "u-dev",
		Username:  "devuser",
		Password:  mustHash("devpass"),
		Role:      models.RoleDeveloper,
		TenantID:  "default",
		Enabled:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("developer cannot create tenant", func(t *testing.T) {
		token := env.LoginAndGetToken(t, "devuser", "devpass")
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/tenants", token, map[string]string{
			"name": "should-fail",
		})
		AssertStatus(t, w, http.StatusForbidden)
	})
}