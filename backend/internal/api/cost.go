package api

import (
	"errors"
	"net/http"

	"fuze-ai-paas/backend/internal/ports"
	"github.com/gin-gonic/gin"
)

const defaultCurrency = "CNY"

type CostSummary struct {
	TenantID      string  `json:"tenant_id"`
	LLMCost       float64 `json:"llm_cost"`
	GPUCost       float64 `json:"gpu_cost"`
	TotalCost     float64 `json:"total_cost"`
	LimitCost     float64 `json:"limit_cost"`
	UsedCost      float64 `json:"used_cost"`
	RemainingCost float64 `json:"remaining_cost"`
	
	Ratio    float64 `json:"ratio"`
	Currency string  `json:"currency"`
	
	Degraded bool `json:"degraded,omitempty"`
}

func (h *Handler) CostSummary(c *gin.Context) {
	tenant := h.principalTenant(c)
	var since, until int64
	parseInt64Query(c.Query("since"), &since)
	parseInt64Query(c.Query("until"), &until)

	if h.llmUsage == nil && h.costRepo == nil {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":    "cost tracking is not configured on this deployment",
			"degraded": true,
		})
		return
	}

	var sum CostSummary
	sum.TenantID = tenant
	sum.Currency = defaultCurrency

	if h.llmUsage != nil {
		_, llmCost, err := h.llmUsage.SumUsage(c.Request.Context(), tenant, since, until)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		sum.LLMCost = llmCost
	} else {
		sum.Degraded = true
	}
	if h.costRepo != nil {
		_, gpuCost, err := h.costRepo.SumGPUUsage(c.Request.Context(), tenant, since, until)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		sum.GPUCost = gpuCost
		q, err := h.costRepo.GetQuota(c.Request.Context(), tenant)
		if err != nil {
			
			if !errors.Is(err, ports.ErrNotFound) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		} else {
			sum.LimitCost = q.LimitCost
			sum.UsedCost = q.UsedCost
			sum.RemainingCost = q.LimitCost - q.UsedCost
			if q.LimitCost > 0 {
				sum.Ratio = q.UsedCost / q.LimitCost
			}
		}
	} else {
		sum.Degraded = true
	}

	sum.TotalCost = sum.LLMCost + sum.GPUCost
	c.JSON(http.StatusOK, sum)
}