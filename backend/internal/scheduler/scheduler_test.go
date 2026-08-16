package scheduler

import (
	"context"
	"testing"

	"fuze-ai-paas/backend/internal/domain/gpu"
	"fuze-ai-paas/backend/internal/models"
)

func TestMockAllocate(t *testing.T) {
	jobRepo := &mockJobRepo{
		resourcesByCl: map[string][]models.Resource{
			"c1": {
				{ID: "r1", Status: models.ResourceStatusAvailable, AvailableMemory: 80},
				{ID: "r2", Status: models.ResourceStatusAvailable, AvailableMemory: 80},
			},
		},
	}
	s := NewScheduler(Repos{Cluster: newMockClusterRepo(), Job: jobRepo, Inference: jobRepo, Dataset: jobRepo, Metrics: jobRepo}, newMockRegistry(), newMockRuntimeRegistry(), nil)

	job := &models.Job{ID: "j1", ClusterID: "c1", Name: "test", Memory: 100, Status: models.JobStatusPending}
	s.mockAllocate(job)

	if job.Status != models.JobStatusRunning {
		t.Fatalf("期望任务 running，实际 %s", job.Status)
	}
	
	if len(jobRepo.updatedResources) != 2 {
		t.Fatalf("期望回写 2 条资源，实际 %d", len(jobRepo.updatedResources))
	}
	got := map[string]models.Resource{}
	for _, r := range jobRepo.updatedResources {
		got[r.ID] = r
	}
	
	if got["r1"].AvailableMemory != 0 || got["r1"].Status != models.ResourceStatusAllocated {
		t.Fatalf("r1 期望 0/allocated，实际 %d/%s", got["r1"].AvailableMemory, got["r1"].Status)
	}
	
	if got["r2"].AvailableMemory != 60 || got["r2"].Status != models.ResourceStatusAvailable {
		t.Fatalf("r2 期望 60/available，实际 %d/%s", got["r2"].AvailableMemory, got["r2"].Status)
	}
}

func TestMockAllocateInsufficient(t *testing.T) {
	jobRepo := &mockJobRepo{
		resourcesByCl: map[string][]models.Resource{
			"c1": {
				{ID: "r1", Status: models.ResourceStatusAvailable, AvailableMemory: 40},
			},
		},
	}
	s := NewScheduler(Repos{Cluster: newMockClusterRepo(), Job: jobRepo, Inference: jobRepo, Dataset: jobRepo, Metrics: jobRepo}, newMockRegistry(), newMockRuntimeRegistry(), nil)

	job := &models.Job{ID: "j1", ClusterID: "c1", Memory: 100, Status: models.JobStatusPending}
	s.mockAllocate(job)

	if job.Status != models.JobStatusPending {
		t.Fatalf("资源不足时任务应仍为 pending，实际 %s", job.Status)
	}
	if len(jobRepo.updatedResources) != 0 {
		t.Fatalf("资源不足时不应当回写资源，实际 %d", len(jobRepo.updatedResources))
	}
}

func TestGetMetrics(t *testing.T) {
	jobRepo := &mockJobRepo{
		allResources: []models.Resource{
			{Type: models.ResourceTypeGPU, TotalGPUs: 1, UsedGPUs: 0, TotalMemory: 80, AvailableMemory: 80, Status: models.ResourceStatusAvailable},
			{Type: models.ResourceTypeGPU, TotalGPUs: 1, UsedGPUs: 1, TotalMemory: 80, AvailableMemory: 0, Status: models.ResourceStatusAllocated},
			{Type: models.ResourceTypeCPU, TotalGPUs: 0, TotalMemory: 999, AvailableMemory: 999}, 
		},
		jobs: []models.Job{
			{Status: models.JobStatusRunning},
			{Status: models.JobStatusPending},
			{Status: models.JobStatusCompleted},
		},
	}
	s := NewScheduler(Repos{Cluster: newMockClusterRepo(), Job: jobRepo, Inference: jobRepo, Dataset: jobRepo, Metrics: jobRepo}, newMockRegistry(), newMockRuntimeRegistry(), nil)

	m, err := s.GetMetrics("")
	if err != nil {
		t.Fatal(err)
	}
	if m.TotalGPUs != 2 || m.UsedGPUs != 1 || m.AvailableGPUs != 1 {
		t.Fatalf("GPU 汇总错误: total=%d used=%d avail=%d", m.TotalGPUs, m.UsedGPUs, m.AvailableGPUs)
	}
	if m.GPUUtilization != 50.0 {
		t.Fatalf("GPU 利用率期望 50.0，实际 %v", m.GPUUtilization)
	}
	if m.TotalMemory != 160 || m.UsedMemory != 80 {
		t.Fatalf("显存汇总错误: total=%d used=%d", m.TotalMemory, m.UsedMemory)
	}
	if m.TotalJobs != 3 || m.RunningJobs != 1 || m.PendingJobs != 1 || m.CompletedJobs != 1 {
		t.Fatalf("任务计数错误: %+v", m)
	}
}

