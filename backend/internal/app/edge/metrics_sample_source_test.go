package edge

import (
	"context"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/domain/edge"
	"fuze-ai-paas/backend/internal/ports"
)

type fakeMetrics struct {
	ranges map[string][]ports.MetricSeries
	latest map[string][]ports.MetricSeries 
	calls  int
}

func (f *fakeMetrics) QueryRange(q ports.MetricQuery) ([]ports.MetricSeries, error) {
	f.calls++
	key := q.Query
	ss, ok := f.ranges[key]
	if !ok {
		return nil, nil
	}
	
	out := make([]ports.MetricSeries, 0, len(ss))
	for _, s := range ss {
		if !labelsMatch(s.Labels, q.Labels) {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeMetrics) QueryLatest(q ports.MetricQuery) (*ports.MetricSample, error) {
	ss, err := f.QueryRange(q)
	if err != nil {
		return nil, err
	}
	for _, s := range ss {
		if len(s.Samples) == 0 {
			continue
		}
		last := s.Samples[len(s.Samples)-1]
		return &last, nil
	}
	return nil, nil
}

func (f *fakeMetrics) Alerts() ([]ports.ActiveAlert, error) { return nil, nil }

func labelsMatch(srLabels, want map[string]string) bool {
	for k, v := range want {
		if srLabels[k] != v {
			return false
		}
	}
	return true
}

func TestMetricsBackedSampleSourceBuildsDriftSample(t *testing.T) {
	now := time.Now().UnixMilli()
	series := func(labels map[string]string, vals ...float64) ports.MetricSeries {
		samples := make([]ports.MetricSample, 0, len(vals))
		for i, v := range vals {
			samples = append(samples, ports.MetricSample{Timestamp: now - int64((len(vals)-i)*1000), Value: v})
		}
		return ports.MetricSeries{Labels: labels, Samples: samples}
	}
	fm := &fakeMetrics{
		ranges: map[string][]ports.MetricSeries{
			"edge_feature_mean": {
				series(map[string]string{"feature": "f1", "deployment_id": "dep-1", "tenant_id": "t1"}, 0.5),
				series(map[string]string{"feature": "f2", "deployment_id": "dep-1", "tenant_id": "t1"}, 1.2),
			},
			"edge_feature_std": {
				series(map[string]string{"feature": "f1", "deployment_id": "dep-1", "tenant_id": "t1"}, 0.1),
				series(map[string]string{"feature": "f2", "deployment_id": "dep-1", "tenant_id": "t1"}, 0.3),
			},
			"edge_feature_p99": {
				series(map[string]string{"feature": "f1", "deployment_id": "dep-1", "tenant_id": "t1"}, 0.9),
			},
			"edge_feature_cat_ratio": {
				series(map[string]string{"feature": "c1", "value": "a", "deployment_id": "dep-1", "tenant_id": "t1"}, 0.6),
				series(map[string]string{"feature": "c1", "value": "b", "deployment_id": "dep-1", "tenant_id": "t1"}, 0.4),
			},
			"edge_prediction_ratio": {
				series(map[string]string{"label": "x", "deployment_id": "dep-1", "tenant_id": "t1"}, 0.7),
				series(map[string]string{"label": "y", "deployment_id": "dep-1", "tenant_id": "t1"}, 0.3),
			},
			"edge_latency_p50_ms": {
				series(map[string]string{"deployment_id": "dep-1", "tenant_id": "t1"}, 12.0),
			},
			"edge_latency_p95_ms": {
				series(map[string]string{"deployment_id": "dep-1", "tenant_id": "t1"}, 45.0),
			},
			"edge_inference_error_rate": {
				series(map[string]string{"deployment_id": "dep-1", "tenant_id": "t1"}, 0.01),
			},
			"edge_inference_accuracy": {
				series(map[string]string{"deployment_id": "dep-1", "tenant_id": "t1"}, 0.95),
			},
			"edge_concept_label_ratio": {
				series(map[string]string{"label": "x", "deployment_id": "dep-1", "tenant_id": "t1"}, 0.8),
				series(map[string]string{"label": "y", "deployment_id": "dep-1", "tenant_id": "t1"}, 0.2),
			},
			"edge_ground_truth_available": {
				series(map[string]string{"deployment_id": "dep-1", "tenant_id": "t1"}, 1.0),
			},
		},
	}

	src := NewMetricsBackedSampleSource(fm, SampleMetricNames{})
	dep := &edge.EdgeDeployment{ID: "dep-1", TenantID: "t1"}
	sample, err := src.Sample(context.Background(), dep, "15m")
	if err != nil {
		t.Fatalf("sample error: %v", err)
	}

	if len(sample.NumericFeatures) != 2 {
		t.Fatalf("expected 2 numeric features, got %d", len(sample.NumericFeatures))
	}
	f1 := sample.NumericFeatures["f1"]
	if f1 == nil || f1.Mean != 0.5 || f1.Std != 0.1 || f1.P99 != 0.9 {
		t.Fatalf("f1 stat wrong: %+v", f1)
	}

	ca := sample.CategoricalFeatures["c1"]
	if ca["a"] != 0.6 || ca["b"] != 0.4 {
		t.Fatalf("categorical not normalized: %+v", ca)
	}

	if sample.PredictionDist["x"] != 0.7 || sample.PredictionDist["y"] != 0.3 {
		t.Fatalf("prediction dist wrong: %+v", sample.PredictionDist)
	}

	if sample.Performance == nil || sample.Performance.LatencyP95Ms != 45 || sample.Performance.ErrorRate != 0.01 {
		t.Fatalf("performance wrong: %+v", sample.Performance)
	}
	if !sample.HasLabel {
		t.Fatalf("expected HasLabel true (ground_truth_available gauge)")
	}
	if sample.ConceptLabels["x"] != 0.8 {
		t.Fatalf("concept labels wrong: %+v", sample.ConceptLabels)
	}
}

func TestMetricsBackedSampleSourceNoMetricsReturnsError(t *testing.T) {
	fm := &fakeMetrics{ranges: map[string][]ports.MetricSeries{}}
	src := NewMetricsBackedSampleSource(fm, SampleMetricNames{})
	dep := &edge.EdgeDeployment{ID: "dep-1", TenantID: "t1"}
	if _, err := src.Sample(context.Background(), dep, "15m"); err != ErrNoDriftMetrics {
		t.Fatalf("expected ErrNoDriftMetrics, got %v", err)
	}
}

type fakeConceptSource struct {
	dist map[string]float64
}

func (f *fakeConceptSource) ConceptLabels(ctx context.Context, tenantID, deploymentID string, window time.Duration) (map[string]float64, bool, error) {
	if f.dist == nil {
		return nil, false, nil
	}
	return f.dist, true, nil
}

func TestMetricsBackedSampleSourceUsesConceptLabelSource(t *testing.T) {
	
	fm := &fakeMetrics{ranges: map[string][]ports.MetricSeries{}}
	src := NewMetricsBackedSampleSource(fm, SampleMetricNames{}).
		WithConceptSource(&fakeConceptSource{dist: map[string]float64{"cat1": 0.8, "cat2": 0.2}})
	dep := &edge.EdgeDeployment{ID: "dep-1", TenantID: "t1"}
	sample, err := src.Sample(context.Background(), dep, "15m")
	if err != nil {
		t.Fatalf("sample error: %v", err)
	}
	
	if sample.ConceptLabels["cat1"] != 0.8 {
		t.Fatalf("concept labels not from source: %+v", sample.ConceptLabels)
	}
	if !sample.HasLabel {
		t.Fatalf("expected HasLabel true via concept source")
	}
}

func TestMetricsBackedSampleSourceConceptSourceOverridesGauge(t *testing.T) {
	now := time.Now().UnixMilli()
	series := func(labels map[string]string, vals ...float64) ports.MetricSeries {
		samples := make([]ports.MetricSample, 0, len(vals))
		for i, v := range vals {
			samples = append(samples, ports.MetricSample{Timestamp: now - int64((len(vals)-i)*1000), Value: v})
		}
		return ports.MetricSeries{Labels: labels, Samples: samples}
	}
	fm := &fakeMetrics{ranges: map[string][]ports.MetricSeries{
		"edge_concept_label_ratio": {
			series(map[string]string{"label": "stale", "deployment_id": "dep-1", "tenant_id": "t1"}, 1.0),
		},
		"edge_ground_truth_available": {
			series(map[string]string{"deployment_id": "dep-1", "tenant_id": "t1"}, 1.0),
		},
	}}
	
	src := NewMetricsBackedSampleSource(fm, SampleMetricNames{}).
		WithConceptSource(&fakeConceptSource{dist: map[string]float64{"live": 0.6, "dead": 0.4}})
	dep := &edge.EdgeDeployment{ID: "dep-1", TenantID: "t1"}
	sample, _ := src.Sample(context.Background(), dep, "15m")
	if sample.ConceptLabels["live"] != 0.6 {
		t.Fatalf("concept source should override gauge-based distribution: %+v", sample.ConceptLabels)
	}
}