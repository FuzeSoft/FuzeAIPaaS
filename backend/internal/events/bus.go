
package events

import (
	"context"
	"log"
	"sync"
	"sync/atomic"

	domainevent "fuze-ai-paas/backend/internal/domain/event"
)

type Handler func(ctx context.Context, e domainevent.Event)

type DropHandler func(e domainevent.Event)

type Bus struct {
	ch      chan domainevent.Event
	subs    map[string][]Handler
	mu      sync.RWMutex
	workers int
	buffer  int
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	dropped atomic.Uint64
	
	onDrop DropHandler
}

func NewBus(buffer, workers int) *Bus {
	if buffer <= 0 {
		buffer = 1024
	}
	if workers <= 0 {
		workers = 4
	}
	return &Bus{
		ch:      make(chan domainevent.Event, buffer),
		subs:    make(map[string][]Handler),
		workers: workers,
		buffer:  buffer,
	}
}

func (b *Bus) Subscribe(eventType string, h Handler) {
	b.mu.Lock()
	b.subs[eventType] = append(b.subs[eventType], h)
	b.mu.Unlock()
}

func (b *Bus) SetDropHandler(h DropHandler) {
	b.onDrop = h
}

func (b *Bus) Dropped() uint64 {
	return b.dropped.Load()
}

func (b *Bus) Publish(e domainevent.Event) {
	select {
	case b.ch <- e:
	default:
		b.dropped.Add(1)
		if b.onDrop != nil {
			b.onDrop(e)
		} else {
			log.Printf("[events] buffer full (cap=%d), drop %s/%s", b.buffer, e.EventType(), e.AggregateID())
		}
	}
}

func (b *Bus) Run(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	b.cancel = cancel
	for i := 0; i < b.workers; i++ {
		b.wg.Add(1)
		go b.loop(runCtx)
	}
}

func (b *Bus) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
}

func (b *Bus) loop(ctx context.Context) {
	defer b.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-b.ch:
			b.dispatch(ctx, e)
		}
	}
}

func (b *Bus) dispatch(ctx context.Context, e domainevent.Event) {
	b.mu.RLock()
	handlers := b.subs[e.EventType()]
	b.mu.RUnlock()
	for _, h := range handlers {
		b.safeCall(ctx, e, h)
	}
}

func (b *Bus) safeCall(ctx context.Context, e domainevent.Event, h Handler) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[events] handler panic recovered: event=%s agg=%s: %v",
				e.EventType(), e.AggregateID(), r)
		}
	}()
	h(ctx, e)
}