
package metricsquery

import (
	"fuze-ai-paas/backend/internal/ports"
)

type Noop struct{}

func NewNoop() ports.MetricsQuery { return Noop{} }

func (Noop) QueryRange(q ports.MetricQuery) ([]ports.MetricSeries, error) {
	return []ports.MetricSeries{}, nil
}

func (Noop) QueryLatest(q ports.MetricQuery) (*ports.MetricSample, error) {
	return nil, nil
}

func (Noop) Alerts() ([]ports.ActiveAlert, error) {
	return []ports.ActiveAlert{}, nil
}