package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	workspacek8s "fuze-ai-paas/backend/internal/k8s/workspace"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/storage"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newWorkspaceTestHandler(t *testing.T) *Handler {
	t.Helper()
	f, err := os.CreateTemp("", "fuze-ws-*.db")
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
	store := storage.NewStorage(db)

	if err := store.UpsertQuota(&models.Quota{
		ID:            "default",
		TenantID:      "default",
		GPUQuota:      8,
		MemoryQuotaGB: 64,
		JobQuota:      8,
	}); err != nil {
		t.Fatalf("seed quota: %v", err)
	}

	h := NewHandler(Repos{
		Workspace:   store,
		WorkspaceRT: workspacek8s.NewDriver(nil, store, ""),
		Quota:       store,
	}, nil, nil, nil, nil)
	return h
}

func TestWorkspaceEndpointsLifecycle(t *testing.T) {
	h := newWorkspaceTestHandler(t)
	router := gin.New()
	RegisterRoutes(router, h, nil, SSOConfig{}, false, nil)

	body := map[string]interface{}{
		"name":         "nb-1",
		"kind":         "notebook",
		"owner_id":     "user-1",
		"image":        "registry.example.com/fuze-notebook:latest",
		"gpu":          1,
		"gpu_model":    "nvidia-a100",
		"cpu":          "4",
		"memory":       "16Gi",
		"idle_timeout": 3600,
	}
	w := doJSON(router, http.MethodPost, "/api/v1/workspaces?tenant_id=default", body)
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
	var created workspaceView
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("parse create: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected workspace id, got %+v", created)
	}
	if created.Status != "starting" {
		t.Fatalf("expected status starting, got %s", created.Status)
	}

	w = doJSON(router, http.MethodGet, "/api/v1/workspaces?tenant_id=default", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
	var list struct {
		Items []workspaceView `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("parse list: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(list.Items))
	}

	w = doJSON(router, http.MethodGet, "/api/v1/workspaces/"+created.ID+"?tenant_id=default", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	w = doJSON(router, http.MethodPost, "/api/v1/workspaces/"+created.ID+"/stop?tenant_id=default", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("stop: expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
	var stopped workspaceView
	if err := json.Unmarshal(w.Body.Bytes(), &stopped); err != nil {
		t.Fatalf("parse stop: %v", err)
	}
	if stopped.Status != "stopped" {
		t.Fatalf("expected status stopped, got %s", stopped.Status)
	}

	w = doJSON(router, http.MethodPost, "/api/v1/workspaces/"+created.ID+"/start?tenant_id=default", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("start: expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
	var started workspaceView
	if err := json.Unmarshal(w.Body.Bytes(), &started); err != nil {
		t.Fatalf("parse start: %v", err)
	}
	if started.Status != "starting" {
		t.Fatalf("expected status starting, got %s", started.Status)
	}

	w = doJSON(router, http.MethodPost, "/api/v1/workspaces/"+created.ID+"/activity?tenant_id=default", map[string]interface{}{
		"active": true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("activity: expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	w = doJSON(router, http.MethodDelete, "/api/v1/workspaces/"+created.ID+"?tenant_id=default", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	w = doJSON(router, http.MethodGet, "/api/v1/workspaces/"+created.ID+"?tenant_id=default", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete: expected 404, got %d", w.Code)
	}
}

func TestWorkspaceEndpointsNotFoundCrossTenant(t *testing.T) {
	h := newWorkspaceTestHandler(t)
	router := gin.New()
	RegisterRoutes(router, h, nil, SSOConfig{}, false, nil)

	w := doJSON(router, http.MethodGet, "/api/v1/workspaces/nope?tenant_id=other", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 cross-tenant, got %d", w.Code)
	}
}

func TestWorkspaceEndpointsNotImplementedWhenUnwired(t *testing.T) {
	
	h := NewHandler(Repos{}, nil, nil, nil, nil)
	router := gin.New()
	RegisterRoutes(router, h, nil, SSOConfig{}, false, nil)

	w := doJSON(router, http.MethodGet, "/api/v1/workspaces?tenant_id=default", nil)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 when unwired, got %d", w.Code)
	}
}

func doJSON(router http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}