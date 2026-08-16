package training

import (
	"testing"
	"time"
)

func newJob() *TrainingJob {
	return &TrainingJob{
		ID:       "tj-1",
		TenantID: "t1",
		Name:     "bert-finetune",
		Spec:     validSpec(),
	}
}

func TestTrainingJobNormalizeAndValidate(t *testing.T) {
	j := newJob()
	j.Name = "  bert-finetune  "
	j.Normalize()
	if j.Name != "bert-finetune" {
		t.Fatalf("name not trimmed: %q", j.Name)
	}
	if j.ClusterID == "" {
		t.Fatal("cluster must default so multi-cluster routing stays deterministic")
	}
	if j.Status != StatusPending {
		t.Fatalf("fresh job must normalize to pending, got %q", j.Status)
	}
	if err := j.Validate(); err != nil {
		t.Fatalf("valid job rejected: %v", err)
	}
}

func TestTrainingJobValidateRequiresName(t *testing.T) {
	j := newJob()
	j.Name = ""
	if err := j.Validate(); err == nil {
		t.Fatal("name is mandatory")
	}

	j = newJob()
	j.Name = string(make([]byte, maxNameLen+1))
	if err := j.Validate(); err == nil {
		t.Fatal("overlong name must be rejected")
	}
}

func TestTrainingJobValidatePropagatesNestedErrors(t *testing.T) {
	j := newJob()
	j.Spec.GPUs = -1
	if err := j.Validate(); err == nil {
		t.Fatal("invalid spec must fail aggregate validation")
	}

	j = newJob()
	j.Registration = ModelRegistration{Enabled: true}
	if err := j.Validate(); err == nil {
		t.Fatal("invalid registration must fail aggregate validation")
	}

	j = newJob()
	j.Checkpointing = CheckpointPolicy{Enabled: true, MaxRetries: -5}
	if err := j.Validate(); err == nil {
		t.Fatal("invalid checkpoint policy must fail aggregate validation")
	}
}

func TestTrainingJobMarkRunningStampsStartOnce(t *testing.T) {
	j := newJob()
	j.Normalize()
	first := time.Now()
	if !j.MarkRunning(first) {
		t.Fatal("pending -> running must be allowed")
	}
	if j.StartedAt == nil || !j.StartedAt.Equal(first) {
		t.Fatal("first transition to running must stamp StartedAt")
	}

	j.MarkRetrying("node lost")
	j.MarkRunning(first.Add(time.Hour))
	if !j.StartedAt.Equal(first) {
		t.Fatal("StartedAt must be stamped only once")
	}
}

func TestTrainingJobTerminalTransitions(t *testing.T) {
	j := newJob()
	j.Normalize()
	j.MarkRunning(time.Now())
	if !j.MarkCompleted() {
		t.Fatal("running -> succeeded must be allowed")
	}
	if !j.IsTerminal() {
		t.Fatal("completed job must be terminal")
	}
	
	if j.MarkRunning(time.Now()) {
		t.Fatal("terminal job must not go back to running")
	}
}

func TestTrainingJobRecordCheckpointMustBeMonotonic(t *testing.T) {
	j := newJob()
	j.Checkpointing = CheckpointPolicy{Enabled: true, IntervalSteps: 100, MaxRetries: 3}
	j.Normalize()

	if err := j.RecordCheckpoint(Checkpoint{URI: "s3://b/ck-100", Step: 100}); err != nil {
		t.Fatalf("RecordCheckpoint: %v", err)
	}
	if j.LatestCheckpoint == nil || j.LatestCheckpoint.Step != 100 {
		t.Fatal("latest checkpoint not recorded")
	}

	if err := j.RecordCheckpoint(Checkpoint{URI: "s3://b/ck-50", Step: 50}); err == nil {
		t.Fatal("stale checkpoint must be rejected")
	}
	if j.LatestCheckpoint.Step != 100 {
		t.Fatal("stale checkpoint must not overwrite progress")
	}

	if err := j.RecordCheckpoint(Checkpoint{URI: "", Step: 200}); err == nil {
		t.Fatal("invalid checkpoint must be rejected")
	}
}

func TestTrainingJobRecordCheckpointRequiresPolicy(t *testing.T) {
	j := newJob()
	j.Normalize()
	if err := j.RecordCheckpoint(Checkpoint{URI: "s3://b/ck", Step: 1}); err == nil {
		t.Fatal("checkpoint upload without an enabled policy must be rejected")
	}
}

