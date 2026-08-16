package aes

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
)

func mustKey(t *testing.T) [32]byte {
	t.Helper()
	var k [32]byte
	if _, err := rand.Read(k[:]); err != nil {
		t.Fatalf("read key: %v", err)
	}
	return k
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c := NewCipher(mustKey(t))
	plain := []byte("kubeconfig-yaml-secret-content")
	env, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if env == "" {
		t.Fatal("envelope must not be empty")
	}
	
	if strings.Contains(env, "kubeconfig-yaml") {
		t.Fatal("envelope must not contain plaintext")
	}
	got, err := c.Decrypt(env)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("roundtrip mismatch: got %q want %q", got, plain)
	}
}

func TestEncryptUniqueNonce(t *testing.T) {
	c := NewCipher(mustKey(t))
	a, _ := c.Encrypt([]byte("same"))
	b, _ := c.Encrypt([]byte("same"))
	if a == b {
		t.Fatal("same plaintext must produce different envelopes (random nonce)")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	c1 := NewCipher(mustKey(t))
	c2 := NewCipher(mustKey(t))
	env, err := c1.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := c2.Decrypt(env); err == nil {
		t.Fatal("decrypt with wrong key must fail")
	}
}

func TestDecryptTamperedFails(t *testing.T) {
	c := NewCipher(mustKey(t))
	env, _ := c.Encrypt([]byte("secret"))
	tampered := env
	if len(tampered) > 0 {
		
		b := []byte(tampered)
		b[len(b)-1] ^= 0x01
		tampered = string(b)
	}
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatal("decrypt of tampered envelope must fail")
	}
}

func TestDecryptInvalidEnvelopeFails(t *testing.T) {
	c := NewCipher(mustKey(t))
	if _, err := c.Decrypt("not-base64-!!!");

 err == nil {
		t.Fatal("invalid envelope must fail")
	}
	if _, err := c.Decrypt(""); err == nil {
		t.Fatal("empty envelope must fail")
	}
}

func TestLoadMasterKeyHex(t *testing.T) {
	var k [32]byte
	if _, err := rand.Read(k[:]); err != nil {
		t.Fatal(err)
	}
	getenv := func(s string) string {
		if s == "KUBECONFIG_ENC_KEY" {
			return hex.EncodeToString(k[:])
		}
		return ""
	}
	got, err := LoadMasterKey(getenv)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != k {
		t.Fatal("loaded key mismatch")
	}
}

func TestLoadMasterKeyMissingFails(t *testing.T) {
	getenv := func(string) string { return "" }
	if _, err := LoadMasterKey(getenv); err == nil {
		t.Fatal("missing key must fail (fail-fast)")
	}
}

func TestLoadMasterKeyBadLengthFails(t *testing.T) {
	getenv := func(s string) string {
		if s == "KUBECONFIG_ENC_KEY" {
			return "00" 
		}
		return ""
	}
	if _, err := LoadMasterKey(getenv); err == nil {
		t.Fatal("non-32-byte key must fail")
	}
}