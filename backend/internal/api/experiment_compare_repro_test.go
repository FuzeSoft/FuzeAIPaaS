package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	experimentapp "fuze-ai-paas/backend/internal/app/experiment"
	trainingapp "fuze-ai-paas/backend/internal/app/training"
	trainingdomain "fuze-ai-paas/backend/internal/domain/training"
	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"github.com/gin-gonic/gin"
)

type fakeExpRepo struct {
	exps map[string]*models.Experiment
	runs map[string]*models.Run 
	byExp map[string][]*models.Run
	seqExp int
	seqRun int
}

func newFakeExpRepo() *fakeExpRepo {
	return &fakeExpRepo{exps: map[string]*models.Experiment{}, runs: map[string]*models.Run{}, byExp: map[string][]*models.Run{}}
}

func (f *fakeExpRepo) GetExperiments(tenantID string) ([]models.Experiment, error) {
	out := []models.Experiment{}
	for _, e := range f.exps {
		if tenantID == "" || e.TenantID == tenantID {
			out = append(out, *e)
		}
	}
	return out, nil
}
func (f *fakeExpRepo) GetExperiment(id string) (*models.Experiment, error) {
	if e, ok := f.exps[id]; ok {
		cp := *e
		return &cp, nil
	}
	return nil, ports.ErrNotFound
}
func (f *fakeExpRepo) CreateExperiment(e *models.Experiment) error {
	f.seqExp++
	e.ID = "exp-" + string(rune('a'+f.seqExp-1))
	cp := *e
	f.exps[e.ID] = &cp
	return nil
}
func (f *fakeExpRepo) UpdateExperiment(e *models.Experiment) error {
	if _, ok := f.exps[e.ID]; !ok {
		return ports.ErrNotFound
	}
	cp := *e
	f.exps[e.ID] = &cp
	return nil
}
func (f *fakeExpRepo) DeleteExperiment(id string) error {
	delete(f.exps, id)
	return nil
}
func (f *fakeExpRepo) GetRuns(experimentID string) ([]models.Run, error) {
	out := []models.Run{}
	for _, r := range f.byExp[experimentID] {
		out = append(out, *r)
	}
	return out, nil
}
func (f *fakeExpRepo) GetRun(id string) (*models.Run, error) {
	if r, ok := f.runs[id]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, ports.ErrNotFound
}
func (f *fakeExpRepo) GetRunByJobID(jobID string) (*models.Run, error) {
	for _, r := range f.runs {
		if r.SourceJobID == jobID {
			cp := *r
			return &cp, nil
		}
	}
	return nil, ports.ErrNotFound
}
func (f *fakeExpRepo) CreateRun(r *models.Run) error {
	f.seqRun++
	if r.ID == "" {
		r.ID = "run-" + string(rune('a'+f.seqRun-1))
	}
	cp := *r
	f.runs[r.ID] = &cp
	f.byExp[r.ExperimentID] = append(f.byExp[r.ExperimentID], &cp)
	return nil
}
func (f *fakeExpRepo) UpdateRun(r *models.Run) error {
	if _, ok := f.runs[r.ID]; !ok {
		return ports.ErrNotFound
	}
	cp := *r
	f.runs[r.ID] = &cp
	
	for i, er := range f.byExp[r.ExperimentID] {
		if er.ID == r.ID {
			f.byExp[r.ExperimentID][i] = &cp
		}
	}
	return nil
}
func (f *fakeExpRepo) DeleteRun(id string) error {
	if r, ok := f.runs[id]; ok {
		delete(f.runs, id)
		for i, er := range f.byExp[r.ExperimentID] {
			if er.ID == id {
				f.byExp[r.ExperimentID] = append(f.byExp[r.ExperimentID][:i], f.byExp[r.ExperimentID][i+1:]...)
				break
			}
		}
	}
	return nil
}

type fakeJobRepo struct {
	jobs map[string]*models.Job
	seq  int
}

func newFakeJobRepo() *fakeJobRepo { return &fakeJobRepo{jobs: map[string]*models.Job{}} }

