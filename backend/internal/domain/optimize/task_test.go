package optimize

import "testing"

func TestNewCompressionTaskDefaultPending(t *testing.T) {
	task := NewCompressionTask("t1", "tenant1", "q", CompressionTypeQuantize, BackendPyTorch, "{}", "mv1")
	if task.Status != StatusPending {
		t.Fatalf("new task should default to pending, got %q", task.Status)
	}
}

func TestTransitionLegal(t *testing.T) {
	task := NewCompressionTask("t1", "tenant1", "q", CompressionTypeQuantize, BackendPyTorch, "{}", "mv1")
	if !task.CanTransitionTo(StatusRunning) {
		t.Fatal("pending -> running should be legal")
	}
	if err := task.TransitionTo(StatusRunning); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Status != StatusRunning {
		t.Fatalf("status should be running, got %q", task.Status)
	}
	if !task.CanTransitionTo(StatusSucceeded) {
		t.Fatal("running -> succeeded should be legal")
	}
	if err := task.TransitionTo(StatusSucceeded); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTransitionIllegal(t *testing.T) {
	task := NewCompressionTask("t1", "tenant1", "q", CompressionTypeQuantize, BackendPyTorch, "{}", "mv1")
	
	if task.CanTransitionTo(StatusSucceeded) {
		t.Fatal("pending -> succeeded should be illegal")
	}
	if err := task.TransitionTo(StatusSucceeded); err == nil {
		t.Fatal("expected error for illegal transition")
	}
	
	_ = task.TransitionTo(StatusRunning)
	if task.CanTransitionTo(StatusPending) {
		t.Fatal("running -> pending should be illegal")
	}
	if err := task.TransitionTo(StatusPending); err == nil {
		t.Fatal("expected error for illegal reverse transition")
	}
}

func TestTransitionCancel(t *testing.T) {
	task := NewCompressionTask("t1", "tenant1", "q", CompressionTypeQuantize, BackendPyTorch, "{}", "mv1")
	if err := task.TransitionTo(StatusCancelled); err != nil {
		t.Fatalf("pending -> cancelled should be legal: %v", err)
	}
	
	if task.CanTransitionTo(StatusRunning) {
		t.Fatal("cancelled -> running should be illegal")
	}
}

func TestTransitionSelfLoopIllegal(t *testing.T) {
	task := NewCompressionTask("t1", "tenant1", "q", CompressionTypeQuantize, BackendPyTorch, "{}", "mv1")
	if task.CanTransitionTo(StatusPending) {
		t.Fatal("self transition should be illegal")
	}
}