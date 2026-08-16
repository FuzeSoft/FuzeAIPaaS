package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

var oidcHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
	},
}

type OIDCConfig struct {
	Enabled       bool
	Issuer        string 
	ClientID      string
	ClientSecret  string
	RedirectURL   string 
	Scopes        []string
	Role          SSORoleConfig
	DefaultTenant string
}

type OIDCProviderDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	Issuer                string `json:"issuer"`
}

type jwk struct {
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

type OIDCProvider struct {
	cfg      OIDCConfig
	disco    *OIDCProviderDiscovery
	oauthCfg *oauth2.Config

	nonceMu    sync.Mutex
	nonceStore map[string]time.Time
	nonceTTL   time.Duration

	pkceStore map[string]string

	stopCh chan struct{}
	wg     sync.WaitGroup

	testKey *rsa.PublicKey
}

const defaultNonceTTL = 10 * time.Minute

func NewOIDCProvider(cfg OIDCConfig) (*OIDCProvider, error) {
	if cfg.Issuer == "" || cfg.ClientID == "" || cfg.RedirectURL == "" {
		return nil, errors.New("oidc: issuer / clientID / redirectURL are required")
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "email", "profile"}
	}
	p := &OIDCProvider{
		cfg:        cfg,
		nonceStore: make(map[string]time.Time),
		pkceStore:  make(map[string]string),
		nonceTTL:   defaultNonceTTL,
		stopCh:     make(chan struct{}),
	}
	p.startCleanup()
	disco, err := p.discover(context.Background())
	if err != nil {
		return nil, err
	}
	p.disco = disco
	p.oauthCfg = &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       cfg.Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  disco.AuthorizationEndpoint,
			TokenURL: disco.TokenEndpoint,
		},
	}
	return p, nil
}

func NewTestOIDCProvider(cfg OIDCConfig, oauthCfg *oauth2.Config, disco *OIDCProviderDiscovery) *OIDCProvider {
	return &OIDCProvider{
		cfg:        cfg,
		oauthCfg:   oauthCfg,
		disco:      disco,
		nonceStore: make(map[string]time.Time),
		nonceTTL:   defaultNonceTTL,
	}
}

func (p *OIDCProvider) discover(ctx context.Context) (*OIDCProviderDiscovery, error) {
	u := strings.TrimRight(p.cfg.Issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := oidcHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc discovery failed: %s", resp.Status)
	}
	var d OIDCProviderDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	if d.JWKSURI == "" || d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" {
		return nil, errors.New("oidc: incomplete discovery document")
	}
	return &d, nil
}

func (p *OIDCProvider) AuthURL(state, codeChallenge string) (string, string) {
	nonce := newNonce()
	p.storeNonce(nonce)
	opts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("nonce", nonce),
	}
	if codeChallenge != "" {
		opts = append(opts,
			oauth2.SetAuthURLParam("code_challenge", codeChallenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)
	}
	url := p.oauthCfg.AuthCodeURL(state, opts...)
	return url, nonce
}

func (p *OIDCProvider) Exchange(ctx context.Context, code, nonce, codeVerifier string) (*SSOUserInfo, error) {
	exchangeOpts := []oauth2.AuthCodeOption{}
	if codeVerifier != "" {
		exchangeOpts = append(exchangeOpts, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
	}
	tok, err := p.oauthCfg.Exchange(ctx, code, exchangeOpts...)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		return nil, errors.New("oidc: id_token missing in token response")
	}
	info, err := p.verifyIDToken(rawID, nonce)
	if err != nil {
		return nil, err
	}
	if nonce != "" {
		if !p.consumeNonce(nonce) {
			return nil, errors.New("oidc: nonce already used or expired (possible replay)")
		}
	}
	return info, nil
}

