package job

import "testing"

func TestJobAggregateAllocate(t *testing.T) {
	agg := &Job{ID: "j1", ClusterID: "c1", Name: "t", Memory: 100, Status: JobStatusPending}
	if agg.Status != JobStatusPending {
		t.Fatalf("初始状态应为 pending，实际 %s", agg.Status)
	}
	resources := []Resource{
		{ID: "r1", Status: ResourceStatusAvailable, AvailableMemory: 80},
		{ID: "r2", Status: ResourceStatusAvailable, AvailableMemory: 80},
	}
	if !agg.CanSchedule(resources) {
		t.Fatal("聚合根应能判定可调度")
	}
	agg.MarkRunning()
	agg.Allocate(resources)
	if agg.Status != JobStatusRunning {
		t.Fatalf("聚合根状态应为 running，实际 %s", agg.Status)
	}
	if resources[0].AvailableMemory != 0 || resources[0].Status != ResourceStatusAllocated {
		t.Fatalf("r1 期望 0/allocated，实际 %d/%s", resources[0].AvailableMemory, resources[0].Status)
	}
}

func TestJobApplyVolcanoState(t *testing.T) {
	agg := &Job{Status: JobStatusPending}
	agg.ApplyVolcanoState(JobStateRunning)
	if agg.Status != JobStatusRunning {
		t.Fatalf("应用 running 后应为 running，实际 %s", agg.Status)
	}
	agg.ApplyVolcanoState(JobStateFailed)
	if agg.Status != JobStatusFailed {
		t.Fatalf("应用 failed 后应为 failed，实际 %s", agg.Status)
	}
}

func TestJobStatusTransitions(t *testing.T) {
	agg := &Job{}
	agg.MarkPending()
	if agg.Status != JobStatusPending {
		t.Fatal("空状态应归一化为 pending")
	}
	if !agg.MarkRunning() || agg.Status != JobStatusRunning {
		t.Fatal("pending -> running 应被允许")
	}
	if !agg.MarkCancelled() || agg.Status != JobStatusCancelled {
		t.Fatal("running -> cancelled 应被允许")
	}
	
	if agg.MarkRunning() || agg.Status != JobStatusCancelled {
		t.Fatalf("cancelled 为终态不应迁出，实际 %s", agg.Status)
	}
}

func TestJobTerminalStateIsFinal(t *testing.T) {
	for _, terminal := range []JobStatus{JobStatusCompleted, JobStatusFailed, JobStatusCancelled} {
		agg := &Job{Status: terminal}
		if !agg.IsTerminal() {
			t.Fatalf("%s 应被识别为终态", terminal)
		}
		if agg.ApplyVolcanoState(JobStateRunning) || agg.Status != terminal {
			t.Fatalf("%s 不应被 Volcano Running 覆盖，实际 %s", terminal, agg.Status)
		}
	}
}

func TestApplyVolcanoStateKeepsStatusWhenUndecidable(t *testing.T) {
	
	undecidable := []JobState{"", "Unknown", JobStateTerminating, JobStateAborting}
	for _, state := range undecidable {
		agg := &Job{Status: JobStatusRunning}
		if agg.ApplyVolcanoState(state) {
			t.Fatalf("phase=%q 不足以判定，不应变更状态", state)
		}
		if agg.Status != JobStatusRunning {
			t.Fatalf("phase=%q 后状态应保持 running，实际 %s", state, agg.Status)
		}
	}
}

func TestApplyVolcanoStateReportsChange(t *testing.T) {
	agg := &Job{Status: JobStatusPending}
	if !agg.ApplyVolcanoState(JobStateRunning) {
		t.Fatal("pending -> running 应报告状态已变更")
	}
	
	if agg.ApplyVolcanoState(JobStateRunning) {
		t.Fatal("重复上报同一状态不应报告变更")
	}
}