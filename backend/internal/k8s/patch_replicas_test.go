package k8s

import (
	"encoding/json"
	"testing"

	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/types"
)

func isvcFixture() []byte {
	return []byte(`{
	  "apiVersion": "serving.kserve.io/v1beta1",
	  "kind": "InferenceService",
	  "metadata": {"name": "demo-isvc", "namespace": "fuze-ai-paas"},
	  "spec": {
	    "predictor": {
	      "minReplicas": 1,
	      "canaryTrafficPercent": 0,
	      "model": {
	        "storageUri": "pvc://models/demo",
	        "resources": {
	          "limits": {"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "2"}
	        }
	      }
	    }
	  }
	}`)
}

func applyToFixture(t *testing.T, payload []byte) map[string]interface{} {
	t.Helper()
	patch, err := jsonpatch.DecodePatch(payload)
	if err != nil {
		t.Fatalf("payload 不是合法 JSON Patch: %v", err)
	}
	out, err := patch.Apply(isvcFixture())
	if err != nil {
		t.Fatalf("Apply 失败: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("结果不是合法 JSON: %v", err)
	}
	return got["spec"].(map[string]interface{})["predictor"].(map[string]interface{})
}

func assertUntouchedFields(t *testing.T, predictor map[string]interface{}) {
	t.Helper()
	model, ok := predictor["model"].(map[string]interface{})
	if !ok {
		t.Fatal("model 被抹掉了")
	}
	if uri, _ := model["storageUri"].(string); uri != "pvc://models/demo" {
		t.Fatalf("storageUri 丢失，实际 %q", uri)
	}
	res, ok := model["resources"].(map[string]interface{})
	if !ok {
		t.Fatal("resources 被抹掉了")
	}
	limits, ok := res["limits"].(map[string]interface{})
	if !ok {
		t.Fatal("resources.limits 被抹掉了")
	}
	if g, _ := limits["nvidia.com/gpu"].(string); g != "2" {
		t.Fatalf("GPU limits 丢失，实际 %q", g)
	}
}

func TestReplicasPatchOnlyTouchesMinReplicas(t *testing.T) {
	patchType, payload, err := replicasPatch(5)
	if err != nil {
		t.Fatalf("replicasPatch 返回错误: %v", err)
	}
	if patchType != types.JSONPatchType {
		t.Fatalf("必须使用 JSONPatchType，实际 %q（MergePatchType 会整体替换数组）", patchType)
	}

	predictor := applyToFixture(t, payload)

	if r, ok := predictor["minReplicas"].(float64); !ok || int(r) != 5 {
		t.Fatalf("minReplicas 应为 5，实际 %v", predictor["minReplicas"])
	}
	assertUntouchedFields(t, predictor)

	if pct, ok := predictor["canaryTrafficPercent"].(float64); !ok || int(pct) != 0 {
		t.Fatalf("扩缩容不应修改 canaryTrafficPercent，实际 %v", predictor["canaryTrafficPercent"])
	}
}

func TestReplicasPatchRejectsNegative(t *testing.T) {
	if _, _, err := replicasPatch(-1); err == nil {
		t.Fatal("副本数为负应返回错误")
	}
}

func TestReplicasPatchAllowsZero(t *testing.T) {
	if _, _, err := replicasPatch(0); err != nil {
		t.Fatalf("缩容到 0 应被允许: %v", err)
	}
}

func TestCanaryPatchOnlyTouchesTrafficPercent(t *testing.T) {
	_, payload, err := canaryPatch(30)
	if err != nil {
		t.Fatalf("canaryPatch 返回错误: %v", err)
	}

	predictor := applyToFixture(t, payload)

	if pct, ok := predictor["canaryTrafficPercent"].(float64); !ok || int(pct) != 30 {
		t.Fatalf("canaryTrafficPercent 应为 30，实际 %v", predictor["canaryTrafficPercent"])
	}
	assertUntouchedFields(t, predictor)

	if r, ok := predictor["minReplicas"].(float64); !ok || int(r) != 1 {
		t.Fatalf("灰度不应修改 minReplicas，实际 %v", predictor["minReplicas"])
	}
}