package api

import (
	"fmt"
	"net/http"
	"time"

	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ListTenants(c *gin.Context) {
	tenants, err := h.tenantRepo.ListTenants()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	if scope := h.tenantScope(c); scope != "" {
		filtered := tenants[:0]
		for _, t := range tenants {
			if t.ID == scope {
				filtered = append(filtered, t)
			}
		}
		tenants = filtered
	}
	c.JSON(http.StatusOK, tenants)
}

func (h *Handler) CreateTenant(c *gin.Context) {
	var t models.Tenant
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if t.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if t.ID == "" {
		t.ID = fmt.Sprintf("tn-%d", time.Now().UnixNano())
	}
	if err := h.tenantRepo.CreateTenant(&t); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	if _, err := h.quotaRepo.GetQuota(t.ID); err != nil {
		if err := h.quotaRepo.UpsertQuota(&models.Quota{ID: t.ID, TenantID: t.ID}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	h.audit(c, models.ActionCreate, models.ResTenant, t.ID, t.Name)
	c.JSON(http.StatusCreated, t)
}

func (h *Handler) GetTenant(c *gin.Context) {
	
	if !h.canAccessTenant(c.Param("id"), c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}
	t, err := h.tenantRepo.GetTenant(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *Handler) UpdateTenant(c *gin.Context) {
	id := c.Param("id")
	
	if !h.canAccessTenant(id, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}
	t, err := h.tenantRepo.GetTenant(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}
	var patch models.Tenant
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if patch.Name != "" {
		t.Name = patch.Name
	}
	if patch.Description != "" {
		t.Description = patch.Description
	}
	if err := h.tenantRepo.UpdateTenant(t); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, models.ActionUpdate, models.ResTenant, id, t.Name)
	c.JSON(http.StatusOK, t)
}

func (h *Handler) DeleteTenant(c *gin.Context) {
	id := c.Param("id")
	if id == "default" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete default tenant"})
		return
	}
	if err := h.tenantRepo.DeleteTenant(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, models.ActionDelete, models.ResTenant, id, "")
	c.JSON(http.StatusNoContent, nil)
}

func (h *Handler) ListQuotas(c *gin.Context) {
	qs, err := h.quotaRepo.ListQuotas()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	if scope := h.tenantScope(c); scope != "" {
		filtered := qs[:0]
		for _, q := range qs {
			if q.TenantID == scope {
				filtered = append(filtered, q)
			}
		}
		qs = filtered
	}
	c.JSON(http.StatusOK, qs)
}

func (h *Handler) GetQuota(c *gin.Context) {
	
	if !h.canAccessTenant(c.Param("tenantId"), c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "quota not found"})
		return
	}
	q, err := h.quotaRepo.GetQuota(c.Param("tenantId"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "quota not found"})
		return
	}
	c.JSON(http.StatusOK, q)
}

type quotaUpdate struct {
	GPUQuota      int `json:"gpu_quota"`
	MemoryQuotaGB int `json:"memory_quota_gb"`
	JobQuota      int `json:"job_quota"`
}

func (h *Handler) UpdateQuota(c *gin.Context) {
	tenantID := c.Param("tenantId")
	
	if !h.canAccessTenant(tenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}
	if _, err := h.tenantRepo.GetTenant(tenantID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}
	var body quotaUpdate
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	if body.GPUQuota < 0 || body.MemoryQuotaGB < 0 || body.JobQuota < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quota values must be non-negative"})
		return
	}
	q := &models.Quota{ID: tenantID, TenantID: tenantID, GPUQuota: body.GPUQuota, MemoryQuotaGB: body.MemoryQuotaGB, JobQuota: body.JobQuota}
	if err := h.quotaRepo.UpsertQuota(q); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, models.ActionUpdate, models.ResQuota, tenantID, fmt.Sprintf("gpu=%d mem=%d job=%d", body.GPUQuota, body.MemoryQuotaGB, body.JobQuota))
	c.JSON(http.StatusOK, q)
}