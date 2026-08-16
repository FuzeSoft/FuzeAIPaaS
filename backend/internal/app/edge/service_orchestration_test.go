package edge

import (
	"context"
	"testing"
	"time"

	domainedge "fuze-ai-paas/backend/internal/domain/edge"
)

type memNodes struct{ m map[string]*domainedge.EdgeNode }

func newMemNodes() *memNodes { return &memNodes{m: map[string]*domainedge.EdgeNode{}} }
func (r *memNodes) Save(_ context.Context, n *domainedge.EdgeNode) error { r.m[n.ID] = n; return nil }
func (r *memNodes) Get(_ context.Context, id string) (*domainedge.EdgeNode, error) {
	if n, ok := r.m[id]; ok {
		return n, nil
	}
	return nil, domainedge.ErrNodeNotFound
}
func (r *memNodes) List(_ context.Context, tid string) ([]*domainedge.EdgeNode, error) {
	out := make([]*domainedge.EdgeNode, 0, len(r.m))
	for _, n := range r.m {
		if tid == "" || n.TenantID == tid {
			out = append(out, n)
		}
	}
	return out, nil
}
func (r *memNodes) Delete(_ context.Context, id string) error { delete(r.m, id); return nil }

type memDeploys struct{ m map[string]*domainedge.EdgeDeployment }

func newMemDeploys() *memDeploys { return &memDeploys{m: map[string]*domainedge.EdgeDeployment{}} }
func (r *memDeploys) Save(_ context.Context, d *domainedge.EdgeDeployment) error { r.m[d.ID] = d; return nil }
func (r *memDeploys) Get(_ context.Context, id string) (*domainedge.EdgeDeployment, error) {
	if d, ok := r.m[id]; ok {
		return d, nil
	}
	return nil, domainedge.ErrDeploymentNotFound
}
func (r *memDeploys) ListByNode(_ context.Context, nid string) ([]*domainedge.EdgeDeployment, error) {
	out := []*domainedge.EdgeDeployment{}
	for _, d := range r.m {
		if d.NodeID == nid {
			out = append(out, d)
		}
	}
	return out, nil
}
func (r *memDeploys) List(_ context.Context, tid string) ([]*domainedge.EdgeDeployment, error) {
	out := []*domainedge.EdgeDeployment{}
	for _, d := range r.m {
		if tid == "" || d.TenantID == tid {
			out = append(out, d)
		}
	}
	return out, nil
}

type memDrifts struct {
	reports  map[string][]*domainedge.DriftReport
	baseline map[string]*domainedge.DriftBaseline
}

func newMemDrifts() *memDrifts {
	return &memDrifts{reports: map[string][]*domainedge.DriftReport{}, baseline: map[string]*domainedge.DriftBaseline{}}
}
func (r *memDrifts) SaveReport(_ context.Context, rep *domainedge.DriftReport) error {
	r.reports[rep.DeploymentID] = append([]*domainedge.DriftReport{rep}, r.reports[rep.DeploymentID]...)
	return nil
}
func (r *memDrifts) LatestByDeployment(_ context.Context, did string) (*domainedge.DriftReport, error) {
	if rs, ok := r.reports[did]; ok && len(rs) > 0 {
		return rs[0], nil
	}
	return nil, domainedge.ErrDriftReportNotFound
}
func (r *memDrifts) ListByDeployment(_ context.Context, did string, _ int) ([]*domainedge.DriftReport, error) {
	return r.reports[did], nil
}
func (r *memDrifts) SaveBaseline(_ context.Context, b *domainedge.DriftBaseline) error { r.baseline[b.DeploymentID] = b; return nil }
func (r *memDrifts) GetBaseline(_ context.Context, did string) (*domainedge.DriftBaseline, error) {
	if b, ok := r.baseline[did]; ok {
		return b, nil
	}
	return nil, domainedge.ErrBaselineNotFound
}

type recordingPublisher struct{ rolledBack int }

func (p *recordingPublisher) Publish(e domainedge.EdgeEvent) {
	if _, ok := e.(domainedge.DeploymentRolledBack); ok {
		p.rolledBack++
	}
}

