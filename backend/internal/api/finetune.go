package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"github.com/gin-gonic/gin"
)

func respondAdapterError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ports.ErrAdapterNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "adapter not found"})
	case errors.Is(err, ports.ErrAdapterConflict):
		
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ports.ErrAdapterMounted):
		
		c.JSON(http.StatusConflict, gin.H{
			"error": "adapter is mounted on an inference service; unmount it first",
		})
	case errors.Is(err, ports.ErrMountConflict):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ports.ErrAdapterNotMounted):
		c.JSON(http.StatusNotFound, gin.H{"error": "adapter not mounted on service"})
	case errors.Is(err, ports.ErrIncompatibleBase),
		errors.Is(err, ports.ErrSourceJobNotFound),
		errors.Is(err, ports.ErrAdapterInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func (h *Handler) CreateFineTuneAdapter(c *gin.Context) {
	if h.finetune == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "finetune repository not configured"})
		return
	}

	var a ports.FineTuneAdapter
	if err := c.ShouldBindJSON(&a); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid adapter body"})
		return
	}

	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	a.TenantID = tenantID
	a.CreatedBy = h.claimsOf(c).UserID

	a.Normalize()
	if err := a.Validate(); err != nil {
		respondAdapterError(c, err)
		return
	}

	if err := ports.ValidateSourceJob(c.Request.Context(), h.adapterJobs, a); err != nil {
		respondAdapterError(c, err)
		return
	}

	if err := h.finetune.Create(c.Request.Context(), &a); err != nil {
		respondAdapterError(c, err)
		return
	}
	c.JSON(http.StatusOK, a)
}

func (h *Handler) ListFineTuneAdapters(c *gin.Context) {
	if h.finetune == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "finetune repository not configured"})
		return
	}

	f := ports.FineTuneFilter{
		
		TenantID:  h.tenantScope(c),
		BaseModel: c.Query("base_model"),
	}
	
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		f.Limit = v
	}

	list, err := h.finetune.List(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"adapters": list, "total": len(list)})
}

func (h *Handler) GetFineTuneAdapter(c *gin.Context) {
	if h.finetune == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "finetune repository not configured"})
		return
	}

	a, err := h.finetune.Get(c.Request.Context(), h.tenantScope(c), c.Param("id"))
	if err != nil {
		respondAdapterError(c, err)
		return
	}
	c.JSON(http.StatusOK, a)
}

func (h *Handler) DeleteFineTuneAdapter(c *gin.Context) {
	if h.finetune == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "finetune repository not configured"})
		return
	}

	if err := h.finetune.Delete(c.Request.Context(), h.tenantScope(c), c.Param("id")); err != nil {
		respondAdapterError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": c.Param("id")})
}

type adapterServiceReader interface {
	GetInferenceServiceForTenant(tenantID, id string) (*models.InferenceService, error)
}

type mountAdapterRequest struct {
	ServiceID string `json:"service_id"`
}

func serviceBaseModel(svc *models.InferenceService) string {
	if svc == nil {
		return ""
	}
	return svc.Name
}

func (h *Handler) MountFineTuneAdapter(c *gin.Context) {
	if h.finetune == nil || h.adapterMounts == nil || h.adapterServices == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "adapter mount not configured"})
		return
	}

	var req mountAdapterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mount body"})
		return
	}
	if strings.TrimSpace(req.ServiceID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "service_id required"})
		return
	}

	tenant := h.tenantScope(c)
	ctx := c.Request.Context()

	adapter, err := h.finetune.Get(ctx, tenant, c.Param("id"))
	if err != nil {
		respondAdapterError(c, err)
		return
	}

	svc, err := h.adapterServices.GetInferenceServiceForTenant(h.principalTenant(c), req.ServiceID)
	if err != nil {
		
		c.JSON(http.StatusNotFound, gin.H{"error": "inference service not found"})
		return
	}

	base := serviceBaseModel(svc)
	if base != adapter.BaseModel {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("%v: adapter requires base %q but service serves %q",
				ports.ErrIncompatibleBase, adapter.BaseModel, base),
		})
		return
	}

	m := &ports.AdapterMount{
		AdapterID:   adapter.ID,
		ServiceID:   svc.ID,
		BaseModel:   adapter.BaseModel,
		AdapterName: adapter.Name,
		TenantID:    adapter.TenantID,
		CreatedBy:   h.claimsOf(c).UserID,
	}
	m.Normalize()

	if err := h.adapterMounts.Mount(ctx, m); err != nil {
		respondAdapterError(c, err)
		return
	}
	h.audit(c, models.ActionMount, models.ResAdapter, m.AdapterID, m.ServedName)
	c.JSON(http.StatusOK, m)
}

func (h *Handler) ListFineTuneAdapterMounts(c *gin.Context) {
	if h.adapterMounts == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "adapter mount not configured"})
		return
	}

	list, err := h.adapterMounts.ListByAdapter(c.Request.Context(), h.tenantScope(c), c.Param("id"))
	if err != nil {
		respondAdapterError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"mounts": list, "total": len(list)})
}

func (h *Handler) UnmountFineTuneAdapter(c *gin.Context) {
	if h.adapterMounts == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "adapter mount not configured"})
		return
	}

	adapterID, serviceID := c.Param("id"), c.Param("serviceId")
	if err := h.adapterMounts.Unmount(c.Request.Context(), h.tenantScope(c), adapterID, serviceID); err != nil {
		respondAdapterError(c, err)
		return
	}
	h.audit(c, models.ActionUnmount, models.ResAdapter, adapterID, serviceID)
	c.JSON(http.StatusOK, gin.H{"unmounted": adapterID, "service_id": serviceID})
}