package tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/domain/event"
	"fuze-ai-paas/backend/internal/events"
)

func TestEventBusSubscribe(t *testing.T) {
	t.Run("subscribe and receive event", func(t *testing.T) {
		bus := events.NewBus(10, 4)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		bus.Run(ctx)
		time.Sleep(50 * time.Millisecond)

		var mu sync.Mutex
		var received event.Event
		done := make(chan struct{})

		bus.Subscribe(event.JobSubmittedType, func(ctx context.Context, e event.Event) {
			mu.Lock()
			received = e
			mu.Unlock()
			close(done)
		})

		evt := event.JobSubmitted{
			BaseEvent: event.BaseEvent{
				Type: event.JobSubmittedType,
			},
			JobID: "job-001",
			GPUs:  4,
		}
		bus.Publish(evt)

		select {
		case <-done:
			mu.Lock()
			defer mu.Unlock()
			receivedEvt, ok := received.(event.JobSubmitted)
			if !ok {
				t.Fatalf("expected JobSubmitted, got %T", received)
			}
			if receivedEvt.JobID != "job-001" {
				t.Errorf("expected job-001, got %s", receivedEvt.JobID)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for event")
		}
	})
}

func TestEventBusMultipleSubscribers(t *testing.T) {
	t.Run("multiple subscribers receive the same event", func(t *testing.T) {
		bus := events.NewBus(10, 4)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		bus.Run(ctx)
		time.Sleep(50 * time.Millisecond)

		var wg sync.WaitGroup
		count := 0
		var mu sync.Mutex

		for i := 0; i < 3; i++ {
			wg.Add(1)
			bus.Subscribe(event.ClusterDiscoveredType, func(ctx context.Context, e event.Event) {
				mu.Lock()
				count++
				mu.Unlock()
				wg.Done()
			})
		}

		evt := event.ClusterDiscovered{
			BaseEvent: event.BaseEvent{
				Type: event.ClusterDiscoveredType,
			},
			ClusterID: "cluster-001",
			TotalGPUs: 8,
		}
		bus.Publish(evt)

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			mu.Lock()
			defer mu.Unlock()
			if count != 3 {
				t.Errorf("expected 3 subscribers to receive event, got %d", count)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for subscribers")
		}
	})
}

func TestDomainEventTypes(t *testing.T) {
	t.Run("verify event type constants", func(t *testing.T) {
		if event.JobSubmittedType == "" {
			t.Error("expected non-empty JobSubmittedType")
		}
		if event.AssignmentCompletedType == "" {
			t.Error("expected non-empty AssignmentCompletedType")
		}
		if event.ClusterDiscoveredType == "" {
			t.Error("expected non-empty ClusterDiscoveredType")
		}
	})
}

func TestJobSubmittedEvent(t *testing.T) {
	t.Run("create job submitted event", func(t *testing.T) {
		evt := event.JobSubmitted{
			BaseEvent: event.BaseEvent{
				Type: event.JobSubmittedType,
			},
			JobID: "job-123",
			GPUs:  8,
		}
		if evt.JobID != "job-123" {
			t.Errorf("expected job-123, got %s", evt.JobID)
		}
		if evt.EventType() != event.JobSubmittedType {
			t.Errorf("expected type %s, got %s", event.JobSubmittedType, evt.EventType())
		}
	})
}

func TestAssignmentCompletedEvent(t *testing.T) {
	t.Run("create assignment completed event", func(t *testing.T) {
		evt := event.AssignmentCompleted{
			BaseEvent: event.BaseEvent{
				Type: event.AssignmentCompletedType,
			},
			JobID:         "job-001",
			AllocatedGPUs: 4,
		}
		if evt.JobID != "job-001" {
			t.Errorf("expected job-001, got %s", evt.JobID)
		}
		if evt.EventType() != event.AssignmentCompletedType {
			t.Errorf("expected type %s, got %s", event.AssignmentCompletedType, evt.EventType())
		}
	})
}