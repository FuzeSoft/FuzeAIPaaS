package tests

import (
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestGetResources(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("list all resources", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/resources")
		AssertStatus(t, w, http.StatusOK)
		resources := ParseJSON[[]models.Resource](t, w)
		if len(resources) < 4 {
			t.Errorf("expected at least 4 seeded resources, got %d", len(resources))
		}
	})

	t.Run("filter resources by cluster_id", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/resources?cluster_id=cluster-001")
		AssertStatus(t, w, http.StatusOK)
		resources := ParseJSON[[]models.Resource](t, w)
		for _, r := range resources {
			if r.ClusterID != "cluster-001" {
				t.Errorf("expected cluster-001, got %s", r.ClusterID)
			}
		}
	})
}

func TestGetResource(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("get existing resource", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/resources/res-001")
		AssertStatus(t, w, http.StatusOK)
		res := ParseJSON[models.Resource](t, w)
		if res.Name != "gpu-node-a100-01" {
			t.Errorf("expected gpu-node-a100-01, got %s", res.Name)
		}
	})

	t.Run("get non-existent resource returns 404", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/resources/nonexistent")
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestCreateResource(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("create a new resource", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/resources", map[string]interface{}{
			"cluster_id":     "cluster-001",
			"name":           "gpu-node-a100-new",
			"type":           "GPU",
			"vendor":         "NVIDIA",
			"model":          "A100 80GB",
			"total_gpus":     8,
			"used_gpus":      0,
			"total_memory":   80,
			"status":         "available",
			"node_name":      "node-new",
		})
		AssertStatus(t, w, http.StatusCreated)
		res := ParseJSON[models.Resource](t, w)
		if res.Name != "gpu-node-a100-new" {
			t.Errorf("expected gpu-node-a100-new, got %s", res.Name)
		}
	})
}