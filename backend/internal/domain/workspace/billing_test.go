package workspace

import (
	"testing"
	"time"
)

func TestBillableDurationZeroBeforeStart(t *testing.T) {
	w := Workspace{Status: StatusPending}
	if got := w.BillableDuration(at); got != 0 {
		t.Fatalf("never-started workspace must not be billable, got %v", got)
	}
}

func TestBillableDurationRunningUsesNow(t *testing.T) {
	started := at
	w := Workspace{Status: StatusRunning, StartedAt: &started}
	if got := w.BillableDuration(at.Add(90 * time.Minute)); got != 90*time.Minute {
		t.Fatalf("BillableDuration = %v, want 90m", got)
	}
}

func TestBillableDurationStoppedUsesStoppedAt(t *testing.T) {
	started := at
	stopped := at.Add(2 * time.Hour)
	w := Workspace{Status: StatusStopped, StartedAt: &started, StoppedAt: &stopped}

	if got := w.BillableDuration(at.Add(100 * time.Hour)); got != 2*time.Hour {
		t.Fatalf("BillableDuration = %v, want 2h", got)
	}
}

func TestBillableDurationFailedUsesStoppedAt(t *testing.T) {
	started := at
	stopped := at.Add(10 * time.Minute)
	w := Workspace{Status: StatusFailed, StartedAt: &started, StoppedAt: &stopped}
	if got := w.BillableDuration(at.Add(50 * time.Hour)); got != 10*time.Minute {
		t.Fatalf("BillableDuration = %v, want 10m", got)
	}
}

func TestBillableDurationNeverNegative(t *testing.T) {
	started := at
	w := Workspace{Status: StatusRunning, StartedAt: &started}
	if got := w.BillableDuration(at.Add(-time.Hour)); got != 0 {
		t.Fatalf("clock skew must clamp to 0, got %v", got)
	}

	stopped := at.Add(-time.Hour)
	w = Workspace{Status: StatusStopped, StartedAt: &started, StoppedAt: &stopped}
	if got := w.BillableDuration(at); got != 0 {
		t.Fatalf("StoppedAt before StartedAt must clamp to 0, got %v", got)
	}
}

func TestBillableDurationTerminalWithoutStoppedAt(t *testing.T) {
	started := at
	w := Workspace{Status: StatusStopped, StartedAt: &started}
	if got := w.BillableDuration(at.Add(100 * time.Hour)); got != 0 {
		t.Fatalf("terminal workspace without StoppedAt must not accrue, got %v", got)
	}
}

func TestBillableHours(t *testing.T) {
	started := at
	w := Workspace{Status: StatusRunning, StartedAt: &started}
	if got := w.BillableHours(at.Add(90 * time.Minute)); got != 1.5 {
		t.Fatalf("BillableHours = %v, want 1.5", got)
	}
}

func TestIsBillable(t *testing.T) {
	cases := map[string]bool{
		StatusPending:  false,
		StatusStarting: false,
		StatusRunning:  true,
		StatusStopping: false,
		StatusStopped:  false,
		StatusFailed:   false,
		StatusDeleted:  false,
	}
	for status, want := range cases {
		w := Workspace{Status: status}
		if got := w.IsBillable(); got != want {
			t.Fatalf("IsBillable(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestBillableDurationAcrossLifecycle(t *testing.T) {
	w := Workspace{Status: StatusPending}
	w.MarkStarting()
	w.MarkRunning(at)

	if got := w.BillableDuration(at.Add(time.Hour)); got != time.Hour {
		t.Fatalf("running duration = %v, want 1h", got)
	}

	w.MarkStopping()
	w.MarkStopped(at.Add(2 * time.Hour))

	if got := w.BillableDuration(at.Add(99 * time.Hour)); got != 2*time.Hour {
		t.Fatalf("settled duration = %v, want 2h", got)
	}

	w.MarkStarting()
	relaunch := at.Add(10 * time.Hour)
	w.MarkRunning(relaunch)
	if got := w.BillableDuration(relaunch.Add(30 * time.Minute)); got != 30*time.Minute {
		t.Fatalf("second run duration = %v, want 30m", got)
	}
}