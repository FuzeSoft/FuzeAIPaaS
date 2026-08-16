package tests

import (
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestGetInferenceServices(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("list all inference services", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/inference-services")
		AssertStatus(t, w, http.StatusOK)
		svcs := ParseJSON[[]svcView](t, w)
		if len(svcs) < 2 {
			t.Errorf("expected at least 2 seeded services, got %d", len(svcs))
		}
	})
}

func TestGetInferenceService(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("get existing inference service", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/inference-services/isvc-001")
		AssertStatus(t, w, http.StatusOK)
		svc := ParseJSON[svcView](t, w)
		if svc.Spec.Name != "llama2-7b-chat" {
			t.Errorf("expected llama2-7b-chat, got %s", svc.Spec.Name)
		}
	})

	t.Run("get non-existent service returns 404", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/inference-services/nonexistent")
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestCreateInferenceService(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	t.Run("create inference service", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/inference-services", map[string]interface{}{
			"spec": map[string]interface{}{
				"name":      "test-inference",
				"framework": "pytorch",
				"image":     "nvcr.io/nvidia/tritonserver:23.10",
				"gpus":      1,
				"memory":    "16Gi",
				"cpu":       "2",
			},
		})
		AssertStatus(t, w, http.StatusCreated)
		svc := ParseJSON[svcView](t, w)
		if svc.Spec.Name != "test-inference" {
			t.Errorf("expected test-inference, got %s", svc.Spec.Name)
		}
		if svc.Spec.ClusterID != "cluster-001" {
			t.Errorf("expected default cluster-001, got %s", svc.Spec.ClusterID)
		}
	})

	t.Run("create inference service with defaults", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/inference-services", map[string]interface{}{
			"spec": map[string]interface{}{"name": "minimal-inference"},
		})
		AssertStatus(t, w, http.StatusCreated)
		svc := ParseJSON[svcView](t, w)
		if svc.Spec.MaxReplicas != 1 {
			t.Errorf("expected default MaxReplicas=1, got %d", svc.Spec.MaxReplicas)
		}
		if svc.Spec.Framework != string(models.FrameworkCustom) {
			t.Errorf("expected default framework custom, got %s", svc.Spec.Framework)
		}
	})

	t.Run("create without spec.name returns 400", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/inference-services", map[string]interface{}{
			"spec": map[string]interface{}{"gpus": 1},
		})
		AssertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("create inference service exceeding quota returns 409", func(t *testing.T) {
		env.EnsureDefaultQuota(t, 0, 0, 0)
		w := env.DoJSON(http.MethodPost, "/api/v1/inference-services",
			applySpec("oversized-inference", map[string]interface{}{"gpus": 4}))
		AssertStatus(t, w, http.StatusConflict)
	})
}

func TestDeleteInferenceService(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	t.Run("delete non-existent service returns 404", func(t *testing.T) {
		w := env.DoJSON(http.MethodDelete, "/api/v1/inference-services/nonexistent", nil)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("delete existing inference service", func(t *testing.T) {
		svc := createV2(t, env, "to-delete-inference")
		w := env.DoJSON(http.MethodDelete, "/api/v1/inference-services/"+svc.ID, nil)
		AssertStatus(t, w, http.StatusNoContent)
	})
}

func TestApplyInferenceServiceQuotaRecalc(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	w := env.DoJSON(http.MethodPut, "/api/v1/inference-services",
		applySpec("quota-apply", map[string]interface{}{"gpus": 1}))
	AssertStatus(t, w, http.StatusCreated)

	t.Run("scaling gpus up within quota succeeds", func(t *testing.T) {
		w := env.DoJSON(http.MethodPut, "/api/v1/inference-services",
			applySpec("quota-apply", map[string]interface{}{"gpus": 2}))
		AssertStatus(t, w, http.StatusOK)
		if got := ParseJSON[svcView](t, w); got.Spec.GPUs != 2 {
			t.Fatalf("expected gpus=2 after apply, got %d", got.Spec.GPUs)
		}
	})

	t.Run("apply exceeding quota returns 409 and keeps old reservation", func(t *testing.T) {
		w := env.DoJSON(http.MethodPut, "/api/v1/inference-services",
			applySpec("quota-apply", map[string]interface{}{"gpus": 999}))
		AssertStatus(t, w, http.StatusConflict)

		got := env.DoGET(http.MethodGet, "/api/v1/inference-services")
		AssertStatus(t, got, http.StatusOK)
		if n := countByName(t, got, "quota-apply"); n != 1 {
			t.Fatalf("failed apply must not create duplicates, got %d", n)
		}
	})
}