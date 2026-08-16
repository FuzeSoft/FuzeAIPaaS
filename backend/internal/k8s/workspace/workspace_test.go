package workspace

import (
	"encoding/json"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/models"
)

func asMap(t *testing.T, v interface{}, what string) map[string]interface{} {
	t.Helper()
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("%s: want map[string]interface{}, got %T", what, v)
	}
	return m
}

func asSlice(t *testing.T, v interface{}, what string) []interface{} {
	t.Helper()
	s, ok := v.([]interface{})
	if !ok {
		t.Fatalf("%s: want []interface{}, got %T", what, v)
	}
	return s
}

func containerOf(t *testing.T, obj map[string]interface{}) map[string]interface{} {
	t.Helper()
	spec := asMap(t, asMap(t, obj["spec"], "spec")["template"], "template")["spec"]
	podSpec := asMap(t, spec, "pod spec")
	containers := asSlice(t, podSpec["containers"], "containers")
	if len(containers) == 0 {
		t.Fatal("no containers")
	}
	return asMap(t, containers[0], "container")
}

func fixture(opts func(*models.Workspace)) *models.Workspace {
	ws := &models.Workspace{
		ID:            "ws-001",
		TenantID:      "tenant-A",
		OwnerID:       "user-1",
		Name:          "My Notebook!",
		Kind:          models.WorkspaceKindNotebook,
		Image:         "registry.example.com/jupyter:latest",
		Status:        models.WorkspaceStatusPending,
		CPURequest:    "2",
		MemoryRequest: "4Gi",
		GPUCount:      1,
		GPUModel:      "nvidia-tesla-v100",
		IdleTimeout:   30 * time.Minute,
	}
	if opts != nil {
		opts(ws)
	}
	return ws
}

func TestBuildWorkspaceManifestStructureAndResources(t *testing.T) {
	obj := BuildWorkspaceManifest(fixture(nil))

	if got := obj.GetKind(); got != "Deployment" {
		t.Fatalf("kind = %q, want Deployment", got)
	}
	md := asMap(t, obj.Object["metadata"], "metadata")
	if got, _ := md["name"].(string); got != "ws-ws-001" {
		t.Fatalf("name = %q, want ws-ws-001 (sanitized)", got)
	}
	if ns, _ := md["namespace"].(string); ns != Namespace {
		t.Fatalf("namespace = %q, want %q", ns, Namespace)
	}

	c := containerOf(t, obj.Object)
	if got, _ := c["image"].(string); got != "registry.example.com/jupyter:latest" {
		t.Fatalf("image = %q", got)
	}
	res := asMap(t, c["resources"], "resources")
	limits := asMap(t, res["limits"], "limits")
	requests := asMap(t, res["requests"], "requests")
	if limits["cpu"] != "2" || limits["memory"] != "4Gi" {
		t.Fatalf("cpu/mem limits wrong: %+v", limits)
	}
	if requests["cpu"] != "2" || requests["memory"] != "4Gi" {
		t.Fatalf("cpu/mem requests wrong: %+v", requests)
	}
}

func TestBuildWorkspaceManifestGPUDeviceResource(t *testing.T) {
	for _, tc := range []struct {
		model string
		want  string
	}{
		{"nvidia-tesla-v100", "nvidia.com/gpu"},
		{"ascend-910b", "ascend.com/vnpu"},
		{"cambricon-mlu370", "cambricon.com/mlu"},
	} {
		ws := fixture(func(w *models.Workspace) {
			w.GPUModel = tc.model
			w.GPUCount = 2
		})
		c := containerOf(t, BuildWorkspaceManifest(ws).Object)
		limits := asMap(t, asMap(t, c["resources"], "resources")["limits"], "limits")
		if got, ok := limits[tc.want].(string); !ok || got != "2" {
			t.Fatalf("%s: want device %s=2, got %+v", tc.model, tc.want, limits)
		}
	}
}

func TestBuildWorkspaceManifestCPUOnlyOmitsDevice(t *testing.T) {
	ws := fixture(func(w *models.Workspace) {
		w.GPUCount = 0
		w.GPUModel = ""
	})
	limits := asMap(t, asMap(t, containerOf(t, BuildWorkspaceManifest(ws).Object)["resources"], "resources")["limits"], "limits")
	for k := range limits {
		if k == "nvidia.com/gpu" || k == "ascend.com/vnpu" || k == "cambricon.com/mlu" {
			t.Fatalf("cpu-only ws must not carry device resource, got %s", k)
		}
	}
}

