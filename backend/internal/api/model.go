package api

import (
	"errors"
	"net/http"

	"fuze-ai-paas/backend/internal/domain/lineage"
	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func notFoundOrError(c *gin.Context, err error, msg string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func (h *Handler) GetModels(c *gin.Context) {
	
	tenant := h.tenantScope(c)
	out, err := h.modelRepo.GetModelsByTenant(tenant)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) GetModel(c *gin.Context) {
	id := c.Param("id")
	m, err := h.modelRepo.GetModel(id)
	if err != nil {
		notFoundOrError(c, err, "model not found")
		return
	}
	
	if !h.canAccessTenant(m.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *Handler) CreateModel(c *gin.Context) {
	var m models.Model
	if err := c.ShouldBindJSON(&m); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if m.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	m.TenantID = tenantID
	if err := h.modelRepo.CreateModel(&m); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, models.ActionCreate, models.ResModel, m.ID, m.Name)
	c.JSON(http.StatusCreated, m)
}

func (h *Handler) UpdateModel(c *gin.Context) {
	id := c.Param("id")
	m, err := h.modelRepo.GetModel(id)
	if err != nil {
		notFoundOrError(c, err, "model not found")
		return
	}
	if !h.canAccessTenant(m.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}
	var patch models.Model
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	if patch.Name != "" {
		m.Name = patch.Name
	}
	if patch.Description != "" {
		m.Description = patch.Description
	}
	if patch.Framework != "" {
		m.Framework = patch.Framework
	}
	if patch.Owner != "" {
		m.Owner = patch.Owner
	}
	if err := h.modelRepo.UpdateModel(m); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *Handler) DeleteModel(c *gin.Context) {
	id := c.Param("id")
	m, err := h.modelRepo.GetModel(id)
	if err != nil {
		notFoundOrError(c, err, "model not found")
		return
	}
	if !h.canAccessTenant(m.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}
	if err := h.modelRepo.DeleteModel(id); err != nil {
		notFoundOrError(c, err, "model not found")
		return
	}
	h.audit(c, models.ActionDelete, models.ResModel, id, "")
	c.JSON(http.StatusNoContent, nil)
}

func (h *Handler) GetModelVersions(c *gin.Context) {
	modelID := c.Param("id")
	m, err := h.modelRepo.GetModel(modelID)
	if err != nil {
		notFoundOrError(c, err, "model not found")
		return
	}
	if !h.canAccessTenant(m.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}
	versions, err := h.modelRepo.GetModelVersions(modelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, versions)
}

func (h *Handler) GetModelVersion(c *gin.Context) {
	modelID := c.Param("id")
	versionID := c.Param("vid")
	v, err := h.modelRepo.GetModelVersion(modelID, versionID)
	if err != nil {
		notFoundOrError(c, err, "model version not found")
		return
	}
	if !h.canAccessTenant(v.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "model version not found"})
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *Handler) GetModelVersionLineage(c *gin.Context) {
	modelID := c.Param("id")
	versionID := c.Param("vid")
	v, err := h.modelRepo.GetModelVersion(modelID, versionID)
	if err != nil {
		notFoundOrError(c, err, "model version not found")
		return
	}
	if !h.canAccessTenant(v.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "model version not found"})
		return
	}

	src := lineage.Source{
		JobID:          v.SourceJobID,
		RunID:          v.SourceRunID,
		CodeRepo:       v.CodeRepo,
		CodeCommit:     v.CodeCommit,
		Image:          v.Image,
		TemplateID:     v.TemplateID,
		DatasetID:      v.DatasetID,
		DatasetName:    v.DatasetName,
		DatasetVersion: v.DatasetVersion,
		Hyperparameters: v.Hyperparameters,
	}

	if src.CodeCommit == "" && src.CodeRepo == "" && v.SourceJobID != "" {
		if job, jerr := h.jobRepo.GetJob(v.SourceJobID); jerr == nil && job != nil {
			src.CodeRepo = job.CodeRepo
			src.CodeCommit = job.CodeCommit
			src.Image = job.Image
			src.TemplateID = job.TemplateID
			if src.DatasetID == "" {
				src.DatasetID = job.DatasetID
			}
			if src.DatasetName == "" {
				src.DatasetName = job.DatasetName
			}
			if src.DatasetVersion == "" {
				src.DatasetVersion = job.DatasetVersion
			}
		}
	}
	if src.Hyperparameters == "" && v.SourceRunID != "" {
		if run, rerr := h.experiment.GetRun(v.SourceRunID); rerr == nil && run != nil {
			src.Hyperparameters = run.Hyperparameters
		}
	}

	g := lineage.BuildGraph(versionID, src)
	c.JSON(http.StatusOK, g)
}

func (h *Handler) CreateModelVersion(c *gin.Context) {
	modelID := c.Param("id")
	m, err := h.modelRepo.GetModel(modelID)
	if err != nil {
		notFoundOrError(c, err, "model not found")
		return
	}
	if !h.canAccessTenant(m.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}
	var v models.ModelVersion
	if err := c.ShouldBindJSON(&v); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if v.Version == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version is required"})
		return
	}
	v.ModelID = modelID
	v.TenantID = m.TenantID
	if err := h.modelRepo.CreateModelVersion(&v); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, v)
}

func (h *Handler) DeleteModelVersion(c *gin.Context) {
	modelID := c.Param("id")
	versionID := c.Param("vid")
	m, err := h.modelRepo.GetModel(modelID)
	if err != nil {
		notFoundOrError(c, err, "model not found")
		return
	}
	if !h.canAccessTenant(m.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}
	if err := h.modelRepo.DeleteModelVersion(modelID, versionID); err != nil {
		notFoundOrError(c, err, "model version not found")
		return
	}
	c.JSON(http.StatusNoContent, nil)
}