package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"github.com/gin-gonic/gin"
)

type fakeClusterRepo struct {
	called bool
}

func (f *fakeClusterRepo) GetClusters() ([]models.Cluster, error) {
	f.called = true
	return []models.Cluster{{ID: "fake-c1", Name: "fake-cluster"}}, nil
}
func (f *fakeClusterRepo) GetCluster(id string) (*models.Cluster, error) { return nil, nil }
func (f *fakeClusterRepo) GetQueues() ([]models.Queue, error)            { return nil, nil }
func (f *fakeClusterRepo) GetResourcesByCluster(clusterID string) ([]models.Resource, error) {
	return nil, nil
}
func (f *fakeClusterRepo) CreateCluster(*models.Cluster) error { return nil }
func (f *fakeClusterRepo) UpdateCluster(*models.Cluster) error { return nil }
func (f *fakeClusterRepo) UpdateClusterStats(id string, stats models.Cluster) error {
	return nil
}
func (f *fakeClusterRepo) DeleteCluster(id string) error { return nil }

type fakeAuditRepo struct{}

func (f *fakeAuditRepo) ListAudit(opts ports.AuditQuery) ([]models.AuditLog, error) {
	return nil, nil
}
func (f *fakeAuditRepo) Record(*models.AuditLog) error { return nil }

func TestHandlerDependsOnPorts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	clusters := &fakeClusterRepo{}
	audit := &fakeAuditRepo{}

	h := NewHandler(Repos{
		Cluster: clusters,
		Audit:   audit,
		
	}, nil, nil, nil, nil)

	r := gin.New()
	r.GET("/api/clusters", h.GetClusters)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/clusters", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/clusters status = %d, body = %s", w.Code, w.Body.String())
	}
	if !clusters.called {
		t.Fatal("expected injected ClusterWriter.GetClusters to be called")
	}

	var got []models.Cluster
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "fake-c1" {
		t.Fatalf("expected fake cluster from injected port, got %+v", got)
	}
}