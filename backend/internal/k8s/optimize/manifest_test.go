package optimize

import (
	"testing"

	"fuze-ai-paas/backend/internal/domain/optimize"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func newTask(backend optimize.CompressionBackend) *optimize.CompressionTask {
	return optimize.NewCompressionTask("T-1", "tenant-1", "q", optimize.CompressionTypeQuantize, backend, `{"method":"dynamic","bits":8}`, "mv-1")
}

func TestBuildCompressionJobSecurityBaseline(t *testing.T) {
	task := newTask(optimize.BackendPyTorch)
	obj, err := BuildCompressionJob(task, nil)
	if err != nil {
		t.Fatalf("build job: %v", err)
	}
	if err := Snapshot(obj, task, nil); err != nil {
		t.Fatalf("snapshot should pass: %v", err)
	}
}

func TestBuildCompressionJobRejectsUnlistedBackend(t *testing.T) {
	task := optimize.NewCompressionTask("T-2", "tenant-1", "q", optimize.CompressionTypeQuantize, optimize.CompressionBackend("unknown"), "{}", "mv-1")
	if _, err := BuildCompressionJob(task, nil); err == nil {
		t.Fatal("unlisted backend should be rejected")
	}
}

func TestSnapshotRejectsHostPath(t *testing.T) {
	task := newTask(optimize.BackendPyTorch)
	obj, _ := BuildCompressionJob(task, nil)
	
	spec := asNested(obj.Object, "spec", "template", "spec")
	vols, _ := spec["volumes"].([]interface{})
	spec["volumes"] = append(vols, map[string]interface{}{"name": "evil", "hostPath": map[string]interface{}{"path": "/etc"}})
	if err := Snapshot(obj, task, nil); err == nil {
		t.Fatal("snapshot must reject hostPath volume")
	}
}

func TestSnapshotRejectsPrivileged(t *testing.T) {
	task := newTask(optimize.BackendPyTorch)
	obj, _ := BuildCompressionJob(task, nil)
	spec := asNested(obj.Object, "spec", "template", "spec")
	containers, _ := spec["containers"].([]interface{})
	c := containers[0].(map[string]interface{})
	sc := c["securityContext"].(map[string]interface{})
	sc["privileged"] = true
	if err := Snapshot(obj, task, nil); err == nil {
		t.Fatal("snapshot must reject privileged container")
	}
}

func TestSnapshotRejectsImageNotInWhitelist(t *testing.T) {
	task := newTask(optimize.BackendPyTorch)
	obj, _ := BuildCompressionJob(task, map[optimize.CompressionBackend]string{
		optimize.BackendPyTorch: "registry.fuze.ai/optimize/pytorch:latest",
	})
	
	spec := asNested(obj.Object, "spec", "template", "spec")
	containers, _ := spec["containers"].([]interface{})
	c := containers[0].(map[string]interface{})
	c["image"] = "evil.io/backdoor:latest"
	if err := Snapshot(obj, task, map[optimize.CompressionBackend]string{
		optimize.BackendPyTorch: "registry.fuze.ai/optimize/pytorch:latest",
	}); err == nil {
		t.Fatal("snapshot must reject image not in whitelist")
	}
}

func TestGetBackendImageOverrideAndDefault(t *testing.T) {
	if img, err := GetBackendImage(optimize.BackendPyTorch, map[optimize.CompressionBackend]string{optimize.BackendPyTorch: "custom:1"}); err != nil || img != "custom:1" {
		t.Fatalf("override failed: %q %v", img, err)
	}
	if img, err := GetBackendImage(optimize.BackendONNXRuntime, nil); err != nil || img == "" {
		t.Fatalf("default fallback failed: %q %v", img, err)
	}
	if _, err := GetBackendImage(optimize.CompressionBackend("nope"), nil); err == nil {
		t.Fatal("unlisted backend should error")
	}
}

func TestBuildCompressionJobNamingAndLabels(t *testing.T) {
	task := newTask(optimize.BackendOpenVINO)
	obj, _ := BuildCompressionJob(task, nil)
	name := obj.GetName()
	if name == "" || name[:len(jobNamePrefix)] != jobNamePrefix {
		t.Fatalf("job name should carry prefix: %q", name)
	}
	labels, _, _ := unstructured.NestedStringMap(obj.Object, "metadata", "labels")
	if labels["fuze.ai/task-id"] != task.ID {
		t.Fatalf("label task-id mismatch: %v", labels)
	}
}