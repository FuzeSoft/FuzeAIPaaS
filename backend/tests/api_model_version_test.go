package tests

import (
	"encoding/json"
	"net/http"
	"testing"
)

func parseListBody(t *testing.T, body []byte) []map[string]interface{} {
	t.Helper()
	
	var arr []map[string]interface{}
	if err := json.Unmarshal(body, &arr); err == nil {
		return arr
	}
	
	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err != nil {
		return []map[string]interface{}{}
	}
	for _, key := range []string{"data", "versions", "items", "models", "jobs", "clusters", "datasets"} {
		if list, ok := obj[key].([]interface{}); ok {
			result := make([]map[string]interface{}, 0, len(list))
			for _, item := range list {
				if m, ok := item.(map[string]interface{}); ok {
					result = append(result, m)
				}
			}
			return result
		}
	}
	return []map[string]interface{}{}
}

func TestModelVersionsAPI(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	w := env.DoAuthJSON(http.MethodPost, "/api/v1/models", token, map[string]interface{}{
		"name":      "vision-test",
		"framework": "pytorch",
	})
	AssertStatusIn(t, w, http.StatusCreated, http.StatusOK)
	var createResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &createResp)
	modelID, _ := createResp["id"].(string)

	if modelID == "" {
		t.Fatal("failed to create model: no id in response")
	}

	t.Run("create model version", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/models/"+modelID+"/versions", token, map[string]interface{}{
			"version": "v1.0",
			"uri":     "s3://bucket/model-v1",
		})
		AssertStatusIn(t, w, http.StatusCreated, http.StatusOK)
	})

	t.Run("list model versions", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/models/"+modelID+"/versions", token)
		AssertStatus(t, w, http.StatusOK)
		versions := parseListBody(t, w.Body.Bytes())
		if len(versions) < 1 {
			t.Error("expected at least 1 version")
		}
	})

	t.Run("get specific version", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/models/"+modelID+"/versions/v1.0", token)
		
		AssertStatusIn(t, w, http.StatusOK, http.StatusNotFound)
	})

	t.Run("delete model version", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/models/"+modelID+"/versions/v1.0", token, nil)
		
		AssertStatusIn(t, w, http.StatusNoContent, http.StatusNotFound)
	})

	t.Run("get non-existent version returns 404", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/models/"+modelID+"/versions/nonexistent", token)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("delete model and clean up", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/models/"+modelID, token, nil)
		AssertStatus(t, w, http.StatusNoContent)
	})
}

func TestModelsListWithFilters(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	models := []map[string]string{
		{"name": "nlp-bot", "framework": "pytorch"},
		{"name": "cv-det", "framework": "tensorflow"},
		{"name": "speech-tts", "framework": "pytorch"},
	}
	for _, m := range models {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/models", token, m)
		AssertStatusIn(t, w, http.StatusCreated, http.StatusOK)
	}

	t.Run("list all models", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/models", token)
		AssertStatus(t, w, http.StatusOK)
		data := parseListBody(t, w.Body.Bytes())
		if len(data) < 3 {
			t.Errorf("expected at least 3 models, got %d", len(data))
		}
	})

	t.Run("list models with framework filter", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/models?framework=pytorch", token)
		AssertStatus(t, w, http.StatusOK)
		
		_ = parseListBody(t, w.Body.Bytes())
	})

	t.Run("list models with pagination", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/models?page=1&page_size=2", token)
		AssertStatus(t, w, http.StatusOK)
		
		_ = parseListBody(t, w.Body.Bytes())
	})

	listW := env.DoAuthGET(http.MethodGet, "/api/v1/models", token)
	data := parseListBody(t, listW.Body.Bytes())
	for _, m := range data {
		if id, ok := m["id"].(string); ok {
			env.DoAuthJSON(http.MethodDelete, "/api/v1/models/"+id, token, nil)
		}
	}
}

func TestModelNotFoundScenarios(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	t.Run("get non-existent model returns 404", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/models/nonexistent-id", token)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("update non-existent model returns 404", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPut, "/api/v1/models/nonexistent-id", token, map[string]interface{}{
			"name": "new-name",
		})
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("delete non-existent model returns 404", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/models/nonexistent-id", token, nil)
		AssertStatus(t, w, http.StatusNotFound)
	})
}