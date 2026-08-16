package job

import "testing"

func TestVolcanoStateToJobStatus(t *testing.T) {
	cases := map[JobState]JobStatus{
		JobStatePending:    JobStatusPending,
		JobStateRunning:    JobStatusRunning,
		JobStateCompleted:  JobStatusCompleted,
		JobStateFailed:     JobStatusFailed,
		JobStateAborted:    JobStatusFailed,
		JobStateTerminated: JobStatusFailed,
		JobStateRestarting: JobStatusPending,
	}
	for state, want := range cases {
		if got := VolcanoStateToJobStatus(state); got != want {
			t.Fatalf("状态 %s: 期望 %s，实际 %s", state, want, got)
		}
	}
}