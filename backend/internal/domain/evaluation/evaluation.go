
package evaluation

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

const (
	JudgeModeThreshold = "threshold" 
	JudgeModeHuman     = "human"     
	JudgeModeLLM       = "llm"       
	JudgeModeHybrid    = "hybrid"    
)

const (
	JudgeTypeHuman = "human"
	JudgeTypeLLM   = "llm"
)

const (
	VerdictPass    = "pass"
	VerdictFail    = "fail"
	VerdictPending = "pending"
)

const (
	OpGTE = ">="
	OpLTE = "<="
	OpGT  = ">"
	OpLT  = "<"
	OpEQ  = "=="
)

type criterion struct {
	Op    string  `json:"op"`
	Value float64 `json:"value"`
}

type Dimension struct {
	Name        string  `json:"name"`
	Weight      float64 `json:"weight"`
	Description string  `json:"description,omitempty"`
}

func ParseDimensions(raw string) ([]Dimension, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var dims []Dimension
	if err := json.Unmarshal([]byte(raw), &dims); err != nil {
		return nil, err
	}
	var sum float64
	for _, d := range dims {
		if d.Weight <= 0 {
			return nil, fmt.Errorf("dimension %q has non-positive weight %v", d.Name, d.Weight)
		}
		sum += d.Weight
	}
	if sum > 0 && sum != 1 {
		for i := range dims {
			dims[i].Weight = dims[i].Weight / sum
		}
	}
	return dims, nil
}

type DimensionReport struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Weight      float64            `json:"weight"`
	Average     float64            `json:"average"`
	ByJudgeType map[string]float64 `json:"by_judge_type"`
}

type JudgeTypeSummary struct {
	Count   int     `json:"count"`
	Overall float64 `json:"overall"`
}

type Report struct {
	EvaluationID string                     `json:"evaluation_id"`
	JudgeMode    string                     `json:"judge_mode"`
	Status       string                     `json:"status"`
	Verdict      string                     `json:"verdict"`
	Dimensions   []DimensionReport          `json:"dimensions"`
	Overall      float64                    `json:"overall"`
	ByJudgeType  map[string]JudgeTypeSummary `json:"by_judge_type"`
	Reviews      []Review                   `json:"reviews"`
	Criteria     string                     `json:"criteria,omitempty"`
	GeneratedAt  time.Time                  `json:"generated_at"`
}

type Evaluation struct {
	ID           string
	Name         string
	Task         string
	Dataset      string
	ExperimentID string
	RunID        string
	ModelID      string
	Criteria     string
	Metrics      string
	Score        float64
	Passed       bool
	Status       string
	FailReason   string
	TenantID     string
	CreatedBy    string
	JudgeMode    string
	Dimensions   string
	Report       string
	CreatedAt    time.Time
}

