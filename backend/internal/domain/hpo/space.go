
package hpo

import (
	"fmt"
)

const (
	ParamFloat      = "float"
	ParamInt        = "int"
	ParamCategorical = "categorical"
	ParamBool       = "bool"
)

type ParamSpec struct {
	Name string
	Type string
	
	Min, Max float64
	
	Step float64
	
	LogScale bool
	
	Choices []any
}

type SearchSpace struct {
	Params []ParamSpec
}

func (p ParamSpec) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("param name must not be empty")
	}
	switch p.Type {
	case ParamFloat, ParamInt:
		if p.Min > p.Max {
			return fmt.Errorf("param %q: min (%v) must not exceed max (%v)", p.Name, p.Min, p.Max)
		}
		if p.LogScale && p.Min <= 0 {
			return fmt.Errorf("param %q: log scale requires min > 0, got %v", p.Name, p.Min)
		}
		if p.Step > 0 && p.Step > (p.Max - p.Min) {
			return fmt.Errorf("param %q: step (%v) must not exceed range width (%v)", p.Name, p.Step, p.Max-p.Min)
		}
	case ParamCategorical:
		if len(p.Choices) == 0 {
			return fmt.Errorf("param %q: categorical param requires non-empty choices", p.Name)
		}
	case ParamBool:
		
	default:
		return fmt.Errorf("param %q: unknown type %q", p.Name, p.Type)
	}
	return nil
}

func (s SearchSpace) Validate() error {
	seen := make(map[string]struct{}, len(s.Params))
	for _, p := range s.Params {
		if err := p.Validate(); err != nil {
			return err
		}
		if _, dup := seen[p.Name]; dup {
			return fmt.Errorf("duplicate param name %q", p.Name)
		}
		seen[p.Name] = struct{}{}
	}
	if len(s.Params) == 0 {
		return fmt.Errorf("search space must contain at least one param")
	}
	return nil
}

func (p ParamSpec) choicesFor() []any {
	if p.Type == ParamBool {
		return []any{true, false}
	}
	return p.Choices
}