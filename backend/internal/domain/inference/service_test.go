package inference

import "testing"

func newSvc() *InferenceService {
	return &InferenceService{
		ID:             "svc-1",
		Name:           "demo",
		Runtime:        RuntimeVLLM,
		TargetReplicas: 2,
		ReadyReplicas:  0,
		Status:         InferenceStatusPending,
	}
}

func TestNeedsDeploy(t *testing.T) {
	s := newSvc() 
	if !s.NeedsDeploy() {
		t.Fatalf("expected NeedsDeploy=true when RuntimeName empty")
	}
	s.RuntimeName = "demo-vllm"
	if s.NeedsDeploy() {
		t.Fatalf("expected NeedsDeploy=false after deploy populated RuntimeName")
	}
}

func TestShouldUseRuntime(t *testing.T) {
	s := newSvc()
	if s.ShouldUseRuntime(false, false) {
		t.Fatalf("unmanaged cluster must NOT use runtime (mock mode)")
	}
	if !s.ShouldUseRuntime(true, true) {
		t.Fatalf("managed+available cluster must use runtime")
	}
	if s.ShouldUseRuntime(true, false) {
		t.Fatalf("managed but unreachable cluster must NOT fake readiness")
	}
}

func TestNeedsScalePush(t *testing.T) {
	s := newSvc()
	if !s.NeedsScalePush() {
		t.Fatalf("expected scale push when Ready!=Target")
	}
	s.ReadyReplicas = s.TargetReplicas
	if s.NeedsScalePush() {
		t.Fatalf("expected no scale push when converged")
	}
	
	s.TargetReplicas = 0
	if !s.NeedsScalePush() {
		t.Fatalf("expected scale push for target_replicas=0 (scale-to-zero)")
	}
}

func TestNeedsCanaryPushAlways(t *testing.T) {
	s := newSvc()
	s.CanaryWeight = 0
	if !s.NeedsCanaryPush() {
		t.Fatalf("canary weight 0 must still be pushed (roll back canary)")
	}
}

func TestMockConvergeObservation(t *testing.T) {
	s := newSvc() 
	if !s.MockConvergeObservation() {
		t.Fatalf("expected convergence when not aligned")
	}
	if s.ReadyReplicas != s.TargetReplicas {
		t.Fatalf("mock converge must set Ready=Target, got %d/%d", s.ReadyReplicas, s.TargetReplicas)
	}
	if s.Status != InferenceStatusReady {
		t.Fatalf("mock converge must mark Ready, got %s", s.Status)
	}
	
	if s.MockConvergeObservation() {
		t.Fatalf("already-converged state must not signal a change")
	}
}