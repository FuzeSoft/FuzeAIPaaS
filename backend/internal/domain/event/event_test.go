package event

import "testing"

func TestClusterDiscoveredFields(t *testing.T) {
	e := NewClusterDiscovered("c1", "C1", ClusterStats{NodeCount: 2, TotalGPUs: 16, UsedGPUs: 4, Status: "healthy", Version: "v1.28"})
	if e.EventType() != ClusterDiscoveredType {
		t.Fatalf("type=%s", e.EventType())
	}
	if e.AggregateID() != "c1" {
		t.Fatalf("aggregate=%s", e.AggregateID())
	}
	if e.TotalGPUs != 16 || e.UsedGPUs != 4 || e.NodeCount != 2 || e.Version != "v1.28" {
		t.Fatalf("stats mismatch: %+v", e)
	}
	if e.OccurredAt().IsZero() {
		t.Fatal("occurred at is zero")
	}
}

func TestJobSubmittedFields(t *testing.T) {
	j := NewJobSubmitted("j1", "c1", "training", 8, "t1")
	if j.EventType() != JobSubmittedType || j.AggregateID() != "j1" {
		t.Fatalf("meta mismatch: %+v", j)
	}
	if j.GPUs != 8 || j.TenantID != "t1" || j.JobType != "training" {
		t.Fatalf("payload mismatch: %+v", j)
	}
}

func TestAssignmentCompletedFields(t *testing.T) {
	a := NewAssignmentCompleted("j1", "c1", 8, 640)
	if a.EventType() != AssignmentCompletedType || a.AggregateID() != "j1" {
		t.Fatalf("meta mismatch: %+v", a)
	}
	if a.AllocatedGPUs != 8 || a.MemoryMiB != 640 {
		t.Fatalf("payload mismatch: %+v", a)
	}
}