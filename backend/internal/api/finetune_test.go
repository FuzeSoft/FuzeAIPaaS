package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"github.com/gin-gonic/gin"
)

type fakeFineTuneRepo struct {
	items []*ports.FineTuneAdapter
	
	lastTenant string
	lastFilter ports.FineTuneFilter
	createErr  error
	
	deleteErr error
}

func (f *fakeFineTuneRepo) Create(_ context.Context, a *ports.FineTuneAdapter) error {
	if f.createErr != nil {
		return f.createErr
	}
	a.ID = "ad-1"
	a.CreatedAt = 1700000000
	cp := *a
	f.items = append(f.items, &cp)
	return nil
}

func (f *fakeFineTuneRepo) Get(_ context.Context, tenantID, id string) (*ports.FineTuneAdapter, error) {
	f.lastTenant = tenantID
	for _, a := range f.items {
		if a.ID != id {
			continue
		}
		if tenantID != "" && a.TenantID != tenantID {
			return nil, ports.ErrAdapterNotFound
		}
		return a, nil
	}
	return nil, ports.ErrAdapterNotFound
}

func (f *fakeFineTuneRepo) List(_ context.Context, filter ports.FineTuneFilter) ([]*ports.FineTuneAdapter, error) {
	f.lastFilter = filter
	out := []*ports.FineTuneAdapter{}
	for _, a := range f.items {
		if filter.TenantID != "" && a.TenantID != filter.TenantID {
			continue
		}
		if filter.BaseModel != "" && a.BaseModel != filter.BaseModel {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

func (f *fakeFineTuneRepo) Delete(_ context.Context, tenantID, id string) error {
	f.lastTenant = tenantID
	if f.deleteErr != nil {
		return f.deleteErr
	}
	for i, a := range f.items {
		if a.ID != id {
			continue
		}
		if tenantID != "" && a.TenantID != tenantID {
			return ports.ErrAdapterNotFound
		}
		f.items = append(f.items[:i], f.items[i+1:]...)
		return nil
	}
	return ports.ErrAdapterNotFound
}

func newFineTuneRouter(repo ports.FineTuneRepository, claims *auth.Claims) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &Handler{finetune: repo}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		if claims != nil {
			auth.SetPrincipal(c, claims)
		}
		c.Next()
	})
	r.POST("/adapters", h.CreateFineTuneAdapter)
	r.GET("/adapters", h.ListFineTuneAdapters)
	r.GET("/adapters/:id", h.GetFineTuneAdapter)
	r.DELETE("/adapters/:id", h.DeleteFineTuneAdapter)
	return r
}

func memberOf(tenant string) *auth.Claims {
	return &auth.Claims{UserID: "u-" + tenant, Role: models.RoleDeveloper, TenantID: tenant}
}

func platformAdmin() *auth.Claims {
	return &auth.Claims{UserID: "root", Role: models.RolePlatformAdmin, TenantID: "default"}
}

