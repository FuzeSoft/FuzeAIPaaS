
package inference

import "time"

type RuntimeKind string

const (
	RuntimeVLLM   RuntimeKind = "vllm"   
	RuntimeTriton RuntimeKind = "triton" 
	RuntimeKServe RuntimeKind = "kserve" 
	RuntimeAscend RuntimeKind = "ascend" 
	RuntimeCustom RuntimeKind = "custom" 
)

type InferenceStatus string

const (
	InferenceStatusPending   InferenceStatus = "pending"    
	InferenceStatusScalingUp InferenceStatus = "scaling_up" 
	InferenceStatusReady     InferenceStatus = "ready"      
	InferenceStatusScaling   InferenceStatus = "scaling"    
	InferenceStatusDegraded  InferenceStatus = "degraded"   
	InferenceStatusOffline   InferenceStatus = "offline"    
	InferenceStatusFailed    InferenceStatus = "failed"     
	InferenceStatusUnknown   InferenceStatus = "unknown"    
)

type InferenceService struct {
	ID         string
	ClusterID  string
	Name       string
	Runtime    RuntimeKind
	StorageURI string 
	Image      string 

	MinReplicas    int
	MaxReplicas    int
	TargetReplicas int 

	CPU       string
	Memory    string
	GPUs      int
	GPUMemory int    
	GPUCores  int    
	Chip      string 

	Status        InferenceStatus
	URL           string
	RuntimeName   string 
	ReadyReplicas int
	CanaryWeight  int 

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *InferenceService) ApplyRuntimeStatus(ready, found, failed bool, replicas int, url string) {
	s.ReadyReplicas = replicas
	if url != "" {
		s.URL = url
	}
	switch {
	case !found:
		s.Status = InferenceStatusPending
	case failed:
		s.Status = InferenceStatusFailed
	case ready:
		if replicas == 0 {
			s.Status = InferenceStatusOffline
		} else {
			s.Status = InferenceStatusReady
		}
	default:
		
		if s.Status == InferenceStatusReady {
			s.Status = InferenceStatusDegraded
		} else {
			s.Status = InferenceStatusScalingUp
		}
	}
}

func (s *InferenceService) ScaleTo(n int) {
	if n < s.MinReplicas {
		n = s.MinReplicas
	}
	if n > s.MaxReplicas {
		n = s.MaxReplicas
	}
	s.TargetReplicas = n
	if n == s.ReadyReplicas {
		return
	}
	if n > s.ReadyReplicas {
		s.Status = InferenceStatusScalingUp
	} else {
		s.Status = InferenceStatusScaling
	}
}

func (s *InferenceService) CanScaleUp() bool {
	return s.ReadyReplicas < s.MaxReplicas
}

func (s *InferenceService) MarkFailed() { s.Status = InferenceStatusFailed }
func (s *InferenceService) MarkOffline() {
	s.ReadyReplicas = 0
	s.Status = InferenceStatusOffline
}

func (s *InferenceService) NeedsDeploy() bool { return s.RuntimeName == "" }

func (s *InferenceService) ShouldUseRuntime(clusterManaged, runtimeAvailable bool) bool {
	if !clusterManaged {
		return false 
	}
	return runtimeAvailable 
}

func (s *InferenceService) NeedsScalePush() bool { return s.TargetReplicas != s.ReadyReplicas }

func (s *InferenceService) NeedsCanaryPush() bool { return true }

func (s *InferenceService) MockConvergeObservation() bool {
	if s.ReadyReplicas == s.TargetReplicas && s.Status == InferenceStatusReady {
		return false
	}
	s.ReadyReplicas = s.TargetReplicas
	s.Status = InferenceStatusReady
	return true
}