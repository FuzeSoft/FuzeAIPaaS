package hpo

import (
	"math"
	"testing"
)

func fptr(v float64) *float64 { return &v }

func TestObjectiveIsBetter(t *testing.T) {
	max := Objective{MetricName: "acc", Direction: DirectionMaximize}
	min := Objective{MetricName: "loss", Direction: DirectionMinimize}

	cases := []struct {
		name string
		obj  Objective
		a, b *float64
		want bool
	}{
		{"max bigger wins", max, fptr(0.9), fptr(0.8), true},
		{"max smaller loses", max, fptr(0.7), fptr(0.8), false},
		{"min smaller wins", min, fptr(0.1), fptr(0.2), true},
		{"min bigger loses", min, fptr(0.3), fptr(0.2), false},
		{"nan is worst", max, floatNaN(), fptr(0.1), false},
		{"inf is worst", min, floatInf(), fptr(0.1), false},
		{"nil is worst", max, nil, fptr(0.1), false},
		{"nil vs nil", max, nil, nil, false},
		{"equal not better", max, fptr(0.5), fptr(0.5), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.obj.IsBetter(c.a, c.b); got != c.want {
				t.Fatalf("IsBetter(%v,%v)=%v want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

func floatNaN() *float64 { v := math.NaN(); return &v }
func floatInf() *float64 { v := math.Inf(1); return &v }