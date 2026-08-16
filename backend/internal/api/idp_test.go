package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"fuze-ai-paas/backend/internal/storage"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func newIdPTestHandler(t *testing.T) (*Handler, ports.IdPRegistry, *auth.Manager) {
	t.Helper()
	f, err := os.CreateTemp("", "fuze-idp-*.db")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	_ = f.Close()
	_ = os.Remove(path)
	db, err := storage.NewSQLiteDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	audit := &memAudit{}
	registry := storage.NewIdPRegistry(db, nil)
	authMgr := auth.NewManager()
	h := NewHandler(Repos{Audit: audit}, nil, nil, authMgr, nil)
	h.sso = SSOConfig{Registry: registry, SecureCookie: false}
	return h, registry, authMgr
}

func TestIdP_CRUD_AdminOnly(t *testing.T) {
	h, _, authMgr := newIdPTestHandler(t)
	router := gin.New()
	
	userClaims := &auth.Claims{UserID: "u-1", Role: models.RoleDeveloper, TenantID: "t-1"}
	RegisterRoutes(router, h, authMgr, SSOConfig{}, true, nil)

	body := map[string]interface{}{
		"provider_id": "okta", "type": "oidc", "name": "Okta", "enabled": true,
		"issuer": "https://okta.example.com", "client_id": "cid", "client_secret": "sec",
	}
	w := doAuthJSON(t, authMgr, userClaims, router, http.MethodPost, "/api/v1/sso/idps", body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestIdP_CRUD_Full(t *testing.T) {
	h, _, authMgr := newIdPTestHandler(t)
	router := gin.New()
	admin := &auth.Claims{UserID: "admin-1", Username: "root", Role: models.RolePlatformAdmin, TenantID: "t-1"}
	RegisterRoutes(router, h, authMgr, SSOConfig{}, true, nil)

	createBody := map[string]interface{}{
		"provider_id": "okta", "type": "oidc", "name": "Okta", "enabled": true,
		"issuer": "https://okta.example.com", "client_id": "cid", "client_secret": "topsecret",
		"scopes": "openid email",
	}
	w := doAuthJSON(t, authMgr, admin, router, http.MethodPost, "/api/v1/sso/idps", createBody)
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var created models.IdPConfig
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ClientSecret != "" {
		t.Fatalf("client_secret must be masked in response, got %q", created.ClientSecret)
	}

	w = doAuthJSON(t, authMgr, admin, router, http.MethodPost, "/api/v1/sso/idps", createBody)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate expected 409, got %d body=%s", w.Code, w.Body.String())
	}

	w = doAuthJSON(t, authMgr, admin, router, http.MethodGet, "/api/v1/sso/idps", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d", w.Code)
	}
	var list struct {
		IdPs []models.IdPConfig `json:"idps"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.IdPs) != 1 || list.IdPs[0].ClientSecret != "" {
		t.Fatalf("list must have 1 masked idp, got %+v", list.IdPs)
	}

	updBody := map[string]interface{}{
		"type": "oidc", "name": "Okta Prod", "enabled": false,
		"issuer": "https://okta.example.com", "client_id": "cid",
	}
	w = doAuthJSON(t, authMgr, admin, router, http.MethodPut, "/api/v1/sso/idps/okta", updBody)
	if w.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", w.Code, w.Body.String())
	}
	var upd models.IdPConfig
	_ = json.Unmarshal(w.Body.Bytes(), &upd)
	if upd.Enabled || upd.Name != "Okta Prod" {
		t.Fatalf("update not applied: %+v", upd)
	}

	w = doAuthJSON(t, authMgr, admin, router, http.MethodDelete, "/api/v1/sso/idps/okta", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d", w.Code)
	}

	w = doAuthJSON(t, authMgr, admin, router, http.MethodGet, "/api/v1/sso/idps/okta", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get-after-delete expected 404, got %d", w.Code)
	}
}

func TestIdP_Create_Validation(t *testing.T) {
	h, _, authMgr := newIdPTestHandler(t)
	router := gin.New()
	admin := &auth.Claims{UserID: "admin-1", Role: models.RolePlatformAdmin, TenantID: "t-1"}
	RegisterRoutes(router, h, authMgr, SSOConfig{}, true, nil)

	bad := map[string]interface{}{
		"provider_id": "x", "type": "oidc", "name": "X",
	}
	w := doAuthJSON(t, authMgr, admin, router, http.MethodPost, "/api/v1/sso/idps", bad)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid oidc expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestListSSOProvidersEmptyRegistry(t *testing.T) {
	h, _, authMgr := newIdPTestHandler(t)
	router := gin.New()
	RegisterRoutes(router, h, authMgr, SSOConfig{}, true, nil)
	w := doJSON(router, http.MethodGet, "/api/v1/auth/sso", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("providers: %d", w.Code)
	}
	var out struct {
		Providers []map[string]interface{} `json:"providers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Providers) != 0 {
		t.Fatalf("empty registry must yield 0 providers, got %d", len(out.Providers))
	}
}

func TestIdPTestEndpoint(t *testing.T) {
	h, registry, authMgr := newIdPTestHandler(t)
	router := gin.New()
	admin := &auth.Claims{UserID: "admin-2", Role: models.RolePlatformAdmin, TenantID: "t-1"}
	RegisterRoutes(router, h, authMgr, SSOConfig{}, true, nil)
	_ = registry 

	ctx := context.Background()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			_ = c.Close()
		}
	}()
	ldapAddr := ln.Addr().String()
	if err := registry.Create(ctx, &models.IdPConfig{
		ProviderID: "ldap1", Type: models.IdPLDAP, Name: "LDAP1",
		LDAPAddr: ldapAddr, DefaultRole: models.RoleDeveloper,
	}); err != nil {
		t.Fatal(err)
	}
	w := doAuthJSON(t, authMgr, admin, router, http.MethodPost, "/api/v1/sso/idps/ldap1/test", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("ldap test http %d body=%s", w.Code, w.Body.String())
	}
	var ldapOut struct {
		OK     bool   `json:"ok"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ldapOut); err != nil {
		t.Fatal(err)
	}
	if !ldapOut.OK {
		t.Fatalf("ldap probe should succeed, got %q", ldapOut.Detail)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"authorization_endpoint":"https://idp/authorize","token_endpoint":"https://idp/token"}`))
	}))
	defer srv.Close()
	if err := registry.Create(ctx, &models.IdPConfig{
		ProviderID: "oidc1", Type: models.IdPOIDC, Name: "OIDC1",
		Issuer: srv.URL, DefaultRole: models.RoleDeveloper,
	}); err != nil {
		t.Fatal(err)
	}
	w = doAuthJSON(t, authMgr, admin, router, http.MethodPost, "/api/v1/sso/idps/oidc1/test", nil)
	var oidcOut struct {
		OK     bool   `json:"ok"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &oidcOut); err != nil {
		t.Fatal(err)
	}
	if !oidcOut.OK {
		t.Fatalf("oidc probe should succeed, got %q", oidcOut.Detail)
	}

	if err := registry.Create(ctx, &models.IdPConfig{
		ProviderID: "oidc2", Type: models.IdPOIDC, Name: "OIDC2",
		Issuer: "http://127.0.0.1:1", DefaultRole: models.RoleDeveloper,
	}); err != nil {
		t.Fatal(err)
	}
	w = doAuthJSON(t, authMgr, admin, router, http.MethodPost, "/api/v1/sso/idps/oidc2/test", nil)
	var badOut struct {
		OK     bool   `json:"ok"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &badOut); err != nil {
		t.Fatal(err)
	}
	if badOut.OK {
		t.Fatal("unreachable issuer should be ok:false")
	}
}

