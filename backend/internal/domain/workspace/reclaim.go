package workspace

import "time"

func (w *Workspace) ShouldReclaim(now time.Time) bool {
	if w.Status != StatusRunning || w.IdleTimeout <= 0 {
		return false
	}
	ref := w.idleSince()
	if ref == nil {
		
		return false
	}
	return now.Sub(*ref) > w.IdleTimeout
}

func (w *Workspace) IdleDuration(now time.Time) time.Duration {
	ref := w.idleSince()
	if ref == nil {
		return 0
	}
	d := now.Sub(*ref)
	if d < 0 {
		return 0
	}
	return d
}

func (w *Workspace) idleSince() *time.Time {
	if w.LastActiveAt != nil {
		return w.LastActiveAt
	}
	return w.StartedAt
}

func (w *Workspace) Touch(at time.Time) bool {
	
	if w.IsTerminal() {
		return false
	}
	if w.LastActiveAt != nil && !at.After(*w.LastActiveAt) {
		return false
	}
	t := at
	w.LastActiveAt = &t
	return true
}