package hpo

import (
	"context"
	"encoding/json"
	"testing"

	"fuze-ai-paas/backend/internal/domain/hpo"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	trainingapp "fuze-ai-paas/backend/internal/app/training"
)

type fakeStudyRepo struct {
	studies map[string]*models.HPOStudy
}

func (f *fakeStudyRepo) CreateStudy(s *models.HPOStudy) error { f.studies[s.ID] = s; return nil }
func (f *fakeStudyRepo) GetStudy(id string) (*models.HPOStudy, error) {
	if s, ok := f.studies[id]; ok {
		return s, nil
	}
	return nil, errNotFound
}
func (f *fakeStudyRepo) UpdateStudy(s *models.HPOStudy) error { f.studies[s.ID] = s; return nil }
func (f *fakeStudyRepo) ListStudies(tenantID string) ([]models.HPOStudy, error) {
	out := []models.HPOStudy{}
	for _, s := range f.studies {
		out = append(out, *s)
	}
	return out, nil
}
func (f *fakeStudyRepo) DeleteStudy(id string) error { delete(f.studies, id); return nil }

type fakeTrialRepo struct {
	trials map[string]*models.HPOTrial
}

func (f *fakeTrialRepo) CreateTrial(t *models.HPOTrial) error { f.trials[t.ID] = t; return nil }
func (f *fakeTrialRepo) GetTrial(id string) (*models.HPOTrial, error) {
	if t, ok := f.trials[id]; ok {
		return t, nil
	}
	return nil, errNotFound
}
func (f *fakeTrialRepo) UpdateTrial(t *models.HPOTrial) error { f.trials[t.ID] = t; return nil }
func (f *fakeTrialRepo) ListTrials(studyID string) ([]models.HPOTrial, error) {
	out := []models.HPOTrial{}
	for _, t := range f.trials {
		if t.StudyID == studyID {
			out = append(out, *t)
		}
	}
	return out, nil
}
func (f *fakeTrialRepo) ListRunningTrials(studyID string) ([]models.HPOTrial, error) {
	out, _ := f.ListTrials(studyID)
	var res []models.HPOTrial
	for _, t := range out {
		if t.Status == "running" {
			res = append(res, t)
		}
	}
	return res, nil
}
func (f *fakeTrialRepo) GetTrialByJobID(jobID string) (*models.HPOTrial, error) {
	for _, t := range f.trials {
		if t.JobID == jobID {
			return t, nil
		}
	}
	return nil, errNotFound
}
func (f *fakeTrialRepo) DeleteTrialsByStudy(studyID string) error {
	for k, t := range f.trials {
		if t.StudyID == studyID {
			delete(f.trials, k)
		}
	}
	return nil
}

type fakeTraining struct {
	submitted []trainingapp.SubmitInput
	cancelled []string
}

func (f *fakeTraining) Submit(tenantID string, in trainingapp.SubmitInput) (*models.Job, error) {
	f.submitted = append(f.submitted, in)
	return &models.Job{ID: "job_" + itoa(len(f.submitted))}, nil
}
func (f *fakeTraining) Cancel(jobID string) (*models.Job, error) {
	f.cancelled = append(f.cancelled, jobID)
	return &models.Job{ID: jobID}, nil
}
func (f *fakeTraining) Complete(jobID string) error { return nil }

type fakeExperiment struct {
	runs map[string]*models.Run
}

func (f *fakeExperiment) GetExperiments(tenantID string) ([]models.Experiment, error) {
	return nil, nil
}
func (f *fakeExperiment) GetExperiment(id string) (*models.Experiment, error) { return &models.Experiment{}, nil }
func (f *fakeExperiment) CreateExperiment(e *models.Experiment) error         { return nil }
func (f *fakeExperiment) UpdateExperiment(e *models.Experiment) error         { return nil }
func (f *fakeExperiment) DeleteExperiment(id string) error                    { return nil }
func (f *fakeExperiment) GetRuns(experimentID string) ([]models.Run, error) {
	out := []models.Run{}
	for _, r := range f.runs {
		out = append(out, *r)
	}
	return out, nil
}
func (f *fakeExperiment) GetRun(id string) (*models.Run, error) {
	if r, ok := f.runs[id]; ok {
		return r, nil
	}
	return nil, errNotFound
}
func (f *fakeExperiment) GetRunByJobID(jobID string) (*models.Run, error) {
	for _, r := range f.runs {
		if r.SourceJobID == jobID {
			return r, nil
		}
	}
	return nil, errNotFound
}
func (f *fakeExperiment) CreateRun(r *models.Run) error { f.runs[r.ID] = r; return nil }
func (f *fakeExperiment) UpdateRun(r *models.Run) error { f.runs[r.ID] = r; return nil }
func (f *fakeExperiment) DeleteRun(id string) error     { delete(f.runs, id); return nil }

