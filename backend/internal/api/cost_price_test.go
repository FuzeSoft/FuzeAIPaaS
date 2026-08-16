package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/storage"

	"github.com/gin-gonic/gin"
)

func newCostPriceHandler(t *testing.T) (*Handler, *storage.Storage, *auth.Manager) {
	t.Helper()
	f, err := os.CreateTemp("", "fuze-cost-*.db")
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
	authMgr := auth.NewManager()
	h := NewHandler(Repos{
		Quota:  store,
		Tenant: store,
	}, nil, nil, authMgr, nil)
	h.priceRepo = storage.NewPriceRepository(db)
	h.costRepo = storage.NewCostRepository(db)
	return h, store, authMgr
}

func costPriceRouter(h *Handler, authMgr *auth.Manager) http.Handler {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	protected := r.Group("/api/v1")
	protected.Use(auth.PassthroughMiddleware())
	priceAdmin := authMgr.RequireRole(models.RolePlatformAdmin, models.RoleTenantAdmin)
	protected.GET("/llm/prices/llm", priceAdmin, h.ListLLMPrices)
	protected.PUT("/llm/prices/llm", priceAdmin, h.UpsertLLMPrice)
	protected.DELETE("/llm/prices/llm/:model", priceAdmin, h.DeleteLLMPrice)
	protected.GET("/llm/prices/gpu", priceAdmin, h.ListGPUPrices)
	protected.PUT("/llm/prices/gpu", priceAdmin, h.UpsertGPUPrice)
	protected.DELETE("/llm/prices/gpu/:gpuType", priceAdmin, h.DeleteGPUPrice)
	protected.GET("/cost/summary", h.CostSummary)
	return r
}

type priceListEnvelope struct {
	Prices []models.LLMPrice `json:"prices"`
}

func TestPriceCRUDLLMAndGPU(t *testing.T) {
	h, _, authMgr := newCostPriceHandler(t)
	r := costPriceRouter(h, authMgr)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/llm/prices/llm", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list llm prices: want 200 got %d", w.Code)
	}
	var env priceListEnvelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if len(env.Prices) != 0 {
		t.Fatalf("expected empty llm prices, got %d", len(env.Prices))
	}

	body, _ := json.Marshal(models.LLMPrice{
		Model:       "gpt-4o",
		InputPer1K:  0.005,
		OutputPer1K: 0.015,
		Currency:    "USD",
	})
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/llm/prices/llm", bytes.NewReader(body))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("upsert llm price: want 200/201 got %d (%s)", w.Code, w.Body.String())
	}

	body, _ = json.Marshal(models.GPUPrice{
		GPUType:         "A100",
		PricePerGPUHour: 2.5,
		Currency:        "USD",
	})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/llm/prices/gpu", bytes.NewReader(body))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("upsert gpu price: want 200/201 got %d (%s)", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/llm/prices/llm", nil))
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	list := env.Prices
	if len(list) != 1 || list[0].Model != "gpt-4o" {
		t.Fatalf("expected 1 llm price gpt-4o, got %+v", list)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/llm/prices/gpu", nil))
	var genv struct {
		Prices []models.GPUPrice `json:"prices"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &genv)
	glist := genv.Prices
	if len(glist) != 1 || glist[0].GPUType != "A100" {
		t.Fatalf("expected 1 gpu price A100, got %+v", glist)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/llm/prices/llm/gpt-4o", nil))
	if w.Code != http.StatusOK && w.Code != http.StatusNoContent {
		t.Fatalf("delete llm price: want 200/204 got %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/llm/prices/llm", nil))
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if len(env.Prices) != 0 {
		t.Fatalf("expected 0 llm prices after delete, got %d", len(env.Prices))
	}
}

func TestCostSummaryEmptyAndNilSafe(t *testing.T) {
	h, _, authMgr := newCostPriceHandler(t)
	r := costPriceRouter(h, authMgr)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/cost/summary", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("cost summary: want 200 got %d (%s)", w.Code, w.Body.String())
	}
	var sum CostSummary
	if err := json.Unmarshal(w.Body.Bytes(), &sum); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if sum.LLMCost != 0 || sum.GPUCost != 0 {
		t.Fatalf("expected zero costs, got llm=%v gpu=%v", sum.LLMCost, sum.GPUCost)
	}

	hNil := NewHandler(Repos{}, nil, nil, auth.NewManager(), nil)
	hNil.priceRepo = nil
	hNil.costRepo = nil
	r2 := gin.New()
	r2g := r2.Group("/api/v1")
	r2g.Use(auth.PassthroughMiddleware())
	r2g.GET("/cost/summary", hNil.CostSummary)
	w = httptest.NewRecorder()
	r2.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/cost/summary", nil))
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("nil-safe cost summary: want 501 got %d (%s)", w.Code, w.Body.String())
	}
}