func (f *fakeJobRepo) GetJobs() ([]models.Job, error) {
	out := []models.Job{}
	for _, j := range f.jobs {
		out = append(out, *j)
	}
	return out, nil
}
func (f *fakeJobRepo) GetJobsByTenant(tenantID string) ([]models.Job, error) {
	out := []models.Job{}
	for _, j := range f.jobs {
		if tenantID == "" || j.TenantID == tenantID {
			out = append(out, *j)
		}
	}
	return out, nil
}
func (f *fakeJobRepo) GetJob(id string) (*models.Job, error) {
	if j, ok := f.jobs[id]; ok {
		cp := *j
		return &cp, nil
	}
	return nil, ports.ErrNotFound
}
func (f *fakeJobRepo) CreateJob(j *models.Job) error {
	f.seq++
	j.ID = "job-" + string(rune('a'+f.seq-1))
	cp := *j
	f.jobs[j.ID] = &cp
	return nil
}
func (f *fakeJobRepo) UpdateJob(j *models.Job) error {
	if _, ok := f.jobs[j.ID]; !ok {
		return ports.ErrNotFound
	}
	cp := *j
	f.jobs[j.ID] = &cp
	return nil
}
func (f *fakeJobRepo) UpdateJobSpec(j *models.Job) error   { return f.UpdateJob(j) }
func (f *fakeJobRepo) UpdateJobStatus(j *models.Job) error { return f.UpdateJob(j) }
func (f *fakeJobRepo) DeleteJob(id string) error           { delete(f.jobs, id); return nil }

type fakeQuota struct{}

func (fakeQuota) GetQuota(string) (*models.Quota, error) { return &models.Quota{}, nil }
func (fakeQuota) ListQuotas() ([]models.Quota, error)    { return nil, nil }
func (fakeQuota) UpsertQuota(*models.Quota) error        { return nil }
func (fakeQuota) CheckAndReserve(string, int, int, int) error { return nil }
func (fakeQuota) Release(string, int, int, int) error    { return nil }
func (fakeQuota) AdjustReservation(string, int, int, int, int) error { return nil }

type fakeModelRepo struct{}

func (fakeModelRepo) GetModels() ([]models.Model, error)      { return nil, nil }
func (fakeModelRepo) GetModelsByTenant(string) ([]models.Model, error) { return nil, nil }
func (fakeModelRepo) GetModel(string) (*models.Model, error)  { return nil, ports.ErrNotFound }
func (fakeModelRepo) GetModelVersions(string) ([]models.ModelVersion, error) {
	return nil, nil
}
func (fakeModelRepo) GetModelVersion(string, string) (*models.ModelVersion, error) {
	return nil, ports.ErrNotFound
}
func (fakeModelRepo) CreateModel(*models.Model) error               { return nil }
func (fakeModelRepo) UpdateModel(*models.Model) error               { return nil }
func (fakeModelRepo) DeleteModel(string) error                      { return nil }
func (fakeModelRepo) CreateModelVersion(*models.ModelVersion) error { return nil }
func (fakeModelRepo) DeleteModelVersion(string, string) error       { return nil }

type fakeScheduler struct{ submitted []string }

func (s *fakeScheduler) SubmitJob(j *models.Job) error {
	s.submitted = append(s.submitted, j.ID)
	return nil
}
func (s *fakeScheduler) TerminateJob(*models.Job) error { return nil }
func (s *fakeScheduler) CancelJob(*models.Job) error    { return nil }

func newExperimentRouter(expRepo *fakeExpRepo, jobRepo *fakeJobRepo, claims *auth.Claims) (*gin.Engine, *fakeScheduler) {
	gin.SetMode(gin.TestMode)
	expSvc := experimentapp.NewService(expRepo)
	sched := &fakeScheduler{}
	trainSvc := trainingapp.NewService(jobRepo, fakeQuota{}, fakeModelRepo{}, sched)
	h := &Handler{
		experiment: expSvc,
		jobRepo:    jobRepo,
		training:   trainSvc,
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if claims != nil {
			auth.SetPrincipal(c, claims)
		}
		c.Next()
	})
	r.GET("/experiments/compare", h.CompareExperiments)
	r.POST("/experiments/runs/:runId/reproduce", h.ReproduceRun)
	r.GET("/experiments/runs/:runId/reproduction", h.GetReproduction)
	return r, sched
}

func adminClaims() *auth.Claims {
	return &auth.Claims{UserID: "root", Role: models.RolePlatformAdmin, TenantID: "default"}
}

func floatPtr(v float64) *float64 { return &v }

