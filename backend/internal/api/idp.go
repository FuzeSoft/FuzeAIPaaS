package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"github.com/gin-gonic/gin"
)

var idpProbeClient = &http.Client{
	Timeout: 8 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 20,
	},
}

type idpCreateRequest struct {
	ProviderID       string   `json:"provider_id"`
	Type             string   `json:"type"`
	Name             string   `json:"name"`
	Enabled          bool     `json:"enabled"`
	Issuer           string   `json:"issuer"`
	ClientID         string   `json:"client_id"`
	ClientSecret     string   `json:"client_secret,omitempty"`
	RedirectURI      string   `json:"redirect_uri,omitempty"`
	Scopes           string   `json:"scopes,omitempty"`
	LDAPAddr         string   `json:"ldap_addr,omitempty"`
	LDAPUseTLS       bool     `json:"ldap_use_tls,omitempty"`
	LDAPSkipVerify   bool     `json:"ldap_skip_verify,omitempty"`
	LDAPUserDNFormat string   `json:"ldap_user_dn_format,omitempty"`
	DefaultRole      string   `json:"default_role,omitempty"`
	AdminGroups      []string `json:"admin_groups,omitempty"`
	AdminRole        string   `json:"admin_role,omitempty"`
	DefaultTenant    string   `json:"default_tenant,omitempty"`
}

func (h *Handler) ListIdPs(c *gin.Context) {
	if !h.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "platform admin required"})
		return
	}
	if h.sso.Registry == nil {
		c.JSON(http.StatusOK, gin.H{"idps": []models.IdPConfig{}})
		return
	}
	list, err := h.sso.Registry.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]models.IdPConfig, 0, len(list))
	for _, p := range list {
		p.ClientSecret = "" 
		out = append(out, p)
	}
	c.JSON(http.StatusOK, gin.H{"idps": out})
}

func (h *Handler) CreateIdP(c *gin.Context) {
	if !h.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "platform admin required"})
		return
	}
	if h.sso.Registry == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "idp registry unavailable"})
		return
	}
	var req idpCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg := toIdPConfig(req)
	if cfg.ProviderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider_id required"})
		return
	}
	if !cfg.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid idp config: missing required fields for type " + string(cfg.Type)})
		return
	}
	if err := h.sso.Registry.Create(c.Request.Context(), &cfg); err != nil {
		if errors.Is(err, ports.ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "provider_id already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	masked := cfg
	masked.ClientSecret = ""
	actor := h.principalUser(c)
	h.auditAs(c, actor.Username, actor.ID, actor.Role, actor.TenantID,
		models.ActionIdPCreate, models.ResAuth, cfg.ProviderID, "idp create "+string(cfg.Type))
	c.JSON(http.StatusOK, masked)
}

func (h *Handler) UpdateIdP(c *gin.Context) {
	if !h.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "platform admin required"})
		return
	}
	if h.sso.Registry == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "idp registry unavailable"})
		return
	}
	providerID := c.Param("id")
	var req idpCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg := toIdPConfig(req)
	cfg.ProviderID = providerID 
	if !cfg.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid idp config: missing required fields for type " + string(cfg.Type)})
		return
	}
	if err := h.sso.Registry.Update(c.Request.Context(), &cfg); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "idp not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	masked := cfg
	masked.ClientSecret = ""
	actor := h.principalUser(c)
	h.auditAs(c, actor.Username, actor.ID, actor.Role, actor.TenantID,
		models.ActionIdPUpdate, models.ResAuth, providerID, "idp update "+string(cfg.Type))
	c.JSON(http.StatusOK, masked)
}

func (h *Handler) DeleteIdP(c *gin.Context) {
	if !h.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "platform admin required"})
		return
	}
	if h.sso.Registry == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "idp registry unavailable"})
		return
	}
	providerID := c.Param("id")
	if err := h.sso.Registry.Delete(c.Request.Context(), providerID); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "idp not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	actor := h.principalUser(c)
	h.auditAs(c, actor.Username, actor.ID, actor.Role, actor.TenantID,
		models.ActionIdPDelete, models.ResAuth, providerID, "idp delete")
	c.JSON(http.StatusOK, gin.H{"deleted": providerID})
}

