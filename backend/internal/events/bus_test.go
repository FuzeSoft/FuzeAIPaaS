package events

import (
	"context"
	"sync"
	"testing"
	"time"

	domainevent "fuze-ai-paas/backend/internal/domain/event"
)

func TestBusAsyncDispatch(t *testing.T) {
	b := NewBus(16, 2)
	b.Run(context.Background())

	got := make(chan string, 1)
	b.Subscribe(domainevent.ClusterDiscoveredType, func(_ context.Context, e domainevent.Event) {
		got <- e.AggregateID()
	})

	b.Publish(domainevent.NewClusterDiscovered("c1", "C1", domainevent.ClusterStats{TotalGPUs: 8}))

	select {
	case id := <-got:
		if id != "c1" {
			t.Fatalf("aggregate id = %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler not invoked within timeout")
	}
}

func TestBusFanoutToMultipleSubscribers(t *testing.T) {
	b := NewBus(16, 2)
	b.Run(context.Background())

	var mu sync.Mutex
	count := 0
	h := func(_ context.Context, _ domainevent.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	}
	b.Subscribe(domainevent.JobSubmittedType, h)
	b.Subscribe(domainevent.JobSubmittedType, h)

	b.Publish(domainevent.NewJobSubmitted("j1", "c1", "training", 8, "t1"))

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		c := count
		mu.Unlock()
		if c >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected 2 handler calls, got %d", c)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestBusPublishNonBlockingWhenFull(t *testing.T) {
	b := NewBus(1, 1)
	
	b.Publish(domainevent.NewJobSubmitted("j1", "c1", "training", 1, "t1"))
	b.Publish(domainevent.NewJobSubmitted("j2", "c1", "training", 1, "t1"))
	
}