func TestSyncVolcanoStatuses(t *testing.T) {
	client := &mockClusterClient{
		enabled: true,
		statuses: map[string]models.JobState{
			"vj-1": models.JobStateRunning,
			"vj-2": models.JobStateCompleted,
		},
	}
	jobRepo := &mockJobRepo{
		jobs: []models.Job{
			{ID: "j1", ClusterID: "c1", VolcanoJobName: "vj-1", Status: models.JobStatusPending},    
			{ID: "j2", ClusterID: "c1", VolcanoJobName: "vj-2", Status: models.JobStatusRunning},    
			{ID: "j3", ClusterID: "c1", VolcanoJobName: "", Status: models.JobStatusPending},        
			{ID: "j4", ClusterID: "other", VolcanoJobName: "vj-1", Status: models.JobStatusPending}, 
		},
	}
	reg := newMockRegistry()
	reg.clients["c1"] = client
	s := NewScheduler(Repos{Cluster: newMockClusterRepo(), Job: jobRepo, Inference: jobRepo, Dataset: jobRepo, Metrics: jobRepo}, reg, newMockRuntimeRegistry(), nil)

	s.syncVolcanoStatuses(context.Background())

	if len(jobRepo.updatedJobs) != 2 {
		t.Fatalf("期望更新 2 个任务，实际 %d", len(jobRepo.updatedJobs))
	}
	got := map[string]models.JobStatus{}
	for _, j := range jobRepo.updatedJobs {
		got[j.ID] = j.Status
	}
	if got["j1"] != models.JobStatusRunning {
		t.Fatalf("j1 期望 running，实际 %s", got["j1"])
	}
	if got["j2"] != models.JobStatusCompleted {
		t.Fatalf("j2 期望 completed，实际 %s", got["j2"])
	}
}

func TestExpandNodesToResources(t *testing.T) {
	devices := []gpu.GPUDevice{
		gpu.NewGPUDevice("node-a", "NVIDIA", "A800 80GB", 4, 1),
		gpu.NewGPUDevice("node-b", "华为", "Ascend 910", 2, 0),
	}
	resources := expandNodesToResources("c1", devices)
	if len(resources) != 6 {
		t.Fatalf("期望展开 6 张卡，实际 %d", len(resources))
	}

	allocatedA := 0
	for _, r := range resources {
		switch r.NodeName {
		case "node-a":
			if r.Type != models.ResourceTypeGPU {
				t.Fatalf("node-a 期望 GPU 类型，实际 %s", r.Type)
			}
			if r.TotalMemory != 80 {
				t.Fatalf("A800 单卡显存期望 80，实际 %d", r.TotalMemory)
			}
			if r.Status == models.ResourceStatusAllocated {
				allocatedA++
			}
		case "node-b":
			if r.Type != models.ResourceTypeNPU {
				t.Fatalf("Ascend 期望 NPU 类型，实际 %s", r.Type)
			}
			if r.TotalMemory != 32 {
				t.Fatalf("Ascend 910 单卡显存期望 32，实际 %d", r.TotalMemory)
			}
		}
	}
	if allocatedA != 1 {
		t.Fatalf("node-a 期望 1 张已分配卡，实际 %d", allocatedA)
	}
}

