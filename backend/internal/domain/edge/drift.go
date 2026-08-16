package edge

import "time"

type DriftType string

const (
	DriftTypeData       DriftType = "data"       
	DriftTypePrediction DriftType = "prediction" 
	DriftTypePerformance DriftType = "performance" 
	DriftTypeConcept    DriftType = "concept"    
)

type DriftSeverity string

const (
	DriftSeverityNone     DriftSeverity = "none"
	DriftSeverityLow      DriftSeverity = "low"
	DriftSeverityMedium   DriftSeverity = "medium"
	DriftSeverityHigh     DriftSeverity = "high"
	DriftSeverityCritical DriftSeverity = "critical"
)

var severityRank = map[DriftSeverity]int{
	DriftSeverityNone:     0,
	DriftSeverityLow:      1,
	DriftSeverityMedium:   2,
	DriftSeverityHigh:     3,
	DriftSeverityCritical: 4,
}

func MaxSeverity(a, b DriftSeverity) DriftSeverity {
	if severityRank[a] >= severityRank[b] {
		return a
	}
	return b
}

type DriftMetric struct {
	Type      DriftType
	Score     float64 
	Threshold float64 
	Drifted   bool
	Detail    string
}

type DriftSample struct {
	
	NumericFeatures map[string]*FeatureStat
	
	CategoricalFeatures map[string]map[string]float64
	
	PredictionDist map[string]float64
	
	Performance *PerformanceSample
	
	ConceptLabels map[string]float64
	
	HasLabel bool
}

type FeatureStat struct {
	Mean   float64
	Std    float64
	Min    float64
	P01    float64
	P25    float64
	P50    float64
	P75    float64
	P99    float64
	Max    float64
}

type PerformanceSample struct {
	LatencyP50Ms  float64
	LatencyP95Ms  float64
	ErrorRate     float64 
	AccuracyProxy float64 
	HasLabel      bool    
}

type DriftBaseline struct {
	DeploymentID   string
	ReferenceWindow string
	NumericFeatures map[string]*FeatureStat
	CategoricalFeatures map[string]map[string]float64
	PredictionDist map[string]float64
	Performance    *PerformanceSample
	ConceptLabels  map[string]float64
}

type DriftReport struct {
	ID             string
	TenantID       string
	DeploymentID   string
	NodeID         string
	EvaluatedAt    time.Time
	DataDrift      *DriftMetric
	PredictionDrift *DriftMetric
	PerformanceDrift *DriftMetric
	ConceptDrift   *DriftMetric
	OverallSeverity DriftSeverity
	TriggeredRollback bool
	Recommendation string
}

type Detector struct {
	dataDriftThreshold       float64 
	predictionDriftThreshold float64 
	perfLatencyFactor        float64 
	perfErrorRate            float64 
	perfAccuracyDrop          float64 
	conceptThreshold         float64 
}

type DetectorOption func(*Detector)

func WithDataDriftThreshold(v float64) DetectorOption { return func(d *Detector) { d.dataDriftThreshold = v } }

func WithPredictionDriftThreshold(v float64) DetectorOption {
	return func(d *Detector) { d.predictionDriftThreshold = v }
}

