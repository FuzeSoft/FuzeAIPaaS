package data

import (
	"testing"
)

func TestPipelineStatusTransitions(t *testing.T) {
	cases := []struct {
		from PipelineStatus
		to   PipelineStatus
		ok   bool
	}{
		{PipelineStatusDraft, PipelineStatusPending, true},
		{PipelineStatusPending, PipelineStatusRunning, true},
		{PipelineStatusRunning, PipelineStatusCompleted, true},
		{PipelineStatusRunning, PipelineStatusFailed, true},
		{PipelineStatusRunning, PipelineStatusCancelled, true},
		{PipelineStatusDraft, PipelineStatusCancelled, true},
		{PipelineStatusCompleted, PipelineStatusRunning, false}, 
		{PipelineStatusFailed, PipelineStatusRunning, false},
		{PipelineStatusCancelled, PipelineStatusRunning, false},
	}
	for _, c := range cases {
		got := CanTransitionPipeline(c.from, c.to)
		if got != c.ok {
			t.Fatalf("transition %s->%s: want %v got %v", c.from, c.to, c.ok, got)
		}
	}
}

func TestStepStatusFromJobStatus(t *testing.T) {
	cases := []struct {
		job  JobStatus
		step StepStatus
	}{
		{JobStatusPending, StepStatusPending},
		{JobStatusRunning, StepStatusRunning},
		{JobStatusRetrying, StepStatusRunning},
		{JobStatusCompleted, StepStatusSucceeded},
		{JobStatusFailed, StepStatusFailed},
		{JobStatusCancelled, StepStatusSkipped},
		{JobStatusPaused, StepStatusPending},
	}
	for _, c := range cases {
		if got := StepStatusFromJobStatus(c.job); got != c.step {
			t.Fatalf("job %s -> step want %s got %s", c.job, c.step, got)
		}
	}
}

func TestDetectCycle(t *testing.T) {
	
	steps := []PipelineStep{
		{ID: "a", DependsOn: `["c"]`},
		{ID: "b", DependsOn: `["a"]`},
		{ID: "c", DependsOn: `["b"]`},
	}
	if err := ValidateDAG(steps); err == nil {
		t.Fatal("expected cycle error, got nil")
	}

	ok := []PipelineStep{
		{ID: "a", DependsOn: `[]`},
		{ID: "b", DependsOn: `["a"]`},
		{ID: "c", DependsOn: `["a","b"]`},
	}
	if err := ValidateDAG(ok); err != nil {
		t.Fatalf("unexpected cycle error: %v", err)
	}

	bad := []PipelineStep{
		{ID: "a", DependsOn: `["ghost"]`},
	}
	if err := ValidateDAG(bad); err == nil {
		t.Fatal("expected missing-dependency error, got nil")
	}
}

func TestReadySteps(t *testing.T) {
	steps := []PipelineStep{
		{ID: "a", DependsOn: `[]`, Status: StepStatusSucceeded},
		{ID: "b", DependsOn: `["a"]`, Status: StepStatusPending},
		{ID: "c", DependsOn: `["a","b"]`, Status: StepStatusPending},
		{ID: "d", DependsOn: `[]`, Status: StepStatusPending},
		{ID: "e", DependsOn: `["b"]`, Status: StepStatusRunning}, 
	}
	ready := ReadySteps(steps)
	got := map[string]bool{}
	for _, s := range ready {
		got[s.ID] = true
	}
	
	if !got["b"] || !got["d"] || got["a"] || got["c"] || got["e"] {
		t.Fatalf("ready set mismatch: %v", got)
	}
}

func TestOperatorBuiltinValidation(t *testing.T) {
	if _, ok := BuiltinOperators["dedup"]; !ok {
		t.Fatal("dedup should be a builtin operator")
	}
	if _, ok := BuiltinOperators["format_convert"]; !ok {
		t.Fatal("format_convert should be a builtin operator")
	}

	if err := ValidateOperator("dedup", `{"method":"exact"}`); err != nil {
		t.Fatalf("valid dedup params rejected: %v", err)
	}
	
	if err := ValidateOperator("nonexistent", `{}`); err == nil {
		t.Fatal("unknown operator should error")
	}
	
	if err := ValidateOperator("", `{}`); err != nil {
		t.Fatalf("custom operator must be allowed: %v", err)
	}
}