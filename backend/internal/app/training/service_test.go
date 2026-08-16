package training

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	trainingdomain "fuze-ai-paas/backend/internal/domain/training"
	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type fakeJobs struct {
	items      map[string]*models.Job
	seq        int
	createErr  error
	updateErr  error
	updateCall int
}

func newFakeJobs() *fakeJobs { return &fakeJobs{items: map[string]*models.Job{}} }

func (f *fakeJobs) GetJobs() ([]models.Job, error) {
	out := []models.Job{}
	for _, j := range f.items {
		out = append(out, *j)
	}
	return out, nil
}
func (f *fakeJobs) GetJobsByTenant(tenantID string) ([]models.Job, error) {
	out := []models.Job{}
	for _, j := range f.items {
		if tenantID == "" || j.TenantID == tenantID {
			out = append(out, *j)
		}
	}
	return out, nil
}
func (f *fakeJobs) GetJob(id string) (*models.Job, error) {
	j, ok := f.items[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *j
	return &cp, nil
}
func (f *fakeJobs) CreateJob(j *models.Job) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.seq++
	j.ID = "job-" + string(rune('a'+f.seq-1))
	j.Status = models.JobStatusPending
	cp := *j
	f.items[j.ID] = &cp
	return nil
}
func (f *fakeJobs) UpdateJob(j *models.Job) error {
	f.updateCall++
	if f.updateErr != nil {
		return f.updateErr
	}
	if _, ok := f.items[j.ID]; !ok {
		return errors.New("not found")
	}
	cp := *j
	f.items[j.ID] = &cp
	return nil
}

func (f *fakeJobs) UpdateJobStatus(j *models.Job) error { return f.UpdateJob(j) }
func (f *fakeJobs) UpdateJobSpec(j *models.Job) error   { return f.UpdateJob(j) }
func (f *fakeJobs) DeleteJob(id string) error { delete(f.items, id); return nil }

type fakeQuota struct {
	gpus, mem, jobs int
	reserveErr      error
	released        bool
	releaseCount    int
	lastRelease     struct{ g, m, j int }
}

func (q *fakeQuota) GetQuota(string) (*models.Quota, error) { return &models.Quota{}, nil }
func (q *fakeQuota) ListQuotas() ([]models.Quota, error)    { return nil, nil }
func (q *fakeQuota) UpsertQuota(*models.Quota) error        { return nil }
func (q *fakeQuota) CheckAndReserve(_ string, g, m, j int) error {
	if q.reserveErr != nil {
		return q.reserveErr
	}
	q.gpus += g
	q.mem += m
	q.jobs += j
	return nil
}
func (q *fakeQuota) Release(_ string, g, m, j int) error {
	q.released = true
	q.releaseCount++
	q.lastRelease = struct{ g, m, j int }{g, m, j}
	q.gpus -= g
	q.mem -= m
	q.jobs -= j
	return nil
}
func (q *fakeQuota) AdjustReservation(_ string, og, om, ng, nm int) error {
	q.gpus += ng - og
	q.mem += nm - om
	return nil
}

type fakeModels struct {
	model     *models.Model
	versions  []models.ModelVersion
	createErr error
}

func (m *fakeModels) GetModels() ([]models.Model, error) { return nil, nil }
func (m *fakeModels) GetModelsByTenant(string) ([]models.Model, error) { return nil, nil }
func (m *fakeModels) GetModel(id string) (*models.Model, error) {
	if m.model == nil || m.model.ID != id {
		return nil, errors.New("not found")
	}
	return m.model, nil
}
func (m *fakeModels) GetModelVersions(string) ([]models.ModelVersion, error) { return m.versions, nil }
func (m *fakeModels) GetModelVersion(string, string) (*models.ModelVersion, error) {
	return nil, errors.New("not found")
}
func (m *fakeModels) CreateModel(*models.Model) error { return nil }
func (m *fakeModels) UpdateModel(*models.Model) error { return nil }
func (m *fakeModels) DeleteModel(string) error        { return nil }
func (m *fakeModels) CreateModelVersion(v *models.ModelVersion) error {
	if m.createErr != nil {
		return m.createErr
	}
	v.ID = "mv-1"
	m.versions = append(m.versions, *v)
	return nil
}
func (m *fakeModels) DeleteModelVersion(string, string) error { return nil }

type fakeScheduler struct {
	submitted  []string
	submitErr  error
	cancelled  []string
	cancelErr  error
	terminated []string
	terminErr  error
}

