package api

import (
	"net/http"

	"fuze-ai-paas/backend/internal/models"
	"github.com/gin-gonic/gin"
)

func (h *Handler) ListLLMPrices(c *gin.Context) {
	if h.priceRepo == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "price repository not configured"})
		return
	}
	rows, err := h.priceRepo.ListLLMPrices(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"prices": rows})
}

func (h *Handler) UpsertLLMPrice(c *gin.Context) {
	if h.priceRepo == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "price repository not configured"})
		return
	}
	var p models.LLMPrice
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid price body"})
		return
	}
	if p.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model is required"})
		return
	}
	if p.InputPer1K < 0 || p.OutputPer1K < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "price must not be negative"})
		return
	}
	if p.Currency == "" {
		p.Currency = "CNY"
	}
	if err := h.priceRepo.SaveLLMPrice(c.Request.Context(), p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) DeleteLLMPrice(c *gin.Context) {
	if h.priceRepo == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "price repository not configured"})
		return
	}
	if err := h.priceRepo.DeleteLLMPrice(c.Request.Context(), c.Param("model")); err != nil {
		respondLLMError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) ListGPUPrices(c *gin.Context) {
	if h.priceRepo == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "price repository not configured"})
		return
	}
	rows, err := h.priceRepo.ListGPUPrices(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"prices": rows})
}

func (h *Handler) UpsertGPUPrice(c *gin.Context) {
	if h.priceRepo == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "price repository not configured"})
		return
	}
	var p models.GPUPrice
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid price body"})
		return
	}
	if p.PricePerGPUHour < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "price must not be negative"})
		return
	}
	if p.Currency == "" {
		p.Currency = "CNY"
	}
	if err := h.priceRepo.SaveGPUPrice(c.Request.Context(), p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) DeleteGPUPrice(c *gin.Context) {
	if h.priceRepo == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "price repository not configured"})
		return
	}
	if err := h.priceRepo.DeleteGPUPrice(c.Request.Context(), c.Param("gpuType")); err != nil {
		respondLLMError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}