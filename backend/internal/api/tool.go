package api

import (
	"errors"
	"log"
	"net/http"
	"net/url"

	"fuze-ai-paas/backend/internal/domain/agent"
	"fuze-ai-paas/backend/internal/ports"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) requireToolRepo(c *gin.Context) bool {
	if h.toolRepo == nil {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": "tool registry not enabled (missing adapter)",
		})
		return false
	}
	return true
}

type createToolRequest struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Kind        agent.ToolKind      `json:"kind"`
	HTTP        *agent.HTTPToolSpec `json:"http,omitempty"`
	Sensitive   bool                `json:"sensitive"`
}

func (h *Handler) ListTools(c *gin.Context) {
	if !h.requireToolRepo(c) {
		return
	}
	list, err := h.toolRepo.List(c.Request.Context(), h.principalTenant(c))
	if err != nil {
		log.Printf("[tool] ListTools failed for tenant %s: %v", h.principalTenant(c), err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tools": list})
}

func (h *Handler) CreateTool(c *gin.Context) {
	if !h.requireToolRepo(c) {
		return
	}
	var req createToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if req.Kind == "" {
		req.Kind = agent.ToolKindHTTP
	}
	if req.Kind == agent.ToolKindHTTP {
		if req.HTTP == nil || req.HTTP.URL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "http tool requires http.url"})
			return
		}
		if !isValidURL(req.HTTP.URL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "http.url must be http(s)"})
			return
		}
	}
	id := req.ID
	if id == "" {
		id = "tool-" + uuid.NewString()
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	t := &agent.Tool{
		ID:          id,
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		Kind:        req.Kind,
		HTTP:        req.HTTP,
		Sensitive:   req.Sensitive,
	}
	if err := h.toolRepo.Create(c.Request.Context(), t); err != nil {
		if errors.Is(err, ports.ErrToolConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "tool name conflict"})
			return
		}
		log.Printf("[tool] CreateTool failed for tenant %s tool %s: %v", h.principalTenant(c), t.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"tool": t})
}

func (h *Handler) GetTool(c *gin.Context) {
	if !h.requireToolRepo(c) {
		return
	}
	t, err := h.toolRepo.Get(c.Request.Context(), h.principalTenant(c), c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ports.ErrToolNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tool": t})
}

func (h *Handler) DeleteTool(c *gin.Context) {
	if !h.requireToolRepo(c) {
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	if err := h.toolRepo.Delete(c.Request.Context(), tenantID, c.Param("id")); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ports.ErrToolNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func isValidURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" {
		return false
	}
	return true
}