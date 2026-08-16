package storage

import (
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	db := testDB(t)
	return &Storage{db: db}
}

func TestModelCRUD(t *testing.T) {
	s := newTestStorage(t)

	m := &models.Model{Name: "llama2", Framework: "pytorch", Owner: "alice"}
	if err := s.CreateModel(m); err != nil {
		t.Fatalf("create model: %v", err)
	}
	if m.ID == "" {
		t.Fatal("model ID not generated")
	}

	got, err := s.GetModel(m.ID)
	if err != nil {
		t.Fatalf("get model: %v", err)
	}
	if got.Name != "llama2" {
		t.Fatalf("unexpected name: %s", got.Name)
	}

	all, err := s.GetModels()
	if err != nil || len(all) != 1 {
		t.Fatalf("list models: %v len=%d", err, len(all))
	}

	got.Description = "updated"
	if err := s.UpdateModel(got); err != nil {
		t.Fatalf("update model: %v", err)
	}
	got2, _ := s.GetModel(m.ID)
	if got2.Description != "updated" {
		t.Fatalf("update not persisted")
	}

	v := &models.ModelVersion{ModelID: m.ID, Version: "v1", StorageURI: "s3://x"}
	if err := s.CreateModelVersion(v); err != nil {
		t.Fatalf("create version: %v", err)
	}
	versions, err := s.GetModelVersions(m.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("list versions: %v len=%d", err, len(versions))
	}
	if _, err := s.GetModelVersion(m.ID, v.ID); err != nil {
		t.Fatalf("get version: %v", err)
	}
	if _, err := s.GetModelVersion(m.ID, "nope"); err == nil {
		t.Fatal("expected error for unknown version")
	}

	if err := s.DeleteModel(m.ID); err != nil {
		t.Fatalf("delete model: %v", err)
	}
	if _, err := s.GetModel(m.ID); err == nil {
		t.Fatal("model should be deleted")
	}
	versions, _ = s.GetModelVersions(m.ID)
	if len(versions) != 0 {
		t.Fatalf("versions should be cascade-deleted, got %d", len(versions))
	}
}