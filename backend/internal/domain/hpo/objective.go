package hpo

import "math"

const (
	DirectionMaximize = "maximize"
	DirectionMinimize = "minimize"
)

type Objective struct {
	MetricName string
	Direction  string 
}

func (o Objective) IsBetter(a, b *float64) bool {
	aBad := a == nil || math.IsNaN(*a) || math.IsInf(*a, 0)
	bBad := b == nil || math.IsNaN(*b) || math.IsInf(*b, 0)
	switch {
	case aBad && bBad:
		return false
	case aBad:
		return false
	case bBad:
		return true
	}
	if o.Direction == DirectionMinimize {
		return *a < *b
	}
	return *a > *b
}

func (o Objective) sign() float64 {
	if o.Direction == DirectionMinimize {
		return -1
	}
	return 1
}