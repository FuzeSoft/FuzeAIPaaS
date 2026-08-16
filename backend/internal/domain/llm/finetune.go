package llm

import (
	"errors"
	"strings"
)

const (
	
	MethodFull = "full"
	
	MethodLoRA = "lora"
	
	MethodQLoRA = "qlora"
)

const (
	QuantNone = ""
	Quant8Bit = "int8"
	Quant4Bit = "int4"
)

var (
	
	ErrInvalidMethod = errors.New("llm: unsupported finetune method")
	
	ErrEmptyBaseModel = errors.New("llm: base model must not be empty")
	
	ErrEmptyDataset = errors.New("llm: dataset must not be empty")
	
	ErrInvalidRank = errors.New("llm: lora rank must be positive")
	
	ErrInvalidAlpha = errors.New("llm: lora alpha must be positive")
	
	ErrInvalidDropout = errors.New("llm: lora dropout must be within [0,1)")
	
	ErrInvalidQuantization = errors.New("llm: unsupported quantization")
	
	ErrQuantRequiredForQLoRA = errors.New("llm: qlora requires int4 or int8 quantization")
	
	ErrInvalidLearningRate = errors.New("llm: learning rate must be positive")
	
	ErrInvalidEpochs = errors.New("llm: epochs must be positive")
	
	ErrAdapterOnFullFinetune = errors.New("llm: full finetune does not produce an adapter")
)

type LoRAConfig struct {
	
	Rank int `json:"rank"`
	
	Alpha int `json:"alpha"`
	
	Dropout float64 `json:"dropout"`
	
	TargetModules []string `json:"target_modules,omitempty"`
}

func DefaultLoRAConfig() LoRAConfig {
	return LoRAConfig{
		Rank:          8,
		Alpha:         16,
		Dropout:       0.05,
		TargetModules: []string{"q_proj", "v_proj"},
	}
}

func (c LoRAConfig) Scaling() float64 {
	if c.Rank == 0 {
		return 0
	}
	return float64(c.Alpha) / float64(c.Rank)
}

func (c LoRAConfig) Validate() error {
	if c.Rank <= 0 {
		return ErrInvalidRank
	}
	if c.Alpha <= 0 {
		return ErrInvalidAlpha
	}
	if c.Dropout < 0 || c.Dropout >= 1 {
		return ErrInvalidDropout
	}
	return nil
}

type FineTuneSpec struct {
	
	Name string `json:"name"`
	
	BaseModel string `json:"base_model"`
	
	Method string `json:"method"`
	
	Dataset string `json:"dataset"`
	
	ValidationDataset string `json:"validation_dataset,omitempty"`
	
	LoRA LoRAConfig `json:"lora"`
	
	Quantization string `json:"quantization,omitempty"`
	
	LearningRate float64 `json:"learning_rate"`
	
	Epochs int `json:"epochs"`
	
	BatchSize int `json:"batch_size,omitempty"`
	
	MaxSeqLen int `json:"max_seq_len,omitempty"`
}

func (s FineTuneSpec) Validate() error {
	if strings.TrimSpace(s.BaseModel) == "" {
		return ErrEmptyBaseModel
	}
	if strings.TrimSpace(s.Dataset) == "" {
		return ErrEmptyDataset
	}
	switch s.Method {
	case MethodFull, MethodLoRA, MethodQLoRA:
	default:
		return ErrInvalidMethod
	}
	switch s.Quantization {
	case QuantNone, Quant8Bit, Quant4Bit:
	default:
		return ErrInvalidQuantization
	}
	
	if s.Method == MethodQLoRA && s.Quantization == QuantNone {
		return ErrQuantRequiredForQLoRA
	}
	if s.UsesLoRA() {
		if err := s.LoRA.Validate(); err != nil {
			return err
		}
	}
	if s.LearningRate <= 0 {
		return ErrInvalidLearningRate
	}
	if s.Epochs <= 0 {
		return ErrInvalidEpochs
	}
	return nil
}

func (s FineTuneSpec) UsesLoRA() bool {
	return s.Method == MethodLoRA || s.Method == MethodQLoRA
}

func (s FineTuneSpec) ProducesAdapter() bool { return s.UsesLoRA() }

type Adapter struct {
	
	ID string `json:"id"`
	
	Name string `json:"name"`
	
	BaseModel string `json:"base_model"`
	
	Path string `json:"path"`
	
	Rank int `json:"rank"`
	
	SourceJobID string `json:"source_job_id,omitempty"`
}

func (a Adapter) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return ErrEmptyModel
	}
	if strings.TrimSpace(a.BaseModel) == "" {
		return ErrEmptyBaseModel
	}
	if a.Rank <= 0 {
		return ErrInvalidRank
	}
	return nil
}

func (a Adapter) CompatibleWith(baseModel string) bool {
	return a.BaseModel == baseModel
}

type ServingPlan struct {
	BaseModel string    `json:"base_model"`
	Adapters  []Adapter `json:"adapters"`
}

func (p ServingPlan) Validate() error {
	if strings.TrimSpace(p.BaseModel) == "" {
		return ErrEmptyBaseModel
	}
	seen := make(map[string]struct{}, len(p.Adapters))
	for _, a := range p.Adapters {
		if err := a.Validate(); err != nil {
			return err
		}
		if !a.CompatibleWith(p.BaseModel) {
			return ErrIncompatibleAdapter
		}
		if _, dup := seen[a.Name]; dup {
			return ErrDuplicateAdapter
		}
		seen[a.Name] = struct{}{}
	}
	return nil
}

var (
	
	ErrIncompatibleAdapter = errors.New("llm: adapter is not compatible with base model")
	
	ErrDuplicateAdapter = errors.New("llm: duplicate adapter name in serving plan")
)

func (p ServingPlan) ServedModelNames() []string {
	out := make([]string, 0, len(p.Adapters)+1)
	out = append(out, p.BaseModel)
	for _, a := range p.Adapters {
		out = append(out, p.BaseModel+":"+a.Name)
	}
	return out
}

func SplitServedName(name string) (base, adapter string) {
	idx := strings.Index(name, ":")
	if idx < 0 {
		return name, ""
	}
	return name[:idx], name[idx+1:]
}