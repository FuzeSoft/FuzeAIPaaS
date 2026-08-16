package ports

import (
	"context"
	"errors"
	"testing"
)

func validMount() AdapterMount {
	return AdapterMount{
		AdapterID:  "ad-1",
		ServiceID:  "svc-1",
		ServedName: "qwen2-7b:sql-expert",
		BaseModel:  "qwen2-7b",
		TenantID:   "t1",
	}
}

func TestAdapterMountValidate(t *testing.T) {
	t.Run("合法挂载通过校验", func(t *testing.T) {
		if err := validMount().Validate(); err != nil {
			t.Fatalf("期望通过校验，实际: %v", err)
		}
	})

	cases := []struct {
		name   string
		mutate func(*AdapterMount)
	}{
		{"适配器 ID 为空", func(m *AdapterMount) { m.AdapterID = "" }},
		{"推理服务 ID 为空", func(m *AdapterMount) { m.ServiceID = "" }},
		{"对外服务名为空", func(m *AdapterMount) { m.ServedName = "" }},
		{"基座模型为空", func(m *AdapterMount) { m.BaseModel = "" }},
		{"租户 ID 为空", func(m *AdapterMount) { m.TenantID = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name+"必须被拒绝", func(t *testing.T) {
			m := validMount()
			tc.mutate(&m)
			err := m.Validate()
			if err == nil {
				t.Fatal("非法挂载被放行")
			}
			if !errors.Is(err, ErrAdapterInvalid) {
				t.Fatalf("错误应可被 errors.Is(ErrAdapterInvalid) 识别，实际: %v", err)
			}
		})
	}

	t.Run("对外服务名必须与基座一致", func(t *testing.T) {
		m := validMount()
		m.BaseModel = "llama3-8b" 
		
		if err := m.Validate(); !errors.Is(err, ErrAdapterInvalid) {
			t.Fatalf("基座与服务名前缀不一致应被拒绝，实际: %v", err)
		}
	})
}

func TestAdapterMountNormalize(t *testing.T) {
	t.Run("去除首尾空白", func(t *testing.T) {
		m := AdapterMount{
			AdapterID:  " ad-1 ",
			ServiceID:  " svc-1 ",
			ServedName: " qwen2-7b:sql-expert ",
			BaseModel:  " qwen2-7b ",
			TenantID:   " t1 ",
		}
		m.Normalize()

		if m.AdapterID != "ad-1" || m.ServiceID != "svc-1" || m.TenantID != "t1" {
			t.Fatalf("首尾空白未被去除: %+v", m)
		}
		if m.ServedName != "qwen2-7b:sql-expert" || m.BaseModel != "qwen2-7b" {
			t.Fatalf("名称字段未归一化: %+v", m)
		}
		if err := m.Validate(); err != nil {
			t.Fatalf("归一化后应通过校验: %v", err)
		}
	})

	t.Run("缺省服务名由基座与适配器名拼出", func(t *testing.T) {
		m := AdapterMount{
			AdapterID:   "ad-1",
			ServiceID:   "svc-1",
			BaseModel:   "qwen2-7b",
			AdapterName: "sql-expert",
			TenantID:    "t1",
		}
		m.Normalize()

		if m.ServedName != "qwen2-7b:sql-expert" {
			t.Fatalf("对外服务名应被推导为 qwen2-7b:sql-expert，实际 %q", m.ServedName)
		}
		if err := m.Validate(); err != nil {
			t.Fatalf("补齐后应通过校验: %v", err)
		}
	})
}

type stubJobChecker struct {
	exists    bool
	err       error
	gotTenant string
	gotJobID  string
	callCount int
}

func (s *stubJobChecker) JobExistsForTenant(_ context.Context, tenantID, jobID string) (bool, error) {
	s.callCount++
	s.gotTenant, s.gotJobID = tenantID, jobID
	return s.exists, s.err
}

func TestValidateSourceJob(t *testing.T) {
	ctx := context.Background()

	t.Run("未填写来源作业时跳过校验", func(t *testing.T) {
		chk := &stubJobChecker{}
		a := validAdapter()
		a.SourceJobID = ""

		if err := ValidateSourceJob(ctx, chk, a); err != nil {
			t.Fatalf("空来源作业应放行（允许登记外部训练的权重），实际: %v", err)
		}
		
		if chk.callCount != 0 {
			t.Fatalf("空来源作业不应触发查询，实际调用 %d 次", chk.callCount)
		}
	})

	t.Run("来源作业存在时通过", func(t *testing.T) {
		chk := &stubJobChecker{exists: true}
		a := validAdapter()
		a.SourceJobID = "job-1"

		if err := ValidateSourceJob(ctx, chk, a); err != nil {
			t.Fatalf("存在的作业应通过，实际: %v", err)
		}
		
		if chk.gotTenant != "t1" || chk.gotJobID != "job-1" {
			t.Fatalf("查询参数应带租户，实际 tenant=%q job=%q", chk.gotTenant, chk.gotJobID)
		}
	})

	t.Run("来源作业不存在时拒绝", func(t *testing.T) {
		chk := &stubJobChecker{exists: false}
		a := validAdapter()
		a.SourceJobID = "job-missing"

		err := ValidateSourceJob(ctx, chk, a)
		
		if !errors.Is(err, ErrSourceJobNotFound) {
			t.Fatalf("不存在的作业应返回 ErrSourceJobNotFound，实际: %v", err)
		}
	})

	t.Run("校验器不可用时放行", func(t *testing.T) {
		a := validAdapter()
		a.SourceJobID = "job-1"

		if err := ValidateSourceJob(ctx, nil, a); err != nil {
			t.Fatalf("校验器为 nil 时应放行，实际: %v", err)
		}
	})

	t.Run("查询出错时上抛而非放行", func(t *testing.T) {
		boom := errors.New("db down")
		chk := &stubJobChecker{err: boom}
		a := validAdapter()
		a.SourceJobID = "job-1"

		err := ValidateSourceJob(ctx, chk, a)
		
		if !errors.Is(err, boom) {
			t.Fatalf("底层错误应被透传，实际: %v", err)
		}
		if errors.Is(err, ErrSourceJobNotFound) {
			t.Fatal("查询故障不应被伪装成来源作业不存在")
		}
	})
}