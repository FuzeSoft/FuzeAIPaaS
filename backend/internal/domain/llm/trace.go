package llm

import (
	"errors"
	"sort"
	"time"
)

const (
	SpanGuardInput  = "guard_input"
	SpanRetrieval   = "retrieval"
	SpanPromptBuild = "prompt_build"
	SpanInference   = "inference"
	SpanGuardOutput = "guard_output"
)

var (
	
	ErrSpanNotStarted = errors.New("llm: span was not started")
	
	ErrTraceFinished = errors.New("llm: trace already finished")
)

type Span struct {
	Name    string        `json:"name"`
	Start   time.Time     `json:"start"`
	Elapsed time.Duration `json:"elapsed"`
	
	Error string `json:"error,omitempty"`
}

type LatencyStats struct {
	
	TTFT time.Duration `json:"ttft"`
	
	TPOT time.Duration `json:"tpot"`
	
	Total time.Duration `json:"total"`
	
	TokensPerSecond float64 `json:"tokens_per_second"`
}

type Trace struct {
	
	ID string `json:"id"`
	
	TenantID string `json:"tenant_id,omitempty"`
	
	UserID string `json:"user_id,omitempty"`
	
	Model string `json:"model"`
	
	Backend string `json:"backend,omitempty"`
	
	Spans []Span `json:"spans"`
	
	Usage Usage `json:"usage"`
	
	Cost float64 `json:"cost"`
	
	Latency LatencyStats `json:"latency"`
	
	Findings []Finding `json:"findings,omitempty"`
	
	Error string `json:"error,omitempty"`
	
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`

	open map[string]time.Time
	
	firstToken time.Time
	
	finished bool
}

func NewTrace(id, model string, now time.Time) *Trace {
	return &Trace{
		ID:        id,
		Model:     model,
		StartedAt: now,
		open:      make(map[string]time.Time),
	}
}

func (t *Trace) StartSpan(name string, now time.Time) {
	if t.finished {
		return
	}
	if t.open == nil {
		t.open = make(map[string]time.Time)
	}
	t.open[name] = now
}

func (t *Trace) EndSpan(name string, now time.Time, err error) error {
	if t.finished {
		return ErrTraceFinished
	}
	start, ok := t.open[name]
	if !ok {
		return ErrSpanNotStarted
	}
	delete(t.open, name)
	span := Span{Name: name, Start: start, Elapsed: now.Sub(start)}
	if err != nil {
		span.Error = err.Error()
	}
	t.Spans = append(t.Spans, span)
	return nil
}

func (t *Trace) MarkFirstToken(now time.Time) {
	if t.finished || !t.firstToken.IsZero() {
		return
	}
	t.firstToken = now
	t.Latency.TTFT = now.Sub(t.StartedAt)
}

func (t *Trace) AddFindings(f ...Finding) {
	if t.finished || len(f) == 0 {
		return
	}
	t.Findings = append(t.Findings, f...)
}

func (t *Trace) Finish(now time.Time, usage Usage, cost float64, err error) {
	if t.finished {
		return
	}
	
	names := make([]string, 0, len(t.open))
	for n := range t.open {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		_ = t.EndSpan(n, now, errors.New("span not closed"))
	}

	t.FinishedAt = now
	t.Usage = usage.Normalize()
	t.Cost = cost
	if err != nil {
		t.Error = err.Error()
	}

	t.Latency.Total = now.Sub(t.StartedAt)
	
	if n := t.Usage.CompletionTokens; n > 1 && !t.firstToken.IsZero() {
		decode := now.Sub(t.firstToken)
		t.Latency.TPOT = decode / time.Duration(n-1)
	}
	if secs := t.Latency.Total.Seconds(); secs > 0 && t.Usage.CompletionTokens > 0 {
		t.Latency.TokensPerSecond = float64(t.Usage.CompletionTokens) / secs
	}
	t.finished = true
}

func (t *Trace) Finished() bool { return t.finished }

func (t *Trace) SpanByName(name string) (Span, bool) {
	for _, s := range t.Spans {
		if s.Name == name {
			return s, true
		}
	}
	return Span{}, false
}

func (t *Trace) Succeeded() bool { return t.finished && t.Error == "" }