func TestListSSOProvidersEnabledOnly(t *testing.T) {
	ctx := context.Background()
	h, registry, authMgr := newIdPTestHandler(t)
	
	if err := registry.Create(ctx, &models.IdPConfig{
		ProviderID: "okta", Type: models.IdPOIDC, Name: "Okta", Enabled: true,
		Issuer: "https://okta.example.com", ClientID: "c",
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Create(ctx, &models.IdPConfig{
		ProviderID: "old", Type: models.IdPLDAP, Name: "Old LDAP", Enabled: false,
		LDAPAddr: "h:389", LDAPUserDNFormat: "uid=%s,dc=x",
	}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterRoutes(router, h, authMgr, SSOConfig{}, true, nil)
	w := doJSON(router, http.MethodGet, "/api/v1/auth/sso", nil)
	var out struct {
		Providers []map[string]interface{} `json:"providers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Providers) != 1 || out.Providers[0]["provider_id"] != "okta" {
		t.Fatalf("only enabled 'okta' expected, got %+v", out.Providers)
	}
}

func TestSSOStart_UnknownProvider_404(t *testing.T) {
	h, _, authMgr := newIdPTestHandler(t)
	router := gin.New()
	RegisterRoutes(router, h, authMgr, SSOConfig{}, true, nil)
	w := doJSON(router, http.MethodGet, "/api/v1/auth/sso/unknown/start", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown provider expected 404, got %d", w.Code)
	}
}