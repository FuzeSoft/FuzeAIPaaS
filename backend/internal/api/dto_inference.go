package api

import (
	"errors"
	"time"

	"fuze-ai-paas/backend/internal/models"
)

type inferenceSpec struct {
	Name           string                    `json:"name"`
	ClusterID      string                    `json:"cluster_id"`
	Framework      models.InferenceFramework `json:"framework"`
	Runtime        string                    `json:"runtime,omitempty"`
	StorageURI     string                    `json:"storage_uri"`
	Image          string                    `json:"image"`
	RuntimeVersion string                    `json:"runtime_version,omitempty"`
	MinReplicas    int                       `json:"min_replicas"`
	MaxReplicas    int                       `json:"max_replicas"`
	CPU            string                    `json:"cpu"`
	Memory         string                    `json:"memory"`
	GPUs           int                       `json:"gpus"`
	GPUMemory      int                       `json:"gpu_memory"`
	GPUCores       int                       `json:"gpu_cores"`
	Chip           string                    `json:"chip,omitempty"`
	TargetReplicas int                       `json:"target_replicas"`
	CanaryWeight   int                       `json:"canary_weight"`
}

type inferenceStatusView struct {
	Phase         models.InferenceStatus `json:"phase"`
	URL           string                 `json:"url"`
	RuntimeName   string                 `json:"runtime_name"`
	ReadyReplicas int                    `json:"ready_replicas"`
	ObservedAt    time.Time              `json:"observed_at"`
}

