package dataset

import (
	"context"
	"errors"
	"testing"

	"fuze-ai-paas/backend/internal/domain/gpu"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type fakeDatasetRepo struct {
	saved   *models.Dataset
	deleted string
	updErr  error
	delErr  error
}

func (f *fakeDatasetRepo) UpdateDataset(ds *models.Dataset) error {
	if f.updErr != nil {
		return f.updErr
	}
	d := *ds
	f.saved = &d
	return nil
}
func (f *fakeDatasetRepo) DeleteDataset(id string) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.deleted = id
	return nil
}

type fakeDatasetClient struct {
	enabled  bool
	createFn func(context.Context, *models.Dataset) error
	deleteFn func(context.Context, *models.Dataset) error
}

func (c *fakeDatasetClient) Enabled() bool { return c.enabled }
func (c *fakeDatasetClient) Namespace() string { return "default" }
func (c *fakeDatasetClient) ServerVersion(context.Context) (string, error) { return "", nil }
func (c *fakeDatasetClient) DiscoverGPUInventory(context.Context) ([]gpu.GPUDevice, error) {
	return nil, nil
}
func (c *fakeDatasetClient) CreateVolcanoJob(context.Context, *models.Job) (string, error) {
	return "", nil
}
func (c *fakeDatasetClient) DeleteVolcanoJob(context.Context, string) error { return nil }
func (c *fakeDatasetClient) GetVolcanoJobStatus(context.Context, string) (models.JobState, error) {
	return "", nil
}
func (c *fakeDatasetClient) SyncJobStatuses(context.Context) (map[string]models.JobState, error) {
	return nil, nil
}
func (c *fakeDatasetClient) GetJobLogs(context.Context, *models.Job, ports.LogQuery) (ports.JobLogs, error) {
	return ports.JobLogs{}, nil
}
func (c *fakeDatasetClient) CreateInferenceService(context.Context, *models.InferenceService) (string, error) {
	return "", nil
}
func (c *fakeDatasetClient) DeleteInferenceService(context.Context, string) error { return nil }
func (c *fakeDatasetClient) CreateDataset(ctx context.Context, ds *models.Dataset) error {
	if c.createFn != nil {
		return c.createFn(ctx, ds)
	}
	return nil
}
func (c *fakeDatasetClient) DeleteDataset(ctx context.Context, ds *models.Dataset) error {
	if c.deleteFn != nil {
		return c.deleteFn(ctx, ds)
	}
	return nil
}

type fakeDatasetRegistry struct {
	client ports.ClusterClientPort
	err    error
}

func (r *fakeDatasetRegistry) Register(*models.Cluster) error { return nil }
func (r *fakeDatasetRegistry) Unregister(string)              {}
func (r *fakeDatasetRegistry) Get(string) (ports.ClusterClientPort, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.client, nil
}
func (r *fakeDatasetRegistry) List() []string { return nil }
func (r *fakeDatasetRegistry) LoadAll([]models.Cluster) []error { return nil }
func (r *fakeDatasetRegistry) K8sClient(string) (ports.ClusterClientPort, error) { return nil, nil }

var _ ports.ClusterRegistry = (*fakeDatasetRegistry)(nil)
var _ ports.ClusterClientPort = (*fakeDatasetClient)(nil)
var _ ports.DatasetRepository = (*fakeDatasetRepo)(nil)

func TestCreateRealClusterBindsPending(t *testing.T) {
	repo := &fakeDatasetRepo{}
	client := &fakeDatasetClient{enabled: true}
	svc := NewService(repo, &fakeDatasetRegistry{client: client})

	ds := &models.Dataset{ID: "d1", ClusterID: "c1"}
	if err := svc.Create(ds); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if ds.Status != models.DatasetStatusPending {
		t.Errorf("expected pending status on real cluster create, got %s", ds.Status)
	}
	if repo.saved == nil || repo.saved.ID != "d1" {
		t.Fatal("expected dataset to be persisted")
	}
}

func TestCreateRealClusterFailureMarksFailed(t *testing.T) {
	repo := &fakeDatasetRepo{}
	client := &fakeDatasetClient{enabled: true, createFn: func(context.Context, *models.Dataset) error {
		return errors.New("k8s reject")
	}}
	svc := NewService(repo, &fakeDatasetRegistry{client: client})

	ds := &models.Dataset{ID: "d1", ClusterID: "c1"}
	if err := svc.Create(ds); err == nil {
		t.Fatal("expected create error to propagate")
	}
	if ds.Status != models.DatasetStatusFailed {
		t.Errorf("expected failed status, got %s", ds.Status)
	}
}

func TestCreateMockClusterBinds(t *testing.T) {
	repo := &fakeDatasetRepo{}
	
	svc := NewService(repo, &fakeDatasetRegistry{err: errors.New("no client")})

	ds := &models.Dataset{ID: "d1", ClusterID: "c1"}
	if err := svc.Create(ds); err != nil {
		t.Fatalf("mock Create returned error: %v", err)
	}
	if ds.Status != models.DatasetStatusBound {
		t.Errorf("expected bound status in mock mode, got %s", ds.Status)
	}
}

func TestDeleteRealClusterThenRepo(t *testing.T) {
	repo := &fakeDatasetRepo{}
	client := &fakeDatasetClient{enabled: true}
	svc := NewService(repo, &fakeDatasetRegistry{client: client})

	ds := &models.Dataset{ID: "d1", ClusterID: "c1"}
	if err := svc.Delete(ds); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if repo.deleted != "d1" {
		t.Errorf("expected repo delete for d1, got %q", repo.deleted)
	}
}