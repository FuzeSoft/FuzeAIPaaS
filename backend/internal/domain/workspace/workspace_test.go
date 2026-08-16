package workspace

import (
	"strings"
	"testing"
	"time"
)

func validWorkspace() Workspace {
	return Workspace{
		TenantID: "t-1",
		OwnerID:  "u-1",
		Name:     "my-notebook",
		Kind:     KindJupyter,
		Image:    "jupyter/base-notebook:latest",
		Resources: ResourceSpec{
			CPU:    2,
			Memory: 8,
		},
		Storage: StorageSpec{SizeGB: 20},
	}
}

func TestNormalizeTrimsAndDefaults(t *testing.T) {
	w := Workspace{
		Name:  "  my-notebook  ",
		Kind:  "  ",
		Image: "  jupyter/base:1  ",
		Storage: StorageSpec{
			SizeGB:     0,
			StorageCls: "  fast-ssd  ",
		},
	}
	w.Normalize()

	if w.Name != "my-notebook" {
		t.Fatalf("name not trimmed: %q", w.Name)
	}
	if w.Image != "jupyter/base:1" {
		t.Fatalf("image not trimmed: %q", w.Image)
	}
	if w.Kind != KindJupyter {
		t.Fatalf("kind should default to jupyter, got %q", w.Kind)
	}
	if w.Status != StatusPending {
		t.Fatalf("status should default to pending, got %q", w.Status)
	}
	if w.Storage.SizeGB != defaultStorageGB {
		t.Fatalf("storage should default to %d, got %d", defaultStorageGB, w.Storage.SizeGB)
	}
	if w.Storage.StorageCls != "fast-ssd" {
		t.Fatalf("storage class not trimmed: %q", w.Storage.StorageCls)
	}
}

func TestNormalizeForcesIdleTimeoutOnGPUWorkspaces(t *testing.T) {
	w := validWorkspace()
	w.Resources.GPU = 1
	w.IdleTimeout = 0 
	w.Normalize()

	if w.IdleTimeout != maxGPUIdleTimeout {
		t.Fatalf("GPU workspace must get a capped idle timeout, got %v", w.IdleTimeout)
	}

	w = validWorkspace()
	w.Resources.GPU = 1
	w.IdleTimeout = 90 * 24 * time.Hour
	w.Normalize()
	if w.IdleTimeout != maxGPUIdleTimeout {
		t.Fatalf("GPU idle timeout must be capped at %v, got %v", maxGPUIdleTimeout, w.IdleTimeout)
	}

	w = validWorkspace()
	w.Resources.GPU = 1
	w.IdleTimeout = 30 * time.Minute
	w.Normalize()
	if w.IdleTimeout != 30*time.Minute {
		t.Fatalf("in-range GPU idle timeout must be preserved, got %v", w.IdleTimeout)
	}

	w = validWorkspace()
	w.IdleTimeout = 0
	w.Normalize()
	if w.IdleTimeout != 0 {
		t.Fatalf("CPU workspace may opt out of reclaim, got %v", w.IdleTimeout)
	}
}

func TestNormalizeDropsEmptyMounts(t *testing.T) {
	w := validWorkspace()
	w.Mounts = []Mount{
		{Name: "  ds  ", MountPath: "  /data  ", Kind: MountKindDataset},
		{Name: "", MountPath: "/x"},
		{Name: "m", MountPath: "   "},
	}
	w.Normalize()

	if len(w.Mounts) != 1 {
		t.Fatalf("expected 1 valid mount, got %d: %+v", len(w.Mounts), w.Mounts)
	}
	if w.Mounts[0].Name != "ds" || w.Mounts[0].MountPath != "/data" {
		t.Fatalf("mount not trimmed: %+v", w.Mounts[0])
	}
	
	if !w.Mounts[0].ReadOnly {
		t.Fatal("mounts must default to read-only")
	}
}

func TestNormalizeClearsMountsWhenAllInvalid(t *testing.T) {
	w := validWorkspace()
	w.Mounts = []Mount{{Name: "  ", MountPath: "  "}}
	w.Normalize()
	if w.Mounts != nil {
		t.Fatalf("all-invalid mounts must normalize to nil, got %+v", w.Mounts)
	}

	w = validWorkspace()
	w.Mounts = []Mount{}
	w.Normalize()
	if w.Mounts != nil {
		t.Fatalf("empty mounts must normalize to nil, got %+v", w.Mounts)
	}
}

