package scheduler

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/domain/gpu"
	"fuze-ai-paas/backend/internal/models"
)

func waitForGoroutines() {
	runtime.GC()
	time.Sleep(150 * time.Millisecond)
	runtime.GC()
}

func newLifecycleScheduler() (*Scheduler, *mockJobRepo, *mockClusterRepo) {
	jobRepo := &mockJobRepo{
		jobsByID:      map[string]*models.Job{},
		infServices:   map[string]*models.InferenceService{},
		resourcesByCl: map[string][]models.Resource{},
		pending:       []models.Job{{ID: "p1", Status: models.JobStatusPending}},
	}
	clusterRepo := newMockClusterRepo()
	clusterRepo.clusters = map[string]*models.Cluster{"c1": {ID: "c1", Enabled: true}}
	
	reg := newMockRegistry()
	reg.clients["c1"] = &mockClusterClient{
		enabled: true,
		devices: []gpu.GPUDevice{},
	}
	return NewScheduler(
		Repos{Cluster: clusterRepo, Job: jobRepo, Inference: jobRepo, Dataset: jobRepo, Metrics: jobRepo},
		reg,
		newMockRuntimeRegistry(),
		nil,
	), jobRepo, clusterRepo
}

func TestRunStopNoGoroutineLeak(t *testing.T) {
	
	waitForGoroutines()
	base := runtime.NumGoroutine()

	s, _, _ := newLifecycleScheduler()
	ctx, cancel := context.WithCancel(context.Background())
	s.Run(ctx)
	time.Sleep(60 * time.Millisecond) 
	cancel()
	s.Stop()
	waitForGoroutines()

	after := runtime.NumGoroutine()
	if after > base+8 {
		t.Errorf("goroutine 泄漏：Run/Stop 后净增 %d (base=%d, after=%d)，预期 ≤8", after-base, base, after)
	}
}

func TestRunIdempotent(t *testing.T) {
	waitForGoroutines()
	base := runtime.NumGoroutine()

	s, _, _ := newLifecycleScheduler()
	ctx, cancel := context.WithCancel(context.Background())
	s.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	
	s.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()
	s.Stop()
	waitForGoroutines()

	after := runtime.NumGoroutine()
	if after > base+8 {
		t.Errorf("重复 Run 疑似启动了额外 goroutine：净增 %d (base=%d, after=%d)", after-base, base, after)
	}
}

func TestStopIdempotent(t *testing.T) {
	s, _, _ := newLifecycleScheduler()
	ctx, cancel := context.WithCancel(context.Background())
	s.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()
	s.Stop()
	s.Stop()
	s.Stop()
}

func TestRunDrivesSubcycles(t *testing.T) {
	s, jobRepo, clusterRepo := newLifecycleScheduler()
	ctx, cancel := context.WithCancel(context.Background())
	s.Run(ctx)
	time.Sleep(60 * time.Millisecond)
	cancel()
	s.Stop()

	if atomic.LoadInt64(&jobRepo.submitCalls) == 0 {
		t.Error("Run 未驱动 submit 子循环（GetPendingJobs 未被调用）")
	}
	if atomic.LoadInt64(&jobRepo.reconcileCalls) == 0 {
		t.Error("Run 未驱动 reconcile 子循环（GetInferenceServices 未被调用）")
	}
	if atomic.LoadInt64(&clusterRepo.discoverCalls) == 0 {
		t.Error("Run 未驱动 discover 子循环（UpsertClusterResources 未被调用）")
	}
}

func TestRunConcurrentWithDirectCalls(t *testing.T) {
	s, _, _ := newLifecycleScheduler()
	ctx, cancel := context.WithCancel(context.Background())
	s.Run(ctx)
	defer func() { cancel(); s.Stop() }()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				s.ReconcileInference(ctx)            
				_ = s.RefreshCluster(ctx, "c1")      
				_, _ = s.GetMetrics("c1")            
			}
		}()
	}
	wg.Wait()
}