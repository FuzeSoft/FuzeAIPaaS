
package training

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/adapter"
	trainingdomain "fuze-ai-paas/backend/internal/domain/training"
	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

var (
	ErrNotFound     = errors.New("training job not found")
	ErrInvalidSpec  = errors.New("invalid training spec")
	ErrInvalidState = errors.New("invalid training job state")
)

const maxFailureReasonLen = 500

type Scheduler interface {
	SubmitJob(*models.Job) error
	TerminateJob(*models.Job) error
	CancelJob(*models.Job) error
}

type SubmitInput struct {
	Name          string
	ClusterID     string
	TemplateID    string
	Spec          trainingdomain.Spec
	Checkpointing trainingdomain.CheckpointPolicy
	Registration  trainingdomain.ModelRegistration
}

type Service struct {
	jobs   ports.JobWriter
	quota  ports.QuotaWriter
	models ports.ModelWriter
	sched  Scheduler

	prices   *llm.PriceBook
	costRepo ports.CostRepository
	
	alert func(ctx context.Context, tenantID string, limitCost, usedCost float64) error

	adapters ports.FineTuneRepository

	experiments ports.ExperimentRepository
}

type TrainingOption func(*Service)

func WithCostBilling(prices *llm.PriceBook, costRepo ports.CostRepository, alert func(ctx context.Context, tenantID string, limitCost, usedCost float64) error) TrainingOption {
	return func(s *Service) {
		s.prices = prices
		s.costRepo = costRepo
		s.alert = alert
	}
}

func WithFineTuneRegistry(repo ports.FineTuneRepository) TrainingOption {
	return func(s *Service) {
		s.adapters = repo
	}
}

func WithExperimentRepository(repo ports.ExperimentRepository) TrainingOption {
	return func(s *Service) {
		s.experiments = repo
	}
}

func NewService(jobs ports.JobWriter, quota ports.QuotaWriter, mdl ports.ModelWriter, sched Scheduler, opts ...TrainingOption) *Service {
	s := &Service{jobs: jobs, quota: quota, models: mdl, sched: sched}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *Service) ListTemplates() []trainingdomain.Template {
	return trainingdomain.BuiltinTemplates()
}

func (s *Service) List(tenantID string) ([]models.Job, error) {
	var (
		all []models.Job
		err error
	)
	if tenantID == "" {
		all, err = s.jobs.GetJobs()
	} else {
		all, err = s.jobs.GetJobsByTenant(tenantID)
	}
	if err != nil {
		return nil, err
	}
	out := make([]models.Job, 0, len(all))
	for i := range all {
		if all[i].Type == models.JobTypeTraining {
			out = append(out, all[i])
		}
	}
	return out, nil
}

func (s *Service) Get(jobID string) (*models.Job, error) {
	job, _, err := s.load(jobID)
	return job, err
}

func (s *Service) Cancel(jobID string) (*models.Job, error) {
	job, agg, err := s.load(jobID)
	if err != nil {
		return nil, err
	}
	if agg.IsTerminal() {
		return nil, fmt.Errorf("%w: job %s is already %s", ErrInvalidState, jobID, agg.Status)
	}

	if err := s.sched.CancelJob(job); err != nil {
		return nil, err
	}
	if !agg.MarkCancelled() {
		return nil, fmt.Errorf("%w: job %s cannot be cancelled from %s", ErrInvalidState, jobID, agg.Status)
	}

	adapter.TrainingSyncToModel(agg, job)
	if err := s.jobs.UpdateJobStatus(job); err != nil {
		return nil, err
	}
	s.releaseQuota(job)
	s.settleGPUCost(agg, models.JobStatusCancelled)
	return job, nil
}

func (s *Service) Delete(jobID string) error {
	job, agg, err := s.load(jobID)
	if err != nil {
		return err
	}

	active := !agg.IsTerminal()
	if active {
		if err := s.sched.TerminateJob(job); err != nil {
			return err
		}
	}
	if err := s.jobs.DeleteJob(jobID); err != nil {
		return err
	}
	
	if active {
		s.releaseQuota(job)
	}
	return nil
}

