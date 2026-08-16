package storage

import (
	"context"
	"os"
	"testing"
	"time"

	domainedge "fuze-ai-paas/backend/internal/domain/edge"
	"fuze-ai-paas/backend/internal/models"
)

func newEdgeStorage(t *testing.T) *Storage {
	t.Helper()
	f, err := os.CreateTemp("", "fuze-edge-store-*.db")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	_ = f.Close()
	_ = os.Remove(path)
	db, err := NewSQLiteDBAt(path)
	if err != nil {
		t.Fatalf("NewSQLiteDBAt: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	return NewStorage(db)
}

func TestEdgeStorageNodeDeployDriftRoundTrip(t *testing.T) {
	store := newEdgeStorage(t)
	nodeRepo, deployRepo, driftRepo, _ := store.Edge()
	ctx := context.Background()

	n := &domainedge.EdgeNode{ID: "n1", TenantID: "t1", Name: "e1", Mode: domainedge.NodeModeAgent, Labels: map[string]string{"region": "cn"}}
	if err := nodeRepo.Save(ctx, n); err != nil {
		t.Fatal(err)
	}
	got, err := nodeRepo.Get(ctx, "n1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "e1" || got.Labels["region"] != "cn" {
		t.Fatalf("node round-trip mismatch: %+v", got)
	}
	list, _ := nodeRepo.List(ctx, "t1")
	if len(list) != 1 {
		t.Fatalf("expected 1 node, got %d", len(list))
	}

	d := &domainedge.EdgeDeployment{
		ID: "d1", TenantID: "t1", NodeID: "n1", ModelID: "m1", Version: "v1",
		DesiredSpec:   domainedge.EdgeDeploySpec{Image: "img:latest", Replicas: 2, Env: map[string]string{"K": "V"}},
		ActiveVersion: "v1", CanaryWeight: 10, Status: domainedge.EdgeDeployActive, AutoRollback: true, DriftGuardEnabled: true,
	}
	if err := deployRepo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	gotD, err := deployRepo.Get(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if gotD.DesiredSpec.Image != "img:latest" || gotD.DesiredSpec.Replicas != 2 || gotD.DesiredSpec.Env["K"] != "V" {
		t.Fatalf("deploy spec round-trip mismatch: %+v", gotD.DesiredSpec)
	}
	if !gotD.AutoRollback || !gotD.DriftGuardEnabled || gotD.CanaryWeight != 10 {
		t.Fatalf("deploy flags mismatch: %+v", gotD)
	}

	base := &domainedge.DriftBaseline{
		DeploymentID: "d1",
		NumericFeatures: map[string]*domainedge.FeatureStat{"f1": {Mean: 1, Std: 2}},
		PredictionDist:  map[string]float64{"cat0": 0.5},
		Performance:     &domainedge.PerformanceSample{LatencyP95Ms: 50, HasLabel: true},
	}
	if err := driftRepo.SaveBaseline(ctx, base); err != nil {
		t.Fatal(err)
	}
	gb, err := driftRepo.GetBaseline(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if gb.NumericFeatures["f1"].Mean != 1 || gb.PredictionDist["cat0"] != 0.5 {
		t.Fatalf("baseline round-trip mismatch: %+v", gb)
	}

	rep := &domainedge.DriftReport{
		ID: "r1", TenantID: "t1", DeploymentID: "d1", NodeID: "n1",
		OverallSeverity:  domainedge.DriftSeverityHigh, TriggeredRollback: true,
		DataDrift: &domainedge.DriftMetric{Type: domainedge.DriftTypeData, Score: 0.5, Threshold: 0.2, Drifted: true},
	}
	if err := driftRepo.SaveReport(ctx, rep); err != nil {
		t.Fatal(err)
	}
	gr, err := driftRepo.LatestByDeployment(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if gr.OverallSeverity != domainedge.DriftSeverityHigh || !gr.TriggeredRollback || gr.DataDrift == nil {
		t.Fatalf("report round-trip mismatch: %+v", gr)
	}
}

func TestEdgeStorageNotFound(t *testing.T) {
	store := newEdgeStorage(t)
	nodeRepo, _, driftRepo, _ := store.Edge()
	ctx := context.Background()
	if _, err := nodeRepo.Get(ctx, "missing"); err != domainedge.ErrNodeNotFound {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}
	if _, err := driftRepo.GetBaseline(ctx, "missing"); err != domainedge.ErrBaselineNotFound {
		t.Fatalf("expected ErrBaselineNotFound, got %v", err)
	}
}

func TestEdgeStorageCorruptBaselineJSON(t *testing.T) {
	store := newEdgeStorage(t)
	_, _, driftRepo, _ := store.Edge()
	ctx := context.Background()

	if err := store.db.WithContext(ctx).Create(&models.DriftBaselineRow{
		DeploymentID: "d-bad",
		Performance:   "{not-valid-json",
	}).Error; err != nil {
		t.Fatalf("seed corrupt baseline: %v", err)
	}

	_, err := driftRepo.GetBaseline(ctx, "d-bad")
	if err == nil {
		t.Fatal("expected error for corrupt baseline JSON, got nil")
	}
}

func TestEdgeStorageLabelFeedback(t *testing.T) {
	store := newEdgeStorage(t)
	_, _, _, labelRepo := store.Edge()
	ctx := context.Background()

	since := time.Now().UTC().Add(-time.Hour)
	if err := labelRepo.Record(ctx, &domainedge.LabelFeedback{TenantID: "t1", DeploymentID: "d1", Label: "cat1"}); err != nil {
		t.Fatal(err)
	}
	if err := labelRepo.Record(ctx, &domainedge.LabelFeedback{TenantID: "t1", DeploymentID: "d1", Label: "cat1"}); err != nil {
		t.Fatal(err)
	}
	if err := labelRepo.Record(ctx, &domainedge.LabelFeedback{TenantID: "t1", DeploymentID: "d1", Label: "cat2"}); err != nil {
		t.Fatal(err)
	}
	
	if err := labelRepo.Record(ctx, &domainedge.LabelFeedback{TenantID: "t2", DeploymentID: "d1", Label: "catX"}); err != nil {
		t.Fatal(err)
	}

	agg, err := labelRepo.Aggregate(ctx, "t1", "d1", since)
	if err != nil {
		t.Fatal(err)
	}
	if agg["cat1"] != 2 || agg["cat2"] != 1 {
		t.Fatalf("aggregate mismatch: %+v", agg)
	}
	
	old, err := labelRepo.Aggregate(ctx, "t1", "d1", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(old) != 0 {
		t.Fatalf("expected empty aggregate for future window, got %+v", old)
	}
	_ = time.Now 
}