package llm

import "testing"

func validSpec() FineTuneSpec {
	return FineTuneSpec{
		Name:         "sft-1",
		BaseModel:    "qwen2-7b",
		Method:       MethodLoRA,
		Dataset:      "ds-1",
		LoRA:         DefaultLoRAConfig(),
		LearningRate: 1e-4,
		Epochs:       3,
	}
}

func TestFineTuneSpecValidate(t *testing.T) {
	if err := validSpec().Validate(); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}

	cases := []struct {
		name string
		mut  func(*FineTuneSpec)
		want error
	}{
		{"no base model", func(s *FineTuneSpec) { s.BaseModel = " " }, ErrEmptyBaseModel},
		{"no dataset", func(s *FineTuneSpec) { s.Dataset = "" }, ErrEmptyDataset},
		{"bad method", func(s *FineTuneSpec) { s.Method = "magic" }, ErrInvalidMethod},
		{"bad quantization", func(s *FineTuneSpec) { s.Quantization = "int2" }, ErrInvalidQuantization},
		{"zero lr", func(s *FineTuneSpec) { s.LearningRate = 0 }, ErrInvalidLearningRate},
		{"negative lr", func(s *FineTuneSpec) { s.LearningRate = -1 }, ErrInvalidLearningRate},
		{"zero epochs", func(s *FineTuneSpec) { s.Epochs = 0 }, ErrInvalidEpochs},
		{"bad rank", func(s *FineTuneSpec) { s.LoRA.Rank = 0 }, ErrInvalidRank},
		{"bad alpha", func(s *FineTuneSpec) { s.LoRA.Alpha = 0 }, ErrInvalidAlpha},
		{"bad dropout", func(s *FineTuneSpec) { s.LoRA.Dropout = 1 }, ErrInvalidDropout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSpec()
			tc.mut(&s)
			if got := s.Validate(); got != tc.want {
				t.Fatalf("Validate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestQLoRARequiresQuantization(t *testing.T) {
	s := validSpec()
	s.Method = MethodQLoRA
	s.Quantization = QuantNone
	if err := s.Validate(); err != ErrQuantRequiredForQLoRA {
		t.Fatalf("want ErrQuantRequiredForQLoRA, got %v", err)
	}

	s.Quantization = Quant4Bit
	if err := s.Validate(); err != nil {
		t.Fatalf("qlora with int4 rejected: %v", err)
	}
}

func TestFullFinetuneSkipsLoRAValidation(t *testing.T) {
	s := validSpec()
	s.Method = MethodFull
	s.LoRA = LoRAConfig{} 
	if err := s.Validate(); err != nil {
		t.Fatalf("full finetune should ignore lora config: %v", err)
	}
	if s.UsesLoRA() {
		t.Fatal("UsesLoRA() = true for full finetune")
	}
	if s.ProducesAdapter() {
		t.Fatal("ProducesAdapter() = true for full finetune")
	}
}

func TestUsesLoRA(t *testing.T) {
	for _, m := range []string{MethodLoRA, MethodQLoRA} {
		s := validSpec()
		s.Method = m
		s.Quantization = Quant4Bit
		if !s.UsesLoRA() || !s.ProducesAdapter() {
			t.Fatalf("method %q should use lora", m)
		}
	}
}

func TestLoRAScaling(t *testing.T) {
	c := LoRAConfig{Rank: 8, Alpha: 16}
	if got := c.Scaling(); got != 2 {
		t.Fatalf("Scaling() = %v, want 2", got)
	}
	
	if got := (LoRAConfig{}).Scaling(); got != 0 {
		t.Fatalf("Scaling() with zero rank = %v, want 0", got)
	}
}

func TestAdapterValidate(t *testing.T) {
	a := Adapter{Name: "cs-bot", BaseModel: "qwen2-7b", Rank: 8}
	if err := a.Validate(); err != nil {
		t.Fatalf("valid adapter rejected: %v", err)
	}
	if err := (Adapter{BaseModel: "b", Rank: 8}).Validate(); err != ErrEmptyModel {
		t.Fatalf("want ErrEmptyModel, got %v", err)
	}
	if err := (Adapter{Name: "n", Rank: 8}).Validate(); err != ErrEmptyBaseModel {
		t.Fatalf("want ErrEmptyBaseModel, got %v", err)
	}
	if err := (Adapter{Name: "n", BaseModel: "b"}).Validate(); err != ErrInvalidRank {
		t.Fatalf("want ErrInvalidRank, got %v", err)
	}
}

func TestAdapterCompatibility(t *testing.T) {
	a := Adapter{Name: "x", BaseModel: "qwen2-7b", Rank: 8}
	if !a.CompatibleWith("qwen2-7b") {
		t.Fatal("adapter should match its own base")
	}
	if a.CompatibleWith("llama3-8b") {
		t.Fatal("adapter must not match a different base")
	}
}

func TestServingPlanValidate(t *testing.T) {
	p := ServingPlan{
		BaseModel: "qwen2-7b",
		Adapters: []Adapter{
			{Name: "a", BaseModel: "qwen2-7b", Rank: 8},
			{Name: "b", BaseModel: "qwen2-7b", Rank: 16},
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}
}

func TestServingPlanRejectsIncompatibleAdapter(t *testing.T) {
	p := ServingPlan{
		BaseModel: "qwen2-7b",
		Adapters:  []Adapter{{Name: "a", BaseModel: "llama3-8b", Rank: 8}},
	}
	if err := p.Validate(); err != ErrIncompatibleAdapter {
		t.Fatalf("want ErrIncompatibleAdapter, got %v", err)
	}
}

func TestServingPlanRejectsDuplicateNames(t *testing.T) {
	p := ServingPlan{
		BaseModel: "qwen2-7b",
		Adapters: []Adapter{
			{Name: "dup", BaseModel: "qwen2-7b", Rank: 8},
			{Name: "dup", BaseModel: "qwen2-7b", Rank: 8},
		},
	}
	if err := p.Validate(); err != ErrDuplicateAdapter {
		t.Fatalf("want ErrDuplicateAdapter, got %v", err)
	}
}

func TestServingPlanEmptyBase(t *testing.T) {
	if err := (ServingPlan{}).Validate(); err != ErrEmptyBaseModel {
		t.Fatalf("want ErrEmptyBaseModel, got %v", err)
	}
}

func TestServedModelNames(t *testing.T) {
	p := ServingPlan{
		BaseModel: "qwen2-7b",
		Adapters: []Adapter{
			{Name: "cs", BaseModel: "qwen2-7b", Rank: 8},
			{Name: "law", BaseModel: "qwen2-7b", Rank: 8},
		},
	}
	got := p.ServedModelNames()
	want := []string{"qwen2-7b", "qwen2-7b:cs", "qwen2-7b:law"}
	if len(got) != len(want) {
		t.Fatalf("ServedModelNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ServedModelNames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitServedName(t *testing.T) {
	base, adapter := SplitServedName("qwen2-7b:cs")
	if base != "qwen2-7b" || adapter != "cs" {
		t.Fatalf("SplitServedName = (%q,%q), want (qwen2-7b,cs)", base, adapter)
	}
	
	base, adapter = SplitServedName("qwen2-7b")
	if base != "qwen2-7b" || adapter != "" {
		t.Fatalf("SplitServedName = (%q,%q), want (qwen2-7b,\"\")", base, adapter)
	}
}