
package edge

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	domainedge "fuze-ai-paas/backend/internal/domain/edge"
	"fuze-ai-paas/backend/internal/ports"
)

type Service struct {
	nodes   domainedge.NodeRepository
	deploys domainedge.DeploymentRepository
	drifts  domainedge.DriftRepository
	runtime domainedge.EdgeRuntime
	
	detector *domainedge.Detector
	
	sampleSrc domainedge.DriftSampleSource
	
	conceptSrc domainedge.ConceptLabelSource
	
	labelRepo domainedge.LabelFeedbackRepository
	
	pub domainedge.Publisher
	
	OfflineThreshold time.Duration
	
	evalMu sync.Mutex
}

type Config struct {
	OfflineThreshold time.Duration
	DetectorOpts     []domainedge.DetectorOption
}

func NewService(
	nodes domainedge.NodeRepository,
	deploys domainedge.DeploymentRepository,
	drifts domainedge.DriftRepository,
	runtime domainedge.EdgeRuntime,
	sampleSrc domainedge.DriftSampleSource,
	conceptSrc domainedge.ConceptLabelSource,
	labelRepo domainedge.LabelFeedbackRepository,
	cfg Config,
	pub domainedge.Publisher,
	metrics ports.MetricsQuery,
	names SampleMetricNames,
) *Service {
	offline := cfg.OfflineThreshold
	if offline <= 0 {
		offline = 2 * time.Minute
	}
	svc := &Service{
		nodes:           nodes,
		deploys:         deploys,
		drifts:          drifts,
		runtime:         runtime,
		sampleSrc:       sampleSrc,
		conceptSrc:      conceptSrc,
		labelRepo:       labelRepo,
		pub:             pub,
		OfflineThreshold: offline,
		detector:        domainedge.NewDetector(cfg.DetectorOpts...),
	}
	if metrics != nil {
		src := NewMetricsBackedSampleSource(metrics, names)
		if labelRepo != nil {
			src = src.WithConceptSource(&repoConceptLabelSource{repo: labelRepo})
		}
		svc.sampleSrc = src
	} else if labelRepo != nil {
		
		svc.conceptSrc = &repoConceptLabelSource{repo: labelRepo}
	}
	return svc
}

type RegisterNodeInput struct {
	ID            string
	Name          string
	Mode          domainedge.NodeMode
	Endpoint      string
	Region        string
	Labels        map[string]string
	CACertPEM     string
	ClientCertPEM string
	ClientKeyPEM  string
}

