package api

import (
	"net/http"

	optimizeapp "fuze-ai-paas/backend/internal/app/optimize"
	"fuze-ai-paas/backend/internal/domain/optimize"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ListCompressionTasks(c *gin.Context) {
	if h.compress == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "compression service not configured"})
		return
	}
	tenantID := ""
	if !h.isPlatformAdmin(c) {
		tenantID = h.principalTenant(c)
	}
	list, err := h.compress.List(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": list})
}

func (h *Handler) CreateCompressionTask(c *gin.Context) {
	if h.compress == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "compression service not configured"})
		return
	}
	var body struct {
		Name           string  `json:"name"`
		Type           string  `json:"type"`
		Backend        string  `json:"backend"`
		Config         string  `json:"config"`
		ModelVersionID string  `json:"model_version_id"`
		GateThreshold  float64 `json:"gate_threshold"`
		OrigAccuracy   float64 `json:"orig_accuracy"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	created, err := h.compress.Create(c.Request.Context(), optimizeapp.CreateInput{
		Name:           body.Name,
		TenantID:       tenantID,
		Type:           optimize.CompressionType(body.Type),
		Backend:        optimize.CompressionBackend(body.Backend),
		ConfigJSON:     body.Config,
		ModelVersionID: body.ModelVersionID,
		GateThreshold:  body.GateThreshold,
		OrigAccuracy:   body.OrigAccuracy,
	})
	if err != nil {
		
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (h *Handler) GetCompressionTask(c *gin.Context) {
	if h.compress == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "compression service not configured"})
		return
	}
	t, err := h.compress.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondOptimizeError(c, err)
		return
	}
	if !h.canAccessTenant(t.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "compression task not found"})
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *Handler) CancelCompressionTask(c *gin.Context) {
	if h.compress == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "compression service not configured"})
		return
	}
	id := c.Param("id")
	owning, err := h.compress.Get(c.Request.Context(), id)
	if err != nil {
		respondOptimizeError(c, err)
		return
	}
	if !h.canAccessTenant(owning.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "compression task not found"})
		return
	}
	if err := h.compress.Cancel(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

func (h *Handler) DeleteCompressionTask(c *gin.Context) {
	if h.compress == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "compression service not configured"})
		return
	}
	id := c.Param("id")
	owning, err := h.compress.Get(c.Request.Context(), id)
	if err != nil {
		respondOptimizeError(c, err)
		return
	}
	if !h.canAccessTenant(owning.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "compression task not found"})
		return
	}
	if err := h.compress.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (h *Handler) HandleCompressionResult(c *gin.Context) {
	if h.compress == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "compression service not configured"})
		return
	}
	id := c.Param("id")
	if err := h.compress.HandleResult(c.Request.Context(), id); err != nil {
		
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "resolved"})
}

func respondOptimizeError(c *gin.Context, err error) {
	if err == optimize.ErrNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "compression task not found"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}