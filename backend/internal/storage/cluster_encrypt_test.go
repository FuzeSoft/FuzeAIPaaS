package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fuze-ai-paas/backend/internal/crypto/aes"
	"fuze-ai-paas/backend/internal/models"
)

func newTestStorageWithCipher(t *testing.T) *Storage {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := OpenDB(DBConfig{Driver: DriverSQLite, DSN: path})
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	var key [32]byte
	for i := range key {
		key[i] = byte(i)
	}
	s := NewStorage(db)
	s.SetCipher(aes.NewCipher(key))
	return s
}

func TestClusterCreateEncryptsKubeConfig(t *testing.T) {
	s := newTestStorageWithCipher(t)
	secret := "apiVersion: v1\nkind: Config\nusers:\n- name: admin\n  user:\n    token: super-secret"
	cluster := &models.Cluster{Name: "gpu-prod", KubeConfig: secret}

	if err := s.CreateCluster(cluster); err != nil {
		t.Fatalf("create: %v", err)
	}
	
	if cluster.KubeConfig != "" {
		t.Fatalf("in-memory KubeConfig should be cleared after create, got %q", cluster.KubeConfig)
	}
	if cluster.KubeConfigEnc == "" {
		t.Fatal("KubeConfigEnc must be set after create")
	}
	if strings.Contains(cluster.KubeConfigEnc, "super-secret") {
		t.Fatal("ciphertext must not contain plaintext")
	}

	var raw models.Cluster
	if err := s.db.Where("id = ?", cluster.ID).First(&raw).Error; err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if raw.KubeConfigEnc == "" {
		t.Fatal("db row must store ciphertext")
	}
}

func TestClusterGetDecryptsKubeConfig(t *testing.T) {
	s := newTestStorageWithCipher(t)
	secret := "token: super-secret-yaml"
	if err := s.CreateCluster(&models.Cluster{Name: "c1", KubeConfig: secret}); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.GetClusterByName("c1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.KubeConfig != secret {
		t.Fatalf("decrypted KubeConfig mismatch: got %q want %q", got.KubeConfig, secret)
	}
}

func TestClusterUpdateReEncrypts(t *testing.T) {
	s := newTestStorageWithCipher(t)
	if err := s.CreateCluster(&models.Cluster{Name: "c1", KubeConfig: "old-secret"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	c, _ := s.GetClusterByName("c1")
	c.KubeConfig = "new-secret"
	if err := s.UpdateCluster(c); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := s.GetClusterByName("c1")
	if got.KubeConfig != "new-secret" {
		t.Fatalf("updated decrypted mismatch: got %q", got.KubeConfig)
	}
}