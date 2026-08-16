package hpo

import "testing"

func TestParamSpecValidate(t *testing.T) {
	cases := []struct {
		name    string
		param   ParamSpec
		wantErr bool
	}{
		{"empty name", ParamSpec{Type: ParamFloat, Min: 0, Max: 1}, true},
		{"min>max", ParamSpec{Name: "lr", Type: ParamFloat, Min: 1, Max: 0}, true},
		{"log scale min<=0", ParamSpec{Name: "lr", Type: ParamFloat, Min: 0, Max: 1, LogScale: true}, true},
		{"log scale ok", ParamSpec{Name: "lr", Type: ParamFloat, Min: 1e-4, Max: 1e-1, LogScale: true}, false},
		{"step>range", ParamSpec{Name: "x", Type: ParamFloat, Min: 0, Max: 1, Step: 2}, true},
		{"step ok", ParamSpec{Name: "x", Type: ParamFloat, Min: 0, Max: 1, Step: 0.1}, false},
		{"categorical empty", ParamSpec{Name: "opt", Type: ParamCategorical, Choices: nil}, true},
		{"categorical ok", ParamSpec{Name: "opt", Type: ParamCategorical, Choices: []any{"a", "b"}}, false},
		{"bool ok", ParamSpec{Name: "flag", Type: ParamBool}, false},
		{"unknown type", ParamSpec{Name: "z", Type: "weird"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.param.Validate()
			if (err != nil) != c.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestSearchSpaceValidate(t *testing.T) {
	t.Run("duplicate name rejected", func(t *testing.T) {
		s := SearchSpace{Params: []ParamSpec{
			{Name: "a", Type: ParamFloat, Min: 0, Max: 1},
			{Name: "a", Type: ParamFloat, Min: 0, Max: 1},
		}}
		if err := s.Validate(); err == nil {
			t.Fatal("expected duplicate-name error")
		}
	})
	t.Run("empty space rejected", func(t *testing.T) {
		if err := (SearchSpace{}).Validate(); err == nil {
			t.Fatal("expected empty-space error")
		}
	})
	t.Run("valid space", func(t *testing.T) {
		s := SearchSpace{Params: []ParamSpec{
			{Name: "lr", Type: ParamFloat, Min: 1e-4, Max: 1e-1, LogScale: true},
			{Name: "opt", Type: ParamCategorical, Choices: []any{"sgd", "adam"}},
		}}
		if err := s.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}