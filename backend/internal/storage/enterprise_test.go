package storage

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func newEntStorage(t testing.TB) *Storage {
	t.Helper()
	f, err := os.CreateTemp("", "fuze-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	_ = f.Close()
	_ = os.Remove(path)
	db, err := NewSQLiteDBAt(path)
	if err != nil {
		t.Fatalf("NewSQLiteDBAt: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	return NewStorage(db)
}

func TestQuotaReserveAndRelease(t *testing.T) {
	s := newEntStorage(t)

	if err := s.CheckAndReserve("default", 10, 100, 1); err != nil {
		t.Fatalf("reserve failed: %v", err)
	}
	q, err := s.GetQuota("default")
	if err != nil {
		t.Fatal(err)
	}
	if q.GPUUsed != 10 {
		t.Fatalf("expected GPUUsed=10, got %d", q.GPUUsed)
	}

	if err := s.CheckAndReserve("default", 100, 0, 0); err == nil {
		t.Fatal("expected quota-exceeded error")
	}

	if err := s.Release("default", 10, 100, 1); err != nil {
		t.Fatalf("release failed: %v", err)
	}
	q, _ = s.GetQuota("default")
	if q.GPUUsed != 0 {
		t.Fatalf("expected GPUUsed=0 after release, got %d", q.GPUUsed)
	}
}

func TestAdjustReservationAtomic(t *testing.T) {
	s := newEntStorage(t)

	if err := s.CheckAndReserve("default", 10, 100, 1); err != nil {
		t.Fatalf("reserve failed: %v", err)
	}

	if err := s.AdjustReservation("default", 10, 100, 20, 200); err != nil {
		t.Fatalf("adjust failed: %v", err)
	}
	q, _ := s.GetQuota("default")
	if q.GPUUsed != 20 || q.MemoryUsedGB != 200 {
		t.Fatalf("expected GPUUsed=20 mem=200, got GPU=%d mem=%d", q.GPUUsed, q.MemoryUsedGB)
	}

	if err := s.AdjustReservation("default", 20, 200, 100, 0); err == nil {
		t.Fatal("expected quota-exceeded error on adjust")
	}
	q, _ = s.GetQuota("default")
	if q.GPUUsed != 20 || q.MemoryUsedGB != 200 {
		t.Fatalf("expected unchanged GPUUsed=20 mem=200 after failed adjust, got GPU=%d mem=%d", q.GPUUsed, q.MemoryUsedGB)
	}
}

func TestUpsertQuota(t *testing.T) {
	s := newEntStorage(t)
	if err := s.UpsertQuota(&models.Quota{TenantID: "t-x", GPUQuota: 4}); err != nil {
		t.Fatal(err)
	}
	q, _ := s.GetQuota("t-x")
	if q.GPUQuota != 4 {
		t.Fatalf("expected 4, got %d", q.GPUQuota)
	}
	
	if err := s.UpsertQuota(&models.Quota{TenantID: "t-x", GPUQuota: 8}); err != nil {
		t.Fatal(err)
	}
	q, _ = s.GetQuota("t-x")
	if q.GPUQuota != 8 {
		t.Fatalf("expected updated 8, got %d", q.GPUQuota)
	}
}

func resetDefaultQuota(t *testing.T, s *Storage, gpuQuota int) {
	t.Helper()
	if err := s.UpsertQuota(&models.Quota{
		TenantID:      "default",
		GPUQuota:      gpuQuota,
		GPUUsed:       0,
		JobQuota:      100,
		JobUsed:       0,
		MemoryQuotaGB: 100000,
		MemoryUsedGB:  0,
	}); err != nil {
		t.Fatalf("reset quota: %v", err)
	}
}

func TestQuotaReserveConcurrentNoOversell(t *testing.T) {
	s := newEntStorage(t)
	resetDefaultQuota(t, s, 64)

	const attempts = 128
	var okCount, failCount int32
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			err := s.CheckAndReserve("default", 1, 0, 0)
			if err != nil {
				atomic.AddInt32(&failCount, 1)
			} else {
				atomic.AddInt32(&okCount, 1)
			}
		}()
	}
	wg.Wait()

	if okCount != 64 {
		t.Fatalf("expected exactly 64 successful reserves, got %d", okCount)
	}
	if failCount != attempts-64 {
		t.Fatalf("expected %d failures, got %d", attempts-64, failCount)
	}
	q, _ := s.GetQuota("default")
	if q.GPUUsed != 64 {
		t.Fatalf("data inconsistency: GPUUsed=%d, want 64 (oversell?)", q.GPUUsed)
	}
}

func TestQuotaReserveReleaseConcurrentConsistent(t *testing.T) {
	s := newEntStorage(t)
	resetDefaultQuota(t, s, 64)

	const workers = 64
	var wg sync.WaitGroup
	var releaseErrs int32
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if err := s.CheckAndReserve("default", 1, 0, 0); err != nil {
				return
			}
			
			if err := s.Release("default", 1, 0, 0); err != nil {
				atomic.AddInt32(&releaseErrs, 1)
				t.Errorf("release failed after successful reserve (配额泄漏): %v", err)
			}
		}()
	}
	wg.Wait()

	if releaseErrs > 0 {
		t.Fatalf("%d 次回收失败，配额已泄漏", releaseErrs)
	}
	q, _ := s.GetQuota("default")
	if q.GPUUsed != 0 {
		t.Fatalf("expected GPUUsed=0 after concurrent reserve/release, got %d", q.GPUUsed)
	}
}

func TestAuditRecordAndList(t *testing.T) {
	s := newEntStorage(t)
	if err := s.Record(&models.AuditLog{
		Actor: "alice", Action: models.ActionCreate, ResourceType: models.ResModel, ResourceID: "m1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Record(&models.AuditLog{
		Actor: "alice", Action: models.ActionDelete, ResourceType: models.ResModel, ResourceID: "m1",
	}); err != nil {
		t.Fatal(err)
	}
	logs, err := s.ListAudit(AuditQuery{Actor: "alice", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) < 2 {
		t.Fatalf("expected >=2 audit logs, got %d", len(logs))
	}
	onlyCreate, err := s.ListAudit(AuditQuery{Actor: "alice", Action: models.ActionCreate})
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyCreate) != 1 {
		t.Fatalf("expected 1 create log, got %d", len(onlyCreate))
	}
}