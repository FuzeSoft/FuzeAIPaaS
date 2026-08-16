package evaluationapp

import (
	"context"
	"testing"

	"fuze-ai-paas/backend/internal/domain/evaluation"
	"fuze-ai-paas/backend/internal/llmjudge"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type memRepo struct {
	byID     map[string]*models.Evaluation
	reviews  map[string][]models.EvaluationReview
}

func newMem() *memRepo {
	return &memRepo{byID: map[string]*models.Evaluation{}, reviews: map[string][]models.EvaluationReview{}}
}

func (m *memRepo) Create(_ context.Context, e *models.Evaluation) error {
	m.byID[e.ID] = e
	return nil
}
func (m *memRepo) Get(_ context.Context, id string) (*models.Evaluation, error) {
	if e, ok := m.byID[id]; ok {
		return e, nil
	}
	return nil, ports.ErrNotFound
}
func (m *memRepo) List(_ context.Context, tenantID string) ([]models.Evaluation, error) {
	out := make([]models.Evaluation, 0)
	for _, e := range m.byID {
		if tenantID == "" || e.TenantID == tenantID {
			out = append(out, *e)
		}
	}
	return out, nil
}
func (m *memRepo) ListByExperiment(_ context.Context, expID string) ([]models.Evaluation, error) {
	out := make([]models.Evaluation, 0)
	for _, e := range m.byID {
		if e.ExperimentID == expID {
			out = append(out, *e)
		}
	}
	return out, nil
}
func (m *memRepo) ListByModel(_ context.Context, modelID string) ([]models.Evaluation, error) {
	out := make([]models.Evaluation, 0)
	for _, e := range m.byID {
		if e.ModelID == modelID {
			out = append(out, *e)
		}
	}
	return out, nil
}
func (m *memRepo) Update(_ context.Context, e *models.Evaluation) error {
	m.byID[e.ID] = e
	return nil
}
func (m *memRepo) Delete(_ context.Context, id string) error {
	if _, ok := m.byID[id]; !ok {
		return ports.ErrNotFound
	}
	delete(m.byID, id)
	delete(m.reviews, id)
	return nil
}

func (m *memRepo) CreateReview(_ context.Context, r *models.EvaluationReview) error {
	m.reviews[r.EvaluationID] = append(m.reviews[r.EvaluationID], *r)
	return nil
}

func (m *memRepo) ListReviews(_ context.Context, evaluationID string) ([]models.EvaluationReview, error) {
	return m.reviews[evaluationID], nil
}

func TestServiceCreateAndRecord(t *testing.T) {
	svc := NewService(newMem(), nil)
	created, err := svc.Create(context.Background(), CreateInput{
		Name:         "acc-eval",
		TenantID:     "t-1",
		ExperimentID: "exp-1",
		RunID:        "run-1",
		Criteria:     `{"accuracy": {"op": ">=", "value": 0.8}}`,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected generated id")
	}
	
	if err := svc.RecordResult(context.Background(), created.ID, map[string]float64{"accuracy": 0.9}, 0.9); err != nil {
		t.Fatalf("record: %v", err)
	}
	got, _ := svc.Get(context.Background(), created.ID)
	if !got.Passed || got.Status != "completed" {
		t.Fatalf("expected passed+completed, got %+v", got)
	}
}

func TestServiceRecordMissingFails(t *testing.T) {
	svc := NewService(newMem(), nil)
	created, err := svc.Create(context.Background(), CreateInput{
		Name:     "x",
		TenantID: "t-1",
		ModelID:  "model-1",
		Criteria: `{"accuracy": {"op": ">=", "value": 0.8}}`,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.RecordResult(context.Background(), created.ID, map[string]float64{"other": 0.9}, 0.9); err != nil {
		t.Fatalf("record: %v", err)
	}
	got, err := svc.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Passed {
		t.Fatalf("expected not passed when metric absent")
	}
}

func TestServiceFail(t *testing.T) {
	svc := NewService(newMem(), nil)
	created, err := svc.Create(context.Background(), CreateInput{Name: "x", TenantID: "t-1", ModelID: "model-1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Fail(context.Background(), created.ID, "boom"); err != nil {
		t.Fatalf("fail: %v", err)
	}
	got, err := svc.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "failed" || got.Passed {
		t.Fatalf("expected failed+not passed, got %+v", got)
	}
}

func TestServiceCreateRequiresReference(t *testing.T) {
	svc := NewService(newMem(), nil)
	if _, err := svc.Create(context.Background(), CreateInput{Name: "x", TenantID: "t-1"}); err == nil {
		t.Fatalf("expected error when neither experiment nor model is referenced")
	}
	if _, err := svc.Create(context.Background(), CreateInput{Name: "x", TenantID: "t-1", ExperimentID: "exp-1"}); err == nil {
		t.Fatalf("expected error when experiment is referenced without run_id")
	}
}

func TestServiceDeleteMissing(t *testing.T) {
	svc := NewService(newMem(), nil)
	if err := svc.Delete(context.Background(), "nope"); err != ports.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

type fakeJudge struct {
	scores map[string]float64
	model  string
}

func (f *fakeJudge) Judge(_ context.Context, _ ports.JudgeRequest) (ports.JudgeResponse, error) {
	return ports.JudgeResponse{Scores: f.scores, Overall: 0.8, Comment: "fake"}, nil
}
func (f *fakeJudge) Model() string { return f.model }

func createJudgeEval(t *testing.T, svc *Service, mode string) *models.Evaluation {
	t.Helper()
	created, err := svc.Create(context.Background(), CreateInput{
		Name:       "judge-eval",
		Task:       "qa",
		TenantID:   "t-1",
		ModelID:    "model-1",
		JudgeMode:  mode,
		Dimensions: `[{"name":"accuracy","weight":0.6},{"name":"fluency","weight":0.4}]`,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return created
}

func TestService_SubmitReview_HumanMode(t *testing.T) {
	svc := NewService(newMem(), nil)
	created := createJudgeEval(t, svc, evaluation.JudgeModeHuman)

	rv, err := svc.SubmitReview(context.Background(), created.ID, ReviewInput{
		JudgeID: "user-1",
		Scores:  `{"accuracy":0.9,"fluency":0.8}`,
		Comment: "good",
	})
	if err != nil {
		t.Fatalf("submit review: %v", err)
	}
	if rv.JudgeType != evaluation.JudgeTypeHuman {
		t.Fatalf("expected judge_type=human, got %s", rv.JudgeType)
	}
	if rv.JudgeID != "user-1" {
		t.Fatalf("expected judge_id=user-1, got %s", rv.JudgeID)
	}

	list, _ := svc.ListReviews(context.Background(), created.ID)
	if len(list) != 1 {
		t.Fatalf("expected 1 review, got %d", len(list))
	}
}

func TestService_SubmitReview_ThresholdRejected(t *testing.T) {
	svc := NewService(newMem(), nil)
	created, _ := svc.Create(context.Background(), CreateInput{
		Name:     "thr",
		TenantID: "t-1",
		ModelID:  "m-1",
		Criteria: `{"accuracy":{"op":">=","value":0.8}}`,
	})
	if _, err := svc.SubmitReview(context.Background(), created.ID, ReviewInput{
		JudgeID: "u-1",
		Scores:  `{"accuracy":0.9}`,
	}); err == nil {
		t.Fatalf("threshold mode should reject reviews")
	}
}

func TestServiceRunLLMJudge(t *testing.T) {
	judge := &fakeJudge{
		scores: map[string]float64{"accuracy": 0.85, "fluency": 0.75},
		model:  "test-llm",
	}
	svc := NewService(newMem(), judge)
	created := createJudgeEval(t, svc, evaluation.JudgeModeLLM)

	rv, err := svc.RunLLMJudge(context.Background(), created.ID, "output", "ref")
	if err != nil {
		t.Fatalf("run llm judge: %v", err)
	}
	if rv.JudgeType != evaluation.JudgeTypeLLM {
		t.Fatalf("expected judge_type=llm, got %s", rv.JudgeType)
	}
	if rv.JudgeID != "test-llm" {
		t.Fatalf("expected judge_id=test-llm, got %s", rv.JudgeID)
	}
}

func TestService_RunLLMJudge_NoJudgeConfigured(t *testing.T) {
	svc := NewService(newMem(), nil)
	created := createJudgeEval(t, svc, evaluation.JudgeModeLLM)
	if _, err := svc.RunLLMJudge(context.Background(), created.ID, "", ""); err == nil {
		t.Fatalf("expected error when judge not configured")
	}
}

func TestService_RunLLMJudge_StubIntegration(t *testing.T) {
	
	svc := NewService(newMem(), llmjudge.NewStub("stub"))
	created := createJudgeEval(t, svc, evaluation.JudgeModeLLM)

	rv, err := svc.RunLLMJudge(context.Background(), created.ID, "", "")
	if err != nil {
		t.Fatalf("run llm judge with stub: %v", err)
	}
	if rv.JudgeID != "stub" {
		t.Fatalf("expected judge_id=stub, got %s", rv.JudgeID)
	}
}

func TestServiceFinalizeReport(t *testing.T) {
	svc := NewService(newMem(), nil)
	created := createJudgeEval(t, svc, evaluation.JudgeModeHuman)

	_, _ = svc.SubmitReview(context.Background(), created.ID, ReviewInput{
		JudgeID: "u-1",
		Scores:  `{"accuracy":0.9,"fluency":0.8}`,
	})

	report, err := svc.FinalizeReport(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	
	expected := 0.86
	if abs(report.Overall-expected) > 0.001 {
		t.Fatalf("expected overall %.4f, got %.4f", expected, report.Overall)
	}

	got, _ := svc.Get(context.Background(), created.ID)
	if got.Status != evaluation.StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if !got.Passed {
		t.Fatalf("expected passed=true")
	}
}

func TestService_FinalizeReport_AlreadyFinalized(t *testing.T) {
	svc := NewService(newMem(), nil)
	created := createJudgeEval(t, svc, evaluation.JudgeModeHuman)
	_, _ = svc.SubmitReview(context.Background(), created.ID, ReviewInput{
		JudgeID: "u-1",
		Scores:  `{"accuracy":0.9,"fluency":0.8}`,
	})
	_, _ = svc.FinalizeReport(context.Background(), created.ID)
	if _, err := svc.FinalizeReport(context.Background(), created.ID); err == nil {
		t.Fatalf("finalize twice should error")
	}
}

func TestService_GetReport_NotFinalized(t *testing.T) {
	svc := NewService(newMem(), nil)
	created := createJudgeEval(t, svc, evaluation.JudgeModeHuman)
	_, _ = svc.SubmitReview(context.Background(), created.ID, ReviewInput{
		JudgeID: "u-1",
		Scores:  `{"accuracy":0.9,"fluency":0.8}`,
	})

	report, err := svc.GetReport(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get report: %v", err)
	}
	if report.Overall <= 0 {
		t.Fatalf("expected positive overall, got %v", report.Overall)
	}
	
	got, _ := svc.Get(context.Background(), created.ID)
	if got.Status == evaluation.StatusCompleted {
		t.Fatalf("get report should not finalize evaluation")
	}
}

func TestService_GetReport_FinalizedFromDB(t *testing.T) {
	svc := NewService(newMem(), nil)
	created := createJudgeEval(t, svc, evaluation.JudgeModeHuman)
	_, _ = svc.SubmitReview(context.Background(), created.ID, ReviewInput{
		JudgeID: "u-1",
		Scores:  `{"accuracy":0.9,"fluency":0.8}`,
	})
	finalized, _ := svc.FinalizeReport(context.Background(), created.ID)

	report, err := svc.GetReport(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get report: %v", err)
	}
	if abs(report.Overall-finalized.Overall) > 0.001 {
		t.Fatalf("expected report from DB %.4f, got %.4f", finalized.Overall, report.Overall)
	}
}

func TestService_HybridMode_AcceptsBoth(t *testing.T) {
	judge := &fakeJudge{scores: map[string]float64{"accuracy": 0.7, "fluency": 0.6}, model: "llm-1"}
	svc := NewService(newMem(), judge)
	created := createJudgeEval(t, svc, evaluation.JudgeModeHybrid)

	if _, err := svc.SubmitReview(context.Background(), created.ID, ReviewInput{
		JudgeID: "u-1",
		Scores:  `{"accuracy":0.9,"fluency":0.8}`,
	}); err != nil {
		t.Fatalf("human review on hybrid: %v", err)
	}
	
	if _, err := svc.RunLLMJudge(context.Background(), created.ID, "", ""); err != nil {
		t.Fatalf("llm review on hybrid: %v", err)
	}

	list, _ := svc.ListReviews(context.Background(), created.ID)
	if len(list) != 2 {
		t.Fatalf("expected 2 reviews, got %d", len(list))
	}

	report, _ := svc.GetReport(context.Background(), created.ID)
	if report.ByJudgeType[evaluation.JudgeTypeHuman].Count != 1 {
		t.Fatalf("expected 1 human in by_judge_type")
	}
	if report.ByJudgeType[evaluation.JudgeTypeLLM].Count != 1 {
		t.Fatalf("expected 1 llm in by_judge_type")
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}