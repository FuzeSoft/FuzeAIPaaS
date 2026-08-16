package storage

import (
	"context"
	"errors"
	"testing"

	"fuze-ai-paas/backend/internal/ports"
)

func newMountTestRepos(t *testing.T) (ports.FineTuneRepository, ports.AdapterMountRepository) {
	t.Helper()
	s := newLLMTestStorage(t)
	return NewFineTuneRepository(s.db), NewAdapterMountRepository(s.db)
}

func seedAdapter(t *testing.T, repo ports.FineTuneRepository, name, tenant, base string) string {
	t.Helper()
	a := newAdapter(name, tenant, base)
	if err := repo.Create(context.Background(), a); err != nil {
		t.Fatalf("seed adapter: %v", err)
	}
	return a.ID
}

func newMount(adapterID, tenant, base, adapterName string) *ports.AdapterMount {
	m := &ports.AdapterMount{
		AdapterID:   adapterID,
		ServiceID:   "svc-1",
		BaseModel:   base,
		AdapterName: adapterName,
		TenantID:    tenant,
		CreatedBy:   "alice",
	}
	m.Normalize()
	return m
}

func TestMountAndResolve(t *testing.T) {
	adapters, mounts := newMountTestRepos(t)
	ctx := context.Background()

	id := seedAdapter(t, adapters, "sql-expert", "t1", "qwen2-7b")
	m := newMount(id, "t1", "qwen2-7b", "sql-expert")
	if err := mounts.Mount(ctx, m); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if m.ID == "" || m.CreatedAt == 0 {
		t.Fatalf("Mount 未回填生成值: %+v", m)
	}

	got, err := mounts.ResolveServedName(ctx, "t1", "qwen2-7b:sql-expert")
	if err != nil {
		t.Fatalf("ResolveServedName: %v", err)
	}
	
	if got.AdapterID != id || got.BaseModel != "qwen2-7b" || got.ServiceID != "svc-1" {
		t.Fatalf("挂载信息回读不一致: %+v", got)
	}
}

func TestResolveUnmounted(t *testing.T) {
	_, mounts := newMountTestRepos(t)

	_, err := mounts.ResolveServedName(context.Background(), "t1", "qwen2-7b:none")
	
	if !errors.Is(err, ports.ErrAdapterNotMounted) {
		t.Fatalf("未挂载应返回 ErrAdapterNotMounted，实际: %v", err)
	}
}

func TestMountDuplicateServedName(t *testing.T) {
	adapters, mounts := newMountTestRepos(t)
	ctx := context.Background()

	a1 := seedAdapter(t, adapters, "sql-expert", "t1", "qwen2-7b")
	if err := mounts.Mount(ctx, newMount(a1, "t1", "qwen2-7b", "sql-expert")); err != nil {
		t.Fatalf("首次挂载失败: %v", err)
	}

	a2 := seedAdapter(t, adapters, "sql-expert-v2", "t1", "qwen2-7b")
	dup := newMount(a2, "t1", "qwen2-7b", "sql-expert") 
	if err := mounts.Mount(ctx, dup); !errors.Is(err, ports.ErrMountConflict) {
		t.Fatalf("重复对外名应返回 ErrMountConflict，实际: %v", err)
	}
}

func TestMountTenantIsolation(t *testing.T) {
	adapters, mounts := newMountTestRepos(t)
	ctx := context.Background()

	id := seedAdapter(t, adapters, "sql-expert", "t1", "qwen2-7b")
	if err := mounts.Mount(ctx, newMount(id, "t1", "qwen2-7b", "sql-expert")); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	t.Run("他租户无法解析", func(t *testing.T) {
		_, err := mounts.ResolveServedName(ctx, "t2", "qwen2-7b:sql-expert")
		if !errors.Is(err, ports.ErrAdapterNotMounted) {
			t.Fatalf("跨租户解析应失败，实际: %v", err)
		}
	})

	t.Run("他租户无法卸载", func(t *testing.T) {
		err := mounts.Unmount(ctx, "t2", id, "svc-1")
		if !errors.Is(err, ports.ErrAdapterNotMounted) {
			t.Fatalf("跨租户卸载应失败，实际: %v", err)
		}
		
		if _, err := mounts.ResolveServedName(ctx, "t1", "qwen2-7b:sql-expert"); err != nil {
			t.Fatalf("跨租户卸载不应影响原挂载: %v", err)
		}
	})

	t.Run("同名对外名在不同租户下可共存", func(t *testing.T) {
		other := seedAdapter(t, adapters, "sql-expert", "t2", "qwen2-7b")
		
		if err := mounts.Mount(ctx, newMount(other, "t2", "qwen2-7b", "sql-expert")); err != nil {
			t.Fatalf("不同租户同名挂载应被允许: %v", err)
		}
	})
}