func TestNormalizeDefaultsMountKind(t *testing.T) {
	w := validWorkspace()
	w.Mounts = []Mount{{Name: "ds", MountPath: "/data"}}
	w.Normalize()
	if w.Mounts[0].Kind != MountKindDataset {
		t.Fatalf("mount kind should default to dataset, got %q", w.Mounts[0].Kind)
	}
	if err := w.Validate(); err != nil {
		t.Fatalf("normalized mount rejected: %v", err)
	}
}

func TestValidateAcceptsMinimalWorkspace(t *testing.T) {
	w := validWorkspace()
	w.Normalize()
	if err := w.Validate(); err != nil {
		t.Fatalf("valid workspace rejected: %v", err)
	}
}

func TestValidateRequiredFields(t *testing.T) {
	cases := map[string]func(*Workspace){
		"missing name":      func(w *Workspace) { w.Name = "   " },
		"missing tenant":    func(w *Workspace) { w.TenantID = "" },
		"missing owner":     func(w *Workspace) { w.OwnerID = "" },
		"missing image":     func(w *Workspace) { w.Image = "  " },
		"unknown kind":      func(w *Workspace) { w.Kind = "emacs" },
		"overlong name":     func(w *Workspace) { w.Name = strings.Repeat("x", maxNameLen+1) },
		"negative gpu":      func(w *Workspace) { w.Resources.GPU = -1 },
		"oversized gpu":     func(w *Workspace) { w.Resources.GPU = maxGPUs + 1 },
		"negative cpu":      func(w *Workspace) { w.Resources.CPU = -1 },
		"oversized cpu":     func(w *Workspace) { w.Resources.CPU = maxCPUCores + 1 },
		"negative memory":   func(w *Workspace) { w.Resources.Memory = -1 },
		"oversized memory":  func(w *Workspace) { w.Resources.Memory = maxMemoryGB + 1 },
		"storage too small": func(w *Workspace) { w.Storage.SizeGB = minStorageGB - 1 },
		"storage too large": func(w *Workspace) { w.Storage.SizeGB = maxStorageGB + 1 },
		"negative idle":     func(w *Workspace) { w.IdleTimeout = -time.Minute },
	}
	for name, mutate := range cases {
		w := validWorkspace()
		mutate(&w)
		if err := w.Validate(); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

func TestValidateAcceptsVSCodeKind(t *testing.T) {
	w := validWorkspace()
	w.Kind = KindVSCode
	w.Normalize()
	if err := w.Validate(); err != nil {
		t.Fatalf("vscode workspace rejected: %v", err)
	}
}

func TestValidateRejectsDuplicateMountPaths(t *testing.T) {
	w := validWorkspace()
	w.Mounts = []Mount{
		{Name: "a", MountPath: "/data", Kind: MountKindDataset},
		{Name: "b", MountPath: "/data", Kind: MountKindModel},
	}
	w.Normalize()
	if err := w.Validate(); err == nil {
		t.Fatal("duplicate mount paths must be rejected")
	}
}

func TestValidateRejectsInvalidMounts(t *testing.T) {
	cases := map[string]Mount{
		"relative path": {Name: "a", MountPath: "data", Kind: MountKindDataset},
		"unknown kind":  {Name: "a", MountPath: "/data", Kind: "nfs"},
	}
	for name, m := range cases {
		w := validWorkspace()
		w.Mounts = []Mount{m}
		w.Normalize()
		if err := w.Validate(); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

func TestValidateRejectsMountShadowingHome(t *testing.T) {
	w := validWorkspace()
	w.Mounts = []Mount{{Name: "a", MountPath: HomePath, Kind: MountKindDataset}}
	w.Normalize()
	if err := w.Validate(); err == nil {
		t.Fatal("mounting over the home directory must be rejected")
	}
}

func TestRequiresGPU(t *testing.T) {
	w := validWorkspace()
	if w.RequiresGPU() {
		t.Fatal("cpu-only workspace must not require GPU")
	}
	w.Resources.GPU = 2
	if !w.RequiresGPU() {
		t.Fatal("workspace with GPU must require GPU")
	}
}