type Review struct {
	ID           string    `json:"id"`
	EvaluationID string    `json:"evaluation_id"`
	TenantID     string    `json:"tenant_id"`
	JudgeType    string    `json:"judge_type"`
	JudgeID      string    `json:"judge_id"`
	Scores       string    `json:"scores"`
	Comment      string    `json:"comment,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func New(input CreateInput) *Evaluation {
	mode := input.JudgeMode
	if mode == "" {
		mode = JudgeModeThreshold
	}
	return &Evaluation{
		ID:           input.ID,
		Name:         input.Name,
		Task:         input.Task,
		Dataset:      input.Dataset,
		ExperimentID: input.ExperimentID,
		RunID:        input.RunID,
		ModelID:      input.ModelID,
		Criteria:     input.Criteria,
		Status:       StatusPending,
		TenantID:     input.TenantID,
		CreatedBy:    input.CreatedBy,
		JudgeMode:    mode,
		Dimensions:   input.Dimensions,
	}
}

type CreateInput struct {
	ID           string
	Name         string
	Task         string
	Dataset      string
	ExperimentID string
	RunID        string
	ModelID      string
	Criteria     string 
	TenantID     string
	CreatedBy    string
	
	JudgeMode string
	
	Dimensions string
}

func (e *Evaluation) MarkRunning() error {
	if e.Status != StatusPending {
		return errors.New("evaluation must be pending to start running")
	}
	e.Status = StatusRunning
	return nil
}

func (e *Evaluation) RecordResult(metrics map[string]float64, score float64) error {
	if e.Status == StatusCompleted || e.Status == StatusFailed {
		return errors.New("evaluation already finalized")
	}
	e.Metrics = mustJSON(metrics)
	e.Score = score
	e.Status = StatusCompleted
	e.Passed = e.evaluate(metrics)
	return nil
}

func (e *Evaluation) Fail(reason string) error {
	if e.Status == StatusCompleted || e.Status == StatusFailed {
		return errors.New("evaluation already finalized")
	}
	e.Status = StatusFailed
	e.FailReason = reason
	e.Passed = false
	return nil
}

func (e *Evaluation) AddReview(r Review) error {
	if e.Status == StatusCompleted || e.Status == StatusFailed {
		return errors.New("evaluation already finalized")
	}
	mode := e.JudgeMode
	if mode == "" {
		mode = JudgeModeThreshold
	}
	switch mode {
	case JudgeModeThreshold:
		return errors.New("threshold-mode evaluation does not accept reviews")
	case JudgeModeHuman:
		if r.JudgeType != JudgeTypeHuman {
			return errors.New("human-mode evaluation only accepts human reviews")
		}
	case JudgeModeLLM:
		if r.JudgeType != JudgeTypeLLM {
			return errors.New("llm-mode evaluation only accepts llm reviews")
		}
	case JudgeModeHybrid:
		if r.JudgeType != JudgeTypeHuman && r.JudgeType != JudgeTypeLLM {
			return errors.New("hybrid-mode evaluation accepts only human or llm reviews")
		}
	default:
		return fmt.Errorf("unknown judge mode %q", mode)
	}
	if r.JudgeType == "" || r.JudgeID == "" {
		return errors.New("review judge_type and judge_id are required")
	}
	return nil
}

func (e *Evaluation) AggregateReport(reviews []Review) (Report, error) {
	dims, err := ParseDimensions(e.Dimensions)
	if err != nil {
		return Report{}, fmt.Errorf("parse dimensions: %w", err)
	}
	if len(dims) == 0 {
		return Report{}, errors.New("no dimensions defined for report aggregation")
	}

	if err := validateCriteriaDimensions(e.Criteria, dims); err != nil {
		return Report{}, err
	}

	mode := e.JudgeMode
	if mode == "" {
		mode = JudgeModeThreshold
	}
	rep := Report{
		EvaluationID: e.ID,
		JudgeMode:    mode,
		Status:       e.Status,
		ByJudgeType:  map[string]JudgeTypeSummary{},
		Reviews:      reviews,
		Criteria:     e.Criteria,
		GeneratedAt:  time.Now(),
	}

	type acc struct{ sum, count float64 }
	for _, d := range dims {
		dr := DimensionReport{
			Name:        d.Name,
			Description: d.Description,
			Weight:      d.Weight,
			ByJudgeType: map[string]float64{},
		}
		all := acc{}
		byType := map[string]acc{}
		for _, rv := range reviews {
			scores := map[string]float64{}
			_ = json.Unmarshal([]byte(rv.Scores), &scores)
			v, ok := scores[d.Name]
			if !ok {
				continue
			}
			all.sum += v
			all.count++
			a := byType[rv.JudgeType]
			a.sum += v
			a.count++
			byType[rv.JudgeType] = a
		}
		if all.count > 0 {
			dr.Average = all.sum / all.count
		}
		for jt, a := range byType {
			if a.count > 0 {
				dr.ByJudgeType[jt] = a.sum / a.count
			}
		}
		rep.Dimensions = append(rep.Dimensions, dr)
		rep.Overall += dr.Average * dr.Weight
	}

	type typeAcc struct{ sum, count float64 }
	ta := map[string]typeAcc{}
	for _, rv := range reviews {
		scores := map[string]float64{}
		_ = json.Unmarshal([]byte(rv.Scores), &scores)
		var ro float64
		for _, d := range dims {
			ro += scores[d.Name] * d.Weight
		}
		a := ta[rv.JudgeType]
		a.sum += ro
		a.count++
		ta[rv.JudgeType] = a
	}
	for jt, a := range ta {
		s := JudgeTypeSummary{Count: int(a.count)}
		if a.count > 0 {
			s.Overall = a.sum / a.count
		}
		rep.ByJudgeType[jt] = s
	}

	rep.Verdict = e.verdict(rep.Overall, len(reviews) > 0)
	return rep, nil
}

func validateCriteriaDimensions(raw string, dims []Dimension) error {
	criteria, err := parseCriteria(raw)
	if err != nil {
		return fmt.Errorf("parse criteria: %w", err)
	}
	if len(criteria) == 0 {
		return nil
	}
	defined := make(map[string]bool, len(dims))
	for _, d := range dims {
		defined[d.Name] = true
	}
	reserved := map[string]bool{"overall": true, "score": true}
	for name := range criteria {
		if reserved[name] {
			continue
		}
		if !defined[name] {
			names := make([]string, 0, len(dims))
			for _, d := range dims {
				names = append(names, d.Name)
			}
			return fmt.Errorf("criteria references undefined dimension %q (available: %v)", name, names)
		}
	}
	return nil
}

func (e *Evaluation) verdict(overall float64, hasReviews bool) string {
	criteria, err := parseCriteria(e.Criteria)
	if err == nil {
		if c, ok := criteria["overall"]; ok && satisfies(overall, c) {
			return VerdictPass
		} else if ok {
			return VerdictFail
		}
		if c, ok := criteria["score"]; ok && satisfies(overall, c) {
			return VerdictPass
		} else if ok {
			return VerdictFail
		}
	}
	if hasReviews {
		return VerdictPass
	}
	return VerdictPending
}

func (e *Evaluation) Finalize(report Report) error {
	if e.Status == StatusCompleted || e.Status == StatusFailed {
		return errors.New("evaluation already finalized")
	}
	e.Report = mustJSON(report)
	e.Score = report.Overall
	e.Status = StatusCompleted
	e.Passed = report.Verdict == VerdictPass
	return nil
}

func (e *Evaluation) evaluate(metrics map[string]float64) bool {
	criteria, err := parseCriteria(e.Criteria)
	if err != nil || len(criteria) == 0 {
		
		return len(criteria) == 0
	}
	for name, c := range criteria {
		v, ok := metrics[name]
		if !ok {
			return false 
		}
		if !satisfies(v, c) {
			return false
		}
	}
	return true
}

func satisfies(v float64, c criterion) bool {
	op := c.Op
	switch {
	case op == OpGTE || op == ">=":
		return v >= c.Value
	case op == OpLTE || op == "<=":
		return v <= c.Value
	case op == OpGT || op == ">":
		return v > c.Value
	case op == OpLT || op == "<":
		return v < c.Value
	case op == OpEQ || op == "==":
		return v == c.Value
	default:
		return false
	}
}

func ParseCriteria(raw string) (map[string]criterion, error) {
	return parseCriteria(raw)
}

func parseCriteria(raw string) (map[string]criterion, error) {
	out := map[string]criterion{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}