func (p *OIDCProvider) verifyIDToken(raw, nonce string) (*SSOUserInfo, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, errors.New("oidc: malformed id_token")
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("oidc: bad header")
	}
	var h struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &h); err != nil {
		return nil, errors.New("oidc: bad header")
	}
	if h.Alg != "RS256" {
		return nil, fmt.Errorf("oidc: unsupported alg %s", h.Alg)
	}
	key, err := p.fetchJWK(h.Kid)
	if err != nil {
		return nil, err
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("oidc: bad payload")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("oidc: bad signature")
	}
	hashed := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed[:], sig); err != nil {
		return nil, errors.New("oidc: signature verification failed")
	}

	var claims struct {
		Sub     string   `json:"sub"`
		Iss     string   `json:"iss"`
		Aud     any      `json:"aud"`
		Exp     int64    `json:"exp"`
		Email   string   `json:"email"`
		Name    string   `json:"name"`
		Groups  []string `json:"groups"`
		PrefUID string   `json:"preferred_username"`
		Nonce   string   `json:"nonce"`
		AMR     []string `json:"amr"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errors.New("oidc: bad claims")
	}
	if claims.Iss != p.disco.Issuer {
		return nil, errors.New("oidc: issuer mismatch")
	}
	if !audienceMatches(claims.Aud, p.cfg.ClientID) {
		return nil, errors.New("oidc: audience mismatch")
	}
	
	if claims.Exp == 0 {
		return nil, errors.New("oidc: id_token missing exp claim")
	}
	if time.Now().Unix() > claims.Exp {
		return nil, errors.New("oidc: id_token expired")
	}
	if nonce != "" && claims.Nonce != nonce {
		return nil, errors.New("oidc: nonce mismatch or missing")
	}

	username := claims.Email
	if username == "" {
		username = claims.PrefUID
	}
	if username == "" {
		username = claims.Sub
	}
	return &SSOUserInfo{
		Provider:    "oidc",
		Subject:     claims.Sub,
		Username:    username,
		Email:       claims.Email,
		DisplayName: claims.Name,
		Groups:      claims.Groups,
		AMR:         claims.AMR,
	}, nil
}

func (p *OIDCProvider) fetchJWK(kid string) (*rsa.PublicKey, error) {
	if p.testKey != nil {
		return p.testKey, nil
	}
	resp, err := oidcHTTPClient.Get(p.disco.JWKSURI)
	if err != nil {
		return nil, fmt.Errorf("oidc jwks fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("oidc: jwks endpoint error")
	}
	var set jwks
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return nil, err
	}
	
	for _, k := range set.Keys {
		if k.Kid == kid {
			return jwkToRSA(k)
		}
	}
	return nil, errors.New("oidc: no matching JWK for kid=" + kid)
}

func jwkToRSA(j jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(j.N)
	if err != nil {
		return nil, fmt.Errorf("oidc: bad JWK modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(j.E)
	if err != nil {
		return nil, fmt.Errorf("oidc: bad JWK exponent: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())
	if e == 0 {
		return nil, errors.New("oidc: bad JWK exponent")
	}
	return &rsa.PublicKey{N: n, E: e}, nil
}

func audienceMatches(aud any, clientID string) bool {
	switch v := aud.(type) {
	case string:
		return v == clientID
	case []any:
		for _, a := range v {
			if s, ok := a.(string); ok && s == clientID {
				return true
			}
		}
	}
	return false
}

func newNonce() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func GeneratePKCEVerifier() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (p *OIDCProvider) storeNonce(nonce string) {
	p.nonceMu.Lock()
	defer p.nonceMu.Unlock()
	if p.nonceStore == nil {
		p.nonceStore = make(map[string]time.Time)
	}
	if p.nonceTTL == 0 {
		p.nonceTTL = defaultNonceTTL
	}
	p.nonceStore[nonce] = time.Now().Add(p.nonceTTL)
}

func (p *OIDCProvider) startCleanup() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(p.nonceTTL)
		defer ticker.Stop()
		for {
			select {
			case <-p.stopCh:
				return
			case <-ticker.C:
				p.sweepExpired()
			}
		}
	}()
}

func (p *OIDCProvider) sweepExpired() {
	now := time.Now()
	p.nonceMu.Lock()
	for k, exp := range p.nonceStore {
		if now.After(exp) {
			delete(p.nonceStore, k)
		}
	}
	p.nonceMu.Unlock()

	p.nonceMu.Lock()
	for k := range p.pkceStore {
		
		if exp, ok := p.nonceStore[k]; !ok || now.After(exp) {
			delete(p.pkceStore, k)
		}
	}
	p.nonceMu.Unlock()
}

func (p *OIDCProvider) Stop() {
	select {
	case <-p.stopCh:
		
	default:
		close(p.stopCh)
	}
	p.wg.Wait()
}

func (p *OIDCProvider) consumeNonce(nonce string) bool {
	p.nonceMu.Lock()
	defer p.nonceMu.Unlock()
	exp, ok := p.nonceStore[nonce]
	if !ok {
		return false
	}
	delete(p.nonceStore, nonce) 
	if time.Now().After(exp) {
		return false
	}
	return true
}

func (p *OIDCProvider) StorePKCE(nonce, verifier string) {
	p.nonceMu.Lock()
	defer p.nonceMu.Unlock()
	if p.pkceStore == nil {
		p.pkceStore = make(map[string]string)
	}
	p.pkceStore[nonce] = verifier
}

func (p *OIDCProvider) ConsumePKCE(nonce string) string {
	p.nonceMu.Lock()
	defer p.nonceMu.Unlock()
	v, ok := p.pkceStore[nonce]
	if !ok {
		return ""
	}
	delete(p.pkceStore, nonce)
	return v
}