func TestUnmountThenResolveFails(t *testing.T) {
	adapters, mounts := newMountTestRepos(t)
	ctx := context.Background()

	id := seedAdapter(t, adapters, "sql-expert", "t1", "qwen2-7b")
	if err := mounts.Mount(ctx, newMount(id, "t1", "qwen2-7b", "sql-expert")); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if err := mounts.Unmount(ctx, "t1", id, "svc-1"); err != nil {
		t.Fatalf("Unmount: %v", err)
	}

	if _, err := mounts.ResolveServedName(ctx, "t1", "qwen2-7b:sql-expert"); !errors.Is(err, ports.ErrAdapterNotMounted) {
		t.Fatalf("卸载后不应再解析到路由，实际: %v", err)
	}
}

func TestListMounts(t *testing.T) {
	adapters, mounts := newMountTestRepos(t)
	ctx := context.Background()

	a1 := seedAdapter(t, adapters, "sql-expert", "t1", "qwen2-7b")
	a2 := seedAdapter(t, adapters, "chat-tuned", "t1", "qwen2-7b")

	m1 := newMount(a1, "t1", "qwen2-7b", "sql-expert")
	m2 := newMount(a2, "t1", "qwen2-7b", "chat-tuned")
	m2.ServiceID = "svc-2"
	for _, m := range []*ports.AdapterMount{m1, m2} {
		if err := mounts.Mount(ctx, m); err != nil {
			t.Fatalf("Mount: %v", err)
		}
	}

	byAdapter, err := mounts.ListByAdapter(ctx, "t1", a1)
	if err != nil {
		t.Fatalf("ListByAdapter: %v", err)
	}
	if len(byAdapter) != 1 || byAdapter[0].ServiceID != "svc-1" {
		t.Fatalf("按适配器过滤失效: %+v", byAdapter)
	}

	byService, err := mounts.ListByService(ctx, "t1", "svc-2")
	if err != nil {
		t.Fatalf("ListByService: %v", err)
	}
	if len(byService) != 1 || byService[0].AdapterID != a2 {
		t.Fatalf("按服务过滤失效: %+v", byService)
	}

	empty, err := mounts.ListByAdapter(ctx, "t2", a1)
	if err != nil {
		t.Fatalf("ListByAdapter 跨租户: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("跨租户不应看到挂载: %+v", empty)
	}
}

func TestDeleteAdapterBlockedWhileMounted(t *testing.T) {
	adapters, mounts := newMountTestRepos(t)
	ctx := context.Background()

	id := seedAdapter(t, adapters, "sql-expert", "t1", "qwen2-7b")
	if err := mounts.Mount(ctx, newMount(id, "t1", "qwen2-7b", "sql-expert")); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	err := adapters.Delete(ctx, "t1", id)
	if !errors.Is(err, ports.ErrAdapterMounted) {
		t.Fatalf("挂载中删除应返回 ErrAdapterMounted，实际: %v", err)
	}
	if _, err := adapters.Get(ctx, "t1", id); err != nil {
		t.Fatalf("删除被拒后适配器应仍存在: %v", err)
	}

	if err := mounts.Unmount(ctx, "t1", id, "svc-1"); err != nil {
		t.Fatalf("Unmount: %v", err)
	}
	if err := adapters.Delete(ctx, "t1", id); err != nil {
		t.Fatalf("卸载后应可删除: %v", err)
	}
}