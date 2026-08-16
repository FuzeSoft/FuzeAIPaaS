package scheduler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"fuze-ai-paas/backend/internal/domain/gpu"
	"fuze-ai-paas/backend/internal/domain/inference"
	"fuze-ai-paas/backend/internal/k8s"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type mockJobRepo struct {
	jobs             []models.Job
	jobsByID         map[string]*models.Job
	pending          []models.Job
	seq              int
	resourcesByCl    map[string][]models.Resource
	allResources     []models.Resource
	updatedJobs      []models.Job
	updatedResources []models.Resource
	infServices      map[string]*models.InferenceService
	updatedInf       []models.InferenceService

	mu sync.Mutex

	submitCalls   int64
	reconcileCalls int64
}

func (m *mockJobRepo) GetJobs() ([]models.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs, nil
}

func (m *mockJobRepo) UpdateJob(job *models.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updatedJobs = append(m.updatedJobs, *job)
	return nil
}

func (m *mockJobRepo) UpdateJobStatus(job *models.Job) error {
	return m.UpdateJob(job)
}

func (m *mockJobRepo) GetPendingJobs() ([]models.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	atomic.AddInt64(&m.submitCalls, 1)
	return m.pending, nil
}

func (m *mockJobRepo) GetJob(id string) (*models.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobsByID[id]; ok {
		return j, nil
	}
	return nil, fmt.Errorf("job %s not found", id)
}

func (m *mockJobRepo) CreateJob(job *models.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	job.ID = fmt.Sprintf("job-%d", m.seq)
	if m.jobsByID == nil {
		m.jobsByID = map[string]*models.Job{}
	}
	m.jobsByID[job.ID] = job
	m.jobs = append(m.jobs, *job)
	return nil
}

func (m *mockJobRepo) GetJobsByTenant(tenantID string) ([]models.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []models.Job
	for _, j := range m.jobs {
		if tenantID == "" || j.TenantID == tenantID {
			out = append(out, j)
		}
	}
	return out, nil
}

func (m *mockJobRepo) UpdateJobSpec(job *models.Job) error { return nil }

func (m *mockJobRepo) DeleteJob(id string) error { return nil }

func (m *mockJobRepo) GetActiveJobs() ([]models.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var active []models.Job
	for _, j := range m.jobs {
		if !j.IsTerminal() {
			active = append(active, j)
		}
	}
	return active, nil
}

func (m *mockJobRepo) GetResourcesByCluster(clusterID string) ([]models.Resource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resourcesByCl[clusterID], nil
}

func (m *mockJobRepo) GetResources() ([]models.Resource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.allResources, nil
}

func (m *mockJobRepo) UpdateResource(resource *models.Resource) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updatedResources = append(m.updatedResources, *resource)
	return nil
}

func (m *mockJobRepo) GetInferenceServices() ([]models.InferenceService, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	atomic.AddInt64(&m.reconcileCalls, 1)
	out := make([]models.InferenceService, 0, len(m.infServices))
	for _, s := range m.infServices {
		out = append(out, *s)
	}
	return out, nil
}

func (m *mockJobRepo) GetInferenceService(id string) (*models.InferenceService, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.infServices[id]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, fmt.Errorf("inference service %s not found", id)
}

func (m *mockJobRepo) UpdateInferenceService(svc *models.InferenceService) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updatedInf = append(m.updatedInf, *svc)
	if m.infServices == nil {
		m.infServices = map[string]*models.InferenceService{}
	}
	cp := *svc
	m.infServices[svc.ID] = &cp
	return nil
}

func (m *mockJobRepo) UpdateInferenceRuntimeStatus(svc *models.InferenceService) error {
	return m.UpdateInferenceService(svc)
}
func (m *mockJobRepo) UpdateInferenceServiceSpec(svc *models.InferenceService) error {
	return m.UpdateInferenceService(svc)
}
func (m *mockJobRepo) DeleteInferenceService(id string) error { return nil }
func (m *mockJobRepo) UpdateDataset(ds *models.Dataset) error { return nil }
func (m *mockJobRepo) DeleteDataset(id string) error          { return nil }

type mockClusterRepo struct {
	mu      sync.Mutex
	clusters map[string]*models.Cluster
	stats    map[string]models.Cluster
	upserts  map[string][]models.Resource

	discoverCalls int64
}

func newMockClusterRepo() *mockClusterRepo {
	return &mockClusterRepo{
		clusters: map[string]*models.Cluster{},
		stats:    map[string]models.Cluster{},
		upserts:  map[string][]models.Resource{},
	}
}

func (m *mockClusterRepo) GetCluster(id string) (*models.Cluster, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.clusters[id]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("cluster %s not found", id)
}

