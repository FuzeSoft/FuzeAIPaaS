package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func decode(t *testing.T, w *httptest.ResponseRecorder, out interface{}) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
		t.Fatalf("failed to decode response: %v, body: %s", err, w.Body.String())
	}
}

func createExperiment(t *testing.T, env *TestEnv, objective, metricName string) models.Experiment {
	t.Helper()
	w := env.DoJSON(http.MethodPost, "/api/v1/experiments", map[string]interface{}{
		"name":        "exp-" + metricName,
		"description": "test experiment",
		"objective":   objective,
		"metric_name": metricName,
		"tags":        []string{"smoke"},
	})
	AssertStatus(t, w, http.StatusCreated)
	return ParseJSON[models.Experiment](t, w)
}

func createRun(t *testing.T, env *TestEnv, expID, name string, hp map[string]interface{}) models.Run {
	t.Helper()
	w := env.DoJSON(http.MethodPost, "/api/v1/experiments/"+expID+"/runs", map[string]interface{}{
		"name":            name,
		"hyperparameters": hp,
	})
	AssertStatus(t, w, http.StatusCreated)
	return ParseJSON[models.Run](t, w)
}

func TestCreateExperimentRequiresMetricName(t *testing.T) {
	env := NewTestEnv(t)
	w := env.DoJSON(http.MethodPost, "/api/v1/experiments", map[string]interface{}{
		"name":      "bad",
		"objective": "maximize",
		
	})
	
	AssertStatus(t, w, http.StatusBadRequest)
	AssertError(t, w)
}

func TestCreateAndGetExperiment(t *testing.T) {
	env := NewTestEnv(t)
	exp := createExperiment(t, env, "maximize", "val_accuracy")

	if exp.ID == "" {
		t.Fatalf("expected experiment id to be populated")
	}
	if exp.Objective != "maximize" {
		t.Fatalf("expected objective maximize, got %s", exp.Objective)
	}

	w := env.DoGET(http.MethodGet, "/api/v1/experiments/"+exp.ID)
	AssertStatus(t, w, http.StatusOK)
	var body struct {
		Experiment models.Experiment `json:"experiment"`
		Runs       []models.Run      `json:"runs"`
	}
	decode(t, w, &body)
	if body.Experiment.ID != exp.ID {
		t.Fatalf("expected experiment id %s, got %s", exp.ID, body.Experiment.ID)
	}
}

func TestCreateRunUnderExperiment(t *testing.T) {
	env := NewTestEnv(t)
	exp := createExperiment(t, env, "minimize", "loss")

	run := createRun(t, env, exp.ID, "trial-1", map[string]interface{}{"lr": 0.01, "bs": 32})
	if run.ID == "" {
		t.Fatalf("expected run id populated")
	}
	if run.Status != models.RunStatusPending {
		t.Fatalf("expected pending status, got %s", run.Status)
	}

	w := env.DoGET(http.MethodGet, "/api/v1/experiments/"+exp.ID+"/runs")
	AssertStatus(t, w, http.StatusOK)
	var runs []models.Run
	decode(t, w, &runs)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
}

func TestRunStateMachineAndBestRunSelection(t *testing.T) {
	env := NewTestEnv(t)
	
	exp := createExperiment(t, env, "minimize", "loss")

	runA := createRun(t, env, exp.ID, "trial-a", map[string]interface{}{"lr": 0.1})
	runB := createRun(t, env, exp.ID, "trial-b", map[string]interface{}{"lr": 0.01})

	wA := env.DoJSON(http.MethodPost, "/api/v1/experiments/"+exp.ID+"/runs/"+runA.ID+"/complete", map[string]interface{}{
		"metric_value": 0.5,
		"metrics":      map[string]interface{}{"loss": 0.5},
		"artifact_uri": "s3://bucket/runA",
	})
	AssertStatus(t, wA, http.StatusOK)

	wB := env.DoJSON(http.MethodPost, "/api/v1/experiments/"+exp.ID+"/runs/"+runB.ID+"/complete", map[string]interface{}{
		"metric_value": 0.2,
		"metrics":      map[string]interface{}{"loss": 0.2},
		"artifact_uri": "s3://bucket/runB",
	})
	AssertStatus(t, wB, http.StatusOK)

	wExp := env.DoGET(http.MethodGet, "/api/v1/experiments/"+exp.ID)
	AssertStatus(t, wExp, http.StatusOK)
	var body struct {
		Experiment models.Experiment `json:"experiment"`
		Runs       []models.Run      `json:"runs"`
	}
	decode(t, wExp, &body)
	if body.Experiment.BestRunID != runB.ID {
		t.Fatalf("expected best run %s, got %s", runB.ID, body.Experiment.BestRunID)
	}
	
	for _, r := range body.Runs {
		if r.ID == runB.ID && !r.IsBest {
			t.Fatalf("expected runB is_best=true")
		}
		if r.ID == runA.ID && r.IsBest {
			t.Fatalf("expected runA is_best=false")
		}
	}
}

func TestCompleteRunTwiceIsConflict(t *testing.T) {
	env := NewTestEnv(t)
	exp := createExperiment(t, env, "maximize", "acc")
	run := createRun(t, env, exp.ID, "trial", nil)

	w1 := env.DoJSON(http.MethodPost, "/api/v1/experiments/"+exp.ID+"/runs/"+run.ID+"/complete", map[string]interface{}{
		"metric_value": 0.9,
	})
	AssertStatus(t, w1, http.StatusOK)

	w2 := env.DoJSON(http.MethodPost, "/api/v1/experiments/"+exp.ID+"/runs/"+run.ID+"/complete", map[string]interface{}{
		"metric_value": 0.95,
	})
	AssertStatus(t, w2, http.StatusConflict)
}

func TestArchiveExperiment(t *testing.T) {
	env := NewTestEnv(t)
	exp := createExperiment(t, env, "maximize", "acc")

	w := env.DoJSON(http.MethodPost, "/api/v1/experiments/"+exp.ID+"/archive", nil)
	AssertStatus(t, w, http.StatusOK)
	var updated models.Experiment
	decode(t, w, &updated)
	if updated.Status != models.ExperimentStatusArchived {
		t.Fatalf("expected archived, got %s", updated.Status)
	}
}

func TestDeleteExperimentCascadesRuns(t *testing.T) {
	env := NewTestEnv(t)
	exp := createExperiment(t, env, "maximize", "acc")
	createRun(t, env, exp.ID, "trial", nil)

	w := env.DoJSON(http.MethodDelete, "/api/v1/experiments/"+exp.ID, nil)
	AssertStatus(t, w, http.StatusOK)

	wGet := env.DoGET(http.MethodGet, "/api/v1/experiments/"+exp.ID)
	AssertStatus(t, wGet, http.StatusNotFound)
}

func TestQueryMetricsNoopReturnsEmpty(t *testing.T) {
	env := NewTestEnv(t)
	w := env.DoJSON(http.MethodPost, "/api/v1/metrics/query", map[string]interface{}{
		"query": "gpu_utilization",
	})
	AssertStatus(t, w, http.StatusOK)
	var body struct {
		Series []interface{} `json:"series"`
	}
	decode(t, w, &body)
	if len(body.Series) != 0 {
		t.Fatalf("expected empty series from noop metrics, got %d", len(body.Series))
	}
}