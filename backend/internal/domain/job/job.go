package job

type Job struct {
	ID        string
	ClusterID string
	Name      string
	Type      JobType
	Status    JobStatus
	Memory    int
}

func (j *Job) CanSchedule(resources []Resource) bool {
	return CanSchedule(j, resources)
}

func (j *Job) Allocate(resources []Resource) {
	Allocate(j, resources)
}

func (j *Job) IsTerminal() bool { return IsTerminal(j.Status) }

func (j *Job) ApplyVolcanoState(state JobState) bool {
	next, ok := MapVolcanoState(state)
	if !ok {
		return false
	}
	return j.transition(next)
}

func (j *Job) MarkRunning() bool   { return j.transition(JobStatusRunning) }
func (j *Job) MarkPending() bool   { return j.transition(JobStatusPending) }
func (j *Job) MarkCancelled() bool { return j.transition(JobStatusCancelled) }
func (j *Job) MarkCompleted() bool { return j.transition(JobStatusCompleted) }
func (j *Job) MarkFailed() bool    { return j.transition(JobStatusFailed) }

func (j *Job) transition(next JobStatus) bool {
	current := NormalizeStatus(j.Status)
	j.Status = current 
	if current == next || !CanTransition(current, next) {
		return false
	}
	j.Status = next
	return true
}