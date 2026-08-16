package tests

import (
	"testing"

	"fuze-ai-paas/backend/internal/adapter"
	"fuze-ai-paas/backend/internal/domain/inference"
	"fuze-ai-paas/backend/internal/domain/job"
	"fuze-ai-paas/backend/internal/models"
)

func TestInferenceFromModel(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		result := adapter.InferenceFromModel(nil)
		if result != nil {
			t.Error("expected nil for nil input")
		}
	})

	t.Run("converts model to domain entity", func(t *testing.T) {
		m := &models.InferenceService{
			ID:         "inf-1",
			ClusterID:  "cluster-1",
			Name:       "test-svc",
			Framework:  models.FrameworkPyTorch,
			StorageURI: "s3://bucket/model",
			Image:      "pytorch/pytorch:2.1",
			MinReplicas: 1,
			MaxReplicas: 5,
			CPU:        "4",
			Memory:     "16Gi",
			GPUs:       2,
			GPUMemory:  24576,
			GPUCores:   80,
			Status:     models.InferenceStatusReady,
			URL:        "http://svc.example.com",
		}
		result := adapter.InferenceFromModel(m)
		if result.ID != "inf-1" {
			t.Errorf("expected id inf-1, got %s", result.ID)
		}
		if result.Name != "test-svc" {
			t.Errorf("expected name test-svc, got %s", result.Name)
		}
		if result.Runtime != inference.RuntimeKServe {
			t.Errorf("expected runtime kserve, got %v", result.Runtime)
		}
		if result.Status != inference.InferenceStatusReady {
			t.Errorf("expected status ready, got %v", result.Status)
		}
		if result.GPUs != 2 {
			t.Errorf("expected 2 gpus, got %d", result.GPUs)
		}
	})

	t.Run("uses explicit Runtime if set", func(t *testing.T) {
		m := &models.InferenceService{
			ID:      "inf-2",
			Name:    "vllm-svc",
			Runtime: "vllm",
		}
		result := adapter.InferenceFromModel(m)
		if result.Runtime != "vllm" {
			t.Errorf("expected runtime vllm, got %v", result.Runtime)
		}
	})

	t.Run("falls back to Framework mapping for KServe runtime", func(t *testing.T) {
		m := &models.InferenceService{
			ID:        "inf-3",
			Name:      "kserve-svc",
			Framework: models.FrameworkPyTorch,
		}
		result := adapter.InferenceFromModel(m)
		if result.Runtime != inference.RuntimeKServe {
			t.Errorf("expected runtime kserve, got %v", result.Runtime)
		}
	})
}

func TestInferenceToModel(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		result := adapter.InferenceToModel(nil)
		if result != nil {
			t.Error("expected nil for nil input")
		}
	})

	t.Run("converts domain entity to model", func(t *testing.T) {
		inf := &inference.InferenceService{
			ID:        "inf-1",
			ClusterID: "cluster-1",
			Name:      "test-svc",
			Runtime:   inference.RuntimeTriton,
			Image:     "nvcr.io/nvidia/tritonserver:23.04",
			GPUs:      1,
			Memory:    "8Gi",
			CPU:       "2",
			Status:    inference.InferenceStatusReady,
			URL:       "http://svc.example.com",
		}
		result := adapter.InferenceToModel(inf)
		if result.ID != "inf-1" {
			t.Errorf("expected id inf-1, got %s", result.ID)
		}
		if result.Name != "test-svc" {
			t.Errorf("expected name test-svc, got %s", result.Name)
		}
		if result.Framework != models.FrameworkTriton {
			t.Errorf("expected framework triton, got %v", result.Framework)
		}
		if result.GPUs != 1 {
			t.Errorf("expected 1 gpu, got %d", result.GPUs)
		}
	})
}

