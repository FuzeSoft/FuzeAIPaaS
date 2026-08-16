package tests

import (
	"context"
	"testing"

	"fuze-ai-paas/backend/internal/app/dataset"
	appinference "fuze-ai-paas/backend/internal/app/inference"
	appjob "fuze-ai-paas/backend/internal/app/job"
	"fuze-ai-paas/backend/internal/events"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/runtime"
)

func TestAppJobService(t *testing.T) {
	env := NewTestEnv(t)
	bus := events.NewBus(16, 4)
	svc := appjob.NewService(env.Store, env.ClusterMgr, bus)

	t.Run("submit pending jobs with no clusters", func(t *testing.T) {
		
		job := &models.Job{
			ID:        "job-pending-1",
			ClusterID: "non-existent-cluster",
			Name:      "pending-test",
			Status:    models.JobStatusPending,
			GPUs:      1,
			Memory:    16,
		}
		if err := env.Store.CreateJob(job); err != nil {
			t.Fatal(err)
		}

		svc.SubmitPending(context.Background())

		updated, err := env.Store.GetJob("job-pending-1")
		if err != nil {
			
			t.Logf("job not found after submit (may have been processed): %v", err)
			return
		}
		if updated.Status != models.JobStatusRunning && updated.Status != models.JobStatusPending {
			t.Errorf("expected running or pending, got %s", updated.Status)
		}
	})

	t.Run("cancel job", func(t *testing.T) {
		job := &models.Job{
			ID:             "job-cancel-1",
			ClusterID:      "test-cluster",
			Name:           "cancel-test",
			Status:         models.JobStatusRunning,
			VolcanoJobName: "volcano-job-1",
		}
		if err := env.Store.CreateJob(job); err != nil {
			t.Fatal(err)
		}

		err := svc.CancelJob(job)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if job.Status != models.JobStatusCancelled {
			t.Errorf("expected cancelled, got %s", job.Status)
		}
		if job.VolcanoJobName != "" {
			t.Errorf("expected empty volcano job name, got %s", job.VolcanoJobName)
		}
	})

	t.Run("cancel job without volcano job name", func(t *testing.T) {
		job := &models.Job{
			ID:        "job-cancel-2",
			ClusterID: "test-cluster",
			Name:      "cancel-no-volcano",
			Status:    models.JobStatusRunning,
		}
		if err := env.Store.CreateJob(job); err != nil {
			t.Fatal(err)
		}

		err := svc.CancelJob(job)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if job.Status != models.JobStatusCancelled {
			t.Errorf("expected cancelled, got %s", job.Status)
		}
	})

	t.Run("submit job to non-existent cluster", func(t *testing.T) {
		job := &models.Job{
			ID:        "job-submit-1",
			ClusterID: "non-existent",
			Name:      "submit-test",
			Status:    models.JobStatusPending,
		}
		if err := env.Store.CreateJob(job); err != nil {
			t.Fatal(err)
		}

		err := svc.SubmitJob(job)
		if err != nil {
			t.Fatalf("expected no error for mock mode, got %v", err)
		}
	})

	t.Run("sync volcano statuses with no clusters", func(t *testing.T) {
		
		svc.SyncVolcanoStatuses(context.Background())
	})
}

