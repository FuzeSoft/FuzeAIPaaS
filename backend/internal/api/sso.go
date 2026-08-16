package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type SSOConfig struct {
	
	Registry ports.IdPRegistry

	OIDC        *auth.OIDCProvider
	OIDCConfig  auth.OIDCConfig
	LDAP        auth.LDAPConfig
	FrontendURL string

	SecureCookie bool

	cache *ssoCache
}

type ssoCache struct {
	mu        sync.Mutex
	providers map[string]*auth.OIDCProvider
}

func (c *SSOConfig) providerFor(ctx context.Context, providerID string) (*auth.OIDCProvider, *models.IdPConfig, error) {
	if c.Registry != nil && providerID != "" {
		cfg, err := c.Registry.Get(ctx, providerID)
		if err != nil {
			return nil, nil, err
		}
		if !cfg.Enabled {
			return nil, nil, ports.ErrNotFound
		}
		if cfg.Type != models.IdPOIDC {
			return nil, nil, fmt.Errorf("sso: provider %q is not oidc", providerID)
		}
		if c.cache == nil {
			c.cache = &ssoCache{providers: make(map[string]*auth.OIDCProvider)}
		}
		c.cache.mu.Lock()
		defer c.cache.mu.Unlock()
		if p, ok := c.cache.providers[providerID]; ok {
			return p, cfg, nil
		}
		oc := auth.OIDCConfig{
			Issuer:       cfg.Issuer,
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURI,
			Scopes:       strings.Fields(cfg.Scopes),
		}
		p, err := auth.NewOIDCProvider(oc)
		if err != nil {
			return nil, nil, err
		}
		c.cache.providers[providerID] = p
		return p, cfg, nil
	}
	
	if c.OIDC != nil {
		return c.OIDC, nil, nil
	}
	return nil, nil, errors.New("sso: no oidc provider configured")
}

func (c SSOConfig) Stop() {
	if c.OIDC != nil {
		c.OIDC.Stop()
	}
	if c.cache != nil {
		c.cache.mu.Lock()
		defer c.cache.mu.Unlock()
		for _, p := range c.cache.providers {
			if p != nil {
				p.Stop()
			}
		}
	}
}