func (s *fakeScheduler) SubmitJob(j *models.Job) error {
	if s.submitErr != nil {
		return s.submitErr
	}
	s.submitted = append(s.submitted, j.ID)
	j.VolcanoJobName = "vj-" + j.ID
	return nil
}
func (s *fakeScheduler) TerminateJob(j *models.Job) error {
	if s.terminErr != nil {
		return s.terminErr
	}
	s.terminated = append(s.terminated, j.ID)
	return nil
}
func (s *fakeScheduler) CancelJob(j *models.Job) error {
	if s.cancelErr != nil {
		return s.cancelErr
	}
	s.cancelled = append(s.cancelled, j.ID)
	return nil
}

func newSvc() (*Service, *fakeJobs, *fakeQuota, *fakeModels, *fakeScheduler) {
	jobs, q, mdl, sch := newFakeJobs(), &fakeQuota{}, &fakeModels{}, &fakeScheduler{}
	return NewService(jobs, q, mdl, sch), jobs, q, mdl, sch
}

func baseInput() SubmitInput {
	return SubmitInput{
		Name: "bert-finetune",
		Spec: trainingdomain.Spec{Image: "pytorch:2.3", Command: "python train.py", GPUs: 2, Memory: 16},
	}
}

func TestSubmitPersistsNormalizedSpecAndReservesQuota(t *testing.T) {
	svc, jobs, q, _, sch := newSvc()

	job, err := svc.Submit("tenant-a", baseInput())
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if job.ID == "" || job.TenantID != "tenant-a" {
		t.Fatalf("job identity wrong: %+v", job)
	}
	if job.Type != models.JobTypeTraining {
		t.Fatalf("expected training type, got %q", job.Type)
	}
	if job.ClusterID != trainingdomain.DefaultClusterID {
		t.Fatalf("cluster not defaulted: %q", job.ClusterID)
	}
	if q.gpus != 2 || q.mem != 16 || q.jobs != 1 {
		t.Fatalf("quota reservation wrong: %+v", q)
	}
	if len(sch.submitted) != 1 {
		t.Fatal("job must be dispatched to the scheduler")
	}
	if _, ok := jobs.items[job.ID]; !ok {
		t.Fatal("job not persisted")
	}
}

func TestSubmitReservesDistributedTotals(t *testing.T) {
	svc, _, q, _, _ := newSvc()
	in := baseInput()
	in.Spec.GPUs = 8
	in.Spec.Memory = 64
	in.Spec.Distributed = true
	in.Spec.Replicas = 3

	if _, err := svc.Submit("t", in); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if q.gpus != 32 || q.mem != 256 {
		t.Fatalf("expected 4 replicas worth of quota, got gpus=%d mem=%d", q.gpus, q.mem)
	}
}

func TestSubmitRejectsInvalidSpec(t *testing.T) {
	svc, _, q, _, _ := newSvc()
	in := baseInput()
	in.Spec.GPUs = -1

	_, err := svc.Submit("t", in)
	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("expected ErrInvalidSpec, got %v", err)
	}
	
	if q.gpus != 0 || q.jobs != 0 {
		t.Fatalf("quota touched on rejected spec: %+v", q)
	}
}

func TestSubmitPropagatesQuotaRejection(t *testing.T) {
	svc, jobs, q, _, _ := newSvc()
	q.reserveErr = ports.ErrQuotaExceeded

	_, err := svc.Submit("t", baseInput())
	if !errors.Is(err, ports.ErrQuotaExceeded) {
		t.Fatalf("expected quota error, got %v", err)
	}
	if len(jobs.items) != 0 {
		t.Fatal("no job may be persisted when quota is exhausted")
	}
}

func TestSubmitReleasesQuotaWhenPersistFails(t *testing.T) {
	svc, jobs, q, _, _ := newSvc()
	jobs.createErr = errors.New("db down")

	if _, err := svc.Submit("t", baseInput()); err == nil {
		t.Fatal("expected persist failure")
	}
	if !q.released || q.gpus != 0 || q.jobs != 0 {
		t.Fatalf("quota not rolled back: %+v", q)
	}
}

func TestSubmitRecordsDispatchFailureWithoutFailingRequest(t *testing.T) {
	svc, jobs, _, _, sch := newSvc()
	sch.submitErr = errors.New("image not found")

	job, err := svc.Submit("t", baseInput())
	if err != nil {
		t.Fatalf("dispatch failure must not fail the request: %v", err)
	}
	stored := jobs.items[job.ID]
	if stored.FailureReason == "" || stored.SubmitAttempts != 1 {
		t.Fatalf("dispatch failure not recorded: %+v", stored)
	}
}

