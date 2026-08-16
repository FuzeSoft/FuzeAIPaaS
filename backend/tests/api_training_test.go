package tests

import (
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
)

func mustHash(pw string) string {
	h, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h)
}

func markTrainingJobRunning(t *testing.T, env *TestEnv, id string) {
	t.Helper()
	job, err := env.Store.GetJob(id)
	if err != nil {
		t.Fatalf("GetJob(%s): %v", id, err)
	}
	job.Status = models.JobStatusRunning
	if err := env.Store.UpdateJobStatus(job); err != nil {
		t.Fatalf("UpdateJobStatus(%s): %v", id, err)
	}
}

func TestGetTrainingJobs(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("list only returns training workloads", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/training-jobs")
		AssertStatus(t, w, http.StatusOK)
		jobs := ParseJSON[[]models.Job](t, w)
		if len(jobs) == 0 {
			t.Fatal("expected at least the seeded training job")
		}
		
		for _, j := range jobs {
			if j.Type != models.JobTypeTraining {
				t.Fatalf("non-training job leaked into list: %s (%s)", j.ID, j.Type)
			}
		}
	})
}

func TestGetTrainingJob(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("get existing training job", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/training-jobs/job-001")
		AssertStatus(t, w, http.StatusOK)
		job := ParseJSON[models.Job](t, w)
		if job.ID != "job-001" {
			t.Errorf("expected job-001, got %s", job.ID)
		}
	})

	t.Run("non-existent job returns 404", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/training-jobs/nonexistent")
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("inference workload is not reachable through the training API", func(t *testing.T) {
		
		w := env.DoGET(http.MethodGet, "/api/v1/training-jobs/job-002")
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestCreateTrainingJob(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	t.Run("create a new training job", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs", map[string]interface{}{
			"name":    "test-training-job",
			"image":   "pytorch:latest",
			"command": "python train.py",
			"gpus":    2,
			"memory":  64,
		})
		AssertStatus(t, w, http.StatusCreated)
		job := ParseJSON[models.Job](t, w)
		if job.Name != "test-training-job" {
			t.Errorf("expected name test-training-job, got %s", job.Name)
		}
		if job.Type != models.JobTypeTraining {
			t.Errorf("expected training type, got %s", job.Type)
		}
		if job.Status != models.JobStatusPending {
			t.Errorf("expected status pending, got %s", job.Status)
		}
	})

	t.Run("template fills in defaults without overriding user input", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs", map[string]interface{}{
			"name":        "templated-job",
			"template_id": "pytorch-ddp",
			"gpus":        1,
		})
		AssertStatus(t, w, http.StatusCreated)
		job := ParseJSON[models.Job](t, w)
		if job.Image == "" {
			t.Error("template should have supplied a default image")
		}
		if job.GPUs != 1 {
			t.Errorf("template must not override user-specified gpus, got %d", job.GPUs)
		}
	})

	t.Run("empty body returns 400", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs", map[string]interface{}{
			"invalid": "data",
		})
		AssertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("negative gpus returns 400", func(t *testing.T) {
		
		env.EnsureDefaultQuota(t, 64, 1024, 10)
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs", map[string]interface{}{
			"name":   "quota-underflow-job",
			"image":  "pytorch:latest",
			"gpus":   -8,
			"memory": 32,
		})
		AssertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("unknown framework returns 400", func(t *testing.T) {
		env.EnsureDefaultQuota(t, 64, 1024, 10)
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs", map[string]interface{}{
			"name":        "bad-framework-job",
			"image":       "pytorch:latest",
			"gpus":        1,
			"distributed": true,
			"replicas":    2,
			"framework":   "not-a-real-framework",
		})
		AssertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("min_available exceeding total replicas returns 400", func(t *testing.T) {
		
		env.EnsureDefaultQuota(t, 64, 1024, 10)
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs", map[string]interface{}{
			"name":          "unschedulable-gang-job",
			"image":         "pytorch:latest",
			"gpus":          1,
			"distributed":   true,
			"replicas":      2,
			"min_available": 99,
		})
		AssertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("distributed job reserves quota for every replica", func(t *testing.T) {
		
		env.EnsureDefaultQuota(t, 8, 1024, 10)
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs", map[string]interface{}{
			"name":        "distributed-job",
			"image":       "pytorch:latest",
			"gpus":        4,
			"memory":      32,
			"distributed": true,
			"replicas":    3,
		})
		AssertStatus(t, w, http.StatusConflict)
	})

	t.Run("exceeding quota returns 409", func(t *testing.T) {
		env.EnsureDefaultQuota(t, 0, 0, 0)
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs", map[string]interface{}{
			"name":    "oversized-job",
			"image":   "pytorch:latest",
			"command": "python train.py",
			"gpus":    8,
			"memory":  512,
		})
		AssertStatus(t, w, http.StatusConflict)
	})
}

