
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	domainevent "fuze-ai-paas/backend/internal/domain/event"
)

type EventSink interface {
	Notify(ctx context.Context, e domainevent.Event) error
}

type WebhookNotifier struct {
	url    string
	client *http.Client
}

func NewWebhookNotifier(url string) *WebhookNotifier {
	if url == "" {
		return nil
	}
	return &WebhookNotifier{
		url: url,
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
			},
		},
	}
}

func (w *WebhookNotifier) Notify(ctx context.Context, e domainevent.Event) error {
	payload := map[string]any{
		"type":         e.EventType(),
		"aggregate_id": e.AggregateID(),
		"occurred_at":  e.OccurredAt(),
		"payload":      e,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

type MultiSink struct {
	sinks []EventSink
}

func NewMultiSink(sinks ...EventSink) *MultiSink {
	return &MultiSink{sinks: sinks}
}

func (m *MultiSink) Notify(ctx context.Context, e domainevent.Event) error {
	for _, s := range m.sinks {
		if err := s.Notify(ctx, e); err != nil {
			log.Printf("[notify] sink error: %v", err)
		}
	}
	return nil
}