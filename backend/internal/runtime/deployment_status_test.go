package runtime

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDeploymentFailedConditions(t *testing.T) {
	tests := []struct {
		name    string
		conds   []any
		wantFail bool
	}{
		{
			name:    "progressing false means rollout failed",
			conds:   conds("Progressing", "False"),
			wantFail: true,
		},
		{
			name:    "progressing true is not failed (still rolling out)",
			conds:   conds("Progressing", "True"),
			wantFail: false,
		},
		{
			name:    "progressing unknown is not failed (starting)",
			conds:   conds("Progressing", "Unknown"),
			wantFail: false,
		},
		{
			name: "available false alone is not failed (initial rollout)",
			conds: conds("Available", "False", "Progressing", "True"),
			wantFail: false,
		},
		{
			name:    "no conditions is not failed",
			conds:   nil,
			wantFail: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: map[string]any{}}
			if tc.conds != nil {
				obj.Object["status"] = map[string]any{"conditions": tc.conds}
			}
			if got := deploymentFailed(obj); got != tc.wantFail {
				t.Fatalf("deploymentFailed=%v, want %v", got, tc.wantFail)
			}
		})
	}
}

func conds(pairs ...string) []any {
	out := make([]any, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{
			"type":   pairs[i],
			"status": pairs[i+1],
		})
	}
	return out
}