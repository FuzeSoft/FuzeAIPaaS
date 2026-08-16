package edge

import (
	"testing"
)

func baseBaseline() *DriftBaseline {
	return &DriftBaseline{
		DeploymentID: "dep-1",
		NumericFeatures: map[string]*FeatureStat{
			"f1": {Mean: 0, Std: 1, P01: -2, P25: -1, P50: 0, P75: 1, P99: 2, Max: 3},
		},
		CategoricalFeatures: map[string]map[string]float64{
			"c1": {"a": 0.6, "b": 0.4},
		},
		PredictionDist: map[string]float64{"cat0": 0.7, "cat1": 0.3},
		Performance: &PerformanceSample{
			LatencyP50Ms: 20, LatencyP95Ms: 50, ErrorRate: 0.01, AccuracyProxy: 0.95, HasLabel: true,
		},
		ConceptLabels: map[string]float64{"yes": 0.5, "no": 0.5},
	}
}

func TestDataDriftTriggersWhenPSIExceedsThreshold(t *testing.T) {
	det := NewDetector()
	base := baseBaseline()
	
	sample := &DriftSample{
		NumericFeatures: map[string]*FeatureStat{
			"f1": {Mean: 10, Std: 2, P01: 6, P25: 8, P50: 10, P75: 12, P99: 14, Max: 16},
		},
		CategoricalFeatures: map[string]map[string]float64{"c1": {"a": 0.6, "b": 0.4}},
		PredictionDist:      map[string]float64{"cat0": 0.7, "cat1": 0.3},
		Performance:         base.Performance,
		ConceptLabels:       base.ConceptLabels,
	}
	rep := det.Evaluate("t1", "dep-1", "n1", base, sample)
	if rep.DataDrift == nil {
		t.Fatal("data drift metric missing")
	}
	if !rep.DataDrift.Drifted {
		t.Fatalf("expected data drift, score=%.3f threshold=%.3f", rep.DataDrift.Score, rep.DataDrift.Threshold)
	}
	if rep.OverallSeverity == DriftSeverityNone {
		t.Fatalf("expected non-none overall severity, got %s", rep.OverallSeverity)
	}
}

func TestPredictionDriftTVD(t *testing.T) {
	det := NewDetector()
	base := baseBaseline()
	sample := &DriftSample{
		NumericFeatures:    base.NumericFeatures,
		CategoricalFeatures: base.CategoricalFeatures,
		
		PredictionDist: map[string]float64{"cat0": 0.1, "cat1": 0.9},
		Performance:    base.Performance,
		ConceptLabels:  base.ConceptLabels,
	}
	rep := det.Evaluate("t1", "dep-1", "n1", base, sample)
	if rep.PredictionDrift == nil || !rep.PredictionDrift.Drifted {
		t.Fatalf("expected prediction drift, got %+v", rep.PredictionDrift)
	}
	
	if rep.PredictionDrift.Score < 0.2 {
		t.Fatalf("TVD too low: %.3f", rep.PredictionDrift.Score)
	}
}

func TestPerformanceDrift(t *testing.T) {
	det := NewDetector()
	base := baseBaseline()
	sample := &DriftSample{
		NumericFeatures:     base.NumericFeatures,
		CategoricalFeatures: base.CategoricalFeatures,
		PredictionDist:      base.PredictionDist,
		Performance: &PerformanceSample{
			LatencyP50Ms: 40, LatencyP95Ms: 200, ErrorRate: 0.12, AccuracyProxy: 0.80, HasLabel: true,
		},
		ConceptLabels: base.ConceptLabels,
	}
	rep := det.Evaluate("t1", "dep-1", "n1", base, sample)
	if rep.PerformanceDrift == nil || !rep.PerformanceDrift.Drifted {
		t.Fatalf("expected performance drift, got %+v", rep.PerformanceDrift)
	}
}

func TestConceptDriftSkippedWithoutLabels(t *testing.T) {
	det := NewDetector()
	base := baseBaseline()
	base.Performance = &PerformanceSample{HasLabel: false}
	sample := &DriftSample{
		NumericFeatures:     base.NumericFeatures,
		CategoricalFeatures: base.CategoricalFeatures,
		PredictionDist:      base.PredictionDist,
		Performance:         &PerformanceSample{HasLabel: false},
		
		ConceptLabels: map[string]float64{"yes": 0.9, "no": 0.1},
	}
	rep := det.Evaluate("t1", "dep-1", "n1", base, sample)
	if rep.ConceptDrift == nil {
		t.Fatal("concept drift metric missing")
	}
	if rep.ConceptDrift.Drifted {
		t.Fatalf("concept drift should be skipped without labels, got drifted")
	}
}

func TestNoDriftWhenSampleMatchesBaseline(t *testing.T) {
	det := NewDetector()
	base := baseBaseline()
	sample := &DriftSample{
		NumericFeatures:     base.NumericFeatures,
		CategoricalFeatures: base.CategoricalFeatures,
		PredictionDist:      base.PredictionDist,
		Performance:         base.Performance,
		ConceptLabels:       base.ConceptLabels,
	}
	rep := det.Evaluate("t1", "dep-1", "n1", base, sample)
	if rep.OverallSeverity != DriftSeverityNone {
		t.Fatalf("expected no drift, got %s", rep.OverallSeverity)
	}
	if rep.TriggeredRollback {
		t.Fatal("should not trigger rollback when no drift")
	}
}

func TestTVDComputation(t *testing.T) {
	a := map[string]float64{"x": 0.5, "y": 0.5}
	b := map[string]float64{"x": 1.0, "y": 0.0}
	if got := tvdCategorical(a, b); got != 0.5 {
		t.Fatalf("TVD = %v, want 0.5", got)
	}
}