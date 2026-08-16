package runtime

import (
	"testing"

	"fuze-ai-paas/backend/internal/domain/inference"
	"fuze-ai-paas/backend/internal/k8s"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRegistryDispatch(t *testing.T) {
	reg := NewDefaultRegistry()
	
	dummyClient := &k8s.Client{}
	for _, kind := range []inference.RuntimeKind{
		inference.RuntimeVLLM, inference.RuntimeTriton,
		inference.RuntimeKServe, inference.RuntimeCustom, inference.RuntimeAscend,
	} {
		rt, err := reg.For("cluster-1", kind, dummyClient)
		if err != nil {
			t.Fatalf("kind=%s 应可解析: %v", kind, err)
		}
		if rt == nil {
			t.Fatalf("kind=%s 返回 nil 客户端", kind)
		}
	}

	if _, err := reg.For("cluster-1", inference.RuntimeKind("bogus"), dummyClient); err == nil {
		t.Error("未知运行时应返回错误")
	}
}

func TestDeploymentManifestChipAnnotations(t *testing.T) {
	d := deploymentRuntime{port: 8000, defaultImage: "img:latest"}

	svc := &inference.InferenceService{
		Name: "llm", Chip: "Ascend", GPUs: 1, GPUMemory: 32, GPUCores: 20,
		MinReplicas: 1, CPU: "4", Memory: "16Gi",
	}
	dep := d.buildDeploymentObject(svc, "llm", "default")
	ann := podAnnotations(t, dep)
	if ann["ascend.com/vnpu"] != "1" {
		t.Errorf("Ascend 注解应为 ascend.com/vnpu=1，实际 %v", ann)
	}
	if _, ok := ann["nvidia.com/gpu"]; ok {
		t.Errorf("Ascend 不应含 nvidia.com/gpu，实际 %v", ann)
	}

	svc2 := &inference.InferenceService{Name: "llm2", GPUs: 2, GPUMemory: 40, GPUCores: 80}
	dep2 := d.buildDeploymentObject(svc2, "llm2", "default")
	ann2 := podAnnotations(t, dep2)
	if ann2["nvidia.com/gpu"] != "2" {
		t.Errorf("NVIDIA 注解应为 nvidia.com/gpu=2，实际 %v", ann2)
	}
	if ann2["nvidia.com/gpumem"] != "40" {
		t.Errorf("NVIDIA 注解应为 nvidia.com/gpumem=40，实际 %v", ann2)
	}
}

func podAnnotations(t *testing.T, dep *unstructured.Unstructured) map[string]interface{} {
	t.Helper()
	tmpl, found, err := unstructured.NestedMap(dep.Object, "spec", "template")
	if err != nil || !found {
		t.Fatalf("template 缺失: found=%v err=%v", found, err)
	}
	md, _, _ := unstructured.NestedMap(tmpl, "metadata")
	ann, _, _ := unstructured.NestedMap(md, "annotations")
	if ann == nil {
		ann = map[string]interface{}{}
	}
	return ann
}