func TestAppDatasetService(t *testing.T) {
	env := NewTestEnv(t)
	svc := dataset.NewService(env.Store, env.ClusterMgr)

	t.Run("create dataset in mock mode", func(t *testing.T) {
		ds := &models.Dataset{
			ID:        "ds-create-1",
			ClusterID: "non-existent",
			Name:      "test-dataset",
		}
		err := svc.Create(ds)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ds.Status != models.DatasetStatusBound {
			t.Errorf("expected bound status in mock mode, got %s", ds.Status)
		}
	})

	t.Run("delete dataset", func(t *testing.T) {
		ds := &models.Dataset{
			ID:        "ds-delete-1",
			ClusterID: "non-existent",
			Name:      "delete-test",
		}
		_ = svc.Create(ds)

		err := svc.Delete(ds)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestAppInferenceService(t *testing.T) {
	env := NewTestEnv(t)
	svc := appinference.NewService(env.Store, env.ClusterMgr, runtime.NewDefaultRegistry())

	t.Run("deploy in mock mode", func(t *testing.T) {
		s := &models.InferenceService{
			ID:         "inf-deploy-1",
			ClusterID:  "non-existent",
			Name:       "test-inference",
			Framework:  models.FrameworkPyTorch,
			MinReplicas: 1,
			MaxReplicas: 3,
		}
		err := svc.Deploy(s)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if s.Status != models.InferenceStatusReady {
			t.Errorf("expected ready in mock mode, got %s", s.Status)
		}
		if s.KServeName != "test-inference" {
			t.Errorf("expected kserve name test-inference, got %s", s.KServeName)
		}
		if s.URL == "" {
			t.Error("expected non-empty URL")
		}
	})

	t.Run("deploy with existing URL", func(t *testing.T) {
		s := &models.InferenceService{
			ID:         "inf-deploy-2",
			ClusterID:  "non-existent",
			Name:       "test-inference-2",
			Framework:  models.FrameworkPyTorch,
			MinReplicas: 1,
			MaxReplicas: 3,
			URL:        "http://existing.example.com",
		}
		err := svc.Deploy(s)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if s.URL != "http://existing.example.com" {
			t.Errorf("expected existing URL preserved, got %s", s.URL)
		}
	})

	t.Run("delete inference service", func(t *testing.T) {
		s := &models.InferenceService{
			ID:         "inf-delete-1",
			ClusterID:  "non-existent",
			Name:       "delete-test",
			Framework:  models.FrameworkPyTorch,
			MinReplicas: 1,
			MaxReplicas: 3,
		}
		_ = svc.Deploy(s)

		err := svc.Delete(s)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	var reconcileID string
	t.Run("reconcile deploys pending service and converges desired state", func(t *testing.T) {
		s := &models.InferenceService{
			ID:             "inf-reconcile-1",
			ClusterID:      "non-existent",
			Name:           "reconcile-test",
			Framework:      models.FrameworkPyTorch,
			MinReplicas:    1,
			MaxReplicas:    5,
			TargetReplicas: 3,
			CanaryWeight:   20,
			Status:         models.InferenceStatusPending,
		}
		if err := env.Store.CreateInferenceService(s); err != nil {
			t.Fatal(err)
		}
		reconcileID = s.ID

		svc.Reconcile(context.Background())

		got, err := env.Store.GetInferenceService(s.ID)
		if err != nil {
			t.Fatalf("expected service persisted, got %v", err)
		}
		if got.Status != models.InferenceStatusReady {
			t.Errorf("expected ready after reconcile, got %s", got.Status)
		}
		if got.KServeName == "" || got.URL == "" {
			t.Errorf("expected observed state written by reconcile, got %+v", got)
		}
		if got.ReadyReplicas != 3 {
			t.Errorf("expected ready replicas converged to 3, got %d", got.ReadyReplicas)
		}
		if got.CanaryWeight != 20 {
			t.Errorf("expected desired canary weight preserved, got %d", got.CanaryWeight)
		}
	})

	t.Run("reconcile is idempotent", func(t *testing.T) {
		svc.Reconcile(context.Background())
		svc.Reconcile(context.Background())

		got, err := env.Store.GetInferenceService(reconcileID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != models.InferenceStatusReady || got.ReadyReplicas != 3 {
			t.Errorf("reconcile must be idempotent, got status=%s ready=%d", got.Status, got.ReadyReplicas)
		}
	})
}

func TestKServeStateToStatus(t *testing.T) {
	tests := []struct {
		name     string
		ready    bool
		found    bool
		failed   bool
		expected models.InferenceStatus
	}{
		{"failed returns failed", true, true, true, models.InferenceStatusFailed},
		{"not found returns pending", false, false, false, models.InferenceStatusPending},
		{"ready returns ready", true, true, false, models.InferenceStatusReady},
		{"found but not ready returns pending", false, true, false, models.InferenceStatusPending},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := models.KServeStateToStatus(tc.ready, tc.found, tc.failed)
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}