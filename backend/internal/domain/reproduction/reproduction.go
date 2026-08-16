
package reproduction

import (
	"math"
	"time"
)

const (
	DefaultAbsTol = 0.01
	DefaultRelTol = 0.05
)

type Config struct {
	AbsTol float64
	RelTol float64
}

type Result struct {
	SourceRunID  string  `json:"source_run_id"`
	ReproRunID   string  `json:"repro_run_id"`
	MetricName   string  `json:"metric_name"`
	SourceMetric float64 `json:"source_metric"`
	ReproMetric  float64 `json:"repro_metric"`
	AbsDeviation float64 `json:"abs_deviation"`
	RelDeviation float64 `json:"rel_deviation"`
	Reproducible bool    `json:"reproducible"`
	AbsTol       float64 `json:"abs_tol"`
	RelTol       float64 `json:"rel_tol"`
	EvaluatedAt  string  `json:"evaluated_at"`
}

func Evaluate(cfg Config, metricName, sourceRunID, reproRunID string, source, repro float64) Result {
	absDev := repro - source
	if absDev < 0 {
		absDev = -absDev
	}
	relDev := 0.0
	if source != 0 {
		relDev = absDev / math.Abs(source)
	}
	reproducible := absDev <= cfg.AbsTol || relDev <= cfg.RelTol
	return Result{
		SourceRunID:  sourceRunID,
		ReproRunID:   reproRunID,
		MetricName:   metricName,
		SourceMetric: source,
		ReproMetric:  repro,
		AbsDeviation: absDev,
		RelDeviation: relDev,
		Reproducible: reproducible,
		AbsTol:       cfg.AbsTol,
		RelTol:       cfg.RelTol,
		EvaluatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
}