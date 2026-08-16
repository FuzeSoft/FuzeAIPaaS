package job

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/domain/gpu"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type fakeRepo struct {
	jobs map[string]*models.Job
	
	updateErr  error
	updateCall int
}

func newFakeRepo(jobs ...*models.Job) *fakeRepo {
	r := &fakeRepo{jobs: map[string]*models.Job{}}
	for _, j := range jobs {
		r.jobs[j.ID] = j
	}
	return r
}

func (r *fakeRepo) snapshot(status ...models.JobStatus) []models.Job {
	var out []models.Job
	for _, j := range r.jobs {
		if len(status) == 0 {
			out = append(out, *j)
			continue
		}
		for _, s := range status {
			if j.Status == s {
				out = append(out, *j)
			}
		}
	}
	return out
}

func (r *fakeRepo) GetJobs() ([]models.Job, error) { return r.snapshot(), nil }
func (r *fakeRepo) GetPendingJobs() ([]models.Job, error) {
	return r.snapshot(models.JobStatusPending), nil
}
func (r *fakeRepo) GetActiveJobs() ([]models.Job, error) {
	return r.snapshot(models.JobStatusPending, models.JobStatusRunning), nil
}

func (r *fakeRepo) UpdateJob(job *models.Job) error {
	r.updateCall++
	if r.updateErr != nil {
		return r.updateErr
	}
	if _, ok := r.jobs[job.ID]; !ok {
		return errors.New("record not found")
	}
	cp := *job
	cp.UpdatedAt = time.Now()
	r.jobs[job.ID] = &cp
	return nil
}

func (r *fakeRepo) UpdateJobStatus(job *models.Job) error { return r.UpdateJob(job) }

func (r *fakeRepo) GetResourcesByCluster(string) ([]models.Resource, error)        { return nil, nil }
func (r *fakeRepo) CreateJob(*models.Job) error                                   { return nil }
func (r *fakeRepo) GetJob(id string) (*models.Job, error)                         { return r.jobs[id], nil }
func (r *fakeRepo) GetJobsByTenant(string) ([]models.Job, error)                   { return r.snapshot(), nil }
func (r *fakeRepo) UpdateJobSpec(*models.Job) error                               { return nil }
func (r *fakeRepo) DeleteJob(string) error                                        { return nil }
func (r *fakeRepo) UpdateResource(*models.Resource) error                         { return nil }

type fakeClient struct {
	enabled      bool
	statuses     map[string]models.JobState
	createErr    error
	deleteErr    error
	created      []string
	deleted      []string
	lastLogQuery ports.LogQuery
}

func (c *fakeClient) Enabled() bool                                 { return c.enabled }
func (c *fakeClient) Namespace() string                             { return "test" }
func (c *fakeClient) ServerVersion(context.Context) (string, error) { return "v1.29", nil }
func (c *fakeClient) DiscoverGPUInventory(context.Context) ([]gpu.GPUDevice, error) {
	return nil, nil
}

func (c *fakeClient) CreateVolcanoJob(_ context.Context, job *models.Job) (string, error) {
	if c.createErr != nil {
		return "", c.createErr
	}
	name := "vj-" + job.ID
	c.created = append(c.created, name)
	return name, nil
}

func (c *fakeClient) DeleteVolcanoJob(_ context.Context, name string) error {
	if c.deleteErr != nil {
		return c.deleteErr
	}
	c.deleted = append(c.deleted, name)
	return nil
}

func (c *fakeClient) GetVolcanoJobStatus(context.Context, string) (models.JobState, error) {
	return models.JobStatePending, nil
}

func (c *fakeClient) SyncJobStatuses(context.Context) (map[string]models.JobState, error) {
	if c.statuses == nil {
		return map[string]models.JobState{}, nil
	}
	return c.statuses, nil
}

