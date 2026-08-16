package hpo

import (
	"fmt"
	"math"
	"math/rand"
)

type GridSampler struct{}

func gridSteps(p ParamSpec) int {
	if p.Type != ParamFloat && p.Type != ParamInt {
		return 0
	}
	if p.Step > 0 {
		n := int(math.Round((p.Max-p.Min)/p.Step)) + 1
		if n < 1 {
			n = 1
		}
		return n
	}
	if p.Type == ParamInt {
		n := int(p.Max-p.Min) + 1
		if n < 1 {
			n = 1
		}
		return n
	}
	return 1
}

func gridValueAt(p ParamSpec, i int) any {
	var raw float64
	if p.Step > 0 {
		raw = p.Min + float64(i)*p.Step
		if raw > p.Max {
			raw = p.Max
		}
	} else if p.Type == ParamInt {
		raw = p.Min + float64(i)
	} else {
		raw = p.Min
	}
	return numericToType(p, raw)
}

func totalCombinations(space SearchSpace) int {
	total := 1
	for _, p := range space.Params {
		switch p.Type {
		case ParamFloat, ParamInt:
			total *= gridSteps(p)
		case ParamCategorical, ParamBool:
			total *= len(p.choicesFor())
		}
	}
	return total
}

func combinationAt(space SearchSpace, idx int) map[string]any {
	params := make(map[string]any, len(space.Params))
	remainder := idx
	for _, p := range space.Params {
		var n int
		switch p.Type {
		case ParamFloat, ParamInt:
			n = gridSteps(p)
			i := remainder % n
			params[p.Name] = gridValueAt(p, i)
			remainder /= n
		case ParamCategorical, ParamBool:
			choices := p.choicesFor()
			n = len(choices)
			params[p.Name] = choices[remainder%n]
			remainder /= n
		}
	}
	return params
}

func (GridSampler) Suggest(space SearchSpace, history []Trial, _ *rand.Rand) (map[string]any, error) {
	if err := space.Validate(); err != nil {
		return nil, err
	}
	total := totalCombinations(space)
	idx := len(history)
	if idx >= total {
		return nil, fmt.Errorf("%w (used %d / %d)", ErrExhausted, idx, total)
	}
	return combinationAt(space, idx), nil
}