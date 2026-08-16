package llm

import (
	"errors"
	"fmt"
	"strings"
)

var (
	
	ErrNegativePrice = errors.New("llm: price must not be negative")
	
	ErrTokenQuotaExceeded = errors.New("llm: token quota exceeded")
)

type Price struct {
	Model string `json:"model"`
	
	InputPer1K float64 `json:"input_per_1k"`
	
	OutputPer1K float64 `json:"output_per_1k"`
	
	Currency string `json:"currency"`
}

func (p Price) Validate() error {
	if strings.TrimSpace(p.Model) == "" {
		return ErrEmptyModel
	}
	if p.InputPer1K < 0 || p.OutputPer1K < 0 {
		return ErrNegativePrice
	}
	return nil
}

func (p Price) Cost(u Usage) float64 {
	return float64(u.PromptTokens)/1000*p.InputPer1K +
		float64(u.CompletionTokens)/1000*p.OutputPer1K
}

type TokenQuota struct {
	TenantID string `json:"tenant_id"`
	
	LimitTokens int64 `json:"limit_tokens"`
	
	UsedTokens int64 `json:"used_tokens"`
	
	LimitCost float64 `json:"limit_cost"`
	
	UsedCost float64 `json:"used_cost"`
}

func (q TokenQuota) Remaining() int64 {
	if q.LimitTokens <= 0 {
		return -1
	}
	if rem := q.LimitTokens - q.UsedTokens; rem > 0 {
		return rem
	}
	return 0
}

func (q TokenQuota) CanConsume(n int64) bool {
	if n <= 0 {
		return true
	}
	if q.LimitTokens <= 0 {
		return true
	}
	return q.UsedTokens+n <= q.LimitTokens
}

func (q TokenQuota) CanSpend(amount float64) bool {
	if amount <= 0 {
		return true
	}
	if q.LimitCost <= 0 {
		return true
	}
	return q.UsedCost+amount <= q.LimitCost
}

type PriceBook struct {
	prices map[string]Price
	
	fallback Price
	
	gpuPrices map[string]float64
	
	gpuCurrency string
}

func NewPriceBook() *PriceBook {
	return &PriceBook{prices: make(map[string]Price), gpuPrices: make(map[string]float64)}
}

func NewPriceBookFromConfig(getenv func(string) string) *PriceBook {
	b := NewPriceBook()
	if getenv == nil {
		return b
	}
	in := atofOrDefault(getenv("LLM_DEFAULT_INPUT_PER_1K"), 0)
	out := atofOrDefault(getenv("LLM_DEFAULT_OUTPUT_PER_1K"), 0)
	cur := getenv("LLM_DEFAULT_CURRENCY")
	if cur == "" {
		cur = "CNY"
	}
	
	if in > 0 || out > 0 {
		_ = b.SetFallback(Price{InputPer1K: in, OutputPer1K: out, Currency: cur})
	}
	return b
}

func atofOrDefault(s string, def float64) float64 {
	if s == "" {
		return def
	}
	var v float64
	if _, err := fmt.Sscanf(s, "%f", &v); err != nil {
		return def
	}
	if v < 0 {
		return def
	}
	return v
}

func (b *PriceBook) Set(p Price) error {
	if err := p.Validate(); err != nil {
		return err
	}
	b.prices[p.Model] = p
	return nil
}

func (b *PriceBook) SetFallback(p Price) error {
	if p.InputPer1K < 0 || p.OutputPer1K < 0 {
		return ErrNegativePrice
	}
	b.fallback = p
	return nil
}

func (b *PriceBook) Lookup(model string) (Price, bool) {
	if p, ok := b.prices[model]; ok {
		return p, true
	}
	
	best := ""
	for name := range b.prices {
		if strings.HasPrefix(model, name) && len(name) > len(best) {
			best = name
		}
	}
	if best != "" {
		return b.prices[best], true
	}
	return b.fallback, false
}

func (b *PriceBook) Cost(model string, u Usage) float64 {
	p, _ := b.Lookup(model)
	return p.Cost(u)
}

func (b *PriceBook) List() []Price {
	out := make([]Price, 0, len(b.prices))
	for _, p := range b.prices {
		out = append(out, p)
	}
	return out
}

func (b *PriceBook) SetGPUPrice(gpuType string, perGPUHour float64, currency string) error {
	if perGPUHour < 0 {
		return ErrNegativePrice
	}
	b.gpuPrices[gpuType] = perGPUHour
	if currency != "" {
		b.gpuCurrency = currency
	}
	return nil
}

func (b *PriceBook) GPUPerHour(gpuType string) (float64, bool) {
	if v, ok := b.gpuPrices[gpuType]; ok {
		return v, true
	}
	if v, ok := b.gpuPrices[""]; ok {
		return v, true
	}
	return 0, false
}

func (b *PriceBook) GPUCurrency() string { return b.gpuCurrency }

func (b *PriceBook) GPUCost(gpuType string, gpuCount int, hours float64) float64 {
	if gpuCount <= 0 || hours <= 0 {
		return 0
	}
	per, ok := b.GPUPerHour(gpuType)
	if !ok {
		return 0
	}
	return per * float64(gpuCount) * hours
}