func TestTrainingJobHandleFailureResumesFromCheckpoint(t *testing.T) {
	j := newJob()
	j.Checkpointing = CheckpointPolicy{Enabled: true, IntervalSteps: 100, MaxRetries: 2}
	j.Normalize()
	j.MarkRunning(time.Now())
	_ = j.RecordCheckpoint(Checkpoint{URI: "s3://b/ck-100", Step: 100})

	if got := j.HandleFailure("OOM"); got != OutcomeRetry {
		t.Fatalf("expected retry, got %v", got)
	}
	if j.Status != StatusRetrying {
		t.Fatalf("status = %q, want retrying", j.Status)
	}
	if j.ResumeFrom != "s3://b/ck-100" {
		t.Fatalf("ResumeFrom = %q, want the latest checkpoint", j.ResumeFrom)
	}
	if j.Attempts != 1 {
		t.Fatalf("Attempts = %d, want 1", j.Attempts)
	}
	if j.IsTerminal() {
		t.Fatal("retrying job still holds its quota and must not be terminal")
	}
}

func TestTrainingJobHandleFailureExhaustsRetryBudget(t *testing.T) {
	j := newJob()
	j.Checkpointing = CheckpointPolicy{Enabled: true, IntervalSteps: 100, MaxRetries: 1}
	j.Normalize()
	j.MarkRunning(time.Now())
	_ = j.RecordCheckpoint(Checkpoint{URI: "s3://b/ck-100", Step: 100})

	if got := j.HandleFailure("OOM"); got != OutcomeRetry {
		t.Fatalf("first failure should retry, got %v", got)
	}
	j.PrepareRerun()
	j.MarkRunning(time.Now())

	if got := j.HandleFailure("OOM again"); got != OutcomeFailed {
		t.Fatalf("budget exhausted, expected terminal failure, got %v", got)
	}
	if j.Status != StatusFailed || j.FailureReason != "OOM again" {
		t.Fatalf("unexpected terminal state: %q / %q", j.Status, j.FailureReason)
	}
}

func TestTrainingJobHandleFailureWithoutCheckpointIsTerminal(t *testing.T) {
	j := newJob()
	j.Checkpointing = CheckpointPolicy{Enabled: true, IntervalSteps: 100, MaxRetries: 3}
	j.Normalize()
	j.MarkRunning(time.Now())

	if got := j.HandleFailure("crashed before first checkpoint"); got != OutcomeFailed {
		t.Fatalf("expected terminal failure, got %v", got)
	}
}

func TestTrainingJobPrepareRerunRequiresRetryingState(t *testing.T) {
	j := newJob()
	j.Normalize()
	if j.PrepareRerun() {
		t.Fatal("only a retrying job can be prepared for rerun")
	}
}

func TestTrainingJobTimedOut(t *testing.T) {
	start := time.Now().Add(-2 * time.Hour)
	j := newJob()
	j.Spec.MaxRuntime = 60
	j.Normalize()
	j.MarkRunning(start)

	if !j.TimedOut(time.Now()) {
		t.Fatal("job running past max_runtime must be reported as timed out")
	}

	j.Spec.MaxRuntime = 0
	if j.TimedOut(time.Now()) {
		t.Fatal("max_runtime = 0 means no limit")
	}
}

func TestTrainingJobShouldRegisterModel(t *testing.T) {
	j := newJob()
	j.Checkpointing = CheckpointPolicy{Enabled: true, IntervalSteps: 100, MaxRetries: 1}
	j.Registration = ModelRegistration{Enabled: true, ModelID: "m1", VersionTag: "v1"}
	j.Normalize()
	j.MarkRunning(time.Now())
	_ = j.RecordCheckpoint(Checkpoint{URI: "s3://b/final", Step: 500})

	if j.ShouldRegisterModel() {
		t.Fatal("model must not be registered before the job succeeds")
	}
	j.MarkCompleted()
	if !j.ShouldRegisterModel() {
		t.Fatal("succeeded job with registration enabled must register its model")
	}

	j.LatestCheckpoint = nil
	if j.ShouldRegisterModel() {
		t.Fatal("registration requires an artifact")
	}
}