package k8s

import (
	"testing"

	"fuze-ai-paas/backend/internal/models"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGetConditionReady(t *testing.T) {
	tests := []struct {
		name      string
		conds     []any
		wantReady bool
		wantFound bool
		wantFailed bool
	}{
		{
			name:       "ready true",
			conds:      condObjs("Ready", "True"),
			wantReady:  true,
			wantFound:  true,
			wantFailed: false,
		},
		{
			name:       "ready false means failed",
			conds:      condObjs("Ready", "False"),
			wantReady:  false,
			wantFound:  true,
			wantFailed: true,
		},
		{
			name:       "ready unknown means starting (not failed)",
			conds:      condObjs("Ready", "Unknown"),
			wantReady:  false,
			wantFound:  true,
			wantFailed: false,
		},
		{
			name:       "no ready condition present",
			conds:      condObjs("Available", "True"),
			wantReady:  false,
			wantFound:  true,
			wantFailed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: map[string]any{}}
			if tc.conds != nil {
				obj.Object["status"] = map[string]any{"conditions": tc.conds}
			}
			ready, found, failed := getConditionReady(obj)
			if ready != tc.wantReady || found != tc.wantFound || failed != tc.wantFailed {
				t.Fatalf("got (ready=%v found=%v failed=%v), want (%v %v %v)",
					ready, found, failed, tc.wantReady, tc.wantFound, tc.wantFailed)
			}
		})
	}
}

func TestKServeStateToStatus(t *testing.T) {
	if got := models.KServeStateToStatus(true, true, true); got != models.InferenceStatusFailed {
		t.Errorf("failed should win over ready, got %s", got)
	}
	if got := models.KServeStateToStatus(true, true, false); got != models.InferenceStatusReady {
		t.Errorf("ready=true should be ready, got %s", got)
	}
	if got := models.KServeStateToStatus(false, true, false); got != models.InferenceStatusPending {
		t.Errorf("not found but present should be pending, got %s", got)
	}
	if got := models.KServeStateToStatus(false, false, false); got != models.InferenceStatusPending {
		t.Errorf("not found should be pending, got %s", got)
	}
}

func condObjs(pairs ...string) []any {
	out := make([]any, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{
			"type":   pairs[i],
			"status": pairs[i+1],
		})
	}
	return out
}