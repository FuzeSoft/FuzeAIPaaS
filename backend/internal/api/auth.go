package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"time"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token, user, err := h.auth.Login(h.userRepo, req.Username, req.Password)
	if err != nil {
		
		if err == auth.ErrAccountLocked {
			
			h.auditAs(c, req.Username, "", "", "", models.ActionLoginFailed, models.ResAuth, "", req.Username)
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":  "account temporarily locked due to too many failed attempts",
				"locked": true,
			})
			return
		}
		
		h.auditAs(c, req.Username, "", "", "", models.ActionLoginFailed, models.ResAuth, "", req.Username)
		
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}
	h.auditAs(c, user.Username, user.ID, user.Role, user.TenantID, models.ActionLogin, models.ResAuth, user.ID, user.Username)
	
	if !user.MFARequired() {
		h.setSSOCookie(c, "fuze_token", token, true)
	}
	
	if user.MFARequired() {
		c.JSON(http.StatusOK, gin.H{
			"mfa_required": true,
			"mfa_token":    token,
			"user": gin.H{
				"id":           user.ID,
				"username":     user.Username,
				"display_name": user.DisplayName,
				"role":         user.Role,
				"tenant_id":    user.TenantID,
			},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":           user.ID,
			"username":     user.Username,
			"display_name": user.DisplayName,
			"role":         user.Role,
			"tenant_id":    user.TenantID,
		},
	})
}

type mfaVerifyRequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
}

func (h *Handler) VerifyMFA(c *gin.Context) {
	var req mfaVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.MFAToken == "" || req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mfa_token and code required"})
		return
	}
	token, err := h.auth.VerifyMFA(h.userRepo, req.MFAToken, req.Code)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	h.setSSOCookie(c, "fuze_token", token, true)
	c.JSON(http.StatusOK, gin.H{"token": token})
}

func (h *Handler) MFAEnroll(c *gin.Context) {
	principal, ok := auth.Principal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if h.auth.MFAService() == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "mfa not enabled"})
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Code == "" {
		secret, uri, codes, err := h.auth.BeginMFA(h.userRepo, principal.Username, "FuzeAI")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"secret":         secret,
			"otpauth_uri":    uri,
			"recovery_codes": codes,
		})
		return
	}
	if err := h.auth.ConfirmMFA(h.userRepo, principal.Username, req.Code); err != nil {
		
		if errors.Is(err, auth.ErrInvalidMFACode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.auditAs(c, principal.Username, principal.UserID, principal.Role, principal.TenantID,
		models.ActionMFASetup, models.ResAuth, principal.UserID, "mfa enroll confirmed")
	c.JSON(http.StatusOK, gin.H{"enabled": true})
}

func (h *Handler) MFADisable(c *gin.Context) {
	principal, ok := auth.Principal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if err := h.auth.DisableMFA(h.userRepo, principal.Username); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.auditAs(c, principal.Username, principal.UserID, principal.Role, principal.TenantID,
		models.ActionMFADisable, models.ResAuth, principal.UserID, "mfa disabled")
	c.JSON(http.StatusOK, gin.H{"disabled": true})
}

func (h *Handler) Logout(c *gin.Context) {
	
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("fuze_token", "", -1, "/", "", true, true)
	c.SetCookie("fuze_token", "", -1, "/", "", true, false)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) Me(c *gin.Context) {
	claims, ok := auth.Principal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	mfaEnabled := false
	passkeyEnabled := false
	if u, err := h.userRepo.GetUserByID(claims.UserID); err == nil && u != nil {
		mfaEnabled = u.MFAEnabled
		passkeyEnabled = u.PasskeyEnabled
	}
	c.JSON(http.StatusOK, gin.H{
		"id":              claims.UserID,
		"username":        claims.Username,
		"role":            claims.Role,
		"tenant_id":       claims.TenantID,
		"mfa_enabled":     mfaEnabled,
		"passkey_enabled": passkeyEnabled,
	})
}

func (h *Handler) ListSSOProviders(c *gin.Context) {
	providers := []gin.H{}
	if h.sso.Registry != nil {
		all, err := h.sso.Registry.ListEnabled(c.Request.Context())
		if err == nil {
			for _, p := range all {
				entry := gin.H{
					"provider_id": p.ProviderID,
					"type":        string(p.Type),
					"name":        p.Name,
				}
				if p.Type == models.IdPOIDC {
					entry["url"] = "/api/v1/auth/sso/" + p.ProviderID + "/start"
				}
				providers = append(providers, entry)
			}
		}
	}
	
	if len(providers) == 0 && h.sso.OIDC != nil {
		providers = append(providers, gin.H{
			"provider_id": "oidc",
			"type":        "oidc",
			"name":        "企业单点登录 (OIDC)",
			"url":         "/api/v1/auth/sso/oidc/start",
		})
	}
	c.JSON(http.StatusOK, gin.H{"providers": providers})
}

func (h *Handler) OIDCStart(c *gin.Context) {
	providerID := c.Param("provider")
	provider, _, err := h.sso.providerFor(c.Request.Context(), providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "oidc provider not available"})
		return
	}
	
	c.SetSameSite(http.SameSiteLaxMode)
	state := randState()
	secureCookie := h.sso.SecureCookie

	stateKey := "oidc_state_" + providerID
	nonceKey := "oidc_nonce_" + providerID
	c.SetCookie(stateKey, state, 600, "/", "", secureCookie, true)

	verifier := auth.GeneratePKCEVerifier()
	challenge := auth.PKCEChallenge(verifier)
	authURL, nonce := provider.AuthURL(state, challenge)
	provider.StorePKCE(nonce, verifier)
	c.SetCookie(nonceKey, nonce, 600, "/", "", secureCookie, true)
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

