package evaluation

import (
	"testing"
)

func sampleEval() *Evaluation {
	return &Evaluation{
		ID:       "e-1",
		Name:     "eval-acc",
		Task:     "classification",
		Dataset:  "imagenet-val",
		Status:   StatusPending,
		TenantID: "t-1",
		Criteria: `{"accuracy": {"op": ">=", "value": 0.8}}`,
		Score:    0,
		Passed:   false,
	}
}

func multiCriteriaEval() *Evaluation {
	return &Evaluation{
		ID:       "e-2",
		Name:     "multi",
		Status:   StatusPending,
		TenantID: "t-1",
		Criteria: `{"accuracy": {"op": ">=", "value": 0.8}, "latency_ms": {"op": "<=", "value": 100}}`,
	}
}

func TestEvaluatePass(t *testing.T) {
	e := sampleEval()
	if err := e.RecordResult(map[string]float64{"accuracy": 0.85}, 0.85); err != nil {
		t.Fatalf("record: %v", err)
	}
	if !e.Passed {
		t.Fatalf("expected passed=true for accuracy 0.85 >= 0.8")
	}
	if e.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", e.Status)
	}
}

func TestEvaluateFail(t *testing.T) {
	e := sampleEval()
	if err := e.RecordResult(map[string]float64{"accuracy": 0.7}, 0.7); err != nil {
		t.Fatalf("record: %v", err)
	}
	if e.Passed {
		t.Fatalf("expected passed=false for accuracy 0.7 < 0.8")
	}
}

func TestEvaluateMultipleCriteriaAllPass(t *testing.T) {
	e := multiCriteriaEval()
	if err := e.RecordResult(map[string]float64{"accuracy": 0.9, "latency_ms": 80}, 0.9); err != nil {
		t.Fatalf("record: %v", err)
	}
	if !e.Passed {
		t.Fatalf("expected passed=true when all criteria met")
	}
}

func TestEvaluateMultipleCriteriaOneFail(t *testing.T) {
	e := multiCriteriaEval()
	if err := e.RecordResult(map[string]float64{"accuracy": 0.9, "latency_ms": 150}, 0.9); err != nil {
		t.Fatalf("record: %v", err)
	}
	if e.Passed {
		t.Fatalf("expected passed=false when latency exceeds bound")
	}
}

func TestEvaluateMissingMetricInCriteria(t *testing.T) {
	e := sampleEval()
	
	if err := e.RecordResult(map[string]float64{"other": 0.9}, 0.9); err != nil {
		t.Fatalf("record: %v", err)
	}
	if e.Passed {
		t.Fatalf("expected passed=false when required metric absent")
	}
}

func TestStateMachine(t *testing.T) {
	e := sampleEval()
	if err := e.MarkRunning(); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	if e.Status != StatusRunning {
		t.Fatalf("expected running")
	}
	
	if err := e.MarkRunning(); err == nil {
		t.Fatalf("expected error re-marking running from running")
	}
	if err := e.RecordResult(map[string]float64{"accuracy": 0.9}, 0.9); err != nil {
		t.Fatalf("record after running: %v", err)
	}
}

func TestEvaluateBoundaryEqualityPasses(t *testing.T) {
	e := sampleEval() 
	if err := e.RecordResult(map[string]float64{"accuracy": 0.8}, 0.8); err != nil {
		t.Fatalf("record: %v", err)
	}
	if !e.Passed {
		t.Fatalf("expected passed=true at exact threshold 0.8 for >=")
	}
}

func TestEvaluateUnknownOperatorFailsClosed(t *testing.T) {
	e := &Evaluation{
		ID:       "e-3",
		Status:   StatusPending,
		TenantID: "t-1",
		Criteria: `{"accuracy": {"op": "~~", "value": 0.8}}`,
	}
	if err := e.RecordResult(map[string]float64{"accuracy": 0.99}, 0.99); err != nil {
		t.Fatalf("record: %v", err)
	}
	if e.Passed {
		t.Fatalf("unknown operator must not pass (fail-closed)")
	}
}

func TestAggregateReportRejectsUndefinedDimensionCriteria(t *testing.T) {
	e := &Evaluation{
		ID:         "e-4",
		Status:     StatusCompleted,
		TenantID:   "t-1",
		JudgeMode:  JudgeModeHuman,
		Dimensions: `[{"name":"accuracy","weight":1}]`,
		
		Criteria: `{"precision": {"op": ">=", "value": 0.8}}`,
	}
	reviews := []Review{{
		EvaluationID: "e-4", JudgeType: JudgeTypeHuman, JudgeID: "u-1",
		Scores: `{"accuracy": 0.9}`,
	}}
	if _, err := e.AggregateReport(reviews); err == nil {
		t.Fatalf("expected error for criteria referencing undefined dimension precision")
	}
}

func TestAggregateReportAcceptsDefinedDimensionAndReservedKeys(t *testing.T) {
	cases := []string{
		`{"accuracy": {"op": ">=", "value": 0.8}}`, 
		`{"overall": {"op": ">=", "value": 0.8}}`,  
		`{"score": {"op": ">=", "value": 0.8}}`,    
	}
	for i, c := range cases {
		e := &Evaluation{
			ID:         "e-5",
			Status:     StatusCompleted,
			TenantID:   "t-1",
			JudgeMode:  JudgeModeHuman,
			Dimensions: `[{"name":"accuracy","weight":1}]`,
			Criteria:   c,
		}
		reviews := []Review{{
			EvaluationID: "e-5", JudgeType: JudgeTypeHuman, JudgeID: "u-1",
			Scores: `{"accuracy": 0.9}`,
		}}
		if _, err := e.AggregateReport(reviews); err != nil {
			t.Fatalf("case %d (criteria=%s) should not error: %v", i, c, err)
		}
	}
}