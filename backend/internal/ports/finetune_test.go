package ports

import (
	"errors"
	"testing"
)

func validAdapter() FineTuneAdapter {
	return FineTuneAdapter{
		Name:      "sql-expert",
		BaseModel: "qwen2-7b",
		Path:      "s3://adapters/sql-expert",
		Rank:      8,
		Method:    MethodLoRA,
		TenantID:  "t1",
	}
}

func TestAdapterValidate(t *testing.T) {
	t.Run("合法定义通过校验", func(t *testing.T) {
		if err := validAdapter().Validate(); err != nil {
			t.Fatalf("期望通过校验，实际: %v", err)
		}
		q := validAdapter()
		q.Method = MethodQLoRA
		if err := q.Validate(); err != nil {
			t.Fatalf("qlora 应为合法方法，实际: %v", err)
		}
	})

	cases := []struct {
		name   string
		mutate func(*FineTuneAdapter)
	}{
		{"名称为空", func(a *FineTuneAdapter) { a.Name = "" }},
		{"基座模型为空", func(a *FineTuneAdapter) { a.BaseModel = "" }},
		{"权重路径为空", func(a *FineTuneAdapter) { a.Path = "" }},
		{"秩为零", func(a *FineTuneAdapter) { a.Rank = 0 }},
		{"秩为负", func(a *FineTuneAdapter) { a.Rank = -1 }},
		{"秩超过上限", func(a *FineTuneAdapter) { a.Rank = maxAdapterRank + 1 }},
		{"未知微调方法", func(a *FineTuneAdapter) { a.Method = "full-finetune" }},
	}
	for _, tc := range cases {
		t.Run(tc.name+"必须被拒绝", func(t *testing.T) {
			a := validAdapter()
			tc.mutate(&a)
			err := a.Validate()
			if err == nil {
				t.Fatal("非法定义被放行")
			}
			if !errors.Is(err, ErrAdapterInvalid) {
				t.Fatalf("错误应可被 errors.Is(ErrAdapterInvalid) 识别，实际: %v", err)
			}
		})
	}
}

func TestAdapterNormalize(t *testing.T) {
	t.Run("去除空白并统一方法名大小写", func(t *testing.T) {
		a := FineTuneAdapter{
			Name:      "  sql-expert  ",
			BaseModel: " qwen2-7b ",
			Path:      " s3://a ",
			Method:    " LoRA ",
			Rank:      4,
		}
		a.Normalize()

		if a.Name != "sql-expert" || a.BaseModel != "qwen2-7b" || a.Path != "s3://a" {
			t.Fatalf("首尾空白未被去除: %+v", a)
		}
		
		if a.Method != MethodLoRA {
			t.Fatalf("方法名未归一化: %q", a.Method)
		}
		if err := a.Validate(); err != nil {
			t.Fatalf("归一化后应通过校验: %v", err)
		}
	})

	t.Run("补齐缺省方法与缺省秩", func(t *testing.T) {
		a := FineTuneAdapter{Name: "n", BaseModel: "b", Path: "p"}
		a.Normalize()

		if a.Method != MethodLoRA {
			t.Fatalf("缺省方法应为 lora，实际 %q", a.Method)
		}
		if a.Rank != 8 {
			t.Fatalf("缺省秩应为 8，实际 %d", a.Rank)
		}
		
		if err := a.Validate(); err != nil {
			t.Fatalf("补齐缺省值后应通过校验: %v", err)
		}
	})

	t.Run("纯空白名称归一化后仍被拒绝", func(t *testing.T) {
		a := validAdapter()
		a.Name = "   "
		a.Normalize()
		
		if err := a.Validate(); !errors.Is(err, ErrAdapterInvalid) {
			t.Fatalf("纯空白名称应被拒绝，实际: %v", err)
		}
	})
}