package hpo

import (
	"math"
	"math/rand"
)

type Sampler interface {
	
	Suggest(space SearchSpace, history []Trial, r *rand.Rand) (map[string]any, error)
}

var ErrExhausted = SamplerExhaustedError{}

type SamplerExhaustedError struct{}

func (SamplerExhaustedError) Error() string { return "search space exhausted: no more candidates" }

func sampleNumeric(p ParamSpec, r *rand.Rand) float64 {
	if p.LogScale {
		lo, hi := math.Log10(p.Min), math.Log10(p.Max)
		return math.Pow(10, lo+(hi-lo)*r.Float64())
	}
	v := p.Min + (p.Max-p.Min)*r.Float64()
	
	if p.Step > 0 {
		steps := math.Round((v - p.Min) / p.Step)
		v = p.Min + steps*p.Step
		if v > p.Max {
			v = p.Max
		}
	}
	return v
}

func numericToType(p ParamSpec, v float64) any {
	if p.Type == ParamInt {
		
		return int(math.Round(v))
	}
	return v
}

func sampleParam(p ParamSpec, r *rand.Rand) any {
	switch p.Type {
	case ParamFloat, ParamInt:
		return numericToType(p, sampleNumeric(p, r))
	case ParamCategorical, ParamBool:
		choices := p.choicesFor()
		return choices[r.Intn(len(choices))]
	default:
		return nil
	}
}