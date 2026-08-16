package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
)

func TestMiddlewarePATTriggersTokenTouch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewManager()

	var mu sync.Mutex
	var touched []string
	touchedCh := make(chan string, 1)
	m.SetTokenTouch(func(_ context.Context, jti string) {
		mu.Lock()
		touched = append(touched, jti)
		mu.Unlock()
		touchedCh <- jti
	})

	pjti := "pat-jti-123"
	patTok, err := m.Sign(&Claims{
		UserID:   "u1",
		Username: "dev",
		Role:     models.RoleDeveloper,
		TenantID: "t1",
		JTI:      pjti,
		Scope:    ScopePAT,
	})
	if err != nil {
		t.Fatalf("sign pat: %v", err)
	}

	r := gin.New()
	r.Use(m.Middleware())
	r.GET("/ok", func(c *gin.Context) { c.String(200, "ok") })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set("Authorization", "Bearer "+patTok)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("PAT 应可访问资源，got %d", w.Code)
	}

	select {
	case got := <-touchedCh:
		if got != pjti {
			t.Fatalf("tokenTouch 应携带 jti=%q，实际 %q", pjti, got)
		}
	case <-time.After(time.Second):
		t.Fatal("PAT 校验后未触发 tokenTouch")
	}
}

func TestMiddlewareNonPATDoesNotTriggerTokenTouch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewManager()

	var mu sync.Mutex
	touched := 0
	m.SetTokenTouch(func(_ context.Context, jti string) {
		mu.Lock()
		defer mu.Unlock()
		touched++
	})

	loginTok, err := m.Sign(&Claims{UserID: "u1", Username: "dev", Role: models.RoleDeveloper, TenantID: "t1"})
	if err != nil {
		t.Fatalf("sign login: %v", err)
	}

	r := gin.New()
	r.Use(m.Middleware())
	r.GET("/ok", func(c *gin.Context) { c.String(200, "ok") })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set("Authorization", "Bearer "+loginTok)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("普通令牌应可访问资源，got %d", w.Code)
	}

	mu.Lock()
	defer mu.Unlock()
	if touched != 0 {
		t.Fatalf("普通登录令牌不应触发 tokenTouch，实际触发 %d 次", touched)
	}
}