func (s *Service) Submit(tenantID string, in SubmitInput) (*models.Job, error) {
	spec := in.Spec
	if in.TemplateID != "" {
		tpl, ok := trainingdomain.FindTemplate(in.TemplateID)
		if !ok {
			return nil, fmt.Errorf("%w: unknown template %q", ErrInvalidSpec, in.TemplateID)
		}
		spec = tpl.Apply(spec)
	}

	agg := &trainingdomain.TrainingJob{
		TenantID:      tenantID,
		ClusterID:     in.ClusterID,
		Name:          in.Name,
		Spec:          spec,
		Checkpointing: in.Checkpointing,
		Registration:  in.Registration,
	}
	agg.Normalize()
	if err := agg.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidSpec, err)
	}

	gpus, mem := agg.Spec.TotalGPUs(), agg.Spec.TotalMemory()
	
	if err := s.quota.CheckAndReserve(tenantID, gpus, mem, 1); err != nil {
		return nil, err
	}

	job := &models.Job{}
	adapter.TrainingSpecToModel(agg, job)
	if err := s.jobs.CreateJob(job); err != nil {
		
		_ = s.quota.Release(tenantID, gpus, mem, 1)
		return nil, err
	}

	if err := s.sched.SubmitJob(job); err != nil {
		log.Printf("[Training] dispatch failed for job %s: %v", job.ID, err)
		job.FailureReason = truncateReason(err.Error())
		job.SubmitAttempts = 1
	}
	if err := s.jobs.UpdateJobStatus(job); err != nil {
		log.Printf("[Training] failed to persist dispatch result for job %s: %v", job.ID, err)
	}
	return job, nil
}

func (s *Service) RecordCheckpoint(jobID string, ck trainingdomain.Checkpoint) error {
	job, agg, err := s.load(jobID)
	if err != nil {
		return err
	}
	if ck.CreatedAt.IsZero() {
		ck.CreatedAt = time.Now()
	}
	
	if err := agg.RecordCheckpoint(ck); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidState, err)
	}
	adapter.TrainingSyncToModel(agg, job)
	return s.jobs.UpdateJobStatus(job)
}

func (s *Service) HandleFailure(jobID, reason string) (trainingdomain.FailureOutcome, error) {
	job, agg, err := s.load(jobID)
	if err != nil {
		return trainingdomain.OutcomeFailed, err
	}

	wasTerminal := agg.IsTerminal()
	outcome := agg.HandleFailure(truncateReason(reason))
	adapter.TrainingSyncToModel(agg, job)
	if outcome == trainingdomain.OutcomeRetry {
		
		job.VolcanoJobName = ""
	}
	if err := s.jobs.UpdateJobStatus(job); err != nil {
		return outcome, err
	}
	
	if outcome == trainingdomain.OutcomeFailed && !wasTerminal {
		s.releaseQuota(job)
		s.settleGPUCost(agg, models.JobStatusFailed)
	}
	return outcome, nil
}

func (s *Service) Resume(jobID string) error {
	job, agg, err := s.load(jobID)
	if err != nil {
		return err
	}
	if !agg.PrepareRerun() {
		return fmt.Errorf("%w: job %s is %s, only a retrying job can be resumed", ErrInvalidState, jobID, agg.Status)
	}

	adapter.TrainingSyncToModel(agg, job)
	job.VolcanoJobName = ""
	job.SubmitAttempts = 0
	if err := s.jobs.UpdateJobStatus(job); err != nil {
		return err
	}

	if err := s.sched.SubmitJob(job); err != nil {
		job.FailureReason = truncateReason(err.Error())
		job.SubmitAttempts = 1
	}
	return s.jobs.UpdateJobStatus(job)
}

