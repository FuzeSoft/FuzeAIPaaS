package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/k8s"
	"fuze-ai-paas/backend/internal/runtime"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/scheduler"
	"fuze-ai-paas/backend/internal/storage"

	"github.com/gin-gonic/gin"
)

func testTenantMW() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth.SetPrincipal(c, auth.SyntheticAdmin())
		c.Next()
	}
}

func newTestRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(testTenantMW())
	return r
}

func newTestHandler(t *testing.T) (*Handler, *storage.Storage) {
	t.Helper()
	f, err := os.CreateTemp("", "fuze-api-*.db")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	_ = f.Close()
	_ = os.Remove(path)
	db, err := storage.NewSQLiteDBAt(path)
	if err != nil {
		t.Fatalf("NewSQLiteDBAt: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	store := storage.NewStorage(db)
	sched := scheduler.NewScheduler(scheduler.Repos{Cluster: store, Job: store, Inference: store, Dataset: store, Metrics: store}, k8s.NewClusterManager(), runtime.NewDefaultRegistry(), nil)
	authMgr := auth.NewManager()
	return NewHandler(Repos{
		Cluster:   store,
		Job:       store,
		Inference: store,
		Resource:  store,
		Model:     store,
		Dataset:   store,
		Tenant:    store,
		Quota:     store,
		Audit:     store,
		User:      store,
		ToolRegistry: storage.NewToolRepository(db),
	}, sched, k8s.NewClusterManager(), authMgr, nil), store
}

func TestLoginAuditRecorded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, store := newTestHandler(t)
	r := gin.New()
	r.POST("/auth/login", h.Login)
	r.GET("/audit", h.ListAudit)

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "admin123"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d", w.Code)
	}

	logs, err := store.ListAudit(storage.AuditQuery{Actor: "admin", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range logs {
		if l.Action == models.ActionLogin && l.Actor == "admin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected login audit for admin, got %d entries", len(logs))
	}
}

func TestInferenceQuotaBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, store := newTestHandler(t)

	if err := store.UpsertQuota(&models.Quota{TenantID: "default", GPUQuota: 0, MemoryQuotaGB: 0, JobQuota: 0}); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.Use(auth.PassthroughMiddleware())
	r.POST("/inference-services", h.CreateInferenceService)

	body, _ := json.Marshal(map[string]any{
		"spec": map[string]any{
			"name": "q", "framework": "pytorch",
			"image": "v:1", "gpus": 1, "runtime": "kserve",
		},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/inference-services", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 quota block, got %d", w.Code)
	}

	if err := store.UpsertQuota(&models.Quota{TenantID: "default", GPUQuota: 64, MemoryQuotaGB: 1024, JobQuota: 10}); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/inference-services", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 after quota relaxed, got %d", w.Code)
	}
}