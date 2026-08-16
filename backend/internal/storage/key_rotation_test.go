package storage

import (
	"testing"

	"fuze-ai-paas/backend/internal/crypto/aes"
	"fuze-ai-paas/backend/internal/models"
)

func testKey(b byte) [32]byte {
	var k [32]byte
	for i := range k {
		k[i] = b
	}
	return k
}

func TestRotateKubeConfigKeys(t *testing.T) {
	s := newEntStorage(t)
	oldC := aes.NewCipher(testKey(1))
	newC := aes.NewCipher(testKey(2))

	plaintexts := []string{"kubeconfig-A", "kubeconfig-B", "kubeconfig-C"}
	for i, pt := range plaintexts {
		enc, err := oldC.EncryptString(pt)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.db.Create(&models.Cluster{
			ID:            "cl-" + string(rune('a'+i)),
			Name:          "cluster-" + string(rune('a'+i)),
			KubeConfigEnc: enc,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	
	if err := s.db.Create(&models.Cluster{ID: "cl-empty", Name: "empty", KubeConfigEnc: ""}).Error; err != nil {
		t.Fatal(err)
	}

	n, err := s.RotateKubeConfigKeys(oldC, newC)
	if err != nil {
		t.Fatalf("RotateKubeConfigKeys: %v", err)
	}
	if n != len(plaintexts) {
		t.Fatalf("expected rotated=%d, got %d", len(plaintexts), n)
	}

	for i, pt := range plaintexts {
		var c models.Cluster
		if err := s.db.Where("id = ?", "cl-"+string(rune('a'+i))).First(&c).Error; err != nil {
			t.Fatal(err)
		}
		got, err := newC.DecryptString(c.KubeConfigEnc)
		if err != nil {
			t.Fatalf("cluster %d: new key decrypt failed: %v", i, err)
		}
		if got != pt {
			t.Fatalf("cluster %d: expected %q, got %q", i, pt, got)
		}
		if _, err := oldC.DecryptString(c.KubeConfigEnc); err == nil {
			t.Fatalf("cluster %d: old key should NOT decrypt after rotation", i)
		}
	}
}

func TestRotateKubeConfigKeysMismatchAborts(t *testing.T) {
	s := newEntStorage(t)
	
	encA := aes.NewCipher(testKey(1))
	wrongOld := aes.NewCipher(testKey(9))
	newC := aes.NewCipher(testKey(2))

	enc, _ := encA.EncryptString("secret")
	if err := s.db.Create(&models.Cluster{ID: "cl-x", Name: "x", KubeConfigEnc: enc}).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := s.RotateKubeConfigKeys(wrongOld, newC); err == nil {
		t.Fatal("expected ErrRotationKeyMismatch, got nil")
	}
	
	var c models.Cluster
	if err := s.db.Where("id = ?", "cl-x").First(&c).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := encA.DecryptString(c.KubeConfigEnc); err != nil {
		t.Fatalf("ciphertext should be unchanged after aborted rotation: %v", err)
	}
}

func TestCountKubeConfigEnc(t *testing.T) {
	s := newEntStorage(t)
	oldC := aes.NewCipher(testKey(1))
	enc, _ := oldC.EncryptString("x")
	if err := s.db.Create(&models.Cluster{ID: "c1", Name: "c1", KubeConfigEnc: enc}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Create(&models.Cluster{ID: "c2", Name: "c2", KubeConfigEnc: ""}).Error; err != nil {
		t.Fatal(err)
	}
	n, err := s.CountKubeConfigEnc()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 enc cluster, got %d", n)
	}
}