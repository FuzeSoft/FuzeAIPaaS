package tests

import (
	"net/http"
	"testing"
)

func TestAPITokenLifecycle(t *testing.T) {
	env := NewTestEnv(t)

	w := env.DoJSON(http.MethodPost, "/api/v1/auth/tokens", map[string]interface{}{
		"name":      "ci",
		"ttl_hours": 24,
	})
	AssertStatus(t, w, http.StatusCreated)
	issued := ParseJSON[map[string]interface{}](t, w)
	token, _ := issued["token"].(string)
	if token == "" {
		t.Fatalf("expected token in response, got %+v", issued)
	}

	w = env.DoJSON(http.MethodGet, "/api/v1/auth/tokens", nil)
	AssertStatus(t, w, http.StatusOK)
	list := ParseJSON[map[string]interface{}](t, w)
	tokens, _ := list["tokens"].([]interface{})
	if len(tokens) < 1 {
		t.Fatalf("expected at least 1 token, got %d", len(tokens))
	}

	w = env.DoAuthJSON(http.MethodGet, "/api/v1/auth/me", token, nil)
	AssertStatus(t, w, http.StatusOK)

	first := tokens[0].(map[string]interface{})
	id, _ := first["id"].(string)
	w = env.DoJSON(http.MethodDelete, "/api/v1/auth/tokens/"+id, nil)
	AssertStatus(t, w, http.StatusOK)

	w = env.DoJSON(http.MethodGet, "/api/v1/auth/tokens", nil)
	list = ParseJSON[map[string]interface{}](t, w)
	tokens, _ = list["tokens"].([]interface{})
	for _, tk := range tokens {
		if tk.(map[string]interface{})["id"].(string) == id {
			t.Fatalf("token %s should have been revoked", id)
		}
	}
	
}

func TestAPITokenRevokeInvalidatesToken(t *testing.T) {
	env := NewTestEnvWithAuth(t)

	loginTok := env.LoginAndGetToken(t, "admin", "admin123")

	w := env.DoAuthJSON(http.MethodPost, "/api/v1/auth/tokens", loginTok, map[string]interface{}{
		"name":      "ci",
		"ttl_hours": 24,
	})
	AssertStatus(t, w, http.StatusCreated)
	issued := ParseJSON[map[string]interface{}](t, w)
	pat, _ := issued["token"].(string)
	if pat == "" {
		t.Fatalf("expected PAT in response, got %+v", issued)
	}

	w = env.DoAuthJSON(http.MethodGet, "/api/v1/auth/me", pat, nil)
	AssertStatus(t, w, http.StatusOK)

	list := ParseJSON[map[string]interface{}](t, env.DoAuthJSON(http.MethodGet, "/api/v1/auth/tokens", loginTok, nil))
	tokens, _ := list["tokens"].([]interface{})
	var patID string
	for _, tk := range tokens {
		if tk.(map[string]interface{})["prefix"].(string) == issued["prefix"].(string) {
			patID = tk.(map[string]interface{})["id"].(string)
			break
		}
	}
	if patID == "" {
		t.Fatalf("could not locate issued PAT id in list")
	}
	w = env.DoAuthJSON(http.MethodDelete, "/api/v1/auth/tokens/"+patID, loginTok, nil)
	AssertStatus(t, w, http.StatusOK)

	w = env.DoAuthJSON(http.MethodGet, "/api/v1/auth/me", pat, nil)
	AssertStatus(t, w, http.StatusUnauthorized)
}

func TestAPITokenCreateRequiresName(t *testing.T) {
	env := NewTestEnv(t)
	w := env.DoJSON(http.MethodPost, "/api/v1/auth/tokens", map[string]interface{}{})
	AssertStatus(t, w, http.StatusBadRequest)
	AssertError(t, w)
}