package job

import (
	"context"
	"sync"
	"testing"

	"fuze-ai-paas/backend/internal/domain/gpu"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type concurrentRepo struct {
	mu   sync.Mutex
	job  *models.Job
	
}

func (r *concurrentRepo) getJob() *models.Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *r.job
	return &cp
}

func (r *concurrentRepo) GetJob(id string) (*models.Job, error) { return r.getJob(), nil }

func (r *concurrentRepo) UpdateJobStatus(job *models.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.job = job
	return nil
}

func (r *concurrentRepo) GetJobs() ([]models.Job, error) { return nil, nil }
func (r *concurrentRepo) GetPendingJobs() ([]models.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.job.Status == models.JobStatusPending && r.job.VolcanoJobName == "" {
		return []models.Job{*r.job}, nil
	}
	return nil, nil
}
func (r *concurrentRepo) GetActiveJobs() ([]models.Job, error)        { return nil, nil }
func (r *concurrentRepo) CreateJob(*models.Job) error                 { return nil }
func (r *concurrentRepo) GetResourcesByCluster(string) ([]models.Resource, error) {
	return nil, nil
}
func (r *concurrentRepo) GetJobsByTenant(string) ([]models.Job, error) { return nil, nil }
func (r *concurrentRepo) UpdateJobSpec(*models.Job) error              { return nil }
func (r *concurrentRepo) DeleteJob(string) error                       { return nil }
func (r *concurrentRepo) UpdateJob(*models.Job) error                  { return nil }
func (r *concurrentRepo) UpdateResource(*models.Resource) error        { return nil }

type concurrentClient struct {
	mu      sync.Mutex
	created int
}

func (c *concurrentClient) Enabled() bool                    { return true }
func (c *concurrentClient) Namespace() string                { return "test" }
func (c *concurrentClient) ServerVersion(context.Context) (string, error) {
	return "v1", nil
}
func (c *concurrentClient) DiscoverGPUInventory(context.Context) ([]gpu.GPUDevice, error) {
	return nil, nil
}

func (c *concurrentClient) CreateVolcanoJob(_ context.Context, job *models.Job) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.created++
	return "vj-" + job.ID, nil
}

func (c *concurrentClient) DeleteVolcanoJob(context.Context, string) error { return nil }
func (c *concurrentClient) GetVolcanoJobStatus(context.Context, string) (models.JobState, error) {
	return models.JobStatePending, nil
}
func (c *concurrentClient) SyncJobStatuses(context.Context) (map[string]models.JobState, error) {
	return map[string]models.JobState{}, nil
}
func (c *concurrentClient) CreateInferenceService(context.Context, *models.InferenceService) (string, error) {
	return "", nil
}
func (c *concurrentClient) DeleteInferenceService(context.Context, string) error { return nil }
func (c *concurrentClient) CreateDataset(context.Context, *models.Dataset) error { return nil }
func (c *concurrentClient) DeleteDataset(context.Context, *models.Dataset) error { return nil }
func (c *concurrentClient) GetJobLogs(context.Context, *models.Job, ports.LogQuery) (ports.JobLogs, error) {
	return ports.JobLogs{}, nil
}

type concurrentRegistry struct{ client *concurrentClient }

func (r *concurrentRegistry) Register(*models.Cluster) error { return nil }
func (r *concurrentRegistry) Unregister(string)              {}
func (r *concurrentRegistry) Get(string) (ports.ClusterClientPort, error) {
	return r.client, nil
}
func (r *concurrentRegistry) List() []string                   { return []string{"c1"} }
func (r *concurrentRegistry) LoadAll([]models.Cluster) []error { return nil }
func (r *concurrentRegistry) K8sClient(string) (ports.ClusterClientPort, error) {
	return nil, nil
}

func TestConcurrentSubmitCreatesSingleVolcanoJob(t *testing.T) {
	job := &models.Job{ID: "j1", ClusterID: "c1", Status: models.JobStatusPending}
	repo := &concurrentRepo{job: job}
	client := &concurrentClient{}
	svc := NewService(repo, &concurrentRegistry{client: client}, nil)

	const workers = 16
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			local := *job
			if err := svc.SubmitJob(&local); err != nil {
				t.Errorf("SubmitJob: %v", err)
			}
			svc.SubmitPending(context.Background())
		}()
	}
	wg.Wait()

	if client.created != 1 {
		t.Fatalf("并发提交应只创建一次 Volcano Job，实际创建 %d 次", client.created)
	}
	got := repo.getJob()
	if got.VolcanoJobName != "vj-j1" {
		t.Fatalf("任务应持久化 VolcanoJobName，实际 %q", got.VolcanoJobName)
	}
}