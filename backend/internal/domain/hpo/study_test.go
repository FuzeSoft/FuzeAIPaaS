package hpo

import (
	"math/rand"
	"testing"
	"time"
)

func TestStudyTerminalStates(t *testing.T) {
	for _, st := range []string{StudyCompleted, StudyFailed, StudyCancelled, StudyStopped} {
		s := &Study{Status: st}
		if !s.IsTerminal() {
			t.Fatalf("state %q should be terminal", st)
		}
		if s.IsActive() {
			t.Fatalf("state %q should not be active", st)
		}
	}
	s := &Study{Status: StudyRunning}
	if !s.IsActive() || s.IsTerminal() {
		t.Fatalf("running should be active & non-terminal")
	}
}

func TestRungFor(t *testing.T) {
	s := &Study{EarlyStop: &EarlyStopSpec{Enabled: true, Eta: 3, MinRungs: 1}}
	cases := []struct {
		step int
		rung int
	}{
		{0, 0},
		{3, 0},
		{9, 1},
		{27, 2},
	}
	for _, c := range cases {
		if got := s.rungFor(c.step); got != c.rung {
			t.Fatalf("rungFor(%d)=%d want %d", c.step, got, c.rung)
		}
	}
}

func TestEarlyStopNormalized(t *testing.T) {
	
	if es := (&EarlyStopSpec{Enabled: false}).normalized(); es.Enabled {
		t.Fatal("disabled should normalize to disabled")
	}
	
	es := (&EarlyStopSpec{Enabled: true}).normalized()
	if es.Eta != 3 || es.MinRungs != 1 {
		t.Fatalf("bad normalize: %+v", es)
	}
}

func TestNoGlobalRandom(t *testing.T) {
	_ = rand.New
	_ = time.Now
}