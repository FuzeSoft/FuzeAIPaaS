package llmgateway

import (
	"context"
	"math/rand"
	"sort"
	"sync"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/ports"
)

const fallbackTenant = "default"

type repoRouteTable struct {
	repo ports.RouteRepository

	mu      sync.RWMutex
	healthy map[string]bool 
	cursor  map[string]int  
}

func NewRepoRouteTable(repo ports.RouteRepository) RouteTable {
	return &repoRouteTable{
		repo:    repo,
		healthy: make(map[string]bool),
		cursor:  make(map[string]int),
	}
}

func backendKey(model, name string) string { return model + "/" + name }

func (t *repoRouteTable) SetHealth(model, backend string, healthy bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := backendKey(model, backend)
	if _, ok := t.healthy[key]; !ok {
		
		return false
	}
	t.healthy[key] = healthy
	return true
}

func (t *repoRouteTable) PickForTenant(tenantID, model string) ([]llm.Backend, error) {
	r := t.lookup(tenantID, model)
	if r == nil {
		return nil, llm.ErrModelNotFound
	}

	t.mu.Lock()
	healthy := make([]llm.Backend, 0, len(r.Backends))
	for _, b := range r.Backends {
		key := backendKey(model, b.Name)
		h, seen := t.healthy[key]
		if seen && !h {
			continue 
		}
		if !seen {
			
			t.healthy[key] = true
		}
		healthy = append(healthy, llm.Backend{
			Name:      b.Name,
			Endpoint:  b.Endpoint,
			Weight:    b.Weight,
			Healthy:   true,
			ServiceID: b.ServiceID,
		})
	}
	t.mu.Unlock()

	if len(healthy) == 0 {
		return nil, llm.ErrNoHealthyBackend
	}

	strategy := r.Strategy
	if strategy == "" {
		strategy = llm.StrategyPriority
	}
	switch strategy {
	case llm.StrategyRoundRobin:
		t.mu.Lock()
		n := len(healthy)
		start := t.cursor[model] % n
		t.cursor[model] = (start + 1) % n
		t.mu.Unlock()
		rotated := make([]llm.Backend, 0, n)
		rotated = append(rotated, healthy[start:]...)
		rotated = append(rotated, healthy[:start]...)
		return rotated, nil
	case llm.StrategyWeighted:
		
		total := 0
		for _, b := range healthy {
			w := b.Weight
			if w < 1 {
				w = 1 
			}
			total += w
		}
		if total <= 0 {
			
			sort.SliceStable(healthy, func(i, j int) bool {
				if healthy[i].Weight != healthy[j].Weight {
					return healthy[i].Weight > healthy[j].Weight
				}
				return healthy[i].Name < healthy[j].Name
			})
			return healthy, nil
		}
		pick := rand.Intn(total) 
		primary := 0
		for i, b := range healthy {
			w := b.Weight
			if w < 1 {
				w = 1
			}
			pick -= w
			if pick < 0 {
				primary = i
				break
			}
		}
		
		ordered := make([]llm.Backend, 0, len(healthy))
		ordered = append(ordered, healthy[primary])
		for i, b := range healthy {
			if i == primary {
				continue
			}
			ordered = append(ordered, b)
		}
		sort.SliceStable(ordered[1:], func(i, j int) bool {
			if ordered[1+i].Weight != ordered[1+j].Weight {
				return ordered[1+i].Weight > ordered[1+j].Weight
			}
			return ordered[1+i].Name < ordered[1+j].Name
		})
		return ordered, nil
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

func (t *repoRouteTable) lookup(tenantID, model string) *llm.Route {
	candidates := []string{tenantID}
	if tenantID != fallbackTenant {
		candidates = append(candidates, fallbackTenant) 
	}
	for _, tid := range candidates {
		if tid == "" {
			continue
		}
		routes, err := t.repo.List(context.Background(), tid)
		if err != nil {
			continue
		}
		for i := range routes {
			if routes[i].Model == model {
				r := routes[i] 
				return &r
			}
		}
	}
	return nil
}