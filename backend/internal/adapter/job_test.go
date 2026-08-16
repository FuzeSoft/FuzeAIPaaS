package adapter

import (
	"testing"

	"fuze-ai-paas/backend/internal/domain/job"
	"fuze-ai-paas/backend/internal/models"
)

func TestJobRoundTrip(t *testing.T) {
	m := &models.Job{ID: "j1", ClusterID: "c1", Name: "t", Type: models.JobTypeTraining, Status: models.JobStatusRunning, Memory: 100}
	agg := JobFromModel(m)
	if agg == nil || agg.ID != m.ID || agg.Type != job.JobTypeTraining || agg.Status != job.JobStatusRunning {
		t.Fatalf("JobFromModel 丢失字段: %+v", agg)
	}
	JobSyncToModel(agg, m)
	if m.Status != models.JobStatusRunning {
		t.Fatalf("JobSyncToModel 应回写 Status，实际 %s", m.Status)
	}
}

func TestJobFromModelNil(t *testing.T) {
	if JobFromModel(nil) != nil {
		t.Fatal("JobFromModel(nil) 应返回 nil")
	}
}

func TestResourceRoundTrip(t *testing.T) {
	m := models.Resource{
		ID: "r1", ClusterID: "c1", Name: "n", Type: models.ResourceTypeNPU,
		Vendor: "HW", Model: "Ascend", TotalGPUs: 4, UsedGPUs: 1,
		TotalMemory: 32, AvailableMemory: 16, Status: models.ResourceStatusAllocated, NodeName: "node1",
	}
	r := ResourceFromModel(m)
	back := ResourceToModel(r)
	if back != m {
		t.Fatalf("Resource 往返丢失字段:\n got %+v\nwant %+v", back, m)
	}
}

func TestJobStateFromModel(t *testing.T) {
	if JobStateFromModel(models.JobStateRunning) != job.JobStateRunning {
		t.Fatal("JobStateFromModel 应保留枚举值")
	}
}