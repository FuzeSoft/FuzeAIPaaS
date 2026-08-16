package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fuze-ai-paas/backend/internal/auth"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

func newOIDCRouter(t *testing.T, secureCookie bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	prov := auth.NewTestOIDCProvider(
		auth.OIDCConfig{Enabled: true, Issuer: "https://idp.example.com", ClientID: "c", RedirectURL: "https://app/cb"},
		&oauth2.Config{
			ClientID:    "c",
			RedirectURL: "https://app/cb",
			Endpoint:    oauth2.Endpoint{AuthURL: "https://idp.example.com/auth", TokenURL: "https://idp.example.com/token"},
		},
		&auth.OIDCProviderDiscovery{
			Issuer:                "https://idp.example.com",
			AuthorizationEndpoint: "https://idp.example.com/auth",
			TokenEndpoint:         "https://idp.example.com/token",
			JWKSURI:               "https://idp.example.com/jwks",
		},
	)
	h := &Handler{sso: SSOConfig{OIDC: prov, SecureCookie: secureCookie}}
	r := gin.New()
	RegisterRoutes(r, h, nil, h.sso, true, nil)
	return r
}

func hasSecureCookie(h http.Header) bool {
	for _, c := range h.Values("Set-Cookie") {
		if strings.Contains(strings.ToLower(c), "secure") {
			return true
		}
	}
	return false
}

func TestOIDCStartSecureCookieEnabled(t *testing.T) {
	r := newOIDCRouter(t, true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/sso/oidc/start", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected redirect, got %d", w.Code)
	}
	if !hasSecureCookie(w.Header()) {
		t.Fatalf("expected Secure cookie when SSOConfig.SecureCookie=true, got headers %v", w.Header().Values("Set-Cookie"))
	}
}

func TestOIDCStartSecureCookieDisabled(t *testing.T) {
	r := newOIDCRouter(t, false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/sso/oidc/start", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected redirect, got %d", w.Code)
	}
	if hasSecureCookie(w.Header()) {
		t.Fatalf("expected NO Secure cookie when SSOConfig.SecureCookie=false, got headers %v", w.Header().Values("Set-Cookie"))
	}
}