func NewDetector(opts ...DetectorOption) *Detector {
	d := &Detector{
		dataDriftThreshold:       0.2,
		predictionDriftThreshold: 0.2,
		perfLatencyFactor:        1.5,
		perfErrorRate:            0.05,
		perfAccuracyDrop:         0.05,
		conceptThreshold:         0.2,
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

func (det *Detector) Evaluate(tenantID, deploymentID, nodeID string, base *DriftBaseline, sample *DriftSample) *DriftReport {
	now := time.Now().UTC()
	r := &DriftReport{
		ID:             generateID("drift"),
		TenantID:       tenantID,
		DeploymentID:   deploymentID,
		NodeID:         nodeID,
		EvaluatedAt:    now,
		OverallSeverity: DriftSeverityNone,
	}

	if base == nil || sample == nil {
		r.Recommendation = "缺少基线或样本，跳过漂移评估"
		return r
	}

	r.DataDrift = det.dataDrift(base, sample)
	r.PredictionDrift = det.predictionDrift(base, sample)
	r.PerformanceDrift = det.performanceDrift(base, sample)
	r.ConceptDrift = det.conceptDrift(base, sample)

	for _, m := range []*DriftMetric{r.DataDrift, r.PredictionDrift, r.PerformanceDrift, r.ConceptDrift} {
		if m == nil {
			continue
		}
		sev := severityOf(m)
		r.OverallSeverity = MaxSeverity(r.OverallSeverity, sev)
	}

	if r.OverallSeverity == DriftSeverityHigh || r.OverallSeverity == DriftSeverityCritical {
		r.TriggeredRollback = true
		r.Recommendation = "检测到显著漂移，建议立即回滚到稳定版本"
	} else if r.OverallSeverity == DriftSeverityMedium {
		r.Recommendation = "存在中等漂移，建议观察并降低灰度权重"
	} else {
		r.Recommendation = "漂移在可接受范围内"
	}
	return r
}

func severityOf(m *DriftMetric) DriftSeverity {
	if !m.Drifted {
		return DriftSeverityNone
	}
	switch {
	case m.Score >= m.Threshold*3:
		return DriftSeverityCritical
	case m.Score >= m.Threshold*2:
		return DriftSeverityHigh
	case m.Score >= m.Threshold:
		return DriftSeverityMedium
	default:
		return DriftSeverityLow
	}
}

func (det *Detector) dataDrift(base *DriftBaseline, sample *DriftSample) *DriftMetric {
	m := &DriftMetric{Type: DriftTypeData, Threshold: det.dataDriftThreshold}

	var worstPSI float64
	var worstFeat string
	for name, bs := range base.NumericFeatures {
		ss, ok := sample.NumericFeatures[name]
		if !ok {
			continue
		}
		psi := psiNumeric(bs, ss)
		if psi > worstPSI {
			worstPSI = psi
			worstFeat = name
		}
	}
	
	for name, bdist := range base.CategoricalFeatures {
		sdist, ok := sample.CategoricalFeatures[name]
		if !ok {
			continue
		}
		tvd := tvdCategorical(bdist, sdist)
		if tvd > worstPSI {
			worstPSI = tvd
			worstFeat = name
		}
	}
	m.Score = worstPSI
	m.Drifted = worstPSI >= det.dataDriftThreshold
	if worstFeat != "" {
		m.Detail = "worst feature=" + worstFeat
	}
	return m
}

func (det *Detector) predictionDrift(base *DriftBaseline, sample *DriftSample) *DriftMetric {
	m := &DriftMetric{Type: DriftTypePrediction, Threshold: det.predictionDriftThreshold}
	tvd := tvdCategorical(base.PredictionDist, sample.PredictionDist)
	m.Score = tvd
	m.Drifted = tvd >= det.predictionDriftThreshold
	m.Detail = "output distribution TVD"
	return m
}

func (det *Detector) performanceDrift(base *DriftBaseline, sample *DriftSample) *DriftMetric {
	m := &DriftMetric{Type: DriftTypePerformance, Threshold: det.perfErrorRate}
	if base.Performance == nil || sample.Performance == nil {
		m.Detail = "missing performance baseline/sample"
		return m
	}
	bps := base.Performance
	sps := sample.Performance

	var worst float64
	var detail string
	
	if bps.LatencyP95Ms > 0 {
		factor := sps.LatencyP95Ms / bps.LatencyP95Ms
		if factor > det.perfLatencyFactor {
			rise := factor - 1
			if rise > worst {
				worst = rise
				detail = "p95 latency rise"
			}
		}
	}
	
	if sps.ErrorRate > det.perfErrorRate {
		if sps.ErrorRate > worst {
			worst = sps.ErrorRate
			detail = "error rate"
		}
	}
	
	if sps.HasLabel && bps.HasLabel {
		drop := bps.AccuracyProxy - sps.AccuracyProxy
		if drop > det.perfAccuracyDrop {
			if drop > worst {
				worst = drop
				detail = "accuracy drop"
			}
		}
	}
	m.Score = worst
	m.Drifted = worst >= det.perfErrorRate
	m.Detail = detail
	return m
}

func (det *Detector) conceptDrift(base *DriftBaseline, sample *DriftSample) *DriftMetric {
	m := &DriftMetric{Type: DriftTypeConcept, Threshold: det.conceptThreshold}
	hasLabel := sample.HasLabel || (sample.Performance != nil && sample.Performance.HasLabel)
	if !hasLabel {
		m.Detail = "no ground truth labels, concept drift skipped"
		return m
	}
	tvd := tvdCategorical(base.ConceptLabels, sample.ConceptLabels)
	m.Score = tvd
	m.Drifted = tvd >= det.conceptThreshold
	m.Detail = "label distribution TVD"
	return m
}

func psiNumeric(base, sample *FeatureStat) float64 {
	
	var drift float64
	
	if base.P50 != 0 {
		drift += min(1, abs((sample.P50-base.P50)/base.P50)*0.5)
	} else if sample.P50 != 0 {
		drift += 0.5
	}
	
	if base.Std != 0 {
		drift += min(1, abs((sample.Std-base.Std)/base.Std)*0.5)
	} else if sample.Std != 0 {
		drift += 0.5
	}
	if drift > 1 {
		drift = 1
	}
	return drift
}

func tvdCategorical(base, sample map[string]float64) float64 {
	keys := make(map[string]struct{})
	for k := range base {
		keys[k] = struct{}{}
	}
	for k := range sample {
		keys[k] = struct{}{}
	}
	var sum float64
	for k := range keys {
		b := base[k]
		s := sample[k]
		sum += abs(b - s)
	}
	return sum / 2
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}