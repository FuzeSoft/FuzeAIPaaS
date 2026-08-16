package tests

import (
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestListQuotas(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("list all quotas", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/quotas")
		AssertStatus(t, w, http.StatusOK)
		quotas := ParseJSON[[]models.Quota](t, w)
		if len(quotas) < 1 {
			t.Errorf("expected at least 1 quota, got %d", len(quotas))
		}
	})
}

func TestGetQuota(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("get quota for default tenant", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/quotas/default")
		AssertStatus(t, w, http.StatusOK)
		quota := ParseJSON[models.Quota](t, w)
		if quota.TenantID != "default" {
			t.Errorf("expected tenant default, got %s", quota.TenantID)
		}
	})

	t.Run("get quota for non-existent tenant returns 404", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/quotas/nonexistent")
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestUpdateQuota(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("update quota for existing tenant", func(t *testing.T) {
		w := env.DoJSON(http.MethodPut, "/api/v1/quotas/default", map[string]interface{}{
			"gpu_quota":        128,
			"memory_quota_gb": 2048,
			"job_quota":        20,
		})
		AssertStatus(t, w, http.StatusOK)
		quota := ParseJSON[models.Quota](t, w)
		if quota.GPUQuota != 128 {
			t.Errorf("expected GPU quota 128, got %d", quota.GPUQuota)
		}
	})

	t.Run("update quota for non-existent tenant returns 404", func(t *testing.T) {
		w := env.DoJSON(http.MethodPut, "/api/v1/quotas/nonexistent", map[string]interface{}{
			"gpu_quota": 10,
		})
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("update quota with type-mismatched body returns 400", func(t *testing.T) {
		w := env.DoJSON(http.MethodPut, "/api/v1/quotas/default", map[string]interface{}{
			"gpu_quota": "not-a-number",
		})
		AssertStatus(t, w, http.StatusBadRequest)
	})
}