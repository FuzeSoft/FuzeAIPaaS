package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func routeInference(h *Handler) *gin.Engine {
	r := newTestRouter(h)
	r.POST("/inference-services", h.ApplyInferenceService)
	r.PUT("/inference-services/:name", h.ApplyInferenceService)
	r.PATCH("/inference-services/:name", h.ApplyInferenceService)
	r.GET("/inference-services/:id", h.GetInferenceService)
	r.DELETE("/inference-services/:id", h.DeleteInferenceService)
	return r
}

func TestApplyInferenceServiceCreateAndUpdate(t *testing.T) {
	h, store := newTestHandler(t)
	r := routeInference(h)

	body := inferenceApplyRequest{
		Spec: inferenceSpec{
			Name:           "svc-1",
			Runtime:        "vllm",
			TargetReplicas: 2,
			MinReplicas:    1,
			MaxReplicas:    4,
			GPUs:            1,
		},
	}

	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/inference-services", bytes.NewReader(raw))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, body = %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected server-assigned service id")
	}
	id := created.ID

	svc, err := store.GetInferenceService(id)
	if err != nil {
		t.Fatalf("GetInferenceService: %v", err)
	}
	if svc.TargetReplicas != 2 || svc.Runtime != "vllm" {
		t.Fatalf("unexpected persisted service: %+v", svc)
	}

	body.Spec.TargetReplicas = 3
	raw, _ = json.Marshal(body)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/inference-services/"+id, bytes.NewReader(raw))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", w.Code, w.Body.String())
	}
	svc, err = store.GetInferenceService(id)
	if err != nil {
		t.Fatalf("GetInferenceService after PUT: %v", err)
	}
	if svc.TargetReplicas != 3 {
		t.Fatalf("PUT did not update replicas, got %d", svc.TargetReplicas)
	}
}

func TestDeleteInferenceService(t *testing.T) {
	h, store := newTestHandler(t)
	r := routeInference(h)

	body := inferenceApplyRequest{
		Spec: inferenceSpec{Name: "svc-del", Runtime: "vllm", TargetReplicas: 1},
	}
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/inference-services", bytes.NewReader(raw)))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST status = %d", w.Code)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/inference-services/"+created.ID, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, body = %s", w.Code, w.Body.String())
	}
	if _, err := store.GetInferenceService(created.ID); err == nil {
		t.Fatal("expected deleted service to be gone")
	}
}

func TestApplyInferenceServiceInvalidBody(t *testing.T) {
	h, _ := newTestHandler(t)
	r := routeInference(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/inference-services", bytes.NewReader([]byte("{not json")))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid json, got %d", w.Code)
	}
}