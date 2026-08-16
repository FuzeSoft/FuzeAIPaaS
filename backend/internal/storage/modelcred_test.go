package storage

import (
	"strings"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func newCredStorage(t *testing.T) *Storage {
	t.Helper()
	s := newTestStorageWithCipher(t)
	return s
}

func TestModelCredentialEncryptsAPIKey(t *testing.T) {
	s := newCredStorage(t)
	secret := "sk-proj-abcdef123456"
	cred := &models.ModelCredential{
		TenantID: "t1",
		Backend:  "openai",
		Name:     "prod-key",
		APIKey:   secret,
		BaseURL:  "https://api.openai.com/v1",
	}
	if err := s.UpsertModelCredential(cred); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if cred.APIKeyEnc == "" {
		t.Fatal("APIKeyEnc must be set")
	}
	if strings.Contains(cred.APIKeyEnc, "sk-proj") {
		t.Fatal("ciphertext must not leak plaintext")
	}

	var raw models.ModelCredential
	if err := s.db.Where("id = ?", cred.ID).First(&raw).Error; err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if raw.APIKeyEnc == "" {
		t.Fatal("db row must store ciphertext")
	}
}

func TestModelCredentialReadDecryptsAndTenantIsolation(t *testing.T) {
	s := newCredStorage(t)
	secret := "sk-hidden"
	if err := s.UpsertModelCredential(&models.ModelCredential{
		TenantID: "t1", Backend: "openai", Name: "k1", APIKey: secret,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertModelCredential(&models.ModelCredential{
		TenantID: "t2", Backend: "openai", Name: "k2", APIKey: "other-tenant",
	}); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListModelCredentials("t1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("t1 should see 1 cred, got %d", len(list))
	}
	if list[0].APIKey != secret {
		t.Fatalf("decrypted APIKey mismatch: got %q", list[0].APIKey)
	}

	if err := s.DeleteModelCredential("t2", list[0].ID); err == nil {
		t.Fatal("cross-tenant delete must return ErrModelCredentialNotFound")
	}
	
	if err := s.DeleteModelCredential("t1", list[0].ID); err != nil {
		t.Fatalf("same-tenant delete: %v", err)
	}
	list, _ = s.ListModelCredentials("t1")
	if len(list) != 0 {
		t.Fatalf("after delete t1 should see 0, got %d", len(list))
	}
}