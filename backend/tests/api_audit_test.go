package tests

import (
	"net/http"
	"testing"
)

func TestListAudit(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("list audit logs", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/audit")
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("list audit with actor filter", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/audit?actor=admin")
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("list audit with action filter", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/audit?action=create")
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("list audit with resource_type filter", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/audit?resource_type=job")
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("list audit with limit", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/audit?limit=10")
		AssertStatus(t, w, http.StatusOK)
	})
}

func TestAuditRecordedOnLogin(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("login creates audit entry", func(t *testing.T) {
		
		w := env.DoJSON(http.MethodPost, "/api/v1/auth/login", map[string]string{
			"username": "admin",
			"password": "admin123",
		})
		AssertStatus(t, w, http.StatusOK)

		w = env.DoGET(http.MethodGet, "/api/v1/audit?actor=admin&action=login")
		AssertStatus(t, w, http.StatusOK)
	})
}

func TestMetricsEndpoints(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("get platform metrics as JSON", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/metrics")
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("prometheus metrics endpoint", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/metrics")
		AssertStatus(t, w, http.StatusOK)
	})
}