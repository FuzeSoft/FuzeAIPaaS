package tests

import (
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestInferenceCreateIsImmediatelyQueryable(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	w := env.DoJSON(http.MethodPost, "/api/v1/inference-services", applySpec("regression-inference", nil))
	AssertStatus(t, w, http.StatusCreated)
	svc := ParseJSON[svcView](t, w)
	if svc.ID == "" {
		t.Fatal("expected non-empty service ID")
	}
	if svc.Status.Phase != string(models.InferenceStatusPending) {
		t.Fatalf("expected pending right after create, got %s", svc.Status.Phase)
	}

	got := env.DoGET(http.MethodGet, "/api/v1/inference-services/"+svc.ID)
	AssertStatus(t, got, http.StatusOK)
}

func TestCreateJobSubmitsAndStaysPending(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	t.Run("create job returns 201 with pending status", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs", map[string]interface{}{
			"name":    "regression-job",
			"image":   "pytorch:latest",
			"command": "python train.py",
			"gpus":    1,
			"memory":  32,
		})
		AssertStatus(t, w, http.StatusCreated)
		job := ParseJSON[models.Job](t, w)
		if job.Status != models.JobStatusPending {
			t.Errorf("expected job status pending, got %s", job.Status)
		}
	})
}