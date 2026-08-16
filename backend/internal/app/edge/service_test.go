package edge

import (
	"context"
	"sync"
	"testing"
	"time"

	domainedge "fuze-ai-paas/backend/internal/domain/edge"
	edgek8s "fuze-ai-paas/backend/internal/k8s/edge"
	"fuze-ai-paas/backend/internal/ports"
)

type tNodes struct {
	mu sync.Mutex
	m  map[string]*domainedge.EdgeNode
}

func (r *tNodes) Save(_ context.Context, n *domainedge.EdgeNode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[n.ID] = n
	return nil
}
func (r *tNodes) Get(_ context.Context, id string) (*domainedge.EdgeNode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n, ok := r.m[id]; ok {
		return n, nil
	}
	return nil, domainedge.ErrNodeNotFound
}
func (r *tNodes) List(_ context.Context, _ string) ([]*domainedge.EdgeNode, error) { return nil, nil }
func (r *tNodes) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, id)
	return nil
}

type tDeploys struct {
	mu sync.Mutex
	m  map[string]*domainedge.EdgeDeployment
}

func (r *tDeploys) Save(_ context.Context, d *domainedge.EdgeDeployment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[d.ID] = d
	return nil
}
func (r *tDeploys) Get(_ context.Context, id string) (*domainedge.EdgeDeployment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d, ok := r.m[id]; ok {
		return d, nil
	}
	return nil, domainedge.ErrDeploymentNotFound
}
func (r *tDeploys) ListByNode(_ context.Context, _ string) ([]*domainedge.EdgeDeployment, error) { return nil, nil }
func (r *tDeploys) List(_ context.Context, _ string) ([]*domainedge.EdgeDeployment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*domainedge.EdgeDeployment{}
	for _, d := range r.m {
		out = append(out, d)
	}
	return out, nil
}

type tDrifts struct {
	mu       sync.Mutex
	reports  map[string][]*domainedge.DriftReport
	baseline map[string]*domainedge.DriftBaseline
}

func (r *tDrifts) SaveReport(_ context.Context, rep *domainedge.DriftReport) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports[rep.DeploymentID] = append([]*domainedge.DriftReport{rep}, r.reports[rep.DeploymentID]...)
	return nil
}
func (r *tDrifts) LatestByDeployment(_ context.Context, did string) (*domainedge.DriftReport, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rs, ok := r.reports[did]; ok && len(rs) > 0 {
		return rs[0], nil
	}
	return nil, domainedge.ErrDriftReportNotFound
}
func (r *tDrifts) ListByDeployment(_ context.Context, did string, _ int) ([]*domainedge.DriftReport, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reports[did], nil
}
func (r *tDrifts) SaveBaseline(_ context.Context, b *domainedge.DriftBaseline) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.baseline[b.DeploymentID] = b
	return nil
}
func (r *tDrifts) GetBaseline(_ context.Context, did string) (*domainedge.DriftBaseline, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.baseline[did]; ok {
		return b, nil
	}
	return nil, domainedge.ErrBaselineNotFound
}

