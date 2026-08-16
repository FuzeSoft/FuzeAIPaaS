package hpo

import (
	"encoding/json"
	"math"
	"math/rand"
	"testing"
)

func testSpace() SearchSpace {
	return SearchSpace{Params: []ParamSpec{
		{Name: "lr", Type: ParamFloat, Min: 1e-4, Max: 1e-1, LogScale: true},
		{Name: "layers", Type: ParamInt, Min: 1, Max: 5},
		{Name: "opt", Type: ParamCategorical, Choices: []any{"sgd", "adam", "rmsprop"}},
		{Name: "bias", Type: ParamBool},
	}}
}

func TestRandomSamplerDeterministic(t *testing.T) {
	space := testSpace()
	a := mustSuggest(t, RandomSampler{}, space, nil, rand.New(rand.NewSource(42)))
	b := mustSuggest(t, RandomSampler{}, space, nil, rand.New(rand.NewSource(42)))
	for k, v := range a {
		if v != b[k] {
			t.Fatalf("same seed produced different %q: %v vs %v", k, v, b[k])
		}
	}
}

func TestRandomSamplerInRange(t *testing.T) {
	space := testSpace()
	r := rand.New(rand.NewSource(7))
	for i := 0; i < 500; i++ {
		p, err := RandomSampler{}.Suggest(space, nil, r)
		if err != nil {
			t.Fatal(err)
		}
		lr := p["lr"].(float64)
		if lr < 1e-4 || lr > 1e-1 {
			t.Fatalf("lr out of range: %v", lr)
		}
		
		if layers, ok := p["layers"].(int); !ok || layers < 1 || layers > 5 {
			t.Fatalf("layers invalid: %v", p["layers"])
		}
		
		opt := p["opt"].(string)
		if opt != "sgd" && opt != "adam" && opt != "rmsprop" {
			t.Fatalf("opt out of choices: %v", opt)
		}
		if _, ok := p["bias"].(bool); !ok {
			t.Fatalf("bias not bool: %v", p["bias"])
		}
	}
}

func TestRandomSamplerLogScale(t *testing.T) {
	
	space := SearchSpace{Params: []ParamSpec{{Name: "lr", Type: ParamFloat, Min: 1e-5, Max: 1e-1, LogScale: true}}}
	r := rand.New(rand.NewSource(1))
	buckets := make([]int, 4) 
	for i := 0; i < 4000; i++ {
		p, err := RandomSampler{}.Suggest(space, nil, r)
		if err != nil {
			t.Fatal(err)
		}
		lr := p["lr"].(float64)
		switch {
		case lr < 1e-4:
			buckets[0]++
		case lr < 1e-3:
			buckets[1]++
		case lr < 1e-2:
			buckets[2]++
		default:
			buckets[3]++
		}
	}
	
	for i, c := range buckets {
		if c == 0 {
			t.Fatalf("log-scale bucket %d is empty (uniform sampling broken): %v", i, buckets)
		}
	}
}

func TestGridSamplerExhaustive(t *testing.T) {
	space := SearchSpace{Params: []ParamSpec{
		{Name: "a", Type: ParamInt, Min: 0, Max: 2}, 
		{Name: "b", Type: ParamCategorical, Choices: []any{"x", "y"}}, 
	}}
	total := totalCombinations(space)
	if total != 6 {
		t.Fatalf("expected 6 combos, got %d", total)
	}
	seen := make(map[string]struct{})
	r := rand.New(rand.NewSource(0))
	for i := 0; i < total; i++ {
		hist := make([]Trial, i) 
		p, err := GridSampler{}.Suggest(space, hist, r)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		seen[formatMap(p)] = struct{}{}
	}
	if len(seen) != total {
		t.Fatalf("grid produced %d unique combos, want %d", len(seen), total)
	}
	
	histFull := make([]Trial, total)
	if _, e := (GridSampler{}).Suggest(space, histFull, r); e == nil {
		t.Fatal("expected ErrExhausted after exhausting grid")
	}
}

func TestGridSamplerStep(t *testing.T) {
	space := SearchSpace{Params: []ParamSpec{{Name: "x", Type: ParamFloat, Min: 0, Max: 1, Step: 0.5}}}
	if totalCombinations(space) != 3 { 
		t.Fatalf("expected 3 stepped points, got %d", totalCombinations(space))
	}
}

func TestTPESamplerFallback(t *testing.T) {
	
	space := testSpace()
	r := rand.New(rand.NewSource(3))
	p, err := TPESampler{Objective: Objective{MetricName: "acc", Direction: DirectionMaximize}}.Suggest(space, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p["lr"].(float64); !ok {
		t.Fatalf("tpe fallback produced bad lr: %v", p["lr"])
	}
}

func TestTPESamplerConvergesNotWorseThanRandom(t *testing.T) {
	
	best := func(sampler Sampler, seed int64) float64 {
		space := SearchSpace{Params: []ParamSpec{{Name: "x", Type: ParamFloat, Min: -1, Max: 1}}}
		obj := Objective{MetricName: "y", Direction: DirectionMaximize}
		
		hist := make([]Trial, 0, 40)
		r := rand.New(rand.NewSource(seed))
		best := math.Inf(-1)
		for i := 0; i < 40; i++ {
			p, err := sampler.Suggest(space, hist, r)
			if err != nil {
				t.Fatal(err)
			}
			x := p["x"].(float64)
			y := -math.Abs(x)
			v := y
			hist = append(hist, Trial{Params: p, Value: &v})
			if y > best {
				best = y
			}
		}
		_ = obj
		return best
	}
	
	var tpeSum, randSum float64
	const runs = 20
	for s := int64(0); s < runs; s++ {
		tpeSum += best(TPESampler{Objective: Objective{MetricName: "y", Direction: DirectionMaximize}}, s)
		randSum += best(RandomSampler{}, s+1000)
	}
	tpeAvg, randAvg := tpeSum/float64(runs), randSum/float64(runs)
	if tpeAvg < randAvg-0.02 { 
		t.Fatalf("TPE (%v) worse than Random (%v) — convergence regression", tpeAvg, randAvg)
	}
}

func mustSuggest(t *testing.T, s Sampler, space SearchSpace, hist []Trial, r *rand.Rand) map[string]any {
	t.Helper()
	p, err := s.Suggest(space, hist, r)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	return p
}

func formatMap(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}