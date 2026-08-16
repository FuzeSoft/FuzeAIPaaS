package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/domain/agent"
	"fuze-ai-paas/backend/internal/k8s"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/runtime"
	"fuze-ai-paas/backend/internal/scheduler"
	"fuze-ai-paas/backend/internal/storage"
	"github.com/gin-gonic/gin"
)

func newAgentHandler(t *testing.T) (*Handler, *storage.Storage) {
	t.Helper()
	f, err := os.CreateTemp("", "fuze-agent-*.db")
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
		Agent:        storage.NewAgentRepository(db),
		ToolRegistry: storage.NewToolRepository(db),
	}, sched, k8s.NewClusterManager(), authMgr, nil), store
}

func sampleDAG() agent.DAG {
	return agent.DAG{
		Nodes: []agent.Node{
			{ID: "n1", Type: agent.NodeLLMCall, Ref: "gpt", Config: map[string]any{"prompt": "hi"}},
			{ID: "n2", Type: agent.NodeLLMCall, Ref: "gpt2", Config: map[string]any{"prompt": "after {n1}"}},
		},
		Edges: map[string][]string{"n1": {"n2"}},
	}
}

func TestAgent_Unconfigured_Returns501(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _ := newTestHandler(t) 
	r := gin.New()
	r.GET("/agents", h.ListAgents)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/agents", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", w.Code)
	}
}

func TestAgent_CreateCompileGet_EndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _ := newAgentHandler(t)
	r := newTestRouter(h)
	r.POST("/agents", h.CreateAgent)
	r.POST("/agents/:id/compile", h.CompileAgent)
	r.GET("/agents/:id", h.GetAgent)
	r.GET("/agents", h.ListAgents)

	body, _ := json.Marshal(map[string]any{
		"id":   "ag-demo",
		"name": "demo",
		"dag":  sampleDAG(),
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/agents", bytes.NewReader(body))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/agents/ag-demo/compile", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("compile status = %d body=%s", w.Code, w.Body.String())
	}
	var comp compileAgentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &comp); err != nil {
		t.Fatal(err)
	}
	if comp.Status != agent.AgentStatusCompiled {
		t.Fatalf("status = %s", comp.Status)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/agents/ag-demo", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d", w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/agents", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}
}

func TestAgent_CreateAndDelete_Audited(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, store := newAgentHandler(t)
	r := newTestRouter(h)
	r.POST("/agents", h.CreateAgent)
	r.DELETE("/agents/:id", h.DeleteAgent)

	body, _ := json.Marshal(map[string]any{"id": "ag-audit", "name": "audit", "dag": sampleDAG()})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/agents", bytes.NewReader(body))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", w.Code, w.Body.String())
	}
	
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodDelete, "/agents/ag-audit", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", w.Code, w.Body.String())
	}

	logs, err := store.ListAudit(storage.AuditQuery{})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	var sawCreate, sawDelete bool
	for _, l := range logs {
		if l.ResourceType == models.ResAgent && l.ResourceID == "ag-audit" {
			if l.Action == models.ActionCreate {
				sawCreate = true
			}
			if l.Action == models.ActionDelete {
				sawDelete = true
			}
		}
	}
	if !sawCreate {
		t.Fatalf("expected audit log for agent create; got %d logs: %+v", len(logs), logs)
	}
	if !sawDelete {
		t.Fatalf("expected audit log for agent delete; got %d logs: %+v", len(logs), logs)
	}
}

func TestDataset_CreateAndDelete_Audited(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, store := newAgentHandler(t)
	r := newTestRouter(h)
	r.POST("/datasets", h.CreateDataset)
	r.DELETE("/datasets/:id", h.DeleteDataset)

	body, _ := json.Marshal(map[string]any{
		"id":         "ds-audit",
		"name":       "audit-ds",
		"mountPoint": "/data/audit",
		"quota":      "10Gi",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/datasets", bytes.NewReader(body))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create dataset status = %d body=%s", w.Code, w.Body.String())
	}
	var createdDs struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &createdDs); err != nil {
		t.Fatalf("unmarshal created dataset: %v", err)
	}
	if createdDs.ID == "" {
		t.Fatalf("created dataset has empty id; body=%s", w.Body.String())
	}
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodDelete, "/datasets/"+createdDs.ID, nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete dataset status = %d body=%s", w.Code, w.Body.String())
	}

	logs, err := store.ListAudit(storage.AuditQuery{})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	var sawCreate, sawDelete bool
	for _, l := range logs {
		if l.ResourceType == "dataset" && l.ResourceID == createdDs.ID {
			if l.Action == models.ActionCreate {
				sawCreate = true
			}
			if l.Action == models.ActionDelete {
				sawDelete = true
			}
		}
	}
	if !sawCreate {
		t.Fatalf("expected audit log for dataset create; got %d logs: %+v", len(logs), logs)
	}
	if !sawDelete {
		t.Fatalf("expected audit log for dataset delete; got %d logs: %+v", len(logs), logs)
	}
}

func TestAgent_StartRun_LLMNilSafeFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _ := newAgentHandler(t)
	r := newTestRouter(h)
	r.POST("/agents", h.CreateAgent)
	r.POST("/agents/:id/compile", h.CompileAgent)
	r.POST("/agents/:id/runs", h.StartRun)

	body, _ := json.Marshal(map[string]any{"id": "ag1", "name": "d", "dag": sampleDAG()})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/agents", bytes.NewReader(body))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d", w.Code)
	}
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/agents/ag1/compile", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("compile = %d", w.Code)
	}
	
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/agents/ag1/runs", bytes.NewReader([]byte(`{"input":"x"}`)))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("start run = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Run agent.Run `json:"run"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Run.Status != agent.RunFailed {
		t.Fatalf("run status = %s (expected failed due to nil LLM)", resp.Run.Status)
	}
}