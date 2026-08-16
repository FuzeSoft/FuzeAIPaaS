package workspace

import (
	"testing"
	"time"
)

var at = time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)

func TestIsTerminal(t *testing.T) {
	cases := map[string]bool{
		StatusPending:  false,
		StatusStarting: false,
		StatusRunning:  false,
		StatusStopping: false,
		StatusStopped:  true,
		StatusFailed:   true,
		StatusDeleted:  true,
	}
	for status, want := range cases {
		w := Workspace{Status: status}
		if got := w.IsTerminal(); got != want {
			t.Fatalf("IsTerminal(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestCanTransitionRejectsUnknownStatus(t *testing.T) {
	if CanTransition("bogus", StatusRunning) {
		t.Fatal("unknown source status must not permit any transition")
	}
	w := Workspace{Status: "bogus"}
	if w.CanStart() || w.CanStop() || w.CanDelete() {
		t.Fatal("unknown status must not permit start/stop/delete")
	}
}

func TestTransitionMatrix(t *testing.T) {
	all := []string{
		StatusPending, StatusStarting, StatusRunning,
		StatusStopping, StatusStopped, StatusFailed, StatusDeleted,
	}

	allowed := map[string]map[string]bool{
		StatusPending:  {StatusStarting: true, StatusFailed: true, StatusDeleted: true},
		StatusStarting: {StatusRunning: true, StatusFailed: true, StatusStopping: true},
		StatusRunning:  {StatusStopping: true, StatusFailed: true},
		StatusStopping: {StatusStopped: true, StatusFailed: true},
		StatusStopped:  {StatusStarting: true, StatusDeleted: true},
		
		StatusFailed:  {StatusStarting: true, StatusDeleted: true},
		StatusDeleted: {},
	}

	markers := map[string]func(*Workspace) bool{
		StatusStarting: func(w *Workspace) bool { return w.MarkStarting() },
		StatusRunning:  func(w *Workspace) bool { return w.MarkRunning(at) },
		StatusStopping: func(w *Workspace) bool { return w.MarkStopping() },
		StatusStopped:  func(w *Workspace) bool { return w.MarkStopped(at) },
		StatusFailed:   func(w *Workspace) bool { return w.MarkFailed("boom", at) },
		StatusDeleted:  func(w *Workspace) bool { return w.MarkDeleted() },
	}

	for _, from := range all {
		for to, mark := range markers {
			w := Workspace{Status: from}
			got := mark(&w)
			want := allowed[from][to]

			if got != want {
				t.Fatalf("transition %s -> %s = %v, want %v", from, to, got, want)
			}
			if want && w.Status != to {
				t.Fatalf("transition %s -> %s succeeded but status is %q", from, to, w.Status)
			}
			
			if !want && w.Status != from {
				t.Fatalf("rejected transition %s -> %s mutated status to %q", from, to, w.Status)
			}
		}
	}
}

func TestRepeatedTransitionIsRejected(t *testing.T) {
	w := Workspace{Status: StatusStarting}
	if !w.MarkRunning(at) {
		t.Fatal("first MarkRunning must succeed")
	}
	if w.MarkRunning(at.Add(time.Hour)) {
		t.Fatal("repeated MarkRunning must be rejected")
	}
	if w.Status != StatusRunning {
		t.Fatalf("status changed on rejected transition: %q", w.Status)
	}
}

func TestMarkRunningStampsStartedAtOnce(t *testing.T) {
	w := Workspace{Status: StatusStarting}
	w.MarkRunning(at)

	if w.StartedAt == nil || !w.StartedAt.Equal(at) {
		t.Fatalf("StartedAt = %v, want %v", w.StartedAt, at)
	}
	
	if w.LastActiveAt == nil || !w.LastActiveAt.Equal(at) {
		t.Fatalf("LastActiveAt = %v, want %v", w.LastActiveAt, at)
	}

	later := at.Add(2 * time.Hour)
	w.MarkStopping()
	w.MarkStopped(later)
	w.MarkStarting()
	relaunch := later.Add(time.Hour)
	w.MarkRunning(relaunch)
	if w.StartedAt == nil || !w.StartedAt.Equal(relaunch) {
		t.Fatalf("restarted StartedAt = %v, want %v", w.StartedAt, relaunch)
	}
}

func TestMarkStartingClearsPreviousRunResidue(t *testing.T) {
	w := Workspace{Status: StatusFailed, FailureReason: "image pull failed"}
	stopped := at
	w.StoppedAt = &stopped

	if !w.MarkStarting() {
		t.Fatal("failed workspace must be restartable")
	}
	if w.StoppedAt != nil {
		t.Fatalf("StoppedAt must be cleared on restart, got %v", w.StoppedAt)
	}
	if w.FailureReason != "" {
		t.Fatalf("FailureReason must be cleared on restart, got %q", w.FailureReason)
	}
}

func TestMarkStoppedStampsStoppedAt(t *testing.T) {
	w := Workspace{Status: StatusStopping}
	w.MarkStopped(at)
	if w.StoppedAt == nil || !w.StoppedAt.Equal(at) {
		t.Fatalf("StoppedAt = %v, want %v", w.StoppedAt, at)
	}
	
	if w.URL != "" {
		t.Fatalf("URL must be cleared when stopped, got %q", w.URL)
	}
}

func TestMarkFailedRecordsReasonAndStopTime(t *testing.T) {
	w := Workspace{Status: StatusRunning}
	if !w.MarkFailed("OOMKilled", at) {
		t.Fatal("running workspace must be markable as failed")
	}
	if w.FailureReason != "OOMKilled" {
		t.Fatalf("FailureReason = %q", w.FailureReason)
	}
	if w.StoppedAt == nil || !w.StoppedAt.Equal(at) {
		t.Fatalf("StoppedAt = %v, want %v", w.StoppedAt, at)
	}
}

func TestCanStartStopDelete(t *testing.T) {
	type want struct{ start, stop, del bool }
	cases := map[string]want{
		StatusPending:  {start: true, stop: false, del: true},
		StatusStarting: {start: false, stop: true, del: false},
		StatusRunning:  {start: false, stop: true, del: false},
		StatusStopping: {start: false, stop: false, del: false},
		StatusStopped:  {start: true, stop: false, del: true},
		StatusFailed:   {start: true, stop: false, del: true},
		StatusDeleted:  {start: false, stop: false, del: false},
	}
	for status, w := range cases {
		ws := Workspace{Status: status}
		if got := ws.CanStart(); got != w.start {
			t.Fatalf("CanStart(%q) = %v, want %v", status, got, w.start)
		}
		if got := ws.CanStop(); got != w.stop {
			t.Fatalf("CanStop(%q) = %v, want %v", status, got, w.stop)
		}
		if got := ws.CanDelete(); got != w.del {
			t.Fatalf("CanDelete(%q) = %v, want %v", status, got, w.del)
		}
	}
}

func TestIsActive(t *testing.T) {
	cases := map[string]bool{
		StatusPending:  false,
		StatusStarting: true,
		StatusRunning:  true,
		StatusStopping: true,
		StatusStopped:  false,
		StatusFailed:   false,
		StatusDeleted:  false,
	}
	for status, want := range cases {
		w := Workspace{Status: status}
		if got := w.IsActive(); got != want {
			t.Fatalf("IsActive(%q) = %v, want %v", status, got, want)
		}
	}
}