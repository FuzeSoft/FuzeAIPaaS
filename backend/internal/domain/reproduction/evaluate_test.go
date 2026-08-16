package reproduction

import (
	"math"
	"testing"
)

func approx(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestEvaluateAbsSatisfied(t *testing.T) {
	cfg := Config{AbsTol: DefaultAbsTol, RelTol: DefaultRelTol}
	r := Evaluate(cfg, "acc", "s1", "r1", 0.91, 0.92)
	if !r.Reproducible {
		t.Fatalf("expected reproducible (abs 0.01 <= %v), got %+v", DefaultAbsTol, r)
	}
	if !approx(r.AbsDeviation, 0.01) {
		t.Errorf("abs deviation = %v, want 0.01", r.AbsDeviation)
	}
}

func TestEvaluateRelSatisfiedAbsNot(t *testing.T) {
	cfg := Config{AbsTol: DefaultAbsTol, RelTol: DefaultRelTol}
	
	r := Evaluate(cfg, "acc", "s1", "r1", 1.0, 1.04)
	if !r.Reproducible {
		t.Fatalf("expected reproducible via rel tolerance, got %+v", r)
	}
	if !approx(r.RelDeviation, 0.04) {
		t.Errorf("rel deviation = %v, want 0.04", r.RelDeviation)
	}
}

func TestEvaluateBothFail(t *testing.T) {
	cfg := Config{AbsTol: DefaultAbsTol, RelTol: DefaultRelTol}
	r := Evaluate(cfg, "acc", "s1", "r1", 1.0, 1.10)
	if r.Reproducible {
		t.Fatalf("expected not reproducible, got %+v", r)
	}
}

func TestEvaluateSourceZero(t *testing.T) {
	cfg := Config{AbsTol: DefaultAbsTol, RelTol: DefaultRelTol}
	
	if !Evaluate(cfg, "m", "s", "r", 0, 0.02).Reproducible {
		t.Errorf("source=0 repro=0.02: rel devolved to 0 <= RelTol -> expected reproducible")
	}
	if !Evaluate(cfg, "m", "s", "r", 0, 0.005).Reproducible {
		t.Errorf("source=0 repro=0.005: expected reproducible (abs 0.005 <= 0.01)")
	}
}

func TestEvaluateNegativeSymmetric(t *testing.T) {
	cfg := Config{AbsTol: DefaultAbsTol, RelTol: DefaultRelTol}
	r := Evaluate(cfg, "loss", "s", "r", -0.5, -0.51)
	if !r.Reproducible {
		t.Fatalf("expected reproducible for negative direction, got %+v", r)
	}
	if !approx(r.AbsDeviation, 0.01) {
		t.Errorf("abs deviation = %v, want 0.01", r.AbsDeviation)
	}
}

func TestDefaults(t *testing.T) {
	if DefaultAbsTol != 0.01 {
		t.Errorf("DefaultAbsTol = %v, want 0.01", DefaultAbsTol)
	}
	if DefaultRelTol != 0.05 {
		t.Errorf("DefaultRelTol = %v, want 0.05", DefaultRelTol)
	}
}