func TestInferenceStatusMapping(t *testing.T) {
	t.Run("maps model statuses to domain statuses", func(t *testing.T) {
		tests := []struct {
			input    models.InferenceStatus
			expected inference.InferenceStatus
		}{
			{models.InferenceStatusReady, inference.InferenceStatusReady},
			{models.InferenceStatusFailed, inference.InferenceStatusFailed},
			{models.InferenceStatusUnknown, inference.InferenceStatusUnknown},
			{models.InferenceStatusPending, inference.InferenceStatusPending},
		}
		for _, tc := range tests {
			result := adapter.InferenceStatusFromModel(tc.input)
			if result != tc.expected {
				t.Errorf("InferenceStatusFromModel(%s) = %v, expected %v", tc.input, result, tc.expected)
			}
		}
	})

	t.Run("maps domain statuses to model statuses", func(t *testing.T) {
		tests := []struct {
			input    inference.InferenceStatus
			expected models.InferenceStatus
		}{
			{inference.InferenceStatusReady, models.InferenceStatusReady},
			{inference.InferenceStatusFailed, models.InferenceStatusFailed},
			{inference.InferenceStatusUnknown, models.InferenceStatusUnknown},
			{inference.InferenceStatusPending, models.InferenceStatusPending},
			{inference.InferenceStatusScalingUp, models.InferenceStatusReady},
			{inference.InferenceStatusScaling, models.InferenceStatusReady},
			{inference.InferenceStatusDegraded, models.InferenceStatusReady},
			{inference.InferenceStatusOffline, models.InferenceStatusReady},
		}
		for _, tc := range tests {
			result := adapter.InferenceStatusToModel(tc.input)
			if result != tc.expected {
				t.Errorf("InferenceStatusToModel(%v) = %s, expected %s", tc.input, result, tc.expected)
			}
		}
	})
}

func TestRuntimeKindFromModel(t *testing.T) {
	t.Run("maps frameworks to runtime kinds", func(t *testing.T) {
		tests := []struct {
			input    models.InferenceFramework
			expected inference.RuntimeKind
		}{
			{models.FrameworkTriton, inference.RuntimeTriton},
			{models.FrameworkCustom, inference.RuntimeCustom},
			{models.FrameworkPyTorch, inference.RuntimeKServe},
			{models.FrameworkTensorflow, inference.RuntimeKServe},
			{models.FrameworkONNX, inference.RuntimeKServe},
			{models.FrameworkSKLearn, inference.RuntimeKServe},
			{models.FrameworkXGBoost, inference.RuntimeKServe},
		}
		for _, tc := range tests {
			result := adapter.RuntimeKindFromModel(tc.input)
			if result != tc.expected {
				t.Errorf("RuntimeKindFromModel(%s) = %v, expected %v", tc.input, result, tc.expected)
			}
		}
	})
}

func TestModelsFrameworkFromRuntime(t *testing.T) {
	t.Run("maps runtime kinds back to frameworks", func(t *testing.T) {
		tests := []struct {
			input    inference.RuntimeKind
			expected models.InferenceFramework
		}{
			{inference.RuntimeTriton, models.FrameworkTriton},
			{inference.RuntimeCustom, models.FrameworkCustom},
			{inference.RuntimeKServe, models.FrameworkPyTorch},
			{inference.RuntimeVLLM, models.FrameworkPyTorch},
			{inference.RuntimeAscend, models.FrameworkPyTorch},
		}
		for _, tc := range tests {
			result := adapter.ModelsFrameworkFromRuntime(tc.input)
			if result != tc.expected {
				t.Errorf("ModelsFrameworkFromRuntime(%v) = %s, expected %s", tc.input, result, tc.expected)
			}
		}
	})
}

func TestJobFromModel(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		result := adapter.JobFromModel(nil)
		if result != nil {
			t.Error("expected nil for nil input")
		}
	})

	t.Run("converts model to domain entity", func(t *testing.T) {
		m := &models.Job{
			ID:        "job-1",
			ClusterID: "cluster-1",
			Name:      "train-job",
			Type:      models.JobTypeTraining,
			Status:    models.JobStatusPending,
			Memory:    64,
		}
		result := adapter.JobFromModel(m)
		if result.ID != "job-1" {
			t.Errorf("expected id job-1, got %s", result.ID)
		}
		if result.Name != "train-job" {
			t.Errorf("expected name train-job, got %s", result.Name)
		}
		if result.Status != job.JobStatusPending {
			t.Errorf("expected status pending, got %v", result.Status)
		}
		if result.Memory != 64 {
			t.Errorf("expected memory 64, got %d", result.Memory)
		}
	})
}

