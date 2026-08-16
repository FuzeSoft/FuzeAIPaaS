package llm

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

const (
	
	StrategyPriority = "priority"
	
	StrategyRoundRobin = "round_robin"
	
	StrategyWeighted = "weighted"
)

var (
	
	ErrNoBackend = errors.New("llm: no backend configured for model")
	
	ErrNoHealthyBackend = errors.New("llm: no healthy backend available")
	
	ErrModelNotFound = errors.New("llm: model not found in route table")
)

type Backend struct {
	
	Name string `json:"name"`
	
	Endpoint string `json:"endpoint"`
	
	Weight int `json:"weight"`
	
	Healthy bool `json:"healthy"`
	
	ServiceID string `json:"service_id,omitempty"`
}

type Route struct {
	
	Model string `json:"model"`
	
	Strategy string `json:"strategy"`
	
	Backends []Backend `json:"backends"`
}

type RouteTable struct {
	mu     sync.RWMutex
	routes map[string]*Route
	
	cursor map[string]int
}

func NewRouteTable() *RouteTable {
	return &RouteTable{
		routes: make(map[string]*Route),
		cursor: make(map[string]int),
	}
}

func (t *RouteTable) Upsert(r Route) error {
	if strings.TrimSpace(r.Model) == "" {
		return ErrEmptyModel
	}
	if len(r.Backends) == 0 {
		return ErrNoBackend
	}
	if r.Strategy == "" {
		r.Strategy = StrategyPriority
	}
	
	backends := make([]Backend, len(r.Backends))
	copy(backends, r.Backends)
	r.Backends = backends

	t.mu.Lock()
	defer t.mu.Unlock()
	t.routes[r.Model] = &r
	return nil
}

func (t *RouteTable) Remove(model string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.routes, model)
	delete(t.cursor, model)
}

func (t *RouteTable) Get(model string) (Route, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	r, ok := t.routes[model]
	if !ok {
		return Route{}, false
	}
	return cloneRoute(r), true
}

func (t *RouteTable) List() []Route {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Route, 0, len(t.routes))
	for _, r := range t.routes {
		out = append(out, cloneRoute(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out
}

func (t *RouteTable) Models() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]string, 0, len(t.routes))
	for m := range t.routes {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

func (t *RouteTable) SetHealth(model, backend string, healthy bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	r, ok := t.routes[model]
	if !ok {
		return false
	}
	for i := range r.Backends {
		if r.Backends[i].Name == backend {
			r.Backends[i].Healthy = healthy
			return true
		}
	}
	return false
}

func (t *RouteTable) Pick(model string) ([]Backend, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r, ok := t.routes[model]
	if !ok {
		return nil, ErrModelNotFound
	}
	return t.pickLocked(r)
}

func (t *RouteTable) PickForTenant(_, model string) ([]Backend, error) {
	return t.Pick(model)
}

func (t *RouteTable) pickLocked(r *Route) ([]Backend, error) {
	healthy := make([]Backend, 0, len(r.Backends))
	for _, b := range r.Backends {
		if b.Healthy {
			healthy = append(healthy, b)
		}
	}
	if len(healthy) == 0 {
		return nil, ErrNoHealthyBackend
	}

	switch r.Strategy {
	case StrategyRoundRobin:
		
		n := len(healthy)
		start := t.cursor[r.Model] % n
		t.cursor[r.Model] = (start + 1) % n
		rotated := make([]Backend, 0, n)
		rotated = append(rotated, healthy[start:]...)
		rotated = append(rotated, healthy[:start]...)
		return rotated, nil
	case StrategyWeighted:
		
		sort.SliceStable(healthy, func(i, j int) bool {
			if healthy[i].Weight != healthy[j].Weight {
				return healthy[i].Weight > healthy[j].Weight
			}
			return healthy[i].Name < healthy[j].Name
		})
		return healthy, nil
	default: 
		sort.SliceStable(healthy, func(i, j int) bool {
			if healthy[i].Weight != healthy[j].Weight {
				return healthy[i].Weight > healthy[j].Weight
			}
			return healthy[i].Name < healthy[j].Name
		})
		return healthy, nil
	}
}

func cloneRoute(r *Route) Route {
	out := *r
	out.Backends = make([]Backend, len(r.Backends))
	copy(out.Backends, r.Backends)
	return out
}