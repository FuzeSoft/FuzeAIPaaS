package hpo

import "testing"

func TestEvaluateASHAStop(t *testing.T) {
	study := &Study{
		Objective: Objective{MetricName: "acc", Direction: DirectionMaximize},
		EarlyStop: &EarlyStopSpec{Enabled: true, Eta: 3, MinRungs: 1},
	}
	
	me := trial("t2", 0.2)
	all := []Trial{
		{ID: "t0", Status: TrialRunning, Intermediate: []Report{{Step: 3, Value: 0.9}}},
		{ID: "t1", Status: TrialRunning, Intermediate: []Report{{Step: 3, Value: 0.8}}},
		me,
	}
	mePtr := &all[2]
	dec := EvaluateASHA(study, mePtr, all)
	if !dec.ShouldStop {
		t.Fatalf("expected prune for worst trial, got %+v", dec)
	}
}

func TestEvaluateASHANotStopWhenGood(t *testing.T) {
	study := &Study{
		Objective: Objective{MetricName: "acc", Direction: DirectionMaximize},
		EarlyStop: &EarlyStopSpec{Enabled: true, Eta: 3, MinRungs: 1},
	}
	me := trial("t2", 0.95) 
	all := []Trial{
		trial("t0", 0.2),
		trial("t1", 0.3),
		me,
	}
	mePtr := &all[2]
	dec := EvaluateASHA(study, mePtr, all)
	if dec.ShouldStop {
		t.Fatalf("best trial must not be pruned: %+v", dec)
	}
}

func TestEvaluateASHADisabled(t *testing.T) {
	study := &Study{EarlyStop: &EarlyStopSpec{Enabled: false}}
	me := trial("t", 0.1)
	dec := EvaluateASHA(study, &me, []Trial{me})
	if dec.ShouldStop {
		t.Fatal("disabled early stop must never prune")
	}
}

func TestEvaluateASHABelowMinRungs(t *testing.T) {
	study := &Study{
		Objective: Objective{MetricName: "acc", Direction: DirectionMaximize},
		EarlyStop: &EarlyStopSpec{Enabled: true, Eta: 3, MinRungs: 2},
	}
	
	me := trial("t", 0.1)
	me.Intermediate = []Report{{Step: 1, Value: 0.1}}
	all := []Trial{
		{ID: "o", Status: TrialRunning, Intermediate: []Report{{Step: 1, Value: 0.9}}},
		me,
	}
	dec := EvaluateASHA(study, &me, all)
	if dec.ShouldStop {
		t.Fatal("below min rungs must not prune")
	}
}

func trial(id string, v float64) Trial {
	return Trial{
		ID:          id,
		Status:      TrialRunning,
		Intermediate: []Report{{Step: 3, Value: v}},
	}
}