func (c *fakeClient) CreateInferenceService(context.Context, *models.InferenceService) (string, error) {
	return "", nil
}
func (c *fakeClient) DeleteInferenceService(context.Context, string) error { return nil }
func (c *fakeClient) CreateDataset(context.Context, *models.Dataset) error { return nil }
func (c *fakeClient) DeleteDataset(context.Context, *models.Dataset) error { return nil }
func (c *fakeClient) GetJobLogs(_ context.Context, job *models.Job, q ports.LogQuery) (ports.JobLogs, error) {
	c.lastLogQuery = q
	if q.Pod == "unknown-pod" {
		return ports.JobLogs{Pods: fakePods}, ports.ErrPodNotFound
	}
	logs := fmt.Sprintf("fake logs for job %s (tail=%d)", job.ID, q.TailLines)
	if q.Pod != "" {
		logs += " pod=" + q.Pod
	}
	if q.Task != "" {
		logs += " task=" + q.Task
	}
	return ports.JobLogs{Logs: logs, Pods: fakePods}, nil
}

var fakePods = []ports.PodRef{
	{Name: "vj-j1-master-0", Task: "master", Phase: "Running"},
	{Name: "vj-j1-worker-0", Task: "worker", Phase: "Running"},
}

type fakeRegistry struct{ client *fakeClient }

func (r *fakeRegistry) Register(*models.Cluster) error { return nil }
func (r *fakeRegistry) Unregister(string)              {}
func (r *fakeRegistry) Get(string) (ports.ClusterClientPort, error) {
	if r.client == nil {
		return nil, errors.New("cluster not registered")
	}
	return r.client, nil
}
func (r *fakeRegistry) List() []string                   { return []string{"cluster-001"} }
func (r *fakeRegistry) LoadAll([]models.Cluster) []error { return nil }
func (r *fakeRegistry) K8sClient(string) (ports.ClusterClientPort, error) {
	return nil, errors.New("no real client")
}

var (
	_ ports.JobRepository   = (*fakeRepo)(nil)
	_ ports.ClusterClientPort = (*fakeClient)(nil)
	_ ports.ClusterRegistry   = (*fakeRegistry)(nil)
)

func newTestService(repo *fakeRepo, client *fakeClient) *Service {
	return NewService(repo, &fakeRegistry{client: client}, nil)
}

func TestSubmitPendingSkipsAlreadySubmittedJob(t *testing.T) {
	repo := newFakeRepo(&models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusPending,
		VolcanoJobName: "vj-j1",
	})
	client := &fakeClient{enabled: true}
	newTestService(repo, client).SubmitPending(context.Background())

	if len(client.created) != 0 {
		t.Fatalf("已下发的任务不应重复创建 Volcano Job，实际创建 %v", client.created)
	}
}

func TestSubmitJobIsIdempotent(t *testing.T) {
	job := &models.Job{ID: "j1", ClusterID: "cluster-001", VolcanoJobName: "vj-j1"}
	client := &fakeClient{enabled: true}
	if err := newTestService(newFakeRepo(job), client).SubmitJob(job); err != nil {
		t.Fatalf("幂等提交不应报错: %v", err)
	}
	if len(client.created) != 0 {
		t.Fatalf("不应重复创建，实际 %v", client.created)
	}
}

func TestSubmitPendingRollsBackClusterJobWhenPersistFails(t *testing.T) {
	repo := newFakeRepo(&models.Job{ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusPending})
	repo.updateErr = errors.New("db down")
	client := &fakeClient{enabled: true}

	newTestService(repo, client).SubmitPending(context.Background())

	if len(client.created) != 1 {
		t.Fatalf("应尝试创建一次 Volcano Job，实际 %v", client.created)
	}
	if len(client.deleted) != 1 || client.deleted[0] != "vj-j1" {
		t.Fatalf("落库失败后应回滚删除集群侧对象，实际删除 %v", client.deleted)
	}
}