func doRawJSON(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateAdapterEndpoint(t *testing.T) {
	t.Run("成功登记并回填归属", func(t *testing.T) {
		repo := &fakeFineTuneRepo{}
		r := newFineTuneRouter(repo, memberOf("t1"))

		w := doRawJSON(r, http.MethodPost, "/adapters",
			`{"name":"sql","base_model":"qwen2-7b","path":"s3://a","rank":16,"method":"lora","source_job_id":"job-9"}`)

		if w.Code != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
		}
		var got ports.FineTuneAdapter
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("解析响应: %v", err)
		}
		if got.ID == "" {
			t.Fatal("响应未包含生成的 ID")
		}
		if got.TenantID != "t1" || got.CreatedBy != "u-t1" {
			t.Fatalf("归属字段未按主体回填: %+v", got)
		}
		if got.SourceJobID != "job-9" {
			t.Fatalf("血缘字段丢失: %+v", got)
		}
	})

	t.Run("请求体伪造的租户必须被忽略", func(t *testing.T) {
		repo := &fakeFineTuneRepo{}
		r := newFineTuneRouter(repo, memberOf("t1"))

		w := doRawJSON(r, http.MethodPost, "/adapters",
			`{"name":"evil","base_model":"b","path":"p","rank":8,"tenant_id":"victim","created_by":"admin"}`)

		if w.Code != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
		}
		if repo.items[0].TenantID != "t1" {
			t.Fatalf("请求体中的 tenant_id 被采信: %q", repo.items[0].TenantID)
		}
		if repo.items[0].CreatedBy != "u-t1" {
			t.Fatalf("请求体中的 created_by 被采信: %q", repo.items[0].CreatedBy)
		}
	})

	t.Run("缺省方法与秩由服务端补齐", func(t *testing.T) {
		repo := &fakeFineTuneRepo{}
		r := newFineTuneRouter(repo, memberOf("t1"))

		w := doRawJSON(r, http.MethodPost, "/adapters", `{"name":"d","base_model":"b","path":"p"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
		}
		if repo.items[0].Method != ports.MethodLoRA || repo.items[0].Rank != 8 {
			t.Fatalf("缺省值未补齐: %+v", repo.items[0])
		}
	})

	t.Run("非法定义返回 400", func(t *testing.T) {
		cases := map[string]string{
			"缺名称":    `{"base_model":"b","path":"p"}`,
			"缺基座":    `{"name":"n","path":"p"}`,
			"缺路径":    `{"name":"n","base_model":"b"}`,
			"秩越界":    `{"name":"n","base_model":"b","path":"p","rank":9999}`,
			"未知方法":   `{"name":"n","base_model":"b","path":"p","method":"full"}`,
			"畸形JSON": `{"name":`,
		}
		for label, body := range cases {
			t.Run(label, func(t *testing.T) {
				repo := &fakeFineTuneRepo{}
				r := newFineTuneRouter(repo, memberOf("t1"))

				w := doRawJSON(r, http.MethodPost, "/adapters", body)
				if w.Code != http.StatusBadRequest {
					t.Fatalf("期望 400，实际 %d: %s", w.Code, w.Body.String())
				}
				
				if len(repo.items) != 0 {
					t.Fatalf("非法请求仍写入了数据: %+v", repo.items)
				}
			})
		}
	})

	t.Run("重名返回 409", func(t *testing.T) {
		repo := &fakeFineTuneRepo{createErr: ports.ErrAdapterConflict}
		r := newFineTuneRouter(repo, memberOf("t1"))

		w := doRawJSON(r, http.MethodPost, "/adapters", `{"name":"dup","base_model":"b","path":"p"}`)
		
		if w.Code != http.StatusConflict {
			t.Fatalf("期望 409，实际 %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestListAdaptersEndpoint(t *testing.T) {
	seed := func() *fakeFineTuneRepo {
		return &fakeFineTuneRepo{items: []*ports.FineTuneAdapter{
			{ID: "a1", Name: "a1", BaseModel: "qwen2-7b", TenantID: "t1"},
			{ID: "a2", Name: "a2", BaseModel: "llama3-8b", TenantID: "t1"},
			{ID: "b1", Name: "b1", BaseModel: "qwen2-7b", TenantID: "t2"},
		}}
	}

	t.Run("普通成员只见本租户且租户条件已下推", func(t *testing.T) {
		repo := seed()
		r := newFineTuneRouter(repo, memberOf("t1"))

		w := doRawJSON(r, http.MethodGet, "/adapters", "")
		if w.Code != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", w.Code)
		}
		
		if repo.lastFilter.TenantID != "t1" {
			t.Fatalf("租户条件未下推到仓储: %q", repo.lastFilter.TenantID)
		}
		var resp struct {
			Adapters []ports.FineTuneAdapter `json:"adapters"`
			Total    int                     `json:"total"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析响应: %v", err)
		}
		if resp.Total != 2 || len(resp.Adapters) != 2 {
			t.Fatalf("期望 2 条，实际 %d", resp.Total)
		}
		for _, a := range resp.Adapters {
			if a.TenantID != "t1" {
				t.Fatalf("返回了他租户适配器: %+v", a)
			}
		}
	})

	t.Run("平台管理员获得跨租户视图", func(t *testing.T) {
		repo := seed()
		r := newFineTuneRouter(repo, platformAdmin())

		w := doRawJSON(r, http.MethodGet, "/adapters", "")
		if w.Code != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", w.Code)
		}
		
		if repo.lastFilter.TenantID != "" {
			t.Fatalf("管理员不应被限定租户: %q", repo.lastFilter.TenantID)
		}
	})

	t.Run("按基座模型过滤", func(t *testing.T) {
		repo := seed()
		r := newFineTuneRouter(repo, memberOf("t1"))

		w := doRawJSON(r, http.MethodGet, "/adapters?base_model=qwen2-7b", "")
		if repo.lastFilter.BaseModel != "qwen2-7b" {
			t.Fatalf("基座过滤条件未透传: %q", repo.lastFilter.BaseModel)
		}
		var resp struct {
			Total int `json:"total"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Total != 1 {
			t.Fatalf("期望 1 条，实际 %d", resp.Total)
		}
	})

	t.Run("非法 limit 被忽略而非报错", func(t *testing.T) {
		repo := seed()
		r := newFineTuneRouter(repo, memberOf("t1"))

		for _, q := range []string{"limit=abc", "limit=-5", "limit=0"} {
			w := doRawJSON(r, http.MethodGet, "/adapters?"+q, "")
			if w.Code != http.StatusOK {
				t.Fatalf("%s 期望 200，实际 %d", q, w.Code)
			}
			
			if repo.lastFilter.Limit != 0 {
				t.Fatalf("%s 非法 limit 未被忽略: %d", q, repo.lastFilter.Limit)
			}
		}
	})
}

func TestGetAndDeleteAdapterEndpoint(t *testing.T) {
	seed := func() *fakeFineTuneRepo {
		return &fakeFineTuneRepo{items: []*ports.FineTuneAdapter{
			{ID: "a1", Name: "a1", BaseModel: "qwen2-7b", TenantID: "t1"},
		}}
	}

	t.Run("本租户可读取", func(t *testing.T) {
		r := newFineTuneRouter(seed(), memberOf("t1"))
		w := doRawJSON(r, http.MethodGet, "/adapters/a1", "")
		if w.Code != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("他租户读取返回 404 而非 403", func(t *testing.T) {
		repo := seed()
		r := newFineTuneRouter(repo, memberOf("t2"))

		w := doRawJSON(r, http.MethodGet, "/adapters/a1", "")
		
		if w.Code != http.StatusNotFound {
			t.Fatalf("期望 404，实际 %d: %s", w.Code, w.Body.String())
		}
		if repo.lastTenant != "t2" {
			t.Fatalf("租户条件未下推: %q", repo.lastTenant)
		}
	})

	t.Run("不存在的 ID 返回 404", func(t *testing.T) {
		r := newFineTuneRouter(seed(), memberOf("t1"))
		w := doRawJSON(r, http.MethodGet, "/adapters/none", "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("期望 404，实际 %d", w.Code)
		}
	})

	t.Run("他租户删除失败且数据保留", func(t *testing.T) {
		repo := seed()
		r := newFineTuneRouter(repo, memberOf("t2"))

		w := doRawJSON(r, http.MethodDelete, "/adapters/a1", "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("期望 404，实际 %d", w.Code)
		}
		if len(repo.items) != 1 {
			t.Fatal("越权删除竟然改动了数据")
		}
	})

	t.Run("本租户删除成功", func(t *testing.T) {
		repo := seed()
		r := newFineTuneRouter(repo, memberOf("t1"))

		w := doRawJSON(r, http.MethodDelete, "/adapters/a1", "")
		if w.Code != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
		}
		if len(repo.items) != 0 {
			t.Fatal("删除未生效")
		}
	})
}

func TestFineTuneEndpointsDegradeWithoutRepo(t *testing.T) {
	r := newFineTuneRouter(nil, platformAdmin())

	cases := []struct{ method, path, body string }{
		{http.MethodPost, "/adapters", `{"name":"n","base_model":"b","path":"p"}`},
		{http.MethodGet, "/adapters", ""},
		{http.MethodGet, "/adapters/a1", ""},
		{http.MethodDelete, "/adapters/a1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := doRawJSON(r, tc.method, tc.path, tc.body)
			if w.Code != http.StatusNotImplemented {
				t.Fatalf("期望 501，实际 %d: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "not configured") {
				t.Fatalf("降级响应缺少原因说明: %s", w.Body.String())
			}
		})
	}
}