func TestTrainingCheckpointAndResume(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs", map[string]interface{}{
		"name":   "resumable-job",
		"image":  "pytorch:latest",
		"gpus":   1,
		"memory": 32,
		"checkpointing": map[string]interface{}{
			"enabled":        true,
			"interval_steps": 100,
			"max_retries":    3,
		},
	})
	AssertStatus(t, w, http.StatusCreated)
	job := ParseJSON[models.Job](t, w)

	t.Run("record checkpoint", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs/"+job.ID+"/checkpoints", map[string]interface{}{
			"uri":  "s3://ckpt/resumable-job/step-500",
			"step": 500,
		})
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("stale checkpoint is rejected", func(t *testing.T) {
		
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs/"+job.ID+"/checkpoints", map[string]interface{}{
			"uri":  "s3://ckpt/resumable-job/step-100",
			"step": 100,
		})
		AssertStatus(t, w, http.StatusConflict)
	})

	t.Run("checkpoint view exposes the latest pointer", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/training-jobs/"+job.ID+"/checkpoints")
		AssertStatus(t, w, http.StatusOK)
		view := ParseJSON[map[string]interface{}](t, w)
		if view["latest_uri"] != "s3://ckpt/resumable-job/step-500" {
			t.Errorf("unexpected latest checkpoint: %v", view["latest_uri"])
		}
	})

	t.Run("failure with budget left parks the job for resume", func(t *testing.T) {
		markTrainingJobRunning(t, env, job.ID)
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs/"+job.ID+"/failures", map[string]interface{}{
			"reason": "CUDA OOM",
		})
		AssertStatus(t, w, http.StatusOK)
		body := ParseJSON[map[string]interface{}](t, w)
		if body["outcome"] != "retry" {
			t.Fatalf("expected retry outcome, got %v", body["outcome"])
		}

		w = env.DoJSON(http.MethodPost, "/api/v1/training-jobs/"+job.ID+"/resume", nil)
		AssertStatus(t, w, http.StatusOK)
		resumed := ParseJSON[models.Job](t, w)
		if resumed.Status != models.JobStatusPending {
			t.Errorf("expected pending after resume, got %s", resumed.Status)
		}
		if resumed.ResumeFrom != "s3://ckpt/resumable-job/step-500" {
			t.Errorf("expected resume pointer to be set, got %q", resumed.ResumeFrom)
		}
	})
}

func TestCancelTrainingJob(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs", map[string]interface{}{
		"name":   "cancellable-job",
		"image":  "pytorch:latest",
		"gpus":   1,
		"memory": 32,
	})
	AssertStatus(t, w, http.StatusCreated)
	job := ParseJSON[models.Job](t, w)

	t.Run("cancel moves the job to a terminal state", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs/"+job.ID+"/cancel", nil)
		AssertStatus(t, w, http.StatusOK)
		if got := ParseJSON[models.Job](t, w); got.Status != models.JobStatusCancelled {
			t.Fatalf("expected cancelled, got %s", got.Status)
		}
	})

	t.Run("cancelling a terminal job returns 409", func(t *testing.T) {
		
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs/"+job.ID+"/cancel", nil)
		AssertStatus(t, w, http.StatusConflict)
	})
}

func TestDeleteTrainingJob(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	t.Run("delete existing job", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs", map[string]interface{}{
			"name":    "job-to-delete",
			"image":   "pytorch:latest",
			"command": "python train.py",
			"gpus":    1,
			"memory":  32,
		})
		AssertStatus(t, w, http.StatusCreated)
		job := ParseJSON[models.Job](t, w)

		w = env.DoJSON(http.MethodDelete, "/api/v1/training-jobs/"+job.ID, nil)
		AssertStatus(t, w, http.StatusNoContent)

		w = env.DoGET(http.MethodGet, "/api/v1/training-jobs/"+job.ID)
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestGetTrainingTemplates(t *testing.T) {
	env := NewTestEnv(t)

	w := env.DoGET(http.MethodGet, "/api/v1/training-templates")
	AssertStatus(t, w, http.StatusOK)
	templates := ParseJSON[[]map[string]interface{}](t, w)
	if len(templates) == 0 {
		t.Fatal("expected builtin training templates")
	}
}