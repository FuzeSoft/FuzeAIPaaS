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

type alertRuleInput struct {
	Name        string            `json:"name"`
	Expr        string            `json:"expr"`
	For         string            `json:"for"`
	Severity    string            `json:"severity"`
	Summary     string            `json:"summary"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"labels"`
	Channels    []string          `json:"channels"`
	Enabled     bool              `json:"enabled"`
}

func alertRuleID(tenantID, name string) string {
	return tenantID + ":" + name
}

func (h *Handler) ListAlertRules(c *gin.Context) {
	if h.alert == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "alert repository not configured"})
		return
	}
	rules, err := h.alert.ListRules(h.tenantScope(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rules)
}

func (h *Handler) CreateAlertRule(c *gin.Context) {
	if h.alert == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "alert repository not configured"})
		return
	}
	var in alertRuleInput
	if err := c.ShouldBindJSON(&in); err != nil || in.Name == "" || in.Expr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alert rule body: name and expr required"})
		return
	}
	tenantID := h.tenantScope(c)
	rule := &models.AlertRule{
		ID:          alertRuleID(tenantID, in.Name),
		TenantID:    tenantID,
		Name:        in.Name,
		Expr:        in.Expr,
		For:         in.For,
		Severity:    models.AlertSeverity(in.Severity),
		Summary:     in.Summary,
		Description: in.Description,
		Labels:      in.Labels,
		Channels:    in.Channels,
		Enabled:     in.Enabled,
		CreatedBy:   h.claimsOf(c).UserID,
	}
	if err := h.alert.CreateRule(rule); err != nil {
		if errors.Is(err, ports.ErrAlertRuleInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, rule)
}

func (h *Handler) UpdateAlertRule(c *gin.Context) {
	if h.alert == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "alert repository not configured"})
		return
	}
	id := c.Param("id")
	var in alertRuleInput
	if err := c.ShouldBindJSON(&in); err != nil || in.Name == "" || in.Expr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alert rule body: name and expr required"})
		return
	}
	tenantID := h.tenantScope(c)
	rule := &models.AlertRule{
		ID:          id,
		TenantID:    tenantID,
		Name:        in.Name,
		Expr:        in.Expr,
		For:         in.For,
		Severity:    models.AlertSeverity(in.Severity),
		Summary:     in.Summary,
		Description: in.Description,
		Labels:      in.Labels,
		Channels:    in.Channels,
		Enabled:     in.Enabled,
	}
	if err := h.alert.UpdateRule(rule); err != nil {
		switch {
		case errors.Is(err, ports.ErrAlertRuleNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, ports.ErrAlertRuleInvalid):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *Handler) DeleteAlertRule(c *gin.Context) {
	if h.alert == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "alert repository not configured"})
		return
	}
	id := c.Param("id")
	if err := h.alert.DeleteRule(h.tenantScope(c), id); err != nil {
		if errors.Is(err, ports.ErrAlertRuleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) ToggleAlertRule(c *gin.Context) {
	if h.alert == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "alert repository not configured"})
		return
	}
	id := c.Param("id")
	tenantID := h.tenantScope(c)
	existing, err := h.alert.GetRule(tenantID, id)
	if err != nil {
		if errors.Is(err, ports.ErrAlertRuleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body: enabled required"})
		return
	}
	existing.Enabled = body.Enabled
	if err := h.alert.UpdateRule(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

func (h *Handler) ListActiveAlerts(c *gin.Context) {
	if h.metrics == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "metrics query not configured"})
		return
	}
	alerts, err := h.metrics.Alerts()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("query prometheus alerts: %v", err)})
		return
	}
	
	if scope := h.tenantScope(c); scope != "" {
		filtered := alerts[:0]
		for _, a := range alerts {
			if tid, ok := a.Labels["tenant_id"]; ok && tid == scope {
				filtered = append(filtered, a)
			}
		}
		alerts = filtered
	}
	c.JSON(http.StatusOK, alerts)
}

type silenceInput struct {
	RuleID   string `json:"rule_id"`
	StartsAt int64  `json:"starts_at"` 
	EndsAt   int64  `json:"ends_at"`   
	Comment  string `json:"comment"`
}

func (h *Handler) ListSilences(c *gin.Context) {
	if h.alert == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "alert repository not configured"})
		return
	}
	silences, err := h.alert.ListSilences(h.tenantScope(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, silences)
}

func (h *Handler) CreateSilence(c *gin.Context) {
	if h.alert == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "alert repository not configured"})
		return
	}
	var in silenceInput
	if err := c.ShouldBindJSON(&in); err != nil || in.EndsAt == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid silence body: ends_at required"})
		return
	}
	tenantID := h.tenantScope(c)
	now := time.Now().UTC()
	starts := now
	if in.StartsAt != 0 {
		starts = time.UnixMilli(in.StartsAt).UTC()
	}
	silence := &models.AlertSilence{
		ID:        fmt.Sprintf("%s:%d", tenantID, now.UnixNano()),
		TenantID:  tenantID,
		RuleID:    in.RuleID,
		StartsAt:  starts,
		EndsAt:    time.UnixMilli(in.EndsAt).UTC(),
		Comment:   in.Comment,
		CreatedBy: h.claimsOf(c).UserID,
	}
	if err := h.alert.CreateSilence(silence); err != nil {
		if errors.Is(err, ports.ErrAlertSilenceInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, silence)
}

func (h *Handler) DeleteSilence(c *gin.Context) {
	if h.alert == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "alert repository not configured"})
		return
	}
	id := c.Param("id")
	if err := h.alert.DeleteSilence(h.tenantScope(c), id); err != nil {
		if errors.Is(err, ports.ErrAlertSilenceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}