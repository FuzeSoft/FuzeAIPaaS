package llm

import (
	"math"
	"testing"
)

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPriceValidate(t *testing.T) {
	if err := (Price{Model: "m", InputPer1K: 1, OutputPer1K: 2}).Validate(); err != nil {
		t.Fatalf("valid price rejected: %v", err)
	}
	if err := (Price{InputPer1K: 1}).Validate(); err != ErrEmptyModel {
		t.Fatalf("want ErrEmptyModel, got %v", err)
	}
	if err := (Price{Model: "m", InputPer1K: -1}).Validate(); err != ErrNegativePrice {
		t.Fatalf("want ErrNegativePrice, got %v", err)
	}
	if err := (Price{Model: "m", OutputPer1K: -1}).Validate(); err != ErrNegativePrice {
		t.Fatalf("want ErrNegativePrice for output, got %v", err)
	}
}

func TestPriceCostSeparatesInputAndOutput(t *testing.T) {
	p := Price{Model: "m", InputPer1K: 1.0, OutputPer1K: 3.0}
	got := p.Cost(Usage{PromptTokens: 1000, CompletionTokens: 1000})
	if !almost(got, 4.0) {
		t.Fatalf("Cost = %v, want 4.0", got)
	}
	
	if got := p.Cost(Usage{PromptTokens: 500}); !almost(got, 0.5) {
		t.Fatalf("Cost(input only) = %v, want 0.5", got)
	}
	
	if got := p.Cost(Usage{}); !almost(got, 0) {
		t.Fatalf("Cost(zero) = %v, want 0", got)
	}
}

func TestTokenQuotaCanConsume(t *testing.T) {
	cases := []struct {
		name  string
		quota TokenQuota
		n     int64
		want  bool
	}{
		{"within limit", TokenQuota{LimitTokens: 100, UsedTokens: 10}, 50, true},
		{"exactly at limit", TokenQuota{LimitTokens: 100, UsedTokens: 50}, 50, true},
		{"exceeds limit", TokenQuota{LimitTokens: 100, UsedTokens: 60}, 50, false},
		{"unlimited", TokenQuota{LimitTokens: 0, UsedTokens: 1e9}, 1e6, true},
		{"zero consume", TokenQuota{LimitTokens: 100, UsedTokens: 100}, 0, true},
		{"already over", TokenQuota{LimitTokens: 100, UsedTokens: 200}, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.quota.CanConsume(tc.n); got != tc.want {
				t.Fatalf("CanConsume(%d) = %v, want %v", tc.n, got, tc.want)
			}
		})
	}
}

func TestTokenQuotaCanSpend(t *testing.T) {
	q := TokenQuota{LimitCost: 10}
	if !q.CanSpend(10) {
		t.Fatal("spending exactly the limit must be allowed")
	}
	if q.CanSpend(10.01) {
		t.Fatal("spending beyond the limit must be rejected")
	}
	if !(TokenQuota{LimitCost: 0}).CanSpend(1e6) {
		t.Fatal("zero LimitCost means unlimited")
	}
}

func TestTokenQuotaRemaining(t *testing.T) {
	if got := (TokenQuota{LimitTokens: 0}).Remaining(); got != -1 {
		t.Fatalf("unlimited Remaining() = %d, want -1", got)
	}
	if got := (TokenQuota{LimitTokens: 100, UsedTokens: 30}).Remaining(); got != 70 {
		t.Fatalf("Remaining() = %d, want 70", got)
	}
	
	if got := (TokenQuota{LimitTokens: 100, UsedTokens: 150}).Remaining(); got != 0 {
		t.Fatalf("over-consumed Remaining() = %d, want 0", got)
	}
}

