package tests

import (
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestGetModels(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("list all models", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/models")
		AssertStatus(t, w, http.StatusOK)
		models_list := ParseJSON[[]models.Model](t, w)
		
		_ = models_list
	})
}

func TestGetModel(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("get non-existent model returns 404", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/models/nonexistent")
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestCreateModel(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("create a new model", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/models", map[string]interface{}{
			"name":        "llama2-7b",
			"description": "Llama 2 7B chat model",
			"framework":   "pytorch",
			"owner":       "ai-team",
		})
		AssertStatus(t, w, http.StatusCreated)
		m := ParseJSON[models.Model](t, w)
		if m.Name != "llama2-7b" {
			t.Errorf("expected llama2-7b, got %s", m.Name)
		}
	})

	t.Run("create model without name returns 400", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/models", map[string]interface{}{
			"description": "no name model",
		})
		AssertStatus(t, w, http.StatusBadRequest)
	})
}

func TestUpdateModel(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("update existing model", func(t *testing.T) {
		
		w := env.DoJSON(http.MethodPost, "/api/v1/models", map[string]interface{}{
			"name": "model-to-update",
		})
		AssertStatus(t, w, http.StatusCreated)
		m := ParseJSON[models.Model](t, w)

		w = env.DoJSON(http.MethodPut, "/api/v1/models/"+m.ID, map[string]interface{}{
			"name":        "updated-model-name",
			"description": "updated description",
		})
		AssertStatus(t, w, http.StatusOK)
		updated := ParseJSON[models.Model](t, w)
		if updated.Name != "updated-model-name" {
			t.Errorf("expected updated-model-name, got %s", updated.Name)
		}
	})

	t.Run("update non-existent model returns 404", func(t *testing.T) {
		w := env.DoJSON(http.MethodPut, "/api/v1/models/nonexistent", map[string]interface{}{
			"name": "test",
		})
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestDeleteModel(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("delete non-existent model returns 404", func(t *testing.T) {
		w := env.DoJSON(http.MethodDelete, "/api/v1/models/nonexistent", nil)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("delete existing model", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/models", map[string]interface{}{
			"name": "model-to-delete",
		})
		AssertStatus(t, w, http.StatusCreated)
		m := ParseJSON[models.Model](t, w)

		w = env.DoJSON(http.MethodDelete, "/api/v1/models/"+m.ID, nil)
		AssertStatus(t, w, http.StatusNoContent)
	})
}

func TestModelVersions(t *testing.T) {
	env := NewTestEnv(t)

	w := env.DoJSON(http.MethodPost, "/api/v1/models", map[string]interface{}{
		"name": "versioned-model",
	})
	AssertStatus(t, w, http.StatusCreated)
	m := ParseJSON[models.Model](t, w)

	t.Run("create model version", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/models/"+m.ID+"/versions", map[string]interface{}{
			"version":     "v1.0.0",
			"storage_uri": "pvc://model-store/v1",
			"image":       "pytorch:latest",
		})
		AssertStatus(t, w, http.StatusCreated)
		v := ParseJSON[models.ModelVersion](t, w)
		if v.Version != "v1.0.0" {
			t.Errorf("expected v1.0.0, got %s", v.Version)
		}
	})

	t.Run("create version without version field returns 400", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/models/"+m.ID+"/versions", map[string]interface{}{
			"storage_uri": "pvc://model-store/v2",
		})
		AssertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("list model versions", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/models/"+m.ID+"/versions")
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("get specific model version", func(t *testing.T) {
		
		w := env.DoJSON(http.MethodPost, "/api/v1/models/"+m.ID+"/versions", map[string]interface{}{
			"version": "v2.0.0",
		})
		AssertStatus(t, w, http.StatusCreated)
		v := ParseJSON[models.ModelVersion](t, w)

		w = env.DoGET(http.MethodGet, "/api/v1/models/"+m.ID+"/versions/"+v.ID)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("delete model version", func(t *testing.T) {
		
		w := env.DoJSON(http.MethodPost, "/api/v1/models/"+m.ID+"/versions", map[string]interface{}{
			"version": "v-to-delete",
		})
		AssertStatus(t, w, http.StatusCreated)
		v := ParseJSON[models.ModelVersion](t, w)

		w = env.DoJSON(http.MethodDelete, "/api/v1/models/"+m.ID+"/versions/"+v.ID, nil)
		AssertStatus(t, w, http.StatusNoContent)
	})

	t.Run("create version for non-existent model returns 404", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/models/nonexistent/versions", map[string]interface{}{
			"version": "v1.0",
		})
		AssertStatus(t, w, http.StatusNotFound)
	})
}