func (s *Service) RegisterNode(ctx context.Context, tenantID string, in RegisterNodeInput) (*domainedge.EdgeNode, error) {
	n := &domainedge.EdgeNode{
		ID:            in.ID,
		TenantID:      tenantID,
		Name:          in.Name,
		Mode:          in.Mode,
		Endpoint:      in.Endpoint,
		Region:        in.Region,
		Labels:        in.Labels,
		CACertPEM:     in.CACertPEM,
		ClientCertPEM: in.ClientCertPEM,
		ClientKeyPEM:  in.ClientKeyPEM,
	}
	n.Register()
	if err := s.nodes.Save(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}

func (s *Service) ListNodes(ctx context.Context, tenantID string) ([]*domainedge.EdgeNode, error) {
	return s.nodes.List(ctx, tenantID)
}

func (s *Service) GetNode(ctx context.Context, tenantID, id string) (*domainedge.EdgeNode, error) {
	n, err := s.nodes.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if n.TenantID != tenantID {
		return nil, domainedge.ErrNodeNotFound
	}
	return n, nil
}

func (s *Service) DeregisterNode(ctx context.Context, tenantID, id string) error {
	n, err := s.GetNode(ctx, tenantID, id)
	if err != nil {
		return err
	}
	n.MarkDecommissioning()
	return s.nodes.Save(ctx, n)
}

func (s *Service) Heartbeat(ctx context.Context, tenantID, id string) (*domainedge.EdgeNode, error) {
	n, err := s.GetNode(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	n.Heartbeat(time.Now().UTC(), s.OfflineThreshold)
	if err := s.nodes.Save(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}

func (s *Service) ReconcileNode(ctx context.Context, tenantID, nodeID string, asOf time.Time) (*domainedge.EdgeNode, error) {
	n, err := s.GetNode(ctx, tenantID, nodeID)
	if err != nil {
		return nil, err
	}
	n.RecomputeLiveness(asOf, s.OfflineThreshold)
	if err := s.nodes.Save(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}

func (s *Service) Deploy(ctx context.Context, tenantID, nodeID, modelID, version string, spec domainedge.EdgeDeploySpec, canaryWeight int, autoRollback, driftGuard bool) (*domainedge.EdgeDeployment, error) {
	if _, err := s.GetNode(ctx, tenantID, nodeID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	d := &domainedge.EdgeDeployment{
		ID:                generateEdgeDeployID(),
		TenantID:          tenantID,
		NodeID:            nodeID,
		ModelID:           modelID,
		Version:           version,
		DesiredSpec:       spec,
		CurrentVersion:    version,
		ActiveVersion:     version,
		CanaryWeight:      canaryWeight,
		Status:            domainedge.EdgeDeployPending,
		AutoRollback:      autoRollback,
		DriftGuardEnabled: driftGuard,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if canaryWeight > 0 {
		d.CanaryVersion = version
		d.Status = domainedge.EdgeDeployDeploying
	}
	if prev := s.previousActiveVersion(ctx, nodeID, modelID, version); prev != "" {
		d.ActiveVersion = prev
	}
	if err := s.deploys.Save(ctx, d); err != nil {
		return nil, err
	}
	node, err := s.nodes.Get(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	res, err := s.runtime.PushDeployment(ctx, node, d)
	if err != nil {
		d.MarkFailing()
		if serr := s.deploys.Save(ctx, d); serr != nil {
			return d, fmt.Errorf("push deployment failed: %w; persist failing status: %v", err, serr)
		}
		return d, err
	}
	if res.Accepted {
		if canaryWeight > 0 {
			d.MarkDeploying()
		} else {
			d.MarkActive()
		}
	}
	if err := s.deploys.Save(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Service) PromoteCanary(ctx context.Context, tenantID, deploymentID string, step int) (*domainedge.EdgeDeployment, error) {
	d, err := s.GetDeployment(ctx, tenantID, deploymentID)
	if err != nil {
		return nil, err
	}
	node, err := s.nodes.Get(ctx, d.NodeID)
	if err != nil {
		return nil, err
	}
	full := d.PromoteCanary(step)
	if err := s.deploys.Save(ctx, d); err != nil {
		return nil, err
	}
	if full {
		d.CompleteCanary()
		if _, err := s.runtime.PushDeployment(ctx, node, d); err != nil {
			d.MarkDegraded()
		}
		if err := s.deploys.Save(ctx, d); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (s *Service) Rollback(ctx context.Context, tenantID, deploymentID, reason string) (*domainedge.EdgeDeployment, error) {
	d, err := s.GetDeployment(ctx, tenantID, deploymentID)
	if err != nil {
		return nil, err
	}
	node, err := s.nodes.Get(ctx, d.NodeID)
	if err != nil {
		return nil, err
	}
	if err := s.runtime.Rollback(ctx, node, d, d.ActiveVersion); err != nil {
		return d, err
	}
	from := d.CurrentVersion
	d.RollbackTo()
	if err := s.deploys.Save(ctx, d); err != nil {
		return nil, err
	}
	if s.pub != nil {
		s.pub.Publish(domainedge.DeploymentRolledBack{
			DeploymentID: d.ID, NodeID: d.NodeID, TenantID: d.TenantID,
			FromVersion: from, ToVersion: d.ActiveVersion, Reason: reason, At: time.Now().UTC(),
		})
	}
	return d, nil
}

func (s *Service) SetGuard(ctx context.Context, tenantID, deploymentID string, driftGuard, autoRollback bool) (*domainedge.EdgeDeployment, error) {
	s.evalMu.Lock()
	defer s.evalMu.Unlock()
	d, err := s.GetDeployment(ctx, tenantID, deploymentID)
	if err != nil {
		return nil, err
	}
	d.DriftGuardEnabled = driftGuard
	d.AutoRollback = autoRollback
	if err := s.deploys.Save(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Service) GetDeployment(ctx context.Context, tenantID, id string) (*domainedge.EdgeDeployment, error) {
	d, err := s.deploys.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if d.TenantID != tenantID {
		return nil, domainedge.ErrDeploymentNotFound
	}
	return d, nil
}

func (s *Service) ListDeployments(ctx context.Context, tenantID string) ([]*domainedge.EdgeDeployment, error) {
	return s.deploys.List(ctx, tenantID)
}

func (s *Service) EvaluateDrift(ctx context.Context, tenantID, deploymentID string) (*domainedge.DriftReport, error) {
	d, err := s.GetDeployment(ctx, tenantID, deploymentID)
	if err != nil {
		return nil, err
	}
	return s.evaluateDrift(ctx, d, nil)
}

func (s *Service) SubmitSample(ctx context.Context, tenantID, id string, sample *domainedge.DriftSample) (*domainedge.DriftReport, error) {
	d, err := s.GetDeployment(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return s.evaluateDrift(ctx, d, sample)
}

func (s *Service) evaluateDrift(ctx context.Context, d *domainedge.EdgeDeployment, sample *domainedge.DriftSample) (*domainedge.DriftReport, error) {
	s.evalMu.Lock()
	defer s.evalMu.Unlock()
	base, err := s.drifts.GetBaseline(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	if sample == nil {
		if s.sampleSrc == nil {
			return nil, domainedge.ErrMissingSampleSource
		}
		sample, err = s.sampleSrc.Sample(ctx, d, "15m")
		if err != nil {
			return nil, err
		}
	}
	report := s.detector.Evaluate(d.TenantID, d.ID, d.NodeID, base, sample)
	if err := s.drifts.SaveReport(ctx, report); err != nil {
		return nil, err
	}
	if report.TriggeredRollback && d.DriftGuardEnabled && d.AutoRollback {
		if rerr := s.autoRollback(ctx, d, report); rerr != nil {
			return report, rerr
		}
	}
	if s.pub != nil {
		s.pub.Publish(domainedge.DriftDetected{
			DeploymentID:      d.ID, NodeID: d.NodeID, TenantID: d.TenantID,
			Severity:          report.OverallSeverity,
			TriggeredRollback: report.TriggeredRollback, EvaluatedAt: report.EvaluatedAt,
		})
	}
	return report, nil
}

func (s *Service) autoRollback(ctx context.Context, d *domainedge.EdgeDeployment, report *domainedge.DriftReport) error {
	node, nerr := s.nodes.Get(ctx, d.NodeID)
	if nerr != nil {
		return fmt.Errorf("drift auto-rollback: node lookup failed: %w", nerr)
	}
	if rerr := s.runtime.Rollback(ctx, node, d, d.ActiveVersion); rerr != nil {
		return fmt.Errorf("drift auto-rollback to %s failed: %w", d.ActiveVersion, rerr)
	}
	from := d.CurrentVersion
	d.RollbackTo()
	if serr := s.deploys.Save(ctx, d); serr != nil {
		
		return fmt.Errorf("runtime rolled back to %s but persist failed: %w", d.ActiveVersion, serr)
	}
	if s.pub != nil {
		s.pub.Publish(domainedge.DeploymentRolledBack{
			DeploymentID: d.ID, NodeID: d.NodeID, TenantID: d.TenantID,
			FromVersion: from, ToVersion: d.ActiveVersion,
			Reason: "auto-rollback on drift (" + string(report.OverallSeverity) + ")",
			At:     time.Now().UTC(),
		})
	}
	return nil
}

func (s *Service) previousActiveVersion(ctx context.Context, nodeID, modelID, currentVersion string) string {
	prevList, err := s.deploys.ListByNode(ctx, nodeID)
	if err != nil || len(prevList) == 0 {
		return ""
	}
	for _, p := range prevList {
		if p.ModelID == modelID && p.Version != currentVersion {
			if p.ActiveVersion != "" {
				return p.ActiveVersion
			}
			return p.Version
		}
	}
	return ""
}

func (s *Service) SubmitLabelFeedback(ctx context.Context, tenantID, deploymentID, label, requestID string) error {
	if s.labelRepo == nil {
		return domainedge.ErrLabelFeedbackNotConfigured
	}
	if _, err := s.GetDeployment(ctx, tenantID, deploymentID); err != nil {
		return err
	}
	f := &domainedge.LabelFeedback{
		ID:           uuid.NewString(),
		TenantID:     tenantID,
		DeploymentID: deploymentID,
		Label:        label,
		RequestID:    requestID,
	}
	return s.labelRepo.Record(ctx, f)
}

func (s *Service) LatestDrift(ctx context.Context, tenantID, id string) (*domainedge.DriftReport, error) {
	if _, err := s.GetDeployment(ctx, tenantID, id); err != nil {
		return nil, err
	}
	return s.drifts.LatestByDeployment(ctx, id)
}

func (s *Service) SetBaseline(ctx context.Context, tenantID, id string, b *domainedge.DriftBaseline) error {
	if _, err := s.GetDeployment(ctx, tenantID, id); err != nil {
		return err
	}
	b.DeploymentID = id
	return s.drifts.SaveBaseline(ctx, b)
}

func (s *Service) SetSampleSource(src domainedge.DriftSampleSource) { s.sampleSrc = src }

func generateEdgeDeployID() string {
	return "edge-dep-" + uuid.NewString()
}

type repoConceptLabelSource struct {
	repo domainedge.LabelFeedbackRepository
}

func (s *repoConceptLabelSource) ConceptLabels(ctx context.Context, tenantID, deploymentID string, window time.Duration) (map[string]float64, bool, error) {
	if s.repo == nil {
		return nil, false, domainedge.ErrLabelFeedbackNotConfigured
	}
	from := time.Now().UTC().Add(-window)
	counts, err := s.repo.Aggregate(ctx, tenantID, deploymentID, from)
	if err != nil {
		return nil, false, err
	}
	total := int64(0)
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		return nil, false, nil
	}
	dist := make(map[string]float64, len(counts))
	for k, c := range counts {
		dist[k] = float64(c) / float64(total)
	}
	return dist, true, nil
}