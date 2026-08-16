package llmgateway

import (
	"context"
	"errors"
	"testing"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/ports"
)

type fakeMountResolver struct {
	mounts    map[string]*ports.AdapterMount
	err       error
	gotTenant string
	gotServed string
	calls     int
}

func (f *fakeMountResolver) ResolveServedName(_ context.Context, tenantID, served string) (*ports.AdapterMount, error) {
	f.calls++
	f.gotTenant, f.gotServed = tenantID, served
	if f.err != nil {
		return nil, f.err
	}
	m, ok := f.mounts[served]
	if !ok {
		return nil, ports.ErrAdapterNotMounted
	}
	return m, nil
}

func mountedResolver() *fakeMountResolver {
	return &fakeMountResolver{mounts: map[string]*ports.AdapterMount{
		"qwen:sql-expert": {
			AdapterID: "ad-1", ServiceID: "svc-1",
			ServedName: "qwen:sql-expert", BaseModel: "qwen", TenantID: "t1",
		},
	}}
}

func adapterReq() Request {
	r := userReq()
	r.Chat.Model = "qwen:sql-expert"
	return r
}

func TestCompletePlainModelUnaffectedByResolver(t *testing.T) {
	fc := &fakeCompleter{text: "ok"}
	res := mountedResolver()
	svc := newSvc(t, Deps{Completer: fc, Routes: routeWith("http://a"), Mounts: res})

	if _, err := svc.Complete(context.Background(), userReq()); err != nil {
		t.Fatalf("纯基座调用不应受影响: %v", err)
	}
	if res.calls != 0 {
		t.Fatalf("纯基座调用不应触发挂载查询，实际 %d 次", res.calls)
	}
	if fc.received.Model != "qwen" {
		t.Fatalf("透传模型名被篡改: %q", fc.received.Model)
	}
}

func TestCompleteResolvesAdapterToBaseRoute(t *testing.T) {
	fc := &fakeCompleter{text: "ok"}
	res := mountedResolver()
	svc := newSvc(t, Deps{Completer: fc, Routes: routeWith("http://a"), Mounts: res})

	if _, err := svc.Complete(context.Background(), adapterReq()); err != nil {
		t.Fatalf("挂载后应能按基座路由: %v", err)
	}

	if res.gotTenant != "t1" || res.gotServed != "qwen:sql-expert" {
		t.Fatalf("挂载查询参数不对: tenant=%q served=%q", res.gotTenant, res.gotServed)
	}
	
	if fc.received.Model != "qwen:sql-expert" {
		t.Fatalf("传给后端的模型名应保留适配器后缀，实际 %q", fc.received.Model)
	}
}

func TestCompleteUnmountedAdapterRejected(t *testing.T) {
	fc := &fakeCompleter{text: "ok"}
	
	res := &fakeMountResolver{mounts: map[string]*ports.AdapterMount{}}
	svc := newSvc(t, Deps{Completer: fc, Routes: routeWith("http://a"), Mounts: res})

	_, err := svc.Complete(context.Background(), adapterReq())
	if !errors.Is(err, ports.ErrAdapterNotMounted) {
		t.Fatalf("未挂载应返回 ErrAdapterNotMounted，实际: %v", err)
	}
	
	if len(fc.callLog()) != 0 {
		t.Fatalf("解析失败不应调用后端，实际: %v", fc.callLog())
	}
}

func TestCompleteWithoutResolverSkipsResolution(t *testing.T) {
	fc := &fakeCompleter{text: "ok"}
	
	svc := newSvc(t, Deps{Completer: fc, Routes: routeWith("http://a")})

	_, err := svc.Complete(context.Background(), adapterReq())
	
	if !errors.Is(err, llm.ErrModelNotFound) {
		t.Fatalf("未接线解析器时应回落既有行为，实际: %v", err)
	}
}

func TestStreamResolvesAdapterToBaseRoute(t *testing.T) {
	fc := &fakeCompleter{chunks: []string{"a", "b"}}
	res := mountedResolver()
	svc := newSvc(t, Deps{Completer: fc, Routes: routeWith("http://a"), Mounts: res})

	var got string
	_, err := svc.Stream(context.Background(), adapterReq(), func(c llm.Chunk) error {
		got += c.Delta
		return nil
	})
	if err != nil {
		t.Fatalf("流式挂载调用应成功: %v", err)
	}
	if got != "ab" {
		t.Fatalf("流式内容不完整: %q", got)
	}
	if fc.received.Model != "qwen:sql-expert" {
		t.Fatalf("流式传给后端的模型名应保留适配器后缀，实际 %q", fc.received.Model)
	}
}

func TestResolveErrorNotMaskedAsNotMounted(t *testing.T) {
	boom := errors.New("db down")
	fc := &fakeCompleter{text: "ok"}
	svc := newSvc(t, Deps{
		Completer: fc, Routes: routeWith("http://a"),
		Mounts: &fakeMountResolver{err: boom},
	})

	_, err := svc.Complete(context.Background(), adapterReq())
	
	if !errors.Is(err, boom) {
		t.Fatalf("底层错误应透传，实际: %v", err)
	}
}