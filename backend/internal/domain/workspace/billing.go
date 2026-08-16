package workspace

import "time"

func (w *Workspace) IsBillable() bool { return w.Status == StatusRunning }

func (w *Workspace) BillableDuration(now time.Time) time.Duration {
	if w.StartedAt == nil {
		return 0
	}

	end := now
	if w.IsTerminal() {
		if w.StoppedAt == nil {
			
			return 0
		}
		end = *w.StoppedAt
	}

	d := end.Sub(*w.StartedAt)
	if d < 0 {
		
		return 0
	}
	return d
}

func (w *Workspace) BillableHours(now time.Time) float64 {
	return w.BillableDuration(now).Hours()
}