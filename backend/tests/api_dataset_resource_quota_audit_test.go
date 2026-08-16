package tests

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestDatasetFullLifecycle(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	w := env.DoAuthJSON(http.MethodPost, "/api/v1/datasets", token, map[string]interface{}{
		"name":   "cifar10-dataset",
		"url":    "s3://bucket/cifar10",
		"format": "parquet",
	})
	AssertStatus(t, w, http.StatusCreated)
	var createResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &createResp)
	datasetID, _ := createResp["id"].(string)

	t.Run("get dataset", func(t *testing.T) {
	if datasetID == "" {
		t.Fatalf("dataset create failed: no id in response")
	}
		w := env.DoAuthGET(http.MethodGet, "/api/v1/datasets/"+datasetID, token)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("list datasets", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/datasets", token)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("delete dataset", func(t *testing.T) {
	if datasetID == "" {
		t.Fatalf("dataset create failed: no id in response")
	}
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/datasets/"+datasetID, token, nil)
		AssertStatus(t, w, http.StatusNoContent)
	})
}

func TestDatasetNotFoundScenarios(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	t.Run("get non-existent dataset returns 404", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/datasets/nonexistent", token)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("delete non-existent dataset returns 404", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/datasets/nonexistent", token, nil)
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestDatasetValidation(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	t.Run("create dataset without name returns 400", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/datasets", token, map[string]interface{}{
			"url": "s3://bucket/test",
		})
		
		AssertStatus(t, w, http.StatusBadRequest)
	})
}

func TestResourceEndpoints(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	t.Run("list all resources", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/resources", token)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("get non-existent resource returns 404", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/resources/nonexistent", token)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("create resource", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/resources", token, map[string]interface{}{
			"name":       "test-gpu-node",
			"type":       "GPU",
			"vendor":     "nvidia",
			"model":      "A100",
			"total_gpus": 8,
		})
		AssertStatus(t, w, http.StatusCreated)
	})
}

func TestQuotaEndpoints(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")
	env.EnsureDefaultQuota(t, 64, 1024, 20)

	t.Run("list all quotas", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/quotas", token)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("get default tenant quota", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/quotas/default", token)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("update quota", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPut, "/api/v1/quotas/default", token, map[string]interface{}{
			"gpu_quota":       256,
			"memory_quota_gb": 4096,
			"job_quota":       50,
		})
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("get non-existent tenant quota returns 404", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/quotas/nonexistent", token)
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestAuditAndMetricsEndpoints(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	t.Run("list audit logs", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/audit", token)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("list audit logs with action filter", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/audit?action=login", token)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("list audit logs with pagination", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/audit?page=1&page_size=10", token)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("get platform metrics as JSON", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/metrics", token)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("prometheus metrics endpoint", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/metrics", token)
		AssertStatus(t, w, http.StatusOK)
	})
}