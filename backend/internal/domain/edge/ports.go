package edge

import (
	"context"
	"time"
)

type NodeRepository interface {
	Save(ctx context.Context, n *EdgeNode) error
	Get(ctx context.Context, id string) (*EdgeNode, error)
	List(ctx context.Context, tenantID string) ([]*EdgeNode, error)
	Delete(ctx context.Context, id string) error
}

type DeploymentRepository interface {
	Save(ctx context.Context, d *EdgeDeployment) error
	Get(ctx context.Context, id string) (*EdgeDeployment, error)
	ListByNode(ctx context.Context, nodeID string) ([]*EdgeDeployment, error)
	List(ctx context.Context, tenantID string) ([]*EdgeDeployment, error)
}

type DriftRepository interface {
	SaveReport(ctx context.Context, r *DriftReport) error
	LatestByDeployment(ctx context.Context, deploymentID string) (*DriftReport, error)
	ListByDeployment(ctx context.Context, deploymentID string, limit int) ([]*DriftReport, error)
	SaveBaseline(ctx context.Context, b *DriftBaseline) error
	GetBaseline(ctx context.Context, deploymentID string) (*DriftBaseline, error)
}

type LabelFeedbackRepository interface {
	
	Record(ctx context.Context, f *LabelFeedback) error
	
	Aggregate(ctx context.Context, tenantID, deploymentID string, since time.Time) (map[string]int64, error)
}

type EdgePushResult struct {
	Accepted  bool
	Message   string
	RuntimeID string 
}

type EdgeRuntimeStatus struct {
	Found    bool
	Ready    bool
	Failed   bool
	Replicas int
	URL      string
}

type EdgeNodeHealth struct {
	Online      bool
	LoadPercent int
	Message     string
}

type EdgeRuntime interface {
	
	PushDeployment(ctx context.Context, node *EdgeNode, d *EdgeDeployment) (EdgePushResult, error)
	
	Status(ctx context.Context, node *EdgeNode, d *EdgeDeployment) (EdgeRuntimeStatus, error)
	
	Rollback(ctx context.Context, node *EdgeNode, d *EdgeDeployment, toVersion string) error
	
	Heartbeat(ctx context.Context, node *EdgeNode) (EdgeNodeHealth, error)
}

type DriftSampleSource interface {
	
	Sample(ctx context.Context, d *EdgeDeployment, window string) (*DriftSample, error)
}

type ConceptLabelSource interface {
	
	ConceptLabels(ctx context.Context, tenantID, deploymentID string, window time.Duration) (dist map[string]float64, hasLabel bool, err error)
}