package llm

import (
	"errors"
	"testing"
	"time"
)

var base = time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)

func TestTraceSpanLifecycle(t *testing.T) {
	tr := NewTrace("t1", "qwen", base)
	tr.StartSpan(SpanInference, base)
	if err := tr.EndSpan(SpanInference, base.Add(2*time.Second), nil); err != nil {
		t.Fatalf("EndSpan: %v", err)
	}
	s, ok := tr.SpanByName(SpanInference)
	if !ok {
		t.Fatal("span not recorded")
	}
	if s.Elapsed != 2*time.Second {
		t.Fatalf("Elapsed = %v, want 2s", s.Elapsed)
	}
}

func TestTraceEndUnstartedSpan(t *testing.T) {
	tr := NewTrace("t1", "qwen", base)
	if err := tr.EndSpan("ghost", base, nil); err != ErrSpanNotStarted {
		t.Fatalf("want ErrSpanNotStarted, got %v", err)
	}
}

func TestTraceSpanRecordsError(t *testing.T) {
	tr := NewTrace("t1", "qwen", base)
	tr.StartSpan(SpanRetrieval, base)
	_ = tr.EndSpan(SpanRetrieval, base.Add(time.Second), errors.New("vector store down"))

	s, _ := tr.SpanByName(SpanRetrieval)
	if s.Error != "vector store down" {
		t.Fatalf("Error = %q", s.Error)
	}
}

func TestTraceFirstTokenIsRecordedOnce(t *testing.T) {
	tr := NewTrace("t1", "qwen", base)
	tr.MarkFirstToken(base.Add(200 * time.Millisecond))
	tr.MarkFirstToken(base.Add(5 * time.Second))

	if tr.Latency.TTFT != 200*time.Millisecond {
		t.Fatalf("TTFT = %v, want 200ms", tr.Latency.TTFT)
	}
}

func TestTraceFinishComputesLatency(t *testing.T) {
	tr := NewTrace("t1", "qwen", base)
	tr.MarkFirstToken(base.Add(1 * time.Second))
	tr.Finish(base.Add(11*time.Second), Usage{PromptTokens: 10, CompletionTokens: 101}, 0.5, nil)

	if tr.Latency.Total != 11*time.Second {
		t.Fatalf("Total = %v, want 11s", tr.Latency.Total)
	}
	
	if tr.Latency.TPOT != 100*time.Millisecond {
		t.Fatalf("TPOT = %v, want 100ms", tr.Latency.TPOT)
	}
	
	if got := tr.Latency.TokensPerSecond; got < 9.1 || got > 9.2 {
		t.Fatalf("TokensPerSecond = %v, want ~9.18", got)
	}
	if !tr.Succeeded() {
		t.Fatal("trace should have succeeded")
	}
}

func TestTraceTPOTExcludesPrefill(t *testing.T) {
	tr := NewTrace("t1", "qwen", base)
	
	tr.MarkFirstToken(base.Add(9 * time.Second))
	tr.Finish(base.Add(10*time.Second), Usage{CompletionTokens: 11}, 0, nil)

	if tr.Latency.TPOT != 100*time.Millisecond {
		t.Fatalf("TPOT = %v, want 100ms (prefill must be excluded)", tr.Latency.TPOT)
	}
}

func TestTraceTPOTSingleToken(t *testing.T) {
	tr := NewTrace("t1", "qwen", base)
	tr.MarkFirstToken(base.Add(time.Second))
	tr.Finish(base.Add(2*time.Second), Usage{CompletionTokens: 1}, 0, nil)
	if tr.Latency.TPOT != 0 {
		t.Fatalf("TPOT = %v, want 0 for single token", tr.Latency.TPOT)
	}
}

func TestTraceWithoutFirstToken(t *testing.T) {
	tr := NewTrace("t1", "qwen", base)
	tr.Finish(base.Add(3*time.Second), Usage{CompletionTokens: 30}, 0, nil)
	if tr.Latency.TPOT != 0 {
		t.Fatalf("TPOT = %v, want 0 when first token unmarked", tr.Latency.TPOT)
	}
	if tr.Latency.TokensPerSecond != 10 {
		t.Fatalf("TokensPerSecond = %v, want 10", tr.Latency.TokensPerSecond)
	}
}

func TestTraceFinishNormalizesUsage(t *testing.T) {
	tr := NewTrace("t1", "qwen", base)
	tr.Finish(base.Add(time.Second), Usage{PromptTokens: 7, CompletionTokens: 3}, 0, nil)
	if tr.Usage.TotalTokens != 10 {
		t.Fatalf("TotalTokens = %d, want 10", tr.Usage.TotalTokens)
	}
}

func TestTraceFinishClosesDanglingSpans(t *testing.T) {
	tr := NewTrace("t1", "qwen", base)
	tr.StartSpan(SpanInference, base)
	tr.Finish(base.Add(time.Second), Usage{}, 0, nil)

	s, ok := tr.SpanByName(SpanInference)
	if !ok {
		t.Fatal("dangling span was dropped")
	}
	if s.Error == "" {
		t.Fatal("dangling span should be marked with an error")
	}
}

func TestTraceIsImmutableAfterFinish(t *testing.T) {
	tr := NewTrace("t1", "qwen", base)
	tr.Finish(base.Add(time.Second), Usage{CompletionTokens: 5}, 1.0, nil)

	tr.StartSpan("late", base)
	if err := tr.EndSpan("late", base, nil); err != ErrTraceFinished {
		t.Fatalf("want ErrTraceFinished, got %v", err)
	}
	tr.AddFindings(Finding{Rule: "late"})
	if len(tr.Findings) != 0 {
		t.Fatal("findings added after finish")
	}
	tr.Finish(base.Add(99*time.Second), Usage{CompletionTokens: 999}, 99, nil)
	if tr.Usage.CompletionTokens != 5 || tr.Cost != 1.0 {
		t.Fatalf("trace mutated after finish: %+v", tr.Usage)
	}
}

func TestTraceRecordsError(t *testing.T) {
	tr := NewTrace("t1", "qwen", base)
	tr.Finish(base.Add(time.Second), Usage{}, 0, errors.New("upstream 503"))
	if tr.Succeeded() {
		t.Fatal("failed trace reported as succeeded")
	}
	if tr.Error != "upstream 503" {
		t.Fatalf("Error = %q", tr.Error)
	}
}

func TestTraceAddFindings(t *testing.T) {
	tr := NewTrace("t1", "qwen", base)
	tr.AddFindings(Finding{Category: CategoryPII, Rule: "pii_phone_cn"})
	if len(tr.Findings) != 1 {
		t.Fatalf("Findings len = %d, want 1", len(tr.Findings))
	}
	tr.AddFindings()
	if len(tr.Findings) != 1 {
		t.Fatal("empty AddFindings should be a no-op")
	}
}