type fakeQuota struct{ avail bool }

func (f *fakeQuota) GetQuota(tenantID string) (*models.Quota, error) {
	if !f.avail {
		return &models.Quota{JobQuota: 1, JobUsed: 1}, nil
	}
	return &models.Quota{JobQuota: 10, JobUsed: 0}, nil
}

var errNotFound = ports.ErrNotFound

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func newTestService() (*Service, *fakeStudyRepo, *fakeTrialRepo, *fakeTraining, *fakeQuota) {
	fs := &fakeStudyRepo{studies: map[string]*models.HPOStudy{}}
	ft := &fakeTrialRepo{trials: map[string]*models.HPOTrial{}}
	fe := &fakeExperiment{runs: map[string]*models.Run{}}
	ftr := &fakeTraining{}
	fq := &fakeQuota{avail: true}
	svc := NewService(fs, ft, fe, ftr, fq)
	svc.RandSeed = 42
	return svc, fs, ft, ftr, fq
}

func sampleSpec() StudySpec {
	return StudySpec{
		Name:      "tune",
		Algorithm: "random",
		Objective: hpo.Objective{MetricName: "acc", Direction: hpo.DirectionMaximize},
		Space: hpo.SearchSpace{Params: []hpo.ParamSpec{
			{Name: "lr", Type: hpo.ParamFloat, Min: 1e-4, Max: 1e-1, LogScale: true},
		}},
		MaxTrials:   3,
		MaxParallel: 2,
		TrainingTemplate: map[string]any{
			"image":   "train:latest",
			"command": "python train.py",
		},
	}
}

func TestCreateStudyValidation(t *testing.T) {
	svc, _, _, _, _ := newTestService()
	
	if _, err := svc.CreateStudy(context.Background(), "t1", StudySpec{MaxTrials: 1, Space: hpo.SearchSpace{Params: []hpo.ParamSpec{{Name: "x", Type: hpo.ParamFloat, Min: 0, Max: 1}}}}); err == nil {
		t.Fatal("expected error for missing objective metric")
	}
	
	if _, err := svc.CreateStudy(context.Background(), "t1", StudySpec{Objective: hpo.Objective{MetricName: "a"}, Space: hpo.SearchSpace{Params: []hpo.ParamSpec{{Name: "x", Type: hpo.ParamFloat, Min: 0, Max: 1}}}}); err == nil {
		t.Fatal("expected error for max_trials<=0")
	}
}

func TestRunOnceSpawnsTrial(t *testing.T) {
	svc, fs, ft, ftr, _ := newTestService()
	m, err := svc.CreateStudy(context.Background(), "t1", sampleSpec())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RunOnce(context.Background(), "t1", m.ID); err != nil {
		t.Fatal(err)
	}
	if len(ftr.submitted) != 1 {
		t.Fatalf("expected 1 training submit, got %d", len(ftr.submitted))
	}
	
	if !contains(ftr.submitted[0].Spec.Command, "--lr") {
		t.Fatalf("params not injected into command: %q", ftr.submitted[0].Spec.Command)
	}
	
	trials, _ := ft.ListTrials(m.ID)
	if len(trials) != 1 || trials[0].Status != "running" {
		t.Fatalf("trial not recorded as running: %+v", trials)
	}
	_ = fs
}

func TestRunOnceDistinctSeedsAcrossTicks(t *testing.T) {
	svc, _, ft, ftr, _ := newTestService()
	spec := sampleSpec()
	spec.MaxParallel = 1 
	spec.MaxTrials = 3
	m, err := svc.CreateStudy(context.Background(), "t1", spec)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.RunOnce(context.Background(), "t1", m.ID); err != nil {
		t.Fatal(err)
	}
	if len(ftr.submitted) != 1 {
		t.Fatalf("tick1: expected 1 submit, got %d", len(ftr.submitted))
	}
	
	trials1, _ := ft.ListTrials(m.ID)
	if len(trials1) != 1 {
		t.Fatalf("tick1: expected 1 trial, got %d", len(trials1))
	}
	if err := svc.OnJobTerminal(context.Background(), trials1[0].JobID, "Succeeded"); err != nil {
		t.Fatal(err)
	}

	if err := svc.RunOnce(context.Background(), "t1", m.ID); err != nil {
		t.Fatal(err)
	}
	if len(ftr.submitted) != 2 {
		t.Fatalf("tick2: expected 2 submits, got %d", len(ftr.submitted))
	}
	trials2, _ := ft.ListTrials(m.ID)
	if len(trials2) != 2 {
		t.Fatalf("tick2: expected 2 trials, got %d", len(trials2))
	}

	a, _ := svc.GetTrial(context.Background(), "t1", m.ID, trials1[0].ID)
	b, _ := svc.GetTrial(context.Background(), "t1", m.ID, trials2[1].ID)
	var pa, pb map[string]any
	if err := json.Unmarshal([]byte(a.ParamsJSON), &pa); err != nil {
		t.Fatalf("unmarshal trial1 params: %v", err)
	}
	if err := json.Unmarshal([]byte(b.ParamsJSON), &pb); err != nil {
		t.Fatalf("unmarshal trial2 params: %v", err)
	}
	lrA, okA := pa["lr"].(float64)
	lrB, okB := pb["lr"].(float64)
	if !okA || !okB {
		t.Fatalf("lr param missing: a=%v b=%v", pa, pb)
	}
	if lrA == lrB {
		t.Fatalf("cross-tick sampling collapsed to identical params (lr_a=%v lr_b=%v); seed not advanced", lrA, lrB)
	}
}