func TestPriceBookExactLookup(t *testing.T) {
	b := NewPriceBook()
	if err := b.Set(Price{Model: "qwen2-7b", InputPer1K: 1, OutputPer1K: 2}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	p, ok := b.Lookup("qwen2-7b")
	if !ok || !almost(p.InputPer1K, 1) {
		t.Fatalf("Lookup = %+v, ok=%v", p, ok)
	}
}

func TestPriceBookRejectsInvalid(t *testing.T) {
	b := NewPriceBook()
	if err := b.Set(Price{Model: "m", InputPer1K: -1}); err != ErrNegativePrice {
		t.Fatalf("want ErrNegativePrice, got %v", err)
	}
	if err := b.SetFallback(Price{OutputPer1K: -1}); err != ErrNegativePrice {
		t.Fatalf("want ErrNegativePrice for fallback, got %v", err)
	}
}

func TestPriceBookPrefixInheritance(t *testing.T) {
	b := NewPriceBook()
	_ = b.Set(Price{Model: "qwen2-7b", InputPer1K: 1, OutputPer1K: 2})

	p, ok := b.Lookup("qwen2-7b:my-lora")
	if !ok {
		t.Fatal("derived model should inherit base price")
	}
	if !almost(p.OutputPer1K, 2) {
		t.Fatalf("inherited OutputPer1K = %v, want 2", p.OutputPer1K)
	}
}

func TestPriceBookPrefersLongestPrefix(t *testing.T) {
	b := NewPriceBook()
	_ = b.Set(Price{Model: "qwen", InputPer1K: 1, OutputPer1K: 1})
	_ = b.Set(Price{Model: "qwen2-72b", InputPer1K: 9, OutputPer1K: 9})

	p, _ := b.Lookup("qwen2-72b-chat")
	if !almost(p.InputPer1K, 9) {
		t.Fatalf("InputPer1K = %v, want 9 (longest prefix wins)", p.InputPer1K)
	}
}

func TestPriceBookFallback(t *testing.T) {
	b := NewPriceBook()
	_ = b.SetFallback(Price{InputPer1K: 0.5, OutputPer1K: 1.5})

	p, ok := b.Lookup("unknown-model")
	if ok {
		t.Fatal("Lookup should report miss for unregistered model")
	}
	if !almost(p.OutputPer1K, 1.5) {
		t.Fatalf("fallback OutputPer1K = %v, want 1.5", p.OutputPer1K)
	}
	got := b.Cost("unknown-model", Usage{PromptTokens: 1000, CompletionTokens: 1000})
	if !almost(got, 2.0) {
		t.Fatalf("Cost = %v, want 2.0", got)
	}
}

func TestPriceBookList(t *testing.T) {
	b := NewPriceBook()
	_ = b.Set(Price{Model: "a", InputPer1K: 1})
	_ = b.Set(Price{Model: "b", InputPer1K: 2})
	if got := b.List(); len(got) != 2 {
		t.Fatalf("List() len = %d, want 2", len(got))
	}
}

func TestNewPriceBookFromConfigRegistersFallback(t *testing.T) {
	env := func(k string) string {
		m := map[string]string{
			"LLM_DEFAULT_INPUT_PER_1K":  "0.1",
			"LLM_DEFAULT_OUTPUT_PER_1K": "0.2",
			"LLM_DEFAULT_CURRENCY":      "USD",
		}
		return m[k]
	}
	b := NewPriceBookFromConfig(env)
	p, ok := b.Lookup("some-unregistered-model")
	if ok {
		t.Fatal("Lookup should report miss for unregistered model")
	}
	if p.Currency != "USD" {
		t.Fatalf("fallback currency = %q, want USD", p.Currency)
	}
	
	if got := b.Cost("some-unregistered-model", Usage{PromptTokens: 1000, CompletionTokens: 1000}); !almost(got, 0.3) {
		t.Fatalf("config fallback Cost = %v, want 0.3", got)
	}
}

func TestNewPriceBookFromConfigNoEnvMeansNoFallback(t *testing.T) {
	b := NewPriceBookFromConfig(func(string) string { return "" })
	p, _ := b.Lookup("x")
	if got := b.Cost("x", Usage{PromptTokens: 1000, CompletionTokens: 1000}); !almost(got, 0) {
		t.Fatalf("empty config Cost = %v, want 0", got)
	}
	if p.InputPer1K != 0 || p.OutputPer1K != 0 {
		t.Fatalf("empty config fallback price = %+v, want zero", p)
	}
}

func TestNewPriceBookFromConfigBadEnvFallsBack(t *testing.T) {
	env := func(k string) string {
		if k == "LLM_DEFAULT_INPUT_PER_1K" {
			return "not-a-number"
		}
		return ""
	}
	b := NewPriceBookFromConfig(env)
	if got := b.Cost("x", Usage{PromptTokens: 1000, CompletionTokens: 1000}); !almost(got, 0) {
		t.Fatalf("bad-env Cost = %v, want 0", got)
	}
}