func TestSubmitAppliesTemplateDefaults(t *testing.T) {
	svc, _, _, _, _ := newSvc()
	in := SubmitInput{
		Name:       "ddp-run",
		TemplateID: trainingdomain.TemplatePyTorchDDP,
		Spec:       trainingdomain.Spec{GPUs: 2},
	}

	job, err := svc.Submit("t", in)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if job.Image == "" {
		t.Fatal("template must fill in the image")
	}
	if !job.Distributed || job.Framework != trainingdomain.FrameworkPyTorchDDP {
		t.Fatalf("template defaults not applied: %+v", job)
	}
	
	if job.GPUs != 2 {
		t.Fatalf("template overrode user GPUs: %d", job.GPUs)
	}
	if job.TemplateID != trainingdomain.TemplatePyTorchDDP {
		t.Fatalf("template id not recorded: %q", job.TemplateID)
	}
}

func TestSubmitRejectsUnknownTemplate(t *testing.T) {
	svc, _, _, _, _ := newSvc()
	in := baseInput()
	in.TemplateID = "no-such"
	if _, err := svc.Submit("t", in); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("unknown template must be rejected, got %v", err)
	}
}

func TestRecordCheckpointPersistsProgress(t *testing.T) {
	svc, jobs, _, _, _ := newSvc()
	job := seedRunningJob(t, svc, jobs)

	if err := svc.RecordCheckpoint(job.ID, trainingdomain.Checkpoint{URI: "s3://b/ck-100", Step: 100}); err != nil {
		t.Fatalf("RecordCheckpoint: %v", err)
	}
	stored := jobs.items[job.ID]
	if stored.LatestCheckpointURI != "s3://b/ck-100" || stored.LatestCheckpointStep != 100 {
		t.Fatalf("checkpoint not persisted: %+v", stored)
	}
	if stored.LatestCheckpointAt == nil {
		t.Fatal("checkpoint timestamp must be stamped")
	}
}

func TestRecordCheckpointRejectsStaleStep(t *testing.T) {
	svc, jobs, _, _, _ := newSvc()
	job := seedRunningJob(t, svc, jobs)
	if err := svc.RecordCheckpoint(job.ID, trainingdomain.Checkpoint{URI: "s3://b/ck-100", Step: 100}); err != nil {
		t.Fatalf("first checkpoint: %v", err)
	}

	err := svc.RecordCheckpoint(job.ID, trainingdomain.Checkpoint{URI: "s3://b/ck-50", Step: 50})
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("stale checkpoint must be rejected as a state conflict, got %v", err)
	}
	if jobs.items[job.ID].LatestCheckpointStep != 100 {
		t.Fatal("stale checkpoint must not roll progress back")
	}
}

