package job

var terminalStatuses = map[JobStatus]struct{}{
	JobStatusCompleted: {},
	JobStatusFailed:    {},
	JobStatusCancelled: {},
}

var allowedTransitions = map[JobStatus]map[JobStatus]struct{}{
	JobStatusPending: {
		JobStatusRunning:   {},
		JobStatusPaused:    {},
		JobStatusCompleted: {}, 
		JobStatusFailed:    {},
		JobStatusCancelled: {},
	},
	JobStatusRunning: {
		JobStatusPaused:    {},
		JobStatusRetrying:  {},
		JobStatusCompleted: {},
		JobStatusFailed:    {},
		JobStatusCancelled: {},
	},
	
	JobStatusRetrying: {
		JobStatusPending:   {},
		JobStatusRunning:   {},
		JobStatusFailed:    {},
		JobStatusCancelled: {},
	},
	
	JobStatusPaused: {
		JobStatusRunning:   {},
		JobStatusPending:   {},
		JobStatusCompleted: {},
		JobStatusFailed:    {},
		JobStatusCancelled: {},
	},
}

func NormalizeStatus(s JobStatus) JobStatus {
	if s == "" {
		return JobStatusPending
	}
	return s
}

func IsTerminal(s JobStatus) bool {
	_, ok := terminalStatuses[NormalizeStatus(s)]
	return ok
}

func IsActive(s JobStatus) bool {
	return !IsTerminal(s)
}

func CanTransition(from, to JobStatus) bool {
	from, to = NormalizeStatus(from), NormalizeStatus(to)
	if from == to {
		return true
	}
	next, ok := allowedTransitions[from]
	if !ok {
		return false 
	}
	_, ok = next[to]
	return ok
}

func MapVolcanoState(state JobState) (JobStatus, bool) {
	switch state {
	case JobStatePending:
		return JobStatusPending, true
	case JobStateRunning:
		return JobStatusRunning, true
	case JobStateCompleted:
		return JobStatusCompleted, true
	case JobStateFailed, JobStateAborted, JobStateTerminated:
		return JobStatusFailed, true
	default:
		
		return "", false
	}
}

func VolcanoStateToJobStatus(state JobState) JobStatus {
	if status, ok := MapVolcanoState(state); ok {
		return status
	}
	return JobStatusPending
}