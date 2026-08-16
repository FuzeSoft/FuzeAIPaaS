package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestAuthURLWithPKCE(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := mockOIDCProvider(t, priv)
	url, _ := p.AuthURL("state-x", "abc123challenge")
	if !strings.Contains(url, "code_challenge=abc123challenge") {
		t.Fatalf("expected code_challenge in url, got %q", url)
	}
	if !strings.Contains(url, "code_challenge_method=S256") {
		t.Fatalf("expected code_challenge_method=S256 in url, got %q", url)
	}
}

func TestAuthURLWithoutPKCE(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := mockOIDCProvider(t, priv)
	url, _ := p.AuthURL("state-x", "")
	if strings.Contains(url, "code_challenge") {
		t.Fatalf("expected NO code_challenge when not using PKCE, got %q", url)
	}
}

func TestExchangePKCEViaMockTokenServer(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)

	var gotVerifier string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotVerifier = r.FormValue("code_verifier")
		
		claims := map[string]any{
			"sub":   "user-1",
			"iss":   "https://idp.example.com",
			"aud":   "client-123",
			"exp":   time.Now().Add(10 * time.Minute).Unix(),
			"nonce": "test-nonce",
		}
		raw := signIDToken(t, priv, claims)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at",
			"token_type":   "Bearer",
			"id_token":     raw,
		})
	}))
	defer ts.Close()

	p := NewTestOIDCProvider(
		OIDCConfig{Enabled: true, Issuer: "https://idp.example.com", ClientID: "client-123", RedirectURL: "https://app/cb"},
		&oauth2.Config{
			ClientID:    "client-123",
			RedirectURL: "https://app/cb",
			Endpoint:    oauth2.Endpoint{AuthURL: "https://idp.example.com/auth", TokenURL: ts.URL},
		},
		&OIDCProviderDiscovery{
			Issuer:                "https://idp.example.com",
			AuthorizationEndpoint: "https://idp.example.com/auth",
			TokenEndpoint:         ts.URL,
			JWKSURI:               "https://idp.example.com/jwks",
		},
	)
	p.testKey = &priv.PublicKey

	p.storeNonce("test-nonce")

	info, err := p.Exchange(context.Background(), "code-xyz", "test-nonce", "verifier-secret")
	if err != nil {
		t.Fatalf("Exchange failed: %v", err)
	}
	if info.Subject != "user-1" {
		t.Fatalf("bad subject %q", info.Subject)
	}
	if gotVerifier != "verifier-secret" {
		t.Fatalf("expected code_verifier forwarded to token endpoint, got %q", gotVerifier)
	}
}

func TestExchangePKCEVerifierStoreRoundTrip(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := mockOIDCProvider(t, priv)
	p.StorePKCE("n", "verifier-secret")
	if got := p.ConsumePKCE("n"); got != "verifier-secret" {
		t.Fatalf("expected stored verifier, got %q", got)
	}
	if got := p.ConsumePKCE("n"); got != "" {
		t.Fatalf("expected verifier single-use, got %q", got)
	}
}