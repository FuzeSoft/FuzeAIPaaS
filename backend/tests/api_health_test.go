package tests

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("health check returns OK", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/health")
		AssertStatus(t, w, http.StatusOK)
		var body map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &body)
		if body["status"] != "healthy" {
			t.Errorf("expected status healthy, got %v", body["status"])
		}
	})

	t.Run("health includes mode", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/health")
		body := ParseJSON[map[string]interface{}](t, w)
		if _, ok := body["mode"]; !ok {
			t.Error("expected mode field in health response")
		}
	})

	t.Run("health includes auth flag", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/health")
		body := ParseJSON[map[string]interface{}](t, w)
		if _, ok := body["auth"]; !ok {
			t.Error("expected auth field in health response")
		}
	})
}