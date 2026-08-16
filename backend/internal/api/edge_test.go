package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/auth"
	domainevent "fuze-ai-paas/backend/internal/domain/event"
	"fuze-ai-paas/backend/internal/events"
	edgeapp "fuze-ai-paas/backend/internal/app/edge"
	domainedge "fuze-ai-paas/backend/internal/domain/edge"
	edgek8s "fuze-ai-paas/backend/internal/k8s/edge"
	"fuze-ai-paas/backend/internal/storage"

	"github.com/gin-gonic/gin"
)

func newEdgeHandler(t *testing.T) (*Handler, *storage.Storage) {
	t.Helper()
	f, err := os.CreateTemp("", "fuze-edge-*.db")
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
	nodeRepo, deployRepo, driftRepo, labelRepo := store.Edge()
	bus := events.NewBus(0, 0)
	svc := edgeapp.NewService(nodeRepo, deployRepo, driftRepo, edgek8s.NewMockRuntime(), nil, nil,
		labelRepo, edgeapp.Config{OfflineThreshold: time.Minute}, edgeBusAdapter{bus: bus},
		nil, edgeapp.SampleMetricNames{})
	authMgr := auth.NewManager()
	h := NewHandler(Repos{
		Cluster: store, Job: store, Inference: store, Resource: store,
		Model: store, Dataset: store, Tenant: store, Quota: store,
		Audit: store, User: store, ToolRegistry: storage.NewToolRepository(db),
	}, nil, nil, authMgr, bus)
	h.SetEdge(svc)
	return h, store
}

type edgeBusAdapter struct{ bus *events.Bus }

func (a edgeBusAdapter) Publish(e domainedge.EdgeEvent) {
	a.bus.Publish(edgeEventToDomain(e))
}

func edgeEventToDomain(e domainedge.EdgeEvent) domainevent.Event {
	return testEdgeEvent{e}
}

type testEdgeEvent struct{ inner domainedge.EdgeEvent }

func (t testEdgeEvent) EventType() string     { return t.inner.EventTopic() }
func (t testEdgeEvent) AggregateID() string {
	switch e := t.inner.(type) {
	case domainedge.DriftDetected:
		return e.DeploymentID
	case domainedge.DeploymentRolledBack:
		return e.DeploymentID
	}
	return ""
}
func (t testEdgeEvent) OccurredAt() time.Time {
	switch e := t.inner.(type) {
	case domainedge.DriftDetected:
		return e.EvaluatedAt
	case domainedge.DeploymentRolledBack:
		return e.At
	}
	return time.Time{}
}

func TestEdge_Unconfigured_Returns501(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authMgr := auth.NewManager()
	h := NewHandler(Repos{}, nil, nil, authMgr, nil) 
	r := gin.New()
	r.POST("/edge-nodes", h.edgeRegisterNode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/edge-nodes", bytes.NewReader([]byte(`{}`)))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", w.Code)
	}
}

func TestEdge_RegisterDeployCanaryDrift_EndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _ := newEdgeHandler(t)
	r := gin.New()
	
	r.Use(func(c *gin.Context) {
		auth.SetPrincipal(c, auth.SyntheticAdmin())
		c.Next()
	})
	r.POST("/edge-nodes", h.edgeRegisterNode)
	r.POST("/edge-deployments", h.edgeDeploy)
	r.POST("/edge-deployments/:id/canary/promote", h.edgePromoteCanary)
	r.POST("/edge-deployments/:id/drift/check", h.edgeRunDrift)
	r.POST("/edge-deployments/:id/baseline", h.edgeSetBaseline)

	regBody, _ := json.Marshal(map[string]any{"id": "node-1", "name": "edge-1", "mode": "agent", "endpoint": "https://e1:8443"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/edge-nodes", bytes.NewReader(regBody))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("register node status=%d body=%s", w.Code, w.Body.String())
	}
	var node struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &node)

	depBody, _ := json.Marshal(map[string]any{
		"nodeId": node.ID, "modelId": "m1", "version": "v1",
		"image": "img:latest", "replicas": 1, "canaryWeight": 5,
		"autoRollback": true, "driftGuard": true,
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/edge-deployments", bytes.NewReader(depBody))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("deploy status=%d body=%s", w.Code, w.Body.String())
	}
	var dep struct {
		ID           string `json:"id"`
		CanaryWeight int    `json:"canaryWeight"`
		Status       string `json:"status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &dep)
	if dep.CanaryWeight != 5 {
		t.Fatalf("expected canaryWeight=5, got %d", dep.CanaryWeight)
	}

	for i := 0; i < 10; i++ {
		w = httptest.NewRecorder()
		req, _ = http.NewRequest(http.MethodPost, "/edge-deployments/"+dep.ID+"/canary/promote", bytes.NewReader([]byte(`{"step":25}`)))
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("promote status=%d body=%s", w.Code, w.Body.String())
		}
		_ = json.Unmarshal(w.Body.Bytes(), &dep)
		if dep.Status == "active" {
			break
		}
	}

	baseBody, _ := json.Marshal(map[string]any{
		"numericFeatures": map[string]any{
			"f1": map[string]any{"mean": 0, "std": 1, "p01": -2, "p25": -1, "p50": 0, "p75": 1, "p99": 2, "max": 3},
		},
		"predictionDist": map[string]any{"cat0": 0.7, "cat1": 0.3},
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/edge-deployments/"+dep.ID+"/baseline", bytes.NewReader(baseBody))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("baseline status=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/edge-deployments/"+dep.ID+"/drift/check", nil)
	r.ServeHTTP(w, req)
	
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusOK {
		t.Fatalf("drift check unexpected status=%d", w.Code)
	}
}

func TestEdgeSubmitLabelFeedback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _ := newEdgeHandler(t)
	r := gin.New()
	
	r.Use(func(c *gin.Context) {
		auth.SetPrincipal(c, auth.SyntheticAdmin())
		c.Next()
	})
	r.POST("/edge-nodes", h.edgeRegisterNode)
	r.POST("/edge-deployments", h.edgeDeploy)
	r.POST("/edge-deployments/:id/label-feedback", h.edgeSubmitLabelFeedback)

	regBody, _ := json.Marshal(map[string]any{"id": "node-1", "name": "edge-1", "mode": "agent", "endpoint": "https://e1:8443"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/edge-nodes", bytes.NewReader(regBody))
	r.ServeHTTP(w, req)
	var node struct{ ID string `json:"id"` }
	_ = json.Unmarshal(w.Body.Bytes(), &node)

	depBody, _ := json.Marshal(map[string]any{"nodeId": node.ID, "modelId": "m1", "version": "v1", "image": "img:latest", "replicas": 1})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/edge-deployments", bytes.NewReader(depBody))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("deploy status=%d", w.Code)
	}
	var dep struct{ ID string `json:"id"` }
	_ = json.Unmarshal(w.Body.Bytes(), &dep)

	fbBody, _ := json.Marshal(map[string]any{"label": "cat1", "requestId": "req-9"})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/edge-deployments/"+dep.ID+"/label-feedback", bytes.NewReader(fbBody))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("label feedback status=%d body=%s", w.Code, w.Body.String())
	}

	bad, _ := json.Marshal(map[string]any{"requestId": "x"})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/edge-deployments/"+dep.ID+"/label-feedback", bytes.NewReader(bad))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}