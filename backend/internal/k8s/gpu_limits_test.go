package k8s

import (
	"testing"

	"fuze-ai-paas/backend/internal/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGPUResourceEntersLimits(t *testing.T) {
	svc := &models.InferenceService{
		ID:          "svc-gpu",
		Name:        "gpu-isvc",
		Framework:   models.FrameworkONNX,
		GPUs:        2,
		GPUMemory:   0,
		GPUCores:    0,
		Chip:        "NVIDIA",
		MinReplicas: 1,
		MaxReplicas: 3,
		CPU:         "1",
		Memory:      "2Gi",
		StorageURI:  "pvc://models/gpu-demo",
		Image:       "registry.example/gpu-predictor:latest",
	}

	obj := (&Client{}).BuildInferenceServiceObject(svc, "NVIDIA")
	got := &unstructured.Unstructured{Object: obj.Object}

	limits, found, err := unstructured.NestedMap(got.Object,
		"spec", "predictor", "model", "resources", "limits")
	if err != nil || !found {
		t.Fatalf("P0-2 NOT FIXED: predictor.model.resources.limits 不存在: found=%v err=%v", found, err)
	}
	
	predictor, _, _ := unstructured.NestedMap(got.Object, "spec", "predictor")
	if _, bad := predictor["resources"]; bad {
		t.Fatalf("predictor 顶层不应存在 resources（KServe schema 无此字段）")
	}
	
	if _, ok := limits["nvidia.com/gpu"]; !ok {
		t.Fatalf("P0-2 NOT FIXED: limits 里没有 nvidia.com/gpu（GPU 资源约束未下发）")
	}
}