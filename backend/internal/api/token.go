package api

import (
	"errors"
	"net/http"
	"time"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"github.com/gin-gonic/gin"
)

func (h *Handler) principalUser(c *gin.Context) *models.User {
	claims, _ := auth.Principal(c)
	return &models.User{
		ID:       claims.UserID,
		Username: claims.Username,
		Role:     claims.Role,
		TenantID: claims.TenantID,
	}
}

func (h *Handler) CreateToken(c *gin.Context) {
	if h.token == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "token service unavailable"})
		return
	}
	var body struct {
		Name     string `json:"name"`
		TTLHours int    `json:"ttl_hours"` 
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	ttl := time.Duration(body.TTLHours) * time.Hour
	if body.TTLHours <= 0 {
		ttl = 0 
	}
	issued, err := h.token.Issue(c.Request.Context(), h.principalUser(c), body.Name, ttl)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, issued)
}

func (h *Handler) ListTokens(c *gin.Context) {
	if h.token == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "token service unavailable"})
		return
	}
	list, err := h.token.List(c.Request.Context(), h.principalUser(c).ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tokens": list})
}

func (h *Handler) DeleteToken(c *gin.Context) {
	if h.token == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "token service unavailable"})
		return
	}
	id := c.Param("id")
	
	tok, err := h.token.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !h.isPlatformAdmin(c) && tok.UserID != h.principalUser(c).ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}
	if err := h.token.Revoke(c.Request.Context(), id); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (h *Handler) RotateToken(c *gin.Context) {
	if h.token == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "token service unavailable"})
		return
	}
	id := c.Param("id")
	
	tok, err := h.token.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !h.isPlatformAdmin(c) && tok.UserID != h.principalUser(c).ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}

	issued, err := h.token.Rotate(c.Request.Context(), id, tok.UserID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	actor := h.principalUser(c)
	h.auditAs(c, actor.Username, actor.ID, actor.Role, actor.TenantID,
		models.ActionTokenRotate, models.ResAuth, tok.ID,
		"rotated to "+issued.ID)

	c.JSON(http.StatusOK, issued)
}