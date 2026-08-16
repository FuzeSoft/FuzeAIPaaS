
package evaluationapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"fuze-ai-paas/backend/internal/domain/evaluation"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type Service struct {
	repos ports.EvaluationRepository
	judge ports.JudgeLLM
}

func NewService(repos ports.EvaluationRepository, judge ports.JudgeLLM) *Service {
	return &Service{repos: repos, judge: judge}
}

type CreateInput struct {
	Name         string
	Task         string
	Dataset      string
	ExperimentID string
	RunID        string
	ModelID      string
	Criteria     string
	TenantID     string
	CreatedBy    string
	
	JudgeMode string
	
	Dimensions string
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*models.Evaluation, error) {
	if in.Name == "" {
		return nil, errors.New("evaluation name is required")
	}
	
	if in.ExperimentID == "" && in.ModelID == "" {
		return nil, errors.New("evaluation must reference an experiment run or a model")
	}
	if in.ExperimentID != "" && in.RunID == "" {
		return nil, errors.New("experiment-linked evaluation requires run_id")
	}

	mode := in.JudgeMode
	if mode == "" {
		mode = evaluation.JudgeModeThreshold
	}
	
	switch mode {
	case evaluation.JudgeModeThreshold:
		if _, err := evaluation.ParseCriteria(in.Criteria); err != nil {
			return nil, errors.New("invalid criteria JSON: " + err.Error())
		}
	case evaluation.JudgeModeHuman, evaluation.JudgeModeLLM, evaluation.JudgeModeHybrid:
		dims, err := evaluation.ParseDimensions(in.Dimensions)
		if err != nil {
			return nil, errors.New("invalid dimensions JSON: " + err.Error())
		}
		if len(dims) == 0 {
			return nil, errors.New(mode + "-mode evaluation requires at least one dimension")
		}
	default:
		return nil, errors.New("unsupported judge_mode: " + mode)
	}

	agg := evaluation.New(evaluation.CreateInput{
		ID:           generateEvalID(),
		Name:         in.Name,
		Task:         in.Task,
		Dataset:      in.Dataset,
		ExperimentID: in.ExperimentID,
		RunID:        in.RunID,
		ModelID:      in.ModelID,
		Criteria:     in.Criteria,
		TenantID:     in.TenantID,
		CreatedBy:    in.CreatedBy,
		JudgeMode:    mode,
		Dimensions:   in.Dimensions,
	})
	m := evalToModel(agg)
	if err := s.repos.Create(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) RecordResult(ctx context.Context, id string, metrics map[string]float64, score float64) error {
	e, err := s.repos.Get(ctx, id)
	if err != nil {
		return err
	}
	agg := evalFromModel(e)
	if err := agg.RecordResult(metrics, score); err != nil {
		return err
	}
	return s.repos.Update(ctx, evalToModel(agg))
}

func (s *Service) Fail(ctx context.Context, id, reason string) error {
	e, err := s.repos.Get(ctx, id)
	if err != nil {
		return err
	}
	agg := evalFromModel(e)
	if err := agg.Fail(reason); err != nil {
		return err
	}
	return s.repos.Update(ctx, evalToModel(agg))
}

func (s *Service) Get(ctx context.Context, id string) (*models.Evaluation, error) {
	return s.repos.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, tenantID string) ([]models.Evaluation, error) {
	return s.repos.List(ctx, tenantID)
}

func (s *Service) ListByExperiment(ctx context.Context, experimentID string) ([]models.Evaluation, error) {
	return s.repos.ListByExperiment(ctx, experimentID)
}

func (s *Service) ListByModel(ctx context.Context, modelID string) ([]models.Evaluation, error) {
	return s.repos.ListByModel(ctx, modelID)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repos.Delete(ctx, id)
}

type ReviewInput struct {
	
	JudgeID string
	
	Scores string
	
	Comment string
}

func (s *Service) SubmitReview(ctx context.Context, evaluationID string, in ReviewInput) (*models.EvaluationReview, error) {
	e, err := s.repos.Get(ctx, evaluationID)
	if err != nil {
		return nil, err
	}
	agg := evalFromModel(e)

	rv := evaluation.Review{
		ID:           generateReviewID(),
		EvaluationID: evaluationID,
		TenantID:     e.TenantID,
		JudgeType:    evaluation.JudgeTypeHuman,
		JudgeID:      in.JudgeID,
		Scores:       in.Scores,
		Comment:      in.Comment,
	}
	if err := agg.AddReview(rv); err != nil {
		return nil, err
	}
	m := reviewToModel(rv)
	if err := s.repos.CreateReview(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) RunLLMJudge(ctx context.Context, evaluationID string, modelOutput, reference string) (*models.EvaluationReview, error) {
	if s.judge == nil {
		return nil, errors.New("llm judge not configured")
	}
	e, err := s.repos.Get(ctx, evaluationID)
	if err != nil {
		return nil, err
	}
	agg := evalFromModel(e)

	dims, err := evaluation.ParseDimensions(e.Dimensions)
	if err != nil {
		return nil, errors.New("parse dimensions: " + err.Error())
	}
	if len(dims) == 0 {
		return nil, errors.New("no dimensions defined for llm judge")
	}

	portDims := make([]ports.JudgeDimension, 0, len(dims))
	for _, d := range dims {
		portDims = append(portDims, ports.JudgeDimension{
			Name:        d.Name,
			Weight:      d.Weight,
			Description: d.Description,
		})
	}
	resp, err := s.judge.Judge(ctx, ports.JudgeRequest{
		Task:        e.Task,
		Dataset:     e.Dataset,
		Dimensions:  portDims,
		ModelOutput: modelOutput,
		Reference:   reference,
	})
	if err != nil {
		return nil, errors.New("llm judge failed: " + err.Error())
	}
	
	for _, d := range dims {
		if _, ok := resp.Scores[d.Name]; !ok {
			resp.Scores[d.Name] = 0
		}
	}
	scoresJSON, err := json.Marshal(resp.Scores)
	if err != nil {
		return nil, errors.New("marshal llm scores: " + err.Error())
	}

	review := evaluation.Review{
		ID:           generateReviewID(),
		EvaluationID: evaluationID,
		TenantID:     e.TenantID,
		JudgeType:    evaluation.JudgeTypeLLM,
		JudgeID:      s.judge.Model(),
		Scores:       string(scoresJSON),
		Comment:      resp.Comment,
	}
	if err := agg.AddReview(review); err != nil {
		return nil, err
	}
	m := reviewToModel(review)
	if err := s.repos.CreateReview(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) FinalizeReport(ctx context.Context, evaluationID string) (*evaluation.Report, error) {
	e, err := s.repos.Get(ctx, evaluationID)
	if err != nil {
		return nil, err
	}
	agg := evalFromModel(e)

	reviewModels, err := s.repos.ListReviews(ctx, evaluationID)
	if err != nil {
		return nil, err
	}
	reviews := reviewsFromModel(reviewModels)
	report, err := agg.AggregateReport(reviews)
	if err != nil {
		return nil, err
	}
	if err := agg.Finalize(report); err != nil {
		return nil, err
	}
	if err := s.repos.Update(ctx, evalToModel(agg)); err != nil {
		return nil, err
	}
	return &report, nil
}

func (s *Service) GetReport(ctx context.Context, evaluationID string) (evaluation.Report, error) {
	e, err := s.repos.Get(ctx, evaluationID)
	if err != nil {
		return evaluation.Report{}, err
	}

	if e.Report != "" {
		var rep evaluation.Report
		if err := json.Unmarshal([]byte(e.Report), &rep); err == nil {
			return rep, nil
		}
		
	}

	agg := evalFromModel(e)
	reviewModels, err := s.repos.ListReviews(ctx, evaluationID)
	if err != nil {
		return evaluation.Report{}, err
	}
	reviews := reviewsFromModel(reviewModels)
	
	report, err := agg.AggregateReport(reviews)
	if err != nil {
		return evaluation.Report{
			EvaluationID: e.ID,
			JudgeMode:    e.JudgeMode,
			Status:       e.Status,
			Verdict:      evaluation.VerdictPending,
			Reviews:      reviews,
			GeneratedAt:  time.Now(),
		}, nil
	}
	return report, nil
}

func (s *Service) ListReviews(ctx context.Context, evaluationID string) ([]models.EvaluationReview, error) {
	return s.repos.ListReviews(ctx, evaluationID)
}

func generateEvalID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "eval-" + time.Now().Format("20060102150405")
	}
	return "eval-" + hex.EncodeToString(b)
}

func generateReviewID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "rev-" + time.Now().Format("20060102150405")
	}
	return "rev-" + hex.EncodeToString(b)
}

