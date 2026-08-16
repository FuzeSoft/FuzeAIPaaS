package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type specView struct {
	Name           string `json:"name"`
	ClusterID      string `json:"cluster_id"`
	Framework      string `json:"framework"`
	Runtime        string `json:"runtime"`
	StorageURI     string `json:"storage_uri"`
	Image          string `json:"image"`
	RuntimeVersion string `json:"runtime_version"`
	MinReplicas    int    `json:"min_replicas"`
	MaxReplicas    int    `json:"max_replicas"`
	CPU            string `json:"cpu"`
	Memory         string `json:"memory"`
	GPUs           int    `json:"gpus"`
	TargetReplicas int    `json:"target_replicas"`
	CanaryWeight   int    `json:"canary_weight"`
}

type statusView struct {
	Phase         string `json:"phase"`
	URL           string `json:"url"`
	RuntimeName   string `json:"runtime_name"`
	ReadyReplicas int    `json:"ready_replicas"`
}

type svcView struct {
	ID       string     `json:"id"`
	TenantID string     `json:"tenant_id"`
	Spec     specView   `json:"spec"`
	Status   statusView `json:"status"`
}

func rawBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var raw map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("parse raw body: %v, body: %s", err, w.Body.String())
	}
	return raw
}

func applySpec(name string, extra map[string]interface{}) map[string]interface{} {
	spec := map[string]interface{}{
		"name":         name,
		"cluster_id":   "cluster-001",
		"framework":    "vllm",
		"storage_uri":  "s3://models/" + name,
		"min_replicas": 1,
		"max_replicas": 3,
		"cpu":          "4",
		"memory":       "16Gi",
		"gpus":         1,
	}
	for k, v := range extra {
		spec[k] = v
	}
	return map[string]interface{}{"spec": spec}
}

func createV2(t *testing.T, env *TestEnv, name string) svcView {
	t.Helper()
	w := env.DoJSON(http.MethodPost, "/api/v1/inference-services", applySpec(name, nil))
	AssertStatus(t, w, http.StatusCreated)
	return ParseJSON[svcView](t, w)
}

func TestInferenceViewSplitsSpecAndStatus(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	w := env.DoJSON(http.MethodPost, "/api/v1/inference-services", applySpec("view-svc", nil))
	AssertStatus(t, w, http.StatusCreated)

	raw := rawBody(t, w)
	for _, k := range []string{"spec", "status", "id"} {
		if _, ok := raw[k]; !ok {
			t.Fatalf("expected key %q in view, got: %s", k, w.Body.String())
		}
	}
	for _, k := range []string{"name", "ready_replicas", "target_replicas", "kserve_name", "url"} {
		if _, ok := raw[k]; ok {
			t.Fatalf("flat field %q must not be exposed at top level: %s", k, w.Body.String())
		}
	}

	got := ParseJSON[svcView](t, w)
	if got.Spec.Name != "view-svc" || got.Spec.GPUs != 1 || got.Spec.MinReplicas != 1 {
		t.Fatalf("spec not mapped correctly: %+v", got.Spec)
	}
	if got.ID == "" {
		t.Fatal("expected id in view")
	}

	wl := env.DoGET(http.MethodGet, "/api/v1/inference-services")
	AssertStatus(t, wl, http.StatusOK)
	if countByName(t, wl, "view-svc") != 1 {
		t.Fatalf("list should return spec/status views, got: %s", wl.Body.String())
	}
}

func countByName(t *testing.T, w *httptest.ResponseRecorder, name string) int {
	t.Helper()
	list := ParseJSON[[]svcView](t, w)
	n := 0
	for _, v := range list {
		if v.Spec.Name == name {
			n++
		}
	}
	return n
}

func TestCreateWritesDesiredStateOnly(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	got := createV2(t, env, "desired-only")
	if got.Status.Phase != "pending" {
		t.Fatalf("expected phase pending right after create, got %q", got.Status.Phase)
	}
	if got.Status.RuntimeName != "" || got.Status.URL != "" {
		t.Fatalf("create must not deploy in request path, got status %+v", got.Status)
	}
	
	if got.Spec.TargetReplicas != 1 {
		t.Fatalf("expected target_replicas defaulted to min_replicas=1, got %d", got.Spec.TargetReplicas)
	}

	env.Scheduler.ReconcileInference(context.Background())

	w := env.DoGET(http.MethodGet, "/api/v1/inference-services/"+got.ID)
	AssertStatus(t, w, http.StatusOK)
	after := ParseJSON[svcView](t, w)
	if after.Status.Phase != "ready" {
		t.Fatalf("expected reconcile to deploy service, got phase %q", after.Status.Phase)
	}
	if after.Status.RuntimeName == "" || after.Status.URL == "" {
		t.Fatalf("expected runtime observed state after reconcile, got %+v", after.Status)
	}
}

