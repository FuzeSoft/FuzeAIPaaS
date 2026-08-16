package storage

import (
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

func newAlertTestRepo(t *testing.T) *AlertRepo {
	t.Helper()
	db := testDB(t)
	return NewAlertRepository(db)
}

func TestAlertRuleCRUD(t *testing.T) {
	repo := newAlertTestRepo(t)

	if err := repo.CreateRule(&models.AlertRule{ID: "t1:r1", TenantID: "t1", Name: "r1", Expr: "up==0", Enabled: true}); err != nil {
		t.Fatalf("create: %v", err)
	}

	t.Run("列出本租户规则", func(t *testing.T) {
		got, err := repo.ListRules("t1")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 1 || got[0].Name != "r1" {
			t.Fatalf("unexpected list: %+v", got)
		}
	})

	t.Run("更新规则", func(t *testing.T) {
		rule := &models.AlertRule{ID: "t1:r1", TenantID: "t1", Name: "r1", Expr: "up==1", For: "3m", Severity: models.SeverityCritical, Enabled: false}
		if err := repo.UpdateRule(rule); err != nil {
			t.Fatalf("update: %v", err)
		}
		got, err := repo.GetRule("t1", "t1:r1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Expr != "up==1" || got.For != "3m" || got.Severity != models.SeverityCritical || got.Enabled {
			t.Fatalf("更新未生效: %+v", got)
		}
	})

	t.Run("跨租户更新应失败", func(t *testing.T) {
		rule := &models.AlertRule{ID: "t1:r1", TenantID: "t2", Name: "r1", Expr: "up==0", Enabled: true}
		if err := repo.UpdateRule(rule); err != ports.ErrAlertRuleNotFound {
			t.Fatalf("跨租户更新必须返回 ErrAlertRuleNotFound，got %v", err)
		}
	})

	t.Run("删除本租户规则", func(t *testing.T) {
		if err := repo.DeleteRule("t1", "t1:r1"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := repo.GetRule("t1", "t1:r1"); err != ports.ErrAlertRuleNotFound {
			t.Fatalf("删除后读取应返回 NotFound，got %v", err)
		}
	})
}

func TestAlertSilence(t *testing.T) {
	repo := newAlertTestRepo(t)
	now := time.Now().UTC()
	s := &models.AlertSilence{
		ID: "t1:s1", TenantID: "t1", RuleID: "t1:r1",
		StartsAt: now, EndsAt: now.Add(time.Hour), Comment: "维护窗口",
	}
	if err := repo.CreateSilence(s); err != nil {
		t.Fatalf("create silence: %v", err)
	}
	got, err := repo.ListSilences("t1")
	if err != nil {
		t.Fatalf("list silences: %v", err)
	}
	if len(got) != 1 || got[0].Comment != "维护窗口" {
		t.Fatalf("unexpected silences: %+v", got)
	}
	if err := repo.DeleteSilence("t2", "t1:s1"); err != ports.ErrAlertSilenceNotFound {
		t.Fatalf("跨租户删除静默必须失败，got %v", err)
	}
	if err := repo.DeleteSilence("t1", "t1:s1"); err != nil {
		t.Fatalf("delete silence: %v", err)
	}
}