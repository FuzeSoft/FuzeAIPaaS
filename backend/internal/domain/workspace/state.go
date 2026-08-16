package workspace

import "time"

const (
	StatusPending  = "pending"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusStopping = "stopping"
	StatusStopped  = "stopped"
	StatusFailed   = "failed"
	StatusDeleted  = "deleted"
)

var transitions = map[string]map[string]struct{}{
	StatusPending: {
		StatusStarting: {},
		StatusFailed:   {},
		StatusDeleted:  {},
	},
	StatusStarting: {
		StatusRunning: {},
		StatusFailed:  {},
		
		StatusStopping: {},
	},
	StatusRunning: {
		StatusStopping: {},
		StatusFailed:   {},
	},
	StatusStopping: {
		StatusStopped: {},
		StatusFailed:  {},
	},
	StatusStopped: {
		StatusStarting: {},
		StatusDeleted:  {},
	},
	StatusFailed: {
		StatusStarting: {},
		StatusDeleted:  {},
	},
	StatusDeleted: {},
}

var terminalStatuses = map[string]struct{}{
	StatusStopped: {},
	StatusFailed:  {},
	StatusDeleted: {},
}

var activeStatuses = map[string]struct{}{
	StatusStarting: {},
	StatusRunning:  {},
	StatusStopping: {},
}

func CanTransition(from, to string) bool {
	next, ok := transitions[from]
	if !ok {
		return false
	}
	_, ok = next[to]
	return ok
}

func (w *Workspace) transition(to string) bool {
	if !CanTransition(w.Status, to) {
		return false
	}
	w.Status = to
	return true
}

func (w *Workspace) IsTerminal() bool {
	_, ok := terminalStatuses[w.Status]
	return ok
}

func (w *Workspace) IsActive() bool {
	_, ok := activeStatuses[w.Status]
	return ok
}

func (w *Workspace) CanStart() bool { return CanTransition(w.Status, StatusStarting) }

func (w *Workspace) CanStop() bool { return CanTransition(w.Status, StatusStopping) }

func (w *Workspace) CanDelete() bool { return CanTransition(w.Status, StatusDeleted) }

func (w *Workspace) MarkStarting() bool {
	if !w.transition(StatusStarting) {
		return false
	}
	w.StoppedAt = nil
	w.FailureReason = ""
	w.URL = ""
	return true
}

func (w *Workspace) MarkRunning(at time.Time) bool {
	if !w.transition(StatusRunning) {
		return false
	}
	t := at
	w.StartedAt = &t
	
	active := at
	w.LastActiveAt = &active
	return true
}

func (w *Workspace) MarkStopping() bool { return w.transition(StatusStopping) }

func (w *Workspace) MarkStopped(at time.Time) bool {
	if !w.transition(StatusStopped) {
		return false
	}
	w.stampStopped(at)
	return true
}

func (w *Workspace) MarkFailed(reason string, at time.Time) bool {
	if !w.transition(StatusFailed) {
		return false
	}
	w.FailureReason = reason
	w.stampStopped(at)
	return true
}

func (w *Workspace) MarkDeleted() bool { return w.transition(StatusDeleted) }

func (w *Workspace) stampStopped(at time.Time) {
	t := at
	w.StoppedAt = &t
	
	w.URL = ""
}