package tests

import (
	"testing"

	"fuze-ai-paas/backend/internal/domain/inference"
)

func TestInferenceScaleTo(t *testing.T) {
	t.Run("scales up within bounds", func(t *testing.T) {
		svc := &inference.InferenceService{
			MinReplicas:    1,
			MaxReplicas:    10,
			ReadyReplicas:  2,
		}
		svc.ScaleTo(5)
		if svc.TargetReplicas != 5 {
			t.Errorf("expected target 5, got %d", svc.TargetReplicas)
		}
		if svc.Status != inference.InferenceStatusScalingUp {
			t.Errorf("expected scaling_up status, got %v", svc.Status)
		}
	})

	t.Run("scales down within bounds", func(t *testing.T) {
		svc := &inference.InferenceService{
			MinReplicas:    1,
			MaxReplicas:    10,
			ReadyReplicas:  5,
		}
		svc.ScaleTo(2)
		if svc.TargetReplicas != 2 {
			t.Errorf("expected target 2, got %d", svc.TargetReplicas)
		}
		if svc.Status != inference.InferenceStatusScaling {
			t.Errorf("expected scaling status, got %v", svc.Status)
		}
	})

	t.Run("clamps to min replicas", func(t *testing.T) {
		svc := &inference.InferenceService{
			MinReplicas:    3,
			MaxReplicas:    10,
			ReadyReplicas:  5,
		}
		svc.ScaleTo(0)
		if svc.TargetReplicas != 3 {
			t.Errorf("expected target clamped to 3, got %d", svc.TargetReplicas)
		}
	})

	t.Run("clamps to max replicas", func(t *testing.T) {
		svc := &inference.InferenceService{
			MinReplicas:    1,
			MaxReplicas:    5,
			ReadyReplicas:  2,
		}
		svc.ScaleTo(100)
		if svc.TargetReplicas != 5 {
			t.Errorf("expected target clamped to 5, got %d", svc.TargetReplicas)
		}
	})

	t.Run("no transition when target equals ready", func(t *testing.T) {
		svc := &inference.InferenceService{
			MinReplicas:    1,
			MaxReplicas:    10,
			ReadyReplicas:  3,
			Status:         inference.InferenceStatusReady,
		}
		svc.ScaleTo(3)
		if svc.TargetReplicas != 3 {
			t.Errorf("expected target 3, got %d", svc.TargetReplicas)
		}
		if svc.Status != inference.InferenceStatusReady {
			t.Errorf("expected status unchanged (ready), got %v", svc.Status)
		}
	})
}

func TestCanScaleUp(t *testing.T) {
	t.Run("can scale up when below max", func(t *testing.T) {
		svc := &inference.InferenceService{
			MaxReplicas:   5,
			ReadyReplicas: 3,
		}
		if !svc.CanScaleUp() {
			t.Error("expected can scale up")
		}
	})

	t.Run("cannot scale up at max", func(t *testing.T) {
		svc := &inference.InferenceService{
			MaxReplicas:   5,
			ReadyReplicas: 5,
		}
		if svc.CanScaleUp() {
			t.Error("expected cannot scale up at max")
		}
	})
}