func TestSubmitPendingRecordsFailureAndTripsBreaker(t *testing.T) {
	repo := newFakeRepo(&models.Job{ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusPending})
	client := &fakeClient{enabled: true, createErr: errors.New("image not found")}
	svc := newTestService(repo, client)

	for i := 0; i < maxSubmitAttempts; i++ {
		
		repo.jobs["j1"].LastSubmitAt = time.Now().Add(-time.Hour)
		svc.SubmitPending(context.Background())
	}

	got := repo.jobs["j1"]
	if got.SubmitAttempts != maxSubmitAttempts {
		t.Errorf("尝试次数应为 %d，实际 %d", maxSubmitAttempts, got.SubmitAttempts)
	}
	if got.Status != models.JobStatusFailed {
		t.Errorf("超过重试上限应熔断为 failed，实际 %s", got.Status)
	}
	if got.FailureReason == "" {
		t.Error("失败原因必须落库，否则用户只能看到一个长期 pending 的任务")
	}
}

func TestSubmitPendingRespectsBackoff(t *testing.T) {
	repo := newFakeRepo(&models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusPending,
		SubmitAttempts: 1, LastSubmitAt: time.Now(),
	})
	client := &fakeClient{enabled: true, createErr: errors.New("still broken")}
	newTestService(repo, client).SubmitPending(context.Background())

	if repo.jobs["j1"].SubmitAttempts != 1 {
		t.Fatalf("退避窗口内不应再次尝试，实际尝试次数 %d", repo.jobs["j1"].SubmitAttempts)
	}
}

func TestSyncKeepsRunningJobOnEmptyPhase(t *testing.T) {
	repo := newFakeRepo(&models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusRunning, VolcanoJobName: "vj-j1",
	})
	client := &fakeClient{enabled: true, statuses: map[string]models.JobState{"vj-j1": ""}}

	newTestService(repo, client).SyncVolcanoStatuses(context.Background())

	if repo.jobs["j1"].Status != models.JobStatusRunning {
		t.Fatalf("空 phase 不足以判定，状态应保持 running，实际 %s", repo.jobs["j1"].Status)
	}
	if repo.updateCall != 0 {
		t.Errorf("状态未变化时不应写库，实际写入 %d 次", repo.updateCall)
	}
}

func TestSyncAppliesTerminalState(t *testing.T) {
	repo := newFakeRepo(&models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusRunning, VolcanoJobName: "vj-j1",
	})
	client := &fakeClient{enabled: true, statuses: map[string]models.JobState{"vj-j1": models.JobStateFailed}}

	newTestService(repo, client).SyncVolcanoStatuses(context.Background())

	got := repo.jobs["j1"]
	if got.Status != models.JobStatusFailed {
		t.Fatalf("期望 failed，实际 %s", got.Status)
	}
	if got.FailureReason == "" {
		t.Error("失败任务应记录原因")
	}
}

func TestSyncIgnoresTerminalJobs(t *testing.T) {
	repo := newFakeRepo(&models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusCompleted, VolcanoJobName: "vj-j1",
	})
	client := &fakeClient{enabled: true, statuses: map[string]models.JobState{"vj-j1": models.JobStateRunning}}

	newTestService(repo, client).SyncVolcanoStatuses(context.Background())

	if repo.jobs["j1"].Status != models.JobStatusCompleted {
		t.Fatalf("终态任务不应被重新拉活，实际 %s", repo.jobs["j1"].Status)
	}
}

func TestSyncReclaimsOrphanedJobAfterThreshold(t *testing.T) {
	repo := newFakeRepo(&models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusRunning, VolcanoJobName: "vj-j1",
	})
	client := &fakeClient{enabled: true, statuses: map[string]models.JobState{}}
	svc := newTestService(repo, client)

	for i := 1; i < orphanConfirmThreshold; i++ {
		svc.SyncVolcanoStatuses(context.Background())
		if repo.jobs["j1"].Status != models.JobStatusRunning {
			t.Fatalf("第 %d 次缺失不应判死（可能只是列表延迟）", i)
		}
	}

	svc.SyncVolcanoStatuses(context.Background())
	if repo.jobs["j1"].Status != models.JobStatusFailed {
		t.Fatalf("连续缺失 %d 次后应回收，实际 %s", orphanConfirmThreshold, repo.jobs["j1"].Status)
	}
}

