package tests

import (
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestGetDatasets(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("list all datasets", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/datasets")
		AssertStatus(t, w, http.StatusOK)
		datasets := ParseJSON[[]models.Dataset](t, w)
		if len(datasets) < 2 {
			t.Errorf("expected at least 2 seeded datasets, got %d", len(datasets))
		}
	})
}

func TestGetDataset(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("get existing dataset", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/datasets/ds-001")
		AssertStatus(t, w, http.StatusOK)
		ds := ParseJSON[models.Dataset](t, w)
		if ds.Name != "imagenet-1k" {
			t.Errorf("expected imagenet-1k, got %s", ds.Name)
		}
	})

	t.Run("get non-existent dataset returns 404", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/datasets/nonexistent")
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestCreateDataset(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("create a new dataset", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/datasets", map[string]interface{}{
			"name":        "test-dataset",
			"mount_point": "oss://ai-datasets/test",
			"runtime":     "alluxio",
		})
		AssertStatus(t, w, http.StatusCreated)
		ds := ParseJSON[models.Dataset](t, w)
		if ds.Name != "test-dataset" {
			t.Errorf("expected test-dataset, got %s", ds.Name)
		}
		if ds.ClusterID != "cluster-001" {
			t.Errorf("expected cluster-001, got %s", ds.ClusterID)
		}
	})

	t.Run("create dataset with defaults", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/datasets", map[string]interface{}{
			"name":        "minimal-dataset",
			"mount_point": "s3://bucket/path",
		})
		AssertStatus(t, w, http.StatusCreated)
		ds := ParseJSON[models.Dataset](t, w)
		if ds.Runtime != models.RuntimeAlluxio {
			t.Errorf("expected default runtime alluxio, got %s", ds.Runtime)
		}
		if ds.Replicas != 1 {
			t.Errorf("expected default replicas=1, got %d", ds.Replicas)
		}
	})
}

func TestDeleteDataset(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("delete non-existent dataset returns 404", func(t *testing.T) {
		w := env.DoJSON(http.MethodDelete, "/api/v1/datasets/nonexistent", nil)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("delete existing dataset", func(t *testing.T) {
		
		w := env.DoJSON(http.MethodPost, "/api/v1/datasets", map[string]interface{}{
			"name":        "dataset-to-delete",
			"mount_point": "oss://to-delete",
		})
		AssertStatus(t, w, http.StatusCreated)
		ds := ParseJSON[models.Dataset](t, w)

		w = env.DoJSON(http.MethodDelete, "/api/v1/datasets/"+ds.ID, nil)
		AssertStatus(t, w, http.StatusNoContent)
	})
}