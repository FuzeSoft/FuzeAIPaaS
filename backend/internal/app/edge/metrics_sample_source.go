
package edge

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/domain/edge"
	"fuze-ai-paas/backend/internal/ports"
)

var ErrNoDriftMetrics = errors.New("no drift metrics available for window")

type SampleMetricNames struct {
	
	FeatureMean string
	FeatureStd  string
	FeatureP01  string
	FeatureP25  string
	FeatureP50  string
	FeatureP75  string
	FeatureP99  string
	FeatureMin  string
	FeatureMax  string
	
	FeatureCatRatio string
	
	PredictionRatio string
	
	LatencyP50MS string
	LatencyP95MS string
	ErrorRate    string
	Accuracy     string
	
	ConceptLabelRatio         string
	GroundTruthAvailableGauge string
}

func DefaultSampleMetricNames() SampleMetricNames {
	return SampleMetricNames{
		FeatureMean:               "edge_feature_mean",
		FeatureStd:                "edge_feature_std",
		FeatureP01:                "edge_feature_p01",
		FeatureP25:                "edge_feature_p25",
		FeatureP50:                "edge_feature_p50",
		FeatureP75:                "edge_feature_p75",
		FeatureP99:                "edge_feature_p99",
		FeatureMin:                "edge_feature_min",
		FeatureMax:                "edge_feature_max",
		FeatureCatRatio:           "edge_feature_cat_ratio",
		PredictionRatio:           "edge_prediction_ratio",
		LatencyP50MS:              "edge_latency_p50_ms",
		LatencyP95MS:              "edge_latency_p95_ms",
		ErrorRate:                 "edge_inference_error_rate",
		Accuracy:                  "edge_inference_accuracy",
		ConceptLabelRatio:         "edge_concept_label_ratio",
		GroundTruthAvailableGauge: "edge_ground_truth_available",
	}
}

type MetricsBackedSampleSource struct {
	metrics ports.MetricsQuery
	names   SampleMetricNames
	
	defaultStep int
	
	conceptSrc edge.ConceptLabelSource
}

func NewMetricsBackedSampleSource(metrics ports.MetricsQuery, names SampleMetricNames) *MetricsBackedSampleSource {
	if names.FeatureMean == "" {
		names = DefaultSampleMetricNames()
	}
	return &MetricsBackedSampleSource{
		metrics:     metrics,
		names:       names,
		defaultStep: 60,
	}
}

func (s *MetricsBackedSampleSource) WithConceptSource(src edge.ConceptLabelSource) *MetricsBackedSampleSource {
	s.conceptSrc = src
	return s
}

func scopeLabels(d *edge.EdgeDeployment) map[string]string {
	return map[string]string{
		"deployment_id": d.ID,
		"tenant_id":     d.TenantID,
	}
}

func (s *MetricsBackedSampleSource) latestSeries(ctx context.Context, metric string, labels map[string]string, window time.Duration, step int) ([]ports.MetricSeries, error) {
	now := time.Now()
	q := ports.MetricQuery{
		Query:  metric,
		Labels: labels,
		Start:  now.Add(-window).UnixMilli(),
		End:    now.UnixMilli(),
		Step:   step,
	}
	if step <= 0 {
		q.Step = s.defaultStep
	}
	return s.metrics.QueryRange(q)
}

func (s *MetricsBackedSampleSource) latestValue(ctx context.Context, metric string, labels map[string]string, window time.Duration) (float64, bool, error) {
	series, err := s.latestSeries(ctx, metric, labels, window, 0)
	if err != nil {
		return 0, false, err
	}
	for _, sr := range series {
		if len(sr.Samples) == 0 {
			continue
		}
		return sr.Samples[len(sr.Samples)-1].Value, true, nil
	}
	return 0, false, nil
}

func lastValueByLabel(series []ports.MetricSeries, dimKey string) map[string]float64 {
	out := map[string]float64{}
	for _, sr := range series {
		lv, ok := sr.Labels[dimKey]
		if !ok || lv == "" {
			continue
		}
		if len(sr.Samples) == 0 {
			continue
		}
		last := sr.Samples[len(sr.Samples)-1].Value
		if last < 0 {
			last = 0
		}
		out[lv] = last
	}
	return out
}

func (s *MetricsBackedSampleSource) numericByFeature(ctx context.Context, metricFor func(string) string, base map[string]string, window time.Duration) (map[string]*edge.FeatureStat, error) {
	
	meanSeries, err := s.latestSeries(ctx, metricFor("Mean"), base, window, 0)
	if err != nil {
		return nil, err
	}
	stats := map[string]*edge.FeatureStat{}
	for _, sr := range meanSeries {
		f, ok := sr.Labels["feature"]
		if !ok || f == "" {
			continue
		}
		if len(sr.Samples) == 0 {
			continue
		}
		stats[f] = &edge.FeatureStat{Mean: sr.Samples[len(sr.Samples)-1].Value}
	}
	if len(stats) == 0 {
		return stats, nil
	}
	
	type fieldFn struct {
		name string
		set  func(*edge.FeatureStat, float64)
	}
	fields := []fieldFn{
		{"Std", func(st *edge.FeatureStat, v float64) { st.Std = v }},
		{"P01", func(st *edge.FeatureStat, v float64) { st.P01 = v }},
		{"P25", func(st *edge.FeatureStat, v float64) { st.P25 = v }},
		{"P50", func(st *edge.FeatureStat, v float64) { st.P50 = v }},
		{"P75", func(st *edge.FeatureStat, v float64) { st.P75 = v }},
		{"P99", func(st *edge.FeatureStat, v float64) { st.P99 = v }},
		{"Min", func(st *edge.FeatureStat, v float64) { st.Min = v }},
		{"Max", func(st *edge.FeatureStat, v float64) { st.Max = v }},
	}
	for _, fld := range fields {
		ss, e := s.latestSeries(ctx, metricFor(fld.name), base, window, 0)
		if e != nil {
			return nil, e
		}
		for _, sr := range ss {
			f := sr.Labels["feature"]
			st, ok := stats[f]
			if !ok || len(sr.Samples) == 0 {
				continue
			}
			fld.set(st, sr.Samples[len(sr.Samples)-1].Value)
		}
	}
	
	for _, st := range stats {
		if st.Min == 0 && st.P01 != 0 {
			st.Min = st.P01
		}
		if st.Max == 0 && st.P99 != 0 {
			st.Max = st.P99
		}
		if st.Min == 0 && st.Max == 0 {
			st.Min, st.Max = math.Inf(-1), math.Inf(1)
		}
	}
	return stats, nil
}