func (s *Service) Complete(jobID string) error {
	job, agg, err := s.load(jobID)
	if err != nil {
		return err
	}
	alreadyDone := agg.Status == trainingdomain.StatusCompleted
	if !agg.MarkCompleted() && !alreadyDone {
		return fmt.Errorf("%w: job %s cannot complete from %s", ErrInvalidState, jobID, agg.Status)
	}
	adapter.TrainingSyncToModel(agg, job)

	if note := s.registerModel(job, agg); note != "" {
		job.FailureReason = truncateReason(note)
	}
	
	if note := s.registerAdapter(job, agg); note != "" {
		job.FailureReason = truncateReason(note)
	}
	if err := s.jobs.UpdateJobStatus(job); err != nil {
		return err
	}
	
	if !alreadyDone {
		s.releaseQuota(job)
		s.settleGPUCost(agg, models.JobStatusCompleted)
	}
	return nil
}

func (s *Service) settleGPUCost(agg *trainingdomain.TrainingJob, status models.JobStatus) {
	if s.prices == nil || s.costRepo == nil {
		return
	}
	gpuType := agg.GPUType()
	gpuCount := agg.Spec.TotalGPUs()
	if gpuCount <= 0 {
		return
	}
	hours := agg.RuntimeHours(time.Now())
	if hours <= 0 {
		return
	}
	cost := s.prices.GPUCost(gpuType, gpuCount, hours)
	if cost <= 0 {
		return
	}
	rec := &models.GPUUsageRecord{
		TenantID: agg.TenantID,
		JobID:    agg.ID,
		GPUType:  gpuType,
		GPUCount: gpuCount,
		Hours:    hours,
		Cost:     cost,
		Currency: s.prices.GPUCurrency(),
		Status:   string(status),
	}
	if err := s.costRepo.RecordGPUCost(context.Background(), rec); err != nil {
		log.Printf("[training] settle gpu cost for job %s failed: %v", agg.ID, err)
		return
	}
	
	if s.alert != nil {
		s.maybeAlert(agg.TenantID)
	}
}

func (s *Service) maybeAlert(tenantID string) {
	q, err := s.costRepo.GetQuota(context.Background(), tenantID)
	if err != nil {
		return
	}
	if q.LimitCost <= 0 {
		return
	}
	ratio := q.UsedCost / q.LimitCost
	if ratio >= 0.8 || q.UsedCost >= q.LimitCost {
		if err := s.alert(context.Background(), tenantID, q.LimitCost, q.UsedCost); err != nil {
			log.Printf("[training] budget alert for tenant %s failed: %v", tenantID, err)
		}
	}
}

func (s *Service) registerModel(job *models.Job, agg *trainingdomain.TrainingJob) string {
	if !agg.ShouldRegisterModel() {
		return ""
	}
	
	if job.RegisteredVersionID != "" {
		return ""
	}
	if _, err := s.models.GetModel(agg.Registration.ModelID); err != nil {
		return fmt.Sprintf("model registration skipped: model %s not found", agg.Registration.ModelID)
	}

	if strings.TrimSpace(agg.Registration.VersionTag) == "" {
		return "model registration skipped: empty version tag"
	}

	v := &models.ModelVersion{
		ModelID:     agg.Registration.ModelID,
		TenantID:    job.TenantID,
		Version:     agg.Registration.VersionTag,
		StorageURI:  agg.LatestCheckpoint.URI,
		Image:       agg.Spec.Image,
		SourceJobID: job.ID,
		Hash:        agg.LatestCheckpoint.Hash,
		SizeBytes:   agg.LatestCheckpoint.SizeBytes,
		Files:       buildVersionFileManifest(agg.LatestCheckpoint),
	}
	
	v.CodeRepo = job.CodeRepo
	v.CodeCommit = job.CodeCommit
	v.TemplateID = job.TemplateID
	v.DatasetID = job.DatasetID
	v.DatasetName = job.DatasetName
	v.DatasetVersion = job.DatasetVersion
	
	if s.experiments != nil {
		if run, rerr := s.experiments.GetRunByJobID(job.ID); rerr == nil && run != nil {
		v.SourceRunID = run.ID
		v.Hyperparameters = run.Hyperparameters
		v.CodeRepo = firstNonEmpty(v.CodeRepo, run.CodeRepo)
		v.CodeCommit = firstNonEmpty(v.CodeCommit, run.CodeCommit)
		v.DatasetID = firstNonEmpty(v.DatasetID, run.DatasetID)
		v.DatasetName = firstNonEmpty(v.DatasetName, run.DatasetName)
		v.DatasetVersion = firstNonEmpty(v.DatasetVersion, run.DatasetVersion)
		}
	}
	if err := s.models.CreateModelVersion(v); err != nil {
		log.Printf("[Training] model registration failed for job %s: %v", job.ID, err)
		return fmt.Sprintf("model registration failed: %v", err)
	}
	job.RegisteredVersionID = v.ID
	return ""
}

