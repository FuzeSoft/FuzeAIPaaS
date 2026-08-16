package experiment

import (
	"testing"

	"fuze-ai-paas/backend/internal/domain/experiment"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

const (
	ObjectiveMinimize = experiment.ObjectiveMinimize
	ObjectiveMaximize = experiment.ObjectiveMaximize
)

type memExperimentRepo struct {
	exps map[string]*models.Experiment
	runs map[string]*models.Run
}

func newMemRepo() *memExperimentRepo {
	return &memExperimentRepo{
		exps: make(map[string]*models.Experiment),
		runs: make(map[string]*models.Run),
	}
}

func (m *memExperimentRepo) GetExperiments(tenantID string) ([]models.Experiment, error) {
	var out []models.Experiment
	for _, e := range m.exps {
		if tenantID == "" || e.TenantID == tenantID {
			out = append(out, *e)
		}
	}
	return out, nil
}
func (m *memExperimentRepo) GetExperiment(id string) (*models.Experiment, error) {
	e, ok := m.exps[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return e, nil
}
func (m *memExperimentRepo) CreateExperiment(e *models.Experiment) error {
	m.exps[e.ID] = e
	return nil
}
func (m *memExperimentRepo) UpdateExperiment(e *models.Experiment) error {
	m.exps[e.ID] = e
	return nil
}
func (m *memExperimentRepo) DeleteExperiment(id string) error {
	delete(m.exps, id)
	return nil
}
func (m *memExperimentRepo) GetRuns(experimentID string) ([]models.Run, error) {
	var out []models.Run
	for _, r := range m.runs {
		if r.ExperimentID == experimentID {
			out = append(out, *r)
		}
	}
	return out, nil
}
func (m *memExperimentRepo) GetRun(id string) (*models.Run, error) {
	r, ok := m.runs[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return r, nil
}
func (m *memExperimentRepo) GetRunByJobID(jobID string) (*models.Run, error) {
	for _, r := range m.runs {
		if r.SourceJobID == jobID {
			cp := *r
			return &cp, nil
		}
	}
	return nil, ports.ErrNotFound
}
func (m *memExperimentRepo) CreateRun(r *models.Run) error {
	m.runs[r.ID] = r
	return nil
}
func (m *memExperimentRepo) UpdateRun(r *models.Run) error {
	m.runs[r.ID] = r
	return nil
}
func (m *memExperimentRepo) DeleteRun(id string) error {
	delete(m.runs, id)
	return nil
}

func fval(v float64) *float64 { return &v }

func TestServiceBestRunRefreshOnComplete(t *testing.T) {
	repo := newMemRepo()
	svc := NewService(repo)

	exp, err := svc.CreateExperiment(ExperimentInput{
		TenantID:   "t1",
		Name:       "exp",
		Objective:  ObjectiveMinimize,
		MetricName: "loss",
	})
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	runA, err := svc.CreateRun(RunInput{TenantID: "t1", ExperimentID: exp.ID, Name: "a"})
	if err != nil {
		t.Fatalf("create run a: %v", err)
	}
	runB, err := svc.CreateRun(RunInput{TenantID: "t1", ExperimentID: exp.ID, Name: "b"})
	if err != nil {
		t.Fatalf("create run b: %v", err)
	}

	if _, err := svc.CompleteRun(runA.ID, fval(0.5), nil, "s3://a"); err != nil {
		t.Fatalf("complete a: %v", err)
	}
	expAfterA, _ := svc.GetExperiment(exp.ID)
	if expAfterA.BestRunID != runA.ID {
		t.Fatalf("expected best runA after first complete, got %s", expAfterA.BestRunID)
	}

	if _, err := svc.CompleteRun(runB.ID, fval(0.2), nil, "s3://b"); err != nil {
		t.Fatalf("complete b: %v", err)
	}
	expAfterB, _ := svc.GetExperiment(exp.ID)
	if expAfterB.BestRunID != runB.ID {
		t.Fatalf("expected best runB (lower loss), got %s", expAfterB.BestRunID)
	}

	rb, _ := svc.GetRun(runB.ID)
	if !rb.IsBest {
		t.Fatalf("expected runB is_best=true")
	}
	ra, _ := svc.GetRun(runA.ID)
	if ra.IsBest {
		t.Fatalf("expected runA is_best=false")
	}
}

func TestServiceCreateRunCrossTenantRejected(t *testing.T) {
	repo := newMemRepo()
	svc := NewService(repo)
	exp, _ := svc.CreateExperiment(ExperimentInput{
		TenantID: "t1", Name: "exp", Objective: ObjectiveMaximize, MetricName: "acc",
	})
	
	if _, err := svc.CreateRun(RunInput{TenantID: "t2", ExperimentID: exp.ID, Name: "x"}); err == nil {
		t.Fatalf("expected cross-tenant run creation to be rejected")
	}
}

func TestServiceCompleteUnknownRun(t *testing.T) {
	repo := newMemRepo()
	svc := NewService(repo)
	if _, err := svc.CompleteRun("nope", fval(1.0), nil, ""); err == nil {
		t.Fatalf("expected error completing unknown run")
	}
}