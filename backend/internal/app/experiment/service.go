
package experiment

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"fuze-ai-paas/backend/internal/domain/experiment"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

var (
	ErrNotFound    = errors.New("experiment not found")
	ErrInvalidSpec = errors.New("invalid experiment spec")
	ErrInvalidState = errors.New("invalid experiment state")
)

type Service struct {
	experiments ports.ExperimentRepository
}

func NewService(repo ports.ExperimentRepository) *Service {
	return &Service{experiments: repo}
}

type ExperimentInput struct {
	TenantID    string
	Name        string
	Description string
	Objective   string 
	MetricName  string
	Tags        []string
}

func (s *Service) CreateExperiment(in ExperimentInput) (*models.Experiment, error) {
	agg := &experiment.Experiment{
		ID:          generateID("exp"),
		TenantID:    in.TenantID,
		Name:        in.Name,
		Description: in.Description,
		Objective:   in.Objective,
		MetricName:  in.MetricName,
		Tags:        in.Tags,
		Status:      experiment.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	agg.Normalize()
	if err := agg.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSpec, err)
	}
	e := toExperimentModel(agg)
	if err := s.experiments.CreateExperiment(e); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Service) ListExperiments(tenantID string) ([]models.Experiment, error) {
	return s.experiments.GetExperiments(tenantID)
}

func (s *Service) GetExperiment(id string) (*models.Experiment, error) {
	e, err := s.experiments.GetExperiment(id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return e, nil
}

func (s *Service) ArchiveExperiment(id string) (*models.Experiment, error) {
	e, err := s.experiments.GetExperiment(id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	agg := fromExperimentModel(e)
	if err := agg.Archive(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}
	updated := toExperimentModel(agg)
	if err := s.experiments.UpdateExperiment(updated); err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) DeleteExperiment(id string) error {
	runs, err := s.experiments.GetRuns(id)
	if err != nil {
		return err
	}
	for i := range runs {
		if err := s.experiments.DeleteRun(runs[i].ID); err != nil {
			return err
		}
	}
	return s.experiments.DeleteExperiment(id)
}

type RunInput struct {
	TenantID        string
	ExperimentID    string
	Name            string
	Hyperparameters map[string]interface{}
	SourceJobID     string
}

func (s *Service) CreateRun(in RunInput) (*models.Run, error) {
	
	parent, err := s.experiments.GetExperiment(in.ExperimentID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, ErrInvalidSpec
		}
		return nil, err
	}
	if parent.TenantID != in.TenantID {
		return nil, ErrInvalidSpec
	}

	agg := &experiment.Run{
		ID:              generateID("run"),
		ExperimentID:    in.ExperimentID,
		TenantID:        in.TenantID,
		Name:            in.Name,
		Hyperparameters: in.Hyperparameters,
		SourceJobID:     in.SourceJobID,
		Status:          experiment.RunStatusPending,
	}
	agg.Normalize()
	if err := agg.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSpec, err)
	}
	r := toRunModel(agg)
	if err := s.experiments.CreateRun(r); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Service) CreateReproductionRun(parent *models.Run, newJobID string) (*models.Run, error) {
	parentAgg := &experiment.Run{
		ID:              parent.ID,
		ExperimentID:    parent.ExperimentID,
		TenantID:        parent.TenantID,
		Name:            parent.Name,
		Hyperparameters: fromRunModel(parent).Hyperparameters,
		SourceJobID:     parent.SourceJobID,
	}
	reproAgg := experiment.NewReproductionRun(parentAgg, newJobID, parent.Name+"-repro")
	reproAgg.Normalize()
	if err := reproAgg.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSpec, err)
	}
	r := toRunModel(reproAgg)
	if err := s.experiments.CreateRun(r); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Service) ListRuns(experimentID string) ([]models.Run, error) {
	return s.experiments.GetRuns(experimentID)
}

