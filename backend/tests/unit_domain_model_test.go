package tests

import (
	"testing"

	"fuze-ai-paas/backend/internal/domain/model"
)

func TestModelFrameworkConstants(t *testing.T) {
	tests := []struct {
		name  string
		value model.Framework
	}{
		{"tensorflow", model.FrameworkTensorFlow},
		{"pytorch", model.FrameworkPyTorch},
		{"onnx", model.FrameworkONNX},
		{"sklearn", model.FrameworkSklearn},
		{"xgboost", model.FrameworkXGBoost},
		{"triton", model.FrameworkTriton},
		{"vllm", model.FrameworkVLLM},
		{"custom", model.FrameworkCustom},
	}
	for _, tc := range tests {
		if string(tc.value) != tc.name {
			t.Errorf("expected %s, got %s", tc.name, tc.value)
		}
	}
}

func TestStorageBackendConstants(t *testing.T) {
	tests := []struct {
		name  string
		value model.StorageBackend
	}{
		{"s3", model.StorageS3},
		{"oss", model.StorageOSS},
		{"pvc", model.StoragePVC},
		{"nfs", model.StorageNFS},
		{"local", model.StorageLocal},
	}
	for _, tc := range tests {
		if string(tc.value) != tc.name {
			t.Errorf("expected %s, got %s", tc.name, tc.value)
		}
	}
}

func TestModelStruct(t *testing.T) {
	m := model.Model{
		ID:          "model-001",
		Name:        "bert-base-uncased",
		Description: "Pre-trained BERT model",
		Framework:   model.FrameworkPyTorch,
		Owner:       "data-team",
	}
	if m.ID != "model-001" {
		t.Errorf("expected id model-001, got %s", m.ID)
	}
	if m.Framework != model.FrameworkPyTorch {
		t.Errorf("expected framework pytorch, got %s", m.Framework)
	}
}

func TestModelVersionReference(t *testing.T) {
	t.Run("returns reference with model name", func(t *testing.T) {
		mv := model.ModelVersion{
			ID:         "ver-001",
			ModelID:    "model-001",
			Version:    "v1.2.0",
			StorageURI: "s3://bucket/model/v1.2.0",
			Image:      "pytorch/serve:2.1",
		}
		ref := mv.Reference("bert-base")
		if ref.ModelID != "model-001" {
			t.Errorf("expected model id model-001, got %s", ref.ModelID)
		}
		if ref.ModelName != "bert-base" {
			t.Errorf("expected model name bert-base, got %s", ref.ModelName)
		}
		if ref.Version != "v1.2.0" {
			t.Errorf("expected version v1.2.0, got %s", ref.Version)
		}
		if ref.StorageURI != "s3://bucket/model/v1.2.0" {
			t.Errorf("expected storage uri, got %s", ref.StorageURI)
		}
		if ref.Image != "pytorch/serve:2.1" {
			t.Errorf("expected image, got %s", ref.Image)
		}
	})

	t.Run("reference with empty model name", func(t *testing.T) {
		mv := model.ModelVersion{
			ID:         "ver-002",
			ModelID:    "model-002",
			Version:    "v2.0",
			StorageURI: "oss://bucket/model/v2.0",
		}
		ref := mv.Reference("")
		if ref.ModelName != "" {
			t.Errorf("expected empty model name, got %s", ref.ModelName)
		}
		if ref.Image != "" {
			t.Errorf("expected empty image, got %s", ref.Image)
		}
	})
}