func (s *MetricsBackedSampleSource) Sample(ctx context.Context, d *edge.EdgeDeployment, window string) (*edge.DriftSample, error) {
	if s.metrics == nil {
		return nil, edge.ErrMissingSampleSource
	}
	w, err := parseWindow(window)
	if err != nil {
		return nil, err
	}
	base := scopeLabels(d)

	numeric, err := s.numericByFeature(ctx, func(field string) string {
		switch field {
		case "Mean":
			return s.names.FeatureMean
		case "Std":
			return s.names.FeatureStd
		case "P01":
			return s.names.FeatureP01
		case "P25":
			return s.names.FeatureP25
		case "P50":
			return s.names.FeatureP50
		case "P75":
			return s.names.FeatureP75
		case "P99":
			return s.names.FeatureP99
		case "Min":
			return s.names.FeatureMin
		case "Max":
			return s.names.FeatureMax
		}
		return ""
	}, base, w)
	if err != nil {
		return nil, err
	}

	categorical := map[string]map[string]float64{}
	catSeries, err := s.latestSeries(ctx, s.names.FeatureCatRatio, base, w, 0)
	if err != nil {
		return nil, err
	}
	for _, sr := range catSeries {
		f, ok := sr.Labels["feature"]
		if !ok || f == "" {
			continue
		}
		v, ok := sr.Labels["value"]
		if !ok || v == "" || len(sr.Samples) == 0 {
			continue
		}
		val := sr.Samples[len(sr.Samples)-1].Value
		if val < 0 {
			val = 0
		}
		if categorical[f] == nil {
			categorical[f] = map[string]float64{}
		}
		categorical[f][v] += val
	}
	
	for f := range categorical {
		categorical[f] = normalize(categorical[f])
	}

	predSeries, err := s.latestSeries(ctx, s.names.PredictionRatio, base, w, 0)
	if err != nil {
		return nil, err
	}
	predictionDist := normalize(lastValueByLabel(predSeries, "label"))

	var perf *edge.PerformanceSample
	latP50, okP50, e1 := s.latestValue(ctx, s.names.LatencyP50MS, base, w)
	latP95, okP95, e2 := s.latestValue(ctx, s.names.LatencyP95MS, base, w)
	errRate, okErr, e3 := s.latestValue(ctx, s.names.ErrorRate, base, w)
	acc, okAcc, e4 := s.latestValue(ctx, s.names.Accuracy, base, w)
	if (okP50 || okP95 || okErr || okAcc) && e1 == nil && e2 == nil && e3 == nil && e4 == nil {
		perf = &edge.PerformanceSample{
			LatencyP50Ms:  latP50,
			LatencyP95Ms: latP95,
			ErrorRate:    errRate,
			AccuracyProxy: acc,
		}
	}

	concept := map[string]float64{}
	hasLabel := false
	if s.conceptSrc != nil {
		if dist, hl, e := s.conceptSrc.ConceptLabels(ctx, d.TenantID, d.ID, w); e == nil && hl {
			concept = dist
			hasLabel = true
		}
	}
	if !hasLabel {
		conceptSeries, err := s.latestSeries(ctx, s.names.ConceptLabelRatio, base, w, 0)
		if err != nil {
			return nil, err
		}
		concept = normalize(lastValueByLabel(conceptSeries, "label"))
		if len(concept) > 0 {
			hasLabel = true
		}
		
		if gt, okGT, e5 := s.latestValue(ctx, s.names.GroundTruthAvailableGauge, base, w); e5 == nil && okGT {
			hasLabel = hasLabel || gt > 0.5
		}
	}

	if len(numeric) == 0 && len(categorical) == 0 && len(predictionDist) == 0 &&
		perf == nil && len(concept) == 0 {
		return nil, ErrNoDriftMetrics
	}

	return &edge.DriftSample{
		NumericFeatures:      numeric,
		CategoricalFeatures:  categorical,
		PredictionDist:       predictionDist,
		Performance:          perf,
		ConceptLabels:        concept,
		HasLabel:             hasLabel,
	}, nil
}

func normalize(m map[string]float64) map[string]float64 {
	if len(m) == 0 {
		return m
	}
	var sum float64
	for _, v := range m {
		if v > 0 {
			sum += v
		}
	}
	if sum <= 0 {
		return m
	}
	for k, v := range m {
		m[k] = v / sum
	}
	return m
}

func parseWindow(window string) (time.Duration, error) {
	w := strings.TrimSpace(window)
	if w == "" {
		return 15 * time.Minute, nil
	}
	d, err := time.ParseDuration(w)
	if err != nil {
		return 0, fmt.Errorf("invalid drift window %q: %w", window, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("drift window must be positive: %q", window)
	}
	return d, nil
}