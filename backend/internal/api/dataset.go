package api

import (
	"errors"
	"log"
	"net/http"

	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handler) GetDatasets(c *gin.Context) {
	
	datasets, err := h.datasetRepo.GetDatasetsByTenant(h.tenantScope(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, datasets)
}

func (h *Handler) GetDataset(c *gin.Context) {
	ds, ok := h.lookupDataset(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, ds)
}

func (h *Handler) lookupDataset(c *gin.Context) (*models.Dataset, bool) {
	ds, err := h.datasetRepo.GetDatasetForTenant(h.tenantScope(c), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Dataset not found"})
		return nil, false
	}
	return ds, true
}

func (h *Handler) CreateDataset(c *gin.Context) {
	var ds models.Dataset
	if err := c.ShouldBindJSON(&ds); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if ds.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if ds.ClusterID == "" {
		ds.ClusterID = "cluster-001"
	}
	if ds.Runtime == "" {
		ds.Runtime = models.RuntimeAlluxio
	}
	if ds.Replicas == 0 {
		ds.Replicas = 1
	}
	if ds.CacheMedium == "" {
		ds.CacheMedium = models.CacheMediumMEM
	}
	if ds.AccessMode == "" {
		ds.AccessMode = "ReadOnly"
	}

	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	ds.TenantID = tenantID

	if err := h.datasetRepo.CreateDataset(&ds); err != nil {
		log.Printf("[dataset] CreateDataset failed for tenant %s name %s: %v", ds.TenantID, ds.Name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.audit(c, models.ActionCreate, "dataset", ds.ID, ds.Name)

	if err := h.scheduler.CreateDataset(&ds); err != nil {
		log.Printf("[Handler] Fluid dataset create failed for %s: %v", ds.ID, err)
	}

	c.JSON(http.StatusCreated, ds)
}

func (h *Handler) DeleteDataset(c *gin.Context) {
	
	ds, ok := h.lookupDataset(c)
	if !ok {
		return
	}

	if err := h.scheduler.DeleteDataset(ds); err != nil {
		
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Dataset not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.audit(c, models.ActionDelete, "dataset", ds.ID, ds.Name)

	c.JSON(http.StatusNoContent, nil)
}