package api

import (
	"errors"
	"log"
	"net/http"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handler) GetInferenceServices(c *gin.Context) {
	
	svcs, err := h.inferenceRepo.GetInferenceServicesByTenant(h.tenantScope(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toInferenceViews(svcs))
}

func (h *Handler) GetInferenceService(c *gin.Context) {
	svc, ok := h.lookupInferenceService(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, toInferenceView(svc))
}

func (h *Handler) lookupInferenceService(c *gin.Context) (*models.InferenceService, bool) {
	svc, err := h.inferenceRepo.GetInferenceServiceForTenant(h.tenantScope(c), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "InferenceService not found"})
		return nil, false
	}
	return svc, true
}

func (h *Handler) CreateInferenceService(c *gin.Context) {
	spec, ok := h.bindInferenceSpec(c)
	if !ok {
		return
	}
	svc, code, err := h.createFromSpec(c, spec)
	if err != nil {
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, models.ActionCreate, models.ResInference, svc.ID, svc.Name)
	c.JSON(http.StatusCreated, toInferenceView(svc))
}

func (h *Handler) ApplyInferenceService(c *gin.Context) {
	spec, ok := h.bindInferenceSpec(c)
	if !ok {
		return
	}
	tenant, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}

	existing, err := h.inferenceRepo.GetInferenceServiceByName(tenant, spec.Name)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		svc, code, cerr := h.createFromSpec(c, spec)
		if cerr != nil {
			c.JSON(code, gin.H{"error": cerr.Error()})
			return
		}
		h.audit(c, models.ActionCreate, models.ResInference, svc.ID, svc.Name)
		c.JSON(http.StatusCreated, toInferenceView(svc))
		return
	}

	old := *existing 
	oldGPUs, oldMemGB := existing.GPUs, svcMemGB(existing)
	spec.applyTo(existing)
	newMemGB := svcMemGB(existing)
	if deploymentChanged(&old, existing) {
		existing.KServeName = "" 
	}

	if err := h.inferenceRepo.ApplySpec(tenant, existing, oldGPUs, oldMemGB, existing.GPUs, newMemGB); err != nil {
		if errors.Is(err, ports.ErrQuotaExceeded) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	h.audit(c, models.ActionUpdate, models.ResInference, existing.ID, "declarative apply")
	c.JSON(http.StatusOK, toInferenceView(existing))
}

func (h *Handler) PatchInferenceService(c *gin.Context) {
	id := c.Param("id")
	var req inferencePatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := req.Spec.rejectImmutable(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := req.Spec.validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	svc, ok := h.lookupInferenceService(c)
	if !ok {
		return
	}

	old := *svc 
	req.Spec.applyTo(svc)
	if deploymentChanged(&old, svc) {
		svc.KServeName = "" 
	}
	if svc.MaxReplicas < svc.MinReplicas {
		c.JSON(http.StatusBadRequest, gin.H{"error": "spec.max_replicas must be >= spec.min_replicas"})
		return
	}
	if err := h.inferenceRepo.UpdateInferenceServiceSpec(svc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.audit(c, models.ActionUpdate, models.ResInference, id, "declarative patch")
	c.JSON(http.StatusOK, toInferenceView(svc))
}

func (h *Handler) DeleteInferenceService(c *gin.Context) {
	id := c.Param("id")
	svc, ok := h.lookupInferenceService(c)
	if !ok {
		return
	}

	tenant := svc.TenantID
	if tenant == "" {
		tenant = "default"
	}
	memGB := svcMemGB(svc)

	_ = h.scheduler.UndeployInferenceService(c.Request.Context(), svc)
	if err := h.inferenceRepo.DeleteInferenceServiceAndReleaseQuota(id, tenant, svc.GPUs, memGB); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, models.ActionDelete, models.ResInference, id, svc.Name)
	c.JSON(http.StatusNoContent, nil)
}

func (h *Handler) bindInferenceSpec(c *gin.Context) (*inferenceSpec, bool) {
	var req inferenceApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, false
	}
	req.Spec.normalize()
	if err := req.Spec.validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, false
	}
	return &req.Spec, true
}

func (h *Handler) createFromSpec(c *gin.Context, spec *inferenceSpec) (*models.InferenceService, int, error) {
	tenant, ok := h.requireWriteTenant(c)
	if !ok {
		return nil, http.StatusBadRequest, errors.New("missing tenant context")
	}

	svc := &models.InferenceService{}
	spec.applyTo(svc)
	svc.TenantID = tenant
	svc.Status = models.InferenceStatusPending

	memGB := svcMemGB(svc)
	if err := h.quotaRepo.CheckAndReserve(tenant, svc.GPUs, memGB, 1); err != nil {
		return nil, http.StatusConflict, err
	}
	if err := h.inferenceRepo.CreateInferenceService(svc); err != nil {
		
		if relErr := h.quotaRepo.Release(tenant, svc.GPUs, memGB, 1); relErr != nil {
			log.Printf("inference: failed to release quota after create failure for tenant %s svc %s: %v", tenant, svc.ID, relErr)
		}
		return nil, http.StatusInternalServerError, err
	}
	return svc, http.StatusCreated, nil
}

func svcMemGB(svc *models.InferenceService) int {
	if svc.GPUMemory > 0 {
		return int(max(int64(1), int64(svc.GPUMemory)*int64(svc.GPUs)/1024))
	}
	if svc.GPUs > 0 {
		return int(max(int64(40), int64(svc.GPUs)*40))
	}
	return 1
}