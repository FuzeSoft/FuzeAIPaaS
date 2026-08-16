package inference

type ScalingMetrics struct {
	QueueDepth    int     
	GPUUtil       float64 
	ReadyReplicas int     
	
	BaseReplicas int
}

func (s *InferenceService) DesiredReplicas(m ScalingMetrics) int {
	base := m.BaseReplicas
	if base == 0 {
		
		base = m.ReadyReplicas
	}
	desired := base
	if m.QueueDepth > 0 {
		desired += (m.QueueDepth + 9) / 10
	}
	if m.GPUUtil > 80 {
		desired++
	}
	if desired < s.MinReplicas {
		desired = s.MinReplicas
	}
	if desired > s.MaxReplicas {
		desired = s.MaxReplicas
	}
	return desired
}