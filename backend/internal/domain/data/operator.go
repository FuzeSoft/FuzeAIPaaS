package data

import (
	"encoding/json"
	"fmt"
)

type OperatorSpec struct {
	ID          string
	Kind        StepKind
	Description string
	
	DefaultImage string
	
	Required []string
	
	Allowed []string
}

const builtinImage = "registry.local/fuze/data-operator:latest"

var BuiltinOperators = map[string]OperatorSpec{
	
	"dedup": {
		ID: "dedup", Kind: StepKindClean,
		Description: "记录/文本去重", DefaultImage: builtinImage,
		Required: []string{"method"}, Allowed: []string{"method", "key"},
	},
	"fillna": {
		ID: "fillna", Kind: StepKindClean,
		Description: "缺失值填充", DefaultImage: builtinImage,
		Required: []string{"strategy"}, Allowed: []string{"strategy", "columns", "value"},
	},
	"drop_outlier": {
		ID: "drop_outlier", Kind: StepKindClean,
		Description: "异常值过滤", DefaultImage: builtinImage,
		Required: []string{"method"}, Allowed: []string{"method", "columns", "threshold"},
	},
	"normalize": {
		ID: "normalize", Kind: StepKindClean,
		Description: "格式/单位标准化", DefaultImage: builtinImage,
		Required: nil, Allowed: []string{"rules"},
	},
	
	"img_flip": {
		ID: "img_flip", Kind: StepKindAugment,
		Description: "图像翻转", DefaultImage: builtinImage,
		Required: nil, Allowed: []string{"mode"},
	},
	"img_crop": {
		ID: "img_crop", Kind: StepKindAugment,
		Description: "随机裁剪", DefaultImage: builtinImage,
		Required: nil, Allowed: []string{"ratio"},
	},
	"text_synonym": {
		ID: "text_synonym", Kind: StepKindAugment,
		Description: "文本同义替换", DefaultImage: builtinImage,
		Required: nil, Allowed: []string{"ratio", "dict_uri"},
	},
	"text_backtranslate": {
		ID: "text_backtranslate", Kind: StepKindAugment,
		Description: "回译增强", DefaultImage: builtinImage,
		Required: nil, Allowed: []string{"target_lang"},
	},
	
	"format_convert": {
		ID: "format_convert", Kind: StepKindETL,
		Description: "格式转换 (jsonl/csv/parquet)", DefaultImage: builtinImage,
		Required: []string{"from", "to"}, Allowed: []string{"from", "to"},
	},
	"split": {
		ID: "split", Kind: StepKindETL,
		Description: "训练/验证切分", DefaultImage: builtinImage,
		Required: nil, Allowed: []string{"train_ratio", "seed"},
	},
	"sample": {
		ID: "sample", Kind: StepKindETL,
		Description: "采样", DefaultImage: builtinImage,
		Required: nil, Allowed: []string{"n", "frac"},
	},
}

type DataJobSpec struct {
	Operator string         `json:"operator"`
	Params   map[string]any `json:"params"`
	Input    string         `json:"input"`
	Output   string         `json:"output"`
}

func ValidateOperator(operator, paramsJSON string) error {
	if operator == "" {
		return nil 
	}
	spec, ok := BuiltinOperators[operator]
	if !ok {
		return fmt.Errorf("unknown operator %q (not builtin; use empty operator for custom image)", operator)
	}
	if paramsJSON == "" {
		paramsJSON = "{}"
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return fmt.Errorf("operator %s: invalid params json: %w", operator, err)
	}
	for _, req := range spec.Required {
		if _, present := params[req]; !present {
			return fmt.Errorf("operator %s: missing required param %q", operator, req)
		}
	}
	allowed := make(map[string]bool, len(spec.Allowed))
	for _, a := range spec.Allowed {
		allowed[a] = true
	}
	for k := range params {
		if !allowed[k] {
			return fmt.Errorf("operator %s: unexpected param %q (allowed: %v)", operator, k, spec.Allowed)
		}
	}
	return nil
}

func OperatorImage(operator, customImage string) string {
	if operator == "" && customImage != "" {
		return customImage
	}
	if spec, ok := BuiltinOperators[operator]; ok {
		return spec.DefaultImage
	}
	return builtinImage
}