type inferenceServiceView struct {
	ID        string              `json:"id"`
	TenantID  string              `json:"tenant_id,omitempty"`
	Spec      inferenceSpec       `json:"spec"`
	Status    inferenceStatusView `json:"status"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

type inferenceApplyRequest struct {
	Spec inferenceSpec `json:"spec"`
}

type inferencePatchSpec struct {
	TargetReplicas *int    `json:"target_replicas"`
	CanaryWeight   *int    `json:"canary_weight"`
	MinReplicas    *int    `json:"min_replicas"`
	MaxReplicas    *int    `json:"max_replicas"`
	StorageURI     *string `json:"storage_uri"`
	Image          *string `json:"image"`
	RuntimeVersion *string `json:"runtime_version"`

	Name      *string `json:"name"`
	ClusterID *string `json:"cluster_id"`
	Framework *string `json:"framework"`
	Runtime   *string `json:"runtime"`
	CPU       *string `json:"cpu"`
	Memory    *string `json:"memory"`
	GPUs      *int    `json:"gpus"`
	GPUMemory *int    `json:"gpu_memory"`
	GPUCores  *int    `json:"gpu_cores"`
	Chip      *string `json:"chip"`
}

type inferencePatchRequest struct {
	Spec inferencePatchSpec `json:"spec"`
}

func toInferenceView(s *models.InferenceService) inferenceServiceView {
	return inferenceServiceView{
		ID:       s.ID,
		TenantID: s.TenantID,
		Spec: inferenceSpec{
			Name:           s.Name,
			ClusterID:      s.ClusterID,
			Framework:      s.Framework,
			Runtime:        s.Runtime,
			StorageURI:     s.StorageURI,
			Image:          s.Image,
			RuntimeVersion: s.RuntimeVer,
			MinReplicas:    s.MinReplicas,
			MaxReplicas:    s.MaxReplicas,
			CPU:            s.CPU,
			Memory:         s.Memory,
			GPUs:           s.GPUs,
			GPUMemory:      s.GPUMemory,
			GPUCores:       s.GPUCores,
			Chip:           s.Chip,
			TargetReplicas: s.TargetReplicas,
			CanaryWeight:   s.CanaryWeight,
		},
		Status: inferenceStatusView{
			Phase:         s.Status,
			URL:           s.URL,
			RuntimeName:   s.KServeName,
			ReadyReplicas: s.ReadyReplicas,
			ObservedAt:    s.UpdatedAt,
		},
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

func toInferenceViews(list []models.InferenceService) []inferenceServiceView {
	views := make([]inferenceServiceView, 0, len(list))
	for i := range list {
		views = append(views, toInferenceView(&list[i]))
	}
	return views
}

func (spec *inferenceSpec) normalize() {
	if spec.ClusterID == "" {
		spec.ClusterID = "cluster-001"
	}
	if spec.Framework == "" {
		spec.Framework = models.FrameworkCustom
	}
	if spec.MaxReplicas == 0 {
		spec.MaxReplicas = max(1, spec.MinReplicas)
	}
	if spec.TargetReplicas == 0 {
		spec.TargetReplicas = spec.MinReplicas
	}
}

func (spec *inferenceSpec) validate() error {
	if spec.Name == "" {
		return errors.New("spec.name is required")
	}
	if spec.MinReplicas < 0 {
		return errors.New("spec.min_replicas must be >= 0")
	}
	if spec.MaxReplicas < spec.MinReplicas {
		return errors.New("spec.max_replicas must be >= spec.min_replicas")
	}
	if spec.TargetReplicas < 0 {
		return errors.New("spec.target_replicas must be >= 0")
	}
	if spec.CanaryWeight < 0 || spec.CanaryWeight > 100 {
		return errors.New("spec.canary_weight must be between 0 and 100")
	}
	if spec.GPUs < 0 {
		return errors.New("spec.gpus must be >= 0")
	}
	return nil
}

func (spec *inferenceSpec) applyTo(s *models.InferenceService) {
	s.Name = spec.Name
	s.ClusterID = spec.ClusterID
	s.Framework = spec.Framework
	s.Runtime = spec.Runtime
	s.StorageURI = spec.StorageURI
	s.Image = spec.Image
	s.RuntimeVer = spec.RuntimeVersion
	s.MinReplicas = spec.MinReplicas
	s.MaxReplicas = spec.MaxReplicas
	s.CPU = spec.CPU
	s.Memory = spec.Memory
	s.GPUs = spec.GPUs
	s.GPUMemory = spec.GPUMemory
	s.GPUCores = spec.GPUCores
	s.Chip = spec.Chip
	s.TargetReplicas = spec.TargetReplicas
	s.CanaryWeight = spec.CanaryWeight
}

func (p *inferencePatchSpec) rejectImmutable() error {
	immutable := []struct {
		field   string
		present bool
	}{
		{"spec.name", p.Name != nil},
		{"spec.cluster_id", p.ClusterID != nil},
		{"spec.framework", p.Framework != nil},
		{"spec.runtime", p.Runtime != nil},
		{"spec.cpu", p.CPU != nil},
		{"spec.memory", p.Memory != nil},
		{"spec.gpus", p.GPUs != nil},
		{"spec.gpu_memory", p.GPUMemory != nil},
		{"spec.gpu_cores", p.GPUCores != nil},
		{"spec.chip", p.Chip != nil},
	}
	for _, f := range immutable {
		if f.present {
			return errors.New(f.field + " cannot be changed via PATCH, use PUT /inference-services to apply a full spec")
		}
	}
	return nil
}

func (p *inferencePatchSpec) validate() error {
	if p.TargetReplicas != nil && *p.TargetReplicas < 0 {
		return errors.New("spec.target_replicas must be >= 0")
	}
	if p.CanaryWeight != nil && (*p.CanaryWeight < 0 || *p.CanaryWeight > 100) {
		return errors.New("spec.canary_weight must be between 0 and 100")
	}
	if p.MinReplicas != nil && *p.MinReplicas < 0 {
		return errors.New("spec.min_replicas must be >= 0")
	}
	if p.MaxReplicas != nil && *p.MaxReplicas < 0 {
		return errors.New("spec.max_replicas must be >= 0")
	}
	return nil
}

func (p *inferencePatchSpec) applyTo(s *models.InferenceService) {
	if p.TargetReplicas != nil {
		s.TargetReplicas = *p.TargetReplicas
	}
	if p.CanaryWeight != nil {
		s.CanaryWeight = *p.CanaryWeight
	}
	if p.MinReplicas != nil {
		s.MinReplicas = *p.MinReplicas
	}
	if p.MaxReplicas != nil {
		s.MaxReplicas = *p.MaxReplicas
	}
	if p.StorageURI != nil {
		s.StorageURI = *p.StorageURI
	}
	if p.Image != nil {
		s.Image = *p.Image
	}
	if p.RuntimeVersion != nil {
		s.RuntimeVer = *p.RuntimeVersion
	}
}

func deploymentChanged(a, b *models.InferenceService) bool {
	return a.StorageURI != b.StorageURI ||
		a.Image != b.Image ||
		a.Runtime != b.Runtime ||
		a.Framework != b.Framework ||
		a.CPU != b.CPU ||
		a.Memory != b.Memory ||
		a.GPUs != b.GPUs ||
		a.GPUMemory != b.GPUMemory ||
		a.GPUCores != b.GPUCores ||
		a.Chip != b.Chip ||
		a.ClusterID != b.ClusterID ||
		a.RuntimeVer != b.RuntimeVer
}