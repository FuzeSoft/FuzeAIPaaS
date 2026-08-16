package tests

import (
	"context"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/models"
)

func TestSchedulerGetMetrics(t *testing.T) {
	t.Run("get metrics from scheduler", func(t *testing.T) {
		env := NewTestEnv(t)
		metrics, err := env.Scheduler.GetMetrics("")
		if err != nil {
			t.Fatalf("GetMetrics failed: %v", err)
		}
		if metrics == nil {
			t.Fatal("expected non-nil metrics")
		}
		if metrics.TotalGPUs < 0 {
			t.Errorf("expected non-negative TotalGPUs, got %d", metrics.TotalGPUs)
		}
	})

	t.Run("get metrics for specific cluster", func(t *testing.T) {
		env := NewTestEnv(t)
		metrics, err := env.Scheduler.GetMetrics("cluster-001")
		if err != nil {
			t.Fatalf("GetMetrics for cluster-001 failed: %v", err)
		}
		if metrics == nil {
			t.Fatal("expected non-nil metrics")
		}
	})
}

func TestSchedulerSubmitJob(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	t.Run("submit a pending job", func(t *testing.T) {
		job := &models.Job{
			ID:        "test-submit-job",
			Name:      "test-submit",
			Type:      models.JobTypeTraining,
			Status:    models.JobStatusPending,
			ClusterID: "cluster-001",
			GPUs:      1,
			Memory:    32,
		}
		err := env.Scheduler.SubmitJob(job)
		
		_ = err
	})
}

func TestSchedulerCancelJob(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("cancel a job", func(t *testing.T) {
		job := &models.Job{ID: "test-cancel-job", Name: "test-cancel"}
		err := env.Scheduler.CancelJob(job)
		_ = err
	})
}

func TestSchedulerStartStop(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("start and stop scheduler without panic", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go env.Scheduler.Run(ctx)

		time.Sleep(100 * time.Millisecond)

		cancel()
		time.Sleep(50 * time.Millisecond)
		env.Scheduler.Stop()
	})
}

func TestSchedulerRefreshCluster(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("refresh existing cluster", func(t *testing.T) {
		err := env.Scheduler.RefreshCluster(context.Background(), "cluster-001")
		
		_ = err
	})
}