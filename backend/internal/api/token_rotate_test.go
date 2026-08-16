package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"fuze-ai-paas/backend/internal/storage"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newRotateTestHandler(t *testing.T) (*Handler, ports.TokenRepository, *auth.Manager) {
	t.Helper()
	f, err := os.CreateTemp("", "fuze-tok-*.db")
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
	tokenRepo := storage.NewTokenRepository(db)
	authMgr := auth.NewManager()

	h := NewHandler(Repos{
		Token: tokenRepo,
		Audit: audit,
	}, nil, nil, authMgr, nil)
	return h, tokenRepo, authMgr
}

type memAudit struct{ rows []models.AuditLog }

func (m *memAudit) Record(e *models.AuditLog) error { m.rows = append(m.rows, *e); return nil }
func (m *memAudit) ListAudit(opts ports.AuditQuery) ([]models.AuditLog, error) {
	return m.rows, nil
}

func seedToken(t *testing.T, repo ports.TokenRepository, id, userID, jti string, expires *time.Time) {
	t.Helper()
	if err := repo.Create(context.Background(), &models.PersonalAccessToken{
		ID:       id,
		Name:     id,
		Prefix:   "fuze_" + id,
		UserID:   userID,
		TenantID: "t-1",
		JTI:      jti,
		ExpiresAt:  expires,
		CreatedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
}

func doAuthJSON(t *testing.T, authMgr *auth.Manager, claims *auth.Claims, router http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	tok, err := authMgr.Sign(claims)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	router.ServeHTTP(w, req)
	return w
}

func TestRotateTokenOwnerSucceeds(t *testing.T) {
	h, repo, authMgr := newRotateTestHandler(t)
	expires := time.Now().Add(30 * 24 * time.Hour)
	seedToken(t, repo, "tok-1", "u-1", "jti-1", &expires)

	router := gin.New()
	claims := &auth.Claims{UserID: "u-1", Role: models.RoleDeveloper, TenantID: "t-1"}
	RegisterRoutes(router, h, authMgr, SSOConfig{}, true, nil)

	w := doAuthJSON(t, authMgr, claims, router, http.MethodPost, "/api/v1/auth/tokens/tok-1/rotate", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("rotate: expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
	var issued struct {
		ID     string `json:"id"`
		Token  string `json:"token"`
		Prefix string `json:"prefix"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &issued); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if issued.Token == "" || issued.ID == "tok-1" {
		t.Fatalf("unexpected issued: %+v", issued)
	}
	if issued.ID == "" {
		t.Fatal("new id expected")
	}
}

func TestRotateTokenWrongOwnerForbidden(t *testing.T) {
	h, repo, authMgr := newRotateTestHandler(t)
	expires := time.Now().Add(30 * 24 * time.Hour)
	seedToken(t, repo, "tok-1", "u-1", "jti-1", &expires)

	router := gin.New()
	claims := &auth.Claims{UserID: "u-2", Role: models.RoleDeveloper, TenantID: "t-1"}
	RegisterRoutes(router, h, authMgr, SSOConfig{}, true, nil)

	w := doAuthJSON(t, authMgr, claims, router, http.MethodPost, "/api/v1/auth/tokens/tok-1/rotate", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (no existence leak), got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestRotateTokenAudited(t *testing.T) {
	h, repo, authMgr := newRotateTestHandler(t)
	expires := time.Now().Add(30 * 24 * time.Hour)
	seedToken(t, repo, "tok-1", "u-1", "jti-1", &expires)

	router := gin.New()
	claims := &auth.Claims{UserID: "u-1", Username: "alice", Role: models.RoleDeveloper, TenantID: "t-1"}
	RegisterRoutes(router, h, authMgr, SSOConfig{}, true, nil)

	w := doAuthJSON(t, authMgr, claims, router, http.MethodPost, "/api/v1/auth/tokens/tok-1/rotate", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("rotate: got %d", w.Code)
	}
	audit := h.auditRepo.(*memAudit)
	found := false
	for _, row := range audit.rows {
		if row.Action == models.ActionTokenRotate && row.ResourceID == "tok-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected token_rotate audit for tok-1, rows=%+v", audit.rows)
	}
}