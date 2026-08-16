package tests

import (
	"testing"

	"fuze-ai-paas/backend/internal/domain/event"
	"fuze-ai-paas/backend/internal/telemetry"
)

func TestTelemetryCreation(t *testing.T) {
	t.Run("create telemetry collector", func(t *testing.T) {
		tc := telemetry.NewCollector()
		if tc == nil {
			t.Fatal("expected non-nil collector")
		}

		snap := tc.Snapshot()
		if snap.ClustersDiscovered != 0 || snap.JobsSubmitted != 0 {
			t.Fatal("expected zero-valued snapshot")
		}
	})
}

func TestTelemetryRecordCluster(t *testing.T) {
	t.Run("record cluster discovery", func(t *testing.T) {
		tc := telemetry.NewCollector()
		evt := event.ClusterDiscovered{
			ClusterID: "cluster-001",
			TotalGPUs: 8,
		}
		tc.RecordCluster(evt)

		snap := tc.Snapshot()
		if snap.ClustersDiscovered < 1 {
			t.Errorf("expected at least 1 cluster discovered, got %d", snap.ClustersDiscovered)
		}
		if snap.GPUsDiscovered < 8 {
			t.Errorf("expected at least 8 GPUs discovered, got %d", snap.GPUsDiscovered)
		}
	})
}

func TestTelemetryRecordSubmit(t *testing.T) {
	t.Run("record job submission", func(t *testing.T) {
		tc := telemetry.NewCollector()
		evt := event.JobSubmitted{
			JobID: "job-001",
			GPUs:  4,
		}
		tc.RecordSubmit(evt)

		snap := tc.Snapshot()
		if snap.JobsSubmitted < 1 {
			t.Errorf("expected at least 1 job submitted, got %d", snap.JobsSubmitted)
		}
		if snap.GPUsSubmitted < 4 {
			t.Errorf("expected at least 4 GPUs submitted, got %d", snap.GPUsSubmitted)
		}
	})
}

func TestTelemetryRecordAssign(t *testing.T) {
	t.Run("record assignment completion", func(t *testing.T) {
		tc := telemetry.NewCollector()
		evt := event.AssignmentCompleted{
			JobID:         "job-001",
			AllocatedGPUs: 4,
		}
		tc.RecordAssign(evt)

		snap := tc.Snapshot()
		if snap.AssignmentsDone < 1 {
			t.Errorf("expected at least 1 assignment done, got %d", snap.AssignmentsDone)
		}
		if snap.GPUsAllocated < 4 {
			t.Errorf("expected at least 4 GPUs allocated, got %d", snap.GPUsAllocated)
		}
	})
}

func TestTelemetrySnapshotIsolation(t *testing.T) {
	t.Run("snapshot returns independent copy", func(t *testing.T) {
		tc := telemetry.NewCollector()

		snap1 := tc.Snapshot()
		snap2 := tc.Snapshot()

		if snap1.ClustersDiscovered != snap2.ClustersDiscovered {
			t.Error("snapshots should match when no new events recorded")
		}
	})
}