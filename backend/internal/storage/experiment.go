package storage

import (
	"errors"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
)

type experimentRepo struct {
	db *gorm.DB
}

func NewExperimentRepository(db *gorm.DB) ports.ExperimentRepository {
	return &experimentRepo{db: db}
}

func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func (r *experimentRepo) GetExperiments(tenantID string) ([]models.Experiment, error) {
	var list []models.Experiment
	if err := r.db.Where("tenant_id = ?", tenantID).Order("created_at desc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *experimentRepo) GetExperiment(id string) (*models.Experiment, error) {
	var e models.Experiment
	if err := r.db.Where("id = ?", id).First(&e).Error; err != nil {
		if isNotFound(err) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return &e, nil
}

func (r *experimentRepo) CreateExperiment(e *models.Experiment) error {
	return r.db.Create(e).Error
}

func (r *experimentRepo) UpdateExperiment(e *models.Experiment) error {
	return r.db.Save(e).Error
}

func (r *experimentRepo) DeleteExperiment(id string) error {
	return r.db.Delete(&models.Experiment{}, "id = ?", id).Error
}

func (r *experimentRepo) GetRuns(experimentID string) ([]models.Run, error) {
	var list []models.Run
	if err := r.db.Where("experiment_id = ?", experimentID).Order("created_at asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *experimentRepo) GetRun(id string) (*models.Run, error) {
	var run models.Run
	if err := r.db.Where("id = ?", id).First(&run).Error; err != nil {
		if isNotFound(err) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return &run, nil
}

func (r *experimentRepo) GetRunByJobID(jobID string) (*models.Run, error) {
	if jobID == "" {
		return nil, ports.ErrNotFound
	}
	var run models.Run
	if err := r.db.Where("source_job_id = ?", jobID).Order("created_at desc").First(&run).Error; err != nil {
		if isNotFound(err) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return &run, nil
}

func (r *experimentRepo) CreateRun(run *models.Run) error {
	return r.db.Create(run).Error
}

func (r *experimentRepo) UpdateRun(run *models.Run) error {
	return r.db.Save(run).Error
}

func (r *experimentRepo) DeleteRun(id string) error {
	return r.db.Delete(&models.Run{}, "id = ?", id).Error
}