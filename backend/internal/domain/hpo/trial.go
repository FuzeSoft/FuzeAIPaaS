package hpo

const (
	TrialPending   = "pending"
	TrialRunning   = "running"
	TrialCompleted = "completed"
	TrialFailed    = "failed"
	TrialPruned    = "pruned"
	TrialCancelled = "cancelled"
)

type Report struct {
	Step  int
	Value float64
}

type Trial struct {
	ID           string
	StudyID      string
	Number       int
	Params       map[string]any
	Status       string
	Value        *float64
	Intermediate []Report
	RunID        string
	JobID        string
}

func (t *Trial) IsTerminal() bool {
	switch t.Status {
	case TrialCompleted, TrialFailed, TrialPruned:
		return true
	default:
		return false
	}
}

func (t *Trial) LatestValue() (float64, bool) {
	if len(t.Intermediate) > 0 {
		return t.Intermediate[len(t.Intermediate)-1].Value, true
	}
	if t.Value != nil {
		return *t.Value, true
	}
	return 0, false
}