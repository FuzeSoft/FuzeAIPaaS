package tests

import (
	"context"
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestReconcileConvergesTargetReplicasInMockMode(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)
	svc := createV2(t, env, "reconcile-svc")

	w := env.DoJSON(http.MethodPatch, "/api/v1/inference-services/"+svc.ID, map[string]interface{}{
		"spec": map[string]interface{}{"target_replicas": 4},
	})
	AssertStatus(t, w, http.StatusOK)

	env.Scheduler.ReconcileInference(context.Background())

	w2 := env.DoGET(http.MethodGet, "/api/v1/inference-services/"+svc.ID)
	AssertStatus(t, w2, http.StatusOK)
	got := ParseJSON[svcView](t, w2)
	if got.Status.ReadyReplicas != 4 {
		t.Fatalf("expected reconcile to converge ready_replicas to 4, got %d", got.Status.ReadyReplicas)
	}
	if got.Status.Phase != string(models.InferenceStatusReady) {
		t.Fatalf("expected status ready after reconcile, got %s", got.Status.Phase)
	}
}

func TestReconcileIsIdempotent(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)
	svc := createV2(t, env, "reconcile-idem-svc")

	w := env.DoJSON(http.MethodPatch, "/api/v1/inference-services/"+svc.ID, map[string]interface{}{
		"spec": map[string]interface{}{"target_replicas": 2},
	})
	AssertStatus(t, w, http.StatusOK)

	env.Scheduler.ReconcileInference(context.Background())
	env.Scheduler.ReconcileInference(context.Background())
	env.Scheduler.ReconcileInference(context.Background())

	got, err := env.Store.GetInferenceService(svc.ID)
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if got.ReadyReplicas != 2 || got.TargetReplicas != 2 {
		t.Fatalf("expected stable converged state target=2 ready=2, got target=%d ready=%d",
			got.TargetReplicas, got.ReadyReplicas)
	}
}

func TestReconcileRetriesUndeployedService(t *testing.T) {
	env := NewTestEnv(t)

	s := &models.InferenceService{
		Name:      "undeployed-svc",
		ClusterID: "cluster-001",
	}
	if err := env.Store.CreateInferenceService(s); err != nil {
		t.Fatalf("seed undeployed service: %v", err)
	}
	if s.KServeName != "" {
		t.Fatalf("precondition: KServeName should be empty, got %q", s.KServeName)
	}

	env.Scheduler.ReconcileInference(context.Background())

	got, err := env.Store.GetInferenceService(s.ID)
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if got.KServeName == "" {
		t.Fatal("expected reconcile to retry deploy and set KServeName")
	}
	if got.Status != models.InferenceStatusReady {
		t.Fatalf("expected status ready after mock deploy retry, got %s", got.Status)
	}
	if got.URL == "" {
		t.Fatal("expected URL to be set after deploy retry")
	}
}