func (m *mockClusterRepo) GetEnabledClusters() ([]models.Cluster, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.clusters) == 0 {
		return nil, nil
	}
	out := make([]models.Cluster, 0, len(m.clusters))
	for _, c := range m.clusters {
		if c.Enabled {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (m *mockClusterRepo) UpsertClusterResources(clusterID string, incoming []models.Resource) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	atomic.AddInt64(&m.discoverCalls, 1)
	m.upserts[clusterID] = incoming
	return nil
}

func (m *mockClusterRepo) UpdateClusterStats(id string, stats models.Cluster) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats[id] = stats
	return nil
}

func (m *mockClusterRepo) CreateCluster(cluster *models.Cluster) error { return nil }
func (m *mockClusterRepo) UpdateCluster(cluster *models.Cluster) error { return nil }
func (m *mockClusterRepo) DeleteCluster(id string) error               { return nil }

type mockClusterClient struct {
	enabled   bool
	namespace string
	version   string
	devices   []gpu.GPUDevice
	statuses  map[string]models.JobState
}

func (c *mockClusterClient) Enabled() bool { return c.enabled }

func (c *mockClusterClient) Namespace() string {
	if c.namespace == "" {
		return "default"
	}
	return c.namespace
}

func (c *mockClusterClient) ServerVersion(ctx context.Context) (string, error) {
	return c.version, nil
}

func (c *mockClusterClient) DiscoverGPUInventory(ctx context.Context) ([]gpu.GPUDevice, error) {
	return c.devices, nil
}

func (c *mockClusterClient) CreateVolcanoJob(ctx context.Context, job *models.Job) (string, error) {
	return "vj-" + job.ID, nil
}

func (c *mockClusterClient) DeleteVolcanoJob(ctx context.Context, name string) error { return nil }

func (c *mockClusterClient) GetVolcanoJobStatus(ctx context.Context, name string) (models.JobState, error) {
	return models.JobStatePending, nil
}

func (c *mockClusterClient) SyncJobStatuses(ctx context.Context) (map[string]models.JobState, error) {
	return c.statuses, nil
}

func (c *mockClusterClient) CreateInferenceService(ctx context.Context, svc *models.InferenceService) (string, error) {
	return "", nil
}

func (c *mockClusterClient) DeleteInferenceService(ctx context.Context, name string) error {
	return nil
}
func (c *mockClusterClient) CreateDataset(ctx context.Context, ds *models.Dataset) error { return nil }
func (c *mockClusterClient) DeleteDataset(ctx context.Context, ds *models.Dataset) error { return nil }
func (c *mockClusterClient) GetJobLogs(ctx context.Context, job *models.Job, q k8s.LogQuery) (k8s.JobLogs, error) {
	return k8s.JobLogs{}, nil
}

type mockRegistry struct {
	clients map[string]k8s.ClusterClientPort
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{clients: map[string]k8s.ClusterClientPort{}}
}

func (m *mockRegistry) Register(cluster *models.Cluster) error { return nil }
func (m *mockRegistry) Unregister(clusterID string)            {}

func (m *mockRegistry) Get(clusterID string) (k8s.ClusterClientPort, error) {
	if c, ok := m.clients[clusterID]; ok {
		return c, nil
	}
	
	return nil, fmt.Errorf("%w: %s", k8s.ErrClusterNotRegistered, clusterID)
}

func (m *mockRegistry) List() []string {
	ids := make([]string, 0, len(m.clients))
	for id := range m.clients {
		ids = append(ids, id)
	}
	return ids
}

func (m *mockRegistry) LoadAll(clusters []models.Cluster) []error { return nil }

func (m *mockRegistry) K8sClient(clusterID string) (ports.ClusterClientPort, error) {
	return nil, fmt.Errorf("no k8s client in mock registry")
}

type mockRuntimeClient struct{}

func (c *mockRuntimeClient) Deploy(ctx context.Context, svc *inference.InferenceService) (string, error) {
	return svc.Name, nil
}
func (c *mockRuntimeClient) Undeploy(ctx context.Context, runtimeName string) error { return nil }
func (c *mockRuntimeClient) Status(ctx context.Context, runtimeName string) (bool, bool, bool, int, string, error) {
	return false, false, false, 0, "", nil
}
func (c *mockRuntimeClient) Scale(ctx context.Context, runtimeName string, replicas int) error {
	return nil
}
func (c *mockRuntimeClient) RolloutCanary(ctx context.Context, runtimeName string, weight int) error {
	return nil
}

type mockRuntimeRegistry struct{}

func newMockRuntimeRegistry() *mockRuntimeRegistry { return &mockRuntimeRegistry{} }

func (r *mockRuntimeRegistry) For(_ string, _ inference.RuntimeKind, _ ports.ClusterClientPort) (inference.RuntimeClient, error) {
	return &mockRuntimeClient{}, nil
}

var (
	_ ports.JobRepository     = (*mockJobRepo)(nil)
	_ ports.ClusterRepository = (*mockClusterRepo)(nil)
	_ k8s.ClusterClientPort   = (*mockClusterClient)(nil)
	_ k8s.ClusterRegistry     = (*mockRegistry)(nil)
	_ ports.RuntimeRegistry   = (*mockRuntimeRegistry)(nil)
)