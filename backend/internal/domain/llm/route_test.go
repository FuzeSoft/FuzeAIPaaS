package llm

import (
	"sync"
	"testing"
)

func healthyRoute() Route {
	return Route{
		Model:    "qwen",
		Strategy: StrategyPriority,
		Backends: []Backend{
			{Name: "a", Endpoint: "http://a", Weight: 10, Healthy: true},
			{Name: "b", Endpoint: "http://b", Weight: 20, Healthy: true},
		},
	}
}

func TestRouteTableUpsertValidation(t *testing.T) {
	tbl := NewRouteTable()
	if err := tbl.Upsert(Route{Backends: []Backend{{Name: "a"}}}); err != ErrEmptyModel {
		t.Fatalf("want ErrEmptyModel, got %v", err)
	}
	if err := tbl.Upsert(Route{Model: "m"}); err != ErrNoBackend {
		t.Fatalf("want ErrNoBackend, got %v", err)
	}
}

func TestRouteTableUpsertDefaultsStrategy(t *testing.T) {
	tbl := NewRouteTable()
	if err := tbl.Upsert(Route{Model: "m", Backends: []Backend{{Name: "a", Healthy: true}}}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, ok := tbl.Get("m")
	if !ok {
		t.Fatal("route not found")
	}
	if got.Strategy != StrategyPriority {
		t.Fatalf("Strategy = %q, want %q", got.Strategy, StrategyPriority)
	}
}

func TestRouteTableIsolatesCallerSlice(t *testing.T) {
	tbl := NewRouteTable()
	backends := []Backend{{Name: "a", Endpoint: "http://a", Healthy: true}}
	if err := tbl.Upsert(Route{Model: "m", Backends: backends}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	backends[0].Endpoint = "http://evil"

	got, _ := tbl.Get("m")
	if got.Backends[0].Endpoint != "http://a" {
		t.Fatalf("route table aliased caller slice: %q", got.Backends[0].Endpoint)
	}

	got.Backends[0].Endpoint = "http://tampered"
	again, _ := tbl.Get("m")
	if again.Backends[0].Endpoint != "http://a" {
		t.Fatalf("Get returned aliased slice: %q", again.Backends[0].Endpoint)
	}
}

func TestRouteTablePickErrors(t *testing.T) {
	tbl := NewRouteTable()
	if _, err := tbl.Pick("missing"); err != ErrModelNotFound {
		t.Fatalf("want ErrModelNotFound, got %v", err)
	}

	_ = tbl.Upsert(Route{Model: "m", Backends: []Backend{{Name: "a", Healthy: false}}})
	if _, err := tbl.Pick("m"); err != ErrNoHealthyBackend {
		t.Fatalf("want ErrNoHealthyBackend, got %v", err)
	}
}

func TestRouteTablePickPriority(t *testing.T) {
	tbl := NewRouteTable()
	r := healthyRoute()
	r.Backends = append(r.Backends, Backend{Name: "dead", Weight: 99, Healthy: false})
	_ = tbl.Upsert(r)

	got, err := tbl.Pick("qwen")
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (unhealthy must be excluded)", len(got))
	}
	if got[0].Name != "b" {
		t.Fatalf("first = %q, want %q (higher weight)", got[0].Name, "b")
	}
}

func TestRouteTablePickRoundRobinRotates(t *testing.T) {
	tbl := NewRouteTable()
	r := healthyRoute()
	r.Strategy = StrategyRoundRobin
	_ = tbl.Upsert(r)

	first := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		got, err := tbl.Pick("qwen")
		if err != nil {
			t.Fatalf("Pick: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2 (full failover order)", len(got))
		}
		first = append(first, got[0].Name)
	}
	if first[0] == first[1] {
		t.Fatalf("round robin did not rotate: %v", first)
	}
	if first[0] != first[2] || first[1] != first[3] {
		t.Fatalf("round robin not cyclic: %v", first)
	}
}

func TestRouteTableSetHealth(t *testing.T) {
	tbl := NewRouteTable()
	_ = tbl.Upsert(healthyRoute())

	if ok := tbl.SetHealth("qwen", "b", false); !ok {
		t.Fatal("SetHealth returned false for existing backend")
	}
	got, _ := tbl.Pick("qwen")
	if len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("unhealthy backend still routed: %+v", got)
	}

	if ok := tbl.SetHealth("qwen", "ghost", true); ok {
		t.Fatal("SetHealth returned true for unknown backend")
	}
	if ok := tbl.SetHealth("ghost", "a", true); ok {
		t.Fatal("SetHealth returned true for unknown model")
	}
}

func TestRouteTableRemoveAndList(t *testing.T) {
	tbl := NewRouteTable()
	_ = tbl.Upsert(Route{Model: "z", Backends: []Backend{{Name: "a", Healthy: true}}})
	_ = tbl.Upsert(Route{Model: "a", Backends: []Backend{{Name: "a", Healthy: true}}})

	if got := tbl.Models(); len(got) != 2 || got[0] != "a" || got[1] != "z" {
		t.Fatalf("Models() = %v, want [a z]", got)
	}
	if got := tbl.List(); got[0].Model != "a" {
		t.Fatalf("List() not sorted: %v", got)
	}

	tbl.Remove("z")
	if _, ok := tbl.Get("z"); ok {
		t.Fatal("route still present after Remove")
	}
}

func TestRouteTableConcurrentAccess(t *testing.T) {
	tbl := NewRouteTable()
	_ = tbl.Upsert(healthyRoute())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); _, _ = tbl.Pick("qwen") }()
		go func() { defer wg.Done(); tbl.SetHealth("qwen", "a", true) }()
		go func() { defer wg.Done(); _ = tbl.List() }()
	}
	wg.Wait()
}