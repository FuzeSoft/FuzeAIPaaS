package evaluation

import (
	"testing"
)

func judgeEval(mode string) *Evaluation {
	return &Evaluation{
		ID:         "e-judge",
		Name:       "judge-eval",
		Task:       "qa",
		Status:     StatusPending,
		TenantID:   "t-1",
		JudgeMode:  mode,
		Dimensions: `[{"name":"accuracy","weight":0.5,"description":"正确性"},{"name":"fluency","weight":0.3,"description":"流畅度"},{"name":"relevance","weight":0.2,"description":"相关性"}]`,
	}
}

func TestParseDimensionsValid(t *testing.T) {
	dims, err := ParseDimensions(`[{"name":"a","weight":0.5},{"name":"b","weight":0.5}]`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(dims) != 2 {
		t.Fatalf("expected 2 dims, got %d", len(dims))
	}
	if dims[0].Name != "a" || dims[0].Weight != 0.5 {
		t.Fatalf("unexpected dim[0]: %+v", dims[0])
	}
}

func TestParseDimensionsNormalizesWeights(t *testing.T) {
	
	dims, err := ParseDimensions(`[{"name":"a","weight":1},{"name":"b","weight":1}]`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if dims[0].Weight != 0.5 || dims[1].Weight != 0.5 {
		t.Fatalf("expected normalized 0.5/0.5, got %v/%v", dims[0].Weight, dims[1].Weight)
	}
}

func TestParseDimensionsEmpty(t *testing.T) {
	dims, err := ParseDimensions("")
	if err != nil {
		t.Fatalf("empty parse should not error: %v", err)
	}
	if dims != nil {
		t.Fatalf("expected nil for empty, got %v", dims)
	}
}

func TestParseDimensionsNonPositiveWeight(t *testing.T) {
	if _, err := ParseDimensions(`[{"name":"a","weight":0}]`); err == nil {
		t.Fatalf("expected error for zero weight")
	}
	if _, err := ParseDimensions(`[{"name":"a","weight":-1}]`); err == nil {
		t.Fatalf("expected error for negative weight")
	}
}

func TestParseDimensionsInvalidJSON(t *testing.T) {
	if _, err := ParseDimensions(`{invalid`); err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}

func TestAddReviewHumanMode(t *testing.T) {
	e := judgeEval(JudgeModeHuman)
	rv := Review{JudgeType: JudgeTypeHuman, JudgeID: "u-1"}
	if err := e.AddReview(rv); err != nil {
		t.Fatalf("human review on human-mode should pass: %v", err)
	}
}

func TestAddReviewLLMRejectedInHumanMode(t *testing.T) {
	e := judgeEval(JudgeModeHuman)
	rv := Review{JudgeType: JudgeTypeLLM, JudgeID: "gpt-4o"}
	if err := e.AddReview(rv); err == nil {
		t.Fatalf("llm review on human-mode should be rejected")
	}
}

func TestAddReviewHybridAcceptsBoth(t *testing.T) {
	e := judgeEval(JudgeModeHybrid)
	if err := e.AddReview(Review{JudgeType: JudgeTypeHuman, JudgeID: "u-1"}); err != nil {
		t.Fatalf("human review on hybrid should pass: %v", err)
	}
	if err := e.AddReview(Review{JudgeType: JudgeTypeLLM, JudgeID: "gpt-4o"}); err != nil {
		t.Fatalf("llm review on hybrid should pass: %v", err)
	}
}

func TestAddReviewThresholdRejectsAll(t *testing.T) {
	e := judgeEval(JudgeModeThreshold)
	rv := Review{JudgeType: JudgeTypeHuman, JudgeID: "u-1"}
	if err := e.AddReview(rv); err == nil {
		t.Fatalf("threshold mode should reject reviews")
	}
}

func TestAddReviewFinalizedRejected(t *testing.T) {
	e := judgeEval(JudgeModeHuman)
	e.Status = StatusCompleted
	if err := e.AddReview(Review{JudgeType: JudgeTypeHuman, JudgeID: "u-1"}); err == nil {
		t.Fatalf("finalized evaluation should reject reviews")
	}
}

func TestAddReviewMissingJudgeID(t *testing.T) {
	e := judgeEval(JudgeModeHuman)
	if err := e.AddReview(Review{JudgeType: JudgeTypeHuman, JudgeID: ""}); err == nil {
		t.Fatalf("missing judge_id should be rejected")
	}
}

func TestAddReviewUnknownMode(t *testing.T) {
	e := judgeEval("bogus")
	if err := e.AddReview(Review{JudgeType: JudgeTypeHuman, JudgeID: "u-1"}); err == nil {
		t.Fatalf("unknown judge mode should be rejected")
	}
}

func TestAggregateReportWeightedOverall(t *testing.T) {
	e := judgeEval(JudgeModeHuman)
	reviews := []Review{
		{JudgeType: JudgeTypeHuman, JudgeID: "u-1", Scores: `{"accuracy":0.9,"fluency":0.8,"relevance":0.7}`},
		{JudgeType: JudgeTypeHuman, JudgeID: "u-2", Scores: `{"accuracy":0.8,"fluency":0.6,"relevance":0.9}`},
	}
	report, err := e.AggregateReport(reviews)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	
	expected := 0.795
	if abs(report.Overall-expected) > 0.001 {
		t.Fatalf("expected overall %.4f, got %.4f", expected, report.Overall)
	}
	
	if report.Verdict != VerdictPass {
		t.Fatalf("expected verdict pass, got %s", report.Verdict)
	}
}

func TestAggregateReportByJudgeType(t *testing.T) {
	e := judgeEval(JudgeModeHybrid)
	reviews := []Review{
		{JudgeType: JudgeTypeHuman, JudgeID: "u-1", Scores: `{"accuracy":1.0,"fluency":1.0,"relevance":1.0}`},
		{JudgeType: JudgeTypeLLM, JudgeID: "stub", Scores: `{"accuracy":0.5,"fluency":0.5,"relevance":0.5}`},
	}
	report, err := e.AggregateReport(reviews)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if report.ByJudgeType[JudgeTypeHuman].Count != 1 {
		t.Fatalf("expected 1 human review, got %d", report.ByJudgeType[JudgeTypeHuman].Count)
	}
	if report.ByJudgeType[JudgeTypeLLM].Count != 1 {
		t.Fatalf("expected 1 llm review, got %d", report.ByJudgeType[JudgeTypeLLM].Count)
	}
	
	if abs(report.ByJudgeType[JudgeTypeHuman].Overall-1.0) > 0.001 {
		t.Fatalf("expected human overall 1.0, got %v", report.ByJudgeType[JudgeTypeHuman].Overall)
	}
}

func TestAggregateReportNoDimensions(t *testing.T) {
	e := &Evaluation{JudgeMode: JudgeModeHuman, Dimensions: ""}
	if _, err := e.AggregateReport(nil); err == nil {
		t.Fatalf("expected error when no dimensions defined")
	}
}

func TestAggregateReportVerdictByOverallThreshold(t *testing.T) {
	e := judgeEval(JudgeModeHuman)
	
	e.Criteria = `{"overall": {"op": ">=", "value": 0.8}}`
	reviews := []Review{
		{JudgeType: JudgeTypeHuman, JudgeID: "u-1", Scores: `{"accuracy":0.5,"fluency":0.5,"relevance":0.5}`},
	}
	
	report, err := e.AggregateReport(reviews)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if report.Verdict != VerdictFail {
		t.Fatalf("expected verdict fail (0.5 < 0.8), got %s", report.Verdict)
	}
}

func TestFinalizeWritesReport(t *testing.T) {
	e := judgeEval(JudgeModeHuman)
	reviews := []Review{
		{JudgeType: JudgeTypeHuman, JudgeID: "u-1", Scores: `{"accuracy":0.9,"fluency":0.8,"relevance":0.7}`},
	}
	report, err := e.AggregateReport(reviews)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if err := e.Finalize(report); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if e.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", e.Status)
	}
	if e.Report == "" {
		t.Fatalf("expected report JSON to be written")
	}
	if !e.Passed {
		t.Fatalf("expected passed=true (verdict=pass)")
	}
}

func TestFinalizeAlreadyFinalized(t *testing.T) {
	e := judgeEval(JudgeModeHuman)
	e.Status = StatusCompleted
	if err := e.Finalize(Report{}); err == nil {
		t.Fatalf("finalize on completed should error")
	}
}

func TestVerdictPendingWhenNoReviews(t *testing.T) {
	e := judgeEval(JudgeModeHuman)
	report, err := e.AggregateReport(nil)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if report.Verdict != VerdictPending {
		t.Fatalf("expected pending when no reviews, got %s", report.Verdict)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}