func (s *Service) GetRun(id string) (*models.Run, error) {
	r, err := s.experiments.GetRun(id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return r, nil
}

func (s *Service) GetRunByJobID(jobID string) (*models.Run, error) {
	r, err := s.experiments.GetRunByJobID(jobID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return r, nil
}

func (s *Service) UpdateRun(r *models.Run) error {
	return s.experiments.UpdateRun(r)
}

func (s *Service) CompleteRun(id string, value *float64, metrics map[string]interface{}, artifactURI string) (*models.Run, error) {
	r, err := s.experiments.GetRun(id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	agg := fromRunModel(r)
	now := time.Now()
	if !agg.Complete(value, artifactURI, now) {
		return nil, fmt.Errorf("%w: run already terminal", ErrInvalidState)
	}
	r.MetricValue = agg.MetricValue
	r.ArtifactURI = agg.ArtifactURI
	r.Status = agg.Status
	r.EndedAt = agg.EndedAt
	if metrics != nil {
		b, _ := json.Marshal(metrics)
		r.Metrics = string(b)
	}
	if err := s.experiments.UpdateRun(r); err != nil {
		return nil, err
	}

	if err := s.refreshBestRun(r.ExperimentID); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Service) FailRun(id string) (*models.Run, error) {
	r, err := s.experiments.GetRun(id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	agg := fromRunModel(r)
	if !agg.MarkFailed(time.Now()) {
		return nil, fmt.Errorf("%w: run already terminal", ErrInvalidState)
	}
	return s.saveRunStatus(r, agg)
}

func (s *Service) CancelRun(id string) (*models.Run, error) {
	r, err := s.experiments.GetRun(id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	agg := fromRunModel(r)
	if !agg.MarkCancelled(time.Now()) {
		return nil, fmt.Errorf("%w: run already terminal", ErrInvalidState)
	}
	return s.saveRunStatus(r, agg)
}

func (s *Service) refreshBestRun(experimentID string) error {
	exp, err := s.experiments.GetExperiment(experimentID)
	if err != nil {
		return err
	}
	runs, err := s.experiments.GetRuns(experimentID)
	if err != nil {
		return err
	}
	agg := fromExperimentModel(exp)

	var currentBest *experiment.Run
	for i := range runs {
		if runs[i].ID == exp.BestRunID {
			currentBest = fromRunModel(&runs[i])
		}
	}
	for i := range runs {
		cand := fromRunModel(&runs[i])
		if agg.UpdateBest(cand, currentBest) {
			currentBest = cand
		}
	}

	newBestID := ""
	if currentBest != nil {
		newBestID = currentBest.ID
	}
	exp.BestRunID = newBestID
	if err := s.experiments.UpdateExperiment(exp); err != nil {
		return err
	}
	for i := range runs {
		isBest := runs[i].ID == newBestID
		if runs[i].IsBest != isBest {
			runs[i].IsBest = isBest
			if err := s.experiments.UpdateRun(&runs[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) saveRunStatus(r *models.Run, agg *experiment.Run) (*models.Run, error) {
	r.Status = agg.Status
	r.EndedAt = agg.EndedAt
	if err := s.experiments.UpdateRun(r); err != nil {
		return nil, err
	}
	return r, nil
}

func toExperimentModel(a *experiment.Experiment) *models.Experiment {
	return &models.Experiment{
		ID:          a.ID,
		TenantID:    a.TenantID,
		Name:        a.Name,
		Description: a.Description,
		Objective:   a.Objective,
		MetricName:  a.MetricName,
		Tags:        joinTags(a.Tags),
		BestRunID:   a.BestRunID,
		Status:      a.Status,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

func fromExperimentModel(e *models.Experiment) *experiment.Experiment {
	return &experiment.Experiment{
		ID:          e.ID,
		TenantID:    e.TenantID,
		Name:        e.Name,
		Description: e.Description,
		Objective:   e.Objective,
		MetricName:  e.MetricName,
		Tags:        splitTags(e.Tags),
		BestRunID:   e.BestRunID,
		Status:      e.Status,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

func toRunModel(a *experiment.Run) *models.Run {
	var hp string
	if a.Hyperparameters != nil {
		b, _ := json.Marshal(a.Hyperparameters)
		hp = string(b)
	}
	return &models.Run{
		ID:              a.ID,
		ExperimentID:    a.ExperimentID,
		TenantID:        a.TenantID,
		Name:            a.Name,
		Hyperparameters: hp,
		SourceJobID:     a.SourceJobID,
		Status:          a.Status,
		ParentRunID:     a.ParentRunID,
		ReproductionState: a.ReproductionState,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

func fromRunModel(r *models.Run) *experiment.Run {
	var hp map[string]interface{}
	if r.Hyperparameters != "" {
		_ = json.Unmarshal([]byte(r.Hyperparameters), &hp)
	}
	return &experiment.Run{
		ID:              r.ID,
		ExperimentID:    r.ExperimentID,
		TenantID:        r.TenantID,
		Name:            r.Name,
		Hyperparameters: hp,
		MetricValue:     r.MetricValue,
		ArtifactURI:     r.ArtifactURI,
		SourceJobID:     r.SourceJobID,
		Status:          r.Status,
		IsBest:          r.IsBest,
		ParentRunID:     r.ParentRunID,
		ReproductionState: r.ReproductionState,
		StartedAt:       r.StartedAt,
		EndedAt:         r.EndedAt,
	}
}

func joinTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

func splitTags(s string) []string {
	if s == "" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(s), &tags); err != nil {
		return nil
	}
	return tags
}