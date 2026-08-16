package hpo

import (
	"math"
	"math/rand"
)

const tpeGamma = 0.25

const tpeMinSamples = 8

type TPESampler struct {
	
	Objective Objective
}

func (s TPESampler) Suggest(space SearchSpace, history []Trial, r *rand.Rand) (map[string]any, error) {
	if err := space.Validate(); err != nil {
		return nil, err
	}
	
	if len(history) < tpeMinSamples {
		return RandomSampler{}.Suggest(space, history, r)
	}

	good, bad := splitGoodBad(history, s.Objective)
	params := make(map[string]any, len(space.Params))
	for _, p := range space.Params {
		params[p.Name] = tpeSampleParam(p, good, bad, r)
	}
	return params, nil
}

func splitGoodBad(history []Trial, obj Objective) (good, bad []map[string]any) {
	type scored struct {
		params map[string]any
		value  float64
	}
	valid := make([]scored, 0, len(history))
	for _, tr := range history {
		if tr.Value == nil || math.IsNaN(*tr.Value) || math.IsInf(*tr.Value, 0) {
			continue
		}
		valid = append(valid, scored{params: tr.Params, value: *tr.Value})
	}
	if len(valid) == 0 {
		return nil, nil
	}
	
	sign := obj.sign()
	for i := range valid {
		valid[i].value *= sign
	}
	for i := 1; i < len(valid); i++ {
		for j := i; j > 0 && valid[j].value > valid[j-1].value; j-- {
			valid[j], valid[j-1] = valid[j-1], valid[j]
		}
	}
	k := int(math.Max(1, math.Floor(tpeGamma*float64(len(valid)))))
	good = make([]map[string]any, 0, k)
	bad = make([]map[string]any, 0, len(valid)-k)
	for i, v := range valid {
		if i < k {
			good = append(good, v.params)
		} else {
			bad = append(bad, v.params)
		}
	}
	return good, bad
}

func tpeSampleParam(p ParamSpec, good, bad []map[string]any, r *rand.Rand) any {
	switch p.Type {
	case ParamFloat, ParamInt:
		return tpeSampleNumeric(p, good, bad, r)
	case ParamCategorical, ParamBool:
		return tpeSampleCategorical(p, good, bad, r)
	default:
		return sampleParam(p, r)
	}
}

func tpeWeightedPick(weights []float64, r *rand.Rand) int {
	var sum float64
	for _, w := range weights {
		sum += w
	}
	if sum <= 0 {
		return r.Intn(len(weights))
	}
	x := r.Float64() * sum
	for i, w := range weights {
		x -= w
		if x <= 0 {
			return i
		}
	}
	return len(weights) - 1
}

func tpeSampleNumeric(p ParamSpec, good, bad []map[string]any, r *rand.Rand) any {
	
	const bins = 50
	weights := make([]float64, bins)
	for i := 0; i < bins; i++ {
		x := p.Min + (p.Max-p.Min)*float64(i)/float64(bins-1)
		if p.LogScale && p.Min > 0 {
			lo, hi := math.Log10(p.Min), math.Log10(p.Max)
			x = math.Pow(10, lo+(hi-lo)*float64(i)/float64(bins-1))
		}
		g := kernelCount(good, p.Name, x)
		b := kernelCount(bad, p.Name, x)
		const prior = 1.0
		weights[i] = (g + prior) / (b + prior)
	}
	idx := tpeWeightedPick(weights, r)
	x := p.Min + (p.Max-p.Min)*float64(idx)/float64(bins-1)
	if p.LogScale && p.Min > 0 {
		lo, hi := math.Log10(p.Min), math.Log10(p.Max)
		x = math.Pow(10, lo+(hi-lo)*float64(idx)/float64(bins-1))
	}
	return numericToType(p, x)
}

func kernelCount(group []map[string]any, name string, x float64) float64 {
	var c float64
	const bw = 0.1 
	for _, m := range group {
		v, ok := toFloat(m[name])
		if !ok {
			continue
		}
		d := (v - x)
		c += math.Exp(-(d * d) / (2 * bw * bw))
	}
	return c
}

func tpeSampleCategorical(p ParamSpec, good, bad []map[string]any, r *rand.Rand) any {
	choices := p.choicesFor()
	weights := make([]float64, len(choices))
	for i, c := range choices {
		g, b := countEq(good, p.Name, c), countEq(bad, p.Name, c)
		const prior = 1.0
		weights[i] = (g + prior) / (b + prior)
	}
	return choices[tpeWeightedPick(weights, r)]
}

func countEq(group []map[string]any, name string, target any) float64 {
	var c float64
	for _, m := range group {
		if m[name] == target {
			c++
		}
	}
	return c
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}