func TestSyncResetsMissingCountWhenJobReappears(t *testing.T) {
	repo := newFakeRepo(&models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusRunning, VolcanoJobName: "vj-j1",
	})
	client := &fakeClient{enabled: true, statuses: map[string]models.JobState{}}
	svc := newTestService(repo, client)

	for i := 1; i < orphanConfirmThreshold; i++ {
		svc.SyncVolcanoStatuses(context.Background())
	}
	client.statuses = map[string]models.JobState{"vj-j1": models.JobStateRunning}
	svc.SyncVolcanoStatuses(context.Background())

	client.statuses = map[string]models.JobState{}
	svc.SyncVolcanoStatuses(context.Background())
	if repo.jobs["j1"].Status != models.JobStatusRunning {
		t.Fatalf("计数归零后单次缺失不应判死，实际 %s", repo.jobs["j1"].Status)
	}
}

func TestCancelJobKeepsStateWhenClusterDeletionFails(t *testing.T) {
	job := &models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusRunning, VolcanoJobName: "vj-j1",
	}
	client := &fakeClient{enabled: true, deleteErr: errors.New("apiserver unreachable")}
	repo := newFakeRepo(job)

	if err := newTestService(repo, client).CancelJob(job); err == nil {
		t.Fatal("集群侧清理失败时 CancelJob 必须返回错误")
	}
	if job.Status != models.JobStatusRunning {
		t.Errorf("清理失败时不应改写状态，实际 %s", job.Status)
	}
	if job.VolcanoJobName != "vj-j1" {
		t.Error("清理失败时必须保留 VolcanoJobName，否则将永久失去对该对象的追踪")
	}
	if repo.updateCall != 0 {
		t.Errorf("清理失败时不应落库，实际写入 %d 次", repo.updateCall)
	}
}

func TestCancelJobSucceeds(t *testing.T) {
	job := &models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusRunning, VolcanoJobName: "vj-j1",
	}
	client := &fakeClient{enabled: true}
	repo := newFakeRepo(job)

	if err := newTestService(repo, client).CancelJob(job); err != nil {
		t.Fatalf("取消不应失败: %v", err)
	}
	if repo.jobs["j1"].Status != models.JobStatusCancelled {
		t.Errorf("期望 cancelled，实际 %s", repo.jobs["j1"].Status)
	}
	if len(client.deleted) != 1 {
		t.Errorf("应删除集群侧对象，实际 %v", client.deleted)
	}
}

func TestTerminateJobDoesNotWriteRepository(t *testing.T) {
	job := &models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusRunning, VolcanoJobName: "vj-j1",
	}
	client := &fakeClient{enabled: true}
	repo := newFakeRepo(job)

	if err := newTestService(repo, client).TerminateJob(job); err != nil {
		t.Fatalf("终止不应失败: %v", err)
	}
	if repo.updateCall != 0 {
		t.Errorf("TerminateJob 不应写库，实际写入 %d 次", repo.updateCall)
	}
	if len(client.deleted) != 1 {
		t.Errorf("应删除集群侧对象，实际 %v", client.deleted)
	}
}

func TestGetJobLogsUnavailableWhenNotSubmitted(t *testing.T) {
	job := &models.Job{ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusPending}
	res, available, err := newTestService(newFakeRepo(job), &fakeClient{enabled: false}).
		GetJobLogs(job, ports.LogQuery{TailLines: 100})
	if err != nil {
		t.Fatalf("未提交任务不应返回错误: %v", err)
	}
	if available {
		t.Fatal("未提交任务应标记 available=false")
	}
	if res.Logs != "" {
		t.Fatalf("未提交任务不应返回日志，实际 %q", res.Logs)
	}
}

