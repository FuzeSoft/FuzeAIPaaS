package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/storage"

	"github.com/gin-gonic/gin"
)

func newAlertsTestHandler(t *testing.T) *Handler {
	t.Helper()
	f, err := os.CreateTemp("", "fuze-alerts-*.db")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	_ = f.Close()
	_ = os.Remove(path)
	db, err := storage.NewSQLiteDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	return NewHandler(Repos{Alert: storage.NewAlertRepository(db)}, nil, nil, nil, nil)
}

func withTenant(tenantID, userID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth.SetPrincipal(c, &auth.Claims{
			UserID:   userID,
			TenantID: tenantID,
			Role:     models.RoleTenantAdmin,
		})
		c.Next()
	}
}

func doAlertsJSON(t *testing.T, h *Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		raw, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	router := gin.New()
	router.Use(withTenant("t1", "u1"))
	router.POST("/api/v1/alerts/rules", h.CreateAlertRule)
	router.GET("/api/v1/alerts/rules", h.ListAlertRules)
	router.DELETE("/api/v1/alerts/rules/:id", h.DeleteAlertRule)
	router.POST("/api/v1/alerts/silences", h.CreateSilence)
	router.GET("/api/v1/alerts/silences", h.ListSilences)
	router.GET("/api/v1/alerts/active", h.ListActiveAlerts)
	router.ServeHTTP(w, r)
	return w
}

func TestAlertsEndpointsCRUD(t *testing.T) {
	h := newAlertsTestHandler(t)

	create := doAlertsJSON(t, h, http.MethodPost, "/api/v1/alerts/rules", map[string]interface{}{
		"name": "gpu-idle", "expr": "avg(fuze_gpu_utilization_percent) < 5", "for": "30m",
		"severity": "info", "enabled": true,
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("创建规则应 201，got %d: %s", create.Code, create.Body.String())
	}
	var created models.AlertRule
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID != "t1:gpu-idle" {
		t.Fatalf("ID 应由租户+名称派生，got %q", created.ID)
	}

	list := doAlertsJSON(t, h, http.MethodGet, "/api/v1/alerts/rules", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("列出规则应 200，got %d", list.Code)
	}

	del := doAlertsJSON(t, h, http.MethodDelete, "/api/v1/alerts/rules/"+created.ID, nil)
	if del.Code != http.StatusNoContent {
		t.Fatalf("删除规则应 204，got %d", del.Code)
	}

	active := doAlertsJSON(t, h, http.MethodGet, "/api/v1/alerts/active", nil)
	if active.Code != http.StatusNotImplemented {
		t.Fatalf("无 metrics 客户端时活跃告警应 501，got %d", active.Code)
	}
}

func TestAlertsEndpointsInvalidBody(t *testing.T) {
	h := newAlertsTestHandler(t)
	
	bad := doAlertsJSON(t, h, http.MethodPost, "/api/v1/alerts/rules", map[string]interface{}{"name": "x"})
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("缺 expr 应 400，got %d", bad.Code)
	}
}