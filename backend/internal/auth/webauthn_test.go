package auth

import (
	"testing"

	"fuze-ai-paas/backend/internal/crypto/aes"
	"fuze-ai-paas/backend/internal/models"

	"github.com/go-webauthn/webauthn/webauthn"
)

func testCipher() *aes.Cipher {
	var k [32]byte
	for i := range k {
		k[i] = byte(i)
	}
	return aes.NewCipher(k)
}

func mustWebAuthn(t *testing.T) *WebAuthnService {
	t.Helper()
	s, err := NewWebAuthnService("example.com", "Fuze", []string{"https://example.com"}, testCipher())
	if err != nil {
		t.Fatalf("webauthn service: %v", err)
	}
	return s
}

func TestNewWebAuthnServiceNilCipher(t *testing.T) {
	if _, err := NewWebAuthnService("example.com", "Fuze", []string{"https://example.com"}, nil); err == nil {
		t.Fatal("expected error for nil cipher")
	}
}

func TestWebAuthnBeginRegistrationProducesOptions(t *testing.T) {
	svc, err := NewWebAuthnService("example.com", "Fuze", []string{"https://example.com"}, testCipher())
	if err != nil {
		t.Fatal(err)
	}
	u := &models.User{ID: "u1", Username: "alice"}
	cc, err := svc.BeginRegistration(u)
	if err != nil {
		t.Fatal(err)
	}
	if cc == nil || cc.Response.RelyingParty.ID != "example.com" {
		t.Fatalf("unexpected CredentialCreation: %+v", cc)
	}
	
	if _, ok := svc.takeSession("u1"); !ok {
		t.Fatal("expected registration session to be stored")
	}
}

func TestWebAuthnCredentialRoundtrip(t *testing.T) {
	svc, _ := NewWebAuthnService("example.com", "Fuze", []string{"https://example.com"}, testCipher())
	u := &models.User{ID: "u2", Username: "bob"}

	enc, err := svc.serializeCredentials(nil)
	if err != nil {
		t.Fatal(err)
	}
	u.Passkeys = enc
	got, err := svc.parseCredentials(u)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 credentials, got %d", len(got))
	}

	sample := []webauthn.Credential{
		{
			ID: []byte("cred-id-1"),
			Authenticator: webauthn.Authenticator{
				AAGUID:    []byte("0123456789abcdef"),
				SignCount: 5,
			},
		},
	}
	enc2, err := svc.serializeCredentials(sample)
	if err != nil {
		t.Fatal(err)
	}
	u.Passkeys = enc2
	got2, err := svc.parseCredentials(u)
	if err != nil {
		t.Fatal(err)
	}
	if len(got2) != 1 || string(got2[0].ID) != "cred-id-1" {
		t.Fatalf("roundtrip mismatch: %+v", got2)
	}
	if got2[0].Authenticator.SignCount != 5 {
		t.Fatalf("sign count not preserved: %d", got2[0].Authenticator.SignCount)
	}
}

func TestWebAuthnBeginLoginNotConfigured(t *testing.T) {
	svc, _ := NewWebAuthnService("example.com", "Fuze", []string{"https://example.com"}, testCipher())
	u := &models.User{ID: "u3", Username: "carol"} 
	if _, err := svc.BeginLogin(u); err != ErrPasskeyNotConfigured {
		t.Fatalf("expected ErrPasskeyNotConfigured, got %v", err)
	}
}

func TestVerifyPasskeyNotConfigured(t *testing.T) {
	m := NewManager()
	m.SetPasskey(nil) 
	store := &stubStore{user: &models.User{ID: "u4", Username: "dave", PasskeyEnabled: true}}
	if _, err := m.VerifyPasskey(store, "any", nil); err == nil {
		t.Fatal("expected error when passkey not configured")
	}
}

func TestClearPasskeys(t *testing.T) {
	m := NewManager()
	m.SetPasskey(mustWebAuthn(t))
	store := &stubStore{user: &models.User{ID: "u5", Username: "erin", PasskeyEnabled: true, Passkeys: "encrypted-blob"}}
	if err := m.ClearPasskeys(store, "erin"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if store.user.PasskeyEnabled {
		t.Fatal("PasskeyEnabled must be false after ClearPasskeys")
	}
	if store.user.Passkeys != "" {
		t.Fatal("Passkeys must be cleared")
	}
}