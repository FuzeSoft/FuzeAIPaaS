package auth

import (
	"strings"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/crypto/aes"
	"fuze-ai-paas/backend/internal/models"
)

func newTestCipher(t *testing.T) *aes.Cipher {
	t.Helper()
	var k [32]byte
	for i := range k {
		k[i] = byte(i)
	}
	return aes.NewCipher(k)
}

func TestTOTPRFC6238Vector(t *testing.T) {
	
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	m := NewMFAService(newTestCipher(t))
	m.digits = 8
	m.step = 30 * time.Second
	code := m.computeTOTP(secret, time.Unix(59, 0))
	if code != "94287082" {
		t.Fatalf("RFC6238 vector mismatch: got %q want 94287082", code)
	}
}

func TestTOTPVerifyWithSkew(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	m := NewMFAService(newTestCipher(t))
	m.digits = 8
	m.step = 30 * time.Second
	
	at := time.Unix(1234567890, 0)
	code := m.computeTOTP(secret, at)
	if !m.VerifyTOTP(secret, code, at) {
		t.Fatal("expected current-window code to verify")
	}
	
	prev := at.Add(-30 * time.Second)
	prevCode := m.computeTOTP(secret, prev)
	if !m.VerifyTOTP(secret, prevCode, at) {
		t.Fatal("expected previous-window code (skew) to verify")
	}
	
	if m.VerifyTOTP(secret, "00000000", at) {
		t.Fatal("expected wrong code to be rejected")
	}
}

func TestTOTPReplayRejected(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	m := NewMFAService(newTestCipher(t))
	m.digits = 8
	at := time.Unix(59, 0)
	code := m.computeTOTP(secret, at)
	if !m.VerifyTOTP(secret, code, at) {
		t.Fatal("first use should pass")
	}
	if m.VerifyTOTP(secret, code, at) {
		t.Fatal("replay of same code within window must be rejected")
	}
}

func TestMFASecretEncryptRoundTrip(t *testing.T) {
	m := NewMFAService(newTestCipher(t))
	plain := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	enc, err := m.EncryptSecret(plain)
	if err != nil {
		t.Fatal(err)
	}
	if enc == plain {
		t.Fatal("secret must not be stored in plaintext")
	}
	if strings.Contains(enc, plain) {
		t.Fatal("plaintext secret leaked in ciphertext")
	}
	dec, err := m.DecryptSecret(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != plain {
		t.Fatalf("decrypt mismatch: got %q", dec)
	}
}

func TestMFAGenerateAndVerifyRecovery(t *testing.T) {
	m := NewMFAService(newTestCipher(t))
	codes, enc, err := m.GenerateRecovery(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 5 {
		t.Fatalf("expected 5 recovery codes, got %d", len(codes))
	}
	
	used := codes[0]
	remaining, ok, err := m.ConsumeRecovery(enc, used)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected valid recovery code to be consumed")
	}
	
	if _, ok2, _ := m.ConsumeRecovery(remaining, used); ok2 {
		t.Fatal("used recovery code must not be reusable")
	}
	
	other := codes[1]
	if _, ok3, _ := m.ConsumeRecovery(remaining, other); !ok3 {
		t.Fatal("unused recovery code should still be valid")
	}
}

func TestMFAGenerateSecretURI(t *testing.T) {
	m := NewMFAService(newTestCipher(t))
	secret, uri, err := m.GenerateSecret("alice@example.com", "FuzeAI")
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" {
		t.Fatal("expected non-empty secret")
	}
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Fatalf("expected otpauth URI, got %q", uri)
	}
	if !strings.Contains(uri, "secret="+secret) {
		t.Fatalf("URI must embed secret, got %q", uri)
	}
}

func TestMFABeginConfirmFlow(t *testing.T) {
	m := NewManager()
	m.SetMFA(NewMFAService(newTestCipher(t)))
	store := &stubStore{user: &models.User{
		ID: "u1", Username: "alice", Role: models.RoleDeveloper, TenantID: "t1", Enabled: true,
	}}

	secret, uri, codes, err := m.BeginMFA(store, "alice", "FuzeAI")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if secret == "" || uri == "" || len(codes) == 0 {
		t.Fatal("begin must return secret/uri/recovery codes")
	}
	if store.user.MFAEnabled {
		t.Fatal("MFA must NOT be enabled after BeginMFA")
	}
	if store.user.PendingTOTPEnc == "" {
		t.Fatal("pending secret must be stored after BeginMFA")
	}

	if err := m.ConfirmMFA(store, "alice", "000000"); err == nil {
		t.Fatal("expected confirm failure for wrong code")
	}
	if store.user.MFAEnabled {
		t.Fatal("MFA must stay disabled after wrong code")
	}

	code := m.mfa.computeTOTP(secret, time.Now())
	if err := m.ConfirmMFA(store, "alice", code); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !store.user.MFAEnabled {
		t.Fatal("MFA must be enabled after correct confirm")
	}
	if store.user.PendingTOTPEnc != "" {
		t.Fatal("pending secret must be cleared after confirm")
	}
}