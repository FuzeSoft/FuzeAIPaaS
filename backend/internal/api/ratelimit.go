package api

import (
	"sync"
	"time"
)

type tenantLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*rateBucket
	limit    int
	window   time.Duration
}

type rateBucket struct {
	count    int
	resetAt  time.Time
}

func newTenantLimiter(limit int) *tenantLimiter {
	if limit <= 0 {
		return nil
	}
	return &tenantLimiter{
		buckets: make(map[string]*rateBucket),
		limit:   limit,
		window:  time.Second,
	}
}

func (l *tenantLimiter) Allow(tenantID string) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[tenantID]
	if !ok || now.After(b.resetAt) {
		b = &rateBucket{resetAt: now.Add(l.window)}
		l.buckets[tenantID] = b
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	return true
}