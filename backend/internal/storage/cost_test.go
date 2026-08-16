package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/models"
	"gorm.io/gorm"
)

func costTestStore(t *testing.T) *Storage {
	t.Helper()
	
	dir := t.TempDir()
	db, err := NewSQLiteDBAt(filepath.Join(dir, "cost.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return &Storage{db: db}
}

func mustSetQuota(t *testing.T, db *gorm.DB, tenant string, limit float64) {
	t.Helper()
	q := models.LLMTokenQuota{TenantID: tenant, LimitCost: limit, UsedCost: 0}
	if err := db.Create(&q).Error; err != nil {
		
		_ = err
	}
}

func TestRecordGPUCostAccumulatesUsedCost(t *testing.T) {
	store := costTestStore(t)
	repo := NewCostRepository(store.db)
	ctx := context.Background()
	const tenant = "t-gpu-1"
	mustSetQuota(t, store.db, tenant, 1000)

	rec := &models.GPUUsageRecord{
		TenantID: tenant, JobID: "job-1", GPUType: "A100",
		GPUCount: 4, Hours: 2, Cost: 20, Currency: "CNY", Status: "completed",
	}
	if err := repo.RecordGPUCost(ctx, rec); err != nil {
		t.Fatalf("RecordGPUCost: %v", err)
	}

	q, err := repo.GetQuota(ctx, tenant)
	if err != nil {
		t.Fatalf("GetQuota: %v", err)
	}
	if q.UsedCost != 20 {
		t.Fatalf("UsedCost = %v, want 20", q.UsedCost)
	}

	if err := repo.RecordGPUCost(ctx, &models.GPUUsageRecord{
		TenantID: tenant, JobID: "job-1", GPUType: "A100",
		GPUCount: 4, Hours: 2, Cost: 20, Currency: "CNY", Status: "completed",
	}); err != nil {
		t.Fatalf("RecordGPUCost idempotent: %v", err)
	}
	q2, _ := repo.GetQuota(ctx, tenant)
	if q2.UsedCost != 20 {
		t.Fatalf("after idempotent retry UsedCost = %v, want 20", q2.UsedCost)
	}

	hours, cost, err := repo.SumGPUUsage(ctx, tenant, 0, 0)
	if err != nil {
		t.Fatalf("SumGPUUsage: %v", err)
	}
	if hours != 2 || cost != 20 {
		t.Fatalf("SumGPUUsage = (%v, %v), want (2, 20)", hours, cost)
	}
}

func TestRecordGPUCostNoQuotaIsNoopOnLimit(t *testing.T) {
	store := costTestStore(t)
	repo := NewCostRepository(store.db)
	ctx := context.Background()
	rec := &models.GPUUsageRecord{
		TenantID: "t-no-limit", JobID: "job-x", GPUType: "",
		GPUCount: 2, Hours: 1, Cost: 5, Currency: "CNY", Status: "completed",
	}
	if err := repo.RecordGPUCost(ctx, rec); err != nil {
		t.Fatalf("RecordGPUCost without quota should not fail: %v", err)
	}
	hours, cost, _ := repo.SumGPUUsage(ctx, "t-no-limit", 0, 0)
	if hours != 1 || cost != 5 {
		t.Fatalf("SumGPUUsage = (%v, %v), want (1, 5)", hours, cost)
	}
}

func TestRecordGPUCostPersistsBeyondLimit(t *testing.T) {
	store := costTestStore(t)
	repo := NewCostRepository(store.db)
	ctx := context.Background()
	const tenant = "t-over-limit"

	if err := store.db.Create(&models.LLMTokenQuota{TenantID: tenant, LimitCost: 10, UsedCost: 9}).Error; err != nil {
		t.Fatalf("seed quota: %v", err)
	}
	rec := &models.GPUUsageRecord{
		TenantID: tenant, JobID: "job-over", GPUType: "A100",
		GPUCount: 2, Hours: 1, Cost: 5, Currency: "CNY", Status: "completed",
	}
	if err := repo.RecordGPUCost(ctx, rec); err != nil {
		t.Fatalf("RecordGPUCost beyond limit: %v", err)
	}

	hours, cost, err := repo.SumGPUUsage(ctx, tenant, 0, 0)
	if err != nil {
		t.Fatalf("SumGPUUsage: %v", err)
	}
	if hours != 1 || cost != 5 {
		t.Fatalf("record lost on over-limit: SumGPUUsage=(%v,%v) want (1,5)", hours, cost)
	}
	
	q, _ := repo.GetQuota(ctx, tenant)
	if q.UsedCost != 14 {
		t.Fatalf("UsedCost = %v, want 14 (still accrued beyond limit)", q.UsedCost)
	}
}

func TestRecordGPUCostConcurrentSameJobID(t *testing.T) {
	store := costTestStore(t)
	repo := NewCostRepository(store.db)
	ctx := context.Background()
	const tenant = "t-conc"
	mustSetQuota(t, store.db, tenant, 1_000_000)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = repo.RecordGPUCost(ctx, &models.GPUUsageRecord{
				TenantID: tenant, JobID: "job-conc", GPUType: "A100",
				GPUCount: 1, Hours: 1, Cost: 1, Currency: "CNY", Status: "completed",
			})
		}()
	}
	wg.Wait()

	hours, cost, err := repo.SumGPUUsage(ctx, tenant, 0, 0)
	if err != nil {
		t.Fatalf("SumGPUUsage: %v", err)
	}
	if hours != 1 || cost != 1 {
		t.Fatalf("concurrent same job: want exactly one record (1,1), got (%v,%v)", hours, cost)
	}
	q, _ := repo.GetQuota(ctx, tenant)
	if q.UsedCost != 1 {
		t.Fatalf("concurrent same job UsedCost = %v, want 1", q.UsedCost)
	}
}

func TestRecordGPUCostConcurrentDistinctJobs(t *testing.T) {
	store := costTestStore(t)
	repo := NewCostRepository(store.db)
	ctx := context.Background()
	const tenant = "t-conc-distinct"
	mustSetQuota(t, store.db, tenant, 1_000_000)

	const n = 30
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_ = repo.RecordGPUCost(ctx, &models.GPUUsageRecord{
				TenantID: tenant, JobID: fmt.Sprintf("job-%d", i), GPUType: "A100",
				GPUCount: 1, Hours: 1, Cost: 1, Currency: "CNY", Status: "completed",
			})
		}(i)
	}
	wg.Wait()

	hours, cost, err := repo.SumGPUUsage(ctx, tenant, 0, 0)
	if err != nil {
		t.Fatalf("SumGPUUsage: %v", err)
	}
	if hours != float64(n) || cost != float64(n) {
		t.Fatalf("concurrent distinct: want (%v,%v), got (%v,%v)", n, n, hours, cost)
	}
}

func TestPriceBookGPUCost(t *testing.T) {
	book := llm.NewPriceBook()
	if err := book.SetGPUPrice("A100", 2.5, "CNY"); err != nil {
		t.Fatalf("SetGPUPrice: %v", err)
	}
	
	got := book.GPUCost("A100", 4, 2.0)
	if got != 20 {
		t.Fatalf("GPUCost = %v, want 20", got)
	}
	
	_ = book.SetGPUPrice("", 1.0, "CNY")
	if got := book.GPUCost("UNKNOWN", 1, 3.0); got != 3.0 {
		t.Fatalf("GPUCost fallback = %v, want 3.0", got)
	}
	
	empty := llm.NewPriceBook()
	if got := empty.GPUCost("A100", 4, 2.0); got != 0 {
		t.Fatalf("GPUCost empty = %v, want 0", got)
	}
}