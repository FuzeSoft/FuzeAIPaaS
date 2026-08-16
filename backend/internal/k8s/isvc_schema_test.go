package k8s

import (
	"testing"

	"fuze-ai-paas/backend/internal/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestInferenceServiceUsesOfficialPredictorSchema(t *testing.T) {
	svc := &models.InferenceService{
		Name:        "demo",
		Framework:   models.FrameworkPyTorch,
		StorageURI:  "pvc://models/demo",
		MinReplicas: 1,
		MaxReplicas: 3,
		CPU:         "4",
		Memory:      "16Gi",
	}

	obj := (&Client{}).BuildInferenceServiceObject(svc, "NVIDIA")

	spec, ok := obj.Object["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("spec 不是 map")
	}

	if _, exists := spec["components"]; exists {
		t.Error("spec.components 不是 KServe v1beta1 的合法字段，会被 API Server 剪除")
	}

	predictor, ok := spec["predictor"].(map[string]interface{})
	if !ok {
		t.Fatal("P1-1 NOT FIXED: spec.predictor 缺失（KServe 官方 schema 要求）")
	}

	if r, ok := predictor["minReplicas"].(int64); !ok || r != 1 {
		t.Errorf("minReplicas 应为 1，实际 %v", predictor["minReplicas"])
	}
	model, ok := predictor["model"].(map[string]interface{})
	if !ok {
		t.Fatal("predictor.model 缺失")
	}
	if uri, _ := model["storageUri"].(string); uri != "pvc://models/demo" {
		t.Errorf("storageUri 错误，实际 %v", model["storageUri"])
	}
}

func TestPatchPathMatchesGeneratedStructure(t *testing.T) {
	if predictorPath != "/spec/predictor" {
		t.Fatalf("patch 路径应为 /spec/predictor，实际 %q", predictorPath)
	}

	svc := &models.InferenceService{
		Name: "demo", Framework: models.FrameworkPyTorch,
		StorageURI: "pvc://m/d", MinReplicas: 1, MaxReplicas: 2,
	}
	obj := (&Client{}).BuildInferenceServiceObject(svc, "NVIDIA")

	if _, found, err := unstructured.NestedMap(obj.Object, "spec", "predictor"); err != nil || !found {
		t.Fatalf("patch 目标路径 /spec/predictor 在生成对象中不存在: found=%v err=%v", found, err)
	}
}