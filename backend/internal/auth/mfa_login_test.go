package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/crypto/aes"
	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
)

func TestLoginMFARequiredReturnsTempToken(t *testing.T) {
	m := NewManager()
	var key [32]byte
	for i := range key {
		key[i] = byte(i)
	}
	m.SetMFA(NewMFAService(aes.NewCipher(key)))
	hash, _ := HashPassword("secret")
	svc := m.mfa
	plainSecret, _, _ := svc.GenerateSecret("alice@example.com", "FuzeAI")
	enc, _ := svc.EncryptSecret(plainSecret)
	store := &stubStore{user: &models.User{
		ID: "u1", Username: "alice", Password: hash, Role: models.RoleDeveloper,
		TenantID: "t1", Enabled: true, MFAEnabled: true, TOTPSecretEnc: enc,
	}}

	tok, u, err := m.Login(store, "alice", "secret")
	if err != nil || tok == "" {
		t.Fatalf("login failed: %v", err)
	}
	claims, err := m.Validate(tok)
	if err != nil {
		t.Fatalf("temp token invalid: %v", err)
	}
	if claims.Scope != "mfa" {
		t.Fatalf("expected scope=mfa temp token, got %q", claims.Scope)
	}
	
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(m.Middleware())
	r.GET("/ok", func(c *gin.Context) { c.String(200, "ok") })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for mfa temp token, got %d", w.Code)
	}
	_ = u
}

func TestVerifyMFAFullFlow(t *testing.T) {
	m := NewManager()
	var key [32]byte
	for i := range key {
		key[i] = byte(i)
	}
	m.SetMFA(NewMFAService(aes.NewCipher(key)))
	hash, _ := HashPassword("secret")
	svc := m.mfa
	plainSecret, _, _ := svc.GenerateSecret("alice@example.com", "FuzeAI")
	enc, _ := svc.EncryptSecret(plainSecret)
	store := &stubStore{user: &models.User{
		ID: "u1", Username: "alice", Password: hash, Role: models.RoleDeveloper,
		TenantID: "t1", Enabled: true, MFAEnabled: true, TOTPSecretEnc: enc,
	}}
	tok, _, err := m.Login(store, "alice", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := m.VerifyMFA(store, tok, "000000"); err == nil {
		t.Fatal("expected rejection for wrong code")
	}
	code := svc.computeTOTP(plainSecret, time.Now())
	full, err := m.VerifyMFA(store, tok, code)
	if err != nil {
		t.Fatalf("verify mfa failed: %v", err)
	}
	fc, err := m.Validate(full)
	if err != nil || fc.Scope != "" {
		t.Fatalf("expected full token without scope, got scope=%q err=%v", fc.Scope, err)
	}
}

func TestVerifyMFARecoveryCode(t *testing.T) {
	m := NewManager()
	var key [32]byte
	for i := range key {
		key[i] = byte(i)
	}
	m.SetMFA(NewMFAService(aes.NewCipher(key)))
	hash, _ := HashPassword("secret")
	svc := m.mfa
	codes, recEnc, _ := svc.GenerateRecovery(3)
	store := &stubStore{user: &models.User{
		ID: "u1", Username: "alice", Password: hash, Role: models.RoleDeveloper,
		TenantID: "t1", Enabled: true, MFAEnabled: true, MFARecoveryEnc: recEnc,
	}}
	tok, _, err := m.Login(store, "alice", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	full, err := m.VerifyMFA(store, tok, codes[0])
	if err != nil {
		t.Fatalf("recovery verify failed: %v", err)
	}
	if _, err := m.Validate(full); err != nil {
		t.Fatalf("full token invalid: %v", err)
	}
}