func TestRecordCheckpointUnknownJob(t *testing.T) {
	svc, _, _, _, _ := newSvc()
	if err := svc.RecordCheckpoint("nope", trainingdomain.Checkpoint{URI: "u", Step: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestHandleFailureSchedulesResumeFromCheckpoint(t *testing.T) {
	svc, jobs, q, _, _ := newSvc()
	job := seedRunningJob(t, svc, jobs)
	_ = svc.RecordCheckpoint(job.ID, trainingdomain.Checkpoint{URI: "s3://b/ck-100", Step: 100})

	outcome, err := svc.HandleFailure(job.ID, "node preempted")
	if err != nil {
		t.Fatalf("HandleFailure: %v", err)
	}
	if outcome != trainingdomain.OutcomeRetry {
		t.Fatalf("expected retry, got %v", outcome)
	}

	stored := jobs.items[job.ID]
	if stored.Status != models.JobStatusRetrying {
		t.Fatalf("status = %q, want retrying", stored.Status)
	}
	if stored.ResumeFrom != "s3://b/ck-100" || stored.RetryAttempts != 1 {
		t.Fatalf("resume state wrong: %+v", stored)
	}
	
	if stored.VolcanoJobName != "" {
		t.Fatalf("volcano job name must be cleared for redispatch, got %q", stored.VolcanoJobName)
	}
	
	if q.released {
		t.Fatal("quota must stay reserved while the job is retrying")
	}
}

func TestHandleFailureWithoutCheckpointIsTerminal(t *testing.T) {
	svc, jobs, _, _, _ := newSvc()
	job := seedRunningJob(t, svc, jobs)

	outcome, err := svc.HandleFailure(job.ID, "crashed")
	if err != nil {
		t.Fatalf("HandleFailure: %v", err)
	}
	if outcome != trainingdomain.OutcomeFailed {
		t.Fatalf("expected terminal failure, got %v", outcome)
	}
	if jobs.items[job.ID].Status != models.JobStatusFailed {
		t.Fatalf("status = %q, want failed", jobs.items[job.ID].Status)
	}
}

func TestResumeRequeuesRetryingJob(t *testing.T) {
	svc, jobs, _, _, sch := newSvc()
	job := seedRunningJob(t, svc, jobs)
	_ = svc.RecordCheckpoint(job.ID, trainingdomain.Checkpoint{URI: "s3://b/ck-100", Step: 100})
	if _, err := svc.HandleFailure(job.ID, "boom"); err != nil {
		t.Fatalf("HandleFailure: %v", err)
	}

	before := len(sch.submitted)
	if err := svc.Resume(job.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	stored := jobs.items[job.ID]
	if stored.Status != models.JobStatusPending {
		t.Fatalf("resumed job must go back to pending, got %q", stored.Status)
	}
	if len(sch.submitted) != before+1 {
		t.Fatal("resumed job must be redispatched")
	}
}

func TestResumeRejectsJobThatIsNotRetrying(t *testing.T) {
	svc, jobs, _, _, _ := newSvc()
	job := seedRunningJob(t, svc, jobs)
	if err := svc.Resume(job.ID); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState, got %v", err)
	}
}

func TestCompleteRegistersModelVersionWithLineage(t *testing.T) {
	svc, jobs, _, mdl, _ := newSvc()
	mdl.model = &models.Model{ID: "m1", Name: "bert"}

	in := baseInput()
	in.Checkpointing = trainingdomain.CheckpointPolicy{Enabled: true}
	in.Registration = trainingdomain.ModelRegistration{Enabled: true, ModelID: "m1", VersionTag: "v1"}
	job, err := svc.Submit("t", in)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	markRunning(t, jobs, job.ID)
	_ = svc.RecordCheckpoint(job.ID, trainingdomain.Checkpoint{URI: "s3://b/final", Step: 900, Hash: "4e2d", SizeBytes: 123456})

	if err := svc.Complete(job.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(mdl.versions) != 1 {
		t.Fatalf("expected one registered version, got %d", len(mdl.versions))
	}
	v := mdl.versions[0]
	if v.ModelID != "m1" || v.Version != "v1" || v.StorageURI != "s3://b/final" {
		t.Fatalf("version content wrong: %+v", v)
	}
	if v.Hash != "4e2d" || v.SizeBytes != 123456 {
		t.Fatalf("version artifact metadata missing: %+v", v)
	}
	if v.Files == "" {
		t.Fatalf("version file manifest must be populated: %+v", v)
	}
	if v.SourceJobID != job.ID {
		t.Fatalf("lineage missing: %+v", v)
	}
	if v.TenantID != "t" {
		t.Fatalf("version must carry the training job's tenant for isolation, got %q", v.TenantID)
	}
	stored := jobs.items[job.ID]
	if stored.RegisteredVersionID != "mv-1" {
		t.Fatalf("registered version id not recorded: %+v", stored)
	}
	if stored.Status != models.JobStatusCompleted {
		t.Fatalf("status = %q, want completed", stored.Status)
	}
}

func TestCompleteIsIdempotent(t *testing.T) {
	svc, jobs, _, mdl, _ := newSvc()
	mdl.model = &models.Model{ID: "m1"}
	in := baseInput()
	in.Checkpointing = trainingdomain.CheckpointPolicy{Enabled: true}
	in.Registration = trainingdomain.ModelRegistration{Enabled: true, ModelID: "m1", VersionTag: "v1"}
	job, _ := svc.Submit("t", in)
	markRunning(t, jobs, job.ID)
	_ = svc.RecordCheckpoint(job.ID, trainingdomain.Checkpoint{URI: "s3://b/final", Step: 900})

	if err := svc.Complete(job.ID); err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	if err := svc.Complete(job.ID); err != nil {
		t.Fatalf("second Complete: %v", err)
	}
	if len(mdl.versions) != 1 {
		t.Fatalf("registration must be idempotent, got %d versions", len(mdl.versions))
	}
}

func TestCompleteSurvivesRegistrationFailure(t *testing.T) {
	svc, jobs, _, mdl, _ := newSvc()
	mdl.model = &models.Model{ID: "m1"}
	mdl.createErr = errors.New("model repo down")

	in := baseInput()
	in.Checkpointing = trainingdomain.CheckpointPolicy{Enabled: true}
	in.Registration = trainingdomain.ModelRegistration{Enabled: true, ModelID: "m1", VersionTag: "v1"}
	job, _ := svc.Submit("t", in)
	markRunning(t, jobs, job.ID)
	_ = svc.RecordCheckpoint(job.ID, trainingdomain.Checkpoint{URI: "s3://b/final", Step: 900})

	if err := svc.Complete(job.ID); err != nil {
		t.Fatalf("Complete must not fail because registration failed: %v", err)
	}
	stored := jobs.items[job.ID]
	if stored.Status != models.JobStatusCompleted {
		t.Fatalf("status = %q, want completed", stored.Status)
	}
	if stored.FailureReason == "" {
		t.Fatal("registration failure must be surfaced to the user")
	}
}

func TestCompleteSkipsRegistrationWhenDisabled(t *testing.T) {
	svc, jobs, _, mdl, _ := newSvc()
	job := seedRunningJob(t, svc, jobs)

	if err := svc.Complete(job.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(mdl.versions) != 0 {
		t.Fatal("no registration was requested")
	}
}

func TestCompleteRejectsUnknownModel(t *testing.T) {
	svc, jobs, _, mdl, _ := newSvc()
	mdl.model = &models.Model{ID: "other"}

	in := baseInput()
	in.Checkpointing = trainingdomain.CheckpointPolicy{Enabled: true}
	in.Registration = trainingdomain.ModelRegistration{Enabled: true, ModelID: "m1", VersionTag: "v1"}
	job, _ := svc.Submit("t", in)
	markRunning(t, jobs, job.ID)
	_ = svc.RecordCheckpoint(job.ID, trainingdomain.Checkpoint{URI: "s3://b/final", Step: 1})

	if err := svc.Complete(job.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(mdl.versions) != 0 {
		t.Fatal("must not register into a non-existent model")
	}
	if jobs.items[job.ID].FailureReason == "" {
		t.Fatal("the reason registration was skipped must be recorded")
	}
}

func TestListTemplates(t *testing.T) {
	svc, _, _, _, _ := newSvc()
	if len(svc.ListTemplates()) == 0 {
		t.Fatal("expected built-in templates")
	}
}

func TestListOnlyReturnsTrainingJobsOfTenant(t *testing.T) {
	svc, jobs, _, _, _ := newSvc()
	if _, err := svc.Submit("t1", baseInput()); err != nil {
		t.Fatalf("submit t1: %v", err)
	}
	if _, err := svc.Submit("t2", baseInput()); err != nil {
		t.Fatalf("submit t2: %v", err)
	}
	
	jobs.items["infer-1"] = &models.Job{ID: "infer-1", TenantID: "t1", Type: models.JobTypeInference}

	got, err := svc.List("t1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 training job for t1, got %d", len(got))
	}
	if got[0].TenantID != "t1" || got[0].Type != models.JobTypeTraining {
		t.Fatalf("unexpected job %+v", got[0])
	}
}

func TestListWithEmptyTenantReturnsAllTrainingJobs(t *testing.T) {
	svc, _, _, _, _ := newSvc()
	if _, err := svc.Submit("t1", baseInput()); err != nil {
		t.Fatalf("submit t1: %v", err)
	}
	if _, err := svc.Submit("t2", baseInput()); err != nil {
		t.Fatalf("submit t2: %v", err)
	}

	got, err := svc.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(got))
	}
}

func TestGetRejectsNonTrainingJob(t *testing.T) {
	svc, jobs, _, _, _ := newSvc()
	jobs.items["infer-1"] = &models.Job{ID: "infer-1", Type: models.JobTypeInference}

	if _, err := svc.Get("infer-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-training job, got %v", err)
	}
}

func TestGetUnknownJob(t *testing.T) {
	svc, _, _, _, _ := newSvc()
	if _, err := svc.Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCancelStopsClusterWorkloadAndReleasesQuota(t *testing.T) {
	svc, jobs, q, _, sch := newSvc()
	job := seedRunningJob(t, svc, jobs)
	before := q.gpus

	got, err := svc.Cancel(job.ID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got.Status != models.JobStatusCancelled {
		t.Fatalf("expected cancelled, got %s", got.Status)
	}
	if len(sch.cancelled) != 1 || sch.cancelled[0] != job.ID {
		t.Fatalf("expected cluster workload cancelled, got %v", sch.cancelled)
	}
	if q.gpus != before-2 {
		t.Fatalf("expected quota released, gpus %d -> %d", before, q.gpus)
	}
	if jobs.items[job.ID].Status != models.JobStatusCancelled {
		t.Fatal("cancelled status not persisted")
	}
}

func TestCancelRejectsTerminalJob(t *testing.T) {
	svc, jobs, _, _, sch := newSvc()
	job := seedRunningJob(t, svc, jobs)
	jobs.items[job.ID].Status = models.JobStatusCompleted

	if _, err := svc.Cancel(job.ID); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState, got %v", err)
	}
	if len(sch.cancelled) != 0 {
		t.Fatal("terminal job must not touch the cluster")
	}
}

func TestCancelKeepsQuotaWhenClusterStopFails(t *testing.T) {
	svc, jobs, q, _, sch := newSvc()
	job := seedRunningJob(t, svc, jobs)
	sch.cancelErr = errors.New("apiserver down")
	before := q.gpus

	if _, err := svc.Cancel(job.ID); err == nil {
		t.Fatal("expected cancel to fail")
	}
	
	if q.gpus != before {
		t.Fatalf("quota must stay reserved, gpus %d -> %d", before, q.gpus)
	}
	if jobs.items[job.ID].Status == models.JobStatusCancelled {
		t.Fatal("status must not flip to cancelled when cluster stop failed")
	}
}

func TestDeleteReleasesQuotaForActiveJob(t *testing.T) {
	svc, jobs, q, _, sch := newSvc()
	job := seedRunningJob(t, svc, jobs)
	before := q.gpus

	if err := svc.Delete(job.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(sch.terminated) != 1 {
		t.Fatalf("expected cluster workload terminated, got %v", sch.terminated)
	}
	if q.gpus != before-2 {
		t.Fatalf("expected quota released, gpus %d -> %d", before, q.gpus)
	}
	if _, ok := jobs.items[job.ID]; ok {
		t.Fatal("job record should be deleted")
	}
}

func TestDeleteTerminalJobDoesNotDoubleRelease(t *testing.T) {
	svc, jobs, q, _, sch := newSvc()
	job := seedRunningJob(t, svc, jobs)
	jobs.items[job.ID].Status = models.JobStatusCompleted
	before := q.gpus

	if err := svc.Delete(job.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if q.gpus != before {
		t.Fatalf("terminal job already gave quota back, gpus %d -> %d", before, q.gpus)
	}
	if len(sch.terminated) != 0 {
		t.Fatal("terminal job must not touch the cluster")
	}
}

func TestDeleteUnknownJob(t *testing.T) {
	svc, _, _, _, _ := newSvc()
	if err := svc.Delete("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func seedRunningJob(t *testing.T, svc *Service, jobs *fakeJobs) *models.Job {
	t.Helper()
	in := baseInput()
	in.Checkpointing = trainingdomain.CheckpointPolicy{Enabled: true, IntervalSteps: 100, MaxRetries: 2}
	job, err := svc.Submit("t", in)
	if err != nil {
		t.Fatalf("seed Submit: %v", err)
	}
	markRunning(t, jobs, job.ID)
	return job
}

func markRunning(t *testing.T, jobs *fakeJobs, id string) {
	t.Helper()
	j := jobs.items[id]
	j.Status = models.JobStatusRunning
	now := time.Now()
	j.StartedAt = &now
}

func TestFailureThenCompleteReleasesQuotaOnce(t *testing.T) {
	svc, jobs, q, _, _ := newSvc()

	in := baseInput() 
	job, err := svc.Submit("t", in)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	markRunning(t, jobs, job.ID)

	if _, err := svc.HandleFailure(job.ID, "node lost"); err != nil {
		t.Fatalf("HandleFailure: %v", err)
	}
	if jobs.items[job.ID].Status != models.JobStatusFailed {
		t.Fatalf("expected terminal Failed, got %q", jobs.items[job.ID].Status)
	}
	if q.releaseCount != 1 {
		t.Fatalf("expected 1 release after terminal failure, got %d", q.releaseCount)
	}

	if err := svc.Complete(job.ID); err == nil {
		t.Fatalf("expected Complete on failed job to be rejected")
	}
	if q.releaseCount != 1 {
		t.Fatalf("quota must not be released twice, got %d releases", q.releaseCount)
	}
}

type fakeCostRepo struct {
	mu        sync.Mutex
	records   []*models.GPUUsageRecord
	sumHours  float64
	sumCost   float64
	limitCost float64
}

func (f *fakeCostRepo) GetQuota(ctx context.Context, tenantID string) (llm.TokenQuota, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return llm.TokenQuota{TenantID: tenantID, LimitTokens: 1000, UsedTokens: 0, LimitCost: f.limitCost, UsedCost: f.sumCost}, nil
}

func (f *fakeCostRepo) SetQuota(ctx context.Context, q llm.TokenQuota) error { return nil }

func (f *fakeCostRepo) ListQuotas(ctx context.Context) ([]llm.TokenQuota, error) {
	return nil, nil
}

func (f *fakeCostRepo) CheckAndConsume(ctx context.Context, tenantID string, tokens int64, cost float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sumCost += cost
	return nil
}

func (f *fakeCostRepo) RecordGPUCost(ctx context.Context, rec *models.GPUUsageRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, rec)
	f.sumHours += rec.Hours
	f.sumCost += rec.Cost
	return nil
}

func (f *fakeCostRepo) SumGPUUsage(ctx context.Context, tenantID string, since, until int64) (float64, float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sumHours, f.sumCost, nil
}

func newSvcWithBilling() (*Service, *fakeJobs, *fakeCostRepo, *int32) {
	jobs := newFakeJobs()
	cost := &fakeCostRepo{}
	alertCount := int32(0)

	prices := llm.NewPriceBook()
	if err := prices.SetGPUPrice("", 1.0, "CNY"); err != nil {
		panic(err)
	}

	svc := NewService(
		jobs,
		&fakeQuota{},
		&fakeModels{},
		&fakeScheduler{},
		WithCostBilling(prices, cost, func(ctx context.Context, tenantID string, limit, used float64) error {
			atomic.AddInt32(&alertCount, 1)
			return nil
		}),
	)

	return svc, jobs, cost, &alertCount
}

func TestTrainingSettleGPUCostOnCompletion(t *testing.T) {
	svc, jobs, cost, alertCount := newSvcWithBilling()

	in := baseInput() 
	job, err := svc.Submit("t", in)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	markRunning(t, jobs, job.ID)

	if err := svc.Complete(job.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	recs := cost.records
	if len(recs) != 1 {
		t.Fatalf("expected 1 GPU usage record, got %d", len(recs))
	}
	got := recs[0]
	if got.TenantID != "t" || got.JobID != job.ID {
		t.Fatalf("record tenant/job mismatch: %+v", got)
	}
	if got.GPUCount != 2 {
		t.Fatalf("expected GPUCount=2, got %d", got.GPUCount)
	}
	if got.Hours <= 0 {
		t.Fatalf("expected positive runtime, got %v", got.Hours)
	}
	
	want := 2.0 * got.Hours
	if math.Abs(got.Cost-want) > 1e-9 {
		t.Fatalf("expected cost %v (2 GPU × 1.0 × %vh), got %v", want, got.Hours, got.Cost)
	}
	if atomic.LoadInt32(alertCount) != 0 {
		t.Fatalf("no alert expected below budget, got %d", atomic.LoadInt32(alertCount))
	}
}

func TestTrainingSettleGPUCostFiresBudgetAlert(t *testing.T) {
	svc, jobs, cost, alertCount := newSvcWithBilling()
	
	cost.limitCost = 1e-9

	in := baseInput()
	job, err := svc.Submit("t", in)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	markRunning(t, jobs, job.ID)
	
	backdated := time.Now().Add(-time.Hour)
	jobs.items[job.ID].StartedAt = &backdated

	if err := svc.Complete(job.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(cost.records) != 1 {
		t.Fatalf("expected 1 GPU record, got %d", len(cost.records))
	}
	if got := atomic.LoadInt32(alertCount); got != 1 {
		t.Fatalf("expected exactly 1 budget alert, got %d", got)
	}
}

func TestTrainingSettleGPUCostOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		act  func(svc *Service, id string) error
	}{
		{"fail", func(s *Service, id string) error {
			_, err := s.HandleFailure(id, "boom")
			return err
		}},
		{"cancel", func(s *Service, id string) error { _, err := s.Cancel(id); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, jobs, cost, _ := newSvcWithBilling()

			in := baseInput()
			job, err := svc.Submit("t", in)
			if err != nil {
				t.Fatalf("Submit: %v", err)
			}
			markRunning(t, jobs, job.ID)

			if err := tc.act(svc, job.ID); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			
			if err := svc.Complete(job.ID); err == nil {
				t.Fatalf("expected terminal re-complete to be rejected")
			}
			if len(cost.records) != 1 {
				t.Fatalf("expected exactly 1 GPU record (idempotent), got %d", len(cost.records))
			}
		})
	}
}

var (
	errFineTuneExists   = errors.New("finetune: adapter name exists")
	errFineTuneNotFound = errors.New("finetune: adapter not found")
)

type fakeFineTune struct {
	mu       sync.Mutex
	adapters []*ports.FineTuneAdapter
	byName   map[string]*ports.FineTuneAdapter
}

func (f *fakeFineTune) Create(ctx context.Context, a *ports.FineTuneAdapter) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, dup := f.byName[a.Name]; dup {
		return errFineTuneExists
	}
	a.ID = "adapter-" + a.Name
	f.adapters = append(f.adapters, a)
	f.byName[a.Name] = a
	return nil
}

func (f *fakeFineTune) List(ctx context.Context, filter ports.FineTuneFilter) ([]*ports.FineTuneAdapter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*ports.FineTuneAdapter, 0, len(f.adapters))
	for _, a := range f.adapters {
		if filter.TenantID != "" && a.TenantID != filter.TenantID {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

func (f *fakeFineTune) Get(ctx context.Context, tenantID, id string) (*ports.FineTuneAdapter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.adapters {
		if a.TenantID == tenantID && a.ID == id {
			return a, nil
		}
	}
	return nil, errFineTuneNotFound
}

func (f *fakeFineTune) Update(ctx context.Context, a *ports.FineTuneAdapter) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, x := range f.adapters {
		if x.ID == a.ID {
			f.adapters[i] = a
			f.byName[a.Name] = a
			return nil
		}
	}
	return errFineTuneNotFound
}

func (f *fakeFineTune) Delete(ctx context.Context, tenantID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, x := range f.adapters {
		if x.TenantID == tenantID && x.ID == id {
			f.adapters = append(f.adapters[:i], f.adapters[i+1:]...)
			delete(f.byName, x.Name)
			return nil
		}
	}
	return errFineTuneNotFound
}

func TestCompleteRegistersFineTuneAdapter(t *testing.T) {
	ft := &fakeFineTune{byName: map[string]*ports.FineTuneAdapter{}}
	
	jobs := newFakeJobs()
	svc := NewService(jobs, &fakeQuota{}, &fakeModels{}, &fakeScheduler{}, WithFineTuneRegistry(ft))

	job := seedRunningJob(t, svc, jobs)

	j := jobs.items[job.ID]
	j.RegisterAdapterEnabled = true
	j.AdapterBaseModel = "qwen2.5-7b"
	j.AdapterMethod = ports.MethodLoRA
	j.AdapterRank = 8

	if err := svc.RecordCheckpoint(job.ID, trainingdomain.Checkpoint{URI: "s3://b/adapter-ckpt", Step: 1000}); err != nil {
		t.Fatalf("RecordCheckpoint: %v", err)
	}

	if err := svc.Complete(job.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	got, err := ft.List(context.Background(), ports.FineTuneFilter{TenantID: "t"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 auto-registered adapter, got %d", len(got))
	}
	ad := got[0]
	if ad.SourceJobID != job.ID {
		t.Fatalf("adapter SourceJobID = %q, want %q", ad.SourceJobID, job.ID)
	}
	if ad.BaseModel != "qwen2.5-7b" || ad.Method != ports.MethodLoRA || ad.Rank != 8 {
		t.Fatalf("adapter base/method/rank mismatch: %+v", ad)
	}
	if ad.Path != "s3://b/adapter-ckpt" {
		t.Fatalf("adapter Path = %q, want checkpoint URI", ad.Path)
	}
	
	if jobs.items[job.ID].RegisteredAdapterID != ad.ID {
		t.Fatalf("job RegisteredAdapterID not set")
	}
}

func TestCompleteNoAdapterWhenDisabled(t *testing.T) {
	ft := &fakeFineTune{byName: map[string]*ports.FineTuneAdapter{}}
	jobs := newFakeJobs()
	svc := NewService(jobs, &fakeQuota{}, &fakeModels{}, &fakeScheduler{}, WithFineTuneRegistry(ft))

	job := seedRunningJob(t, svc, jobs)
	_ = svc.RecordCheckpoint(job.ID, trainingdomain.Checkpoint{URI: "s3://b/ck", Step: 1})

	if err := svc.Complete(job.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got, _ := ft.List(context.Background(), ports.FineTuneFilter{TenantID: "t"}); len(got) != 0 {
		t.Fatalf("expected no adapter when closure disabled, got %d", len(got))
	}
}