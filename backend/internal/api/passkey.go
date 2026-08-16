package api

import (
	"net/http"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) PasskeyRegisterBegin(c *gin.Context) {
	principal, ok := auth.Principal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	pk := h.auth.PasskeyService()
	if pk == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "passkey not enabled"})
		return
	}
	u, err := h.userRepo.GetUserByUsername(principal.Username)
	if err != nil || u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	creation, err := pk.BeginRegistration(u)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, creation)
}

func (h *Handler) PasskeyRegisterFinish(c *gin.Context) {
	principal, ok := auth.Principal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	pk := h.auth.PasskeyService()
	if pk == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "passkey not enabled"})
		return
	}
	u, err := h.userRepo.GetUserByUsername(principal.Username)
	if err != nil || u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	passkeysEnc, enabled, err := pk.FinishRegistration(u, c.Request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	u.Passkeys = passkeysEnc
	u.PasskeyEnabled = enabled
	if err := h.userRepo.UpdateUser(u); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist passkey"})
		return
	}
	h.audit(c, models.ActionPasskeyRegister, models.ResUser, u.ID, "registered a passkey")
	c.JSON(http.StatusOK, gin.H{"passkey_enabled": enabled})
}

func (h *Handler) PasskeyLoginBegin(c *gin.Context) {
	var req mfaVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.MFAToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mfa_token required"})
		return
	}
	claims, err := h.auth.Validate(req.MFAToken)
	if err != nil || claims.Scope != "mfa" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired passkey challenge"})
		return
	}
	pk := h.auth.PasskeyService()
	if pk == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "passkey not enabled"})
		return
	}
	u, err := h.userRepo.GetUserByID(claims.UserID)
	if err != nil || u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	assertion, err := pk.BeginLogin(u)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, assertion)
}

func (h *Handler) PasskeyLoginFinish(c *gin.Context) {
	var req mfaVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.MFAToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mfa_token required"})
		return
	}
	token, err := h.auth.VerifyPasskey(h.userRepo, req.MFAToken, c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

func (h *Handler) PasskeyDisable(c *gin.Context) {
	principal, ok := auth.Principal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if h.auth.PasskeyService() == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "passkey not enabled"})
		return
	}
	if err := h.auth.ClearPasskeys(h.userRepo, principal.Username); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, models.ActionPasskeyRegister, models.ResUser, principal.UserID, "disabled passkey")
	c.JSON(http.StatusOK, gin.H{"passkey_enabled": false})
}