package storage

import (
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
)

type studyRepo struct {
	db *gorm.DB
}

func NewStudyRepository(db *gorm.DB) ports.StudyRepository {
	return &studyRepo{db: db}
}

type trialRepo struct {
	db *gorm.DB
}

func NewTrialRepository(db *gorm.DB) ports.TrialRepository {
	return &trialRepo{db: db}
}

func (r *studyRepo) CreateStudy(s *models.HPOStudy) error {
	return r.db.Create(s).Error
}

func (r *studyRepo) GetStudy(id string) (*models.HPOStudy, error) {
	var s models.HPOStudy
	if err := r.db.Where("id = ?", id).First(&s).Error; err != nil {
		if isNotFound(err) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

func (r *studyRepo) UpdateStudy(s *models.HPOStudy) error {
	return r.db.Save(s).Error
}

func (r *studyRepo) ListStudies(tenantID string) ([]models.HPOStudy, error) {
	var list []models.HPOStudy
	if err := r.db.Where("tenant_id = ?", tenantID).Order("created_at desc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *studyRepo) DeleteStudy(id string) error {
	return r.db.Delete(&models.HPOStudy{}, "id = ?", id).Error
}

func (r *trialRepo) CreateTrial(t *models.HPOTrial) error {
	return r.db.Create(t).Error
}

func (r *trialRepo) GetTrial(id string) (*models.HPOTrial, error) {
	var t models.HPOTrial
	if err := r.db.Where("id = ?", id).First(&t).Error; err != nil {
		if isNotFound(err) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *trialRepo) UpdateTrial(t *models.HPOTrial) error {
	return r.db.Save(t).Error
}

func (r *trialRepo) ListTrials(studyID string) ([]models.HPOTrial, error) {
	var list []models.HPOTrial
	if err := r.db.Where("study_id = ?", studyID).Order("number asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *trialRepo) ListRunningTrials(studyID string) ([]models.HPOTrial, error) {
	var list []models.HPOTrial
	if err := r.db.Where("study_id = ?", studyID).Where("status = ?", "running").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *trialRepo) GetTrialByJobID(jobID string) (*models.HPOTrial, error) {
	var t models.HPOTrial
	if err := r.db.Where("job_id = ?", jobID).First(&t).Error; err != nil {
		if isNotFound(err) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *trialRepo) DeleteTrialsByStudy(studyID string) error {
	if err := r.db.Delete(&models.HPOTrialReport{}, "study_id = ?", studyID).Error; err != nil {
		return err
	}
	return r.db.Delete(&models.HPOTrial{}, "study_id = ?", studyID).Error
}

type reportRepo struct {
	db *gorm.DB
}

func NewReportRepository(db *gorm.DB) ports.ReportRepository {
	return &reportRepo{db: db}
}

func (r *reportRepo) CreateReport(rep *models.HPOTrialReport) error {
	return r.db.Create(rep).Error
}