package optimize

import (
	"encoding/json"
	"fmt"
)

const (
	QuantizeDynamic = "dynamic"
	QuantizeStatic  = "static"
)

const (
	PruneStructured   = "structured"
	PruneUnstructured = "unstructured"
)

const (
	ConvertONNX     = "onnx"
	ConvertTensorRT = "tensorrt"
	ConvertOpenVINO = "openvino"
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("optimize: invalid %s: %s", e.Field, e.Message)
}

type QuantizeConfig struct {
	Method           string `json:"method"`            
	Bits             int    `json:"bits"`              
	CalibrationDataset string `json:"calibration_dataset,omitempty"` 
}

func (c QuantizeConfig) Validate() error {
	if c.Method != QuantizeDynamic && c.Method != QuantizeStatic {
		return &ValidationError{Field: "method", Message: "must be dynamic or static"}
	}
	if c.Bits <= 0 || c.Bits > 32 {
		return &ValidationError{Field: "bits", Message: "must be in (0, 32]"}
	}
	if c.Method == QuantizeStatic && c.CalibrationDataset == "" {
		return &ValidationError{Field: "calibration_dataset", Message: "required for static quantization"}
	}
	return nil
}

type PruneConfig struct {
	Strategy  string  `json:"strategy"` 
	Sparsity  float64 `json:"sparsity"` 
}

func (c PruneConfig) Validate() error {
	if c.Strategy != PruneStructured && c.Strategy != PruneUnstructured {
		return &ValidationError{Field: "strategy", Message: "must be structured or unstructured"}
	}
	if c.Sparsity <= 0 || c.Sparsity >= 1 {
		return &ValidationError{Field: "sparsity", Message: "must be in (0, 1)"}
	}
	return nil
}

type DistillConfig struct {
	TeacherModelURI string  `json:"teacher_model_uri"`
	Temperature     float64 `json:"temperature"` 
	Alpha           float64 `json:"alpha"`       
}

func (c DistillConfig) Validate() error {
	if c.TeacherModelURI == "" {
		return &ValidationError{Field: "teacher_model_uri", Message: "required"}
	}
	if c.Temperature <= 0 {
		return &ValidationError{Field: "temperature", Message: "must be > 0"}
	}
	if c.Alpha < 0 || c.Alpha > 1 {
		return &ValidationError{Field: "alpha", Message: "must be in [0, 1]"}
	}
	return nil
}

type ConvertConfig struct {
	TargetFormat string `json:"target_format"` 
}

func (c ConvertConfig) Validate() error {
	switch c.TargetFormat {
	case ConvertONNX, ConvertTensorRT, ConvertOpenVINO:
		return nil
	default:
		return &ValidationError{Field: "target_format", Message: "must be onnx, tensorrt or openvino"}
	}
}

func ParseConfig(typ CompressionType, raw string) (interface{}, error) {
	switch typ {
	case CompressionTypeQuantize:
		var c QuantizeConfig
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, fmt.Errorf("parse quantize config: %w", err)
		}
		return c, c.Validate()
	case CompressionTypePrune:
		var c PruneConfig
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, fmt.Errorf("parse prune config: %w", err)
		}
		return c, c.Validate()
	case CompressionTypeDistill:
		var c DistillConfig
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, fmt.Errorf("parse distill config: %w", err)
		}
		return c, c.Validate()
	case CompressionTypeConvert:
		var c ConvertConfig
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, fmt.Errorf("parse convert config: %w", err)
		}
		return c, c.Validate()
	default:
		return nil, fmt.Errorf("unknown compression type %q", typ)
	}
}