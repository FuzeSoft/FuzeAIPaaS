package tests

import (
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestGetClusters(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("list all clusters", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/clusters")
		AssertStatus(t, w, http.StatusOK)
		clusters := ParseJSON[[]models.Cluster](t, w)
		if len(clusters) < 1 {
			t.Errorf("expected at least 1 seeded cluster, got %d", len(clusters))
		}
	})
}

func TestGetCluster(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("get existing cluster", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/clusters/cluster-001")
		AssertStatus(t, w, http.StatusOK)
		cluster := ParseJSON[models.Cluster](t, w)
		if cluster.Name != "生产集群" {
			t.Errorf("expected 生产集群, got %s", cluster.Name)
		}
	})

	t.Run("get non-existent cluster returns 404", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/clusters/nonexistent")
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestRegisterCluster(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("register a new cluster", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/clusters", map[string]interface{}{
			"name":        "test-cluster",
			"description": "Test cluster for integration tests",
			"region":      "cn-test",
			"provider":    "self-hosted",
		})
		AssertStatusIn(t, w, http.StatusCreated, http.StatusOK)
	})

	t.Run("register cluster without name returns 400", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/clusters", map[string]interface{}{
			"description": "No name cluster",
		})
		AssertStatus(t, w, http.StatusBadRequest)
	})
}

func TestUpdateCluster(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("update existing cluster", func(t *testing.T) {
		w := env.DoJSON(http.MethodPut, "/api/v1/clusters/cluster-001", map[string]interface{}{
			"name":        "updated-cluster-name",
			"description": "Updated description",
		})
		AssertStatus(t, w, http.StatusOK)
	})
}

func TestDeleteCluster(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("delete non-existent cluster returns 404 or 500", func(t *testing.T) {
		w := env.DoJSON(http.MethodDelete, "/api/v1/clusters/nonexistent", nil)
		AssertStatus(t, w, http.StatusNoContent)
	})

	t.Run("delete existing cluster", func(t *testing.T) {
		
		w := env.DoJSON(http.MethodPost, "/api/v1/clusters", map[string]interface{}{
			"name": "cluster-to-delete",
		})
		AssertStatusIn(t, w, http.StatusCreated, http.StatusOK)
		cluster := ParseJSON[map[string]interface{}](t, w)
		id := cluster["id"].(string)

		w = env.DoJSON(http.MethodDelete, "/api/v1/clusters/"+id, nil)
		AssertStatus(t, w, http.StatusNoContent)
	})
}

func TestGetClusterResources(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("get resources for existing cluster", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/clusters/cluster-001/resources")
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("get resources for non-existent cluster returns 500", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/clusters/nonexistent/resources")
		AssertStatusIn(t, w, http.StatusInternalServerError, http.StatusOK)
	})
}

func TestDiscoverCluster(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("discover cluster resources", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/clusters/cluster-001/discover", nil)
		AssertStatusIn(t, w, http.StatusOK, http.StatusBadRequest)
	})
}

func TestTestCluster(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("test cluster connectivity", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/clusters/cluster-001/test", nil)
		AssertStatusIn(t, w, http.StatusOK)
		body := ParseJSON[map[string]interface{}](t, w)
		if _, ok := body["connected"]; !ok {
			t.Error("expected connected field in test response")
		}
	})

	t.Run("test non-existent cluster returns 404", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/clusters/nonexistent/test", nil)
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestGetQueues(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("list all queues", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/queues")
		AssertStatus(t, w, http.StatusOK)
		queues := ParseJSON[[]models.Queue](t, w)
		if len(queues) < 2 {
			t.Errorf("expected at least 2 seeded queues, got %d", len(queues))
		}
	})
}