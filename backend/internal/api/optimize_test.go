package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/domain/optimize"
	"fuze-ai-paas/backend/internal/storage"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

type fakeOptExecutor struct {
	submitted   map[string]bool
	resultReady bool
	result      *optimize.CompressionResult
}

func newFakeOptExecutor() *fakeOptExecutor {
	return &fakeOptExecutor{submitted: map[string]bool{}, resultReady: true}
}
func (f *fakeOptExecutor) Submit(t *optimize.CompressionTask) (string, error) {
	jobID := "job-" + t.ID
	f.submitted[jobID] = true
	return jobID, nil
}
func (f *fakeOptExecutor) Cancel(jobID string) error { return nil }
func (f *fakeOptExecutor) GetResult(jobID string) (*optimize.CompressionResult, error) {
	if !f.resultReady {
		return nil, errors.New("not ready")
	}
	r := *f.result
	r.JobID = jobID
	return &r, nil
}

func newOptimizeTestHandler(t *testing.T) (*Handler, *gin.Engine, *fakeOptExecutor) {
	t.Helper()
	f, err := os.CreateTemp("", "fuze-opt-*.db")
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

	ex := newFakeOptExecutor()
	h := NewHandler(Repos{
		OptimizeRepo:     store.Compression(),
		OptimizeExecutor: ex,
	}, nil, nil, nil, nil)

	r := gin.New()
	r.Use(auth.PassthroughMiddleware())
	r.GET("/api/v1/optimize/tasks", h.ListCompressionTasks)
	r.POST("/api/v1/optimize/tasks", h.CreateCompressionTask)
	r.GET("/api/v1/optimize/tasks/:id", h.GetCompressionTask)
	r.POST("/api/v1/optimize/tasks/:id/cancel", h.CancelCompressionTask)
	r.POST("/api/v1/optimize/tasks/:id/result", h.HandleCompressionResult)
	r.DELETE("/api/v1/optimize/tasks/:id", h.DeleteCompressionTask)
	return h, r, ex
}

func doOptJSON(r *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(body)
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func createTask(t *testing.T, r *gin.Engine) map[string]interface{} {
	w := doOptJSON(r, http.MethodPost, "/api/v1/optimize/tasks", map[string]interface{}{
		"name":             "q8",
		"type":             "quantize",
		"backend":          "pytorch",
		"config":           `{"method":"dynamic","bits":8}`,
		"model_version_id": "mv1",
		"orig_accuracy":    0.95,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d, body=%s", w.Code, w.Body.String())
	}
	var got map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	return got
}

func TestOptimizeAPICreateAndGet(t *testing.T) {
	_, r, _ := newOptimizeTestHandler(t)
	created := createTask(t, r)
	if created["status"] != "running" {
		t.Fatalf("expected running, got %v", created["status"])
	}
	id, _ := created["id"].(string)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/optimize/tasks/"+id, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", rec.Code)
	}
}

func TestOptimizeAPICreateInvalidConfig(t *testing.T) {
	_, r, _ := newOptimizeTestHandler(t)
	w := doOptJSON(r, http.MethodPost, "/api/v1/optimize/tasks", map[string]interface{}{
		"name":             "q",
		"type":             "quantize",
		"backend":          "pytorch",
		"config":           `{"method":"bogus"}`,
		"model_version_id": "mv1",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid config: expected 400, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestOptimizeAPIResultGatePassAndFail(t *testing.T) {
	_, r, ex := newOptimizeTestHandler(t)
	created := createTask(t, r)
	id, _ := created["id"].(string)

	ex.resultReady = true
	ex.result = &optimize.CompressionResult{Accuracy: 0.945, CompressionRatio: 4, Speedup: 4}
	wr := httptest.NewRequest(http.MethodPost, "/api/v1/optimize/tasks/"+id+"/result", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, wr)
	if rec.Code != http.StatusOK {
		t.Fatalf("result pass: expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	created2 := createTask(t, r)
	id2, _ := created2["id"].(string)
	ex.result = &optimize.CompressionResult{Accuracy: 0.90, CompressionRatio: 5, Speedup: 5}
	wr2 := httptest.NewRequest(http.MethodPost, "/api/v1/optimize/tasks/"+id2+"/result", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, wr2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("result fail: expected 200, got %d, body=%s", rec2.Code, rec2.Body.String())
	}
	wg := httptest.NewRequest(http.MethodGet, "/api/v1/optimize/tasks/"+id2, nil)
	rg := httptest.NewRecorder()
	r.ServeHTTP(rg, wg)
	var got map[string]interface{}
	_ = json.Unmarshal(rg.Body.Bytes(), &got)
	if got["status"] != "failed" {
		t.Fatalf("gate fail should set failed, got %v", got["status"])
	}
}

func TestOptimizeAPIResultNotReady(t *testing.T) {
	_, r, ex := newOptimizeTestHandler(t)
	created := createTask(t, r)
	id, _ := created["id"].(string)
	ex.resultReady = false
	wr := httptest.NewRequest(http.MethodPost, "/api/v1/optimize/tasks/"+id+"/result", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, wr)
	if rec.Code != http.StatusConflict {
		t.Fatalf("not-ready result: expected 409, got %d, body=%s", rec.Code, rec.Body.String())
	}
}