func TestClientCannotWriteStatus(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	body := applySpec("readonly-status", nil)
	body["status"] = map[string]interface{}{
		"phase":          "ready",
		"ready_replicas": 9,
		"url":            "http://forged",
	}
	w := env.DoJSON(http.MethodPost, "/api/v1/inference-services", body)
	AssertStatus(t, w, http.StatusCreated)

	got := ParseJSON[svcView](t, w)
	if got.Status.Phase != "pending" || got.Status.ReadyReplicas != 0 || got.Status.URL != "" {
		t.Fatalf("client-supplied status must be ignored, got %+v", got.Status)
	}
}

func TestApplyByNameIsIdempotent(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	w1 := env.DoJSON(http.MethodPut, "/api/v1/inference-services", applySpec("apply-svc", nil))
	AssertStatus(t, w1, http.StatusCreated)
	first := ParseJSON[svcView](t, w1)

	w2 := env.DoJSON(http.MethodPut, "/api/v1/inference-services", applySpec("apply-svc", map[string]interface{}{
		"target_replicas": 3,
		"max_replicas":    5,
	}))
	AssertStatus(t, w2, http.StatusOK)
	second := ParseJSON[svcView](t, w2)

	if second.ID != first.ID {
		t.Fatalf("apply must be idempotent by name: id changed %s -> %s", first.ID, second.ID)
	}
	if second.Spec.TargetReplicas != 3 || second.Spec.MaxReplicas != 5 {
		t.Fatalf("apply must overwrite desired state, got %+v", second.Spec)
	}

	wl := env.DoGET(http.MethodGet, "/api/v1/inference-services")
	AssertStatus(t, wl, http.StatusOK)
	if n := countByName(t, wl, "apply-svc"); n != 1 {
		t.Fatalf("apply must not duplicate resources, got %d", n)
	}
}

func TestLegacyActionEndpointsRemoved(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)
	svc := createV2(t, env, "legacy-svc")

	cases := []struct {
		path string
		body map[string]interface{}
	}{
		{"/scale", map[string]interface{}{"replicas": 3}},
		{"/canary", map[string]interface{}{"weight": 20}},
		{"/status", nil},
	}
	for _, c := range cases {
		w := env.DoJSON(http.MethodPost, "/api/v1/inference-services/"+svc.ID+c.path, c.body)
		if w.Code != http.StatusNotFound {
			t.Fatalf("legacy endpoint %s must be removed, got %d", c.path, w.Code)
		}
	}
}

func TestPatchAcceptsDesiredStateSubset(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)
	svc := createV2(t, env, "patch-subset")

	w := env.DoJSON(http.MethodPatch, "/api/v1/inference-services/"+svc.ID, map[string]interface{}{
		"spec": map[string]interface{}{
			"target_replicas": 4,
			"max_replicas":    6,
			"image":           "repo/vllm:0.5.0",
		},
	})
	AssertStatus(t, w, http.StatusOK)
	got := ParseJSON[svcView](t, w)
	if got.Spec.TargetReplicas != 4 || got.Spec.MaxReplicas != 6 || got.Spec.Image != "repo/vllm:0.5.0" {
		t.Fatalf("patch did not apply desired state: %+v", got.Spec)
	}
	
	if got.Spec.MinReplicas != 1 || got.Spec.GPUs != 1 {
		t.Fatalf("patch must not touch unspecified fields: %+v", got.Spec)
	}
	
	if got.Status.ReadyReplicas == 4 {
		t.Fatal("patch must not converge observed state in request path")
	}
}

