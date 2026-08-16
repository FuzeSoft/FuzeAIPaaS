
package hpo

import (
	"encoding/json"
	"fmt"
	"time"

	"fuze-ai-paas/backend/internal/domain/hpo"
	"fuze-ai-paas/backend/internal/models"
)

func toDomainStudy(m *models.HPOStudy) (*hpo.Study, error) {
	s := &hpo.Study{
		ID:           m.ID,
		TenantID:     m.TenantID,
		ExperimentID: m.ExperimentID,
		Name:         m.Name,
		Algorithm:    m.Algorithm,
		MaxTrials:    m.MaxTrials,
		MaxParallel:  m.MaxParallel,
		MaxDuration:  time.Duration(m.MaxDurationSec) * time.Second,
		Status:       m.Status,
		BestTrialID:  m.BestTrialID,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
	if err := json.Unmarshal([]byte(m.ObjectiveJSON), &s.Objective); err != nil {
		return nil, fmt.Errorf("unmarshal objective: %w", err)
	}
	if err := json.Unmarshal([]byte(m.SpaceJSON), &s.Space); err != nil {
		return nil, fmt.Errorf("unmarshal space: %w", err)
	}
	if m.EarlyStopJSON != "" {
		es := &hpo.EarlyStopSpec{}
		if err := json.Unmarshal([]byte(m.EarlyStopJSON), es); err != nil {
			return nil, fmt.Errorf("unmarshal early stop: %w", err)
		}
		s.EarlyStop = es
	}
	if m.TrainingTemplateJSON != "" {
		tmpl := map[string]any{}
		if err := json.Unmarshal([]byte(m.TrainingTemplateJSON), &tmpl); err != nil {
			return nil, fmt.Errorf("unmarshal training template: %w", err)
		}
		s.TrainingTemplate = tmpl
	}
	return s, nil
}

func toModelStudy(s *hpo.Study) (*models.HPOStudy, error) {
	obj, err := json.Marshal(s.Objective)
	if err != nil {
		return nil, fmt.Errorf("marshal objective: %w", err)
	}
	space, err := json.Marshal(s.Space)
	if err != nil {
		return nil, fmt.Errorf("marshal space: %w", err)
	}
	m := &models.HPOStudy{
		ID:              s.ID,
		TenantID:        s.TenantID,
		ExperimentID:    s.ExperimentID,
		Name:            s.Name,
		Algorithm:       s.Algorithm,
		ObjectiveJSON:   string(obj),
		SpaceJSON:       string(space),
		MaxTrials:       s.MaxTrials,
		MaxParallel:     s.MaxParallel,
		MaxDurationSec:  int(s.MaxDuration.Seconds()),
		Status:          s.Status,
		BestTrialID:     s.BestTrialID,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
	}
	if s.EarlyStop != nil {
		es, err := json.Marshal(s.EarlyStop)
		if err != nil {
			return nil, fmt.Errorf("marshal early stop: %w", err)
		}
		m.EarlyStopJSON = string(es)
	}
	if s.TrainingTemplate != nil {
		tmpl, err := json.Marshal(s.TrainingTemplate)
		if err != nil {
			return nil, fmt.Errorf("marshal training template: %w", err)
		}
		m.TrainingTemplateJSON = string(tmpl)
	}
	return m, nil
}

func toDomainTrial(m *models.HPOTrial) (*hpo.Trial, error) {
	t := &hpo.Trial{
		ID:      m.ID,
		StudyID: m.StudyID,
		Number:  m.Number,
		Status:  m.Status,
		Value:   m.Value,
		RunID:   m.RunID,
		JobID:   m.JobID,
	}
	if err := json.Unmarshal([]byte(m.ParamsJSON), &t.Params); err != nil {
		return nil, fmt.Errorf("unmarshal params: %w", err)
	}
	if m.IntermediateJSON != "" {
		if err := json.Unmarshal([]byte(m.IntermediateJSON), &t.Intermediate); err != nil {
			return nil, fmt.Errorf("unmarshal intermediate: %w", err)
		}
	}
	return t, nil
}

func toModelTrial(t *hpo.Trial) (*models.HPOTrial, error) {
	params, err := json.Marshal(t.Params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}
	m := &models.HPOTrial{
		ID:         t.ID,
		StudyID:    t.StudyID,
		Number:     t.Number,
		ParamsJSON: string(params),
		Status:     t.Status,
		Value:      t.Value,
		RunID:      t.RunID,
		JobID:      t.JobID,
	}
	if t.Intermediate != nil {
		inter, err := json.Marshal(t.Intermediate)
		if err != nil {
			return nil, fmt.Errorf("marshal intermediate: %w", err)
		}
		m.IntermediateJSON = string(inter)
	}
	return m, nil
}