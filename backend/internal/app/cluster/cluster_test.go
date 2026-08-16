package cluster

import (
	"context"
	"errors"
	"testing"

	"fuze-ai-paas/backend/internal/domain/gpu"
	"fuze-ai-paas/backend/internal/events"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type fakeClusterRepo struct {
	cluster   *models.Cluster
	resources []models.Resource
	stats     *models.Cluster
	err       error
}

func (f *fakeClusterRepo) GetCluster(id string) (*models.Cluster, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.cluster, nil
}

func (f *fakeClusterRepo) UpsertClusterResources(_ string, incoming []models.Resource) error {
	if f.err != nil {
		return f.err
	}
	f.resources = incoming
	return nil
}

func (f *fakeClusterRepo) UpdateClusterStats(_ string, stats models.Cluster) error {
	if f.err != nil {
		return f.err
	}
	s := stats
	f.stats = &s
	return nil
}

type fakeClient struct {
	enabled bool
	version string
	devices []gpu.GPUDevice
	discErr error
	verErr  error
}

func (c *fakeClient) Enabled() bool { return c.enabled }
func (c *fakeClient) Namespace() string { return "default" }
func (c *fakeClient) ServerVersion(context.Context) (string, error) { return c.version, c.verErr }
func (c *fakeClient) DiscoverGPUInventory(context.Context) ([]gpu.GPUDevice, error) {
	return c.devices, c.discErr
}
func (c *fakeClient) CreateVolcanoJob(context.Context, *models.Job) (string, error) { return "", nil }
func (c *fakeClient) DeleteVolcanoJob(context.Context, string) error                 { return nil }
func (c *fakeClient) GetVolcanoJobStatus(context.Context, string) (models.JobState, error) {
	return "", nil
}
func (c *fakeClient) SyncJobStatuses(context.Context) (map[string]models.JobState, error) {
	return nil, nil
}
func (c *fakeClient) GetJobLogs(context.Context, *models.Job, ports.LogQuery) (ports.JobLogs, error) {
	return ports.JobLogs{}, nil
}
func (c *fakeClient) CreateInferenceService(context.Context, *models.InferenceService) (string, error) {
	return "", nil
}
func (c *fakeClient) DeleteInferenceService(context.Context, string) error { return nil }
func (c *fakeClient) CreateDataset(context.Context, *models.Dataset) error  { return nil }
func (c *fakeClient) DeleteDataset(context.Context, *models.Dataset) error  { return nil }

type fakeRegistry struct {
	client *fakeClient
	err    error
	list   []string
}

func (r *fakeRegistry) Register(*models.Cluster) error { return nil }
func (r *fakeRegistry) Unregister(string)              {}
func (r *fakeRegistry) Get(string) (ports.ClusterClientPort, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.client, nil
}
func (r *fakeRegistry) List() []string { return r.list }
func (r *fakeRegistry) LoadAll([]models.Cluster) []error { return nil }
func (r *fakeRegistry) K8sClient(string) (ports.ClusterClientPort, error) { return nil, nil }

var _ ports.ClusterRegistry = (*fakeRegistry)(nil)
var _ ports.ClusterClientPort = (*fakeClient)(nil)
var _ ports.ClusterRepository = (*fakeClusterRepo)(nil)

func TestExpandNodesToResourcesGPUAndNPU(t *testing.T) {
	devices := []gpu.GPUDevice{
		{NodeName: "n1", Vendor: "NVIDIA", Model: "A100", Total: 2, Used: 1},
		{NodeName: "n2", Vendor: "Ascend", Model: "910B", Total: 1, Used: 0}, 
	}

	resources := ExpandNodesToResources("c1", devices)
	if len(resources) != 3 {
		t.Fatalf("expected 3 per-card resources, got %d", len(resources))
	}
	gpuCount, npuCount, alloc := 0, 0, 0
	for _, r := range resources {
		switch r.Type {
		case models.ResourceTypeNPU:
			npuCount++
		default:
			gpuCount++
		}
		if r.Status == models.ResourceStatusAllocated {
			alloc++
		}
	}
	if gpuCount != 2 || npuCount != 1 {
		t.Errorf("gpu/npu split wrong: gpu=%d npu=%d", gpuCount, npuCount)
	}
	if alloc != 1 {
		t.Errorf("expected 1 allocated card (A100 used=1), got %d", alloc)
	}
}

func TestRefreshHappyPath(t *testing.T) {
	repo := &fakeClusterRepo{cluster: &models.Cluster{ID: "c1", Name: "C1"}}
	client := &fakeClient{enabled: true, version: "v1.28", devices: []gpu.GPUDevice{
		{NodeName: "n1", Vendor: "NVIDIA", Model: "A100", Total: 2, Used: 1},
	}}
	bus := events.NewBus(16, 1)
	bus.Run(context.Background())
	defer bus.Stop()

	svc := NewService(repo, &fakeRegistry{client: client}, bus)
	if err := svc.Refresh(context.Background(), "c1"); err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if repo.stats == nil {
		t.Fatal("cluster stats were not written back")
	}
	if repo.stats.Status != models.ClusterStatusHealthy {
		t.Errorf("expected healthy status, got %s", repo.stats.Status)
	}
	if repo.stats.TotalGPUs != 2 {
		t.Errorf("expected 2 total GPUs, got %d", repo.stats.TotalGPUs)
	}
	if repo.stats.UsedGPUs != 1 {
		t.Errorf("expected 1 used GPU, got %d", repo.stats.UsedGPUs)
	}
	if len(repo.resources) != 2 {
		t.Errorf("expected 2 per-card resources upserted, got %d", len(repo.resources))
	}
}

func TestRefreshDisabledClientReturnsError(t *testing.T) {
	repo := &fakeClusterRepo{cluster: &models.Cluster{ID: "c1"}}
	svc := NewService(repo, &fakeRegistry{client: &fakeClient{enabled: false}}, nil)
	if err := svc.Refresh(context.Background(), "c1"); err == nil {
		t.Fatal("expected error for disabled client")
	}
}

func TestRefreshDiscoverErrorMarksUnhealthy(t *testing.T) {
	repo := &fakeClusterRepo{cluster: &models.Cluster{ID: "c1"}}
	client := &fakeClient{enabled: true, discErr: errors.New("boom")}
	svc := NewService(repo, &fakeRegistry{client: client}, nil)
	if err := svc.Refresh(context.Background(), "c1"); err == nil {
		t.Fatal("expected discover error to propagate")
	}
	if repo.stats == nil || repo.stats.Status != models.ClusterStatusUnhealthy {
		t.Errorf("expected unhealthy status after discover failure")
	}
}

func TestRefreshRegistryGetError(t *testing.T) {
	repo := &fakeClusterRepo{cluster: &models.Cluster{ID: "c1"}}
	svc := NewService(repo, &fakeRegistry{err: errors.New("no such cluster")}, nil)
	if err := svc.Refresh(context.Background(), "c1"); err == nil {
		t.Fatal("expected registry Get error to propagate")
	}
}

func TestServiceList(t *testing.T) {
	svc := NewService(&fakeClusterRepo{}, &fakeRegistry{list: []string{"c1", "c2"}}, nil)
	if got := svc.List(); len(got) != 2 {
		t.Fatalf("expected 2 cluster IDs, got %d", len(got))
	}
}