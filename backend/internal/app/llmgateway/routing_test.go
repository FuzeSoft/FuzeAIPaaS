package llmgateway

import (
	"context"
	"testing"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newRoutingTestStore(t *testing.T) *storage.Storage {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&models.LLMRoute{}, &models.LLMPrice{}, &models.LLMTokenQuota{},
		&models.LLMUsageRecord{}, &models.LLMTrace{}, &models.LLMPrompt{},
		&models.LLMKnowledgeBase{}, &models.LLMDocument{}, &models.LLMAdapter{},
	); err != nil {
		t.Fatalf("migrate llm models: %v", err)
	}
	return storage.NewStorage(db)
}

func TestRepoRouteTableTenantIsolation(t *testing.T) {
	store := newRoutingTestStore(t)
	ctx := context.Background()

	if err := store.Route().Save(ctx, "tenantA", llm.Route{
		Model: "gpt-4o-mini",
		Backends: []llm.Backend{
			{Name: "a-only", Endpoint: "http://a.local/v1", Weight: 1, Healthy: true},
		},
	}); err != nil {
		t.Fatalf("save A route: %v", err)
	}
	
	if err := store.Route().Save(ctx, "default", llm.Route{
		Model: "gpt-3.5-turbo",
		Backends: []llm.Backend{
			{Name: "shared", Endpoint: "http://shared.local/v1", Weight: 1, Healthy: true},
		},
	}); err != nil {
		t.Fatalf("save default route: %v", err)
	}
	
	if err := store.Route().Save(ctx, "tenantB", llm.Route{
		Model: "claude-3",
		Backends: []llm.Backend{
			{Name: "b-only", Endpoint: "http://b.local/v1", Weight: 1, Healthy: true},
		},
	}); err != nil {
		t.Fatalf("save B route: %v", err)
	}

	rt := NewRepoRouteTable(store.Route())

	got, err := rt.PickForTenant("tenantA", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("pick tenantA: %v", err)
	}
	if len(got) != 1 || got[0].Name != "a-only" {
		t.Fatalf("tenantA 隔离失败：%+v", got)
	}

	got, err = rt.PickForTenant("tenantX", "gpt-3.5-turbo")
	if err != nil {
		t.Fatalf("pick tenantX: %v", err)
	}
	if len(got) != 1 || got[0].Name != "shared" {
		t.Fatalf("tenantX 回落 default 失败：%+v", got)
	}

	if _, err := rt.PickForTenant("tenantB", "gpt-4o-mini"); err == nil {
		t.Fatal("tenantB 不应看到 tenantA 的私有模型")
	}
}

func TestRepoRouteTableUnhealthyExcludedAfterMark(t *testing.T) {
	store := newRoutingTestStore(t)
	ctx := context.Background()
	if err := store.Route().Save(ctx, "default", llm.Route{
		Model: "m",
		Backends: []llm.Backend{
			{Name: "good", Endpoint: "http://good/v1", Weight: 1, Healthy: true},
			{Name: "bad", Endpoint: "http://bad/v1", Weight: 1, Healthy: true},
		},
	}); err != nil {
		t.Fatalf("save route: %v", err)
	}

	rt := NewRepoRouteTable(store.Route())
	
	if _, err := rt.PickForTenant("default", "m"); err != nil {
		t.Fatalf("first pick: %v", err)
	}
	
	if !rt.SetHealth("m", "bad", false) {
		t.Fatal("SetHealth 应命中已知后端")
	}
	
	got, err := rt.PickForTenant("default", "m")
	if err != nil {
		t.Fatalf("pick after mark: %v", err)
	}
	for _, b := range got {
		if b.Name == "bad" {
			t.Fatal("不健康的后端未被剔除")
		}
	}
	if len(got) != 1 || got[0].Name != "good" {
		t.Fatalf("期望只剩 good：%+v", got)
	}
}

