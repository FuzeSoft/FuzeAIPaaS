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

func newLimitTestHandler(t *testing.T, maxFails, lockSec int) (*Handler, *storage.Storage) {
	t.Helper()
	f, err := os.CreateTemp("", "fuze-limit-*.db")
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
	authMgr.SetLoginLimit(maxFails, lockSec)
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
	}, sched, k8s.NewClusterManager(), authMgr, nil), store
}

func TestLoginLockedReturns423(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, store := newLimitTestHandler(t, 3, 900)

	hash, _ := auth.HashPassword("secret")
	if _, err := store.UpsertSSOUser(&models.User{
		ID: "u-lock", Username: "locky", Email: "locky@example.com",
		Role: models.RoleDeveloper, TenantID: "default", Password: hash, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	
	if _, err := store.GetUserByUsername("locky"); err != nil {
		t.Fatalf("seed user not found after upsert: %v", err)
	}

	r := gin.New()
	r.POST("/auth/login", h.Login)
	r.GET("/audit", h.ListAudit)

	login := func(pw string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"username": "locky", "password": pw})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	for i := 1; i <= 2; i++ {
		w := login("bad")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i, w.Code)
		}
	}
	
	w := login("bad")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 423 after threshold, got %d body=%s", w.Code, w.Body.String())
	}
	
	w = login("secret")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 423 during lock with correct password, got %d", w.Code)
	}

	logs, err := store.ListAudit(storage.AuditQuery{Actor: "locky", Action: models.ActionLoginFailed, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) < 3 {
		t.Fatalf("expected >=3 login_failed audit logs, got %d", len(logs))
	}
}