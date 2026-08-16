package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agentdomain "fuze-ai-paas/backend/internal/domain/agent"

	"github.com/gin-gonic/gin"
)

func routeTools(h *Handler) *gin.Engine {
	r := newTestRouter(h)
	r.GET("/tools", h.ListTools)
	r.POST("/tools", h.CreateTool)
	r.GET("/tools/:id", h.GetTool)
	r.DELETE("/tools/:id", h.DeleteTool)
	return r
}

func TestToolCRUD(t *testing.T) {
	h, _ := newTestHandler(t)
	r := routeTools(h)

	created := agentdomain.Tool{
		Name:        "weather",
		Description: "fetch weather",
		Kind:        agentdomain.ToolKindHTTP,
		TenantID:    "t1",
		HTTP: &agentdomain.HTTPToolSpec{
			URL:    "https://api.example.com/weather",
			Method: http.MethodGet,
		},
	}
	raw, _ := json.Marshal(created)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tools", bytes.NewReader(raw)))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Tool agentdomain.Tool `json:"tool"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}
	got := resp.Tool
	if got.ID == "" {
		t.Fatal("expected server-assigned tool ID")
	}
	if got.HTTP == nil || got.HTTP.URL != "https://api.example.com/weather" {
		t.Fatalf("unexpected http spec: %+v", got.HTTP)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tools/"+got.ID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d", w.Code)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tools", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("LIST status = %d", w.Code)
	}
	var listResp struct {
		Tools []agentdomain.Tool `json:"tools"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("list unmarshal: %v", err)
	}
	if len(listResp.Tools) == 0 {
		t.Fatal("expected at least one tool")
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/tools/"+got.ID, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d", w.Code)
	}
	if _, err := h.toolRepo.GetByName(context.Background(), "t1", "weather"); err == nil {
		t.Fatal("expected deleted tool to be gone")
	}
}

func TestCreateToolRejectsInvalidURL(t *testing.T) {
	h, _ := newTestHandler(t)
	r := routeTools(h)

	bad := agentdomain.Tool{
		Name:     "bad",
		Kind:     agentdomain.ToolKindHTTP,
		TenantID: "t1",
		HTTP:     &agentdomain.HTTPToolSpec{URL: "ftp://nope", Method: http.MethodGet},
	}
	raw, _ := json.Marshal(bad)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tools", bytes.NewReader(raw)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid scheme, got %d (body %s)", w.Code, w.Body.String())
	}
}

func TestCreateToolRequiresTenantPrincipal(t *testing.T) {
	h, _ := newTestHandler(t)
	
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/tools", h.CreateTool)

	body, _ := json.Marshal(agentdomain.Tool{Name: "x", Kind: agentdomain.ToolKindHTTP})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tools", bytes.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without tenant principal, got %d (body %s)", w.Code, w.Body.String())
	}
}

func TestCreateToolInvalidBody(t *testing.T) {
	h, _ := newTestHandler(t)
	r := routeTools(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tools", bytes.NewReader([]byte("{bad}"))))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}