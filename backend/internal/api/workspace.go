package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/app/workspace"
	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"github.com/gin-gonic/gin"
)

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		if b == "" {
			return a
		}
		return a + "/" + b
	}
	return a + b
}

type workspaceView struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	Name          string     `json:"name"`
	Kind          string     `json:"kind"`
	OwnerID       string     `json:"owner_id"`
	Image         string     `json:"image"`
	Status        string     `json:"status"`
	GPUCount      int        `json:"gpu_count"`
	GPUModel      string     `json:"gpu_model"`
	CPURequest    string     `json:"cpu_request"`
	MemoryRequest string     `json:"memory_request"`
	IdleTimeout   int        `json:"idle_timeout_seconds"`
	LastActiveAt  *time.Time `json:"last_active_at,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	StoppedAt     *time.Time `json:"stopped_at,omitempty"`
	URL           string     `json:"url,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func toWorkspaceView(ws *models.Workspace, url string) workspaceView {
	return workspaceView{
		ID:            ws.ID,
		TenantID:      ws.TenantID,
		Name:          ws.Name,
		Kind:          string(ws.Kind),
		OwnerID:       ws.OwnerID,
		Image:         ws.Image,
		Status:        string(ws.Status),
		GPUCount:      ws.GPUCount,
		GPUModel:      ws.GPUModel,
		CPURequest:    ws.CPURequest,
		MemoryRequest: ws.MemoryRequest,
		IdleTimeout:   int(ws.IdleTimeout.Seconds()),
		LastActiveAt:  ws.LastActiveAt,
		StartedAt:     ws.StartedAt,
		StoppedAt:     ws.StoppedAt,
		URL:           url,
		CreatedAt:     ws.CreatedAt,
		UpdatedAt:     ws.UpdatedAt,
	}
}

type workspaceCreateReq struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	OwnerID       string `json:"owner_id"`
	Image         string `json:"image"`
	GPUCount      int    `json:"gpu_count"`
	GPUModel      string `json:"gpu_model"`
	CPURequest    string `json:"cpu_request"`
	MemoryRequest string `json:"memory_request"`
	IdleTimeout   int    `json:"idle_timeout_seconds"`
}

func (h *Handler) workspaceService(c *gin.Context) (*workspace.Service, bool) {
	if h.workspace == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "workspace service not configured"})
		return nil, false
	}
	return h.workspace, true
}

func (h *Handler) workspaceWired() bool {
	return h.workspace != nil && h.workspaceRepo != nil && h.workspaceRT != nil
}

func (h *Handler) tenantIDFromQuery(c *gin.Context) string {
	t := c.Query("tenant_id")
	if t == "" {
		return h.principalTenant(c)
	}
	
	if claims, ok := auth.Principal(c); ok && claims.Role == models.RolePlatformAdmin {
		return t
	}
	return h.principalTenant(c)
}

func (h *Handler) ListWorkspaces(c *gin.Context) {
	if !h.workspaceWired() {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "workspace service not configured"})
		return
	}
	tenantID := h.tenantIDFromQuery(c)
	filter := ports.WorkspaceFilter{
		OwnerID: c.Query("owner_id"),
	}
	if s := c.Query("status"); s != "" {
		filter.Status = models.WorkspaceStatus(s)
	}
	items, err := h.workspaceRepo.ListWorkspaces(tenantID, filter)
	if err != nil {
		renderError(c, err)
		return
	}
	views := make([]workspaceView, 0, len(items))
	for i := range items {
		views = append(views, toWorkspaceView(&items[i], ""))
	}
	c.JSON(http.StatusOK, gin.H{"items": views, "total": len(views)})
}

func (h *Handler) CreateWorkspace(c *gin.Context) {
	svc, ok := h.workspaceService(c)
	if !ok {
		return
	}
	var req workspaceCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	tenantID := h.tenantIDFromQuery(c)
	ws := &models.Workspace{
		TenantID:      tenantID,
		Name:          req.Name,
		Kind:          models.WorkspaceKind(req.Kind),
		OwnerID:       req.OwnerID,
		Image:         req.Image,
		GPUCount:      req.GPUCount,
		GPUModel:      req.GPUModel,
		CPURequest:    req.CPURequest,
		MemoryRequest: req.MemoryRequest,
		IdleTimeout:   time.Duration(req.IdleTimeout) * time.Second,
	}
	if err := svc.Create(c.Request.Context(), ws); err != nil {
		renderError(c, err)
		return
	}
	h.audit(c, models.ActionCreate, models.ResWorkspace, ws.ID, ws.Name)
	c.JSON(http.StatusOK, toWorkspaceView(ws, ""))
}

func (h *Handler) GetWorkspace(c *gin.Context) {
	if !h.workspaceWired() {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "workspace service not configured"})
		return
	}
	tenantID := h.tenantIDFromQuery(c)
	ws, err := h.workspaceRepo.GetWorkspaceForTenant(tenantID, c.Param("id"))
	if err != nil {
		renderError(c, err)
		return
	}
	url, _ := h.workspaceRT.URL(ws)
	c.JSON(http.StatusOK, toWorkspaceView(ws, url))
}

