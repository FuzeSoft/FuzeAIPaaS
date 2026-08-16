package data

import (
	"encoding/json"
	"fmt"
)

func CanTransitionPipeline(from, to PipelineStatus) bool {
	if from == to {
		return true
	}
	if from.IsTerminal() {
		return false
	}
	switch to {
	case PipelineStatusPending,
		PipelineStatusRunning,
		PipelineStatusCancelled:
		return true
	case PipelineStatusCompleted, PipelineStatusFailed:
		
		return from == PipelineStatusRunning
	default:
		return false
	}
}

func StepStatusFromJobStatus(js JobStatus) StepStatus {
	switch js {
	case JobStatusPending, JobStatusPaused:
		return StepStatusPending
	case JobStatusRunning, JobStatusRetrying:
		return StepStatusRunning
	case JobStatusCompleted:
		return StepStatusSucceeded
	case JobStatusFailed:
		return StepStatusFailed
	case JobStatusCancelled:
		return StepStatusSkipped
	default:
		return StepStatusPending
	}
}

func ValidateDAG(steps []PipelineStep) error {
	ids := make(map[string]bool, len(steps))
	for _, s := range steps {
		ids[s.ID] = true
	}
	deps := make(map[string][]string, len(steps))
	for _, s := range steps {
		var ds []string
		if s.DependsOn != "" {
			if err := json.Unmarshal([]byte(s.DependsOn), &ds); err != nil {
				return fmt.Errorf("step %s: invalid depends_on json: %w", s.ID, err)
			}
		}
		for _, d := range ds {
			if !ids[d] {
				return fmt.Errorf("step %s depends on missing step %q", s.ID, d)
			}
		}
		deps[s.ID] = ds
	}
	
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(steps))
	var visit func(id string, stack []string) error
	visit = func(id string, stack []string) error {
		color[id] = gray
		for _, d := range deps[id] {
			switch color[d] {
			case gray:
				return fmt.Errorf("cycle detected: %v -> %s", append(stack, id), d)
			case white:
				if err := visit(d, append(stack, id)); err != nil {
					return err
				}
			}
		}
		color[id] = black
		return nil
	}
	for _, s := range steps {
		if color[s.ID] == white {
			if err := visit(s.ID, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func ReadySteps(steps []PipelineStep) []PipelineStep {
	state := make(map[string]StepStatus, len(steps))
	for _, s := range steps {
		state[s.ID] = s.Status
	}
	var ready []PipelineStep
	for _, s := range steps {
		if s.Status != StepStatusPending {
			continue
		}
		var ds []string
		if s.DependsOn != "" {
			_ = json.Unmarshal([]byte(s.DependsOn), &ds)
		}
		ok := true
		for _, d := range ds {
			if state[d] != StepStatusSucceeded {
				ok = false
				break
			}
		}
		if ok {
			ready = append(ready, s)
		}
	}
	return ready
}

func DerivePipelineStatus(steps []PipelineStep, current PipelineStatus) PipelineStatus {
	if current.IsTerminal() {
		return current
	}
	anyFailed := false
	allTerminal := true
	for _, s := range steps {
		if s.Status == StepStatusFailed {
			anyFailed = true
		}
		if !s.Status.IsTerminal() {
			allTerminal = false
		}
	}
	if anyFailed {
		return PipelineStatusFailed
	}
	if allTerminal {
		return PipelineStatusCompleted
	}
	if current == PipelineStatusPending {
		return PipelineStatusRunning
	}
	return current
}