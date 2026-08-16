package inference

import (
	"fmt"
	"testing"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type fakeInferenceRepo struct {
	store map[string]*models.InferenceService
	byID  map[string]*models.InferenceService
}

func (f *fakeInferenceRepo) GetInferenceServices() ([]models.InferenceService, error) {
	out := make([]models.InferenceService, 0, len(f.store))
	for _, s := range f.store {
		out = append(out, *s)
	}
	return out, nil
}

func (f *fakeInferenceRepo) GetInferenceService(id string) (*models.InferenceService, error) {
	if s, ok := f.byID[id]; ok {
		return s, nil
	}
	return nil, nil
}

func (f *fakeInferenceRepo) UpdateInferenceService(s *models.InferenceService) error {
	if s == nil {
		return nil
	}
	f.store[s.Name] = s
	if s.ID != "" {
		f.byID[s.ID] = s
	}
	return nil
}

func (f *fakeInferenceRepo) UpdateInferenceRuntimeStatus(s *models.InferenceService) error {
	return f.UpdateInferenceService(s)
}

func (f *fakeInferenceRepo) UpdateInferenceServiceSpec(s *models.InferenceService) error {
	return f.UpdateInferenceService(s)
}

func (f *fakeInferenceRepo) DeleteInferenceService(id string) error {
	delete(f.byID, id)
	return nil
}

type fakeUnavailableCluster struct{}

func (fakeUnavailableCluster) Register(*models.Cluster) error { return nil }
func (fakeUnavailableCluster) Unregister(string)              {}
func (fakeUnavailableCluster) Get(string) (ports.ClusterClientPort, error) {
	return nil, fmt.Errorf("unreachable")
}
func (fakeUnavailableCluster) List() []string                   { return nil }
func (fakeUnavailableCluster) LoadAll([]models.Cluster) []error { return nil }
func (fakeUnavailableCluster) K8sClient(string) (ports.ClusterClientPort, error) {
	return nil, fmt.Errorf("unreachable")
}

func newTestService(clusterMgr ports.ClusterRegistry, runtimeReg ports.RuntimeRegistry) *Service {
	return NewService(&fakeInferenceRepo{
		store: map[string]*models.InferenceService{},
		byID:  map[string]*models.InferenceService{},
	}, clusterMgr, runtimeReg)
}

func TestDeployNilServiceNoPanic(t *testing.T) {
	svc := newTestService(nil, nil)
	if err := svc.Deploy(nil); err == nil {
		t.Fatal("expect error, not silent success")
	}
}

func TestDeleteNilServiceNoPanic(t *testing.T) {
	svc := newTestService(nil, nil)
	if err := svc.Delete(nil); err == nil {
		t.Fatal("expect error, not silent success")
	}
}

func TestDeployClusterUnavailableStaysPending(t *testing.T) {
	svc := newTestService(fakeUnavailableCluster{}, nil)
	s := &models.InferenceService{Name: "svc-x", ClusterID: "c1", TenantID: "t1", TargetReplicas: 1}
	_ = svc.Deploy(s)
	if s.Status == models.InferenceStatusReady {
		t.Fatal("must not falsely report ready when cluster unreachable")
	}
	if s.Status != models.InferenceStatusPending {
		t.Fatalf("expect pending, got %s", s.Status)
	}
	if s.FailureReason == "" {
		t.Fatal("should record cluster-unreachable reason")
	}
}