package optimize

import (
	"math"
	"testing"
)

func TestEvaluateGatePass(t *testing.T) {
	v := EvaluateGate(0.95, 0.94, 0.01)
	if !v.Pass {
		t.Fatalf("expected pass (drop 0.01 <= 0.01), got %+v", v)
	}
	if math.Abs(v.AccDelta-0.01) > 1e-9 {
		t.Fatalf("acc delta should be 0.01, got %v", v.AccDelta)
	}
}

func TestEvaluateGateBoundaryPass(t *testing.T) {
	
	v := EvaluateGate(0.90, 0.89, 0.01)
	if !v.Pass {
		t.Fatalf("boundary (drop == threshold) should pass, got %+v", v)
	}
}

func TestEvaluateGateFail(t *testing.T) {
	v := EvaluateGate(0.95, 0.90, 0.01)
	if v.Pass {
		t.Fatalf("expected fail (drop 0.05 > 0.01), got %+v", v)
	}
	if v.Reason == "" {
		t.Fatal("failed verdict should carry a reason")
	}
}

func TestEvaluateGateImprovementPasses(t *testing.T) {
	v := EvaluateGate(0.90, 0.92, 0.01)
	if !v.Pass {
		t.Fatalf("accuracy improvement should pass, got %+v", v)
	}
}