func TestPatchRejectsInvalidAndImmutableFields(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)
	svc := createV2(t, env, "patch-invalid")
	base := "/api/v1/inference-services/" + svc.ID

	cases := []struct {
		name string
		body map[string]interface{}
	}{
		{"gpus is immutable via patch", map[string]interface{}{"spec": map[string]interface{}{"gpus": 4}}},
		{"cpu is immutable via patch", map[string]interface{}{"spec": map[string]interface{}{"cpu": "8"}}},
		{"canary weight out of range", map[string]interface{}{"spec": map[string]interface{}{"canary_weight": 150}}},
		{"negative target replicas", map[string]interface{}{"spec": map[string]interface{}{"target_replicas": -1}}},
	}
	for _, c := range cases {
		w := env.DoJSON(http.MethodPatch, base, c.body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d, body: %s", c.name, w.Code, w.Body.String())
		}
	}

	w := env.DoJSON(http.MethodPatch, "/api/v1/inference-services/does-not-exist", map[string]interface{}{
		"spec": map[string]interface{}{"target_replicas": 1},
	})
	AssertStatus(t, w, http.StatusNotFound)
}

func TestApplyDriftTriggersRedeploy(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	w1 := env.DoJSON(http.MethodPut, "/api/v1/inference-services", applySpec("drift-svc", nil))
	AssertStatus(t, w1, http.StatusCreated)
	id := ParseJSON[svcView](t, w1).ID

	env.Scheduler.ReconcileInference(context.Background())
	w0 := env.DoGET(http.MethodGet, "/api/v1/inference-services/"+id)
	AssertStatus(t, w0, http.StatusOK)
	if ParseJSON[svcView](t, w0).Status.RuntimeName == "" {
		t.Fatal("expected runtime_name populated after initial reconcile")
	}

	w2 := env.DoJSON(http.MethodPut, "/api/v1/inference-services", applySpec("drift-svc", map[string]interface{}{
		"image": "repo/vllm:9.9.9",
	}))
	AssertStatus(t, w2, http.StatusOK)
	
	if ParseJSON[svcView](t, w2).Status.RuntimeName != "" {
		t.Fatalf("expected runtime_name cleared on drift, got %q", ParseJSON[svcView](t, w2).Status.RuntimeName)
	}

	env.Scheduler.ReconcileInference(context.Background())
	w3 := env.DoGET(http.MethodGet, "/api/v1/inference-services/"+id)
	AssertStatus(t, w3, http.StatusOK)
	after := ParseJSON[svcView](t, w3)
	if after.Status.Phase != "ready" || after.Status.RuntimeName == "" {
		t.Fatalf("expected redeploy after drift, got %+v", after.Status)
	}
	if after.Spec.Image != "repo/vllm:9.9.9" {
		t.Fatalf("expected new image in spec, got %q", after.Spec.Image)
	}
}

func TestPatchImageTriggersRedeploy(t *testing.T) {
	env := NewTestEnv(t)
	env.EnsureDefaultQuota(t, 64, 1024, 10)
	svc := createV2(t, env, "patch-drift")

	env.Scheduler.ReconcileInference(context.Background())
	w0 := env.DoGET(http.MethodGet, "/api/v1/inference-services/"+svc.ID)
	AssertStatus(t, w0, http.StatusOK)
	if ParseJSON[svcView](t, w0).Status.RuntimeName == "" {
		t.Fatal("expected runtime_name after reconcile")
	}

	wp := env.DoJSON(http.MethodPatch, "/api/v1/inference-services/"+svc.ID, map[string]interface{}{
		"spec": map[string]interface{}{"image": "repo/vllm:7.7.7"},
	})
	AssertStatus(t, wp, http.StatusOK)
	if ParseJSON[svcView](t, wp).Status.RuntimeName != "" {
		t.Fatalf("expected runtime_name cleared on image drift, got %q", ParseJSON[svcView](t, wp).Status.RuntimeName)
	}

	env.Scheduler.ReconcileInference(context.Background())
	w1 := env.DoGET(http.MethodGet, "/api/v1/inference-services/"+svc.ID)
	AssertStatus(t, w1, http.StatusOK)
	after := ParseJSON[svcView](t, w1)
	if after.Status.Phase != "ready" || after.Status.RuntimeName == "" {
		t.Fatalf("expected redeploy after image patch drift, got %+v", after.Status)
	}
	if after.Spec.Image != "repo/vllm:7.7.7" {
		t.Fatalf("expected new image in spec, got %q", after.Spec.Image)
	}
}