func (h *Handler) OIDCCallback(c *gin.Context) {
	providerID := c.Param("provider")
	provider, idpCfg, err := h.sso.providerFor(c.Request.Context(), providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "oidc provider not available"})
		return
	}
	stateKey := "oidc_state_" + providerID
	nonceKey := "oidc_nonce_" + providerID
	c.SetSameSite(http.SameSiteLaxMode)
	state := c.Query("state")
	cookieState, err := c.Cookie(stateKey)
	if err != nil || cookieState == "" || cookieState != state {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid oidc state"})
		return
	}
	c.SetCookie(stateKey, "", -1, "/", "", false, true)

	nonce, err := c.Cookie(nonceKey)
	if err != nil || nonce == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing oidc nonce"})
		return
	}
	c.SetCookie(nonceKey, "", -1, "/", "", false, true)

	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing authorization code"})
		return
	}
	verifier := provider.ConsumePKCE(nonce)
	info, err := provider.Exchange(c.Request.Context(), code, nonce, verifier)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "oidc exchange failed: " + err.Error()})
		return
	}
	
	roleCfg := h.sso.OIDCConfig.Role
	defTenant := h.sso.OIDCConfig.DefaultTenant
	if idpCfg != nil {
		roleCfg = auth.SSORoleConfig{
			DefaultRole: idpCfg.DefaultRole,
			AdminGroups: idpCfg.AdminGroups,
			AdminRole:   idpCfg.AdminRole,
		}
		if idpCfg.DefaultTenant != "" {
			defTenant = idpCfg.DefaultTenant
		}
	}
	token, user, err := h.auth.SSOLogin(h.userRepo, *info, roleCfg, defTenant)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.auditAs(c, user.Username, user.ID, user.Role, user.TenantID, models.ActionLogin, models.ResAuth, user.ID, "oidc:sso:"+providerID)

	frontend := h.sso.FrontendURL
	if frontend == "" {
		frontend = "/"
	}
	if user.MFARequired() {
		
		h.setSSOCookie(c, "fuze_token", token, false)
		target := frontend + "/login?mfa_required=true&mfa_token=" + url.QueryEscape(token) + "&sso_user=" + url.QueryEscape(user.Username)
		c.Redirect(http.StatusTemporaryRedirect, target)
		return
	}
	
	h.setSSOCookie(c, "fuze_token", token, true)
	target := frontend + "/login?sso_user=" + url.QueryEscape(user.Username)
	c.Redirect(http.StatusTemporaryRedirect, target)
}

func (h *Handler) setSSOCookie(c *gin.Context, name, value string, httpOnly bool) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, 600, "/", "", true, httpOnly)
}

func (h *Handler) LDAPLogin(c *gin.Context) {
	providerID := c.Param("provider")
	cfg, idpCfg, err := h.sso.ldapConfigFor(c.Request.Context(), providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ldap provider not available"})
		return
	}
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	info, err := auth.LDAPLogin(*cfg, req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	roleCfg := auth.SSORoleConfig{
		DefaultRole: h.sso.LDAP.DefaultRole,
		AdminGroups: h.sso.LDAP.AdminGroups,
		AdminRole:   h.sso.LDAP.AdminRole,
	}
	defTenant := h.sso.LDAP.DefaultTenant
	if idpCfg != nil {
		roleCfg = auth.SSORoleConfig{
			DefaultRole: idpCfg.DefaultRole,
			AdminGroups: idpCfg.AdminGroups,
			AdminRole:   idpCfg.AdminRole,
		}
		if idpCfg.DefaultTenant != "" {
			defTenant = idpCfg.DefaultTenant
		}
	}
	token, user, err := h.auth.SSOLogin(h.userRepo, info, roleCfg, defTenant)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.auditAs(c, user.Username, user.ID, user.Role, user.TenantID, models.ActionLogin, models.ResAuth, user.ID, "ldap:sso:"+providerID)
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":           user.ID,
			"username":     user.Username,
			"display_name": user.DisplayName,
			"role":         user.Role,
			"tenant_id":    user.TenantID,
		},
	})
}

func randState() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}