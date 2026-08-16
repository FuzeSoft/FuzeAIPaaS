package tests

import (
	"testing"

	"fuze-ai-paas/backend/internal/adapter"
	"fuze-ai-paas/backend/internal/domain/model"
	"fuze-ai-paas/backend/internal/models"
)

func TestModelFromModelAdapter(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		result := adapter.ModelFromModel(nil)
		if result != nil {
			t.Error("expected nil for nil input")
		}
	})

	t.Run("converts model to domain entity", func(t *testing.T) {
		m := &models.Model{
			ID:          "model-001",
			Name:        "bert-base",
			Description: "BERT base model",
			Framework:   "pytorch",
			Owner:       "data-team",
		}
		result := adapter.ModelFromModel(m)
		if result.ID != "model-001" {
			t.Errorf("expected id model-001, got %s", result.ID)
		}
		if result.Name != "bert-base" {
			t.Errorf("expected name bert-base, got %s", result.Name)
		}
		if result.Framework != model.FrameworkPyTorch {
			t.Errorf("expected framework pytorch, got %s", result.Framework)
		}
		if result.Owner != "data-team" {
			t.Errorf("expected owner data-team, got %s", result.Owner)
		}
	})
}

func TestModelSyncToModel(t *testing.T) {
	t.Run("nil inputs are handled safely", func(t *testing.T) {
		adapter.ModelSyncToModel(nil, nil)
		adapter.ModelSyncToModel(&model.Model{ID: "m1"}, nil)
		adapter.ModelSyncToModel(nil, &models.Model{ID: "m1"})
	})

	t.Run("syncs mutable fields from domain to model", func(t *testing.T) {
		agg := &model.Model{
			Name:        "updated-name",
			Description: "updated description",
			Framework:   model.FrameworkTensorFlow,
			Owner:       "new-owner",
		}
		m := &models.Model{
			ID:   "model-001",
			Name: "old-name",
		}
		adapter.ModelSyncToModel(agg, m)
		if m.Name != "updated-name" {
			t.Errorf("expected name updated-name, got %s", m.Name)
		}
		if m.Description != "updated description" {
			t.Errorf("expected description, got %s", m.Description)
		}
		if m.Framework != "tensorflow" {
			t.Errorf("expected framework tensorflow, got %s", m.Framework)
		}
		if m.Owner != "new-owner" {
			t.Errorf("expected owner new-owner, got %s", m.Owner)
		}
	})
}

func TestModelVersionFromModelAdapter(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		result := adapter.ModelVersionFromModel(nil)
		if result != nil {
			t.Error("expected nil for nil input")
		}
	})

	t.Run("converts model version to domain entity", func(t *testing.T) {
		m := &models.ModelVersion{
			ID:         "ver-001",
			ModelID:    "model-001",
			Version:    "v1.0",
			StorageURI: "s3://bucket/model-v1",
			Image:      "pytorch/serve:2.1",
			SizeBytes:  1024000,
			Hash:       "abc123",
		}
		result := adapter.ModelVersionFromModel(m)
		if result.ID != "ver-001" {
			t.Errorf("expected id ver-001, got %s", result.ID)
		}
		if result.ModelID != "model-001" {
			t.Errorf("expected model id model-001, got %s", result.ModelID)
		}
		if result.Version != "v1.0" {
			t.Errorf("expected version v1.0, got %s", result.Version)
		}
		if result.StorageURI != "s3://bucket/model-v1" {
			t.Errorf("expected storage uri, got %s", result.StorageURI)
		}
		if result.SizeBytes != 1024000 {
			t.Errorf("expected size 1024000, got %d", result.SizeBytes)
		}
		if result.Hash != "abc123" {
			t.Errorf("expected hash abc123, got %s", result.Hash)
		}
	})
}

func TestModelVersionSyncToModel(t *testing.T) {
	t.Run("nil inputs are handled safely", func(t *testing.T) {
		adapter.ModelVersionSyncToModel(nil, nil)
		adapter.ModelVersionSyncToModel(&model.ModelVersion{ID: "v1"}, nil)
		adapter.ModelVersionSyncToModel(nil, &models.ModelVersion{ID: "v1"})
	})

	t.Run("syncs fields from domain to model", func(t *testing.T) {
		agg := &model.ModelVersion{
			ModelID:    "model-002",
			Version:    "v2.0",
			StorageURI: "oss://bucket/v2",
			Image:      "tf/serving:2.14",
			SizeBytes:  2048000,
			Hash:       "def456",
		}
		m := &models.ModelVersion{
			ID:      "ver-002",
			ModelID: "model-001",
			Version: "v1.0",
		}
		adapter.ModelVersionSyncToModel(agg, m)
		if m.ModelID != "model-002" {
			t.Errorf("expected model id model-002, got %s", m.ModelID)
		}
		if m.Version != "v2.0" {
			t.Errorf("expected version v2.0, got %s", m.Version)
		}
		if m.StorageURI != "oss://bucket/v2" {
			t.Errorf("expected storage uri, got %s", m.StorageURI)
		}
		if m.SizeBytes != 2048000 {
			t.Errorf("expected size 2048000, got %d", m.SizeBytes)
		}
		if m.Hash != "def456" {
			t.Errorf("expected hash def456, got %s", m.Hash)
		}
	})
}