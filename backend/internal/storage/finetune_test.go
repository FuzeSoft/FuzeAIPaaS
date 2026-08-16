package storage

import (
	"context"
	"errors"
	"sync"
	"testing"

	"fuze-ai-paas/backend/internal/ports"
)

func newAdapter(name, tenant, base string) *ports.FineTuneAdapter {
	return &ports.FineTuneAdapter{
		Name:      name,
		BaseModel: base,
		Path:      "s3://adapters/" + name,
		Rank:      8,
		Method:    ports.MethodLoRA,
		TenantID:  tenant,
		CreatedBy: "alice",
	}
}

func TestFineTuneCreateAndGet(t *testing.T) {
	repo := NewFineTuneRepository(newLLMTestStorage(t).db)
	ctx := context.Background()

	a := newAdapter("sql-expert", "t1", "qwen2-7b")
	a.SourceJobID = "job-1"
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if a.ID == "" {
		t.Fatal("Create 未回填 ID")
	}
	if a.CreatedAt == 0 {
		t.Fatal("Create 未回填 CreatedAt")
	}

	got, err := repo.Get(ctx, "t1", a.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "sql-expert" || got.BaseModel != "qwen2-7b" {
		t.Fatalf("字段回读不一致: %+v", got)
	}
	
	if got.SourceJobID != "job-1" {
		t.Fatalf("SourceJobID 丢失: %+v", got)
	}
	if got.Rank != 8 || got.Method != ports.MethodLoRA || got.CreatedBy != "alice" {
		t.Fatalf("元数据回读不一致: %+v", got)
	}
}

func TestFineTuneGetMissing(t *testing.T) {
	repo := NewFineTuneRepository(newLLMTestStorage(t).db)

	_, err := repo.Get(context.Background(), "t1", "nope")
	if !errors.Is(err, ports.ErrAdapterNotFound) {
		t.Fatalf("期望 ErrAdapterNotFound，实际: %v", err)
	}
}

func TestFineTuneDuplicateNameRejected(t *testing.T) {
	repo := NewFineTuneRepository(newLLMTestStorage(t).db)
	ctx := context.Background()

	if err := repo.Create(ctx, newAdapter("dup", "t1", "qwen2-7b")); err != nil {
		t.Fatalf("首次创建: %v", err)
	}

	err := repo.Create(ctx, newAdapter("dup", "t1", "qwen2-7b"))
	if !errors.Is(err, ports.ErrAdapterConflict) {
		t.Fatalf("同租户重名应冲突，实际: %v", err)
	}

	if err := repo.Create(ctx, newAdapter("dup", "t2", "qwen2-7b")); err != nil {
		t.Fatalf("跨租户同名应被允许，实际: %v", err)
	}
}

func TestFineTuneConcurrentCreateSameName(t *testing.T) {
	repo := NewFineTuneRepository(newLLMTestStorage(t).db)
	ctx := context.Background()

	const n = 6
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = repo.Create(ctx, newAdapter("race", "t1", "qwen2-7b"))
		}(i)
	}
	wg.Wait()

	success := 0
	for _, err := range errs {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("并发同名创建应恰好成功 1 次，实际 %d 次", success)
	}

	list, err := repo.List(ctx, ports.FineTuneFilter{TenantID: "t1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("库中应只有 1 条记录，实际 %d 条", len(list))
	}
}

func TestFineTuneListFilters(t *testing.T) {
	repo := NewFineTuneRepository(newLLMTestStorage(t).db)
	ctx := context.Background()

	for _, a := range []*ports.FineTuneAdapter{
		newAdapter("a1", "t1", "qwen2-7b"),
		newAdapter("a2", "t1", "llama3-8b"),
		newAdapter("b1", "t2", "qwen2-7b"),
	} {
		if err := repo.Create(ctx, a); err != nil {
			t.Fatalf("Create %s: %v", a.Name, err)
		}
	}

	t.Run("按租户隔离", func(t *testing.T) {
		list, err := repo.List(ctx, ports.FineTuneFilter{TenantID: "t1"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("期望 2 条，实际 %d", len(list))
		}
		for _, a := range list {
			if a.TenantID != "t1" {
				t.Fatalf("越权返回他租户适配器: %+v", a)
			}
		}
	})

	t.Run("空租户返回跨租户全量", func(t *testing.T) {
		
		list, err := repo.List(ctx, ports.FineTuneFilter{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("期望 3 条，实际 %d", len(list))
		}
	})

	t.Run("按基座模型过滤且不越租户", func(t *testing.T) {
		list, err := repo.List(ctx, ports.FineTuneFilter{TenantID: "t1", BaseModel: "qwen2-7b"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		
		if len(list) != 1 || list[0].Name != "a1" {
			t.Fatalf("基座过滤结果不符: %+v", list)
		}
	})

	t.Run("limit 生效", func(t *testing.T) {
		list, err := repo.List(ctx, ports.FineTuneFilter{TenantID: "t1", Limit: 1})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("limit 未生效，返回 %d 条", len(list))
		}
	})
}

func TestFineTuneTenantIsolation(t *testing.T) {
	repo := NewFineTuneRepository(newLLMTestStorage(t).db)
	ctx := context.Background()

	owned := newAdapter("secret", "t1", "qwen2-7b")
	if err := repo.Create(ctx, owned); err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("他租户读取按不存在处理", func(t *testing.T) {
		
		_, err := repo.Get(ctx, "t2", owned.ID)
		if !errors.Is(err, ports.ErrAdapterNotFound) {
			t.Fatalf("期望 ErrAdapterNotFound，实际: %v", err)
		}
	})

	t.Run("他租户删除必须失败且数据保留", func(t *testing.T) {
		if err := repo.Delete(ctx, "t2", owned.ID); !errors.Is(err, ports.ErrAdapterNotFound) {
			t.Fatalf("跨租户删除应失败，实际: %v", err)
		}
		if _, err := repo.Get(ctx, "t1", owned.ID); err != nil {
			t.Fatalf("越权删除后原数据应完好，实际: %v", err)
		}
	})

	t.Run("平台管理员视图可跨租户读取", func(t *testing.T) {
		if _, err := repo.Get(ctx, "", owned.ID); err != nil {
			t.Fatalf("空租户应可读取，实际: %v", err)
		}
	})
}

func TestFineTuneDelete(t *testing.T) {
	repo := NewFineTuneRepository(newLLMTestStorage(t).db)
	ctx := context.Background()

	a := newAdapter("gone", "t1", "qwen2-7b")
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, "t1", a.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Get(ctx, "t1", a.ID); !errors.Is(err, ports.ErrAdapterNotFound) {
		t.Fatalf("删除后应查不到，实际: %v", err)
	}
	
	if err := repo.Delete(ctx, "t1", a.ID); !errors.Is(err, ports.ErrAdapterNotFound) {
		t.Fatalf("重复删除应返回 NotFound，实际: %v", err)
	}

	if err := repo.Create(ctx, newAdapter("gone", "t1", "qwen2-7b")); err != nil {
		t.Fatalf("删除后同名应可重建，实际: %v", err)
	}
}