func (s *Service) registerAdapter(job *models.Job, agg *trainingdomain.TrainingJob) string {
	if s.adapters == nil || !job.RegisterAdapterEnabled {
		return ""
	}
	if job.RegisteredAdapterID != "" {
		return ""
	}
	
	if agg.LatestCheckpoint == nil || strings.TrimSpace(agg.LatestCheckpoint.URI) == "" {
		return "adapter registration skipped: no checkpoint artifact"
	}
	if strings.TrimSpace(job.AdapterBaseModel) == "" {
		return "adapter registration skipped: base model required"
	}
	if job.AdapterMethod != ports.MethodLoRA && job.AdapterMethod != ports.MethodQLoRA {
		return "adapter registration skipped: invalid adapter method"
	}

	adapter := &ports.FineTuneAdapter{
		Name:       job.Name,
		BaseModel:  strings.TrimSpace(job.AdapterBaseModel),
		Path:       agg.LatestCheckpoint.URI,
		Rank:       job.AdapterRank,
		Method:     job.AdapterMethod,
		SourceJobID: job.ID,
		TenantID:    job.TenantID,
		CreatedBy:   job.TenantID,
	}
	adapter.Normalize()
	if err := adapter.Validate(); err != nil {
		return fmt.Sprintf("adapter registration skipped: %v", err)
	}
	if err := s.adapters.Create(context.Background(), adapter); err != nil {
		
		log.Printf("[Training] adapter registration failed for job %s: %v", job.ID, err)
		return fmt.Sprintf("adapter registration failed: %v", err)
	}
	job.RegisteredAdapterID = adapter.ID
	return ""
}

type versionFile struct {
	Path     string `json:"path"`
	Hash     string `json:"hash,omitempty"`
	SizeBytes int64 `json:"size_bytes,omitempty"`
}

func buildVersionFileManifest(ck *trainingdomain.Checkpoint) string {
	if ck == nil || strings.TrimSpace(ck.URI) == "" {
		return ""
	}
	manifest := []versionFile{{
		Path:      ck.URI,
		Hash:      ck.Hash,
		SizeBytes: ck.SizeBytes,
	}}
	b, err := json.Marshal(manifest)
	if err != nil {
		
		log.Printf("[Training] failed to marshal version file manifest: %v", err)
		return ""
	}
	return string(b)
}

func (s *Service) load(jobID string) (*models.Job, *trainingdomain.TrainingJob, error) {
	job, err := s.jobs.GetJob(jobID)
	if err != nil || job == nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrNotFound, jobID)
	}
	if job.Type != models.JobTypeTraining {
		return nil, nil, fmt.Errorf("%w: %s is not a training job", ErrNotFound, jobID)
	}
	return job, adapter.TrainingFromModel(job), nil
}

func (s *Service) releaseQuota(job *models.Job) {
	if job.TenantID == "" {
		return
	}
	spec := adapter.TrainingFromModel(job).Spec
	if err := s.quota.Release(job.TenantID, spec.TotalGPUs(), spec.TotalMemory(), 1); err != nil {
		log.Printf("[Training] release quota for job %s: %v", job.ID, err)
	}
}

func truncateReason(reason string) string {
	if len(reason) <= maxFailureReasonLen {
		return reason
	}
	return reason[:maxFailureReasonLen] + "..."
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}