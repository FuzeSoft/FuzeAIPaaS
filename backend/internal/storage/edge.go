package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"fuze-ai-paas/backend/internal/domain/edge"
	"fuze-ai-paas/backend/internal/models"
	"gorm.io/gorm"
)

func (s *Storage) Edge() (edge.NodeRepository, edge.DeploymentRepository, edge.DriftRepository, edge.LabelFeedbackRepository) {
	return &edgeNodeRepo{s}, &edgeDeployRepo{s}, &edgeDriftRepo{s}, &edgeLabelRepo{s}
}

type edgeNodeRepo struct{ s *Storage }

func (r *edgeNodeRepo) Save(ctx context.Context, n *edge.EdgeNode) error {
	row, err := r.s.nodeRowFromDomain(n)
	if err != nil {
		return err
	}
	return r.s.db.WithContext(ctx).Save(&row).Error
}

func (r *edgeNodeRepo) Get(ctx context.Context, id string) (*edge.EdgeNode, error) {
	var row models.EdgeNodeRow
	if err := r.s.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, edge.ErrNodeNotFound
		}
		return nil, err
	}
	return r.s.nodeToDomain(&row)
}

func (r *edgeNodeRepo) List(ctx context.Context, tenantID string) ([]*edge.EdgeNode, error) {
	var rows []models.EdgeNodeRow
	q := r.s.db.WithContext(ctx).Order("created_at desc")
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*edge.EdgeNode, 0, len(rows))
	for i := range rows {
		d, err := r.s.nodeToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (r *edgeNodeRepo) Delete(ctx context.Context, id string) error {
	return r.s.db.WithContext(ctx).Where("id = ?", id).Delete(&models.EdgeNodeRow{}).Error
}

func (s *Storage) nodeRowFromDomain(n *edge.EdgeNode) (models.EdgeNodeRow, error) {
	labels, err := json.Marshal(orEmptyMap(n.Labels))
	if err != nil {
		return models.EdgeNodeRow{}, err
	}
	row := models.EdgeNodeRow{
		ID:          n.ID,
		TenantID:    n.TenantID,
		Name:        n.Name,
		Mode:        string(n.Mode),
		Status:      string(n.Status),
		Endpoint:    n.Endpoint,
		Region:      n.Region,
		Labels:      string(labels),
		HeartbeatAt: n.HeartbeatAt,
		LastSeenAt:  n.LastSeenAt,
		CreatedAt:   n.CreatedAt,
		UpdatedAt:   n.UpdatedAt,
	}
	row.CACertPEM = n.CACertPEM
	row.ClientCertPEM = n.ClientCertPEM
	row.ClientKeyPEM = n.ClientKeyPEM
	return row, nil
}

func (s *Storage) nodeToDomain(row *models.EdgeNodeRow) (*edge.EdgeNode, error) {
	var labels map[string]string
	if row.Labels != "" {
		if err := json.Unmarshal([]byte(row.Labels), &labels); err != nil {
			return nil, err
		}
	}
	return &edge.EdgeNode{
		ID:            row.ID,
		TenantID:      row.TenantID,
		Name:          row.Name,
		Mode:          edge.NodeMode(row.Mode),
		Status:        edge.NodeStatus(row.Status),
		Endpoint:      row.Endpoint,
		Region:        row.Region,
		Labels:        labels,
		CACertPEM:     row.CACertPEM,
		ClientCertPEM: row.ClientCertPEM,
		ClientKeyPEM:  row.ClientKeyPEM,
		HeartbeatAt:   row.HeartbeatAt,
		LastSeenAt:    row.LastSeenAt,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}, nil
}

type edgeDeployRepo struct{ s *Storage }

func (r *edgeDeployRepo) Save(ctx context.Context, d *edge.EdgeDeployment) error {
	row, err := r.s.deployRowFromDomain(d)
	if err != nil {
		return err
	}
	return r.s.db.WithContext(ctx).Save(&row).Error
}

func (r *edgeDeployRepo) Get(ctx context.Context, id string) (*edge.EdgeDeployment, error) {
	var row models.EdgeDeploymentRow
	if err := r.s.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, edge.ErrDeploymentNotFound
		}
		return nil, err
	}
	return r.s.deployToDomain(&row)
}

