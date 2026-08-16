
package data

import (
	"encoding/json"
	"fmt"

	"fuze-ai-paas/backend/internal/adapter"
	datadomain "fuze-ai-paas/backend/internal/domain/data"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type Service struct {
	dataRepo ports.DataRepository
	jobRepo  ports.JobWriter     
	artifact ports.ArtifactStore 
	
	exportRoot string
}

func NewService(dataRepo ports.DataRepository, jobRepo ports.JobWriter, artifact ports.ArtifactStore) *Service {
	return &Service{dataRepo: dataRepo, jobRepo: jobRepo, artifact: artifact}
}

func (s *Service) SetExportRoot(root string) { s.exportRoot = root }

func (s *Service) CreatePipeline(p *models.DataPipeline, steps []models.PipelineStep) error {
	if err := datadomain.ValidateDAG(adapter.StepsToDomain(steps)); err != nil {
		return fmt.Errorf("invalid DAG: %w", err)
	}
	for i := range steps {
		if err := datadomain.ValidateOperator(steps[i].Operator, steps[i].Params); err != nil {
			return fmt.Errorf("step %s: %w", steps[i].Name, err)
		}
		steps[i].PipelineID = p.ID
		steps[i].Status = models.StepStatusPending
	}
	if err := s.dataRepo.CreatePipeline(p); err != nil {
		return err
	}
	for i := range steps {
		if err := s.dataRepo.CreateStep(&steps[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) SubmitPipeline(pipelineID string) error {
	p, err := s.dataRepo.GetPipeline(pipelineID)
	if err != nil {
		return err
	}
	if p.Status.IsTerminal() {
		return fmt.Errorf("pipeline %s is in terminal state %s", pipelineID, p.Status)
	}
	steps, err := s.dataRepo.GetSteps(pipelineID)
	if err != nil {
		return err
	}
	ready := datadomain.ReadySteps(adapter.StepsToDomain(steps))
	if len(ready) == 0 {
		return fmt.Errorf("pipeline %s has no runnable step", pipelineID)
	}
	if err := s.dispatchSteps(p, adapter.StepsToModel(ready)); err != nil {
		return err
	}
	p.Status = models.PipelineStatusRunning
	return s.dataRepo.UpdatePipeline(p)
}

func (s *Service) dispatchSteps(p *models.DataPipeline, steps []models.PipelineStep) error {
	for _, st := range steps {
		if st.JobID != "" {
			continue 
		}
		job, err := buildStepJob(p, st)
		if err != nil {
			return err
		}
		if err := s.jobRepo.CreateJob(job); err != nil {
			return fmt.Errorf("dispatch step %s: %w", st.ID, err)
		}
		st.JobID = job.ID
		st.Status = models.StepStatusPending
		if err := s.dataRepo.UpdateStep(&st); err != nil {
			return err
		}
		
		run := &models.PipelineStepRun{
			ID:         "run-" + st.ID,
			StepID:     st.ID,
			PipelineID: st.PipelineID,
			Status:     models.StepStatusPending,
		}
		_ = s.dataRepo.CreateStepRun(run)
	}
	return nil
}

func (s *Service) SyncPipeline(pipelineID string) error {
	p, err := s.dataRepo.GetPipeline(pipelineID)
	if err != nil {
		return err
	}
	if p.Status.IsTerminal() {
		return nil
	}
	steps, err := s.dataRepo.GetSteps(pipelineID)
	if err != nil {
		return err
	}
	
	for i := range steps {
		st := &steps[i]
		if st.JobID == "" {
			continue
		}
		job, err := s.jobRepo.GetJob(st.JobID)
		if err != nil || job == nil {
			continue
		}
		newStatus := datadomain.StepStatusFromJobStatus(datadomain.JobStatus(job.Status))
		if newStatus != datadomain.StepStatus(st.Status) {
			st.Status = models.StepStatus(newStatus)
			if newStatus == datadomain.StepStatusFailed {
				st.FailureReason = job.FailureReason
			}
			if err := s.dataRepo.UpdateStep(st); err != nil {
				return err
			}
		}
	}
	
	reread, err := s.dataRepo.GetSteps(pipelineID)
	if err != nil {
		return err
	}
	ready := datadomain.ReadySteps(adapter.StepsToDomain(reread))
	if len(ready) > 0 {
		if err := s.dispatchSteps(p, adapter.StepsToModel(ready)); err != nil {
			return err
		}
	}
	
	finalSteps, err := s.dataRepo.GetSteps(pipelineID)
	if err != nil {
		return err
	}
	derived := datadomain.DerivePipelineStatus(adapter.StepsToDomain(finalSteps), datadomain.PipelineStatus(p.Status))
	if derived != datadomain.PipelineStatus(p.Status) {
		p.Status = models.PipelineStatus(derived)
		return s.dataRepo.UpdatePipeline(p)
	}
	return nil
}

func (s *Service) CancelPipeline(pipelineID string) error {
	p, err := s.dataRepo.GetPipeline(pipelineID)
	if err != nil {
		return err
	}
	if p.Status.IsTerminal() {
		return nil
	}
	steps, err := s.dataRepo.GetSteps(pipelineID)
	if err != nil {
		return err
	}
	for _, st := range steps {
		if st.JobID == "" {
			continue
		}
		job, err := s.jobRepo.GetJob(st.JobID)
		if err != nil || job == nil {
			continue
		}
		if !job.Status.IsTerminal() {
			job.Status = models.JobStatusCancelled
			if err := s.jobRepo.UpdateJobStatus(job); err != nil {
				return err
			}
		}
		if st.Status != models.StepStatusSucceeded {
			st.Status = models.StepStatusSkipped
			_ = s.dataRepo.UpdateStep(&st)
		}
	}
	p.Status = models.PipelineStatusCancelled
	return s.dataRepo.UpdatePipeline(p)
}

func (s *Service) GetPipeline(pipelineID string) (*models.DataPipeline, []models.PipelineStep, error) {
	p, err := s.dataRepo.GetPipeline(pipelineID)
	if err != nil {
		return nil, nil, err
	}
	steps, err := s.dataRepo.GetSteps(pipelineID)
	if err != nil {
		return nil, nil, err
	}
	return p, steps, nil
}

func (s *Service) ListPipelines(tenantID string) ([]models.DataPipeline, error) {
	return s.dataRepo.ListPipelines(tenantID)
}

func (s *Service) ListActivePipelines() ([]models.DataPipeline, error) {
	return s.dataRepo.ListActivePipelines()
}

func buildStepJob(p *models.DataPipeline, st models.PipelineStep) (*models.Job, error) {
	spec := datadomain.DataJobSpec{
		Operator: st.Operator,
		Params:   mustParseParams(st.Params),
		Input:    joinPath(p.MountPath, st.InputPath),
		Output:   joinPath(p.MountPath, st.OutputPath),
	}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	image := st.Image
	command := st.Command
	if image == "" {
		
		image = datadomain.OperatorImage(st.Operator, "")
		command = "data-operator"
	}
	job := &models.Job{
		Name:         fmt.Sprintf("%s-%s", p.Name, st.Name),
		Type:         adapter.StepKindToJobType(datadomain.StepKind(st.Kind)),
		Status:       models.JobStatusPending,
		Image:        image,
		Command:      command,
		GPUs:         st.GPUs,
		Memory:       st.Memory,
		DatasetName:  p.DatasetName,
		MountPath:    p.MountPath,
		TenantID:     p.TenantID,
		ClusterID:    p.ClusterID,
		QueueName:    p.QueueName, 
		DataSpecJSON: string(specJSON),
	}
	return job, nil
}

func mustParseParams(raw string) map[string]any {
	if raw == "" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]any{}
	}
	return m
}

func joinPath(mount, sub string) string {
	if sub == "" {
		return mount
	}
	
	if len(mount) > 0 && mount[len(mount)-1] == '/' {
		return mount + sub
	}
	return mount + "/" + sub
}