func (h *Handler) StartWorkspace(c *gin.Context) {
	svc, ok := h.workspaceService(c)
	if !ok {
		return
	}
	tenantID := h.tenantIDFromQuery(c)
	actor := actorOrDefault(c)
	if err := svc.Start(c.Request.Context(), tenantID, actor, true, c.Param("id")); err != nil {
		renderError(c, err)
		return
	}
	ws, err := h.workspaceRepo.GetWorkspaceForTenant(tenantID, c.Param("id"))
	if err != nil {
		renderError(c, err)
		return
	}
	h.audit(c, models.ActionUpdate, models.ResWorkspace, ws.ID, "start")
	c.JSON(http.StatusOK, toWorkspaceView(ws, ""))
}

func (h *Handler) StopWorkspace(c *gin.Context) {
	svc, ok := h.workspaceService(c)
	if !ok {
		return
	}
	tenantID := h.tenantIDFromQuery(c)
	actor := actorOrDefault(c)
	if err := svc.Stop(c.Request.Context(), tenantID, actor, true, c.Param("id")); err != nil {
		renderError(c, err)
		return
	}
	ws, err := h.workspaceRepo.GetWorkspaceForTenant(tenantID, c.Param("id"))
	if err != nil {
		renderError(c, err)
		return
	}
	h.audit(c, models.ActionUpdate, models.ResWorkspace, ws.ID, "stop")
	c.JSON(http.StatusOK, toWorkspaceView(ws, ""))
}

func (h *Handler) DeleteWorkspace(c *gin.Context) {
	svc, ok := h.workspaceService(c)
	if !ok {
		return
	}
	tenantID := h.tenantIDFromQuery(c)
	actor := actorOrDefault(c)
	if err := svc.Delete(c.Request.Context(), tenantID, actor, true, c.Param("id")); err != nil {
		renderError(c, err)
		return
	}
	h.audit(c, models.ActionDelete, models.ResWorkspace, c.Param("id"), "")
	c.JSON(http.StatusOK, gin.H{"deleted": c.Param("id")})
}

func actorOrDefault(c *gin.Context) string {
	if p, ok := auth.Principal(c); ok && p != nil && p.Username != "" {
		return p.Username
	}
	if a := c.Query("actor"); a != "" {
		return a
	}
	return "platform-admin"
}

func (h *Handler) WorkspaceActivity(c *gin.Context) {
	if !h.workspaceWired() {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "workspace service not configured"})
		return
	}
	tenantID := h.tenantIDFromQuery(c)
	
	if _, err := h.workspaceRepo.GetWorkspaceForTenant(tenantID, c.Param("id")); err != nil {
		renderError(c, err)
		return
	}
	if err := h.workspaceRepo.TouchWorkspace(c.Param("id"), time.Now()); err != nil {
		renderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": c.Param("id"), "active": true})
}

func (h *Handler) WorkspaceProxy(c *gin.Context) {
	if !h.workspaceWired() {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "workspace proxy not configured"})
		return
	}
	tenantID := h.tenantIDFromQuery(c)
	ws, err := h.workspaceRepo.GetWorkspaceForTenant(tenantID, c.Param("id"))
	if err != nil {
		renderError(c, err)
		return
	}
	if ws.Status != models.WorkspaceStatusRunning {
		c.JSON(http.StatusConflict, gin.H{"error": "workspace not running"})
		return
	}
	target, err := h.workspaceRT.ProxyTarget(ws)
	if err != nil || target == "" {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "workspace proxy target unavailable (no cluster)"})
		return
	}
	
	if !h.proxyLimiter.Allow(tenantID) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "workspace proxy rate limited for tenant"})
		return
	}
	proxyWorkspace(c, target, h.workspaceRT, ws.ID)
}

func renderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, workspace.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, workspace.ErrImageNotAllowed),
		errors.Is(err, workspace.ErrIllegalState):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

func proxyWorkspace(c *gin.Context, target string, rt interface{}, wsID string) {
	targetURL, err := url.Parse(target)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid proxy target: " + err.Error()})
		return
	}

	const prefix = "/api/v1/workspaces/"
	rest := strings.TrimPrefix(c.Request.URL.Path, prefix)
	
	const proxySeg = "/proxy"
	if idx := strings.Index(rest, proxySeg); idx >= 0 {
		rest = rest[idx+len(proxySeg):]
	}
	if rest == "" {
		rest = "/"
	}

	rp := &httputil.ReverseProxy{
		
		Rewrite: func(req *httputil.ProxyRequest) {
			req.SetURL(targetURL)
			req.SetXForwarded()
			req.Out.Host = targetURL.Host
			req.Out.URL.Path = singleJoiningSlash(targetURL.Path, rest)
			req.Out.URL.RawQuery = c.Request.URL.RawQuery
			if _, ok := req.Out.Header["User-Agent"]; !ok {
				req.Out.Header.Set("User-Agent", "")
			}
		},
	}
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":"workspace proxy upstream unreachable: `+e.Error()+`"}`)
	}
	
	rp.FlushInterval = -1
	rp.ServeHTTP(c.Writer, c.Request)

	if notifier, ok := rt.(interface {
		Heartbeat(context.Context, string, time.Time) error
	}); ok {
		_ = notifier.Heartbeat(c.Request.Context(), wsID, time.Now())
	}
}