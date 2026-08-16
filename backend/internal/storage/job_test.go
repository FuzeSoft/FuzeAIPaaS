package storage

import (
	"path/filepath"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestGetActiveJobsExcludesTerminalOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "active.db")
	db, err := OpenDB(DBConfig{Driver: DriverSQLite, DSN: path})
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	store := NewStorage(db)

	cases := []models.JobStatus{
		models.JobStatusPending,
		models.JobStatusRunning,
		models.JobStatusPaused,
		models.JobStatusRetrying,
		models.JobStatusCompleted,
		models.JobStatusFailed,
		models.JobStatusCancelled,
	}
	for i, st := range cases {
		job := models.Job{
			ID:       string(rune('a' + i)),
			Status:   st,
			GPUs:     1,
			TenantID: "t1",
		}
		if err := db.Create(&job).Error; err != nil {
			t.Fatalf("seed job %s: %v", st, err)
		}
	}

	active, err := store.GetActiveJobs()
	if err != nil {
		t.Fatalf("GetActiveJobs: %v", err)
	}

	got := map[models.JobStatus]bool{}
	for _, j := range active {
		got[j.Status] = true
	}

	for _, st := range []models.JobStatus{
		models.JobStatusPending,
		models.JobStatusRunning,
		models.JobStatusPaused,
		models.JobStatusRetrying,
	} {
		if !got[st] {
			t.Errorf("active set missing non-terminal status %q", st)
		}
	}

	for _, st := range []models.JobStatus{
		models.JobStatusCompleted,
		models.JobStatusFailed,
		models.JobStatusCancelled,
	} {
		if got[st] {
			t.Errorf("active set must not contain terminal status %q", st)
		}
	}
}