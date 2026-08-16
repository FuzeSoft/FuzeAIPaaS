package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"github.com/gin-gonic/gin"
)

type fakeMountRepo struct {
	items      []*ports.AdapterMount
	lastTenant string
	mountErr   error
}

func (f *fakeMountRepo) Mount(_ context.Context, m *ports.AdapterMount) error {
	if f.mountErr != nil {
		return f.mountErr
	}
	for _, e := range f.items {
		if e.TenantID == m.TenantID && e.ServedName == m.ServedName {
			return ports.ErrMountConflict
		}
	}
	m.ID = "mnt-1"
	m.CreatedAt = 1700000000
	cp := *m
	f.items = append(f.items, &cp)
	return nil
}

func (f *fakeMountRepo) Unmount(_ context.Context, tenantID, adapterID, serviceID string) error {
	f.lastTenant = tenantID
	for i, e := range f.items {
		if e.AdapterID != adapterID || e.ServiceID != serviceID {
			continue
		}
		if tenantID != "" && e.TenantID != tenantID {
			return ports.ErrAdapterNotMounted
		}
		f.items = append(f.items[:i], f.items[i+1:]...)
		return nil
	}
	return ports.ErrAdapterNotMounted
}

func (f *fakeMountRepo) ListByAdapter(_ context.Context, tenantID, adapterID string) ([]*ports.AdapterMount, error) {
	f.lastTenant = tenantID
	out := []*ports.AdapterMount{}
	for _, e := range f.items {
		if tenantID != "" && e.TenantID != tenantID {
			continue
		}
		if e.AdapterID == adapterID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeMountRepo) ListByService(_ context.Context, tenantID, serviceID string) ([]*ports.AdapterMount, error) {
	f.lastTenant = tenantID
	out := []*ports.AdapterMount{}
	for _, e := range f.items {
		if tenantID != "" && e.TenantID != tenantID {
			continue
		}
		if e.ServiceID == serviceID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeMountRepo) ResolveServedName(_ context.Context, tenantID, served string) (*ports.AdapterMount, error) {
	for _, e := range f.items {
		if e.TenantID == tenantID && e.ServedName == served {
			return e, nil
		}
	}
	return nil, ports.ErrAdapterNotMounted
}

var errNoSuchService = errors.New("record not found")

type fakeInferenceReader struct {
	svcs      map[string]*models.InferenceService
	lastQuery [2]string
}

func (f *fakeInferenceReader) GetInferenceServiceForTenant(tenantID, id string) (*models.InferenceService, error) {
	f.lastQuery = [2]string{tenantID, id}
	s, ok := f.svcs[id]
	if !ok {
		return nil, errNoSuchService
	}
	if tenantID != "" && s.TenantID != tenantID {
		
		return nil, errNoSuchService
	}
	return s, nil
}

func newHandlerRouter(h *Handler, claims *auth.Claims) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if claims != nil {
			auth.SetPrincipal(c, claims)
		}
		c.Next()
	})
	r.POST("/adapters", h.CreateFineTuneAdapter)
	r.DELETE("/adapters/:id", h.DeleteFineTuneAdapter)
	r.POST("/adapters/:id/mounts", h.MountFineTuneAdapter)
	r.GET("/adapters/:id/mounts", h.ListFineTuneAdapterMounts)
	r.DELETE("/adapters/:id/mounts/:serviceId", h.UnmountFineTuneAdapter)
	return r
}

func newMountRouter(
	adapters ports.FineTuneRepository,
	mounts ports.AdapterMountRepository,
	infer adapterServiceReader,
	claims *auth.Claims,
) *gin.Engine {
	return newHandlerRouter(&Handler{
		finetune: adapters, adapterMounts: mounts, adapterServices: infer,
	}, claims)
}

func tenantAdminOf(tenant string) *auth.Claims {
	return &auth.Claims{UserID: "admin-" + tenant, Role: models.RoleTenantAdmin, TenantID: tenant}
}

