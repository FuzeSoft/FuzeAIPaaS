package tests

import (
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestListTenants(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("list all tenants", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/tenants")
		AssertStatus(t, w, http.StatusOK)
		tenants := ParseJSON[[]models.Tenant](t, w)
		if len(tenants) < 1 {
			t.Errorf("expected at least 1 tenant, got %d", len(tenants))
		}
	})
}

func TestGetTenant(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("get existing tenant", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/tenants/default")
		AssertStatus(t, w, http.StatusOK)
		tenant := ParseJSON[models.Tenant](t, w)
		if tenant.Name != "默认租户" {
			t.Errorf("expected 默认租户, got %s", tenant.Name)
		}
	})

	t.Run("get non-existent tenant returns 404", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/tenants/nonexistent")
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestCreateTenant(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("create a new tenant", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/tenants", map[string]interface{}{
			"name":        "test-tenant",
			"description": "Test tenant",
		})
		AssertStatus(t, w, http.StatusCreated)
		tenant := ParseJSON[models.Tenant](t, w)
		if tenant.Name != "test-tenant" {
			t.Errorf("expected test-tenant, got %s", tenant.Name)
		}
	})

	t.Run("create tenant without name returns 400", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/tenants", map[string]interface{}{
			"description": "No name tenant",
		})
		AssertStatus(t, w, http.StatusBadRequest)
	})
}

func TestUpdateTenant(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("update existing tenant", func(t *testing.T) {
		w := env.DoJSON(http.MethodPut, "/api/v1/tenants/default", map[string]interface{}{
			"name":        "updated-default",
			"description": "Updated default tenant",
		})
		AssertStatus(t, w, http.StatusOK)
	})

	t.Run("update non-existent tenant returns 404", func(t *testing.T) {
		w := env.DoJSON(http.MethodPut, "/api/v1/tenants/nonexistent", map[string]interface{}{
			"name": "test",
		})
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestDeleteTenant(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("delete non-default tenant", func(t *testing.T) {
		
		w := env.DoJSON(http.MethodPost, "/api/v1/tenants", map[string]interface{}{
			"name": "tenant-to-delete",
		})
		AssertStatus(t, w, http.StatusCreated)
		tenant := ParseJSON[models.Tenant](t, w)

		w = env.DoJSON(http.MethodDelete, "/api/v1/tenants/"+tenant.ID, nil)
		AssertStatus(t, w, http.StatusNoContent)

		w = env.DoGET(http.MethodGet, "/api/v1/tenants/"+tenant.ID)
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("cannot delete default tenant", func(t *testing.T) {
		w := env.DoJSON(http.MethodDelete, "/api/v1/tenants/default", nil)
		AssertStatus(t, w, http.StatusBadRequest)
	})
}