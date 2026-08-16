package ports

import "fuze-ai-paas/backend/internal/models"

type StudyRepository interface {
	
	CreateStudy(s *models.HPOStudy) error
	
	GetStudy(id string) (*models.HPOStudy, error)
	
	UpdateStudy(s *models.HPOStudy) error
	
	ListStudies(tenantID string) ([]models.HPOStudy, error)
	
	DeleteStudy(id string) error
}

type TrialRepository interface {
	
	CreateTrial(t *models.HPOTrial) error
	
	GetTrial(id string) (*models.HPOTrial, error)
	
	UpdateTrial(t *models.HPOTrial) error
	
	ListTrials(studyID string) ([]models.HPOTrial, error)
	
	ListRunningTrials(studyID string) ([]models.HPOTrial, error)
	
	GetTrialByJobID(jobID string) (*models.HPOTrial, error)
	
	DeleteTrialsByStudy(studyID string) error
}

type ReportRepository interface {
	
	CreateReport(r *models.HPOTrialReport) error
}