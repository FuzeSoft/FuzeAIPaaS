package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	domainevent "fuze-ai-paas/backend/internal/domain/event"
)

type mockSink struct {
	mu     sync.Mutex
	called int
	last   domainevent.Event
}

func (m *mockSink) Notify(ctx context.Context, e domainevent.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called++
	m.last = e
	return nil
}

func (m *mockSink) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called
}

func TestNewWebhookNotifierEmptyURL(t *testing.T) {
	if NewWebhookNotifier("") != nil {
		t.Fatal("expected nil notifier for empty URL")
	}
}

func TestWebhookNotifierSuccess(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL)
	e := domainevent.NewClusterDiscovered("c1", "cluster-1", domainevent.ClusterStats{TotalGPUs: 4})
	if err := n.Notify(context.Background(), e); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}
	if got["type"] != "ClusterDiscovered" {
		t.Errorf("payload type = %v, want ClusterDiscovered", got["type"])
	}
	if got["aggregate_id"] != "c1" {
		t.Errorf("payload aggregate_id = %v, want c1", got["aggregate_id"])
	}
	if _, ok := got["occurred_at"]; !ok {
		t.Error("payload missing occurred_at")
	}
}

func TestWebhookNotifierBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL)
	err := n.Notify(context.Background(), domainevent.NewClusterDiscovered("c", "n", domainevent.ClusterStats{}))
	if err == nil {
		t.Fatal("expected error on non-2xx status")
	}
}

func TestMultiSinkFanout(t *testing.T) {
	s1, s2 := &mockSink{}, &mockSink{}
	ms := NewMultiSink(s1, s2)

	e := domainevent.NewJobSubmitted("j1", "c1", "train", 2, "t1")
	if err := ms.Notify(context.Background(), e); err != nil {
		t.Fatalf("MultiSink.Notify error: %v", err)
	}
	if s1.count() != 1 || s2.count() != 1 {
		t.Fatalf("fan-out failed: s1=%d s2=%d, want 1/1", s1.count(), s2.count())
	}
	if s1.last.AggregateID() != "j1" {
		t.Errorf("s1 received event aggregate = %q, want j1", s1.last.AggregateID())
	}
}

type failingSink struct{}

func (failingSink) Notify(ctx context.Context, e domainevent.Event) error { return io.ErrUnexpectedEOF }

func TestMultiSinkIsolatesFailures(t *testing.T) {
	ok := &mockSink{}
	ms := NewMultiSink(failingSink{}, ok)
	
	if err := ms.Notify(context.Background(), domainevent.NewJobSubmitted("j", "c", "train", 1, "t")); err != nil {
		t.Fatalf("MultiSink should not surface sink errors: %v", err)
	}
	if ok.count() != 1 {
		t.Fatalf("ok sink should still be called despite other sink failing; got %d", ok.count())
	}
}