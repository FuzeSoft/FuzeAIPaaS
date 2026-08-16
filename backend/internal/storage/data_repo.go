package storage

import (
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
)

type dataRepo struct {
	db *gorm.DB
}

var _ ports.DataRepository = (*dataRepo)(nil)

func newDataRepo(db *gorm.DB) *dataRepo { return &dataRepo{db: db} }

func (r *dataRepo) CreatePipeline(p *models.DataPipeline) error {
	if p.Status == "" {
		p.Status = models.PipelineStatusDraft
	}
	now := now()
	p.CreatedAt, p.UpdatedAt = now, now
	return r.db.Create(p).Error
}

func (r *dataRepo) GetPipeline(id string) (*models.DataPipeline, error) {
	var p models.DataPipeline
	if err := r.db.First(&p, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *dataRepo) ListPipelines(tenantID string) ([]models.DataPipeline, error) {
	var ps []models.DataPipeline
	err := r.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&ps).Error
	return ps, err
}

func (r *dataRepo) ListActivePipelines() ([]models.DataPipeline, error) {
	var ps []models.DataPipeline
	err := r.db.Where("status NOT IN ?", []string{
		string(models.PipelineStatusCompleted),
		string(models.PipelineStatusFailed),
		string(models.PipelineStatusCancelled),
	}).Order("created_at ASC").Find(&ps).Error
	return ps, err
}

func (r *dataRepo) UpdatePipeline(p *models.DataPipeline) error {
	p.UpdatedAt = now()
	return r.db.Model(&models.DataPipeline{}).Where("id = ?", p.ID).
		Select("Status", "Priority", "QueueName", "ClusterID", "Description", "UpdatedAt").
		Updates(p).Error
}

func (r *dataRepo) CreateStep(st *models.PipelineStep) error {
	if st.Status == "" {
		st.Status = models.StepStatusPending
	}
	now := now()
	st.CreatedAt, st.UpdatedAt = now, now
	return r.db.Create(st).Error
}

func (r *dataRepo) GetSteps(pipelineID string) ([]models.PipelineStep, error) {
	var steps []models.PipelineStep
	err := r.db.Where("pipeline_id = ?", pipelineID).Order("created_at ASC").Find(&steps).Error
	return steps, err
}

func (r *dataRepo) UpdateStep(st *models.PipelineStep) error {
	st.UpdatedAt = now()
	return r.db.Model(&models.PipelineStep{}).Where("id = ?", st.ID).
		Select("Status", "JobID", "FailureReason", "UpdatedAt").
		Updates(st).Error
}

func (r *dataRepo) CreateStepRun(run *models.PipelineStepRun) error {
	now := now()
	run.CreatedAt, run.UpdatedAt = now, now
	return r.db.Create(run).Error
}

func (r *dataRepo) UpdateStepRun(run *models.PipelineStepRun) error {
	run.UpdatedAt = now()
	return r.db.Model(&models.PipelineStepRun{}).Where("id = ?", run.ID).
		Select("Status", "VolcanoJobName", "Attempts", "LogRef", "StartedAt", "FinishedAt", "UpdatedAt").
		Updates(run).Error
}

func (r *dataRepo) GetStepRuns(stepID string) ([]models.PipelineStepRun, error) {
	var runs []models.PipelineStepRun
	err := r.db.Where("step_id = ?", stepID).Order("created_at ASC").Find(&runs).Error
	return runs, err
}

func (r *dataRepo) CreateAnnotation(a *models.AnnotationTask) error {
	if a.Status == "" {
		a.Status = models.AnnotationStatusOpen
	}
	now := now()
	a.CreatedAt, a.UpdatedAt = now, now
	return r.db.Create(a).Error
}

func (r *dataRepo) GetAnnotation(id string) (*models.AnnotationTask, error) {
	var a models.AnnotationTask
	if err := r.db.First(&a, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *dataRepo) ListAnnotations(tenantID string) ([]models.AnnotationTask, error) {
	var as []models.AnnotationTask
	err := r.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&as).Error
	return as, err
}

func (r *dataRepo) UpdateAnnotation(a *models.AnnotationTask) error {
	a.UpdatedAt = now()
	return r.db.Model(&models.AnnotationTask{}).Where("id = ?", a.ID).
		Select("Status", "Progress", "Assignee", "ExportedURI", "UpdatedAt").
		Updates(a).Error
}