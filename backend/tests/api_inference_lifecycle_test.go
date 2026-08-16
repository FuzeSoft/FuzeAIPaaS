package tests

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestInferenceServiceLifecycle(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")
	env.EnsureDefaultQuota(t, 128, 2048, 50)

	w := env.DoAuthJSON(http.MethodPost, "/api/v1/inference-services", token, map[string]interface{}{
		"spec": map[string]interface{}{
			"name":         "llm-inference",
			"framework":    "pytorch",
			"storage_uri":  "s3://bucket/model",
			"gpus":         1,
			"min_replicas": 1,
			"max_replicas": 3,
		},
	})
	AssertStatus(t, w, http.StatusCreated)
	svcID := ParseJSON[svcView](t, w).ID
	if svcID == "" {
		t.Fatal("failed to create inference service: no id in response")
	}

	t.Run("get inference service", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/inference-services/"+svcID, token)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("declare replicas via patch", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPatch, "/api/v1/inference-services/"+svcID, token, map[string]interface{}{
			"spec": map[string]interface{}{"target_replicas": 3},
		})
		AssertStatus(t, w, http.StatusOK)
		if got := ParseJSON[svcView](t, w); got.Spec.TargetReplicas != 3 {
			t.Fatalf("expected target_replicas=3, got %d", got.Spec.TargetReplicas)
		}
	})

	t.Run("declare canary weight via patch", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPatch, "/api/v1/inference-services/"+svcID, token, map[string]interface{}{
			"spec": map[string]interface{}{"canary_weight": 50},
		})
		AssertStatus(t, w, http.StatusOK)
		if got := ParseJSON[svcView](t, w); got.Spec.CanaryWeight != 50 {
			t.Fatalf("expected canary_weight=50, got %d", got.Spec.CanaryWeight)
		}
	})

	t.Run("delete inference service", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/inference-services/"+svcID, token, nil)
		AssertStatus(t, w, http.StatusNoContent)
	})
}

func TestInferenceServicesList(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")
	env.EnsureDefaultQuota(t, 128, 2048, 50)

	services := []map[string]interface{}{
		applySpec("svc-a", map[string]interface{}{"framework": "pytorch", "gpus": 1, "max_replicas": 2}),
		applySpec("svc-b", map[string]interface{}{"framework": "pytorch", "gpus": 2, "max_replicas": 3}),
	}
	var createdIDs []string
	for _, s := range services {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/inference-services", token, s)
		if w.Code >= 200 && w.Code < 300 {
			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)
			if id, ok := resp["id"].(string); ok {
				createdIDs = append(createdIDs, id)
			}
		}
	}
	if len(createdIDs) != 2 {
		t.Fatalf("expected 2 services created, got %d", len(createdIDs))
	}

	t.Run("list all inference services", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/inference-services", token)
		AssertStatus(t, w, http.StatusOK)
		if list := ParseJSON[[]svcView](t, w); len(list) < 2 {
			t.Errorf("expected at least 2 inference services, got %d", len(list))
		}
	})

	for _, id := range createdIDs {
		env.DoAuthJSON(http.MethodDelete, "/api/v1/inference-services/"+id, token, nil)
	}
}

func TestInferenceNotFoundScenarios(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	t.Run("get non-existent service returns 404", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/inference-services/nonexistent", token)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("patch non-existent service returns 404", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPatch, "/api/v1/inference-services/nonexistent", token, map[string]interface{}{
			"spec": map[string]interface{}{"target_replicas": 1},
		})
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("delete non-existent service returns 404", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/inference-services/nonexistent", token, nil)
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestInferenceValidation(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")
	env.EnsureDefaultQuota(t, 128, 2048, 50)

	t.Run("create inference with minimal spec", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/inference-services", token, map[string]interface{}{
			"spec": map[string]interface{}{
				"name":        "minimal-svc",
				"framework":   "pytorch",
				"storage_uri": "s3://bucket/model",
			},
		})
		AssertStatus(t, w, http.StatusCreated)
	})

	t.Run("create inference with excessive GPU returns 409", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/inference-services", token,
			applySpec("exceed-gpu", map[string]interface{}{"gpus": 10000}))
		AssertStatus(t, w, http.StatusConflict)
	})

	t.Run("create with invalid replica range returns 400", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/inference-services", token,
			applySpec("bad-replicas", map[string]interface{}{"min_replicas": 5, "max_replicas": 2}))
		AssertStatus(t, w, http.StatusBadRequest)
	})
}