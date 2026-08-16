package quota

import (
	"context"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

type fakeJobRepo struct {
	jobs []models.Job
	err  error
}

func (f *fakeJobRepo) GetJobs() ([]models.Job, error)        { return f.jobs, f.err }
func (f *fakeJobRepo) GetPendingJobs() ([]models.Job, error) { return nil, nil }

func (f *fakeJobRepo) GetActiveJobs() ([]models.Job, error) {
	if f.err != nil {
		return nil, f.err
	}
	var active []models.Job
	for _, j := range f.jobs {
		if !j.IsTerminal() {
			active = append(active, j)
		}
	}
	return active, nil
}

func (f *fakeJobRepo) GetResourcesByCluster(string) ([]models.Resource, error) { return nil, nil }
func (f *fakeJobRepo) CreateJob(*models.Job) error          { return nil }
func (f *fakeJobRepo) GetJob(string) (*models.Job, error)   { return nil, nil }
func (f *fakeJobRepo) GetJobsByTenant(string) ([]models.Job, error) { return f.jobs, f.err }
func (f *fakeJobRepo) UpdateJobSpec(*models.Job) error      { return nil }
func (f *fakeJobRepo) DeleteJob(string) error              { return nil }
func (f *fakeJobRepo) UpdateJob(*models.Job) error                             { return nil }
func (f *fakeJobRepo) UpdateJobStatus(*models.Job) error                       { return nil }
func (f *fakeJobRepo) UpdateResource(*models.Resource) error                   { return nil }

type fakeQuotaRepo struct {
	quotas []models.Quota
	upsert []models.Quota
	err    error
}

func (f *fakeQuotaRepo) GetQuota(string) (*models.Quota, error) { return nil, nil }
func (f *fakeQuotaRepo) ListQuotas() ([]models.Quota, error)    { return f.quotas, f.err }
func (f *fakeQuotaRepo) UpsertQuota(q *models.Quota) error {
	if f.err != nil {
		return f.err
	}
	f.upsert = append(f.upsert, *q)
	return nil
}
func (f *fakeQuotaRepo) CheckAndReserve(string, int, int, int) error { return nil }
func (f *fakeQuotaRepo) Release(string, int, int, int) error         { return nil }

func findUpsert(ups []models.Quota, tenant string) (models.Quota, bool) {
	for _, q := range ups {
		if q.TenantID == tenant {
			return q, true
		}
	}
	return models.Quota{}, false
}

func TestReconcileComputesUsedFromActiveJobs(t *testing.T) {
	jobs := []models.Job{
		{TenantID: "t1", Status: models.JobStatusRunning, GPUs: 4, Memory: 8},
		{TenantID: "t1", Status: models.JobStatusPending, GPUs: 2, Memory: 4},
		{TenantID: "t1", Status: models.JobStatusCompleted, GPUs: 8, Memory: 16}, 
		{TenantID: "t2", Status: models.JobStatusRunning, GPUs: 1, Memory: 2},
	}
	quotas := []models.Quota{
		{TenantID: "t1", GPUQuota: 100, MemoryQuotaGB: 200, JobQuota: 10},
		{TenantID: "t2", GPUQuota: 100, MemoryQuotaGB: 200, JobQuota: 10},
	}
	jobRepo := &fakeJobRepo{jobs: jobs}
	quotaRepo := &fakeQuotaRepo{quotas: quotas}

	r := NewReconciler(jobRepo, quotaRepo)
	r.Reconcile(context.Background())

	if q, ok := findUpsert(quotaRepo.upsert, "t1"); !ok {
		t.Fatal("t1 quota not reconciled")
	} else {
		if q.GPUUsed != 6 {
			t.Errorf("t1 GPUUsed = %d, want 6 (running 4 + pending 2)", q.GPUUsed)
		}
		if q.MemoryUsedGB != 12 {
			t.Errorf("t1 MemoryUsedGB = %d, want 12", q.MemoryUsedGB)
		}
		if q.JobUsed != 2 {
			t.Errorf("t1 JobUsed = %d, want 2", q.JobUsed)
		}
	}

	if q, ok := findUpsert(quotaRepo.upsert, "t2"); !ok {
		t.Fatal("t2 quota not reconciled")
	} else {
		if q.GPUUsed != 1 || q.MemoryUsedGB != 2 || q.JobUsed != 1 {
			t.Errorf("t2 used = gpu %d mem %d job %d, want 1/2/1", q.GPUUsed, q.MemoryUsedGB, q.JobUsed)
		}
	}
}

func TestReconcileCreatesMissingTenantEntry(t *testing.T) {
	jobs := []models.Job{
		{TenantID: "ghost", Status: models.JobStatusRunning, GPUs: 3, Memory: 5},
	}
	jobRepo := &fakeJobRepo{jobs: jobs}
	quotaRepo := &fakeQuotaRepo{quotas: nil} 

	r := NewReconciler(jobRepo, quotaRepo)
	r.Reconcile(context.Background())

	if q, ok := findUpsert(quotaRepo.upsert, "ghost"); !ok {
		t.Fatal("ghost tenant should be upserted from active jobs")
	} else {
		if q.GPUUsed != 3 || q.MemoryUsedGB != 5 || q.JobUsed != 1 {
			t.Errorf("ghost used = gpu %d mem %d job %d, want 3/5/1", q.GPUUsed, q.MemoryUsedGB, q.JobUsed)
		}
	}
}

func TestReconcilePropagatesListError(t *testing.T) {
	jobRepo := &fakeJobRepo{}
	quotaRepo := &fakeQuotaRepo{err: context.Canceled}

	r := NewReconciler(jobRepo, quotaRepo)
	r.Reconcile(context.Background()) 
	if len(quotaRepo.upsert) != 0 {
		t.Errorf("expected no upserts on list error, got %d", len(quotaRepo.upsert))
	}
}