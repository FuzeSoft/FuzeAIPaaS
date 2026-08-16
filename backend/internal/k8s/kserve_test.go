package k8s

import (
	"testing"

	"fuze-ai-paas/backend/internal/models"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuildInferenceServiceObjectChipAnnotations(t *testing.T) {
	c := &Client{namespace: "default"}

	nvidiaSvc := &models.InferenceService{
		ID: "svc-1", Name: "llm", Framework: models.FrameworkPyTorch,
		StorageURI: "s3://m/llm", GPUs: 2, GPUMemory: 40, GPUCores: 80,
	}
	obj := c.BuildInferenceServiceObject(nvidiaSvc, "")
	ann := predictorAnnotations(t, obj)
	if ann["nvidia.com/gpu"] != "2" {
		t.Errorf("NVIDIA 注解应为 nvidia.com/gpu=2，实际 %v", ann)
	}

	ascendSvc := &models.InferenceService{
		ID: "svc-2", Name: "llm-a", Framework: models.FrameworkPyTorch,
		StorageURI: "s3://m/llm", GPUs: 1, GPUMemory: 32, GPUCores: 20, Chip: "Ascend",
	}
	objA := c.BuildInferenceServiceObject(ascendSvc, ascendSvc.Chip)
	annA := predictorAnnotations(t, objA)
	if annA["ascend.com/vnpu"] != "1" {
		t.Errorf("Ascend 注解应为 ascend.com/vnpu=1，实际 %v", annA)
	}
	if _, ok := annA["nvidia.com/gpu"]; ok {
		t.Errorf("Ascend 不应包含 nvidia.com/gpu 注解，实际 %v", annA)
	}
}

func TestBuildInferenceServiceObjectSchemaFieldPlacement(t *testing.T) {
	c := &Client{namespace: "default"}

	cases := []struct {
		name      string
		framework models.InferenceFramework
		
		resourcePath []string
	}{
		{"model", models.FrameworkPyTorch, []string{"model", "resources"}},
		{"custom", models.FrameworkCustom, nil}, 
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &models.InferenceService{
				ID: "svc-x", Name: "llm", Framework: tc.framework,
				StorageURI: "s3://m/llm", Image: "repo/img:v1",
				GPUs: 1, GPUMemory: 24, GPUCores: 50,
			}
			obj := c.BuildInferenceServiceObject(svc, "")
			pred, found, err := unstructured.NestedMap(obj.Object, "spec", "predictor")
			if err != nil || !found {
				t.Fatalf("predictor 缺失: found=%v err=%v", found, err)
			}

			if _, ok := pred["resources"]; ok {
				t.Error("predictor 顶层不应存在 resources（非法字段，会被 CRD 剪枝）")
			}
			if _, ok := pred["annotations"]; ok {
				t.Error("predictor 顶层不应存在 annotations（非法字段，会被 CRD 剪枝）")
			}

			ann, found, _ := unstructured.NestedMap(pred, "metadata", "annotations")
			if !found || ann["nvidia.com/gpu"] != "1" {
				t.Errorf("注解应落在 predictor.metadata.annotations 且含 nvidia.com/gpu=1，实际 %v", ann)
			}

			var limits map[string]interface{}
			if tc.resourcePath != nil {
				path := append(tc.resourcePath, "limits") 
				limits, _, _ = unstructured.NestedMap(pred, path...)
			} else {
				containers, _, _ := unstructured.NestedSlice(pred, "containers")
				if len(containers) == 0 {
					t.Fatal("custom framework 应生成 containers")
				}
				ctr, _ := containers[0].(map[string]interface{})
				limits, _, _ = unstructured.NestedMap(ctr, "resources", "limits")
			}
			if limits["nvidia.com/gpu"] != "1" {
				t.Errorf("容器级 limits 应含 nvidia.com/gpu=1，实际 %v", limits)
			}
		})
	}
}

func predictorAnnotations(t *testing.T, obj *unstructured.Unstructured) map[string]interface{} {
	t.Helper()
	pred, found, err := unstructured.NestedMap(obj.Object, "spec", "predictor")
	if err != nil || !found {
		t.Fatalf("predictor 缺失: found=%v err=%v", found, err)
	}
	ann, _, _ := unstructured.NestedMap(pred, "metadata", "annotations")
	if ann == nil {
		ann = map[string]interface{}{}
	}
	return ann
}