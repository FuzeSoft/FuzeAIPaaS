package tests

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestTrainingJobLifecycle(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")
	env.EnsureDefaultQuota(t, 128, 2048, 50)

	w := env.DoAuthJSON(http.MethodPost, "/api/v1/training-jobs", token, map[string]interface{}{
		"name":    "train-resnet",
		"image":   "pytorch/pytorch:2.1",
		"command": "python train.py",
		"gpus":    1,
		"memory":  16,
	})
	AssertStatus(t, w, http.StatusCreated)
	var createResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &createResp)
	jobID, _ := createResp["id"].(string)
	if jobID == "" {
		t.Fatal("failed to create training job: no id in response")
	}

	t.Run("get job shows created state", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/training-jobs/"+jobID, token)
		AssertStatus(t, w, http.StatusOK)
		body := ParseJSON[map[string]interface{}](t, w)
		if body["id"] != jobID {
			t.Errorf("expected job id %s, got %v", jobID, body["id"])
		}
		if body["status"] != "pending" {
			t.Errorf("expected pending, got %v", body["status"])
		}
	})

	t.Run("complete a running job", func(t *testing.T) {
		markTrainingJobRunning(t, env, jobID)
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/training-jobs/"+jobID+"/complete", token, nil)
		AssertStatus(t, w, http.StatusOK)
		body := ParseJSON[map[string]interface{}](t, w)
		if body["status"] != "completed" {
			t.Errorf("expected completed, got %v", body["status"])
		}
	})

	t.Run("resuming a completed job is rejected", func(t *testing.T) {
		
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/training-jobs/"+jobID+"/resume", token, nil)
		AssertStatus(t, w, http.StatusConflict)
	})

	t.Run("delete job", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/training-jobs/"+jobID, token, nil)
		AssertStatus(t, w, http.StatusNoContent)
	})
}

func TestTrainingJobsList(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")
	env.EnsureDefaultQuota(t, 128, 2048, 50)

	specs := []map[string]interface{}{
		{"name": "job-a", "image": "pytorch/pytorch:2.0", "gpus": 1, "memory": 8},
		{"name": "job-b", "image": "tensorflow/tensorflow:2.14", "gpus": 2, "memory": 32},
		{"name": "job-c", "image": "pytorch/pytorch:2.0", "gpus": 1, "memory": 16},
	}

	var createdIDs []string
	for _, s := range specs {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/training-jobs", token, s)
		AssertStatus(t, w, http.StatusCreated)
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		if id, ok := resp["id"].(string); ok {
			createdIDs = append(createdIDs, id)
		}
	}

	t.Run("list contains every created job", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/training-jobs", token)
		AssertStatus(t, w, http.StatusOK)
		listed := ParseJSON[[]map[string]interface{}](t, w)
		seen := map[string]bool{}
		for _, j := range listed {
			if id, ok := j["id"].(string); ok {
				seen[id] = true
			}
		}
		for _, id := range createdIDs {
			if !seen[id] {
				t.Errorf("created job %s missing from list", id)
			}
		}
	})

	for _, id := range createdIDs {
		env.DoAuthJSON(http.MethodDelete, "/api/v1/training-jobs/"+id, token, nil)
	}
}

func TestTrainingJobNotFoundScenarios(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	t.Run("get non-existent job returns 404", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/training-jobs/nonexistent-id", token)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("acting on a non-existent job returns 404", func(t *testing.T) {
		
		for _, action := range []string{"/cancel", "/resume", "/complete"} {
			w := env.DoAuthJSON(http.MethodPost, "/api/v1/training-jobs/nonexistent-id"+action, token, nil)
			AssertStatus(t, w, http.StatusNotFound)
		}
		w := env.DoAuthGET(http.MethodGet, "/api/v1/training-jobs/nonexistent-id", token)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("delete non-existent job returns 404", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/training-jobs/nonexistent-id", token, nil)
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestTrainingJobsValidation(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")
	env.EnsureDefaultQuota(t, 128, 2048, 50)

	t.Run("missing image is rejected", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/training-jobs", token, map[string]interface{}{
			"name": "minimal-job",
		})
		AssertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("physically impossible GPU count is rejected as bad input", func(t *testing.T) {
		
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/training-jobs", token, map[string]interface{}{
			"name":   "absurd-gpus",
			"image":  "pytorch/pytorch:2.0",
			"gpus":   10000,
			"memory": 8,
		})
		AssertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("GPU count beyond quota returns 409", func(t *testing.T) {
		
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/training-jobs", token, map[string]interface{}{
			"name":   "exceed-quota",
			"image":  "pytorch/pytorch:2.0",
			"gpus":   900,
			"memory": 8,
		})
		AssertStatus(t, w, http.StatusConflict)
	})
}