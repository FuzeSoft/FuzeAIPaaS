package adapter

import (
	"fuze-ai-paas/backend/internal/domain/inference"
	"fuze-ai-paas/backend/internal/models"
)

func InferenceFromModel(m *models.InferenceService) *inference.InferenceService {
	if m == nil {
		return nil
	}
	
	runtime := inference.RuntimeKind(m.Runtime)
	if runtime == "" {
		runtime = RuntimeKindFromModel(m.Framework)
	}
	return &inference.InferenceService{
		ID:             m.ID,
		ClusterID:      m.ClusterID,
		Name:           m.Name,
		Runtime:        runtime,
		StorageURI:     m.StorageURI,
		Image:          m.Image,
		MinReplicas:    m.MinReplicas,
		MaxReplicas:    m.MaxReplicas,
		CPU:            m.CPU,
		Memory:         m.Memory,
		GPUs:           m.GPUs,
		GPUMemory:      m.GPUMemory,
		GPUCores:       m.GPUCores,
		Chip:           m.Chip,
		Status:         InferenceStatusFromModel(m.Status),
		URL:            m.URL,
		RuntimeName:    m.KServeName,
		ReadyReplicas:  m.ReadyReplicas,
		TargetReplicas: m.TargetReplicas,
		CanaryWeight:   m.CanaryWeight,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

func InferenceSyncToModel(agg *inference.InferenceService, m *models.InferenceService) {
	if agg == nil || m == nil {
		return
	}
	m.Status = InferenceStatusToModel(agg.Status)
	m.URL = agg.URL
	m.KServeName = agg.RuntimeName
	m.ReadyReplicas = agg.ReadyReplicas
	m.TargetReplicas = agg.TargetReplicas
	m.CanaryWeight = agg.CanaryWeight
	m.Chip = agg.Chip
	m.UpdatedAt = agg.UpdatedAt
}

func InferenceToModel(agg *inference.InferenceService) *models.InferenceService {
	if agg == nil {
		return nil
	}
	return &models.InferenceService{
		ID:             agg.ID,
		ClusterID:      agg.ClusterID,
		Name:           agg.Name,
		Framework:      ModelsFrameworkFromRuntime(agg.Runtime),
		Runtime:        string(agg.Runtime),
		StorageURI:     agg.StorageURI,
		Image:          agg.Image,
		MinReplicas:    agg.MinReplicas,
		MaxReplicas:    agg.MaxReplicas,
		CPU:            agg.CPU,
		Memory:         agg.Memory,
		GPUs:           agg.GPUs,
		GPUMemory:      agg.GPUMemory,
		GPUCores:       agg.GPUCores,
		Chip:           agg.Chip,
		Status:         InferenceStatusToModel(agg.Status),
		URL:            agg.URL,
		KServeName:     agg.RuntimeName,
		ReadyReplicas:  agg.ReadyReplicas,
		TargetReplicas: agg.TargetReplicas,
		CanaryWeight:   agg.CanaryWeight,
		CreatedAt:      agg.CreatedAt,
		UpdatedAt:      agg.UpdatedAt,
	}
}

func ModelsFrameworkFromRuntime(r inference.RuntimeKind) models.InferenceFramework {
	switch r {
	case inference.RuntimeTriton:
		return models.FrameworkTriton
	case inference.RuntimeCustom:
		return models.FrameworkCustom
	default:
		return models.FrameworkPyTorch
	}
}

func RuntimeKindFromModel(f models.InferenceFramework) inference.RuntimeKind {
	switch f {
	case models.FrameworkTriton:
		return inference.RuntimeTriton
	case models.FrameworkCustom:
		return inference.RuntimeCustom
	default:
		
		return inference.RuntimeKServe
	}
}

func InferenceStatusFromModel(s models.InferenceStatus) inference.InferenceStatus {
	switch s {
	case models.InferenceStatusReady:
		return inference.InferenceStatusReady
	case models.InferenceStatusFailed:
		return inference.InferenceStatusFailed
	case models.InferenceStatusUnknown:
		return inference.InferenceStatusUnknown
	default:
		return inference.InferenceStatusPending
	}
}

func InferenceStatusToModel(s inference.InferenceStatus) models.InferenceStatus {
	switch s {
	case inference.InferenceStatusReady, inference.InferenceStatusScalingUp,
		inference.InferenceStatusScaling, inference.InferenceStatusDegraded,
		inference.InferenceStatusOffline:
		return models.InferenceStatusReady
	case inference.InferenceStatusFailed:
		return models.InferenceStatusFailed
	case inference.InferenceStatusUnknown:
		return models.InferenceStatusUnknown
	default:
		return models.InferenceStatusPending
	}
}