package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"github.com/gin-gonic/gin"
)

type guardrailRuleInput struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Direction   string `json:"direction"`
	Action      string `json:"action"`
	Pattern     string `json:"pattern"`
	Keywords    string `json:"keywords"`
	Replacement string `json:"replacement"`
	Enabled     bool   `json:"enabled"`
}

func (h *Handler) ListGuardrailRules(c *gin.Context) {
	if h.guardrail == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "guardrail repository not configured"})
		return
	}
	rules, err := h.guardrail.List(c.Request.Context(), h.tenantScope(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rules)
}

func (h *Handler) UpsertGuardrailRule(c *gin.Context) {
	if h.guardrail == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "guardrail repository not configured"})
		return
	}
	var in guardrailRuleInput
	if err := c.ShouldBindJSON(&in); err != nil || in.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guardrail rule body: name required"})
		return
	}

	tenantID := h.tenantScope(c)
	rule := &models.LLMGuardrailRule{
		
		ID:          guardrailRuleID(tenantID, in.Name),
		TenantID:    tenantID,
		Name:        in.Name,
		Category:    in.Category,
		Direction:   in.Direction,
		Action:      in.Action,
		Pattern:     in.Pattern,
		Keywords:    in.Keywords,
		Replacement: in.Replacement,
		Enabled:     in.Enabled,
		CreatedBy:   h.claimsOf(c).UserID,
		UpdatedAt:   time.Now().UTC(),
	}

	if err := h.guardrail.Upsert(c.Request.Context(), rule); err != nil {
		if errors.Is(err, ports.ErrGuardrailInvalidRule) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateGuardCache(tenantID)
	h.audit(c, "upsert", "llm_guardrail_rule", rule.ID, rule.Name)
	c.JSON(http.StatusOK, rule)
}

func (h *Handler) DeleteGuardrailRule(c *gin.Context) {
	if h.guardrail == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "guardrail repository not configured"})
		return
	}
	tenantID := h.tenantScope(c)
	id := c.Param("id")

	if err := h.guardrail.Delete(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, ports.ErrGuardrailRuleNotFound) {
			
			c.JSON(http.StatusNotFound, gin.H{"error": "guardrail rule not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateGuardCache(tenantID)
	h.audit(c, "delete", "llm_guardrail_rule", id, "")
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (h *Handler) invalidateGuardCache(tenantID string) {
	if h.guardCache == nil {
		return
	}
	if tenantID == "" {
		h.guardCache.InvalidateAll()
		return
	}
	h.guardCache.Invalidate(tenantID)
}

func guardrailRuleID(tenantID, name string) string {
	if tenantID == "" {
		return fmt.Sprintf("gr-global-%s", name)
	}
	return fmt.Sprintf("gr-%s-%s", tenantID, name)
}