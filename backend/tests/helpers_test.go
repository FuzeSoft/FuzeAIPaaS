package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"fuze-ai-paas/backend/internal/api"
	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/events"
	"fuze-ai-paas/backend/internal/k8s"
	"fuze-ai-paas/backend/internal/runtime"
	"fuze-ai-paas/backend/internal/metricsquery"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/scheduler"
	"fuze-ai-paas/backend/internal/storage"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type TestEnv struct {
	Handler    *api.Handler
	Store      *storage.Storage
	Scheduler  *scheduler.Scheduler
	Router     *gin.Engine
	ClusterMgr k8s.ClusterRegistry
	AuthMgr    *auth.Manager
	DBPath     string
}

func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	f, err := os.CreateTemp("", "fuze-test-*.db")
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
	clusterMgr := k8s.NewClusterManager()
	authMgr := auth.NewManager()
	
	authMgr.SetRevokedCheck(func(jti string) bool {
		ok, _ := store.Token().IsBlacklisted(context.Background(), jti)
		return ok
	})

	sched := scheduler.NewScheduler(scheduler.Repos{
		Cluster:   store,
		Job:       store,
		Inference: store,
		Dataset:   store,
		Metrics:   store,
	}, clusterMgr, runtime.NewDefaultRegistry(), nil)

	h := api.NewHandler(api.Repos{
		Cluster:      store,
		Job:          store,
		Inference:    store,
		Resource:     store,
		Model:        store,
		Dataset:      store,
		Tenant:       store,
		Quota:        store,
		Audit:        store,
		User:         store,
		Experiment:   store.Experiment(),
		MetricsQuery: metricsquery.NewNoop(),
		Token:        store.Token(),
		Evaluation:   store.Evaluation(),
		Guardrail:    storage.NewGuardrailRepository(db),
	}, sched, clusterMgr, authMgr, events.NewBus(1024, 8))

	router := gin.New()
	api.RegisterRoutes(router, h, authMgr, api.SSOConfig{}, false, clusterMgr)

	return &TestEnv{
		Handler:    h,
		Store:      store,
		Scheduler:  sched,
		Router:     router,
		ClusterMgr: clusterMgr,
		AuthMgr:    authMgr,
		DBPath:     path,
	}
}

func NewTestEnvWithAuth(t *testing.T) *TestEnv {
	t.Helper()

	env := NewTestEnv(t)
	router := gin.New()
	api.RegisterRoutes(router, env.Handler, env.AuthMgr, api.SSOConfig{}, true, env.ClusterMgr)
	env.Router = router
	return env
}

func (env *TestEnv) DoJSON(method, path string, body interface{}) *httptest.ResponseRecorder {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	env.Router.ServeHTTP(w, req)
	return w
}

func (env *TestEnv) DoGET(method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, nil)
	env.Router.ServeHTTP(w, req)
	return w
}

func ParseJSON[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v, body: %s", err, w.Body.String())
	}
	return result
}

func AssertStatus(t *testing.T, w *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if w.Code != expected {
		t.Fatalf("expected status %d, got %d, body: %s", expected, w.Code, w.Body.String())
	}
}

func AssertStatusIn(t *testing.T, w *httptest.ResponseRecorder, expected ...int) {
	t.Helper()
	for _, code := range expected {
		if w.Code == code {
			return
		}
	}
	t.Fatalf("expected status in %v, got %d, body: %s", expected, w.Code, w.Body.String())
}

func AssertError(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Fatalf("expected error field in response, got: %s", w.Body.String())
	}
}

func (env *TestEnv) LoginAndGetToken(t *testing.T, username, password string) string {
	t.Helper()
	w := env.DoJSON(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	AssertStatus(t, w, http.StatusOK)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	token, ok := resp["token"].(string)
	if !ok || token == "" {
		t.Fatalf("expected token in login response, got: %v", resp)
	}
	return token
}

func (env *TestEnv) DoAuthJSON(method, path, token string, body interface{}) *httptest.ResponseRecorder {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	env.Router.ServeHTTP(w, req)
	return w
}

func (env *TestEnv) DoAuthGET(method, path, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	env.Router.ServeHTTP(w, req)
	return w
}

func (env *TestEnv) EnsureDefaultQuota(t *testing.T, gpus, memGB, jobs int) {
	t.Helper()
	err := env.Store.UpsertQuota(&models.Quota{
		ID:            "default",
		TenantID:      "default",
		GPUQuota:      gpus,
		MemoryQuotaGB: memGB,
		JobQuota:      jobs,
	})
	if err != nil {
		t.Fatalf("failed to set default quota: %v", err)
	}
}