package tests

import (
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/k8s"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/telemetry"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello-world", "hello-world"},
		{"Hello World", "hello-world"},
		{"test_name", "test-name"},
		{"test.name", "test.name"},
		{"UPPERCASE", "uppercase"},
		{"special!@#chars", "special---chars"},
		{"---leading", "leading"},
		{"trailing---", "trailing"},
		{"", "task"},
		{"!!!", "task"},
		{"123abc", "123abc"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := k8s.SanitizeName(tc.input)
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestVolcanoJobGVR(t *testing.T) {
	gvr := k8s.VolcanoJobGVR()
	if gvr.Group != "batch.volcano.sh" {
		t.Errorf("expected group batch.volcano.sh, got %s", gvr.Group)
	}
	if gvr.Version != "v1alpha1" {
		t.Errorf("expected version v1alpha1, got %s", gvr.Version)
	}
	if gvr.Resource != "jobs" {
		t.Errorf("expected resource jobs, got %s", gvr.Resource)
	}
}

func TestSetTelemetry(t *testing.T) {
	env := NewTestEnv(t)
	collector := telemetry.NewCollector()
	env.Handler.SetTelemetry(collector)
	
}

func TestSSOEndpointsNotConfigured(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("OIDC start without config returns 404", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/auth/sso/oidc/start")
		
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("OIDC callback without config returns 404", func(t *testing.T) {
		w := env.DoGET(http.MethodGet, "/api/v1/auth/sso/oidc/callback")
		AssertStatus(t, w, http.StatusNotFound)
	})

	t.Run("LDAP login without config returns 404", func(t *testing.T) {
		w := env.DoJSON(http.MethodPost, "/api/v1/auth/sso/ldap", map[string]string{
			"username": "test",
			"password": "pass",
		})
		AssertStatus(t, w, http.StatusNotFound)
	})
}

func TestModelConstants(t *testing.T) {
	t.Run("job type to queue mapping", func(t *testing.T) {
		if models.JobTypeToQueue == nil {
			t.Error("expected non-nil JobTypeToQueue map")
		}
	})

	t.Run("inference framework constants", func(t *testing.T) {
		if models.FrameworkPyTorch != "pytorch" {
			t.Errorf("expected pytorch, got %s", models.FrameworkPyTorch)
		}
		if models.FrameworkTriton != "triton" {
			t.Errorf("expected triton, got %s", models.FrameworkTriton)
		}
		if models.FrameworkCustom != "custom" {
			t.Errorf("expected custom, got %s", models.FrameworkCustom)
		}
	})

	t.Run("inference status constants", func(t *testing.T) {
		if models.InferenceStatusReady != "ready" {
			t.Errorf("expected ready, got %s", models.InferenceStatusReady)
		}
		if models.InferenceStatusPending != "pending" {
			t.Errorf("expected pending, got %s", models.InferenceStatusPending)
		}
		if models.InferenceStatusFailed != "failed" {
			t.Errorf("expected failed, got %s", models.InferenceStatusFailed)
		}
		if models.InferenceStatusUnknown != "unknown" {
			t.Errorf("expected unknown, got %s", models.InferenceStatusUnknown)
		}
	})

	t.Run("resource status constants", func(t *testing.T) {
		if models.ResourceStatusAvailable != "available" {
			t.Errorf("expected available, got %s", models.ResourceStatusAvailable)
		}
		if models.ResourceStatusAllocated != "allocated" {
			t.Errorf("expected allocated, got %s", models.ResourceStatusAllocated)
		}
	})

	t.Run("job status constants", func(t *testing.T) {
		if models.JobStatusPending != "pending" {
			t.Errorf("expected pending, got %s", models.JobStatusPending)
		}
		if models.JobStatusRunning != "running" {
			t.Errorf("expected running, got %s", models.JobStatusRunning)
		}
		if models.JobStatusCompleted != "completed" {
			t.Errorf("expected completed, got %s", models.JobStatusCompleted)
		}
		if models.JobStatusFailed != "failed" {
			t.Errorf("expected failed, got %s", models.JobStatusFailed)
		}
		if models.JobStatusCancelled != "cancelled" {
			t.Errorf("expected cancelled, got %s", models.JobStatusCancelled)
		}
	})
}

func TestGPUDomain(t *testing.T) {
	t.Run("GPU device creation and properties", func(t *testing.T) {
		
	})
}