func TestOnJobTerminalDrivesScheduling(t *testing.T) {
	svc, _, ft, _, _ := newTestService()
	m, _ := svc.CreateStudy(context.Background(), "t1", sampleSpec())
	if err := svc.RunOnce(context.Background(), "t1", m.ID); err != nil {
		t.Fatal(err)
	}
	trials, _ := ft.ListTrials(m.ID)
	if len(trials) != 1 {
		t.Fatalf("expected 1 trial, got %d", len(trials))
	}
	jobID := trials[0].JobID
	trialID := trials[0].ID

	if err := svc.OnJobTerminal(context.Background(), jobID, "Succeeded"); err != nil {
		t.Fatal(err)
	}
	
	updated, err := svc.GetTrial(context.Background(), "t1", m.ID, trialID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "completed" {
		t.Fatalf("expected trial completed via job terminal, got %s", updated.Status)
	}
	
	sm, _ := svc.GetStudy(context.Background(), "t1", m.ID)
	if sm.Status == "" {
		t.Fatal("study status not updated")
	}
}

func TestOnJobTerminalIgnoresNonHPOJob(t *testing.T) {
	svc, _, _, _, _ := newTestService()
	if err := svc.OnJobTerminal(context.Background(), "job-unknown-123", "Failed"); err != nil {
		t.Fatalf("non-HPO job should be ignored, got err: %v", err)
	}
}

func TestRunOnceParallelSaturatedThenComplete(t *testing.T) {
	svc, fs, ft, ftr, _ := newTestService()
	m, _ := svc.CreateStudy(context.Background(), "t1", sampleSpec())
	
	if err := svc.RunOnce(context.Background(), "t1", m.ID); err != nil {
		t.Fatal(err)
	}
	
	if err := svc.RunOnce(context.Background(), "t1", m.ID); err != nil {
		t.Fatal(err)
	}
	if len(ftr.submitted) != 2 {
		t.Fatalf("expected 2 submits, got %d", len(ftr.submitted))
	}
	
	if err := svc.RunOnce(context.Background(), "t1", m.ID); err != nil {
		t.Fatal(err)
	}
	if len(ftr.submitted) != 2 {
		t.Fatalf("expected still 2 submits (saturated), got %d", len(ftr.submitted))
	}
	
	for _, tr := range mustList(t, ft, m.ID) {
		_ = svc.ReportFinal(context.Background(), "t1", tr.ID, 0.9)
	}
	if err := svc.RunOnce(context.Background(), "t1", m.ID); err != nil {
		t.Fatal(err)
	}
	if len(ftr.submitted) != 3 {
		t.Fatalf("expected 3 submits after completing first two, got %d", len(ftr.submitted))
	}
	
	for _, tr := range mustList(t, ft, m.ID) {
		st := tr.Status
		if st != "completed" {
			_ = svc.ReportFinal(context.Background(), "t1", tr.ID, 0.95)
		}
	}
	if err := svc.RunOnce(context.Background(), "t1", m.ID); err != nil {
		t.Fatal(err)
	}
	updated, _ := fs.GetStudy(m.ID)
	if updated.Status != hpo.StudyCompleted {
		t.Fatalf("study should complete after max_trials reached, got %q", updated.Status)
	}
}

func TestQuotaUnavailableSkipsSpawn(t *testing.T) {
	svc, _, _, ftr, _ := newTestService()
	m, _ := svc.CreateStudy(context.Background(), "t1", sampleSpec())
	
	svc.quota = &fakeQuota{avail: false}
	if err := svc.RunOnce(context.Background(), "t1", m.ID); err != nil {
		t.Fatal(err)
	}
	if len(ftr.submitted) != 0 {
		t.Fatalf("expected no submit when quota unavailable, got %d", len(ftr.submitted))
	}
}

func mustList(t *testing.T, ft *fakeTrialRepo, studyID string) []*models.HPOTrial {
	t.Helper()
	ts, err := ft.ListTrials(studyID)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]*models.HPOTrial, len(ts))
	for i := range ts {
		out[i] = &ts[i]
	}
	return out
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}