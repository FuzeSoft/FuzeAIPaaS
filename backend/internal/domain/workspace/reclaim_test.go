package workspace

import (
	"testing"
	"time"
)

func runningWorkspace(lastActive time.Time, idle time.Duration) Workspace {
	la := lastActive
	return Workspace{
		Status:       StatusRunning,
		IdleTimeout:  idle,
		LastActiveAt: &la,
	}
}

func TestShouldReclaimWhenIdleExceeded(t *testing.T) {
	w := runningWorkspace(at, time.Hour)
	if !w.ShouldReclaim(at.Add(time.Hour + time.Second)) {
		t.Fatal("workspace idle beyond timeout must be reclaimable")
	}
}

func TestShouldReclaimBoundaryIsExclusive(t *testing.T) {
	w := runningWorkspace(at, time.Hour)
	if w.ShouldReclaim(at.Add(time.Hour)) {
		t.Fatal("exact boundary must not be reclaimed")
	}
	if w.ShouldReclaim(at.Add(time.Hour - time.Nanosecond)) {
		t.Fatal("before boundary must not be reclaimed")
	}
}

func TestShouldReclaimDisabledWhenNoTimeout(t *testing.T) {
	w := runningWorkspace(at, 0)
	if w.ShouldReclaim(at.Add(1000 * time.Hour)) {
		t.Fatal("zero idle timeout means never reclaim")
	}
}

func TestShouldReclaimOnlyRunning(t *testing.T) {
	for _, status := range []string{
		StatusPending, StatusStarting, StatusStopping,
		StatusStopped, StatusFailed, StatusDeleted,
	} {
		w := runningWorkspace(at, time.Hour)
		w.Status = status
		if w.ShouldReclaim(at.Add(10 * time.Hour)) {
			t.Fatalf("status %q must not be reclaimable", status)
		}
	}
}

func TestShouldReclaimFallsBackToStartedAt(t *testing.T) {
	started := at
	w := Workspace{
		Status:      StatusRunning,
		IdleTimeout: time.Hour,
		StartedAt:   &started,
	}
	if !w.ShouldReclaim(at.Add(2 * time.Hour)) {
		t.Fatal("never-accessed workspace must fall back to StartedAt")
	}
	if w.ShouldReclaim(at.Add(30 * time.Minute)) {
		t.Fatal("within timeout from StartedAt must not be reclaimed")
	}
}

func TestShouldReclaimSkipsWhenNoReferenceTime(t *testing.T) {
	w := Workspace{Status: StatusRunning, IdleTimeout: time.Hour}
	if w.ShouldReclaim(at.Add(100 * time.Hour)) {
		t.Fatal("missing reference time must not trigger reclaim")
	}
}

func TestTouchUpdatesLastActive(t *testing.T) {
	w := runningWorkspace(at, time.Hour)
	later := at.Add(time.Minute)
	if !w.Touch(later) {
		t.Fatal("Touch with a newer timestamp must update")
	}
	if !w.LastActiveAt.Equal(later) {
		t.Fatalf("LastActiveAt = %v, want %v", w.LastActiveAt, later)
	}
}

func TestTouchRejectsStaleTimestamp(t *testing.T) {
	w := runningWorkspace(at, time.Hour)
	if w.Touch(at.Add(-time.Minute)) {
		t.Fatal("stale timestamp must be rejected")
	}
	if !w.LastActiveAt.Equal(at) {
		t.Fatalf("LastActiveAt must stay at %v, got %v", at, w.LastActiveAt)
	}
	if w.Touch(at) {
		t.Fatal("equal timestamp is not an update")
	}
}

func TestTouchInitializesWhenAbsent(t *testing.T) {
	w := Workspace{Status: StatusRunning}
	if !w.Touch(at) {
		t.Fatal("Touch must initialize an absent LastActiveAt")
	}
	if w.LastActiveAt == nil || !w.LastActiveAt.Equal(at) {
		t.Fatalf("LastActiveAt = %v, want %v", w.LastActiveAt, at)
	}
}

func TestTouchRejectedOnTerminalWorkspace(t *testing.T) {
	for _, status := range []string{StatusStopped, StatusFailed, StatusDeleted} {
		w := Workspace{Status: status}
		if w.Touch(at) {
			t.Fatalf("Touch must be rejected in status %q", status)
		}
		if w.LastActiveAt != nil {
			t.Fatalf("LastActiveAt must stay nil in status %q", status)
		}
	}
}

func TestIdleDuration(t *testing.T) {
	w := runningWorkspace(at, time.Hour)
	if got := w.IdleDuration(at.Add(30 * time.Minute)); got != 30*time.Minute {
		t.Fatalf("IdleDuration = %v, want 30m", got)
	}
	
	if got := w.IdleDuration(at.Add(-time.Hour)); got != 0 {
		t.Fatalf("IdleDuration on clock skew = %v, want 0", got)
	}
	
	empty := Workspace{Status: StatusRunning}
	if got := empty.IdleDuration(at); got != 0 {
		t.Fatalf("IdleDuration without reference = %v, want 0", got)
	}
}