func TestAppEdgeSubmitSampleTriggersAutoRollback(t *testing.T) {
	nodes := &tNodes{m: map[string]*domainedge.EdgeNode{}}
	deploys := &tDeploys{m: map[string]*domainedge.EdgeDeployment{}}
	drifts := &tDrifts{reports: map[string][]*domainedge.DriftReport{}, baseline: map[string]*domainedge.DriftBaseline{}}

	svc := NewService(nodes, deploys, drifts, edgek8s.NewMockRuntime(), nil, nil, nil,
		Config{OfflineThreshold: time.Minute}, nil, nil, SampleMetricNames{})

	_ = nodes.Save(context.Background(), &domainedge.EdgeNode{ID: "n1", TenantID: "t1", Name: "e1", Mode: domainedge.NodeModeAgent})
	_, _ = svc.Deploy(context.Background(), "t1", "n1", "m1", "v1", domainedge.EdgeDeploySpec{Replicas: 1}, 0, true, true)
	d, _ := deploys.Get(context.Background(), "")
	
	for _, v := range deploys.m {
		d = v
	}
	_ = drifts.SaveBaseline(context.Background(), &domainedge.DriftBaseline{
		DeploymentID:   d.ID,
		NumericFeatures: map[string]*domainedge.FeatureStat{"f1": {Mean: 0, Std: 1, P50: 0}},
		PredictionDist:  map[string]float64{"cat0": 0.7, "cat1": 0.3},
		Performance:     &domainedge.PerformanceSample{LatencyP95Ms: 50, HasLabel: false},
	})

	sample := &domainedge.DriftSample{
		NumericFeatures: map[string]*domainedge.FeatureStat{"f1": {Mean: 100, Std: 50, P50: 100}},
		PredictionDist:  map[string]float64{"cat0": 0.1, "cat1": 0.9},
		Performance:     &domainedge.PerformanceSample{LatencyP95Ms: 600, HasLabel: false},
	}
	rep, err := svc.SubmitSample(context.Background(), "t1", d.ID, sample)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.TriggeredRollback {
		t.Fatalf("expected auto-rollback triggered, severity=%s", rep.OverallSeverity)
	}
	after, _ := deploys.Get(context.Background(), d.ID)
	if after.Status != domainedge.EdgeDeployRolledBack {
		t.Fatalf("expected rolled_back status, got %s", after.Status)
	}
}

type stubMetrics struct{ latency float64 }

func (m *stubMetrics) QueryRange(_ ports.MetricQuery) ([]ports.MetricSeries, error) {
	return []ports.MetricSeries{{Labels: map[string]string{"gpu": "0"}, Samples: []ports.MetricSample{{Timestamp: time.Now().UnixMilli(), Value: m.latency}}}}, nil
}
func (m *stubMetrics) QueryLatest(_ ports.MetricQuery) (*ports.MetricSample, error) {
	return &ports.MetricSample{Timestamp: time.Now().UnixMilli(), Value: m.latency}, nil
}
func (m *stubMetrics) Alerts() ([]ports.ActiveAlert, error) { return nil, nil }

func TestSubmitSampleDoesNotMutateSharedSampleSource(t *testing.T) {
	nodes := &tNodes{m: map[string]*domainedge.EdgeNode{}}
	deploys := &tDeploys{m: map[string]*domainedge.EdgeDeployment{}}
	drifts := &tDrifts{reports: map[string][]*domainedge.DriftReport{}, baseline: map[string]*domainedge.DriftBaseline{}}
	svc := NewService(nodes, deploys, drifts, edgek8s.NewMockRuntime(), nil, nil, nil,
		Config{OfflineThreshold: time.Minute}, nil, &stubMetrics{latency: 10}, SampleMetricNames{})

	ctx := context.Background()
	
	_ = nodes.Save(ctx, &domainedge.EdgeNode{ID: "n1", TenantID: "t1", Name: "e1", Mode: domainedge.NodeModeAgent})
	dep, err := svc.Deploy(ctx, "t1", "n1", "m1", "v1", domainedge.EdgeDeploySpec{Replicas: 1}, 0, true, true)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	_ = drifts.SaveBaseline(ctx, &domainedge.DriftBaseline{
		DeploymentID:   dep.ID,
		NumericFeatures: map[string]*domainedge.FeatureStat{"f1": {Mean: 0, Std: 1, P50: 0}},
		PredictionDist:  map[string]float64{"cat0": 0.7, "cat1": 0.3},
		Performance:     &domainedge.PerformanceSample{LatencyP95Ms: 50, HasLabel: false},
	})

	if rep, err := svc.EvaluateDrift(ctx, "t1", dep.ID); err != nil || rep == nil {
		t.Fatalf("periodic EvaluateDrift: err=%v rep=%v", err, rep)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = svc.SubmitSample(ctx, "t1", dep.ID, &domainedge.DriftSample{
				NumericFeatures: map[string]*domainedge.FeatureStat{"f1": {Mean: 9999, Std: 50, P50: 9999}},
			})
		}()
		go func() {
			defer wg.Done()
			if r, e := svc.EvaluateDrift(ctx, "t1", dep.ID); e != nil || r == nil {
				t.Errorf("concurrent EvaluateDrift: err=%v rep=%v", e, r)
			}
		}()
	}
	wg.Wait()
}