func newTestService(rt domainedge.EdgeRuntime, sample domainedge.DriftSampleSource) (*Service, *memNodes, *memDeploys) {
	nodes := newMemNodes()
	deploys := newMemDeploys()
	drifts := newMemDrifts()
	svc := NewService(nodes, deploys, drifts, rt, sample, nil, nil,
		Config{OfflineThreshold: time.Minute}, nil, nil, SampleMetricNames{})
	return svc, nodes, deploys
}

func TestRegisterAndReconcileNode(t *testing.T) {
	svc, nodes, _ := newTestService(NewMockRuntimeForTest(), nil)
	if _, err := svc.RegisterNode(context.Background(), "t1", RegisterNodeInput{ID: "n1", Name: "edge-1", Mode: domainedge.NodeModeAgent}); err != nil {
		t.Fatal(err)
	}
	got, _ := nodes.Get(context.Background(), "n1")
	if got.Status != domainedge.NodeStatusPending {
		t.Fatalf("expected pending, got %s", got.Status)
	}
	
	_, err := svc.ReconcileNode(context.Background(), "t1", "n1", time.Now().UTC().Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	got, _ = nodes.Get(context.Background(), "n1")
	if got.Status != domainedge.NodeStatusOffline {
		t.Fatalf("expected offline after threshold, got %s", got.Status)
	}
}

func TestCanaryPromoteToActive(t *testing.T) {
	rt := NewMockRuntimeForTest()
	svc, nodes, deploys := newTestService(rt, nil)
	mustSaveNode(t, nodes, "n1", "t1")
	d, err := svc.Deploy(context.Background(), "t1", "n1", "m1", "v2", domainedge.EdgeDeploySpec{Replicas: 1}, 5, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.CanaryVersion != "v2" || d.CanaryWeight != 5 {
		t.Fatalf("canary not set: %+v", d)
	}
	
	step := 25
	for d.Status != domainedge.EdgeDeployActive && d.CanaryWeight < 100 {
		d, err = svc.PromoteCanary(context.Background(), "t1", d.ID, step)
		if err != nil {
			t.Fatal(err)
		}
	}
	if d.Status != domainedge.EdgeDeployActive || d.ActiveVersion != "v2" {
		t.Fatalf("expected active@v2, got %s@%s", d.Status, d.ActiveVersion)
	}
	_ = deploys
}

func TestManualRollback(t *testing.T) {
	rt := NewMockRuntimeForTest()
	svc, nodes, _ := newTestService(rt, nil)
	mustSaveNode(t, nodes, "n1", "t1")
	d, _ := svc.Deploy(context.Background(), "t1", "n1", "m1", "v1", domainedge.EdgeDeploySpec{Replicas: 1}, 0, true, true)
	
	d2, _ := svc.Deploy(context.Background(), "t1", "n1", "m1", "v2", domainedge.EdgeDeploySpec{Replicas: 1}, 0, true, true)
	if d2.ActiveVersion != "v1" {
		t.Fatalf("precondition: active should remain v1, got %s", d2.ActiveVersion)
	}
	d2, err := svc.Rollback(context.Background(), "t1", d2.ID, "manual test")
	if err != nil {
		t.Fatal(err)
	}
	if d2.Status != domainedge.EdgeDeployRolledBack || d2.CurrentVersion != "v1" {
		t.Fatalf("rollback failed: %+v", d2)
	}
	_ = d
}

func TestAutoRollbackOnDrift(t *testing.T) {
	rt := NewMockRuntimeForTest()
	pub := &recordingPublisher{}
	
	sampleSrc := driftSampleSourceFunc(func(_ context.Context, _ *domainedge.EdgeDeployment, _ string) (*domainedge.DriftSample, error) {
		return &domainedge.DriftSample{
			NumericFeatures: map[string]*domainedge.FeatureStat{
				"f1": {Mean: 100, Std: 50, P01: 0, P25: 50, P50: 100, P75: 150, P99: 200, Max: 300},
			},
			CategoricalFeatures: map[string]map[string]float64{"c1": {"a": 0.5, "b": 0.5}},
			PredictionDist:      map[string]float64{"cat0": 0.1, "cat1": 0.9},
			Performance:         &domainedge.PerformanceSample{LatencyP95Ms: 500, ErrorRate: 0.3, HasLabel: false},
			ConceptLabels:       map[string]float64{"yes": 0.5, "no": 0.5},
		}, nil
	})
	nodes := newMemNodes()
	deploys := newMemDeploys()
	drifts := newMemDrifts()
	svc := NewService(nodes, deploys, drifts, rt, sampleSrc, nil, nil,
		Config{OfflineThreshold: time.Minute}, pub, nil, SampleMetricNames{})

	mustSaveNode(t, nodes, "n1", "t1")
	
	_ = drifts.SaveBaseline(context.Background(), &domainedge.DriftBaseline{
		DeploymentID:      "dep-x",
		NumericFeatures:   map[string]*domainedge.FeatureStat{"f1": {Mean: 0, Std: 1, P01: -2, P25: -1, P50: 0, P75: 1, P99: 2, Max: 3}},
		CategoricalFeatures: map[string]map[string]float64{"c1": {"a": 0.6, "b": 0.4}},
		PredictionDist:      map[string]float64{"cat0": 0.7, "cat1": 0.3},
		Performance:         &domainedge.PerformanceSample{LatencyP95Ms: 50, ErrorRate: 0.01, HasLabel: false},
		ConceptLabels:       map[string]float64{"yes": 0.5, "no": 0.5},
	})
	d, _ := svc.Deploy(context.Background(), "t1", "n1", "m1", "v1", domainedge.EdgeDeploySpec{Replicas: 1}, 0, true, true)
	
	_ = drifts.SaveBaseline(context.Background(), &domainedge.DriftBaseline{
		DeploymentID:      d.ID,
		NumericFeatures:   map[string]*domainedge.FeatureStat{"f1": {Mean: 0, Std: 1, P01: -2, P25: -1, P50: 0, P75: 1, P99: 2, Max: 3}},
		CategoricalFeatures: map[string]map[string]float64{"c1": {"a": 0.6, "b": 0.4}},
		PredictionDist:      map[string]float64{"cat0": 0.7, "cat1": 0.3},
		Performance:         &domainedge.PerformanceSample{LatencyP95Ms: 50, ErrorRate: 0.01, HasLabel: false},
		ConceptLabels:       map[string]float64{"yes": 0.5, "no": 0.5},
	})
	rep, err := svc.EvaluateDrift(context.Background(), "t1", d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.TriggeredRollback {
		t.Fatalf("expected drift to trigger rollback, severity=%s", rep.OverallSeverity)
	}
	after, _ := deploys.Get(context.Background(), d.ID)
	if after.Status != domainedge.EdgeDeployRolledBack {
		t.Fatalf("expected rolled_back status, got %s", after.Status)
	}
	if pub.rolledBack == 0 {
		t.Fatal("expected DeploymentRolledBack event published")
	}
}

func NewMockRuntimeForTest() domainedge.EdgeRuntime { return &recordingRuntime{rolls: map[string]string{}} }

type recordingRuntime struct {
	rolls map[string]string
}

func (r *recordingRuntime) PushDeployment(_ context.Context, _ *domainedge.EdgeNode, d *domainedge.EdgeDeployment) (domainedge.EdgePushResult, error) {
	return domainedge.EdgePushResult{Accepted: true, RuntimeID: "rt-" + d.ID}, nil
}
func (r *recordingRuntime) Status(_ context.Context, _ *domainedge.EdgeNode, _ *domainedge.EdgeDeployment) (domainedge.EdgeRuntimeStatus, error) {
	return domainedge.EdgeRuntimeStatus{Found: true, Ready: true}, nil
}
func (r *recordingRuntime) Rollback(_ context.Context, _ *domainedge.EdgeNode, d *domainedge.EdgeDeployment, toVersion string) error {
	r.rolls[d.ID] = toVersion
	return nil
}
func (r *recordingRuntime) Heartbeat(_ context.Context, _ *domainedge.EdgeNode) (domainedge.EdgeNodeHealth, error) {
	return domainedge.EdgeNodeHealth{Online: true}, nil
}

type driftSampleSourceFunc func(ctx context.Context, d *domainedge.EdgeDeployment, window string) (*domainedge.DriftSample, error)

func (f driftSampleSourceFunc) Sample(ctx context.Context, d *domainedge.EdgeDeployment, window string) (*domainedge.DriftSample, error) {
	return f(ctx, d, window)
}

func mustSaveNode(t *testing.T, nodes *memNodes, id, tid string) {
	t.Helper()
	_ = nodes.Save(context.Background(), &domainedge.EdgeNode{ID: id, TenantID: tid, Name: id, Mode: domainedge.NodeModeAgent})
}