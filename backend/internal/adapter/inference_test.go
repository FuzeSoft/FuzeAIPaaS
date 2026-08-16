package adapter

import (
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/models"
)

func TestInferenceRoundTrip(t *testing.T) {
	now := time.Now()
	m := &models.InferenceService{
		ID:            "svc-1",
		ClusterID:     "cluster-001",
		Name:          "llm",
		Framework:     models.FrameworkTriton,
		StorageURI:    "s3://models/llm",
		MinReplicas:   1,
		MaxReplicas:   4,
		GPUs:          2,
		GPUMemory:     40,
		GPUCores:      80,
		Status:        models.InferenceStatusReady,
		URL:           "http://llm",
		KServeName:    "isvc-llm",
		ReadyReplicas: 2,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	agg := InferenceFromModel(m)
	if agg == nil {
		t.Fatal("聚合根不应为 nil")
	}
	if agg.Runtime != "triton" {
		t.Errorf("Runtime 映射错误: %s", agg.Runtime)
	}
	if agg.Status != "ready" {
		t.Errorf("Status 映射错误: %s", agg.Status)
	}

	agg.Status = "failed"
	agg.ReadyReplicas = 0
	InferenceSyncToModel(agg, m)
	if m.Status != models.InferenceStatusFailed {
		t.Errorf("回写 Status 错误: %s", m.Status)
	}
	if m.ReadyReplicas != 0 {
		t.Errorf("回写 ReadyReplicas 错误: %d", m.ReadyReplicas)
	}
}

func TestInferenceNilSafe(t *testing.T) {
	if InferenceFromModel(nil) != nil {
		t.Error("nil 输入应返回 nil")
	}
	
	InferenceSyncToModel(nil, &models.InferenceService{})
	InferenceSyncToModel(InferenceFromModel(&models.InferenceService{}), nil)
}

func TestInferenceRuntimePreference(t *testing.T) {
	
	m := &models.InferenceService{
		ID:        "svc-2",
		Name:      "llm",
		Framework: models.FrameworkTriton,
		Runtime:   "vllm",
	}
	agg := InferenceFromModel(m)
	if agg.Runtime != "vllm" {
		t.Fatalf("显式 Runtime 应优先，期望 vllm 实际 %s", agg.Runtime)
	}

	m2 := &models.InferenceService{ID: "svc-3", Name: "x", Framework: models.FrameworkTriton}
	if InferenceFromModel(m2).Runtime != "triton" {
		t.Fatal("Runtime 为空时应回退为 triton")
	}

	back := InferenceToModel(agg)
	if back.Runtime != "vllm" {
		t.Fatalf("ToModel 应持久化 Runtime=vllm，实际 %s", back.Runtime)
	}

	agg.TargetReplicas = 3
	agg.CanaryWeight = 20
	InferenceSyncToModel(agg, m)
	if m.TargetReplicas != 3 || m.CanaryWeight != 20 {
		t.Fatalf("SyncToModel 生命周期字段未同步: target=%d canary=%d", m.TargetReplicas, m.CanaryWeight)
	}
}