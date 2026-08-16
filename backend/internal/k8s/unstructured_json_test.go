package k8s

import (
	"testing"

	"fuze-ai-paas/backend/internal/models"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestBuildInferenceServiceObjectIsJSONCompatible(t *testing.T) {
	c := &Client{namespace: "default"}

	cases := []struct {
		name string
		svc  *models.InferenceService
	}{
		{"preset-runtime", &models.InferenceService{
			ID: "svc-1", Name: "resnet", Framework: models.FrameworkONNX,
			StorageURI: "s3://models/resnet", MinReplicas: 1, MaxReplicas: 3,
			GPUs: 1, GPUMemory: 8192, GPUCores: 30,
		}},
		{"custom-container", &models.InferenceService{
			ID: "svc-2", Name: "llm", Framework: models.FrameworkCustom,
			Image: "repo/vllm:v1", StorageURI: "s3://models/llm",
			MinReplicas: 1, MaxReplicas: 2, GPUs: 2,
		}},
		{"cpu-only", &models.InferenceService{
			ID: "svc-3", Name: "sklearn-svc", Framework: models.FrameworkSKLearn,
			StorageURI: "s3://models/sk", MinReplicas: 1, MaxReplicas: 1,
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := c.BuildInferenceServiceObject(tc.svc, "")
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("InferenceService 含非 JSON 兼容类型，下发时会 panic: %v", r)
				}
			}()
			runtime.DeepCopyJSON(obj.Object)
		})
	}
}