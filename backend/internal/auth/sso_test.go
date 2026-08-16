package auth

import (
	"net"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestResolveRole(t *testing.T) {
	cfg := SSORoleConfig{
		DefaultRole: models.RoleDeveloper,
		AdminGroups: []string{"admins", "fuze-admin"},
		AdminRole:   models.RoleTenantAdmin,
	}
	if r := cfg.resolveRole([]string{"users"}); r != models.RoleDeveloper {
		t.Fatalf("expected developer, got %s", r)
	}
	if r := cfg.resolveRole([]string{"Admins"}); r != models.RoleTenantAdmin {
		t.Fatalf("expected tenant_admin, got %s", r)
	}
	if r := cfg.resolveRole(nil); r != models.RoleDeveloper {
		t.Fatalf("expected developer default, got %s", r)
	}
}

func fakeLDAP(t *testing.T, code byte) (addr string, stop func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 1024)
			conn.Read(buf) 
			resp := []byte{
				0x30, 0x0c,
				0x02, 0x01, 0x01,
				0x61, 0x07,
				0x0a, 0x01, code,
				0x04, 0x00,
				0x04, 0x00,
			}
			conn.Write(resp)
			conn.Close()
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func TestLDAPBindSuccess(t *testing.T) {
	addr, stop := fakeLDAP(t, 0x00)
	defer stop()
	if err := ldapBind(addr, false, false, "uid=alice,dc=ex,dc=com", "secret"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestLDAPBindFailure(t *testing.T) {
	addr, stop := fakeLDAP(t, 0x31) 
	defer stop()
	if err := ldapBind(addr, false, false, "uid=bob,dc=ex,dc=com", "wrong"); err == nil {
		t.Fatal("expected bind failure")
	}
}

func TestLDAPLogin(t *testing.T) {
	addr, stop := fakeLDAP(t, 0x00)
	defer stop()
	cfg := LDAPConfig{Enabled: true, Addr: addr, UserDNFormat: "uid=%s,dc=ex,dc=com"}
	info, err := LDAPLogin(cfg, "alice", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if info.Username != "alice" || info.Provider != "ldap" {
		t.Fatalf("bad info %+v", info)
	}
}