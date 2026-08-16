package storage

import (
	"context"
	"testing"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"gorm.io/gorm"
)

func newIdPTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testDB(t)
	return db
}

func TestIdPRegistryCRUD(t *testing.T) {
	ctx := context.Background()
	repo := NewIdPRegistry(newIdPTestDB(t), nil)

	oidc := &models.IdPConfig{
		ProviderID:   "okta",
		Type:         models.IdPOIDC,
		Name:         "Okta SSO",
		Enabled:      true,
		Issuer:       "https://okta.example.com",
		ClientID:     "cid-1",
		ClientSecret: "secret-1",
		RedirectURI:  "https://app/callback",
		Scopes:       "openid email",
	}

	if err := repo.Create(ctx, oidc); err != nil {
		t.Fatalf("create: %v", err)
	}
	
	if err := repo.Create(ctx, oidc); err != ports.ErrConflict {
		t.Fatalf("expected ErrConflict, got %v", err)
	}

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("want 1, got %d", len(all))
	}
	
	if _, ok := interface{}(all[0]).(models.IdPConfig); !ok {
		t.Fatal("unexpected type")
	}

	oidc.Enabled = false
	if err := repo.Update(ctx, oidc); err != nil {
		t.Fatalf("update: %v", err)
	}
	enabled, err := repo.ListEnabled(ctx)
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	if len(enabled) != 0 {
		t.Fatalf("want 0 enabled, got %d", len(enabled))
	}

	got, err := repo.Get(ctx, "okta")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Enabled {
		t.Fatal("expected disabled after update")
	}

	if err := repo.Delete(ctx, "okta"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(ctx, "okta"); err != ports.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestIdPConfigIsValid(t *testing.T) {
	oidcBad := models.IdPConfig{Type: models.IdPOIDC}
	if oidcBad.IsValid() {
		t.Fatal("oidc without issuer/client_id must be invalid")
	}
	oidcOk := models.IdPConfig{Type: models.IdPOIDC, Issuer: "https://x", ClientID: "c"}
	if !oidcOk.IsValid() {
		t.Fatal("valid oidc must pass")
	}
	ldapBad := models.IdPConfig{Type: models.IdPLDAP}
	if ldapBad.IsValid() {
		t.Fatal("ldap without addr/dn must be invalid")
	}
	ldapOk := models.IdPConfig{Type: models.IdPLDAP, LDAPAddr: "h:389", LDAPUserDNFormat: "uid=%s,dc=x"}
	if !ldapOk.IsValid() {
		t.Fatal("valid ldap must pass")
	}
	unknown := models.IdPConfig{Type: "saml"}
	if unknown.IsValid() {
		t.Fatal("unknown type must be invalid")
	}
}