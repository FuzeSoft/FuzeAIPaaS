package models

import "time"

type HPOStudy struct {
	ID                 string `gorm:"primaryKey" json:"id"`
	TenantID           string `gorm:"index" json:"tenant_id"`
	ExperimentID       string `gorm:"index" json:"experiment_id"`
	Name               string `json:"name"`
	Algorithm          string `json:"algorithm"`
	ObjectiveJSON      string `json:"objective_json"`        
	SpaceJSON          string `json:"space_json"`            
	MaxTrials          int    `json:"max_trials"`
	MaxParallel        int    `json:"max_parallel"`
	MaxDurationSec     int    `json:"max_duration_sec"`      
	EarlyStopJSON      string `json:"early_stop_json"`       
	TrainingTemplateJSON string `json:"training_template_json"` 
	Status             string `json:"status"`
	BestTrialID        string `json:"best_trial_id,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type HPOTrial struct {
	ID               string `gorm:"primaryKey" json:"id"`
	StudyID          string `gorm:"index" json:"study_id"`
	Number           int    `json:"number"`
	ParamsJSON       string `json:"params_json"`        
	Status           string `json:"status"`
	Value            *float64 `gorm:"column:value" json:"value,omitempty"`
	IntermediateJSON string `json:"intermediate_json"` 
	RunID            string `gorm:"index" json:"run_id,omitempty"`
	JobID            string `json:"job_id,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type HPOTrialReport struct {
	ID        string `gorm:"primaryKey" json:"id"`
	TrialID   string `gorm:"index" json:"trial_id"`
	StudyID   string `gorm:"index" json:"study_id"`
	Step      int    `json:"step"`
	Value     float64 `json:"value"`
	CreatedAt time.Time `json:"createdAt"`
}

func (HPOStudy) TableName() string        { return "hpo_studies" }
func (HPOTrial) TableName() string        { return "hpo_trials" }
func (HPOTrialReport) TableName() string  { return "hpo_trial_reports" }