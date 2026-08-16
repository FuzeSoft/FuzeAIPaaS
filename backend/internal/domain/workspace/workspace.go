
package workspace

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	KindJupyter = "jupyter"
	KindVSCode  = "vscode"
)

const (
	MountKindDataset = "dataset"
	MountKindModel   = "model"
)

const HomePath = "/home/jovyan"

const (
	maxNameLen  = 200
	maxGPUs     = 8    
	maxCPUCores = 256  
	maxMemoryGB = 2048 

	minStorageGB     = 1
	maxStorageGB     = 2048
	defaultStorageGB = 20

	maxGPUIdleTimeout = 24 * time.Hour
)

var validKinds = map[string]struct{}{
	KindJupyter: {},
	KindVSCode:  {},
}

var validMountKinds = map[string]struct{}{
	MountKindDataset: {},
	MountKindModel:   {},
}

type ResourceSpec struct {
	CPU     int    `json:"cpu"`    
	Memory  int    `json:"memory"` 
	GPU     int    `json:"gpu"`    
	GPUType string `json:"gpu_type,omitempty"`
}

type StorageSpec struct {
	SizeGB     int    `json:"size_gb"`
	StorageCls string `json:"storage_class,omitempty"`
}

type Mount struct {
	Name      string `json:"name"`
	MountPath string `json:"mount_path"`
	Kind      string `json:"kind"`
	ReadOnly  bool   `json:"read_only"`
}

type Workspace struct {
	ID       string
	TenantID string
	OwnerID  string
	Name     string
	Kind     string 
	Image    string

	Resources ResourceSpec
	Storage   StorageSpec
	Mounts    []Mount

	Status string
	
	RuntimeName string
	
	URL           string
	FailureReason string

	IdleTimeout  time.Duration
	LastActiveAt *time.Time

	CreatedAt time.Time
	StartedAt *time.Time
	StoppedAt *time.Time
}

func (w *Workspace) Normalize() {
	w.Name = strings.TrimSpace(w.Name)
	w.Kind = strings.TrimSpace(w.Kind)
	w.Image = strings.TrimSpace(w.Image)
	w.Storage.StorageCls = strings.TrimSpace(w.Storage.StorageCls)
	w.Resources.GPUType = strings.TrimSpace(w.Resources.GPUType)

	if w.Kind == "" {
		w.Kind = KindJupyter
	}
	if w.Status == "" {
		w.Status = StatusPending
	}
	if w.Storage.SizeGB == 0 {
		w.Storage.SizeGB = defaultStorageGB
	}

	if w.RequiresGPU() && (w.IdleTimeout <= 0 || w.IdleTimeout > maxGPUIdleTimeout) {
		w.IdleTimeout = maxGPUIdleTimeout
	}

	w.normalizeMounts()
}

func (w *Workspace) normalizeMounts() {
	if len(w.Mounts) == 0 {
		w.Mounts = nil
		return
	}
	kept := make([]Mount, 0, len(w.Mounts))
	for _, m := range w.Mounts {
		m.Name = strings.TrimSpace(m.Name)
		m.MountPath = strings.TrimSpace(m.MountPath)
		m.Kind = strings.TrimSpace(m.Kind)
		if m.Name == "" || m.MountPath == "" {
			continue
		}
		if m.Kind == "" {
			m.Kind = MountKindDataset
		}
		
		m.ReadOnly = true
		kept = append(kept, m)
	}
	if len(kept) == 0 {
		w.Mounts = nil
		return
	}
	w.Mounts = kept
}

func (w *Workspace) Validate() error {
	if strings.TrimSpace(w.Name) == "" {
		return errors.New("name is required")
	}
	if len(w.Name) > maxNameLen {
		return fmt.Errorf("name exceeds %d characters", maxNameLen)
	}
	if strings.TrimSpace(w.TenantID) == "" {
		return errors.New("tenant_id is required")
	}
	if strings.TrimSpace(w.OwnerID) == "" {
		return errors.New("owner_id is required")
	}
	if strings.TrimSpace(w.Image) == "" {
		return errors.New("image is required")
	}
	if _, ok := validKinds[w.Kind]; !ok {
		return fmt.Errorf("unsupported workspace kind %q", w.Kind)
	}
	if err := w.Resources.validate(); err != nil {
		return err
	}
	if err := w.Storage.validate(); err != nil {
		return err
	}
	if w.IdleTimeout < 0 {
		return errors.New("idle_timeout must not be negative")
	}
	return w.validateMounts()
}

func (r ResourceSpec) validate() error {
	if r.CPU < 0 || r.CPU > maxCPUCores {
		return fmt.Errorf("cpu must be within [0, %d]", maxCPUCores)
	}
	if r.Memory < 0 || r.Memory > maxMemoryGB {
		return fmt.Errorf("memory must be within [0, %d] GB", maxMemoryGB)
	}
	if r.GPU < 0 || r.GPU > maxGPUs {
		return fmt.Errorf("gpu must be within [0, %d]", maxGPUs)
	}
	return nil
}

func (s StorageSpec) validate() error {
	if s.SizeGB < minStorageGB || s.SizeGB > maxStorageGB {
		return fmt.Errorf("storage size must be within [%d, %d] GB", minStorageGB, maxStorageGB)
	}
	return nil
}

func (w *Workspace) validateMounts() error {
	seen := make(map[string]struct{}, len(w.Mounts))
	for _, m := range w.Mounts {
		if !strings.HasPrefix(m.MountPath, "/") {
			return fmt.Errorf("mount path %q must be absolute", m.MountPath)
		}
		
		if strings.TrimRight(m.MountPath, "/") == strings.TrimRight(HomePath, "/") {
			return fmt.Errorf("mount path %q shadows the home directory", m.MountPath)
		}
		if _, ok := validMountKinds[m.Kind]; !ok {
			return fmt.Errorf("unsupported mount kind %q", m.Kind)
		}
		
		if _, dup := seen[m.MountPath]; dup {
			return fmt.Errorf("duplicate mount path %q", m.MountPath)
		}
		seen[m.MountPath] = struct{}{}
	}
	return nil
}

func (w *Workspace) RequiresGPU() bool { return w.Resources.GPU > 0 }