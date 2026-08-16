package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/app/workspace"
	wsk8s "fuze-ai-paas/backend/internal/k8s/workspace"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"github.com/gin-gonic/gin"
)

type fakeWSRepo struct {
	ws *models.Workspace
}

func (f *fakeWSRepo) ListWorkspaces(string, ports.WorkspaceFilter) ([]models.Workspace, error) {
	return nil, nil
}
func (f *fakeWSRepo) GetWorkspaceForTenant(string, string) (*models.Workspace, error) { return f.ws, nil }
func (f *fakeWSRepo) CreateWorkspace(*models.Workspace) error                          { return nil }
func (f *fakeWSRepo) UpdateWorkspace(*models.Workspace) error                          { return nil }
func (f *fakeWSRepo) DeleteWorkspace(string) error                                     { return nil }
func (f *fakeWSRepo) DeleteWorkspaceAndReleaseQuota(string, string, int, int) error    { return nil }
func (f *fakeWSRepo) GetWorkspaceByName(string, string) (*models.Workspace, error)     { return f.ws, nil }
func (f *fakeWSRepo) TouchWorkspace(string, time.Time) error                           { return nil }
func (f *fakeWSRepo) ListReclaimable(time.Time) ([]models.Workspace, error)            { return nil, nil }

type fakeWSRT struct {
	target    string
	heartbeat bool
}

func (f *fakeWSRT) ProxyTarget(*models.Workspace) (string, error) { return f.target, nil }
func (f *fakeWSRT) URL(*models.Workspace) (string, error)         { return f.target, nil }
func (f *fakeWSRT) Provision(context.Context, *models.Workspace) (string, error) {
	return "ws-x", nil
}
func (f *fakeWSRT) Deprovision(context.Context, *models.Workspace) error { return nil }
func (f *fakeWSRT) Heartbeat(_ context.Context, _ string, _ time.Time) error {
	f.heartbeat = true
	return nil
}

type proxyFakeQuota struct{}

func (proxyFakeQuota) GetQuota(string) (*models.Quota, error)      { return &models.Quota{}, nil }
func (proxyFakeQuota) ListQuotas() ([]models.Quota, error)         { return nil, nil }
func (proxyFakeQuota) UpsertQuota(*models.Quota) error             { return nil }
func (proxyFakeQuota) CheckAndReserve(string, int, int, int) error { return nil }
func (proxyFakeQuota) Release(string, int, int, int) error         { return nil }

func newProxyHandler(repo *fakeWSRepo, rt *fakeWSRT) *Handler {
	svc := workspace.NewService(repo, proxyFakeQuota{}, rt, workspace.DefaultImagePolicy())
	return &Handler{workspace: svc, workspaceRepo: repo, workspaceRT: rt}
}

func newProxyServer(h *Handler) *httptest.Server {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/workspaces/:id/proxy/*path", h.WorkspaceProxy)
	return httptest.NewServer(r)
}

func TestWorkspaceProxyForwardsStrippingPrefix(t *testing.T) {
	var gotPath, gotToken string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Fuze-Token")
		_, _ = w.Write([]byte("JUPYTER_OK:" + r.URL.Path))
	}))
	defer upstream.Close()

	repo := &fakeWSRepo{ws: &models.Workspace{ID: "ws-123", TenantID: "t1", Status: models.WorkspaceStatusRunning}}
	rt := &fakeWSRT{target: upstream.URL}
	ts := newProxyServer(newProxyHandler(repo, rt))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/workspaces/ws-123/proxy/api/kernels/abc", nil)
	req.Header.Set("X-Fuze-Token", "secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if gotPath != "/api/kernels/abc" {
		t.Errorf("upstream path = %q, want %q", gotPath, "/api/kernels/abc")
	}
	if !strings.HasPrefix(string(body), "JUPYTER_OK:/api/kernels/abc") {
		t.Errorf("body = %q", body)
	}
	if gotToken != "secret" {
		t.Errorf("upstream token = %q, want %q", gotToken, "secret")
	}
	if !rt.heartbeat {
		t.Errorf("expected Heartbeat to be called on proxy")
	}
}

func TestWorkspaceProxyRootPath(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte("OK"))
	}))
	defer upstream.Close()

	repo := &fakeWSRepo{ws: &models.Workspace{ID: "ws-1", TenantID: "t1", Status: models.WorkspaceStatusRunning}}
	rt := &fakeWSRT{target: upstream.URL}
	ts := newProxyServer(newProxyHandler(repo, rt))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/workspaces/ws-1/proxy/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK || gotPath != "/" {
		t.Fatalf("status=%d upstreamPath=%q", resp.StatusCode, gotPath)
	}
}

func TestWorkspaceProxyNotRunning(t *testing.T) {
	repo := &fakeWSRepo{ws: &models.Workspace{ID: "ws-1", TenantID: "t1", Status: models.WorkspaceStatusStopped}}
	rt := &fakeWSRT{target: "http://unused"}
	ts := newProxyServer(newProxyHandler(repo, rt))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/workspaces/ws-1/proxy/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestWorkspaceProxyWebSocketUpgrade(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Upgrade", "websocket")
		w.Header().Set("Connection", "Upgrade")
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	defer upstream.Close()

	repo := &fakeWSRepo{ws: &models.Workspace{ID: "ws-1", TenantID: "t1", Status: models.WorkspaceStatusRunning}}
	rt := &fakeWSRT{target: upstream.URL}
	ts := newProxyServer(newProxyHandler(repo, rt))
	defer ts.Close()

	u := strings.Replace(ts.URL, "http://", "", 1)
	conn, err := net.Dial("tcp", u)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	reqPath := "/api/v1/workspaces/ws-1/proxy/api/kernels/ws-1/channels"
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: x3JJHMbDL1EzLkh9GBhXDw==\r\nSec-WebSocket-Version: 13\r\n\r\n", reqPath, u)

	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(statusLine, "101") {
		t.Fatalf("status line = %q, want 101 Switching Protocols", statusLine)
	}
	
	for {
		line, err := br.ReadString('\n')
		if err != nil || line == "\r\n" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "upgrade:") && !strings.Contains(strings.ToLower(line), "websocket") {
			t.Errorf("upgrade header = %q", line)
		}
	}
}

func TestWorkspaceProxyTargetResolution(t *testing.T) {
	ws := &models.Workspace{ID: "ws-9"}

	d := wsk8s.NewDriver(nil, nil, "http://localhost:8888")
	got, err := d.ProxyTarget(ws)
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://localhost:8888/ws-ws-9" {
		t.Errorf("fallback target = %q, want http://localhost:8888/ws-ws-9", got)
	}

	d2 := wsk8s.NewDriver(nil, nil, "")
	if _, err := d2.ProxyTarget(ws); err == nil {
		t.Errorf("expected error when no cluster and no base URL")
	}
}