func TestJobSyncToModel(t *testing.T) {
	t.Run("nil inputs are handled safely", func(t *testing.T) {
		adapter.JobSyncToModel(nil, nil)
		adapter.JobSyncToModel(&job.Job{ID: "j1"}, nil)
		adapter.JobSyncToModel(nil, &models.Job{ID: "j1"})
	})

	t.Run("syncs status from domain to model", func(t *testing.T) {
		agg := &job.Job{
			ID:     "job-1",
			Status: job.JobStatusRunning,
		}
		m := &models.Job{
			ID:     "job-1",
			Status: models.JobStatusPending,
		}
		adapter.JobSyncToModel(agg, m)
		if m.Status != models.JobStatusRunning {
			t.Errorf("expected status running, got %v", m.Status)
		}
	})
}

func TestResourceMapping(t *testing.T) {
	t.Run("ResourceFromModel converts correctly", func(t *testing.T) {
		m := models.Resource{
			ID:              "res-1",
			ClusterID:       "cluster-1",
			Name:            "gpu-node-1",
			Type:            models.ResourceTypeGPU,
			Vendor:          "nvidia",
			Model:           "A100",
			TotalGPUs:       8,
			UsedGPUs:        2,
			TotalMemory:     512,
			AvailableMemory: 256,
			Status:          models.ResourceStatusAvailable,
			NodeName:        "node-1",
		}
		r := adapter.ResourceFromModel(m)
		if r.ID != "res-1" {
			t.Errorf("expected id res-1, got %s", r.ID)
		}
		if r.TotalGPUs != 8 {
			t.Errorf("expected 8 total gpus, got %d", r.TotalGPUs)
		}
		if r.Type != job.ResourceTypeGPU {
			t.Errorf("expected GPU type, got %v", r.Type)
		}
	})

	t.Run("ResourceToModel converts correctly", func(t *testing.T) {
		r := job.Resource{
			ID:              "res-2",
			ClusterID:       "cluster-2",
			Name:            "npu-node-1",
			Type:            job.ResourceTypeNPU,
			Vendor:          "ascend",
			TotalGPUs:       4,
			UsedGPUs:        1,
			TotalMemory:     256,
			AvailableMemory: 128,
			Status:          job.ResourceStatusAvailable,
		}
		m := adapter.ResourceToModel(r)
		if m.ID != "res-2" {
			t.Errorf("expected id res-2, got %s", m.ID)
		}
		if m.Type != models.ResourceTypeNPU {
			t.Errorf("expected NPU type, got %v", m.Type)
		}
	})
}

func TestInferenceSyncToModel(t *testing.T) {
	t.Run("nil inputs are handled safely", func(t *testing.T) {
		adapter.InferenceSyncToModel(nil, nil)
		adapter.InferenceSyncToModel(&inference.InferenceService{ID: "i1"}, nil)
		adapter.InferenceSyncToModel(nil, &models.InferenceService{ID: "i1"})
	})

	t.Run("syncs runtime status fields", func(t *testing.T) {
		inf := &inference.InferenceService{
			Status:        inference.InferenceStatusReady,
			URL:           "http://svc.example.com",
			RuntimeName:   "kserve-svc",
			ReadyReplicas: 3,
			TargetReplicas: 3,
			CanaryWeight:  20,
			Chip:          "nvidia",
		}
		m := &models.InferenceService{ID: "inf-1"}
		adapter.InferenceSyncToModel(inf, m)
		if m.Status != models.InferenceStatusReady {
			t.Errorf("expected status ready, got %v", m.Status)
		}
		if m.URL != "http://svc.example.com" {
			t.Errorf("expected url, got %s", m.URL)
		}
		if m.ReadyReplicas != 3 {
			t.Errorf("expected 3 ready replicas, got %d", m.ReadyReplicas)
		}
		if m.CanaryWeight != 20 {
			t.Errorf("expected canary weight 20, got %d", m.CanaryWeight)
		}
	})
}