func seedExperiment(t *testing.T, repo *fakeExpRepo, name, metricName, objective string) *models.Experiment {
	t.Helper()
	e := &models.Experiment{Name: name, MetricName: metricName, Objective: objective, Status: "active", TenantID: "default"}
	if err := repo.CreateExperiment(e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestCompareExperimentsMetricMismatchReturns400(t *testing.T) {
	repo := newFakeExpRepo()
	e1 := seedExperiment(t, repo, "baseline", "acc", "maximize")
	e2 := seedExperiment(t, repo, "other", "f1", "maximize")

	r, _ := newExperimentRouter(repo, newFakeJobRepo(), adminClaims())
	req := httptest.NewRequest(http.MethodGet, "/experiments/compare?ids="+e1.ID+","+e2.ID+"&metric_name=acc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for metric mismatch, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), e2.Name) {
		t.Errorf("error should name the mismatching experiment, body=%s", w.Body.String())
	}
}

func TestCompareExperimentsReturnsBestRunsAndOverall(t *testing.T) {
	repo := newFakeExpRepo()
	e1 := seedExperiment(t, repo, "baseline", "acc", "maximize")
	e2 := seedExperiment(t, repo, "tuned", "acc", "maximize")

	repo.CreateRun(&models.Run{ExperimentID: e1.ID, TenantID: "default", Name: "r1", Status: "completed", MetricValue: floatPtr(0.88)})
	repo.CreateRun(&models.Run{ExperimentID: e1.ID, TenantID: "default", Name: "r2", Status: "completed", MetricValue: floatPtr(0.90)})
	repo.CreateRun(&models.Run{ExperimentID: e2.ID, TenantID: "default", Name: "r3", Status: "completed", MetricValue: floatPtr(0.95)})

	r, _ := newExperimentRouter(repo, newFakeJobRepo(), adminClaims())
	req := httptest.NewRequest(http.MethodGet, "/experiments/compare?ids="+e1.ID+","+e2.ID+"&metric_name=acc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp experimentCompareResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.OverallBestExperimentID != e2.ID {
		t.Errorf("overall best = %q, want %q", resp.OverallBestExperimentID, e2.ID)
	}
	if len(resp.Experiments) != 2 {
		t.Fatalf("want 2 rows, got %d", len(resp.Experiments))
	}
	for _, row := range resp.Experiments {
		if row.BestRun == nil {
			t.Errorf("experiment %q has nil best run", row.Name)
		}
	}
}

func TestReproduceRunClonesAndLinksParent(t *testing.T) {
	repo := newFakeExpRepo()
	jobRepo := newFakeJobRepo()
	e := seedExperiment(t, repo, "baseline", "acc", "maximize")
	sourceJob := &models.Job{
		TenantID: "default", Name: "src", Status: "completed",
		Image: "pytorch:2.3", Command: "python train.py", GPUs: 2, Memory: 16,
	}
	jobRepo.CreateJob(sourceJob)
	sourceRun := &models.Run{
		ExperimentID: e.ID, TenantID: "default", Name: "src-run", Status: "completed",
		MetricValue: floatPtr(0.91), SourceJobID: sourceJob.ID,
		Hyperparameters: `{"lr":0.01}`,
	}
	repo.CreateRun(sourceRun)

	r, sched := newExperimentRouter(repo, jobRepo, adminClaims())
	req := httptest.NewRequest(http.MethodPost, "/experiments/runs/"+sourceRun.ID+"/reproduce", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d body=%s", w.Code, w.Body.String())
	}
	var repro models.Run
	if err := json.Unmarshal(w.Body.Bytes(), &repro); err != nil {
		t.Fatal(err)
	}
	if repro.ParentRunID != sourceRun.ID {
		t.Errorf("ParentRunID = %q, want %q", repro.ParentRunID, sourceRun.ID)
	}
	if repro.ReproductionState != "pending" {
		t.Errorf("ReproductionState = %q, want pending", repro.ReproductionState)
	}
	if len(sched.submitted) != 1 {
		t.Errorf("expected training submit to be triggered once, got %d", len(sched.submitted))
	}
}

func TestReproduceRunMissingSourceJobReturns400(t *testing.T) {
	repo := newFakeExpRepo()
	e := seedExperiment(t, repo, "baseline", "acc", "maximize")
	sourceRun := &models.Run{ExperimentID: e.ID, TenantID: "default", Name: "src-run", Status: "completed", MetricValue: floatPtr(0.91)}
	repo.CreateRun(sourceRun)

	r, _ := newExperimentRouter(repo, newFakeJobRepo(), adminClaims())
	req := httptest.NewRequest(http.MethodPost, "/experiments/runs/"+sourceRun.ID+"/reproduce", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing source_job_id, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetReproductionWithinToleranceMarkedMatched(t *testing.T) {
	repo := newFakeExpRepo()
	e := seedExperiment(t, repo, "baseline", "acc", "maximize")
	sourceRun := &models.Run{ExperimentID: e.ID, TenantID: "default", Name: "src", Status: "completed", MetricValue: floatPtr(0.91), Hyperparameters: `{"lr":0.01}`}
	repo.CreateRun(sourceRun)
	
	reproRun := &models.Run{ExperimentID: e.ID, TenantID: "default", Name: "repro", Status: "completed", MetricValue: floatPtr(0.915), ParentRunID: sourceRun.ID, ReproductionState: "pending"}
	repo.CreateRun(reproRun)

	r, _ := newExperimentRouter(repo, newFakeJobRepo(), adminClaims())
	req := httptest.NewRequest(http.MethodGet, "/experiments/runs/"+reproRun.ID+"/reproduction", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["reproducible"] != true {
		t.Errorf("expected reproducible=true, body=%s", w.Body.String())
	}
	if body["reproduction_state"] != "matched" {
		t.Errorf("expected reproduction_state=matched, got %v", body["reproduction_state"])
	}
	
	stored, _ := repo.GetRun(reproRun.ID)
	if stored.ReproductionState != "matched" {
		t.Errorf("stored reproduction_state = %q, want matched", stored.ReproductionState)
	}
}

func TestGetReproductionOutsideToleranceMarkedDiverged(t *testing.T) {
	repo := newFakeExpRepo()
	e := seedExperiment(t, repo, "baseline", "acc", "maximize")
	sourceRun := &models.Run{ExperimentID: e.ID, TenantID: "default", Name: "src", Status: "completed", MetricValue: floatPtr(0.91)}
	repo.CreateRun(sourceRun)
	
	reproRun := &models.Run{ExperimentID: e.ID, TenantID: "default", Name: "repro", Status: "failed", MetricValue: floatPtr(0.80), ParentRunID: sourceRun.ID, ReproductionState: "pending"}
	repo.CreateRun(reproRun)

	r, _ := newExperimentRouter(repo, newFakeJobRepo(), adminClaims())
	req := httptest.NewRequest(http.MethodGet, "/experiments/runs/"+reproRun.ID+"/reproduction", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["reproducible"] != false {
		t.Errorf("expected reproducible=false, body=%s", w.Body.String())
	}
	if body["reproduction_state"] != "diverged" {
		t.Errorf("expected reproduction_state=diverged, got %v", body["reproduction_state"])
	}
}

func TestGetReproductionPendingWhenNoMetric(t *testing.T) {
	repo := newFakeExpRepo()
	e := seedExperiment(t, repo, "baseline", "acc", "maximize")
	sourceRun := &models.Run{ExperimentID: e.ID, TenantID: "default", Name: "src", Status: "completed", MetricValue: floatPtr(0.91)}
	repo.CreateRun(sourceRun)
	reproRun := &models.Run{ExperimentID: e.ID, TenantID: "default", Name: "repro", Status: "running", ParentRunID: sourceRun.ID, ReproductionState: "pending"}
	repo.CreateRun(reproRun)

	r, _ := newExperimentRouter(repo, newFakeJobRepo(), adminClaims())
	req := httptest.NewRequest(http.MethodGet, "/experiments/runs/"+reproRun.ID+"/reproduction", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["status"] != "pending" {
		t.Errorf("expected status=pending, got %v", body["status"])
	}
	if body["reproducible"] != false {
		t.Errorf("expected reproducible=false for pending, body=%s", w.Body.String())
	}
}

func TestGetReproductionNotReproductionRunReturns400(t *testing.T) {
	repo := newFakeExpRepo()
	e := seedExperiment(t, repo, "baseline", "acc", "maximize")
	normal := &models.Run{ExperimentID: e.ID, TenantID: "default", Name: "n", Status: "completed", MetricValue: floatPtr(0.91)}
	repo.CreateRun(normal)

	r, _ := newExperimentRouter(repo, newFakeJobRepo(), adminClaims())
	req := httptest.NewRequest(http.MethodGet, "/experiments/runs/"+normal.ID+"/reproduction", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for non-reproduction run, got %d body=%s", w.Code, w.Body.String())
	}
}

var _ = trainingdomain.Spec{}