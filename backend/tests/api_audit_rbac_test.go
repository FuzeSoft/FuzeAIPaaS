package tests

import (
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func seedAudit(t *testing.T, env *TestEnv, tenant, actor string) {
	t.Helper()
	if err := env.Store.Record(&models.AuditLog{
		Actor:        actor,
		ActorID:      actor,
		ActorRole:    models.RoleDeveloper,
		TenantID:     tenant,
		Action:       "create",
		ResourceType: "job",
		ResourceID:   "res-" + tenant,
		Detail:       "seeded",
		ClientIP:     "10.0.0.1",
	}); err != nil {
		t.Fatalf("seed audit for tenant %s: %v", tenant, err)
	}
}

func TestAuditRBAC(t *testing.T) {
	env := NewTestEnvWithAuth(t)

	seedAudit(t, env, "audit-acme", "alice")

	t.Run("viewer 不得访问审计日志", func(t *testing.T) {
		tok := mintToken(env, "v1", "audit-acme", models.RoleViewer)
		w := env.DoAuthGET(http.MethodGet, "/api/v1/audit", tok)
		AssertStatus(t, w, http.StatusForbidden)
	})

	t.Run("developer 不得访问审计日志", func(t *testing.T) {
		tok := mintToken(env, "d1", "audit-acme", models.RoleDeveloper)
		w := env.DoAuthGET(http.MethodGet, "/api/v1/audit", tok)
		AssertStatus(t, w, http.StatusForbidden)
	})

	t.Run("tenant_admin 可访问审计日志", func(t *testing.T) {
		tok := mintToken(env, "ta1", "audit-acme", models.RoleTenantAdmin)
		w := env.DoAuthGET(http.MethodGet, "/api/v1/audit", tok)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("platform_admin 可访问审计日志", func(t *testing.T) {
		tok := mintToken(env, "pa1", "default", models.RolePlatformAdmin)
		w := env.DoAuthGET(http.MethodGet, "/api/v1/audit", tok)
		AssertStatus(t, w, http.StatusOK)
	})
}

func TestAuditTenantIsolation(t *testing.T) {
	env := NewTestEnvWithAuth(t)

	const victim, attacker = "audit-victim", "audit-attacker"
	seedAudit(t, env, victim, "victim-user")
	seedAudit(t, env, attacker, "attacker-user")

	attackerTok := mintToken(env, "atk", attacker, models.RoleTenantAdmin)
	adminTok := mintToken(env, "root", "default", models.RolePlatformAdmin)

	t.Run("租户管理员传他人 tenant_id 不得越权", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/audit?tenant_id="+victim, attackerTok)
		AssertStatus(t, w, http.StatusOK)

		for _, e := range ParseJSON[[]models.AuditLog](t, w) {
			if e.TenantID != attacker {
				t.Fatalf("越权：攻击者读到了租户 %s 的审计记录（actor=%s, ip=%s）",
					e.TenantID, e.Actor, e.ClientIP)
			}
		}
	})

	t.Run("租户管理员不传 tenant_id 仅见本租户", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/audit", attackerTok)
		AssertStatus(t, w, http.StatusOK)

		logs := ParseJSON[[]models.AuditLog](t, w)
		if len(logs) == 0 {
			t.Fatal("租户管理员应能看到本租户审计记录")
		}
		for _, e := range logs {
			if e.TenantID != attacker {
				t.Fatalf("越权：返回了租户 %s 的审计记录", e.TenantID)
			}
		}
	})

	t.Run("平台管理员可按 tenant_id 跨租户查询", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/audit?tenant_id="+victim, adminTok)
		AssertStatus(t, w, http.StatusOK)

		logs := ParseJSON[[]models.AuditLog](t, w)
		if len(logs) == 0 {
			t.Fatal("平台管理员应能查询指定租户的审计记录")
		}
		for _, e := range logs {
			if e.TenantID != victim {
				t.Fatalf("tenant_id 过滤未生效，返回了租户 %s 的记录", e.TenantID)
			}
		}
	})

	t.Run("平台管理员不传 tenant_id 可见全租户", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/audit", adminTok)
		AssertStatus(t, w, http.StatusOK)

		seen := map[string]bool{}
		for _, e := range ParseJSON[[]models.AuditLog](t, w) {
			seen[e.TenantID] = true
		}
		if !seen[victim] || !seen[attacker] {
			t.Fatalf("平台管理员应看到全部租户记录，实际仅见 %v", seen)
		}
	})
}