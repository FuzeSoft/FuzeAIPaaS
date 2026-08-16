package data

import (
	"fmt"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

type fakeDataRepo struct {
	pipelines map[string]*models.DataPipeline
	steps     map[string]*models.PipelineStep
	runs      map[string]*models.PipelineStepRun
	anns      map[string]*models.AnnotationTask
}

func newFakeDataRepo() *fakeDataRepo {
	return &fakeDataRepo{
		pipelines: map[string]*models.DataPipeline{},
		steps:     map[string]*models.PipelineStep{},
		runs:      map[string]*models.PipelineStepRun{},
		anns:      map[string]*models.AnnotationTask{},
	}
}

func (f *fakeDataRepo) CreatePipeline(p *models.DataPipeline) error {
	f.pipelines[p.ID] = p
	return nil
}
func (f *fakeDataRepo) GetPipeline(id string) (*models.DataPipeline, error) {
	return f.pipelines[id], nil
}
func (f *fakeDataRepo) ListPipelines(string) ([]models.DataPipeline, error) {
	var out []models.DataPipeline
	for _, p := range f.pipelines {
		out = append(out, *p)
	}
	return out, nil
}
func (f *fakeDataRepo) ListActivePipelines() ([]models.DataPipeline, error) {
	var out []models.DataPipeline
	for _, p := range f.pipelines {
		if !p.Status.IsTerminal() {
			out = append(out, *p)
		}
	}
	return out, nil
}
func (f *fakeDataRepo) UpdatePipeline(p *models.DataPipeline) error {
	f.pipelines[p.ID] = p
	return nil
}
func (f *fakeDataRepo) CreateStep(s *models.PipelineStep) error { f.steps[s.ID] = s; return nil }
func (f *fakeDataRepo) GetSteps(pid string) ([]models.PipelineStep, error) {
	var out []models.PipelineStep
	for _, s := range f.steps {
		if s.PipelineID == pid {
			out = append(out, *s)
		}
	}
	return out, nil
}
func (f *fakeDataRepo) UpdateStep(s *models.PipelineStep) error { f.steps[s.ID] = s; return nil }
func (f *fakeDataRepo) CreateStepRun(r *models.PipelineStepRun) error {
	f.runs[r.ID] = r
	return nil
}
func (f *fakeDataRepo) UpdateStepRun(r *models.PipelineStepRun) error        { f.runs[r.ID] = r; return nil }
func (f *fakeDataRepo) GetStepRuns(string) ([]models.PipelineStepRun, error) { return nil, nil }
func (f *fakeDataRepo) CreateAnnotation(a *models.AnnotationTask) error      { f.anns[a.ID] = a; return nil }
func (f *fakeDataRepo) GetAnnotation(id string) (*models.AnnotationTask, error) {
	return f.anns[id], nil
}
func (f *fakeDataRepo) ListAnnotations(string) ([]models.AnnotationTask, error) {
	var out []models.AnnotationTask
	for _, a := range f.anns {
		out = append(out, *a)
	}
	return out, nil
}
func (f *fakeDataRepo) UpdateAnnotation(a *models.AnnotationTask) error { f.anns[a.ID] = a; return nil }

type fakeJobRepo struct {
	jobs   map[string]*models.Job
	jobSeq int
}

func newFakeJobRepo() *fakeJobRepo { return &fakeJobRepo{jobs: map[string]*models.Job{}} }

func (f *fakeJobRepo) GetJobs() ([]models.Job, error)               { return nil, nil }
func (f *fakeJobRepo) GetJobsByTenant(string) ([]models.Job, error) { return nil, nil }
func (f *fakeJobRepo) UpdateJob(*models.Job) error                  { return nil }
func (f *fakeJobRepo) UpdateJobSpec(*models.Job) error              { return nil }
func (f *fakeJobRepo) UpdateJobStatus(*models.Job) error            { return nil }
func (f *fakeJobRepo) DeleteJob(string) error                       { return nil }
func (f *fakeJobRepo) GetJob(id string) (*models.Job, error)        { return f.jobs[id], nil }
func (f *fakeJobRepo) CreateJob(j *models.Job) error {
	f.jobSeq++
	j.ID = fmt.Sprintf("job-%d", f.jobSeq)
	f.jobs[j.ID] = j
	return nil
}

func fixturePipeline() (*models.DataPipeline, []models.PipelineStep) {
	p := &models.DataPipeline{
		ID: "pipe-1", TenantID: "t1", Name: "clean-etl",
		DatasetName: "ds-a", MountPath: "/mnt/data", Status: models.PipelineStatusDraft,
	}
	steps := []models.PipelineStep{
		{ID: "s1", PipelineID: "pipe-1", Name: "dedup", Kind: models.StepKindClean, Operator: "dedup", DependsOn: `[]`, InputPath: "raw", OutputPath: "clean", Params: `{"method":"exact"}`},
		{ID: "s2", PipelineID: "pipe-1", Name: "aug", Kind: models.StepKindAugment, Operator: "img_flip", DependsOn: `["s1"]`, InputPath: "clean", OutputPath: "aug"},
		{ID: "s3", PipelineID: "pipe-1", Name: "export", Kind: models.StepKindETL, Operator: "format_convert", DependsOn: `["s2"]`, InputPath: "aug", OutputPath: "out", Params: `{"from":"jsonl","to":"csv"}`},
	}
	return p, steps
}

func TestCreatePipelineRejectsCycle(t *testing.T) {
	dr := newFakeDataRepo()
	jr := newFakeJobRepo()
	svc := NewService(dr, jr, nil)
	p, steps := fixturePipeline()
	steps[0].DependsOn = `["s3"]` 
	if err := svc.CreatePipeline(p, steps); err == nil {
		t.Fatal("expected cycle rejection")
	}
}

func TestSubmitDerivesJobsInOrder(t *testing.T) {
	dr := newFakeDataRepo()
	jr := newFakeJobRepo()
	svc := NewService(dr, jr, nil)
	p, steps := fixturePipeline()
	if err := svc.CreatePipeline(p, steps); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.SubmitPipeline("pipe-1"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	
	got, _ := dr.GetSteps("pipe-1")
	dispatched := 0
	for _, s := range got {
		if s.JobID != "" {
			dispatched++
		}
	}
	if dispatched != 1 {
		t.Fatalf("expected 1 dispatched job initially, got %d", dispatched)
	}
	
	steps1, _ := dr.GetSteps("pipe-1")
	var s1 *models.PipelineStep
	for i := range steps1 {
		if steps1[i].ID == "s1" {
			s1 = &steps1[i]
		}
	}
	job := jr.jobs[s1.JobID]
	if job == nil || job.Type != models.JobTypeDataClean {
		t.Fatalf("s1 job not derived correctly: %+v", job)
	}
	
	if job.DatasetName != "ds-a" || job.MountPath != "/mnt/data" {
		t.Fatalf("dataset mount not injected: %+v", job)
	}
	if job.DataSpecJSON == "" {
		t.Fatal("DataSpecJSON not injected")
	}
	
	p2, _ := dr.GetPipeline("pipe-1")
	if p2.Status != models.PipelineStatusRunning {
		t.Fatalf("expected running, got %s", p2.Status)
	}
}

func jobOf(t *testing.T, jr *fakeJobRepo, dr *fakeDataRepo, stepID string) *models.Job {
	t.Helper()
	steps, _ := dr.GetSteps("pipe-1")
	for i := range steps {
		if steps[i].ID == stepID {
			return jr.jobs[steps[i].JobID]
		}
	}
	t.Fatalf("step %s not found", stepID)
	return nil
}

func TestSyncAdvancesDAG(t *testing.T) {
	dr := newFakeDataRepo()
	jr := newFakeJobRepo()
	svc := NewService(dr, jr, nil)
	p, steps := fixturePipeline()
	_ = svc.CreatePipeline(p, steps)
	_ = svc.SubmitPipeline("pipe-1")

	job := jobOf(t, jr, dr, "s1")
	job.Status = models.JobStatusCompleted
	if err := svc.SyncPipeline("pipe-1"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	steps2, _ := dr.GetSteps("pipe-1")
	byID := map[string]*models.PipelineStep{}
	for i := range steps2 {
		byID[steps2[i].ID] = &steps2[i]
	}
	if byID["s1"].Status != models.StepStatusSucceeded {
		t.Fatalf("s1 should be succeeded")
	}
	if byID["s2"].JobID == "" {
		t.Fatalf("s2 should be dispatched after s1 succeeded")
	}
	if byID["s3"].JobID != "" {
		t.Fatalf("s3 should still be blocked")
	}

	jobOf(t, jr, dr, "s2").Status = models.JobStatusCompleted
	_ = svc.SyncPipeline("pipe-1")
	steps3, _ := dr.GetSteps("pipe-1")
	for i := range steps3 {
		byID[steps3[i].ID] = &steps3[i]
	}
	if byID["s3"].JobID == "" {
		t.Fatalf("s3 should be dispatched after s2 succeeded")
	}

	jobOf(t, jr, dr, "s3").Status = models.JobStatusCompleted
	_ = svc.SyncPipeline("pipe-1")
	p2, _ := dr.GetPipeline("pipe-1")
	if p2.Status != models.PipelineStatusCompleted {
		t.Fatalf("expected completed, got %s", p2.Status)
	}
}

func TestSyncFailsPipelineOnStepFailure(t *testing.T) {
	dr := newFakeDataRepo()
	jr := newFakeJobRepo()
	svc := NewService(dr, jr, nil)
	p, steps := fixturePipeline()
	_ = svc.CreatePipeline(p, steps)
	_ = svc.SubmitPipeline("pipe-1")
	jobOf(t, jr, dr, "s1").Status = models.JobStatusFailed
	if err := svc.SyncPipeline("pipe-1"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	p2, _ := dr.GetPipeline("pipe-1")
	if p2.Status != models.PipelineStatusFailed {
		t.Fatalf("expected failed, got %s", p2.Status)
	}
}

func TestCancelPipeline(t *testing.T) {
	dr := newFakeDataRepo()
	jr := newFakeJobRepo()
	svc := NewService(dr, jr, nil)
	p, steps := fixturePipeline()
	_ = svc.CreatePipeline(p, steps)
	_ = svc.SubmitPipeline("pipe-1")
	if err := svc.CancelPipeline("pipe-1"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	p2, _ := dr.GetPipeline("pipe-1")
	if p2.Status != models.PipelineStatusCancelled {
		t.Fatalf("expected cancelled, got %s", p2.Status)
	}
	
	for _, j := range jr.jobs {
		if j.Status != models.JobStatusCancelled {
			t.Fatalf("job %s not cancelled: %s", j.ID, j.Status)
		}
	}
}