func evalToModel(a *evaluation.Evaluation) *models.Evaluation {
	return &models.Evaluation{
		ID:           a.ID,
		Name:         a.Name,
		Task:         a.Task,
		Dataset:      a.Dataset,
		ExperimentID: a.ExperimentID,
		RunID:        a.RunID,
		ModelID:      a.ModelID,
		Criteria:     a.Criteria,
		Metrics:      a.Metrics,
		Score:        a.Score,
		Passed:       a.Passed,
		Status:       a.Status,
		FailReason:   a.FailReason,
		TenantID:     a.TenantID,
		CreatedBy:    a.CreatedBy,
		JudgeMode:    a.JudgeMode,
		Dimensions:   a.Dimensions,
		Report:       a.Report,
		CreatedAt:    a.CreatedAt,
	}
}

func evalFromModel(e *models.Evaluation) *evaluation.Evaluation {
	return &evaluation.Evaluation{
		ID:           e.ID,
		Name:         e.Name,
		Task:         e.Task,
		Dataset:      e.Dataset,
		ExperimentID: e.ExperimentID,
		RunID:        e.RunID,
		ModelID:      e.ModelID,
		Criteria:     e.Criteria,
		Metrics:      e.Metrics,
		Score:        e.Score,
		Passed:       e.Passed,
		Status:       e.Status,
		FailReason:   e.FailReason,
		TenantID:     e.TenantID,
		CreatedBy:    e.CreatedBy,
		JudgeMode:    e.JudgeMode,
		Dimensions:   e.Dimensions,
		Report:       e.Report,
		CreatedAt:    e.CreatedAt,
	}
}

func reviewToModel(r evaluation.Review) *models.EvaluationReview {
	return &models.EvaluationReview{
		ID:           r.ID,
		EvaluationID: r.EvaluationID,
		TenantID:     r.TenantID,
		JudgeType:    r.JudgeType,
		JudgeID:      r.JudgeID,
		Scores:       r.Scores,
		Comment:      r.Comment,
		CreatedAt:    r.CreatedAt,
	}
}

func reviewFromModel(r models.EvaluationReview) evaluation.Review {
	return evaluation.Review{
		ID:           r.ID,
		EvaluationID: r.EvaluationID,
		TenantID:     r.TenantID,
		JudgeType:    r.JudgeType,
		JudgeID:      r.JudgeID,
		Scores:       r.Scores,
		Comment:      r.Comment,
		CreatedAt:    r.CreatedAt,
	}
}

func reviewsFromModel(rs []models.EvaluationReview) []evaluation.Review {
	out := make([]evaluation.Review, 0, len(rs))
	for i := range rs {
		out = append(out, reviewFromModel(rs[i]))
	}
	return out
}