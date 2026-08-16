package storage

import (
	"context"
	"testing"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/models"
)

func newGuardrailTestRepo(t *testing.T) *GuardrailRepo {
	t.Helper()
	db := testDB(t)
	return NewGuardrailRepository(db)
}

func mustUpsert(t *testing.T, repo *GuardrailRepo, r *models.LLMGuardrailRule) {
	t.Helper()
	if err := repo.Upsert(context.Background(), r); err != nil {
		t.Fatalf("upsert rule %s: %v", r.Name, err)
	}
}

func TestGuardrailFallbackToDefaults(t *testing.T) {
	repo := newGuardrailTestRepo(t)
	ctx := context.Background()

	t.Run("规则表为空时回退内建规则", func(t *testing.T) {
		rules, err := repo.Resolve(ctx, "any-tenant")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(rules) == 0 {
			t.Fatal("规则集为空——护栏将完全失效，必须回退到内建默认规则")
		}
		if len(rules) != len(llm.DefaultRules()) {
			t.Fatalf("应回退到全部内建规则，got %d want %d", len(rules), len(llm.DefaultRules()))
		}
	})

	t.Run("租户仅有停用规则时仍回退内建规则", func(t *testing.T) {
		mustUpsert(t, repo, &models.LLMGuardrailRule{
			ID: "g-disabled", TenantID: "t-off", Name: "off",
			Category: llm.CategoryPII, Direction: llm.DirectionBoth,
			Action: llm.ActionRedact, Pattern: `\d{4}`, Enabled: false,
		})
		rules, err := repo.Resolve(ctx, "t-off")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(rules) != len(llm.DefaultRules()) {
			t.Fatalf("停用规则不应导致空集，got %d", len(rules))
		}
	})
}

func TestGuardrailResolutionOrder(t *testing.T) {
	repo := newGuardrailTestRepo(t)
	ctx := context.Background()

	mustUpsert(t, repo, &models.LLMGuardrailRule{
		ID: "g-global", TenantID: "", Name: "global_rule",
		Category: llm.CategorySensitive, Direction: llm.DirectionBoth,
		Action: llm.ActionBlock, Keywords: "内部机密", Enabled: true,
	})
	
	mustUpsert(t, repo, &models.LLMGuardrailRule{
		ID: "g-tenant", TenantID: "t-custom", Name: "tenant_rule",
		Category: llm.CategoryPII, Direction: llm.DirectionBoth,
		Action: llm.ActionRedact, Pattern: `EMP-\d{6}`, Replacement: "[EMP]", Enabled: true,
	})

	t.Run("租户有自定义规则时优先生效", func(t *testing.T) {
		rules, err := repo.Resolve(ctx, "t-custom")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(rules) != 1 || rules[0].Name != "tenant_rule" {
			t.Fatalf("应仅生效租户自定义规则，got %+v", rules)
		}
	})

	t.Run("租户无规则时回退全局规则", func(t *testing.T) {
		rules, err := repo.Resolve(ctx, "t-none")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(rules) != 1 || rules[0].Name != "global_rule" {
			t.Fatalf("应回退到全局规则，got %+v", rules)
		}
	})

	t.Run("解析出的规则可直接构造可用护栏", func(t *testing.T) {
		rules, err := repo.Resolve(ctx, "t-custom")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		res := llm.NewGuard(rules...).Check("工号 EMP-123456", llm.DirectionInput)
		if !res.Modified() {
			t.Fatalf("租户自定义脱敏规则未生效: %+v", res)
		}
	})
}

func TestGuardrailRuleCRUD(t *testing.T) {
	repo := newGuardrailTestRepo(t)
	ctx := context.Background()

	mustUpsert(t, repo, &models.LLMGuardrailRule{
		ID: "g-1", TenantID: "t-1", Name: "r1",
		Category: llm.CategoryPII, Direction: llm.DirectionBoth,
		Action: llm.ActionRedact, Pattern: `\d{6}`, Enabled: true,
	})

	t.Run("列出租户规则", func(t *testing.T) {
		got, err := repo.List(ctx, "t-1")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 1 || got[0].Name != "r1" {
			t.Fatalf("unexpected list result: %+v", got)
		}
	})

	t.Run("不得列出他租户规则", func(t *testing.T) {
		got, err := repo.List(ctx, "t-2")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("租户隔离失效，读到他人规则: %+v", got)
		}
	})

	t.Run("同 ID 再次写入为更新", func(t *testing.T) {
		mustUpsert(t, repo, &models.LLMGuardrailRule{
			ID: "g-1", TenantID: "t-1", Name: "r1",
			Category: llm.CategoryPII, Direction: llm.DirectionBoth,
			Action: llm.ActionBlock, Pattern: `\d{8}`, Enabled: true,
		})
		got, err := repo.List(ctx, "t-1")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("upsert 产生了重复记录: %+v", got)
		}
		if got[0].Action != llm.ActionBlock {
			t.Fatalf("更新未生效: %+v", got[0])
		}
	})

	t.Run("删除他租户规则应失败", func(t *testing.T) {
		if err := repo.Delete(ctx, "t-2", "g-1"); err == nil {
			t.Fatal("跨租户删除必须失败")
		}
	})

	t.Run("删除本租户规则", func(t *testing.T) {
		if err := repo.Delete(ctx, "t-1", "g-1"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		got, _ := repo.List(ctx, "t-1")
		if len(got) != 0 {
			t.Fatalf("删除未生效: %+v", got)
		}
	})
}

func TestGuardrailRejectsBadPattern(t *testing.T) {
	repo := newGuardrailTestRepo(t)
	err := repo.Upsert(context.Background(), &models.LLMGuardrailRule{
		ID: "g-bad", TenantID: "t-1", Name: "bad",
		Category: llm.CategoryPII, Direction: llm.DirectionBoth,
		Action: llm.ActionRedact, Pattern: `([unclosed`, Enabled: true,
	})
	if err == nil {
		t.Fatal("坏正则必须在写入期被拒绝，否则会成为永不命中的死规则")
	}
}