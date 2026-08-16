
package event

import (
	"time"
)

type Event interface {
	
	EventType() string
	
	OccurredAt() time.Time
	
	AggregateID() string
}

type BaseEvent struct {
	Type      string
	At        time.Time
	Aggregate string
}

func (e BaseEvent) EventType() string     { return e.Type }
func (e BaseEvent) OccurredAt() time.Time { return e.At }
func (e BaseEvent) AggregateID() string   { return e.Aggregate }

type ClusterStats struct {
	NodeCount int
	TotalGPUs int
	UsedGPUs  int
	Status    string
	Version   string
}

type ClusterDiscovered struct {
	BaseEvent
	ClusterID   string
	ClusterName string
	NodeCount   int
	TotalGPUs   int
	UsedGPUs    int
	Status      string
	Version     string
}

const ClusterDiscoveredType = "ClusterDiscovered"

func NewClusterDiscovered(clusterID, name string, stats ClusterStats) ClusterDiscovered {
	return ClusterDiscovered{
		BaseEvent: BaseEvent{
			Type:      ClusterDiscoveredType,
			At:        time.Now().UTC(),
			Aggregate: clusterID,
		},
		ClusterID:   clusterID,
		ClusterName: name,
		NodeCount:   stats.NodeCount,
		TotalGPUs:   stats.TotalGPUs,
		UsedGPUs:    stats.UsedGPUs,
		Status:      stats.Status,
		Version:     stats.Version,
	}
}

type JobSubmitted struct {
	BaseEvent
	JobID     string
	ClusterID string
	JobType   string
	GPUs      int
	TenantID  string
}

const JobSubmittedType = "JobSubmitted"

func NewJobSubmitted(jobID, clusterID, jobType string, gpus int, tenantID string) JobSubmitted {
	return JobSubmitted{
		BaseEvent: BaseEvent{
			Type:      JobSubmittedType,
			At:        time.Now().UTC(),
			Aggregate: jobID,
		},
		JobID:     jobID,
		ClusterID: clusterID,
		JobType:   jobType,
		GPUs:      gpus,
		TenantID:  tenantID,
	}
}

type JobStateChanged struct {
	BaseEvent
	JobID     string
	ClusterID string
	Status    string
	TenantID  string
	
	Terminal bool
}

const JobStateChangedType = "JobStateChanged"

func NewJobStateChanged(jobID, clusterID, status, tenantID string, terminal bool) JobStateChanged {
	return JobStateChanged{
		BaseEvent: BaseEvent{
			Type:      JobStateChangedType,
			At:        time.Now().UTC(),
			Aggregate: jobID,
		},
		JobID:     jobID,
		ClusterID: clusterID,
		Status:    status,
		TenantID:  tenantID,
		Terminal:  terminal,
	}
}

type AssignmentCompleted struct {
	BaseEvent
	JobID         string
	ClusterID     string
	AllocatedGPUs int
	MemoryMiB     int
}

const AssignmentCompletedType = "AssignmentCompleted"

func NewAssignmentCompleted(jobID, clusterID string, allocatedGPUs, memoryMiB int) AssignmentCompleted {
	return AssignmentCompleted{
		BaseEvent: BaseEvent{
			Type:      AssignmentCompletedType,
			At:        time.Now().UTC(),
			Aggregate: jobID,
		},
		JobID:         jobID,
		ClusterID:     clusterID,
		AllocatedGPUs: allocatedGPUs,
		MemoryMiB:     memoryMiB,
	}
}

type WorkspaceReclaimed struct {
	BaseEvent
	WorkspaceID string
	TenantID     string
	OwnerID      string
	Name         string
	
	IdleTimeout time.Duration
	
	LastActiveAt time.Time
}

const WorkspaceReclaimedType = "WorkspaceReclaimed"

type WorkspaceInfo struct {
	ID          string
	TenantID    string
	OwnerID     string
	Name        string
	IdleTimeout time.Duration
	
	LastActiveAt *time.Time
}

func NewWorkspaceReclaimed(ws WorkspaceInfo) WorkspaceReclaimed {
	var lastActive time.Time
	if ws.LastActiveAt != nil {
		lastActive = *ws.LastActiveAt
	}
	return WorkspaceReclaimed{
		BaseEvent: BaseEvent{
			Type:      WorkspaceReclaimedType,
			At:        time.Now().UTC(),
			Aggregate: ws.ID,
		},
		WorkspaceID: ws.ID,
		TenantID:     ws.TenantID,
		OwnerID:      ws.OwnerID,
		Name:         ws.Name,
		IdleTimeout:  ws.IdleTimeout,
		LastActiveAt: lastActive,
	}
}

type BudgetThresholdExceeded struct {
	BaseEvent
	TenantID      string
	LimitCost     float64
	UsedCost      float64
	
	Ratio float64
	
	Breached bool
}

const BudgetThresholdExceededType = "BudgetThresholdExceeded"

func NewBudgetThresholdExceeded(tenantID string, limitCost, usedCost float64) BudgetThresholdExceeded {
	ratio := 0.0
	breached := false
	if limitCost > 0 {
		ratio = usedCost / limitCost
		breached = usedCost >= limitCost
	}
	return BudgetThresholdExceeded{
		BaseEvent: BaseEvent{
			Type:      BudgetThresholdExceededType,
			At:        time.Now().UTC(),
			Aggregate: tenantID,
		},
		TenantID:  tenantID,
		LimitCost: limitCost,
		UsedCost:  usedCost,
		Ratio:     ratio,
		Breached:  breached,
	}
}

type AgentRunFinished struct {
	BaseEvent
	TenantID     string
	AgentID      string
	RunID        string
	Status       string 
	NodeCount    int    
	FinalOutput  string 
	Error        string 
	ActorID      string 
	ActorName    string 
	DurationMs   int64  
}

const AgentRunFinishedType = "AgentRunFinished"

func NewAgentRunFinished(tenantID, agentID, runID, status, actorID, actorName string, nodeCount int, durationMs int64, finalOutput, errMsg string) AgentRunFinished {
	return AgentRunFinished{
		BaseEvent: BaseEvent{
			Type:      AgentRunFinishedType,
			At:        time.Now().UTC(),
			Aggregate: runID,
		},
		TenantID:    tenantID,
		AgentID:     agentID,
		RunID:       runID,
		Status:      status,
		NodeCount:   nodeCount,
		FinalOutput: finalOutput,
		Error:       errMsg,
		ActorID:     actorID,
		ActorName:   actorName,
		DurationMs:  durationMs,
	}
}