func toIdPConfig(req idpCreateRequest) models.IdPConfig {
	return models.IdPConfig{
		ProviderID:       req.ProviderID,
		Type:             models.IdPType(req.Type),
		Name:             req.Name,
		Enabled:          req.Enabled,
		Issuer:           req.Issuer,
		ClientID:         req.ClientID,
		ClientSecret:     req.ClientSecret,
		RedirectURI:      req.RedirectURI,
		Scopes:           req.Scopes,
		LDAPAddr:         req.LDAPAddr,
		LDAPUseTLS:       req.LDAPUseTLS,
		LDAPSkipVerify:   req.LDAPSkipVerify,
		LDAPUserDNFormat: req.LDAPUserDNFormat,
		DefaultRole:      models.Role(req.DefaultRole),
		AdminGroups:      req.AdminGroups,
		AdminRole:        models.Role(req.AdminRole),
		DefaultTenant:    req.DefaultTenant,
	}
}

func (h *Handler) TestIdP(c *gin.Context) {
	if !h.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "platform admin required"})
		return
	}
	if h.sso.Registry == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "idp registry unavailable"})
		return
	}
	providerID := c.Param("id")
	cfg, err := h.sso.Registry.Get(c.Request.Context(), providerID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "idp not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	switch cfg.Type {
	case models.IdPOIDC:
		if cfg.Issuer == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "issuer 为空，无法探测", "ok": false})
			return
		}
		
		discovery := fmt.Sprintf("%s/.well-known/openid-configuration", cfg.Issuer)
		if u, perr := url.Parse(cfg.Issuer); perr == nil && u.Scheme == "" {
			discovery = fmt.Sprintf("https://%s/.well-known/openid-configuration", cfg.Issuer)
		} else if perr == nil {
			discovery = u.String() + "/.well-known/openid-configuration"
		}
		du, perr := url.Parse(discovery)
		if perr != nil || (du.Scheme != "https" && du.Scheme != "http") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "issuer 必须使用 http(s) 协议", "ok": false})
			return
		}
		host := du.Hostname()
		if h, _, serr := net.SplitHostPort(du.Host); serr == nil {
			host = h
		}
		if err := assertReachablePublicHost(host); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "issuer 指向非可信地址，已拒绝", "ok": false})
			return
		}
		resp, derr := idpProbeClient.Get(discovery)
		if derr != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "detail": "无法访问 discovery 文档"})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			c.JSON(http.StatusOK, gin.H{"ok": false, "detail": fmt.Sprintf("discovery 返回 %d", resp.StatusCode)})
			return
		}
		type disc struct {
			AuthorizationEndpoint string `json:"authorization_endpoint"`
		}
		var d disc
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&d); err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "detail": "discovery 文档解析失败"})
			return
		}
		if d.AuthorizationEndpoint == "" {
			c.JSON(http.StatusOK, gin.H{"ok": false, "detail": "discovery 缺少 authorization_endpoint"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "detail": "discovery 文档可达且包含授权端点"})
	case models.IdPLDAP:
		if cfg.LDAPAddr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ldap_addr 为空，无法探测", "ok": false})
			return
		}
		
		host := cfg.LDAPAddr
		if h, _, serr := net.SplitHostPort(cfg.LDAPAddr); serr == nil {
			host = h
		}
		if err := assertReachablePublicHost(host); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ldap_addr 指向非可信地址，已拒绝", "ok": false})
			return
		}
		conn, lerr := ldapDialGuarded(cfg.LDAPAddr, 5*time.Second)
		if lerr != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "detail": "无法连接 LDAP"})
			return
		}
		conn.Close()
		c.JSON(http.StatusOK, gin.H{"ok": true, "detail": "TCP 连接成功 (" + cfg.LDAPAddr + ")"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持探测的类型: " + string(cfg.Type), "ok": false})
	}
}

func assertReachablePublicHost(host string) error {
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS 解析失败: %w", err)
	}
	for _, ip := range ips {
		if ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("地址 %s 为内网/保留地址", ip.String())
		}
	}
	return nil
}

func ldapDialGuarded(addr string, timeout time.Duration) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}
	for _, ip := range ips {
		if ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return nil, fmt.Errorf("address %s is private/reserved", ip.String())
		}
	}
	var d net.Dialer
	return d.DialContext(context.Background(), "tcp", addr)
}