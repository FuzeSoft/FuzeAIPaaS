
package training

import (
	"errors"
	"fmt"
	"strings"
)

const (
	maxGPUs           = 1024 
	maxMemoryGB       = 8192 
	maxReplicas       = 512  
	maxCommandLen     = 8192
	maxGPUCores       = 100           
	maxRuntimeMinutes = 30 * 24 * 60  
	DefaultClusterID  = "cluster-001" 
)

const (
	FrameworkPyTorchDDP = "pytorch-ddp"
	FrameworkDeepSpeed  = "deepspeed"
	FrameworkTensorFlow = "tensorflow"
	FrameworkMPI        = "mpi"
)

var distributedFrameworks = map[string]struct{}{
	FrameworkPyTorchDDP: {},
	FrameworkDeepSpeed:  {},
	FrameworkTensorFlow: {},
	FrameworkMPI:        {},
}

type Spec struct {
	Image    string `json:"image"`
	Command  string `json:"command"`
	Priority int    `json:"priority"`

	GPUs      int `json:"gpus"`
	Memory    int `json:"memory"`     
	GPUMemory int `json:"gpu_memory"` 
	GPUCores  int `json:"gpu_cores"`  
	
	GPUType string `json:"gpu_type,omitempty"`

	Distributed  bool   `json:"distributed"`
	Framework    string `json:"framework"`
	Replicas     int    `json:"replicas"`      
	MinAvailable int    `json:"min_available"` 

	DatasetName string `json:"dataset_name,omitempty"`
	MountPath   string `json:"mount_path,omitempty"`
	
	CodeCommit string `json:"code_commit,omitempty"`
	TemplateID string `json:"template_id,omitempty"`

	MaxRuntime int `json:"max_runtime"`
}

func (s *Spec) Normalize() {
	s.Image = strings.TrimSpace(s.Image)
	s.Framework = strings.TrimSpace(s.Framework)
	s.DatasetName = strings.TrimSpace(s.DatasetName)
	s.MountPath = strings.TrimSpace(s.MountPath)
	s.CodeCommit = strings.TrimSpace(s.CodeCommit)
	s.TemplateID = strings.TrimSpace(s.TemplateID)

	if s.Distributed && s.Replicas <= 0 {
		s.Distributed = false
	}
	if !s.Distributed {
		s.Replicas = 0
		s.MinAvailable = 0
		return
	}
	if s.Framework == "" {
		s.Framework = FrameworkPyTorchDDP
	}
}

func (s *Spec) Validate() error {
	if strings.TrimSpace(s.Image) == "" {
		return errors.New("image is required")
	}
	if len(s.Command) > maxCommandLen {
		return fmt.Errorf("command exceeds %d characters", maxCommandLen)
	}
	if s.GPUs < 0 || s.GPUs > maxGPUs {
		return fmt.Errorf("gpus must be within [0, %d]", maxGPUs)
	}
	if s.Memory < 0 || s.Memory > maxMemoryGB {
		return fmt.Errorf("memory must be within [0, %d] GB", maxMemoryGB)
	}
	if s.GPUMemory < 0 {
		return errors.New("gpu_memory must not be negative")
	}
	if s.GPUCores < 0 || s.GPUCores > maxGPUCores {
		return fmt.Errorf("gpu_cores must be within [0, %d]", maxGPUCores)
	}
	if s.MaxRuntime < 0 || s.MaxRuntime > maxRuntimeMinutes {
		return fmt.Errorf("max_runtime must be within [0, %d] minutes", maxRuntimeMinutes)
	}

	if !s.Distributed {
		return nil
	}
	if s.Replicas <= 0 || s.Replicas > maxReplicas {
		return fmt.Errorf("replicas must be within [1, %d] for distributed jobs", maxReplicas)
	}
	if _, ok := distributedFrameworks[s.Framework]; !ok {
		return fmt.Errorf("unsupported distributed framework %q", s.Framework)
	}
	if s.MinAvailable < 0 {
		return errors.New("min_available must not be negative")
	}
	
	if total := s.EffectiveReplicas(); s.MinAvailable > total {
		return fmt.Errorf("min_available (%d) must not exceed total replicas (%d)", s.MinAvailable, total)
	}
	return nil
}

func (s *Spec) EffectiveReplicas() int {
	if !s.Distributed || s.Replicas <= 0 {
		return 1
	}
	return 1 + s.Replicas
}

func (s *Spec) TotalGPUs() int {
	if s.GPUs <= 0 {
		return 0
	}
	return s.GPUs * s.EffectiveReplicas()
}

func (s *Spec) TotalMemory() int {
	if s.Memory <= 0 {
		return 0
	}
	return s.Memory * s.EffectiveReplicas()
}