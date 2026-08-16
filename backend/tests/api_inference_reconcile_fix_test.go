package tests

import (
	"context"
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestReconcileScaleToZeroConverges(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)
	svc := createV2(t, env, "scale-to-zero-svc")

	w := env.DoJSON(http.MethodPatch, "/api/v1/inference-services/"+svc.ID, map[string]interface{}{
		"spec": map[string]interface{}{"target_replicas": 2},
	})
	AssertStatus(t, w, http.StatusOK)
	env.Scheduler.ReconcileInference(context.Background())
	if got := getSvc(t, env, svc.ID); got.Status.ReadyReplicas != 2 {
		t.Fatalf("precondition: expected 2 replicas, got %d", got.Status.ReadyReplicas)
	}

	w = env.DoJSON(http.MethodPatch, "/api/v1/inference-services/"+svc.ID, map[string]interface{}{
		"spec": map[string]interface{}{"target_replicas": 0},
	})
	AssertStatus(t, w, http.StatusOK)
	env.Scheduler.ReconcileInference(context.Background())

	got := getSvc(t, env, svc.ID)
	if got.Status.ReadyReplicas != 0 {
		t.Fatalf("FIX 3 regression: scale-to-zero did not converge, ready_replicas=%d", got.Status.ReadyReplicas)
	}
}

func TestReconcilePreservesPatchedTargetReplicas(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)
	svc := createV2(t, env, "preserve-target-svc")

	w := env.DoJSON(http.MethodPatch, "/api/v1/inference-services/"+svc.ID, map[string]interface{}{
		"spec": map[string]interface{}{"target_replicas": 5},
	})
	AssertStatus(t, w, http.StatusOK)

	env.Scheduler.ReconcileInference(context.Background())
	env.Scheduler.ReconcileInference(context.Background())

	got := getSvc(t, env, svc.ID)
	if got.Spec.TargetReplicas != 5 {
		t.Fatalf("FIX 4 regression: target_replicas=%d, expected 5", got.Spec.TargetReplicas)
	}
	if got.Status.Phase != string(models.InferenceStatusReady) {
		t.Fatalf("expected ready after reconcile, got %s", got.Status.Phase)
	}
}

func TestDeleteInferenceServiceReleasesQuota(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	before, err := env.Store.GetQuota("default")
	if err != nil {
		t.Fatalf("get quota before: %v", err)
	}

	w := env.DoJSON(http.MethodPost, "/api/v1/inference-services", applySpec("del-quota-svc", map[string]interface{}{"gpus": 2}))
	AssertStatus(t, w, http.StatusCreated)
	id := ParseJSON[svcView](t, w).ID

	afterCreate, _ := env.Store.GetQuota("default")
	if afterCreate.GPUUsed != before.GPUUsed+2 {
		t.Fatalf("expected quota +2 after create, got %d (before=%d)", afterCreate.GPUUsed, before.GPUUsed)
	}

	w = env.DoJSON(http.MethodDelete, "/api/v1/inference-services/"+id, nil)
	AssertStatus(t, w, http.StatusNoContent)

	afterDelete, _ := env.Store.GetQuota("default")
	if afterDelete.GPUUsed != before.GPUUsed {
		t.Fatalf("FIX 5 regression: quota not released after delete, used=%d (expected %d)", afterDelete.GPUUsed, before.GPUUsed)
	}
}

func getSvc(t *testing.T, env *TestEnv, id string) svcView {
	t.Helper()
	w := env.DoGET(http.MethodGet, "/api/v1/inference-services/"+id)
	AssertStatus(t, w, http.StatusOK)
	return ParseJSON[svcView](t, w)
}