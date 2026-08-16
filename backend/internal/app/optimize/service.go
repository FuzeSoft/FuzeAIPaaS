
package optimizeapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"fuze-ai-paas/backend/internal/domain/optimize"
)

const defaultGateThreshold = 0.01

type Service struct {
	repo     optimize.CompressionRepository
	executor optimize.CompressionExecutor
}

func NewService(repo optimize.CompressionRepository, executor optimize.CompressionExecutor) *Service {
	return &Service{repo: repo, executor: executor}
}

type CreateInput struct {
	Name           string
	TenantID       string
	Type           optimize.CompressionType
	Backend        optimize.CompressionBackend
	ConfigJSON     string
	ModelVersionID string
	
	GateThreshold float64
	
	OrigAccuracy float64
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*optimize.CompressionTask, error) {
	if in.Name == "" {
		return nil, errors.New("compression task name is required")
	}
	if in.ModelVersionID == "" {
		return nil, errors.New("compression task must reference a model version")
	}
	
	if _, err := optimize.ParseConfig(in.Type, in.ConfigJSON); err != nil {
		return nil, errors.New("invalid config: " + err.Error())
	}
	
	if _, err := optimize.SelectOptimizer(in.Type, in.Backend); err != nil {
		return nil, errors.New("unsupported (type, backend): " + err.Error())
	}

	thr := in.GateThreshold
	if thr <= 0 {
		thr = defaultGateThreshold
	}

	task := optimize.NewCompressionTask(generateOptID(), in.TenantID, in.Name, in.Type, in.Backend, in.ConfigJSON, in.ModelVersionID)

	withGate(task, thr, in.OrigAccuracy)
	if err := s.repo.Create(ctx, task); err != nil {
		return nil, err
	}
	
	jobID, err := s.executor.Submit(task)
	if err != nil {
		
		_ = task.TransitionTo(optimize.StatusFailed)
		task.JobID = jobID
		task.FailReason = "submit failed: " + err.Error()
		_ = s.repo.Update(ctx, task)
		return nil, errors.New("submit compression job: " + err.Error())
	}
	if err := task.TransitionTo(optimize.StatusRunning); err != nil {
		return nil, err
	}
	task.JobID = jobID
	if err := s.repo.Update(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *Service) Get(ctx context.Context, id string) (*optimize.CompressionTask, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, tenantID string) ([]*optimize.CompressionTask, error) {
	return s.repo.ListByTenant(ctx, tenantID)
}

func (s *Service) Cancel(ctx context.Context, id string) error {
	task, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if task.Status == optimize.StatusSucceeded || task.Status == optimize.StatusFailed {
		return errors.New("task already terminal, cannot cancel")
	}
	if task.JobID != "" {
		if err := s.executor.Cancel(task.JobID); err != nil {
			return errors.New("cancel job: " + err.Error())
		}
	}
	if err := task.TransitionTo(optimize.StatusCancelled); err != nil {
		return err
	}
	return s.persist(ctx, task, nil)
}

func (s *Service) HandleResult(ctx context.Context, id string) error {
	task, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if task.JobID == "" {
		return errors.New("task has no running job")
	}
	res, err := s.executor.GetResult(task.JobID)
	if err != nil {
		return err 
	}
	
	verdict := optimize.EvaluateGate(task.OrigAccuracy, res.Accuracy, task.GateThreshold)
	if err := task.TransitionTo(optimize.StatusSucceeded); err != nil {
		return err
	}
	return s.persist(ctx, task, func(m *optimize.CompressionTask) {
		m.CompressedSizeBytes = res.CompressedSizeBytes
		m.LatencyMs = res.LatencyMs
		m.Accuracy = res.Accuracy
		m.ArtifactURI = res.ArtifactURI
		m.CompressionRatio = res.CompressionRatio
		m.Speedup = res.Speedup
		m.GatePass = verdict.Pass
		if !verdict.Pass {
			m.Status = optimize.StatusFailed
			m.FailReason = verdict.Reason
		}
	})
}

func (s *Service) Delete(ctx context.Context, id string) error {
	task, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if task.Status == optimize.StatusRunning && task.JobID != "" {
		_ = s.executor.Cancel(task.JobID) 
	}
	return s.repo.Delete(ctx, id)
}

func (s *Service) persist(ctx context.Context, task *optimize.CompressionTask, mutate func(*optimize.CompressionTask)) error {
	if mutate != nil {
		mutate(task)
	}
	return s.repo.Update(ctx, task)
}

func withGate(task *optimize.CompressionTask, thr, origAcc float64) {
	task.GateThreshold = thr
	task.OrigAccuracy = origAcc
}

func generateOptID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "opt-" + time.Now().Format("20060102150405")
	}
	return "opt-" + hex.EncodeToString(b)
}