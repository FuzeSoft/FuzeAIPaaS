package runtime

import (
	"context"
	"fmt"

	"fuze-ai-paas/backend/internal/adapter"
	"fuze-ai-paas/backend/internal/domain/inference"
	"fuze-ai-paas/backend/internal/k8s"
	"fuze-ai-paas/backend/internal/models"
)

type KServeRuntime struct {
	client *k8s.Client
}

func (r *KServeRuntime) Deploy(ctx context.Context, svc *inference.InferenceService) (string, error) {
	if r.client == nil || !r.client.Enabled() {
		return "", fmt.Errorf("k8s client not available")
	}
	m := adapter.InferenceToModel(svc)
	name, err := r.client.CreateInferenceService(ctx, m)
	if err != nil {
		return "", err
	}
	return name, nil
}

func (r *KServeRuntime) Undeploy(ctx context.Context, runtimeName string) error {
	if r.client == nil || !r.client.Enabled() {
		return fmt.Errorf("k8s client not available")
	}
	return r.client.DeleteInferenceService(ctx, runtimeName)
}

func (r *KServeRuntime) Status(ctx context.Context, runtimeName string) (bool, bool, bool, int, string, error) {
	if r.client == nil || !r.client.Enabled() {
		return false, false, false, 0, "", fmt.Errorf("k8s client not available")
	}
	st, reps, url, err := r.client.GetInferenceServiceStatus(ctx, runtimeName)
	if err != nil {
		
		return false, false, false, 0, "", err
	}
	ready := st == models.InferenceStatusReady
	failed := st == models.InferenceStatusFailed
	return ready, true, failed, reps, url, nil
}

func (r *KServeRuntime) Scale(ctx context.Context, runtimeName string, replicas int) error {
	if r.client == nil || !r.client.Enabled() {
		return fmt.Errorf("k8s client not available")
	}
	return r.client.PatchInferenceServiceReplicas(ctx, runtimeName, replicas)
}

func (r *KServeRuntime) RolloutCanary(ctx context.Context, runtimeName string, weight int) error {
	if r.client == nil || !r.client.Enabled() {
		return fmt.Errorf("k8s client not available")
	}
	return r.client.PatchInferenceServiceCanary(ctx, runtimeName, weight)
}