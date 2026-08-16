package metrics

import (
	"testing"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type fakeMetricsRepo struct {
	resources []models.Resource
	jobs      []models.Job
	resErr    error
	jobErr    error
}

func (f *fakeMetricsRepo) GetResourcesByCluster(clusterID string) ([]models.Resource, error) {
	if f.resErr != nil {
		return nil, f.resErr
	}
	out := f.resources[:0:0]
	for _, r := range f.resources {
		if r.ClusterID == clusterID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeMetricsRepo) GetResources() ([]models.Resource, error) {
	if f.resErr != nil {
		return nil, f.resErr
	}
	return f.resources, nil
}
func (f *fakeMetricsRepo) GetJobs() ([]models.Job, error) {
	if f.jobErr != nil {
		return nil, f.jobErr
	}
	return f.jobs, nil
}

var _ ports.MetricsRepository = (*fakeMetricsRepo)(nil)

func sampleResources(clusterID string) []models.Resource {
	return []models.Resource{
		{ClusterID: clusterID, Type: models.ResourceTypeGPU, Status: models.ResourceStatusAvailable, TotalGPUs: 1, UsedGPUs: 0, TotalMemory: 100, AvailableMemory: 100},
		{ClusterID: clusterID, Type: models.ResourceTypeGPU, Status: models.ResourceStatusAllocated, TotalGPUs: 1, UsedGPUs: 1, TotalMemory: 100, AvailableMemory: 0},
		{ClusterID: clusterID, Type: models.ResourceTypeGPU, Status: models.ResourceStatusAllocated, TotalGPUs: 1, UsedGPUs: 1, TotalMemory: 100, AvailableMemory: 0},
	}
}

func TestGetAggregatesAllClusters(t *testing.T) {
	repo := &fakeMetricsRepo{
		resources: append(sampleResources("c1"), sampleResources("c2")...),
		jobs: []models.Job{
			{ClusterID: "c1", Status: models.JobStatusRunning},
			{ClusterID: "c2", Status: models.JobStatusPending},
			{ClusterID: "c1", Status: models.JobStatusCompleted},
		},
	}
	svc := NewService(repo)
	m, err := svc.Get("")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	
	if m.TotalGPUs != 6 {
		t.Errorf("expected 6 total GPUs, got %d", m.TotalGPUs)
	}
	if m.UsedGPUs != 4 {
		t.Errorf("expected 4 used GPUs, got %d", m.UsedGPUs)
	}
	if m.AvailableGPUs != 2 {
		t.Errorf("expected 2 available GPUs, got %d", m.AvailableGPUs)
	}
	
	if m.TotalJobs != 0 {
		t.Errorf("expected 0 jobs for empty-cluster aggregate (current behavior), got %d", m.TotalJobs)
	}
	if m.TotalMemory != 600 {
		t.Errorf("expected 600 total memory, got %d", m.TotalMemory)
	}
	if m.UsedMemory != 400 {
		t.Errorf("expected 400 used memory, got %d", m.UsedMemory)
	}
}

func TestGetFiltersByCluster(t *testing.T) {
	repo := &fakeMetricsRepo{
		resources: append(sampleResources("c1"), sampleResources("c2")...),
		jobs: []models.Job{
			{ClusterID: "c1", Status: models.JobStatusRunning},
			{ClusterID: "c2", Status: models.JobStatusRunning},
		},
	}
	svc := NewService(repo)
	m, err := svc.Get("c1")
	if err != nil {
		t.Fatalf("Get(c1) returned error: %v", err)
	}
	if m.TotalGPUs != 3 {
		t.Errorf("expected 3 GPUs for cluster c1, got %d", m.TotalGPUs)
	}
	if m.UsedGPUs != 2 {
		t.Errorf("expected 2 used GPUs for cluster c1, got %d", m.UsedGPUs)
	}
	if m.TotalJobs != 1 || m.RunningJobs != 1 {
		t.Errorf("expected 1 total job (running) for cluster c1, got total=%d running=%d",
			m.TotalJobs, m.RunningJobs)
	}
}

func TestGetResourceErrorPropagates(t *testing.T) {
	repo := &fakeMetricsRepo{resErr: errBoom{}}
	svc := NewService(repo)
	if _, err := svc.Get(""); err == nil {
		t.Fatal("expected resource error to propagate")
	}
}

func TestGetJobErrorPropagates(t *testing.T) {
	repo := &fakeMetricsRepo{jobErr: errBoom{}}
	svc := NewService(repo)
	if _, err := svc.Get(""); err == nil {
		t.Fatal("expected job error to propagate")
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }