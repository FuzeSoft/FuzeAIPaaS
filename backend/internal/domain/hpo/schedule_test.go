package hpo

import (
	"math/rand"
	"testing"
	"time"
)

func baseStudy() *Study {
	return &Study{
		ID:        "s1",
		Status:    StudyRunning,
		Algorithm: "random",
		Objective: Objective{MetricName: "acc", Direction: DirectionMaximize},
		Space: SearchSpace{Params: []ParamSpec{
			{Name: "lr", Type: ParamFloat, Min: 1e-4, Max: 1e-1, LogScale: true},
		}},
		MaxTrials:   10,
		MaxParallel: 2,
		TrainingTemplate: map[string]any{"image": "train:latest"},
		CreatedAt:  time.Now().Add(-time.Hour),
	}
}

func ctx() SchedulingContext {
	return SchedulingContext{QuotaAvailable: true, Now: time.Now(), Rand: rand.New(rand.NewSource(1))}
}

func TestNextActionsSpawn(t *testing.T) {
	s := baseStudy()
	acts := NextActions(s, nil, ctx())
	if len(acts) != 1 || acts[0].Kind != ActionSpawnTrial {
		t.Fatalf("expected spawn, got %+v", acts)
	}
	if acts[0].Params["lr"] == nil {
		t.Fatal("spawn params missing lr")
	}
}

func TestNextActionsParallelSaturated(t *testing.T) {
	s := baseStudy() 
	trials := []Trial{
		{ID: "a", Status: TrialRunning},
		{ID: "b", Status: TrialRunning},
	}
	acts := NextActions(s, trials, ctx())
	if len(acts) != 1 || acts[0].Kind != ActionNoop || acts[0].Reason != "parallelism saturated" {
		t.Fatalf("expected saturated noop, got %+v", acts)
	}
}

func TestNextActionsMaxTrials(t *testing.T) {
	s := baseStudy() 
	trials := make([]Trial, 10)
	for i := range trials {
		trials[i] = Trial{ID: "t", Status: TrialCompleted, Value: fptr(0.5)}
	}
	acts := NextActions(s, trials, ctx())
	if len(acts) != 1 || acts[0].Kind != ActionCompleteStudy {
		t.Fatalf("expected complete, got %+v", acts)
	}
}

func TestNextActionsNoQuota(t *testing.T) {
	s := baseStudy()
	c := ctx()
	c.QuotaAvailable = false
	acts := NextActions(s, nil, c)
	if acts[0].Kind != ActionNoop || acts[0].Reason != "quota unavailable" {
		t.Fatalf("expected quota noop, got %+v", acts)
	}
}

func TestNextActionsTimeoutStopsAndCompletes(t *testing.T) {
	s := baseStudy()
	s.MaxDuration = time.Minute
	trials := []Trial{{ID: "a", Status: TrialRunning}}
	c := ctx()
	c.Now = s.CreatedAt.Add(2 * time.Minute)
	acts := NextActions(s, trials, c)
	
	var stopped, completed bool
	for _, a := range acts {
		if a.Kind == ActionStopTrial && a.TrialID == "a" {
			stopped = true
		}
		if a.Kind == ActionCompleteStudy {
			completed = true
		}
	}
	if !stopped || !completed {
		t.Fatalf("timeout should stop+complete, got %+v", acts)
	}
}

func TestNextActionsPruneWhenOverParallel(t *testing.T) {
	s := baseStudy() 
	trials := []Trial{
		{ID: "good", Status: TrialRunning, Intermediate: []Report{{Step: 5, Value: 0.9}}},
		{ID: "mid", Status: TrialRunning, Intermediate: []Report{{Step: 5, Value: 0.6}}},
		{ID: "bad", Status: TrialRunning, Intermediate: []Report{{Step: 5, Value: 0.1}}}, 
	}
	s.EarlyStop = &EarlyStopSpec{Enabled: true, Eta: 3, MinRungs: 1}
	acts := NextActions(s, trials, ctx())
	if len(acts) != 1 || acts[0].Kind != ActionStopTrial || acts[0].TrialID != "bad" {
		t.Fatalf("expected prune worst 'bad', got %+v", acts)
	}
}

func TestNextActionsTerminal(t *testing.T) {
	s := baseStudy()
	s.Status = StudyCompleted
	acts := NextActions(s, nil, ctx())
	if acts[0].Kind != ActionNoop {
		t.Fatalf("terminal study must noop, got %+v", acts)
	}
}

func TestBestTrial(t *testing.T) {
	trials := []Trial{
		{ID: "a", Status: TrialCompleted, Value: fptr(0.5)},
		{ID: "b", Status: TrialCompleted, Value: fptr(0.9)},
		{ID: "c", Status: TrialFailed, Value: fptr(0.99)},  
		{ID: "d", Status: TrialCompleted, Value: nil},      
	}
	best := BestTrial(Objective{MetricName: "acc", Direction: DirectionMaximize}, trials)
	if best == nil || best.ID != "b" {
		t.Fatalf("expected best=b, got %+v", best)
	}
}

func TestNewSamplerUnknown(t *testing.T) {
	if _, err := NewSampler("hyperband", Objective{}); err == nil {
		t.Fatal("expected error for unknown algorithm")
	}
}