package inference

import "errors"

var ErrCanaryUnsupported = errors.New("canary rollout unsupported by runtime")

func (s *InferenceService) PromoteCanary(weight int) {
	if weight < 0 {
		weight = 0
	}
	if weight > 100 {
		weight = 100
	}
	s.CanaryWeight = weight
}

func (s *InferenceService) IsCanaryActive() bool {
	return s.CanaryWeight > 0 && s.CanaryWeight < 100
}