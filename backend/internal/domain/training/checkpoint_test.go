package training

import (
	"testing"
	"time"
)

func TestCheckpointPolicyNormalizeDefaults(t *testing.T) {
	p := CheckpointPolicy{Enabled: true}
	p.Normalize()
	if p.IntervalSteps <= 0 {
		t.Fatal("enabled policy must get a positive default interval")
	}
	if p.MaxRetries <= 0 {
		t.Fatal("enabled policy must get a positive default retry budget")
	}

	off := CheckpointPolicy{Enabled: false, IntervalSteps: 100, MaxRetries: 3}
	off.Normalize()
	if off.IntervalSteps != 0 || off.MaxRetries != 0 {
		t.Fatalf("disabled policy must be cleared: %+v", off)
	}
}

func TestCheckpointPolicyValidate(t *testing.T) {
	bad := []CheckpointPolicy{
		{Enabled: true, IntervalSteps: -1, MaxRetries: 1},
		{Enabled: true, IntervalSteps: 100, MaxRetries: -1},
		{Enabled: true, IntervalSteps: 100, MaxRetries: maxCheckpointRetries + 1},
	}
	for _, p := range bad {
		if err := p.Validate(); err == nil {
			t.Fatalf("policy %+v must be rejected", p)
		}
	}

	ok := CheckpointPolicy{Enabled: true, IntervalSteps: 100, MaxRetries: 3}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
}

func TestCheckpointValidate(t *testing.T) {
	if err := (Checkpoint{URI: "", Step: 1}).Validate(); err == nil {
		t.Fatal("checkpoint without URI must be rejected")
	}
	if err := (Checkpoint{URI: "s3://b/k", Step: -1}).Validate(); err == nil {
		t.Fatal("negative step must be rejected")
	}
	if err := (Checkpoint{URI: "s3://b/k", Step: 100, CreatedAt: time.Now()}).Validate(); err != nil {
		t.Fatalf("valid checkpoint rejected: %v", err)
	}
}