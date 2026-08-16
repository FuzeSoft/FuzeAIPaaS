package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNewRegistryNoSnapshot(t *testing.T) {
	
	reg := NewRegistry(nil)
	if reg == nil {
		t.Fatalf("expected non-nil registry")
	}
}

func TestHandlerExposesGoRuntimeMetrics(t *testing.T) {
	reg := NewRegistry(nil)
	srv := httptest.NewServer(reg.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get /metrics: %v", err)
	}
	defer resp.Body.Close()
	body := readBody(resp)

	if !strings.Contains(body, "go_goroutines") {
		t.Fatalf("expected go_goroutines metric in output")
	}
	if !strings.Contains(body, "process_resident_memory_bytes") {
		t.Fatalf("expected process_resident_memory_bytes metric")
	}
}

func TestHandlerExposesBusinessMetrics(t *testing.T) {
	snap := func() (BusinessSnapshot, error) {
		return BusinessSnapshot{
			TotalGPUs:      10,
			UsedGPUs:       6,
			RunningJobs:    3,
			PendingJobs:    2,
			InferenceTotal: 5,
			InferenceReady: 4,
			Datasets: []DatasetCache{
				{Name: "imagenet", CachedPercent: 85.5},
			},
			JobsSubmitted: 42,
		}, nil
	}
	reg := NewRegistry(snap)
	srv := httptest.NewServer(reg.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body := readBody(resp)

	checks := []string{
		"fuze_gpu_total",
		"fuze_gpu_used",
		"fuze_jobs",
		"fuze_inference_services_total",
		"fuze_events_jobs_submitted",
		"fuze_dataset_cached_percent",
	}
	for _, m := range checks {
		if !strings.Contains(body, m) {
			t.Fatalf("expected metric %q in output", m)
		}
	}
}

func TestHandlerExposesWorkspaceMetrics(t *testing.T) {
	snap := func() (BusinessSnapshot, error) {
		return BusinessSnapshot{
			WorkspaceTotal:    5,
			WorkspaceRunning:  2,
			WorkspaceByStatus: map[string]int{"running": 2, "stopped": 3},
		}, nil
	}
	reg := NewRegistry(snap)
	srv := httptest.NewServer(reg.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body := readBody(resp)

	for _, m := range []string{
		"fuze_workspaces_total",
		"fuze_workspaces_running",
		"fuze_workspaces{status=\"running\"}",
		"fuze_workspaces{status=\"stopped\"}",
	} {
		if !strings.Contains(body, m) {
			t.Fatalf("expected workspace metric %q in output", m)
		}
	}
}

func TestHandlerSnapshotErrorEmitsZero(t *testing.T) {
	
	snap := func() (BusinessSnapshot, error) {
		return BusinessSnapshot{}, errSnap
	}
	reg := NewRegistry(snap)
	srv := httptest.NewServer(reg.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body := readBody(resp)

	if !strings.Contains(body, "fuze_gpu_total") {
		t.Fatalf("expected fuze_gpu_total even on snapshot error")
	}
}

func TestMiddlewareRecordsRequestMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := NewRegistry(nil)
	r := gin.New()
	r.Use(NewMiddleware(reg))
	r.GET("/api/v1/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	srv := httptest.NewServer(reg.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body := readBody(resp)

	if !strings.Contains(body, "http_requests_total") {
		t.Fatalf("expected http_requests_total metric")
	}
	if !strings.Contains(body, "http_request_duration_seconds") {
		t.Fatalf("expected http_request_duration_seconds metric")
	}
}

func TestMiddlewareSkipsMetricsAndHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := NewRegistry(nil)
	r := gin.New()
	r.Use(NewMiddleware(reg))
	r.GET("/metrics", gin.WrapH(reg.Handler()))
	r.GET("/api/v1/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	for _, path := range []string{"/metrics", "/api/v1/health"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	srv := httptest.NewServer(reg.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body := readBody(resp)

	if strings.Contains(body, `path="/metrics"`) {
		t.Fatalf("/metrics should be skipped by middleware")
	}
}

func TestMiddlewareUsesRouteTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := NewRegistry(nil)
	r := gin.New()
	r.Use(NewMiddleware(reg))
	r.GET("/api/v1/items/:id", func(c *gin.Context) { c.JSON(200, gin.H{"id": c.Param("id")}) })

	for _, id := range []string{"abc", "def", "ghi"} {
		req := httptest.NewRequest("GET", "/api/v1/items/"+id, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	srv := httptest.NewServer(reg.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body := readBody(resp)

	if !strings.Contains(body, `path="/api/v1/items/:id"`) {
		t.Fatalf("expected route template as path label")
	}
}

var errSnap = &snapErr{}

type snapErr struct{}

func (e *snapErr) Error() string { return "snapshot unavailable" }

func readBody(resp *http.Response) string {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return string(buf)
}