func TestRefreshCluster(t *testing.T) {
	client := &mockClusterClient{
		enabled: true,
		version: "v1.28.0",
		devices: []gpu.GPUDevice{
			gpu.NewGPUDevice("node-a", "NVIDIA", "A800 80GB", 4, 2),
		},
	}
	reg := newMockRegistry()
	reg.clients["c1"] = client

	clusterRepo := newMockClusterRepo()
	clusterRepo.clusters["c1"] = &models.Cluster{ID: "c1"}

	s := NewScheduler(Repos{Cluster: clusterRepo, Job: &mockJobRepo{}, Inference: &mockJobRepo{}, Dataset: &mockJobRepo{}, Metrics: &mockJobRepo{}}, reg, newMockRuntimeRegistry(), nil)

	if err := s.RefreshCluster(context.Background(), "c1"); err != nil {
		t.Fatalf("RefreshCluster 失败: %v", err)
	}
	if got := len(clusterRepo.upserts["c1"]); got != 4 {
		t.Fatalf("期望 upsert 4 张卡，实际 %d", got)
	}
	stats := clusterRepo.stats["c1"]
	if stats.Status != models.ClusterStatusHealthy {
		t.Fatalf("期望集群状态 healthy，实际 %s", stats.Status)
	}
	if stats.NodeCount != 1 || stats.TotalGPUs != 4 || stats.UsedGPUs != 2 {
		t.Fatalf("集群统计错误: node=%d total=%d used=%d", stats.NodeCount, stats.TotalGPUs, stats.UsedGPUs)
	}
	if stats.Version != "v1.28.0" {
		t.Fatalf("期望版本 v1.28.0，实际 %s", stats.Version)
	}
}

func TestReconcileInferenceConvergesDesiredState(t *testing.T) {
	jobRepo := &mockJobRepo{
		infServices: map[string]*models.InferenceService{
			"svc1": {
				ID: "svc1", Name: "demo", ClusterID: "c1",
				Framework: models.FrameworkPyTorch, MinReplicas: 1, MaxReplicas: 5,
				TargetReplicas: 5, CanaryWeight: 30,
				Status: models.InferenceStatusPending,
			},
		},
	}
	s := NewScheduler(Repos{Cluster: newMockClusterRepo(), Job: jobRepo, Inference: jobRepo, Dataset: jobRepo, Metrics: jobRepo}, newMockRegistry(), newMockRuntimeRegistry(), nil) 

	s.ReconcileInference(context.Background())

	got := jobRepo.infServices["svc1"]
	if got.KServeName == "" || got.URL == "" {
		t.Fatalf("期望 reconcile 补齐部署并写入观测态，实际 %+v", got)
	}
	if got.Status != models.InferenceStatusReady {
		t.Fatalf("期望状态 ready，实际 %s", got.Status)
	}
	if got.ReadyReplicas != 5 {
		t.Fatalf("期望观测副本收敛到 5，实际 %d", got.ReadyReplicas)
	}
	if got.TargetReplicas != 5 || got.CanaryWeight != 30 {
		t.Fatalf("reconcile 不得篡改期望态: %+v", got)
	}
}

func TestReconcileInferenceIsIdempotent(t *testing.T) {
	jobRepo := &mockJobRepo{
		infServices: map[string]*models.InferenceService{
			"svc1": {
				ID: "svc1", Name: "demo", ClusterID: "c1",
				Framework:  models.FrameworkPyTorch,
				KServeName: "demo", URL: "http://demo",
				MinReplicas: 1, MaxReplicas: 5,
				TargetReplicas: 2, ReadyReplicas: 2,
				Status: models.InferenceStatusReady,
			},
		},
	}
	s := NewScheduler(Repos{Cluster: newMockClusterRepo(), Job: jobRepo, Inference: jobRepo, Dataset: jobRepo, Metrics: jobRepo}, newMockRegistry(), newMockRuntimeRegistry(), nil)

	s.ReconcileInference(context.Background())
	s.ReconcileInference(context.Background())

	if len(jobRepo.updatedInf) != 0 {
		t.Fatalf("已对齐状态不应触发回写，实际 %d 次", len(jobRepo.updatedInf))
	}
	if got := jobRepo.infServices["svc1"]; got.Status != models.InferenceStatusReady || got.ReadyReplicas != 2 {
		t.Fatalf("幂等性被破坏: status=%s ready=%d", got.Status, got.ReadyReplicas)
	}
}