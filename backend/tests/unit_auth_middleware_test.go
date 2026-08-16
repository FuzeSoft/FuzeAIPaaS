package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
)

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := auth.NewManager()

	t.Run("rejects request without authorization header", func(t *testing.T) {
		r := gin.New()
		r.Use(mgr.Middleware())
		r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("rejects request with non-bearer authorization", func(t *testing.T) {
		r := gin.New()
		r.Use(mgr.Middleware())
		r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Basic abc123")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("rejects request with invalid token", func(t *testing.T) {
		r := gin.New()
		r.Use(mgr.Middleware())
		r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer invalid.token.here")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("accepts request with valid token", func(t *testing.T) {
		r := gin.New()
		r.Use(mgr.Middleware())
		r.GET("/test", func(c *gin.Context) {
			claims, ok := auth.Principal(c)
			if !ok {
				c.JSON(500, gin.H{"error": "no principal"})
				return
			}
			c.JSON(200, gin.H{"username": claims.Username})
		})

		token, err := mgr.Sign(&auth.Claims{
			UserID:   "u1",
			Username: "testuser",
			Role:     models.RolePlatformAdmin,
			TenantID: "default",
		})
		if err != nil {
			t.Fatal(err)
		}

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d, body: %s", w.Code, w.Body.String())
		}
	})
}

func TestRequireRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := auth.NewManager()

	t.Run("allows access for correct role", func(t *testing.T) {
		r := gin.New()
		r.Use(mgr.Middleware())
		r.GET("/admin", mgr.RequireRole(models.RolePlatformAdmin), func(c *gin.Context) {
			c.JSON(200, gin.H{"ok": true})
		})

		token, _ := mgr.Sign(&auth.Claims{
			UserID:   "u1",
			Username: "admin",
			Role:     models.RolePlatformAdmin,
			TenantID: "default",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/admin", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("denies access for wrong role", func(t *testing.T) {
		r := gin.New()
		r.Use(mgr.Middleware())
		r.GET("/admin", mgr.RequireRole(models.RolePlatformAdmin), func(c *gin.Context) {
			c.JSON(200, gin.H{"ok": true})
		})

		token, _ := mgr.Sign(&auth.Claims{
			UserID:   "u2",
			Username: "developer",
			Role:     models.RoleDeveloper,
			TenantID: "default",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/admin", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", w.Code)
		}
	})
}

func TestPassthroughMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("injects system admin principal", func(t *testing.T) {
		r := gin.New()
		r.Use(auth.PassthroughMiddleware())
		r.GET("/test", func(c *gin.Context) {
			claims, ok := auth.Principal(c)
			if !ok {
				c.JSON(500, gin.H{"error": "no principal"})
				return
			}
			c.JSON(200, gin.H{
				"username": claims.Username,
				"role":     claims.Role,
			})
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

}

func TestTokenValidation(t *testing.T) {
	mgr := auth.NewManager()

	t.Run("rejects token with wrong format", func(t *testing.T) {
		_, err := mgr.Validate("not-a-token")
		if err == nil {
			t.Error("expected error for invalid token format")
		}
	})

	t.Run("rejects token with wrong number of parts", func(t *testing.T) {
		_, err := mgr.Validate("only.two")
		if err == nil {
			t.Error("expected error for two-part token")
		}
	})

	t.Run("rejects expired token", func(t *testing.T) {
		token, _ := mgr.Sign(&auth.Claims{
			UserID:   "u1",
			Username: "expired-user",
			Role:     models.RoleDeveloper,
			TenantID: "default",
			IssuedAt: 1000,
			Expires:  1001, 
		})
		_, err := mgr.Validate(token)
		if err == nil {
			t.Error("expected error for expired token")
		}
	})

	t.Run("accepts valid token with custom expiry", func(t *testing.T) {
		token, _ := mgr.Sign(&auth.Claims{
			UserID:   "u1",
			Username: "valid-user",
			Role:     models.RoleDeveloper,
			TenantID: "default",
		})
		claims, err := mgr.Validate(token)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if claims.Username != "valid-user" {
			t.Errorf("expected username valid-user, got %s", claims.Username)
		}
	})
}