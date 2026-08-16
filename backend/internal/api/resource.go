package api

import (
	"net/http"

	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) GetResources(c *gin.Context) {
	clusterID := c.Query("cluster_id")
	var resources []models.Resource
	var err error
	if clusterID != "" {
		resources, err = h.resourceRepo.GetResourcesByCluster(clusterID)
	} else {
		resources, err = h.resourceRepo.GetResources()
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resources)
}

func (h *Handler) GetResource(c *gin.Context) {
	id := c.Param("id")
	resource, err := h.resourceRepo.GetResource(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Resource not found"})
		return
	}
	c.JSON(http.StatusOK, resource)
}

func (h *Handler) CreateResource(c *gin.Context) {
	
	if !h.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "platform admin required"})
		return
	}
	var resource models.Resource
	if err := c.ShouldBindJSON(&resource); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	if resource.Name == "" || resource.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and type are required"})
		return
	}
	if resource.TotalGPUs < 0 || resource.UsedGPUs < 0 || resource.TotalMemory < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resource counts must be non-negative"})
		return
	}

	if err := h.resourceRepo.CreateResource(&resource); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create resource"})
		return
	}

	c.JSON(http.StatusCreated, resource)
}