func TestGetJobLogsAvailableFromCluster(t *testing.T) {
	job := &models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusRunning, VolcanoJobName: "vj-j1",
	}
	client := &fakeClient{enabled: true}
	res, available, err := newTestService(newFakeRepo(job), client).GetJobLogs(job, ports.LogQuery{TailLines: 50})
	if err != nil {
		t.Fatalf("拉取日志不应失败: %v", err)
	}
	if !available {
		t.Fatal("已提交且集群连通的任务应 available=true")
	}
	if res.Logs == "" || !contains(res.Logs, "j1") {
		t.Fatalf("应返回集群日志，实际 %q", res.Logs)
	}
	if len(res.Pods) != 2 {
		t.Fatalf("应回传任务下全部副本供前端下钻，实际 %+v", res.Pods)
	}
}

func TestGetJobLogsPassesDrillDownQuery(t *testing.T) {
	job := &models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusRunning, VolcanoJobName: "vj-j1",
	}
	client := &fakeClient{enabled: true}
	query := ports.LogQuery{Pod: "vj-j1-worker-0", Task: "worker", TailLines: 30}
	res, _, err := newTestService(newFakeRepo(job), client).GetJobLogs(job, query)
	if err != nil {
		t.Fatalf("下钻拉取不应失败: %v", err)
	}
	if client.lastLogQuery != query {
		t.Fatalf("下钻条件应原样透传，实际 %+v", client.lastLogQuery)
	}
	if !contains(res.Logs, "pod=vj-j1-worker-0") || !contains(res.Logs, "task=worker") {
		t.Fatalf("应返回指定副本的日志，实际 %q", res.Logs)
	}
}

func TestGetJobLogsRejectsForeignPod(t *testing.T) {
	job := &models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusRunning, VolcanoJobName: "vj-j1",
	}
	res, available, err := newTestService(newFakeRepo(job), &fakeClient{enabled: true}).
		GetJobLogs(job, ports.LogQuery{Pod: "unknown-pod"})
	if !errors.Is(err, ports.ErrPodNotFound) {
		t.Fatalf("越界副本应返回 ErrPodNotFound，实际 %v", err)
	}
	if !available {
		t.Fatal("任务本身可查，available 应为 true")
	}
	if len(res.Pods) == 0 {
		t.Fatal("出错时仍应回传副本列表，便于前端提示可选副本")
	}
}

func TestEnforceRuntimeLimitsExpiresOverdueJob(t *testing.T) {
	started := time.Now().Add(-2 * time.Minute)
	job := &models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusRunning,
		VolcanoJobName: "vj-j1", MaxRuntime: 1, StartedAt: &started,
	}
	client := &fakeClient{enabled: true}
	repo := newFakeRepo(job)
	newTestService(repo, client).enforceRuntimeLimits()

	got := repo.jobs["j1"]
	if got.Status != models.JobStatusFailed {
		t.Fatalf("超时任务应被标记为 failed，实际 %s", got.Status)
	}
	if got.VolcanoJobName != "" {
		t.Error("超时任务应清除 VolcanoJobName，避免后续误命中幂等闸门")
	}
	if !contains(got.FailureReason, "max runtime") {
		t.Fatalf("失败原因应说明超时，实际 %q", got.FailureReason)
	}
	if len(client.deleted) != 1 || client.deleted[0] != "vj-j1" {
		t.Fatalf("应清理集群侧负载，实际 %v", client.deleted)
	}
}

func TestEnforceRuntimeLimitsIgnoresUnlimitedJob(t *testing.T) {
	started := time.Now().Add(-72 * time.Hour) 
	job := &models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusRunning,
		VolcanoJobName: "vj-j1", MaxRuntime: 0, StartedAt: &started,
	}
	client := &fakeClient{enabled: true}
	repo := newFakeRepo(job)
	newTestService(repo, client).enforceRuntimeLimits()

	if repo.jobs["j1"].Status != models.JobStatusRunning {
		t.Fatalf("不限时任务不应被超时熔断，实际 %s", repo.jobs["j1"].Status)
	}
	if len(client.deleted) != 0 {
		t.Fatalf("不应清理集群侧负载，实际 %v", client.deleted)
	}
}

