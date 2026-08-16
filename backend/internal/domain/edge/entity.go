
package edge

import (
	"errors"
	"time"
)

var (
	ErrNodeNotFound        = errors.New("edge node not found")
	ErrDeploymentNotFound  = errors.New("edge deployment not found")
	ErrDriftReportNotFound = errors.New("drift report not found")
	ErrBaselineNotFound    = errors.New("drift baseline not found")
	ErrMissingSampleSource = errors.New("drift sample source not configured")
	ErrLabelFeedbackNotConfigured = errors.New("label feedback repository not configured")
)

type NodeMode string

const (
	
	NodeModeAgent NodeMode = "agent"
	
	NodeModeKubeEdge NodeMode = "kubeedge"
)

type NodeStatus string

const (
	NodeStatusPending        NodeStatus = "pending"        
	NodeStatusOnline         NodeStatus = "online"         
	NodeStatusOffline        NodeStatus = "offline"        
	NodeStatusDegraded       NodeStatus = "degraded"       
	NodeStatusDecommissioning NodeStatus = "decommissioning" 
	NodeStatusUnknown        NodeStatus = "unknown"
)

type EdgeNode struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenantId"`
	Name      string
	Mode      NodeMode
	Status    NodeStatus
	Endpoint  string 
	Region    string
	Labels    map[string]string

	CACertPEM     string
	ClientCertPEM string
	ClientKeyPEM  string

	HeartbeatAt time.Time
	LastSeenAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (n *EdgeNode) Register() {
	now := time.Now().UTC()
	n.Status = NodeStatusPending
	n.CreatedAt = now
	n.UpdatedAt = now
}

func (n *EdgeNode) Heartbeat(at time.Time, offlineThreshold time.Duration) {
	n.LastSeenAt = at
	n.HeartbeatAt = at
	if n.Status == NodeStatusOffline || n.Status == NodeStatusUnknown {
		n.Status = NodeStatusOnline
	} else if n.Status == NodeStatusPending {
		n.Status = NodeStatusOnline
	}
	n.UpdatedAt = at
}

func (n *EdgeNode) RecomputeLiveness(now time.Time, offlineThreshold time.Duration) {
	if n.Status == NodeStatusDecommissioning {
		return
	}
	if n.LastSeenAt.IsZero() {
		
		if now.Sub(n.CreatedAt) > offlineThreshold {
			n.Status = NodeStatusOffline
		}
		return
	}
	if now.Sub(n.LastSeenAt) > offlineThreshold {
		n.Status = NodeStatusOffline
	}
}

func (n *EdgeNode) MarkOffline() { n.Status = NodeStatusOffline }

func (n *EdgeNode) MarkDecommissioning() { n.Status = NodeStatusDecommissioning }

type EdgeDeployStatus string

const (
	EdgeDeployPending    EdgeDeployStatus = "pending"    
	EdgeDeployDeploying  EdgeDeployStatus = "deploying"  
	EdgeDeployActive     EdgeDeployStatus = "active"     
	EdgeDeployDegraded   EdgeDeployStatus = "degraded"   
	EdgeDeployFailing    EdgeDeployStatus = "failing"    
	EdgeDeployRolledBack EdgeDeployStatus = "rolled_back" 
	EdgeDeploySuperseded EdgeDeployStatus = "superseded" 
)

type EdgeDeploySpec struct {
	Image       string            
	Command     []string
	Args        []string
	Env         map[string]string
	Replicas    int
	CPU         string
	Memory      string
	GPUs        int
	HealthCheck *EdgeHealthCheck
}

type EdgeHealthCheck struct {
	Path                string
	Port                int
	InitialDelaySeconds int
	PeriodSeconds       int
}

type EdgeDeployment struct {
	ID             string
	TenantID       string
	NodeID         string
	ModelID        string
	Version        string 
	DesiredSpec    EdgeDeploySpec

	CurrentVersion string 
	ActiveVersion  string 
	CanaryVersion  string 
	CanaryWeight   int    

	Status          EdgeDeployStatus
	AutoRollback    bool 
	DriftGuardEnabled bool 

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (d *EdgeDeployment) PromoteCanary(step int) bool {
	if step <= 0 {
		step = 25
	}
	d.CanaryVersion = d.Version
	d.Status = EdgeDeployDeploying
	if d.CanaryWeight+step >= 100 {
		d.CanaryWeight = 100
	} else {
		d.CanaryWeight += step
	}
	d.UpdatedAt = time.Now().UTC()
	return d.CanaryWeight >= 100
}

func (d *EdgeDeployment) CompleteCanary() {
	d.ActiveVersion = d.Version
	d.CurrentVersion = d.Version
	d.CanaryVersion = ""
	d.CanaryWeight = 0
	d.Status = EdgeDeployActive
	d.UpdatedAt = time.Now().UTC()
}

func (d *EdgeDeployment) RollbackTo() {
	d.CurrentVersion = d.ActiveVersion
	d.CanaryVersion = ""
	d.CanaryWeight = 0
	d.Status = EdgeDeployRolledBack
	d.UpdatedAt = time.Now().UTC()
}

func (d *EdgeDeployment) MarkDeploying() { d.Status = EdgeDeployDeploying; d.UpdatedAt = time.Now().UTC() }
func (d *EdgeDeployment) MarkActive()    { d.Status = EdgeDeployActive; d.UpdatedAt = time.Now().UTC() }
func (d *EdgeDeployment) MarkFailing()   { d.Status = EdgeDeployFailing; d.UpdatedAt = time.Now().UTC() }
func (d *EdgeDeployment) MarkDegraded()  { d.Status = EdgeDeployDegraded; d.UpdatedAt = time.Now().UTC() }

type LabelFeedback struct {
	ID           string
	TenantID     string
	DeploymentID string
	
	Label string
	
	RequestID string
	
	FeedbackAt time.Time
}