func TestApplyRuntimeStatus(t *testing.T) {
	t.Run("sets pending when not found", func(t *testing.T) {
		svc := &inference.InferenceService{}
		svc.ApplyRuntimeStatus(false, false, false, 0, "")
		if svc.Status != inference.InferenceStatusPending {
			t.Errorf("expected pending, got %v", svc.Status)
		}
	})

	t.Run("sets ready when found and ready with replicas", func(t *testing.T) {
		svc := &inference.InferenceService{}
		svc.ApplyRuntimeStatus(true, true, false, 3, "http://svc.example.com")
		if svc.Status != inference.InferenceStatusReady {
			t.Errorf("expected ready, got %v", svc.Status)
		}
		if svc.ReadyReplicas != 3 {
			t.Errorf("expected 3 replicas, got %d", svc.ReadyReplicas)
		}
		if svc.URL != "http://svc.example.com" {
			t.Errorf("expected url, got %s", svc.URL)
		}
	})

	t.Run("sets offline when ready but zero replicas", func(t *testing.T) {
		svc := &inference.InferenceService{}
		svc.ApplyRuntimeStatus(true, true, false, 0, "")
		if svc.Status != inference.InferenceStatusOffline {
			t.Errorf("expected offline, got %v", svc.Status)
		}
	})

	t.Run("sets failed when runtime reports failure", func(t *testing.T) {
		svc := &inference.InferenceService{Status: inference.InferenceStatusReady}
		svc.ApplyRuntimeStatus(false, true, true, 1, "")
		if svc.Status != inference.InferenceStatusFailed {
			t.Errorf("expected failed, got %v", svc.Status)
		}
	})

	t.Run("sets degraded when was ready but not ready now", func(t *testing.T) {
		svc := &inference.InferenceService{Status: inference.InferenceStatusReady}
		svc.ApplyRuntimeStatus(false, true, false, 2, "")
		if svc.Status != inference.InferenceStatusDegraded {
			t.Errorf("expected degraded, got %v", svc.Status)
		}
	})

	t.Run("sets scaling up when not ready and was not ready", func(t *testing.T) {
		svc := &inference.InferenceService{Status: inference.InferenceStatusPending}
		svc.ApplyRuntimeStatus(false, true, false, 1, "")
		if svc.Status != inference.InferenceStatusScalingUp {
			t.Errorf("expected scaling_up, got %v", svc.Status)
		}
	})

	t.Run("updates url when provided", func(t *testing.T) {
		svc := &inference.InferenceService{URL: "http://old.example.com"}
		svc.ApplyRuntimeStatus(true, true, false, 1, "http://new.example.com")
		if svc.URL != "http://new.example.com" {
			t.Errorf("expected new url, got %s", svc.URL)
		}
	})

	t.Run("preserves old url when empty", func(t *testing.T) {
		svc := &inference.InferenceService{URL: "http://old.example.com"}
		svc.ApplyRuntimeStatus(true, true, false, 1, "")
		if svc.URL != "http://old.example.com" {
			t.Errorf("expected old url preserved, got %s", svc.URL)
		}
	})
}

func TestMarkFailedAndOffline(t *testing.T) {
	t.Run("mark failed sets failed status", func(t *testing.T) {
		svc := &inference.InferenceService{Status: inference.InferenceStatusReady}
		svc.MarkFailed()
		if svc.Status != inference.InferenceStatusFailed {
			t.Errorf("expected failed, got %v", svc.Status)
		}
	})

	t.Run("mark offline sets offline and zero replicas", func(t *testing.T) {
		svc := &inference.InferenceService{
			Status:        inference.InferenceStatusReady,
			ReadyReplicas: 5,
		}
		svc.MarkOffline()
		if svc.Status != inference.InferenceStatusOffline {
			t.Errorf("expected offline, got %v", svc.Status)
		}
		if svc.ReadyReplicas != 0 {
			t.Errorf("expected 0 replicas, got %d", svc.ReadyReplicas)
		}
	})
}

