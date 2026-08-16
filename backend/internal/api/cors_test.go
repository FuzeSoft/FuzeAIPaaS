package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func runCORS(t *testing.T, frontendURL, origin string) http.Header {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(newCORSMiddleware(frontendURL))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Header()
}

func TestCORS_ProductionOrigin_AllowedWithCredentials(t *testing.T) {
	h := runCORS(t, "https://console.fuze.ai", "https://console.fuze.ai")
	if got := h.Get("Access-Control-Allow-Origin"); got != "https://console.fuze.ai" {
		t.Fatalf("expected allow-origin %q, got %q", "https://console.fuze.ai", got)
	}
	if got := h.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials true for prod origin, got %q", got)
	}
	if got := h.Get("Vary"); got != "Origin" {
		t.Fatalf("expected Vary: Origin, got %q", got)
	}
}

func TestCORS_ArbitraryOrigin_NotReflected(t *testing.T) {
	h := runCORS(t, "https://console.fuze.ai", "https://evil.example.com")
	if got := h.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("arbitrary origin must not be echoed, got %q", got)
	}
	if got := h.Get("Access-Control-Allow-Credentials"); got == "true" {
		t.Fatalf("credentials must not be set for non-allowlisted origin")
	}
}

func TestCORS_DevMode_LocalhostAllowedNoCredentials(t *testing.T) {
	h := runCORS(t, "", "http://localhost:3000")
	if got := h.Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("expected localhost allowed, got %q", got)
	}
	if got := h.Get("Access-Control-Allow-Credentials"); got == "true" {
		t.Fatalf("dev mode must not enable credentials")
	}

	h2 := runCORS(t, "", "https://evil.example.com")
	if got := h2.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("dev mode must not reflect arbitrary origin, got %q", got)
	}
}

func TestCORS_WildcardFrontend_NoCredentials(t *testing.T) {
	h := runCORS(t, "*", "https://anything.example.com")
	
	if got := h.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("wildcard frontend must not reflect arbitrary origin, got %q", got)
	}
	if got := h.Get("Access-Control-Allow-Credentials"); got == "true" {
		t.Fatalf("wildcard frontend must not enable credentials")
	}
}