package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/types"
)

func TestCanaryPatchUsesJSONPatchType(t *testing.T) {
	patchType, payload, err := canaryPatch(30)
	if err != nil {
		t.Fatalf("canaryPatch 返回错误: %v", err)
	}
	if patchType != types.JSONPatchType {
		t.Fatalf("必须使用 JSONPatchType 精确定位，实际 %q", patchType)
	}
	if len(payload) == 0 {
		t.Fatal("payload 为空")
	}
}

func TestCanaryPatchRejectsOutOfRangeWeight(t *testing.T) {
	for _, w := range []int{-1, 101, 999} {
		if _, _, err := canaryPatch(w); err == nil {
			t.Fatalf("权重 %d 越界，应返回错误", w)
		}
	}
}

func TestCanaryPatchAllowsBoundaryWeights(t *testing.T) {
	for _, w := range []int{0, 100} {
		if _, _, err := canaryPatch(w); err != nil {
			t.Fatalf("权重 %d 合法，不应报错: %v", w, err)
		}
	}
}