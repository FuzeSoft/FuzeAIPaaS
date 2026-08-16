package experiment

import (
	"testing"
	"time"
)

func fptr(v float64) *float64 { return &v }

func TestExperimentValidate(t *testing.T) {
	
	e := &Experiment{Name: "x", Objective: ObjectiveMaximize}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected validation error for missing metric_name")
	}

	e = &Experiment{Name: "x", Objective: "average", MetricName: "acc"}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected validation error for bad objective")
	}

	e = &Experiment{Name: "x", Objective: ObjectiveMaximize, MetricName: "acc"}
	e.Normalize()
	if err := e.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if e.Status != StatusActive {
		t.Fatalf("expected default active status, got %s", e.Status)
	}
}

func TestExperimentIsBetter(t *testing.T) {
	
	maxe := &Experiment{Objective: ObjectiveMaximize, MetricName: "acc"}
	if !maxe.IsBetter(&Run{MetricValue: fptr(0.9)}, &Run{MetricValue: fptr(0.8)}) {
		t.Fatalf("expected 0.9 better than 0.8 under maximize")
	}
	if maxe.IsBetter(&Run{MetricValue: fptr(0.7)}, &Run{MetricValue: fptr(0.8)}) {
		t.Fatalf("expected 0.7 worse than 0.8 under maximize")
	}

	mine := &Experiment{Objective: ObjectiveMinimize, MetricName: "loss"}
	if !mine.IsBetter(&Run{MetricValue: fptr(0.1)}, &Run{MetricValue: fptr(0.5)}) {
		t.Fatalf("expected 0.1 better than 0.5 under minimize")
	}

	if mine.IsBetter(&Run{}, &Run{MetricValue: fptr(0.5)}) {
		t.Fatalf("expected nil metric value to be not better")
	}
	
	if !maxe.IsBetter(&Run{MetricValue: fptr(0.1)}, nil) {
		t.Fatalf("expected any valued candidate better than nil current")
	}
}

func TestExperimentUpdateBest(t *testing.T) {
	e := &Experiment{Objective: ObjectiveMaximize, MetricName: "acc"}
	runA := &Run{ID: "a", MetricValue: fptr(0.8)}
	runB := &Run{ID: "b", MetricValue: fptr(0.95)}

	if !e.UpdateBest(runA, nil) {
		t.Fatalf("expected first run to become best")
	}
	if e.BestRunID != "a" {
		t.Fatalf("expected best a, got %s", e.BestRunID)
	}
	if !e.UpdateBest(runB, runA) {
		t.Fatalf("expected better run to update best")
	}
	if e.BestRunID != "b" {
		t.Fatalf("expected best b, got %s", e.BestRunID)
	}
	
	if e.UpdateBest(&Run{ID: "c", MetricValue: fptr(0.5)}, runB) {
		t.Fatalf("expected worse run not to update best")
	}
	if e.BestRunID != "b" {
		t.Fatalf("expected best still b, got %s", e.BestRunID)
	}
}

func TestRunStateMachine(t *testing.T) {
	r := &Run{Name: "t", ExperimentID: "e", Status: RunStatusPending}
	r.Normalize()

	if !r.MarkRunning(timeNow()) {
		t.Fatalf("expected mark running")
	}
	if r.Status != RunStatusRunning {
		t.Fatalf("expected running, got %s", r.Status)
	}
	if r.StartedAt == nil {
		t.Fatalf("expected started_at set")
	}

	if r.MarkRunning(timeNow()) {
		t.Fatalf("expected duplicate mark running to be no-op")
	}

	if !r.Complete(fptr(0.9), "s3://x", timeNow()) {
		t.Fatalf("expected complete")
	}
	if r.Status != RunStatusCompleted || r.EndedAt == nil {
		t.Fatalf("expected completed with ended_at")
	}

	if r.Complete(fptr(0.1), "s3://y", timeNow()) {
		t.Fatalf("expected complete on terminal to be no-op")
	}
	if r.MarkFailed(timeNow()) {
		t.Fatalf("expected mark failed on terminal to be no-op")
	}
}

func TestRunTerminal(t *testing.T) {
	cases := []string{RunStatusCompleted, RunStatusFailed, RunStatusCancelled}
	for _, s := range cases {
		r := &Run{Status: s}
		if !r.IsTerminal() {
			t.Fatalf("expected %s to be terminal", s)
		}
	}
	if (&Run{Status: RunStatusRunning}).IsTerminal() {
		t.Fatalf("expected running to be non-terminal")
	}
}

func timeNow() time.Time { return time.Now() }