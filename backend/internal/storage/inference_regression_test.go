package storage

import (
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestUpdateInferenceRuntimeStatusPreservesSpec(t *testing.T) {
	s := newEntStorage(t)

	svc := &models.InferenceService{
		Name: "race-svc", TenantID: "default", ClusterID: "cluster-001",
		GPUs: 1, TargetReplicas: 2, Status: models.InferenceStatusPending,
	}
	if err := s.CreateInferenceService(svc); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := s.db.Model(&models.InferenceService{}).
		Where("id = ?", svc.ID).
		Update("target_replicas", 9).Error; err != nil {
		t.Fatalf("simulate concurrent patch: %v", err)
	}

	stale := &models.InferenceService{
		ID: svc.ID, Status: models.InferenceStatusReady,
		ReadyReplicas: 2, KServeName: "race-svc", URL: "http://race-svc",
		
	}
	if err := s.UpdateInferenceRuntimeStatus(stale); err != nil {
		t.Fatalf("update runtime status: %v", err)
	}

	got, err := s.GetInferenceService(svc.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TargetReplicas != 9 {
		t.Fatalf("FIX 4 regression: target_replicas clobbered to %d, expected 9 (concurrent PATCH must survive reconcile)", got.TargetReplicas)
	}
	if got.Status != models.InferenceStatusReady {
		t.Fatalf("expected status ready, got %s", got.Status)
	}
	if got.ReadyReplicas != 2 || got.KServeName != "race-svc" {
		t.Fatalf("observed state not persisted: replicas=%d kserve=%s", got.ReadyReplicas, got.KServeName)
	}
}

func TestUpdateInferenceRuntimeStatusClearsFailureReason(t *testing.T) {
	s := newEntStorage(t)
	svc := &models.InferenceService{
		Name: "recover-svc", TenantID: "default", ClusterID: "cluster-001",
		GPUs: 1, Status: models.InferenceStatusFailed, FailureReason: "crashloop",
	}
	if err := s.CreateInferenceService(svc); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := s.UpdateInferenceRuntimeStatus(&models.InferenceService{
		ID: svc.ID, Status: models.InferenceStatusReady, ReadyReplicas: 1,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := s.GetInferenceService(svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FailureReason != "" {
		t.Fatalf("failure_reason should be cleared on recovery, got %q", got.FailureReason)
	}
	if got.Status != models.InferenceStatusReady {
		t.Fatalf("expected ready, got %s", got.Status)
	}
}

func TestDeleteInferenceServiceAndReleaseQuotaAtomic(t *testing.T) {
	s := newEntStorage(t)

	svc := &models.InferenceService{
		Name: "del-svc", TenantID: "default", ClusterID: "cluster-001",
		GPUs: 3, Status: models.InferenceStatusReady,
	}
	if err := s.CreateInferenceService(svc); err != nil {
		t.Fatalf("create: %v", err)
	}
	
	memGB := svc.GPUs * 40
	if err := s.CheckAndReserve("default", svc.GPUs, memGB, 1); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	before, _ := s.GetQuota("default")

	if err := s.DeleteInferenceServiceAndReleaseQuota(svc.ID, "default", svc.GPUs, memGB); err != nil {
		t.Fatalf("delete+release: %v", err)
	}

	if _, err := s.GetInferenceService(svc.ID); err == nil {
		t.Fatal("record should be deleted")
	}
	
	after, _ := s.GetQuota("default")
	if after.GPUUsed != before.GPUUsed-svc.GPUs {
		t.Fatalf("quota not released: before=%d after=%d", before.GPUUsed, after.GPUUsed)
	}
	if after.JobUsed != before.JobUsed-1 {
		t.Fatalf("job slot not released: before=%d after=%d", before.JobUsed, after.JobUsed)
	}
}

func BenchmarkUpdateInferenceRuntimeStatus(b *testing.B) {
	s := newEntStorage(b)
	svc := &models.InferenceService{Name: "bench-rt", TenantID: "default", ClusterID: "c1", GPUs: 1}
	if err := s.CreateInferenceService(svc); err != nil {
		b.Fatal(err)
	}
	upd := &models.InferenceService{ID: svc.ID, Status: models.InferenceStatusReady, ReadyReplicas: 2, KServeName: "bench-rt"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.UpdateInferenceRuntimeStatus(upd); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUpdateInferenceServiceFullSave(b *testing.B) {
	s := newEntStorage(b)
	svc := &models.InferenceService{Name: "bench-full", TenantID: "default", ClusterID: "c1", GPUs: 1}
	if err := s.CreateInferenceService(svc); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Status = models.InferenceStatusReady
		svc.ReadyReplicas = 2
		if err := s.UpdateInferenceService(svc); err != nil {
			b.Fatal(err)
		}
	}
}