func TestBuildWorkspaceManifestSecurityBaseline(t *testing.T) {
	c := containerOf(t, BuildWorkspaceManifest(fixture(nil)).Object)
	sc := asMap(t, c["securityContext"], "securityContext")

	runAsNonRoot, _ := sc["runAsNonRoot"].(bool)
	if !runAsNonRoot {
		t.Error("runAsNonRoot must be true")
	}
	privileged, _ := sc["privileged"].(bool)
	if privileged {
		t.Error("privileged must be false")
	}
	readOnlyRoot, _ := sc["readOnlyRootFilesystem"].(bool)
	if !readOnlyRoot {
		t.Error("readOnlyRootFilesystem must be true")
	}
	runAsUser, _ := sc["runAsUser"].(int64)
	if runAsUser != 1000 {
		t.Errorf("runAsUser = %v, want 1000", runAsUser)
	}
	allowPriv, _ := sc["allowPrivilegeEscalation"].(bool)
	if allowPriv {
		t.Error("allowPrivilegeEscalation must be false")
	}
	caps := asMap(t, sc["capabilities"], "capabilities")
	drop := asSlice(t, caps["drop"], "capabilities.drop")
	if len(drop) != 1 || drop[0] != "ALL" {
		t.Errorf("capabilities.drop must be [ALL], got %+v", drop)
	}
}

func TestBuildWorkspaceManifestPVCAndReaperEnv(t *testing.T) {
	obj := BuildWorkspaceManifest(fixture(nil)).Object
	podSpec := asMap(t, asMap(t, asMap(t, obj["spec"], "spec")["template"], "template")["spec"], "pod spec")

	vols := asSlice(t, podSpec["volumes"], "volumes")
	var foundPVC bool
	for _, raw := range vols {
		v := asMap(t, raw, "volume")
		if pvc, ok := v["persistentVolumeClaim"].(map[string]interface{}); ok {
			if pvc["claimName"] == "ws-ws-001-home" && pvc["readOnly"] == false {
				foundPVC = true
			}
		}
	}
	if !foundPVC {
		t.Fatalf("home PVC volume not found / wrong claimName or not R/W: %+v", vols)
	}

	c := containerOf(t, obj)
	mounts := asSlice(t, c["volumeMounts"], "volumeMounts")
	var homeMounted bool
	for _, raw := range mounts {
		m := asMap(t, raw, "mount")
		if m["mountPath"] == "/home/jovyan" {
			homeMounted = true
		}
	}
	if !homeMounted {
		t.Fatal("home directory not mounted from PVC")
	}

	env := asSlice(t, c["env"], "env")
	var idleSet bool
	for _, raw := range env {
		e := asMap(t, raw, "env")
		if e["name"] == "FUZE_IDLE_TIMEOUT_SECONDS" {
			idleSet = true
			if e["value"] != "1800" { 
				t.Errorf("idle timeout value = %v, want 1800", e["value"])
			}
		}
	}
	if !idleSet {
		t.Fatal("FUZE_IDLE_TIMEOUT_SECONDS env not injected")
	}
}

func TestBuildWorkspaceManifestDefaultsApplied(t *testing.T) {
	ws := fixture(func(w *models.Workspace) {
		w.CPURequest = ""
		w.MemoryRequest = ""
		w.GPUModel = ""
		w.GPUCount = 1
	})
	limits := asMap(t, asMap(t, containerOf(t, BuildWorkspaceManifest(ws).Object)["resources"], "resources")["limits"], "limits")
	if limits["cpu"] != "1" || limits["memory"] != "2Gi" {
		t.Fatalf("defaults wrong: %+v", limits)
	}
	if limits["nvidia.com/gpu"] != "1" {
		t.Fatalf("default vendor should be nvidia: %+v", limits)
	}
}

func TestSnapshotRejectInsecureBaseline(t *testing.T) {
	obj := BuildWorkspaceManifest(fixture(nil))
	
	c := containerOf(t, obj.Object)
	sc := asMap(t, c["securityContext"], "securityContext")
	delete(sc, "runAsNonRoot")
	c["securityContext"] = sc
	if err := Snapshot(obj); err == nil {
		t.Fatal("Snapshot must reject manifest missing runAsNonRoot")
	}

	if err := Snapshot(BuildWorkspaceManifest(fixture(nil))); err != nil {
		t.Fatalf("Snapshot of baseline manifest should pass, got %v", err)
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"My Notebook!": "my-notebook",
		"UP_case.X":    "up-case.x",
		"--weird--":    "weird",
		"":             "ws",
		"already-fine": "already-fine",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSnapshotRejectsMissingSecurityContext(t *testing.T) {
	obj := BuildWorkspaceManifest(fixture(nil))
	c := containerOf(t, obj.Object)
	delete(c, "securityContext")
	if err := Snapshot(obj); err == nil {
		t.Fatal("Snapshot must reject manifest with missing container securityContext")
	}
}

func TestBuildWorkspaceManifestOutputsValidUnstructured(t *testing.T) {
	obj := BuildWorkspaceManifest(fixture(nil))
	if _, err := json.Marshal(obj.Object); err != nil {
		t.Fatalf("manifest not JSON-serializable: %v", err)
	}
}