func TestDesiredReplicas(t *testing.T) {
	t.Run("returns ready replicas when no load", func(t *testing.T) {
		svc := &inference.InferenceService{MinReplicas: 1, MaxReplicas: 10}
		m := inference.ScalingMetrics{QueueDepth: 0, GPUUtil: 50, ReadyReplicas: 3}
		desired := svc.DesiredReplicas(m)
		if desired != 3 {
			t.Errorf("expected 3, got %d", desired)
		}
	})

	t.Run("adds replicas for queue depth", func(t *testing.T) {
		svc := &inference.InferenceService{MinReplicas: 1, MaxReplicas: 10}
		m := inference.ScalingMetrics{QueueDepth: 25, GPUUtil: 50, ReadyReplicas: 2}
		desired := svc.DesiredReplicas(m)
		
		if desired != 5 {
			t.Errorf("expected 5, got %d", desired)
		}
	})

	t.Run("adds replica for high GPU utilization", func(t *testing.T) {
		svc := &inference.InferenceService{MinReplicas: 1, MaxReplicas: 10}
		m := inference.ScalingMetrics{QueueDepth: 0, GPUUtil: 85, ReadyReplicas: 2}
		desired := svc.DesiredReplicas(m)
		
		if desired != 3 {
			t.Errorf("expected 3, got %d", desired)
		}
	})

	t.Run("clamps to min replicas", func(t *testing.T) {
		svc := &inference.InferenceService{MinReplicas: 3, MaxReplicas: 10}
		m := inference.ScalingMetrics{QueueDepth: 0, GPUUtil: 10, ReadyReplicas: 1}
		desired := svc.DesiredReplicas(m)
		if desired != 3 {
			t.Errorf("expected 3 (clamped to min), got %d", desired)
		}
	})

	t.Run("clamps to max replicas", func(t *testing.T) {
		svc := &inference.InferenceService{MinReplicas: 1, MaxReplicas: 5}
		m := inference.ScalingMetrics{QueueDepth: 100, GPUUtil: 95, ReadyReplicas: 4}
		desired := svc.DesiredReplicas(m)
		if desired != 5 {
			t.Errorf("expected 5 (clamped to max), got %d", desired)
		}
	})
}

func TestCanaryPromotion(t *testing.T) {
	t.Run("sets canary weight", func(t *testing.T) {
		svc := &inference.InferenceService{}
		svc.PromoteCanary(50)
		if svc.CanaryWeight != 50 {
			t.Errorf("expected weight 50, got %d", svc.CanaryWeight)
		}
	})

	t.Run("clamps negative weight to 0", func(t *testing.T) {
		svc := &inference.InferenceService{}
		svc.PromoteCanary(-10)
		if svc.CanaryWeight != 0 {
			t.Errorf("expected weight 0, got %d", svc.CanaryWeight)
		}
	})

	t.Run("clamps weight over 100 to 100", func(t *testing.T) {
		svc := &inference.InferenceService{}
		svc.PromoteCanary(150)
		if svc.CanaryWeight != 100 {
			t.Errorf("expected weight 100, got %d", svc.CanaryWeight)
		}
	})

	t.Run("is canary active when weight is between 0 and 100", func(t *testing.T) {
		svc := &inference.InferenceService{CanaryWeight: 50}
		if !svc.IsCanaryActive() {
			t.Error("expected canary active")
		}
	})

	t.Run("is not canary active when weight is 0", func(t *testing.T) {
		svc := &inference.InferenceService{CanaryWeight: 0}
		if svc.IsCanaryActive() {
			t.Error("expected canary not active")
		}
	})

	t.Run("is not canary active when weight is 100", func(t *testing.T) {
		svc := &inference.InferenceService{CanaryWeight: 100}
		if svc.IsCanaryActive() {
			t.Error("expected canary not active at 100%")
		}
	})
}

func TestRuntimeKindConstants(t *testing.T) {
	tests := []struct {
		name  string
		value inference.RuntimeKind
	}{
		{"vllm", inference.RuntimeVLLM},
		{"triton", inference.RuntimeTriton},
		{"kserve", inference.RuntimeKServe},
		{"ascend", inference.RuntimeAscend},
		{"custom", inference.RuntimeCustom},
	}
	for _, tc := range tests {
		if string(tc.value) != tc.name {
			t.Errorf("expected %s, got %s", tc.name, tc.value)
		}
	}
}

func TestInferenceStatusConstants(t *testing.T) {
	tests := []struct {
		name  string
		value inference.InferenceStatus
	}{
		{"pending", inference.InferenceStatusPending},
		{"scaling_up", inference.InferenceStatusScalingUp},
		{"ready", inference.InferenceStatusReady},
		{"scaling", inference.InferenceStatusScaling},
		{"degraded", inference.InferenceStatusDegraded},
		{"offline", inference.InferenceStatusOffline},
		{"failed", inference.InferenceStatusFailed},
		{"unknown", inference.InferenceStatusUnknown},
	}
	for _, tc := range tests {
		if string(tc.value) != tc.name {
			t.Errorf("expected %s, got %s", tc.name, tc.value)
		}
	}
}