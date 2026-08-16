package hpo

import "time"

const (
	StudyPending   = "pending"
	StudyRunning   = "running"
	StudyCompleted = "completed"
	StudyFailed    = "failed"
	StudyCancelled = "cancelled"
	StudyStopped   = "stopped"
)

type EarlyStopSpec struct {
	
	Enabled bool
	
	Eta float64
	
	MinRungs int
}

func (e *EarlyStopSpec) normalized() EarlyStopSpec {
	if e == nil || !e.Enabled {
		return EarlyStopSpec{}
	}
	out := *e
	if out.Eta <= 1 {
		out.Eta = 3 
	}
	if out.MinRungs <= 0 {
		out.MinRungs = 1
	}
	return out
}

type Study struct {
	ID, TenantID, ExperimentID string
	Name        string
	Space       SearchSpace
	Algorithm   string 
	Objective   Objective
	MaxTrials   int
	MaxParallel int
	MaxDuration time.Duration
	EarlyStop   *EarlyStopSpec
	TrainingTemplate map[string]any 
	Status      string
	BestTrialID string

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *Study) IsTerminal() bool {
	switch s.Status {
	case StudyCompleted, StudyFailed, StudyCancelled, StudyStopped:
		return true
	default:
		return false
	}
}

func (s *Study) IsActive() bool {
	return s.Status == StudyRunning || s.Status == StudyPending
}

func (s *Study) rungFor(step int) int {
	es := s.EarlyStop.normalized()
	if es.Eta <= 1 {
		return 0
	}
	rung := 0
	bound := int(es.Eta)
	for step > bound {
		rung++
		bound = int(float64(bound) * es.Eta)
	}
	return rung
}

func runningCount(trials []Trial) int {
	n := 0
	for i := range trials {
		if trials[i].Status == TrialRunning {
			n++
		}
	}
	return n
}

func completedCount(trials []Trial) int {
	n := 0
	for i := range trials {
		if trials[i].Status == TrialCompleted {
			n++
		}
	}
	return n
}

func failedCount(trials []Trial) int {
	n := 0
	for i := range trials {
		if trials[i].Status == TrialFailed || trials[i].Status == TrialPruned {
			n++
		}
	}
	return n
}