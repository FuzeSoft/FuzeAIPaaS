package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/storage"

	"github.com/gin-gonic/gin"
)

func routeModels(h *Handler) *gin.Engine {
	r := newTestRouter(h)
	r.GET("/models", h.GetModels)
	r.POST("/models", h.CreateModel)
	r.GET("/models/:id", h.GetModel)
	r.PUT("/models/:id", h.UpdateModel)
	r.DELETE("/models/:id", h.DeleteModel)
	r.POST("/models/:id/versions", h.CreateModelVersion)
	r.GET("/models/:id/versions", h.GetModelVersions)
	return r
}

func TestModelCRUD(t *testing.T) {
	h, _ := newTestHandler(t)
	r := routeModels(h)

	created := models.Model{
		Name:        "Qwen2",
		Description: "test model",
		Framework:   "pytorch",
	}
	raw, _ := json.Marshal(created)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/models", bytes.NewReader(raw)))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, body = %s", w.Code, w.Body.String())
	}
	var got models.Model
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}
	if got.ID == "" {
		t.Fatal("expected server-assigned model ID")
	}
	id := got.ID

	stored, err := h.modelRepo.GetModel(id)
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if stored.Name != "Qwen2" || stored.Framework != "pytorch" {
		t.Fatalf("unexpected persisted model: %+v", stored)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/models/"+id, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d", w.Code)
	}

	patch := models.Model{Description: "updated"}
	raw, _ = json.Marshal(patch)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/models/"+id, bytes.NewReader(raw)))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", w.Code, w.Body.String())
	}
	stored, _ = h.modelRepo.GetModel(id)
	if stored.Description != "updated" {
		t.Fatalf("PUT did not persist description, got %q", stored.Description)
	}

	ver := models.ModelVersion{Version: "v1", StorageURI: "hf://qwen2"}
	raw, _ = json.Marshal(ver)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/models/"+id+"/versions", bytes.NewReader(raw)))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST version status = %d, body = %s", w.Code, w.Body.String())
	}
	versions, err := h.modelRepo.GetModelVersions(id)
	if err != nil {
		t.Fatalf("GetModelVersions: %v", err)
	}
	if len(versions) != 1 || versions[0].Version != "v1" {
		t.Fatalf("unexpected versions: %+v", versions)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/models", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("LIST status = %d", w.Code)
	}
	var list []models.Model
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("list unmarshal: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least one model in list")
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/models/"+id, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d", w.Code)
	}
	if _, err := h.modelRepo.GetModel(id); err == nil {
		t.Fatal("expected deleted model to be gone")
	}
}

func TestModelCreateInvalidBody(t *testing.T) {
	h, _ := newTestHandler(t)
	r := routeModels(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/models", bytes.NewReader([]byte("{bad}"))))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestModelCRUDAudits(t *testing.T) {
	h, store := newTestHandler(t)
	r := routeModels(h)

	created := models.Model{Name: "Llama3", Framework: "pytorch"}
	raw, _ := json.Marshal(created)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/models", bytes.NewReader(raw)))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST status = %d", w.Code)
	}
	var got models.Model
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}
	createdID := got.ID

	recs, err := store.ListAudit(storage.AuditQuery{Limit: 100})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	var found bool
	for _, rec := range recs {
		if rec.ResourceType == models.ResModel && rec.Action == models.ActionCreate && rec.ResourceID == createdID {
			found = true
		}
	}
	if !found {
		t.Fatal("expected audit record for model create")
	}
}