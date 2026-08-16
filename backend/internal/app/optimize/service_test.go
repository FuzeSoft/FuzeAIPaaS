package optimizeapp

import (
	"context"
	"errors"
	"testing"

	"fuze-ai-paas/backend/internal/domain/optimize"
)

type memRepo struct {
	tasks map[string]*optimize.CompressionTask
}

func newMemRepo() *memRepo { return &memRepo{tasks: map[string]*optimize.CompressionTask{}} }

func (m *memRepo) Create(_ context.Context, t *optimize.CompressionTask) error {
	m.tasks[t.ID] = t
	return nil
}
func (m *memRepo) Get(_ context.Context, id string) (*optimize.CompressionTask, error) {
	t, ok := m.tasks[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return t, nil
}
func (m *memRepo) ListByTenant(_ context.Context, tenantID string) ([]*optimize.CompressionTask, error) {
	var out []*optimize.CompressionTask
	for _, t := range m.tasks {
		if tenantID == "" || t.TenantID == tenantID {
			out = append(out, t)
		}
	}
	return out, nil
}
func (m *memRepo) Update(_ context.Context, t *optimize.CompressionTask) error {
	m.tasks[t.ID] = t
	return nil
}
func (m *memRepo) Delete(_ context.Context, id string) error {
	delete(m.tasks, id)
	return nil
}

type fakeExecutor struct {
	submitted map[string]bool
	cancelled map[string]bool
	resultReady bool
	result     *optimize.CompressionResult
}

func newFakeExecutor() *fakeExecutor {
	return &fakeExecutor{submitted: map[string]bool{}, cancelled: map[string]bool{}, resultReady: true}
}

func (f *fakeExecutor) Submit(t *optimize.CompressionTask) (string, error) {
	jobID := "job-" + t.ID
	f.submitted[jobID] = true
	return jobID, nil
}
func (f *fakeExecutor) Cancel(jobID string) error {
	f.cancelled[jobID] = true
	return nil
}
func (f *fakeExecutor) GetResult(jobID string) (*optimize.CompressionResult, error) {
	if !f.resultReady {
		return nil, errors.New("not ready")
	}
	r := *f.result
	r.JobID = jobID
	return &r, nil
}

func TestService_Create_Success(t *testing.T) {
	repo := newMemRepo()
	ex := newFakeExecutor()
	svc := NewService(repo, ex)

	task, err := svc.Create(context.Background(), CreateInput{
		Name:           "q8",
		TenantID:       "t1",
		Type:           optimize.CompressionTypeQuantize,
		Backend:        optimize.BackendPyTorch,
		ConfigJSON:     `{"method":"dynamic","bits":8}`,
		ModelVersionID: "mv1",
		OrigAccuracy:   0.95,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if task.Status != optimize.StatusRunning {
		t.Fatalf("expected running after submit, got %q", task.Status)
	}
	if task.JobID == "" {
		t.Fatal("job id should be set")
	}
	if task.GateThreshold != defaultGateThreshold {
		t.Fatalf("default gate threshold should be 0.01, got %v", task.GateThreshold)
	}
	if !ex.submitted[task.JobID] {
		t.Fatal("executor should have submitted the job")
	}
}

func TestService_Create_InvalidConfig(t *testing.T) {
	svc := NewService(newMemRepo(), newFakeExecutor())
	_, err := svc.Create(context.Background(), CreateInput{
		Name:           "q",
		Type:           optimize.CompressionTypeQuantize,
		Backend:        optimize.BackendPyTorch,
		ConfigJSON:     `{"method":"bad"}`,
		ModelVersionID: "mv1",
	})
	if err == nil {
		t.Fatal("invalid config should fail")
	}
}

func TestService_Create_UnsupportedBackend(t *testing.T) {
	svc := NewService(newMemRepo(), newFakeExecutor())
	_, err := svc.Create(context.Background(), CreateInput{
		Name:           "q",
		Type:           optimize.CompressionTypePrune,
		Backend:        optimize.BackendONNXRuntime, 
		ConfigJSON:     `{"strategy":"structured","sparsity":0.5}`,
		ModelVersionID: "mv1",
	})
	if err == nil {
		t.Fatal("unsupported (type,backend) should fail at create")
	}
}

func TestService_HandleResult_GatePass(t *testing.T) {
	repo := newMemRepo()
	ex := newFakeExecutor()
	ex.result = &optimize.CompressionResult{
		CompressedSizeBytes: 250,
		LatencyMs:           50,
		Accuracy:            0.945,
		ArtifactURI:         "s3://x/model",
		CompressionRatio:    4.0,
		Speedup:             4.0,
	}
	svc := NewService(repo, ex)
	task, _ := svc.Create(context.Background(), CreateInput{
		Name: "q", Type: optimize.CompressionTypeQuantize, Backend: optimize.BackendPyTorch,
		ConfigJSON: `{"method":"dynamic","bits":8}`, ModelVersionID: "mv1", OrigAccuracy: 0.95,
	})
	if err := svc.HandleResult(context.Background(), task.ID); err != nil {
		t.Fatalf("handle result: %v", err)
	}
	got, _ := repo.Get(context.Background(), task.ID)
	if got.Status != optimize.StatusSucceeded {
		t.Fatalf("gate pass should set succeeded, got %q", got.Status)
	}
	if !got.GatePass {
		t.Fatal("gate should pass (drop 0.005 <= 0.01)")
	}
	if got.CompressionRatio != 4.0 {
		t.Fatalf("compression ratio should be 4.0, got %v", got.CompressionRatio)
	}
}

func TestService_HandleResult_GateFail(t *testing.T) {
	repo := newMemRepo()
	ex := newFakeExecutor()
	ex.result = &optimize.CompressionResult{Accuracy: 0.90, CompressionRatio: 5, Speedup: 5}
	svc := NewService(repo, ex)
	task, _ := svc.Create(context.Background(), CreateInput{
		Name: "q", Type: optimize.CompressionTypeQuantize, Backend: optimize.BackendPyTorch,
		ConfigJSON: `{"method":"dynamic","bits":8}`, ModelVersionID: "mv1", OrigAccuracy: 0.95,
	})
	if err := svc.HandleResult(context.Background(), task.ID); err != nil {
		t.Fatalf("handle result: %v", err)
	}
	got, _ := repo.Get(context.Background(), task.ID)
	if got.Status != optimize.StatusFailed {
		t.Fatalf("gate fail should set failed, got %q", got.Status)
	}
	if got.GatePass {
		t.Fatal("gate should not pass (drop 0.05 > 0.01)")
	}
	if got.FailReason == "" {
		t.Fatal("fail reason should record the gate verdict")
	}
}

func TestService_HandleResult_NotReady(t *testing.T) {
	repo := newMemRepo()
	ex := newFakeExecutor()
	ex.resultReady = false 
	svc := NewService(repo, ex)
	task, _ := svc.Create(context.Background(), CreateInput{
		Name: "q", Type: optimize.CompressionTypeQuantize, Backend: optimize.BackendPyTorch,
		ConfigJSON: `{"method":"dynamic","bits":8}`, ModelVersionID: "mv1", OrigAccuracy: 0.95,
	})
	if err := svc.HandleResult(context.Background(), task.ID); err == nil {
		t.Fatal("not-ready result should return error for retry")
	}
}

func TestServiceCancel(t *testing.T) {
	repo := newMemRepo()
	ex := newFakeExecutor()
	svc := NewService(repo, ex)
	task, _ := svc.Create(context.Background(), CreateInput{
		Name: "q", Type: optimize.CompressionTypeQuantize, Backend: optimize.BackendPyTorch,
		ConfigJSON: `{"method":"dynamic","bits":8}`, ModelVersionID: "mv1",
	})
	if err := svc.Cancel(context.Background(), task.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	got, _ := repo.Get(context.Background(), task.ID)
	if got.Status != optimize.StatusCancelled {
		t.Fatalf("expected cancelled, got %q", got.Status)
	}
	if !ex.cancelled[got.JobID] {
		t.Fatal("executor cancel should be called")
	}
}

func TestService_Delete_RunningCancelsJob(t *testing.T) {
	repo := newMemRepo()
	ex := newFakeExecutor()
	svc := NewService(repo, ex)
	task, _ := svc.Create(context.Background(), CreateInput{
		Name: "q", Type: optimize.CompressionTypeQuantize, Backend: optimize.BackendPyTorch,
		ConfigJSON: `{"method":"dynamic","bits":8}`, ModelVersionID: "mv1",
	})
	if err := svc.Delete(context.Background(), task.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(context.Background(), task.ID); err == nil {
		t.Fatal("task should be deleted")
	}
	if !ex.cancelled[task.JobID] {
		t.Fatal("delete of running task should cancel the job")
	}
}