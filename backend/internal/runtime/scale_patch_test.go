package runtime

import (
	"encoding/json"
	"testing"

	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/types"
)

func TestScalePatchTypeMatchesPayload(t *testing.T) {
	patchType, payload := scalePatch(3)

	if patchType != types.JSONPatchType {
		t.Fatalf("patch 体是 JSON Patch 数组，patch 类型必须为 JSONPatchType，实际 %q", patchType)
	}

	var ops []map[string]interface{}
	if err := json.Unmarshal(payload, &ops); err != nil {
		t.Fatalf("payload 不是合法 JSON Patch 数组: %v", err)
	}
	if _, err := jsonpatch.DecodePatch(payload); err != nil {
		t.Fatalf("payload 无法被 JSON Patch 解析: %v", err)
	}
}

func TestScalePatchPreservesDeploymentSpec(t *testing.T) {
	original := []byte(`{
	  "apiVersion": "apps/v1",
	  "kind": "Deployment",
	  "metadata": {"name": "vllm-demo", "namespace": "fuze-ai-paas"},
	  "spec": {
	    "replicas": 1,
	    "selector": {"matchLabels": {"app": "vllm-demo"}},
	    "template": {
	      "metadata": {"labels": {"app": "vllm-demo"}},
	      "spec": {"containers": [{"name": "server", "image": "vllm/vllm-openai:latest"}]}
	    }
	  }
	}`)

	_, payload := scalePatch(4)
	patch, err := jsonpatch.DecodePatch(payload)
	if err != nil {
		t.Fatalf("DecodePatch 失败: %v", err)
	}
	out, err := patch.Apply(original)
	if err != nil {
		t.Fatalf("Apply 失败: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("结果不是合法 JSON: %v", err)
	}
	spec := got["spec"].(map[string]interface{})

	if r := spec["replicas"].(float64); int(r) != 4 {
		t.Fatalf("replicas 未更新为 4，实际 %v", r)
	}
	if _, ok := spec["selector"]; !ok {
		t.Fatal("selector 被抹掉了")
	}
	tmpl, ok := spec["template"].(map[string]interface{})
	if !ok {
		t.Fatal("template 被抹掉了")
	}
	podSpec := tmpl["spec"].(map[string]interface{})
	containers, ok := podSpec["containers"].([]interface{})
	if !ok || len(containers) != 1 {
		t.Fatal("containers 被抹掉了")
	}
	if img := containers[0].(map[string]interface{})["image"].(string); img != "vllm/vllm-openai:latest" {
		t.Fatalf("container image 丢失，实际 %q", img)
	}
}