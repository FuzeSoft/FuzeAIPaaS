package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"crypto"
	"golang.org/x/oauth2"
)

func mockOIDCProvider(t *testing.T, priv *rsa.PrivateKey) *OIDCProvider {
	t.Helper()
	p := NewTestOIDCProvider(
		OIDCConfig{
			Enabled:     true,
			Issuer:      "https://idp.example.com",
			ClientID:    "client-123",
			RedirectURL: "https://app.example.com/callback",
		},
		&oauth2.Config{
			ClientID:    "client-123",
			RedirectURL: "https://app.example.com/callback",
			Endpoint:    oauth2.Endpoint{AuthURL: "https://idp.example.com/auth", TokenURL: "https://idp.example.com/token"},
		},
		&OIDCProviderDiscovery{
			Issuer:                "https://idp.example.com",
			AuthorizationEndpoint: "https://idp.example.com/auth",
			TokenEndpoint:         "https://idp.example.com/token",
			JWKSURI:               "https://idp.example.com/jwks",
		},
	)
	
	p.testKey = &priv.PublicKey
	return p
}

func signIDToken(t *testing.T, priv *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"test"}`))
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	signingInput := header + "." + payload
	hashed := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, hashed[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func baseClaims(nonce string) map[string]any {
	c := map[string]any{
		"sub":   "user-1",
		"iss":   "https://idp.example.com",
		"aud":   "client-123",
		"exp":   time.Now().Add(10 * time.Minute).Unix(),
		"email": "alice@example.com",
	}
	if nonce != "" {
		c["nonce"] = nonce
	}
	return c
}

func TestVerifyIDTokenNonceMatch(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := mockOIDCProvider(t, priv)
	raw := signIDToken(t, priv, baseClaims("expected-abc"))
	info, err := p.verifyIDToken(raw, "expected-abc")
	if err != nil {
		t.Fatalf("expected nonce match to pass, got %v", err)
	}
	if info.Subject != "user-1" {
		t.Fatalf("bad subject %q", info.Subject)
	}
}

func TestVerifyIDTokenNonceMismatch(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := mockOIDCProvider(t, priv)
	raw := signIDToken(t, priv, baseClaims("attacker-nonce"))
	_, err := p.verifyIDToken(raw, "expected-abc")
	if err == nil {
		t.Fatal("expected nonce mismatch to be rejected")
	}
}

func TestVerifyIDTokenNonceMissingWhenRequired(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := mockOIDCProvider(t, priv)
	raw := signIDToken(t, priv, baseClaims("")) 
	_, err := p.verifyIDToken(raw, "expected-abc")
	if err == nil {
		t.Fatal("expected missing nonce (when verification requested) to be rejected")
	}
}

func TestAuthURLGeneratesAndStoresNonce(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := mockOIDCProvider(t, priv)
	url, nonce := p.AuthURL("state-xyz", "")
	if nonce == "" {
		t.Fatal("expected AuthURL to return a nonce")
	}
	if !strings.Contains(url, "nonce="+nonce) {
		t.Fatalf("expected auth url to carry nonce, got %q", url)
	}
	if !p.consumeNonce(nonce) {
		t.Fatal("expected stored nonce to be consumable")
	}
	if p.consumeNonce(nonce) {
		t.Fatal("expected nonce to be single-use")
	}
}

func TestAuthURLNonceExpiry(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := mockOIDCProvider(t, priv)
	p.nonceTTL = -1 * time.Second 
	_, nonce := p.AuthURL("state-xyz", "")
	if p.consumeNonce(nonce) {
		t.Fatal("expected expired nonce to be rejected")
	}
}