func seedMountFixtures(t *testing.T) (*fakeFineTuneRepo, *fakeMountRepo, *fakeInferenceReader) {
	t.Helper()
	adapters := &fakeFineTuneRepo{items: []*ports.FineTuneAdapter{{
		ID: "ad-1", Name: "sql-expert", BaseModel: "qwen2-7b",
		Path: "s3://a", Rank: 8, Method: ports.MethodLoRA, TenantID: "t1",
	}}}
	infer := &fakeInferenceReader{svcs: map[string]*models.InferenceService{
		"svc-1": {ID: "svc-1", Name: "qwen2-7b", TenantID: "t1"},
		"svc-x": {ID: "svc-x", Name: "llama3-8b", TenantID: "t1"},
		"svc-o": {ID: "svc-o", Name: "qwen2-7b", TenantID: "t2"},
	}}
	return adapters, &fakeMountRepo{}, infer
}

func TestMountAdapterEndpoint(t *testing.T) {
	t.Run("挂载成功并推导对外名", func(t *testing.T) {
		adapters, mounts, infer := seedMountFixtures(t)
		r := newMountRouter(adapters, mounts, infer, tenantAdminOf("t1"))

		w := doRawJSON(r, http.MethodPost, "/adapters/ad-1/mounts", `{"service_id":"svc-1"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
		}

		var got ports.AdapterMount
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("解析响应: %v", err)
		}
		
		if got.ServedName != "qwen2-7b:sql-expert" {
			t.Fatalf("对外名推导错误: %+v", got)
		}
		if got.TenantID != "t1" || got.BaseModel != "qwen2-7b" {
			t.Fatalf("归属或基座字段不对: %+v", got)
		}
	})

	t.Run("基座不匹配必须拒绝", func(t *testing.T) {
		adapters, mounts, infer := seedMountFixtures(t)
		r := newMountRouter(adapters, mounts, infer, tenantAdminOf("t1"))

		w := doRawJSON(r, http.MethodPost, "/adapters/ad-1/mounts", `{"service_id":"svc-x"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("基座不匹配应返回 400，实际 %d: %s", w.Code, w.Body.String())
		}
		if len(mounts.items) != 0 {
			t.Fatal("校验失败不应落库")
		}
	})

	t.Run("他租户的推理服务不可见", func(t *testing.T) {
		adapters, mounts, infer := seedMountFixtures(t)
		r := newMountRouter(adapters, mounts, infer, tenantAdminOf("t1"))

		w := doRawJSON(r, http.MethodPost, "/adapters/ad-1/mounts", `{"service_id":"svc-o"}`)
		
		if w.Code != http.StatusNotFound {
			t.Fatalf("跨租户挂载应返回 404，实际 %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("他租户的适配器不可挂载", func(t *testing.T) {
		adapters, mounts, infer := seedMountFixtures(t)
		r := newMountRouter(adapters, mounts, infer, tenantAdminOf("t2"))

		w := doRawJSON(r, http.MethodPost, "/adapters/ad-1/mounts", `{"service_id":"svc-o"}`)
		if w.Code != http.StatusNotFound {
			t.Fatalf("跨租户适配器应返回 404，实际 %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("对外名冲突返回 409", func(t *testing.T) {
		adapters, mounts, infer := seedMountFixtures(t)
		mounts.mountErr = ports.ErrMountConflict
		r := newMountRouter(adapters, mounts, infer, tenantAdminOf("t1"))

		w := doRawJSON(r, http.MethodPost, "/adapters/ad-1/mounts", `{"service_id":"svc-1"}`)
		
		if w.Code != http.StatusConflict {
			t.Fatalf("重名应返回 409，实际 %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("未接线仓储时降级为 501", func(t *testing.T) {
		r := newMountRouter(&fakeFineTuneRepo{}, nil, nil, tenantAdminOf("t1"))
		w := doRawJSON(r, http.MethodPost, "/adapters/ad-1/mounts", `{"service_id":"svc-1"}`)
		if w.Code != http.StatusNotImplemented {
			t.Fatalf("未接线应返回 501，实际 %d", w.Code)
		}
	})
}

func TestUnmountAndListEndpoints(t *testing.T) {
	adapters, mounts, infer := seedMountFixtures(t)
	r := newMountRouter(adapters, mounts, infer, tenantAdminOf("t1"))

	if w := doRawJSON(r, http.MethodPost, "/adapters/ad-1/mounts", `{"service_id":"svc-1"}`); w.Code != http.StatusOK {
		t.Fatalf("前置挂载失败: %s", w.Body.String())
	}

	t.Run("列出挂载点", func(t *testing.T) {
		w := doRawJSON(r, http.MethodGet, "/adapters/ad-1/mounts", "")
		if w.Code != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", w.Code)
		}
		var body struct {
			Mounts []*ports.AdapterMount `json:"mounts"`
			Total  int                   `json:"total"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析响应: %v", err)
		}
		if body.Total != 1 || len(body.Mounts) != 1 {
			t.Fatalf("挂载列表不对: %+v", body)
		}
		
		if mounts.lastTenant != "t1" {
			t.Fatalf("租户未下推: %q", mounts.lastTenant)
		}
	})

	t.Run("卸载成功", func(t *testing.T) {
		w := doRawJSON(r, http.MethodDelete, "/adapters/ad-1/mounts/svc-1", "")
		if w.Code != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
		}
		if len(mounts.items) != 0 {
			t.Fatal("卸载后挂载应被移除")
		}
	})

	t.Run("卸载不存在的挂载返回 404", func(t *testing.T) {
		w := doRawJSON(r, http.MethodDelete, "/adapters/ad-1/mounts/svc-1", "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("期望 404，实际 %d", w.Code)
		}
	})
}

func TestDeleteMountedAdapterReturnsConflict(t *testing.T) {
	repo := &fakeFineTuneRepo{deleteErr: ports.ErrAdapterMounted}
	r := newFineTuneRouter(repo, tenantAdminOf("t1"))

	w := doRawJSON(r, http.MethodDelete, "/adapters/ad-1", "")
	
	if w.Code != http.StatusConflict {
		t.Fatalf("挂载中删除应返回 409，实际 %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAdapterValidatesSourceJob(t *testing.T) {
	t.Run("来源作业不存在时拒绝且不落库", func(t *testing.T) {
		repo := &fakeFineTuneRepo{}
		h := &Handler{finetune: repo, adapterJobs: &fakeJobChecker{exists: false}}
		r := newHandlerRouter(h, tenantAdminOf("t1"))

		w := doRawJSON(r, http.MethodPost, "/adapters",
			`{"name":"sql","base_model":"qwen2-7b","path":"s3://a","source_job_id":"job-missing"}`)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("非法血缘应返回 400，实际 %d: %s", w.Code, w.Body.String())
		}
		
		if len(repo.items) != 0 {
			t.Fatal("校验失败不应落库")
		}
	})

	t.Run("来源作业存在时放行且带上租户", func(t *testing.T) {
		repo := &fakeFineTuneRepo{}
		chk := &fakeJobChecker{exists: true}
		h := &Handler{finetune: repo, adapterJobs: chk}
		r := newHandlerRouter(h, tenantAdminOf("t1"))

		w := doRawJSON(r, http.MethodPost, "/adapters",
			`{"name":"sql","base_model":"qwen2-7b","path":"s3://a","source_job_id":"job-1"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("合法血缘应放行，实际 %d: %s", w.Code, w.Body.String())
		}
		
		if chk.gotTenant != "t1" || chk.gotJob != "job-1" {
			t.Fatalf("校验未带租户: tenant=%q job=%q", chk.gotTenant, chk.gotJob)
		}
	})

	t.Run("未填写来源作业时不触发校验", func(t *testing.T) {
		repo := &fakeFineTuneRepo{}
		chk := &fakeJobChecker{exists: false}
		h := &Handler{finetune: repo, adapterJobs: chk}
		r := newHandlerRouter(h, tenantAdminOf("t1"))

		w := doRawJSON(r, http.MethodPost, "/adapters",
			`{"name":"sql","base_model":"qwen2-7b","path":"s3://a"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("空血缘应放行，实际 %d: %s", w.Code, w.Body.String())
		}
		if chk.calls != 0 {
			t.Fatalf("空血缘不应触发查询，实际 %d 次", chk.calls)
		}
	})
}

type fakeJobChecker struct {
	exists    bool
	gotTenant string
	gotJob    string
	calls     int
}

func (f *fakeJobChecker) JobExistsForTenant(_ context.Context, tenantID, jobID string) (bool, error) {
	f.calls++
	f.gotTenant, f.gotJob = tenantID, jobID
	return f.exists, nil
}