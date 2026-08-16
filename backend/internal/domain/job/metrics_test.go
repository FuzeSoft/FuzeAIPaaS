package job

import (
	"math"
	"testing"
)

func TestComputeMetrics(t *testing.T) {
	resources := []Resource{
		{Type: ResourceTypeGPU, TotalGPUs: 1, UsedGPUs: 0, TotalMemory: 80, AvailableMemory: 80, Status: ResourceStatusAvailable},
		{Type: ResourceTypeGPU, TotalGPUs: 1, UsedGPUs: 1, TotalMemory: 80, AvailableMemory: 0, Status: ResourceStatusAllocated},
		{Type: ResourceTypeCPU, TotalGPUs: 0, TotalMemory: 999, AvailableMemory: 999},
	}
	jobs := []Job{
		{ClusterID: "c1", Status: JobStatusRunning},
		{ClusterID: "c1", Status: JobStatusPending},
		{ClusterID: "c1", Status: JobStatusCompleted},
		{ClusterID: "other", Status: JobStatusRunning},
	}
	m := ComputeMetrics(resources, jobs, "c1")
	if m.TotalGPUs != 2 {
		t.Errorf("TotalGPUs 应为 2，实际 %d", m.TotalGPUs)
	}
	if m.UsedGPUs != 1 {
		t.Errorf("UsedGPUs 应为 1，实际 %d", m.UsedGPUs)
	}
	if m.AvailableGPUs != 1 {
		t.Errorf("AvailableGPUs 应为 1，实际 %d", m.AvailableGPUs)
	}
	if m.GPUUtilization != 50 {
		t.Errorf("GPUUtilization 应为 50，实际 %v", m.GPUUtilization)
	}
	if m.TotalJobs != 3 {
		t.Errorf("TotalJobs 应为 3，实际 %d", m.TotalJobs)
	}
	if m.RunningJobs != 1 || m.PendingJobs != 1 || m.CompletedJobs != 1 {
		t.Errorf("Running/Pending/Completed 应为 1/1/1，实际 %d/%d/%d", m.RunningJobs, m.PendingJobs, m.CompletedJobs)
	}
	if m.TotalMemory != 160 {
		t.Errorf("TotalMemory 应为 160（仅 GPU/NPU 计入），实际 %d", m.TotalMemory)
	}
	if m.UsedMemory != 80 {
		t.Errorf("UsedMemory 应为 80，实际 %d", m.UsedMemory)
	}
	if math.Abs(m.MemoryUtilization-50) > 0.01 {
		t.Errorf("MemoryUtilization 应为 50，实际 %v", m.MemoryUtilization)
	}
}

func TestComputeMetricsAllClusters(t *testing.T) {
	resources := []Resource{{Type: ResourceTypeGPU, TotalGPUs: 2, UsedGPUs: 1, TotalMemory: 100, AvailableMemory: 40, Status: ResourceStatusAllocated}}
	jobs := []Job{{ClusterID: "c1", Status: JobStatusRunning}}
	m := ComputeMetrics(resources, jobs, "c1")
	if m.TotalGPUs != 2 || m.UsedGPUs != 1 || m.AvailableGPUs != 1 {
		t.Fatalf("c1 指标错误: %+v", m)
	}
	if m.GPUUtilization != 50 {
		t.Errorf("GPUUtilization 应为 50，实际 %v", m.GPUUtilization)
	}
}