func TestRepoRouteTableWeightedOrdering(t *testing.T) {
	store := newRoutingTestStore(t)
	ctx := context.Background()
	if err := store.Route().Save(ctx, "default", llm.Route{
		Model:    "m",
		Strategy: llm.StrategyWeighted,
		Backends: []llm.Backend{
			{Name: "light", Endpoint: "http://light/v1", Weight: 1, Healthy: true},
			{Name: "heavy", Endpoint: "http://heavy/v1", Weight: 9, Healthy: true},
		},
	}); err != nil {
		t.Fatalf("save route: %v", err)
	}
	rt := NewRepoRouteTable(store.Route())

	const n = 5000
	heavy, light := 0, 0
	for i := 0; i < n; i++ {
		got, err := rt.PickForTenant("default", "m")
		if err != nil {
			t.Fatalf("pick: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("weighted 应返回全部健康后端：%v", got)
		}
		
		switch got[0].Name {
		case "heavy":
			heavy++
		case "light":
			light++
		default:
			t.Fatalf("选中未知后端: %s", got[0].Name)
		}
	}
	
	ratio := float64(heavy) / float64(n)
	if ratio < 0.70 || ratio > 0.95 {
		t.Fatalf("weighted 选主分布异常：heavy 占比 %.3f (期望 ~0.9)", ratio)
	}
}

func TestRepoRouteTableWeightedZeroWeight(t *testing.T) {
	store := newRoutingTestStore(t)
	ctx := context.Background()
	if err := store.Route().Save(ctx, "default", llm.Route{
		Model:    "m",
		Strategy: llm.StrategyWeighted,
		Backends: []llm.Backend{
			{Name: "a", Endpoint: "http://a/v1", Weight: 0, Healthy: true},
			{Name: "b", Endpoint: "http://b/v1", Weight: 0, Healthy: true},
		},
	}); err != nil {
		t.Fatalf("save route: %v", err)
	}
	rt := NewRepoRouteTable(store.Route())

	seen := map[string]int{}
	for i := 0; i < 400; i++ {
		got, err := rt.PickForTenant("default", "m")
		if err != nil {
			t.Fatalf("pick: %v", err)
		}
		seen[got[0].Name]++
	}
	
	if len(seen) != 2 {
		t.Fatalf("零权重后端未被兜底均分：%v", seen)
	}
	for name, c := range seen {
		if c < 100 || c > 300 {
			t.Fatalf("零权重兜底不均：%s=%d", name, c)
		}
	}
}

func TestRepoRouteTableRoundRobinRotation(t *testing.T) {
	store := newRoutingTestStore(t)
	ctx := context.Background()
	if err := store.Route().Save(ctx, "default", llm.Route{
		Model:    "m",
		Strategy: llm.StrategyRoundRobin,
		Backends: []llm.Backend{
			{Name: "a", Endpoint: "http://a/v1", Weight: 1, Healthy: true},
			{Name: "b", Endpoint: "http://b/v1", Weight: 1, Healthy: true},
			{Name: "c", Endpoint: "http://c/v1", Weight: 1, Healthy: true},
		},
	}); err != nil {
		t.Fatalf("save route: %v", err)
	}
	rt := NewRepoRouteTable(store.Route())

	want := []string{"a", "b", "c"}
	for i, w := range want {
		got, err := rt.PickForTenant("default", "m")
		if err != nil {
			t.Fatalf("pick #%d: %v", i, err)
		}
		if got[0].Name != w {
			t.Fatalf("第 %d 次轮询首位期望 %s，got %s", i, w, got[0].Name)
		}
		
		if len(got) != 3 {
			t.Fatalf("round_robin 应返回全量旋转序列，got %v", got)
		}
	}
}

func TestRepoRouteTableAllUnhealthy(t *testing.T) {
	store := newRoutingTestStore(t)
	ctx := context.Background()
	if err := store.Route().Save(ctx, "default", llm.Route{
		Model:    "m",
		Strategy: llm.StrategyWeighted,
		Backends: []llm.Backend{
			{Name: "x", Endpoint: "http://x/v1", Weight: 1, Healthy: true},
			{Name: "y", Endpoint: "http://y/v1", Weight: 1, Healthy: true},
		},
	}); err != nil {
		t.Fatalf("save route: %v", err)
	}
	rt := NewRepoRouteTable(store.Route())
	
	if _, err := rt.PickForTenant("default", "m"); err != nil {
		t.Fatalf("preflight pick: %v", err)
	}
	
	if !rt.SetHealth("m", "x", false) || !rt.SetHealth("m", "y", false) {
		t.Fatal("SetHealth 应命中已知后端")
	}
	if _, err := rt.PickForTenant("default", "m"); err != llm.ErrNoHealthyBackend {
		t.Fatalf("全部不健康时应返回 ErrNoHealthyBackend，got %v", err)
	}
}