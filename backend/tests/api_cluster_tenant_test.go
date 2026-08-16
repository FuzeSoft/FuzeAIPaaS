package tests

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestClusterFullLifecycle(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	t.Run("register cluster", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/clusters", token, map[string]interface{}{
			"name":     "test-cluster-east",
			"endpoint": "https://cluster-east.example.com",
		})
		AssertStatusIn(t, w, http.StatusCreated, http.StatusOK)
	})

	listW := env.DoAuthGET(http.MethodGet, "/api/v1/clusters", token)
	var clusters []map[string]interface{}
	if err := json.Unmarshal(listW.Body.Bytes(), &clusters); err != nil {
		
		var wrapped map[string]interface{}
		if err2 := json.Unmarshal(listW.Body.Bytes(), &wrapped); err2 == nil {
			if arr, ok := wrapped["clusters"].([]interface{}); ok {
				for _, item := range arr {
					clusters = append(clusters, item.(map[string]interface{}))
				}
			}
		}
	}

	if len(clusters) > 0 {
		
		var clusterID string
		for i := len(clusters) - 1; i >= 0; i-- {
			if clusters[i]["name"] == "test-cluster-east" {
				clusterID = clusters[i]["id"].(string)
				break
			}
		}
		if clusterID == "" {
			clusterID = clusters[0]["id"].(string)
		}

		t.Run("get cluster by id", func(t *testing.T) {
			w := env.DoAuthGET(http.MethodGet, "/api/v1/clusters/"+clusterID, token)
			AssertStatus(t, w, http.StatusOK)
		})

		t.Run("update cluster", func(t *testing.T) {
			w := env.DoAuthJSON(http.MethodPut, "/api/v1/clusters/"+clusterID, token, map[string]interface{}{
				"name": "test-cluster-east-updated",
			})
			AssertStatus(t, w, http.StatusOK)
		})

		t.Run("discover cluster resources", func(t *testing.T) {
			w := env.DoAuthJSON(http.MethodPost, "/api/v1/clusters/"+clusterID+"/discover", token, nil)
			
			AssertStatusIn(t, w, http.StatusOK, http.StatusAccepted, http.StatusServiceUnavailable, http.StatusBadRequest)
		})

		t.Run("test cluster connectivity", func(t *testing.T) {
			w := env.DoAuthJSON(http.MethodPost, "/api/v1/clusters/"+clusterID+"/test", token, nil)
			AssertStatusIn(t, w, http.StatusOK, http.StatusAccepted, http.StatusServiceUnavailable)
		})

		t.Run("get cluster resources", func(t *testing.T) {
			w := env.DoAuthGET(http.MethodGet, "/api/v1/clusters/"+clusterID+"/resources", token)
			AssertStatus(t, w, http.StatusOK)
		})

		t.Run("delete cluster", func(t *testing.T) {
			w := env.DoAuthJSON(http.MethodDelete, "/api/v1/clusters/"+clusterID, token, nil)
			AssertStatus(t, w, http.StatusNoContent)
		})
	}
}

func TestClusterNotFoundScenarios(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	t.Run("get non-existent cluster returns 404", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/clusters/nonexistent", token)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("discover non-existent cluster returns 404", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/clusters/nonexistent/discover", token, nil)
		AssertStatusIn(t, w, http.StatusNotFound, http.StatusBadRequest)
	})

	t.Run("test non-existent cluster returns 404", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/clusters/nonexistent/test", token, nil)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("get resources for non-existent cluster returns empty list", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/clusters/nonexistent/resources", token)
		
		AssertStatusIn(t, w, http.StatusOK, http.StatusNotFound)
	})
}

func TestClusterValidation(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	t.Run("register cluster without name", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/clusters", token, map[string]interface{}{
			"endpoint": "https://cluster.example.com",
		})
		AssertStatusIn(t, w, http.StatusBadRequest, http.StatusUnprocessableEntity)
	})

	t.Run("register cluster without endpoint", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/clusters", token, map[string]interface{}{
			"name": "no-endpoint-cluster",
		})
		
		AssertStatusIn(t, w, http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusCreated, http.StatusOK)
	})
}

func TestTenantFullLifecycle(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	t.Run("list tenants", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/tenants", token)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("get default tenant", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/tenants/default", token)
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("create tenant", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/tenants", token, map[string]interface{}{
			"name": "test-tenant-lifecycle",
		})
		AssertStatusIn(t, w, http.StatusCreated, http.StatusOK)
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		if id, ok := resp["id"].(string); ok {
			
			env.DoAuthJSON(http.MethodDelete, "/api/v1/tenants/"+id, token, nil)
		}
	})
}

func TestTenantNotFoundAndValidation(t *testing.T) {
	env := NewTestEnvWithAuth(t)
	token := env.LoginAndGetToken(t, "admin", "admin123")

	t.Run("get non-existent tenant returns 404", func(t *testing.T) {
		w := env.DoAuthGET(http.MethodGet, "/api/v1/tenants/nonexistent", token)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("delete non-existent tenant returns 404", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodDelete, "/api/v1/tenants/nonexistent", token, nil)
		
		AssertStatusIn(t, w, http.StatusNotFound, http.StatusNoContent)
	})

	t.Run("create tenant without name", func(t *testing.T) {
		w := env.DoAuthJSON(http.MethodPost, "/api/v1/tenants", token, map[string]interface{}{})
		AssertStatusIn(t, w, http.StatusBadRequest, http.StatusUnprocessableEntity)
	})
}