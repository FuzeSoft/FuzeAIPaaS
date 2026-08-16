package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
)

type stubStore struct {
	user *models.User
}

func (s *stubStore) GetUserByUsername(username string) (*models.User, error) {
	if s.user != nil && s.user.Username == username {
		return s.user, nil
	}
	return nil, nil
}

func (s *stubStore) UpdateUserMFARecovery(userID, recoveryEnc string) error {
	if s.user != nil && s.user.ID == userID {
		s.user.MFARecoveryEnc = recoveryEnc
	}
	return nil
}

func (s *stubStore) UpdateUser(u *models.User) error {
	s.user = u
	return nil
}

func (s *stubStore) GetUserByID(id string) (*models.User, error) {
	if s.user != nil && s.user.ID == id {
		return s.user, nil
	}
	return nil, nil
}

func (s *stubStore) RecordLoginFailure(username string, maxFails, lockSec int) (*models.User, bool, error) {
	if s.user == nil || s.user.Username != username {
		return nil, false, nil
	}
	s.user.LoginFails++
	locked := false
	if maxFails > 0 && s.user.LoginFails >= maxFails {
		t := time.Now().Add(time.Duration(lockSec) * time.Second)
		s.user.LockedUntil = &t
		s.user.LoginFails = 0
		locked = true
	}
	return s.user, locked, nil
}

func (s *stubStore) ClearLoginFailures(username string) error {
	if s.user != nil && s.user.Username == username {
		s.user.LoginFails = 0
		s.user.LockedUntil = nil
	}
	return nil
}

func TestTokenRoundTrip(t *testing.T) {
	m := NewManager()
	claims := &Claims{UserID: "u1", Username: "alice", Role: models.RoleDeveloper, TenantID: "t1"}
	tok, err := m.Sign(claims)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	got, err := m.Validate(tok)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if got.UserID != "u1" || got.Username != "alice" || got.Role != models.RoleDeveloper || got.TenantID != "t1" {
		t.Fatalf("claims mismatch: %+v", got)
	}

	if _, err := m.Validate(tok + "x"); err == nil {
		t.Fatal("expected validation error on tampered token")
	}

	expTok, _ := m.Sign(&Claims{UserID: "u2", Expires: time.Now().Add(-time.Hour).Unix()})
	if _, err := m.Validate(expTok); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidateRejectsTamperedHeader(t *testing.T) {
	m := NewManager()
	tok, err := m.Sign(&Claims{UserID: "u1", Username: "alice"})
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected token shape: %q", tok)
	}

	evilHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	evil := evilHeader + "." + parts[1] + "." + parts[2]
	if _, err := m.Validate(evil); err == nil {
		t.Fatal("expected validation error for tampered header (alg=none)")
	}

	if _, err := m.Validate("eyJmb28iOiJiYXIifQ." + parts[1] + "." + parts[2]); err == nil {
		t.Fatal("expected validation error for non-canonical header")
	}
}

func TestLogin(t *testing.T) {
	m := NewManager()
	hash, _ := HashPassword("secret")
	store := &stubStore{user: &models.User{ID: "u1", Username: "alice", Password: hash, Role: models.RoleDeveloper, TenantID: "t1", Enabled: true}}

	tok, u, err := m.Login(store, "alice", "secret")
	if err != nil || tok == "" || u.Username != "alice" {
		t.Fatalf("login failed: tok=%q user=%v err=%v", tok, u, err)
	}
	if _, _, err := m.Login(store, "alice", "wrong"); err == nil {
		t.Fatal("expected error for wrong password")
	}
	store.user.Enabled = false
	if _, _, err := m.Login(store, "alice", "secret"); err == nil {
		t.Fatal("expected error for disabled user")
	}
}

func TestLoginRateLimit(t *testing.T) {
	m := NewManager()
	m.SetLoginLimit(3, 900) 
	hash, _ := HashPassword("secret")
	store := &stubStore{user: &models.User{ID: "u1", Username: "alice", Password: hash, Role: models.RoleDeveloper, TenantID: "t1", Enabled: true}}

	for i := 1; i <= 2; i++ {
		if _, _, err := m.Login(store, "alice", "bad"); err == nil {
			t.Fatalf("attempt %d: expected wrong-password error", i)
		}
	}
	if store.user.LockedUntil != nil {
		t.Fatal("expected not locked before threshold")
	}

	if _, _, err := m.Login(store, "alice", "bad"); err == nil {
		t.Fatal("attempt 3: expected wrong-password error")
	}
	if store.user.LockedUntil == nil {
		t.Fatal("expected account locked after threshold")
	}

	if _, _, err := m.Login(store, "alice", "secret"); err != ErrAccountLocked {
		t.Fatalf("expected ErrAccountLocked during lock, got %v", err)
	}

	store.user.LockedUntil = nil
	tok, u, err := m.Login(store, "alice", "secret")
	if err != nil || tok == "" {
		t.Fatalf("expected successful login after unlock, err=%v", err)
	}
	if u.LoginFails != 0 {
		t.Fatalf("expected LoginFails reset to 0 after success, got %d", u.LoginFails)
	}
}

func TestMiddlewareAndRoleGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewManager()

	tokDev, _ := m.Sign(&Claims{UserID: "u1", Username: "dev", Role: models.RoleDeveloper, TenantID: "t1"})
	tokAdmin, _ := m.Sign(&Claims{UserID: "u2", Username: "root", Role: models.RolePlatformAdmin, TenantID: "t1"})

	r := gin.New()
	r.Use(m.Middleware())
	r.GET("/ok", func(c *gin.Context) {
		cl, _ := Principal(c)
		c.String(200, cl.Username)
	})
	adminOnly := r.Group("/admin")
	adminOnly.Use(m.RequireRole(models.RolePlatformAdmin))
	adminOnly.GET("/x", func(c *gin.Context) { c.String(200, "ok") })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ok", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set("Authorization", "Bearer "+tokDev)
	r.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "dev" {
		t.Fatalf("expected 200 dev, got %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/admin/x", nil)
	req.Header.Set("Authorization", "Bearer "+tokDev)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for dev on admin route, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/admin/x", nil)
	req.Header.Set("Authorization", "Bearer "+tokAdmin)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 for admin, got %d", w.Code)
	}
}

func TestClaimsJSONShape(t *testing.T) {
	c := Claims{UserID: "u1", Username: "a", Role: models.RoleViewer, TenantID: "t1"}
	b, _ := json.Marshal(c)
	var m map[string]string
	_ = json.Unmarshal(b, &m)
	if m["role"] != string(models.RoleViewer) {
		t.Fatalf("role not serialized: %v", m)
	}
}

func TestNewManagerForEnvRejectsDevPlaceholderSecret(t *testing.T) {
	if os.Getenv("_AUTH_TEST_PHASE") == "1" {
		
		os.Setenv("AUTH_SECRET", "fuze-dev-secret-change-me")
		_ = NewManagerForEnv(true)
		
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestNewManagerForEnvRejectsDevPlaceholderSecret")
	cmd.Env = append(os.Environ(), "_AUTH_TEST_PHASE=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 1 {
		t.Fatalf("expected process exit code 1 when AUTH_SECRET is dev placeholder, got err=%v", err)
	}
}

func TestNewManagerForEnvAcceptsStrongSecret(t *testing.T) {
	os.Setenv("AUTH_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv("AUTH_SECRET")
	if m := NewManagerForEnv(true); m == nil || len(m.secret) < 32 {
		t.Fatalf("expected manager with strong secret, got %+v", m)
	}
}