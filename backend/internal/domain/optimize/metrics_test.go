package optimize

import (
	"testing"
)

func TestComputeCompressionRatio(t *testing.T) {
	r, err := ComputeCompressionRatio(1000, 250)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r != 4.0 {
		t.Fatalf("expected ratio 4.0, got %v", r)
	}
	
	if _, err := ComputeCompressionRatio(1000, 0); err == nil {
		t.Fatal("expected error for zero compressed size")
	}
	if _, err := ComputeCompressionRatio(1000, -1); err == nil {
		t.Fatal("expected error for negative compressed size")
	}
}

func TestComputeSpeedup(t *testing.T) {
	s, err := ComputeSpeedup(200.0, 50.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != 4.0 {
		t.Fatalf("expected speedup 4.0, got %v", s)
	}
	if _, err := ComputeSpeedup(200.0, 0); err == nil {
		t.Fatal("expected error for zero latency")
	}
	if _, err := ComputeSpeedup(200.0, -1); err == nil {
		t.Fatal("expected error for negative latency")
	}
}