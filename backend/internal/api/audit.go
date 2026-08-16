package api

import (
	"net/http"
	"strconv"

	"fuze-ai-paas/backend/internal/ports"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ListAudit(c *gin.Context) {
	tenantID := c.Query("tenant_id")
	if scope := h.tenantScope(c); scope != "" {
		
		tenantID = scope
	}
	opts := ports.AuditQuery{
		Actor:        c.Query("actor"),
		TenantID:     tenantID,
		Action:       c.Query("action"),
		ResourceType: c.Query("resource_type"),
	}
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			opts.Limit = n
		}
	}
	logs, err := h.auditRepo.ListAudit(opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, logs)
}