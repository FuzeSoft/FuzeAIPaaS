package hpo

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"fuze-ai-paas/backend/internal/domain/hpo"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	trainingapp "fuze-ai-paas/backend/internal/app/training"
)

type TrainingSubmitter interface {
	Submit(tenantID string, in trainingapp.SubmitInput) (*models.Job, error)
	Cancel(jobID string) (*models.Job, error)
	Complete(jobID string) error
}

type ExperimentTracker interface {
	GetExperiment(id string) (*models.Experiment, error)
	GetRun(id string) (*models.Run, error)
}

type QuotaChecker interface {
	GetQuota(tenantID string) (*models.Quota, error)
}

type Service struct {
	studies    ports.StudyRepository
	trials     ports.TrialRepository
	exp        ports.ExperimentRepository
	training   TrainingSubmitter
	quota      QuotaChecker
	reportRepo ports.ReportRepository 
	
	RandSeed int64
}

func NewService(
	studies ports.StudyRepository,
	trials ports.TrialRepository,
	exp ports.ExperimentRepository,
	training TrainingSubmitter,
	quota QuotaChecker,
) *Service {
	return &Service{
		studies:  studies,
		trials:   trials,
		exp:      exp,
		training: training,
		quota:    quota,
		RandSeed: time.Now().UnixNano(),
	}
}

func (s *Service) SetReportRepository(r ports.ReportRepository) {
	s.reportRepo = r
}

func (s *Service) CreateStudy(ctx context.Context, tenantID string, spec StudySpec) (*models.HPOStudy, error) {
	if err := spec.Space.Validate(); err != nil {
		return nil, fmt.Errorf("invalid search space: %w", err)
	}
	if spec.Objective.MetricName == "" {
		return nil, fmt.Errorf("objective metric name is required")
	}
	if spec.MaxTrials <= 0 {
		return nil, fmt.Errorf("max_trials must be > 0")
	}
	if spec.MaxParallel <= 0 {
		spec.MaxParallel = 1
	}
	domainStudy := &hpo.Study{
		ID:              newID("study"),
		TenantID:        tenantID,
		ExperimentID:    spec.ExperimentID,
		Name:            spec.Name,
		Space:           spec.Space,
		Algorithm:       spec.Algorithm,
		Objective:       spec.Objective,
		MaxTrials:       spec.MaxTrials,
		MaxParallel:     spec.MaxParallel,
		MaxDuration:     spec.MaxDuration,
		EarlyStop:       spec.EarlyStop,
		TrainingTemplate: spec.TrainingTemplate,
		Status:          hpo.StudyPending,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	
	if domainStudy.Algorithm == "" {
		domainStudy.Algorithm = "tpe"
	}
	m, err := toModelStudy(domainStudy)
	if err != nil {
		return nil, err
	}
	if err := s.studies.CreateStudy(m); err != nil {
		return nil, err
	}
	return m, nil
}

type StudySpec struct {
	ExperimentID    string
	Name            string
	Algorithm       string
	Objective       hpo.Objective
	Space           hpo.SearchSpace
	MaxTrials       int
	MaxParallel     int
	MaxDuration     time.Duration
	EarlyStop       *hpo.EarlyStopSpec
	TrainingTemplate map[string]any
}

func (s *Service) ListStudies(ctx context.Context, tenantID string) ([]models.HPOStudy, error) {
	return s.studies.ListStudies(tenantID)
}

func (s *Service) GetStudy(ctx context.Context, tenantID, id string) (*models.HPOStudy, error) {
	m, err := s.studies.GetStudy(id)
	if err != nil {
		return nil, err
	}
	if m.TenantID != tenantID {
		return nil, ports.ErrNotFound
	}
	return m, nil
}

func (s *Service) DeleteStudy(ctx context.Context, tenantID, id string) error {
	m, err := s.studies.GetStudy(id)
	if err != nil {
		return err
	}
	if m.TenantID != tenantID {
		return ports.ErrNotFound
	}
	if err := s.trials.DeleteTrialsByStudy(id); err != nil {
		return err
	}
	return s.studies.DeleteStudy(id)
}

func (s *Service) ListTrials(ctx context.Context, tenantID, studyID string) ([]models.HPOTrial, error) {
	if _, err := s.GetStudy(ctx, tenantID, studyID); err != nil {
		return nil, err
	}
	return s.trials.ListTrials(studyID)
}

func (s *Service) GetTrial(ctx context.Context, tenantID, studyID, trialID string) (*models.HPOTrial, error) {
	if _, err := s.GetStudy(ctx, tenantID, studyID); err != nil {
		return nil, err
	}
	return s.trials.GetTrial(trialID)
}

func (s *Service) RunOnce(ctx context.Context, tenantID, studyID string) error {
	m, err := s.studies.GetStudy(studyID)
	if err != nil {
		return err
	}
	if tenantID != "" && m.TenantID != tenantID {
		return ports.ErrNotFound 
	}
	study, err := toDomainStudy(m)
	if err != nil {
		return err
	}
	if !study.IsActive() {
		return nil 
	}

	trialModels, err := s.trials.ListTrials(studyID)
	if err != nil {
		return err
	}
	trials := make([]hpo.Trial, 0, len(trialModels))
	for i := range trialModels {
		dt, err := toDomainTrial(&trialModels[i])
		if err != nil {
			return err
		}
		trials = append(trials, *dt)
	}

	ctx2 := hpo.SchedulingContext{
		QuotaAvailable: s.quotaAvailable(study.TenantID),
		Now:            time.Now(),
		
		Rand: newRand(atomic.AddInt64(&s.RandSeed, 1)),
	}
	actions := hpo.NextActions(study, trials, ctx2)
	for _, act := range actions {
		if err := s.applyAction(ctx, study, act); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) OnJobTerminal(ctx context.Context, jobID, status string) error {
	trial, err := s.trials.GetTrialByJobID(jobID)
	if err != nil {
		
		if err == ports.ErrNotFound {
			return nil
		}
		return err
	}
	return s.OnRunCompleted(ctx, trial.ID, status, nil)
}

func (s *Service) applyAction(ctx context.Context, study *hpo.Study, act hpo.Action) error {
	switch act.Kind {
	case hpo.ActionSpawnTrial:
		return s.spawnTrial(ctx, study, act.Params)
	case hpo.ActionStopTrial:
		return s.stopTrial(ctx, study, act.TrialID)
	case hpo.ActionCompleteStudy:
		return s.completeStudy(ctx, study, act.Reason)
	default:
		return nil
	}
}