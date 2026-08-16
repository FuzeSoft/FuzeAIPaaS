package tests

import (
	"net/http"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"fuze-ai-paas/backend/internal/models"
)

func seedTenantUser(t *testing.T, env *TestEnv, tenantID, username string, role models.Role) string {
	t.Helper()

	if err := env.Store.CreateTenant(&models.Tenant{
		ID:   tenantID,
		Name: tenantID,
	}); err != nil {
		t.Fatalf("create tenant %s: %v", tenantID, err)
	}
	if err := env.Store.UpsertQuota(&models.Quota{
		ID:            tenantID,
		TenantID:      tenantID,
		GPUQuota:      128,
		MemoryQuotaGB: 2048,
		JobQuota:      50,
	}); err != nil {
		t.Fatalf("upsert quota %s: %v", tenantID, err)
	}

	const password = "passw0rd123"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt %s: %v", username, err)
	}
	if err := env.Store.CreateUser(&models.User{
		ID:          "u-" + username,
		Username:    username,
		DisplayName: username,
		Password:    string(hash),
		Role:        role,
		TenantID:    tenantID,
		Email:       username + "@fuze.local",
		SSOProvider: "local",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return env.LoginAndGetToken(t, username, password)
}

func createInferenceSvc(t *testing.T, env *TestEnv, token, name string) string {
	t.Helper()
	w := env.DoAuthJSON(http.MethodPost, "/api/v1/inference-services", token, map[string]interface{}{
		"spec": map[string]interface{}{
			"name":         name,
			"framework":    "pytorch",
			"storage_uri":  "s3://bucket/" + name,
			"gpus":         1,
			"min_replicas": 1,
			"max_replicas": 2,
		},
	})
	AssertStatus(t, w, http.StatusCreated)
	id := ParseJSON[svcView](t, w).ID
	if id == "" {
		t.Fatal("created inference service has empty id")
	}
	return id
}

func TestInferenceServiceTenantIsolation(t *testing.T) {
	env := NewTestEnvWithAuth(t)

	victimToken := seedTenantUser(t, env, "tenant-victim", "victim-dev", models.RoleDeveloper)
	attackerToken := seedTenantUser(t, env, "tenant-attacker", "attacker-dev", models.RoleDeveloper)

	victimSvcID := createInferenceSvc(t, env, victimToken, "victim-llm")
	attackerSvcID := createInferenceSvc(t, env, attackerToken, "attacker-llm")

	t.Run("list 不得泄露他租户推理服务", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/inference-services", attackerToken)
		AssertStatus(t, w, http.StatusOK)

		for _, sv := range ParseJSON[[]svcView](t, w) {
			if sv.ID == victimSvcID {
				t.Fatalf("越权：攻击者租户列表中出现了受害者的推理服务 %s", victimSvcID)
			}
		}
	})

	t.Run("list 仍能看到本租户资源", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/inference-services", attackerToken)
		AssertStatus(t, w, http.StatusOK)

		var found bool
		for _, sv := range ParseJSON[[]svcView](t, w) {
			if sv.ID == attackerSvcID {
				found = true
			}
		}
		if !found {
			t.Fatalf("租户过滤过严：本租户资源 %s 未出现在列表中", attackerSvcID)
		}
	})

	t.Run("get 他租户资源按不存在处理", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/inference-services/"+victimSvcID, attackerToken)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("patch 他租户资源按不存在处理且不改动数据", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPatch, "/api/v1/inference-services/"+victimSvcID, attackerToken,
			map[string]interface{}{"spec": map[string]interface{}{"target_replicas": 99}})
		AssertStatus(t, w, http.StatusNotFound)

		svc, err := env.Store.GetInferenceService(victimSvcID)
		if err != nil {
			t.Fatalf("受害者资源应仍然存在: %v", err)
		}
		if svc.TargetReplicas == 99 {
			t.Fatal("越权：攻击者成功修改了受害者租户的期望副本数")
		}
	})

	t.Run("delete 他租户资源按不存在处理且不删除数据", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/inference-services/"+victimSvcID, attackerToken, nil)
		AssertStatus(t, w, http.StatusNotFound)

		if _, err := env.Store.GetInferenceService(victimSvcID); err != nil {
			t.Fatalf("越权：受害者租户的推理服务被攻击者删除: %v", err)
		}
	})

	t.Run("本租户资源仍可正常读写", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/inference-services/"+attackerSvcID, attackerToken)
		AssertStatus(t, w, http.StatusOK)

		w = env.DoAuthJSON(http.MethodPatch, "/api/v1/inference-services/"+attackerSvcID, attackerToken,
			map[string]interface{}{"spec": map[string]interface{}{"target_replicas": 2}})
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("平台管理员保留跨租户视图", func(t *testing.T) {
		adminToken := env.LoginAndGetToken(t, "admin", "admin123")

		w := env.DoAuthGET(http.MethodGet, "/api/v1/inference-services", adminToken)
		AssertStatus(t, w, http.StatusOK)

		seen := map[string]bool{}
		for _, sv := range ParseJSON[[]svcView](t, w) {
			seen[sv.ID] = true
		}
		if !seen[victimSvcID] || !seen[attackerSvcID] {
			t.Fatalf("平台管理员应能看到全部租户资源，实际: %v", seen)
		}

		w = env.DoAuthGET(http.MethodGet, "/api/v1/inference-services/"+victimSvcID, adminToken)
		AssertStatus(t, w, http.StatusOK)
	})
}