func TestEnforceRuntimeLimitsSkipsJobWithoutStartTime(t *testing.T) {
	job := &models.Job{
		ID: "j1", ClusterID: "cluster-001", Status: models.JobStatusRunning,
		VolcanoJobName: "vj-j1", MaxRuntime: 1, 
	}
	client := &fakeClient{enabled: true}
	repo := newFakeRepo(job)
	newTestService(repo, client).enforceRuntimeLimits()

	if repo.jobs["j1"].Status != models.JobStatusRunning {
		t.Fatalf("无起始时刻的任务不应被超时熔断，实际 %s", repo.jobs["j1"].Status)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestBackoffAnchoredOnLastSubmitAttempt(t *testing.T) {
	svc := &Service{}

	t.Run("无关写库刷新 UpdatedAt 不应重置退避窗口", func(t *testing.T) {
		job := &models.Job{
			SubmitAttempts: 1,
			
			LastSubmitAt: time.Now().Add(-time.Minute),
			
			UpdatedAt: time.Now(),
		}
		if !svc.backoffElapsed(job) {
			t.Error("退避窗口应已到期：基准必须是 LastSubmitAt，不能被 UpdatedAt 重置")
		}
	})

	t.Run("窗口未到期仍需继续等待", func(t *testing.T) {
		job := &models.Job{
			SubmitAttempts: 1,
			LastSubmitAt:   time.Now(),
			
			UpdatedAt: time.Now().Add(-time.Hour),
		}
		if svc.backoffElapsed(job) {
			t.Error("退避窗口未到期，不应放行")
		}
	})

	t.Run("首次提交不退避", func(t *testing.T) {
		if !svc.backoffElapsed(&models.Job{}) {
			t.Error("从未尝试过的任务应立即放行")
		}
	})

	t.Run("历史数据 LastSubmitAt 为零值时回退到 UpdatedAt", func(t *testing.T) {
		stale := &models.Job{SubmitAttempts: 1, UpdatedAt: time.Now().Add(-time.Hour)}
		if !svc.backoffElapsed(stale) {
			t.Error("零值应回退到 UpdatedAt 并判定为已到期")
		}
		fresh := &models.Job{SubmitAttempts: 1, UpdatedAt: time.Now()}
		if svc.backoffElapsed(fresh) {
			t.Error("零值回退后，UpdatedAt 很新时不应放行")
		}
	})

	t.Run("退避随尝试次数指数增长并封顶", func(t *testing.T) {
		
		if svc.backoffElapsed(&models.Job{
			SubmitAttempts: 4, LastSubmitAt: time.Now().Add(-30 * time.Second),
		}) {
			t.Error("第 4 次尝试的退避窗口为 40s，30s 时不应放行")
		}
		
		if !svc.backoffElapsed(&models.Job{
			SubmitAttempts: 64, LastSubmitAt: time.Now().Add(-submitBackoffMax - time.Second),
		}) {
			t.Error("超过上限的退避应封顶在 submitBackoffMax 并放行")
		}
	})
}

func TestRecordSubmitFailureStampsLastSubmitAt(t *testing.T) {
	job := &models.Job{ID: "j1"}
	svc := &Service{jobRepo: newFakeRepo(job)}

	before := time.Now()
	svc.recordSubmitFailure(job, errors.New("cluster unreachable"))

	if job.LastSubmitAt.Before(before) {
		t.Errorf("LastSubmitAt 应被刷新为当前时间，实际 %v", job.LastSubmitAt)
	}
	if job.SubmitAttempts != 1 {
		t.Errorf("SubmitAttempts = %d, want 1", job.SubmitAttempts)
	}
}