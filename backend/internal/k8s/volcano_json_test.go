package k8s

import (
	"context"
	"testing"

	"fuze-ai-paas/backend/internal/models"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestCreateVolcanoJobProducesJSONCompatibleObject(t *testing.T) {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{VolcanoJobGVR(): "JobList"})
	c := &Client{enabled: true, namespace: "fuze-ai-paas", dynamicClient: fake}

	cases := []struct {
		name string
		job  *models.Job
	}{
		{"single", &models.Job{
			ID: "job-1", Name: "sft", Type: models.JobTypeTraining,
			Image: "repo/train:v1", Command: "python train.py",
			Memory: 32, GPUs: 1,
		}},
		{"distributed", &models.Job{
			ID: "job-2", Name: "ddp", Type: models.JobTypeTraining,
			Image: "repo/train:v1", Command: "torchrun train.py",
			Memory: 64, GPUs: 2,
			Distributed: true, Replicas: 2, Framework: "pytorch",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("下发 Volcano Job 发生 panic（存在非 JSON 兼容类型）: %v", r)
				}
			}()
			if _, err := c.CreateVolcanoJob(context.Background(), tc.job); err != nil {
				t.Fatalf("CreateVolcanoJob 失败: %v", err)
			}
		})
	}
}