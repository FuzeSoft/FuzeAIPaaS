package job

import "testing"

func TestRetryingIsNotTerminal(t *testing.T) {
	if IsTerminal(JobStatusRetrying) {
		t.Fatal("retrying must not be terminal — the job still holds its quota")
	}
	if !IsActive(JobStatusRetrying) {
		t.Fatal("retrying must count as active")
	}
}

func TestRetryingTransitions(t *testing.T) {
	allowed := [][2]JobStatus{
		{JobStatusRunning, JobStatusRetrying},
		{JobStatusRetrying, JobStatusPending},
		{JobStatusRetrying, JobStatusRunning},
		{JobStatusRetrying, JobStatusFailed},
		{JobStatusRetrying, JobStatusCancelled},
	}
	for _, e := range allowed {
		if !CanTransition(e[0], e[1]) {
			t.Fatalf("%s -> %s must be allowed", e[0], e[1])
		}
	}

	for _, from := range []JobStatus{JobStatusCompleted, JobStatusFailed, JobStatusCancelled} {
		if CanTransition(from, JobStatusRetrying) {
			t.Fatalf("%s -> retrying must be rejected", from)
		}
	}
	
	if CanTransition(JobStatusPending, JobStatusRetrying) {
		t.Fatal("pending -> retrying must be rejected")
	}
}