func (r *edgeDeployRepo) ListByNode(ctx context.Context, nodeID string) ([]*edge.EdgeDeployment, error) {
	var rows []models.EdgeDeploymentRow
	if err := r.s.db.WithContext(ctx).Where("node_id = ?", nodeID).Order("created_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return r.s.deployRowsToDomain(rows)
}

func (r *edgeDeployRepo) List(ctx context.Context, tenantID string) ([]*edge.EdgeDeployment, error) {
	var rows []models.EdgeDeploymentRow
	q := r.s.db.WithContext(ctx).Order("created_at desc")
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return r.s.deployRowsToDomain(rows)
}

func (s *Storage) deployRowFromDomain(d *edge.EdgeDeployment) (models.EdgeDeploymentRow, error) {
	spec, err := json.Marshal(d.DesiredSpec)
	if err != nil {
		return models.EdgeDeploymentRow{}, err
	}
	return models.EdgeDeploymentRow{
		ID:               d.ID,
		TenantID:         d.TenantID,
		NodeID:           d.NodeID,
		ModelID:          d.ModelID,
		Version:          d.Version,
		DesiredSpec:      string(spec),
		CurrentVersion:   d.CurrentVersion,
		ActiveVersion:    d.ActiveVersion,
		CanaryVersion:    d.CanaryVersion,
		CanaryWeight:     d.CanaryWeight,
		Status:           string(d.Status),
		AutoRollback:     d.AutoRollback,
		DriftGuardEnabled: d.DriftGuardEnabled,
		CreatedAt:        d.CreatedAt,
		UpdatedAt:        d.UpdatedAt,
	}, nil
}

func (s *Storage) deployToDomain(row *models.EdgeDeploymentRow) (*edge.EdgeDeployment, error) {
	var spec edge.EdgeDeploySpec
	if row.DesiredSpec != "" {
		if err := json.Unmarshal([]byte(row.DesiredSpec), &spec); err != nil {
			return nil, err
		}
	}
	return &edge.EdgeDeployment{
		ID:               row.ID,
		TenantID:         row.TenantID,
		NodeID:           row.NodeID,
		ModelID:          row.ModelID,
		Version:          row.Version,
		DesiredSpec:      spec,
		CurrentVersion:   row.CurrentVersion,
		ActiveVersion:    row.ActiveVersion,
		CanaryVersion:    row.CanaryVersion,
		CanaryWeight:     row.CanaryWeight,
		Status:           edge.EdgeDeployStatus(row.Status),
		AutoRollback:     row.AutoRollback,
		DriftGuardEnabled: row.DriftGuardEnabled,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}, nil
}

func (s *Storage) deployRowsToDomain(rows []models.EdgeDeploymentRow) ([]*edge.EdgeDeployment, error) {
	out := make([]*edge.EdgeDeployment, 0, len(rows))
	for i := range rows {
		d, err := s.deployToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

type edgeDriftRepo struct{ s *Storage }

func (r *edgeDriftRepo) SaveReport(ctx context.Context, rep *edge.DriftReport) error {
	row, err := reportRowFromDomain(rep)
	if err != nil {
		return err
	}
	return r.s.db.WithContext(ctx).Create(&row).Error
}

func (r *edgeDriftRepo) LatestByDeployment(ctx context.Context, deploymentID string) (*edge.DriftReport, error) {
	var row models.DriftReportRow
	if err := r.s.db.WithContext(ctx).Where("deployment_id = ?", deploymentID).Order("evaluated_at desc").First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, edge.ErrDriftReportNotFound
		}
		return nil, err
	}
	return reportToDomain(&row)
}

func (r *edgeDriftRepo) ListByDeployment(ctx context.Context, deploymentID string, limit int) ([]*edge.DriftReport, error) {
	var rows []models.DriftReportRow
	q := r.s.db.WithContext(ctx).Where("deployment_id = ?", deploymentID).Order("evaluated_at desc")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*edge.DriftReport, 0, len(rows))
	for i := range rows {
		d, err := reportToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (r *edgeDriftRepo) SaveBaseline(ctx context.Context, b *edge.DriftBaseline) error {
	row, err := baselineRowFromDomain(b)
	if err != nil {
		return err
	}
	return r.s.db.WithContext(ctx).Save(&row).Error
}

func (r *edgeDriftRepo) GetBaseline(ctx context.Context, deploymentID string) (*edge.DriftBaseline, error) {
	var row models.DriftBaselineRow
	if err := r.s.db.WithContext(ctx).Where("deployment_id = ?", deploymentID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, edge.ErrBaselineNotFound
		}
		return nil, err
	}
	return baselineToDomain(&row)
}

func marshalMetricPtr(m *edge.DriftMetric) (string, error) {
	if m == nil {
		return "", nil
	}
	b, err := json.Marshal(m)
	return string(b), err
}

func unmarshalMetric(s string) (*edge.DriftMetric, error) {
	if s == "" {
		return nil, nil
	}
	var m edge.DriftMetric
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func reportRowFromDomain(r *edge.DriftReport) (models.DriftReportRow, error) {
	data, err := marshalMetricPtr(r.DataDrift)
	if err != nil {
		return models.DriftReportRow{}, err
	}
	pred, err := marshalMetricPtr(r.PredictionDrift)
	if err != nil {
		return models.DriftReportRow{}, err
	}
	perf, err := marshalMetricPtr(r.PerformanceDrift)
	if err != nil {
		return models.DriftReportRow{}, err
	}
	conc, err := marshalMetricPtr(r.ConceptDrift)
	if err != nil {
		return models.DriftReportRow{}, err
	}
	return models.DriftReportRow{
		ID:               r.ID,
		TenantID:         r.TenantID,
		DeploymentID:     r.DeploymentID,
		NodeID:           r.NodeID,
		EvaluatedAt:      r.EvaluatedAt,
		DataDrift:        data,
		PredictionDrift:  pred,
		PerformanceDrift: perf,
		ConceptDrift:     conc,
		OverallSeverity:  string(r.OverallSeverity),
		TriggeredRollback: r.TriggeredRollback,
		Recommendation:   r.Recommendation,
	}, nil
}

func reportToDomain(row *models.DriftReportRow) (*edge.DriftReport, error) {
	data, err := unmarshalMetric(row.DataDrift)
	if err != nil {
		return nil, err
	}
	pred, err := unmarshalMetric(row.PredictionDrift)
	if err != nil {
		return nil, err
	}
	perf, err := unmarshalMetric(row.PerformanceDrift)
	if err != nil {
		return nil, err
	}
	conc, err := unmarshalMetric(row.ConceptDrift)
	if err != nil {
		return nil, err
	}
	return &edge.DriftReport{
		ID:               row.ID,
		TenantID:         row.TenantID,
		DeploymentID:     row.DeploymentID,
		NodeID:           row.NodeID,
		EvaluatedAt:      row.EvaluatedAt,
		DataDrift:        data,
		PredictionDrift:  pred,
		PerformanceDrift: perf,
		ConceptDrift:     conc,
		OverallSeverity:  edge.DriftSeverity(row.OverallSeverity),
		TriggeredRollback: row.TriggeredRollback,
		Recommendation:   row.Recommendation,
	}, nil
}

func baselineRowFromDomain(b *edge.DriftBaseline) (models.DriftBaselineRow, error) {
	num, err := json.Marshal(orEmptyMap(b.NumericFeatures))
	if err != nil {
		return models.DriftBaselineRow{}, fmt.Errorf("marshal NumericFeatures: %w", err)
	}
	cat, err := json.Marshal(orEmptyMap(b.CategoricalFeatures))
	if err != nil {
		return models.DriftBaselineRow{}, fmt.Errorf("marshal CategoricalFeatures: %w", err)
	}
	pred, err := json.Marshal(orEmptyMap(b.PredictionDist))
	if err != nil {
		return models.DriftBaselineRow{}, fmt.Errorf("marshal PredictionDist: %w", err)
	}
	perf, err := json.Marshal(b.Performance)
	if err != nil {
		return models.DriftBaselineRow{}, fmt.Errorf("marshal Performance: %w", err)
	}
	conc, err := json.Marshal(orEmptyMap(b.ConceptLabels))
	if err != nil {
		return models.DriftBaselineRow{}, fmt.Errorf("marshal ConceptLabels: %w", err)
	}
	return models.DriftBaselineRow{
		DeploymentID:      b.DeploymentID,
		ReferenceWindow:   b.ReferenceWindow,
		NumericFeatures:   string(num),
		CategoricalFeatures: string(cat),
		PredictionDist:    string(pred),
		Performance:       string(perf),
		ConceptLabels:     string(conc),
	}, nil
}

func baselineToDomain(row *models.DriftBaselineRow) (*edge.DriftBaseline, error) {
	var num map[string]*edge.FeatureStat
	var cat map[string]map[string]float64
	var pred, conc map[string]float64
	var perf *edge.PerformanceSample
	
	if err := unmarshalInto(row.NumericFeatures, &num); err != nil {
		return nil, fmt.Errorf("baseline %s: decode NumericFeatures: %w", row.DeploymentID, err)
	}
	if err := unmarshalInto(row.CategoricalFeatures, &cat); err != nil {
		return nil, fmt.Errorf("baseline %s: decode CategoricalFeatures: %w", row.DeploymentID, err)
	}
	if err := unmarshalInto(row.PredictionDist, &pred); err != nil {
		return nil, fmt.Errorf("baseline %s: decode PredictionDist: %w", row.DeploymentID, err)
	}
	if err := unmarshalInto(row.ConceptLabels, &conc); err != nil {
		return nil, fmt.Errorf("baseline %s: decode ConceptLabels: %w", row.DeploymentID, err)
	}
	if err := unmarshalInto(row.Performance, &perf); err != nil {
		return nil, fmt.Errorf("baseline %s: decode Performance: %w", row.DeploymentID, err)
	}
	return &edge.DriftBaseline{
		DeploymentID:      row.DeploymentID,
		ReferenceWindow:   row.ReferenceWindow,
		NumericFeatures:   num,
		CategoricalFeatures: cat,
		PredictionDist:    pred,
		Performance:       perf,
		ConceptLabels:     conc,
	}, nil
}

func unmarshalInto(s string, v interface{}) error {
	if s == "" {
		return nil
	}
	return json.Unmarshal([]byte(s), v)
}

func orEmptyMap[V any](m map[string]V) map[string]V {
	if m == nil {
		return map[string]V{}
	}
	return m
}

type edgeLabelRepo struct{ s *Storage }

func (r *edgeLabelRepo) Record(ctx context.Context, f *edge.LabelFeedback) error {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	if f.FeedbackAt.IsZero() {
		f.FeedbackAt = r.s.Now().UTC()
	}
	row := models.EdgeLabelFeedbackRow{
		ID:           f.ID,
		TenantID:     f.TenantID,
		DeploymentID: f.DeploymentID,
		Label:        f.Label,
		RequestID:    f.RequestID,
		FeedbackAt:   f.FeedbackAt,
	}
	return r.s.db.WithContext(ctx).Create(&row).Error
}

func (r *edgeLabelRepo) Aggregate(ctx context.Context, tenantID, deploymentID string, since time.Time) (map[string]int64, error) {
	type cnt struct {
		Label string
		C     int64
	}
	sinceUTC := since.UTC()
	var rows []cnt
	q := r.s.db.WithContext(ctx).
		Model(&models.EdgeLabelFeedbackRow{}).
		Select("label, count(*) as c").
		Where("tenant_id = ? AND deployment_id = ? AND feedback_at >= ?", tenantID, deploymentID, sinceUTC)
	if err := q.Group("label").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		if r.Label == "" {
			continue
		}
		out[r.Label] = r.C
	}
	return out, nil
}