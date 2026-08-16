package tests

import (
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/models"
)

func guardrailPayload(name, pattern string) map[string]any {
	return map[string]any{
		"name":        name,
		"category":    llm.CategoryPII,
		"direction":   llm.DirectionBoth,
		"action":      llm.ActionRedact,
		"pattern":     pattern,
		"replacement": "[X]",
		"enabled":     true,
	}
}

func TestGuardrailRuleRBAC(t *testing.T) {
	env := NewTestEnvWithAuth(t)

	t.Run("viewer 不得创建规则", func(t *testing.T) {
		tok := mintToken(env, "v1", "gr-acme", models.RoleViewer)
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/llm/guardrail/rules", tok,
			guardrailPayload("v_rule", `\d{6}`))
		AssertStatus(t, w, http.StatusForbidden)
	})

	t.Run("developer 不得创建规则", func(t *testing.T) {
		tok := mintToken(env, "d1", "gr-acme", models.RoleDeveloper)
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/llm/guardrail/rules", tok,
			guardrailPayload("d_rule", `\d{6}`))
		AssertStatus(t, w, http.StatusForbidden)
	})

	t.Run("tenant_admin 可创建本租户规则", func(t *testing.T) {
		tok := mintToken(env, "ta1", "gr-acme", models.RoleTenantAdmin)
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/llm/guardrail/rules", tok,
			guardrailPayload("ta_rule", `EMP-\d{6}`))
		AssertStatus(t, w, http.StatusOK)
	})
}

func TestGuardrailRuleTenantIsolation(t *testing.T) {
	env := NewTestEnvWithAuth(t)

	aTok := mintToken(env, "a1", "gr-a", models.RoleTenantAdmin)
	bTok := mintToken(env, "b1", "gr-b", models.RoleTenantAdmin)

	w := env.DoAuthJSON(http.MethodPost, "/api/v1/llm/guardrail/rules", aTok,
		guardrailPayload("a_secret", `AAA-\d{4}`))
	AssertStatus(t, w, http.StatusOK)

	t.Run("他租户不得看到本租户规则", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/llm/guardrail/rules", bTok)
		AssertStatus(t, w, http.StatusOK)
		for _, r := range ParseJSON[[]models.LLMGuardrailRule](t, w) {
			if r.Name == "a_secret" {
				t.Fatalf("租户隔离失效：读到他租户规则 %+v", r)
			}
		}
	})

	t.Run("创建时 tenant_id 被强制为自身租户", func(t *testing.T) {
		payload := guardrailPayload("spoof", `\d{5}`)
		payload["tenant_id"] = "gr-a" 
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/llm/guardrail/rules", bTok, payload)
		AssertStatus(t, w, http.StatusOK)

		w = env.DoAuthGET(http.MethodGet, "/api/v1/llm/guardrail/rules", aTok)
		for _, r := range ParseJSON[[]models.LLMGuardrailRule](t, w) {
			if r.Name == "spoof" {
				t.Fatal("越权：规则被写入了他人租户")
			}
		}
	})
}

func TestGuardrailRuleValidation(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	tok := mintToken(env, "ta1", "gr-v", models.RoleTenantAdmin)

	t.Run("坏正则返回 400", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/llm/guardrail/rules", tok,
			guardrailPayload("bad", `([unclosed`))
		AssertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("未知 action 返回 400", func(t *testing.T) {
		p := guardrailPayload("bad_action", `\d+`)
		p["action"] = "explode"
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/llm/guardrail/rules", tok, p)
		AssertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("pattern 与 keywords 均为空返回 400", func(t *testing.T) {
		p := guardrailPayload("empty", "")
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/llm/guardrail/rules", tok, p)
		AssertStatus(t, w, http.StatusBadRequest)
	})
}

func TestGuardrailRuleTakesEffect(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	tok := mintToken(env, "ta1", "gr-live", models.RoleTenantAdmin)

	w := env.DoAuthJSON(http.MethodPost, "/api/v1/llm/guardrail/rules", tok,
		guardrailPayload("live_rule", `LIVE-\d{4}`))
	AssertStatus(t, w, http.StatusOK)

	w = env.DoAuthGET(http.MethodGet, "/api/v1/llm/guardrail/rules", tok)
	AssertStatus(t, w, http.StatusOK)
	rules := ParseJSON[[]models.LLMGuardrailRule](t, w)
	if len(rules) != 1 || rules[0].Name != "live_rule" {
		t.Fatalf("规则未正确写入: %+v", rules)
	}
}

func TestGuardrailRuleDelete(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	aTok := mintToken(env, "a1", "gr-d-a", models.RoleTenantAdmin)
	bTok := mintToken(env, "b1", "gr-d-b", models.RoleTenantAdmin)

	w := env.DoAuthJSON(http.MethodPost, "/api/v1/llm/guardrail/rules", aTok,
		guardrailPayload("to_delete", `\d{7}`))
	AssertStatus(t, w, http.StatusOK)
	created := ParseJSON[models.LLMGuardrailRule](t, w)

	t.Run("他租户删除应 404", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/llm/guardrail/rules/"+created.ID, bTok, nil)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("本租户可删除", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/llm/guardrail/rules/"+created.ID, aTok, nil)
		AssertStatus(t, w, http.StatusOK)
	})
}