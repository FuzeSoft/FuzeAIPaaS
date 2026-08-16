package telemetry

import (
	"testing"

	domainevent "fuze-ai-paas/backend/internal/domain/event"
)

func TestCollectorRecordsAndSnapshots(t *testing.T) {
	c := NewCollector()

	c.RecordCluster(domainevent.NewClusterDiscovered("c1", "cluster-1", domainevent.ClusterStats{TotalGPUs: 8}))
	c.RecordCluster(domainevent.NewClusterDiscovered("c2", "cluster-2", domainevent.ClusterStats{TotalGPUs: 4}))
	c.RecordSubmit(domainevent.NewJobSubmitted("j1", "c1", "train", 4, "t1"))
	c.RecordAssign(domainevent.NewAssignmentCompleted("j1", "c1", 4, 8192))

	s := c.Snapshot()
	if s.ClustersDiscovered != 2 {
		t.Errorf("ClustersDiscovered = %d, want 2", s.ClustersDiscovered)
	}
	if s.GPUsDiscovered != 12 {
		t.Errorf("GPUsDiscovered = %d, want 12", s.GPUsDiscovered)
	}
	if s.JobsSubmitted != 1 {
		t.Errorf("JobsSubmitted = %d, want 1", s.JobsSubmitted)
	}
	if s.GPUsSubmitted != 4 {
		t.Errorf("GPUsSubmitted = %d, want 4", s.GPUsSubmitted)
	}
	if s.AssignmentsDone != 1 {
		t.Errorf("AssignmentsDone = %d, want 1", s.AssignmentsDone)
	}
	if s.GPUsAllocated != 4 {
		t.Errorf("GPUsAllocated = %d, want 4", s.GPUsAllocated)
	}
}

func TestCollectorSnapshotIsolated(t *testing.T) {
	c := NewCollector()
	c.RecordSubmit(domainevent.NewJobSubmitted("j1", "c1", "train", 2, "t1"))
	s1 := c.Snapshot()

	c.RecordSubmit(domainevent.NewJobSubmitted("j2", "c1", "train", 3, "t1"))
	s2 := c.Snapshot()

	if s1.JobsSubmitted != 1 {
		t.Errorf("s1.JobsSubmitted = %d, want 1 (snapshot must be isolated)", s1.JobsSubmitted)
	}
	if s2.JobsSubmitted != 2 {
		t.Errorf("s2.JobsSubmitted = %d, want 2", s2.JobsSubmitted)
	}
	if s2.GPUsSubmitted != 5 {
		t.Errorf("s2.GPUsSubmitted = %d, want 5", s2.GPUsSubmitted)
	}
}