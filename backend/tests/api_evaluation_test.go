package tests

import (
	"net/http"
	"testing"
)

func TestAPIEvaluationLifecycle(t *testing.T) {
	env := NewTestEnv(t)

	exp := createExperiment(t, env, "maximize", "accuracy")
	run := createRun(t, env, exp.ID, "trial-1", nil)

	body := map[string]interface{}{
		"name":          "acc-eval",
		"task":          "classification",
		"dataset":       "imagenet-val",
		"experiment_id": exp.ID,
		"run_id":        run.ID,
		"criteria":      `{"accuracy": {"op": ">=", "value": 0.8}}`,
	}
	w := env.DoJSON(http.MethodPost, "/api/v1/evaluations", body)
	AssertStatus(t, w, http.StatusCreated)
	created := ParseJSON[map[string]interface{}](t, w)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("expected id in create response, got %+v", created)
	}

	w = env.DoJSON(http.MethodGet, "/api/v1/evaluations", nil)
	AssertStatus(t, w, http.StatusOK)
	list := ParseJSON[map[string]interface{}](t, w)
	evs, _ := list["evaluations"].([]interface{})
	if len(evs) < 1 {
		t.Fatalf("expected >=1 evaluation, got %d", len(evs))
	}

	w = env.DoJSON(http.MethodPost, "/api/v1/evaluations/"+id+"/result", map[string]interface{}{
		"metrics": map[string]float64{"accuracy": 0.9},
		"score":   0.9,
	})
	AssertStatus(t, w, http.StatusNoContent)

	w = env.DoJSON(http.MethodGet, "/api/v1/evaluations/"+id, nil)
	AssertStatus(t, w, http.StatusOK)
	got := ParseJSON[map[string]interface{}](t, w)
	if got["status"] != "completed" {
		t.Fatalf("expected completed, got %v", got["status"])
	}
	if passed, _ := got["passed"].(bool); !passed {
		t.Fatalf("expected passed=true, got %+v", got)
	}

	w = env.DoJSON(http.MethodGet, "/api/v1/experiments/"+exp.ID+"/evaluations", nil)
	AssertStatus(t, w, http.StatusOK)

	w = env.DoJSON(http.MethodDelete, "/api/v1/evaluations/"+id, nil)
	AssertStatus(t, w, http.StatusOK)
}

func TestAPIEvaluationCreateRequiresAssociation(t *testing.T) {
	env := NewTestEnv(t)
	w := env.DoJSON(http.MethodPost, "/api/v1/evaluations", map[string]interface{}{
		"name": "orphan",
	})
	AssertStatus(t, w, http.StatusBadRequest)
	AssertError(t, w)
}

func TestAPIEvaluationCreateRequiresExperimentRunPair(t *testing.T) {
	env := NewTestEnv(t)
	w := env.DoJSON(http.MethodPost, "/api/v1/evaluations", map[string]interface{}{
		"name":          "x",
		"experiment_id": "exp-1", 
	})
	AssertStatus(t, w, http.StatusBadRequest)
	AssertError(t, w)
}

func TestAPIEvaluationResultMissingFails(t *testing.T) {
	env := NewTestEnv(t)
	w := env.DoJSON(http.MethodPost, "/api/v1/evaluations", map[string]interface{}{
		"name":    "x",
		"model_id": "m-1",
		"criteria": `{"accuracy": {"op": ">=", "value": 0.8}}`,
	})
	AssertStatus(t, w, http.StatusCreated)
	id := ParseJSON[map[string]interface{}](t, w)["id"].(string)

	w = env.DoJSON(http.MethodPost, "/api/v1/evaluations/"+id+"/result", map[string]interface{}{
		"metrics": map[string]float64{"other": 0.9},
		"score":   0.9,
	})
	AssertStatus(t, w, http.StatusNoContent)
	w = env.DoJSON(http.MethodGet, "/api/v1/evaluations/"+id, nil)
	got := ParseJSON[map[string]interface{}](t, w)
	if passed, _ := got["passed"].(bool); passed {
		t.Fatalf("expected passed=false when required metric absent")
	}
}