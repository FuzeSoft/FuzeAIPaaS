package agent

import (
	"sort"
	"time"
)

const (
	
	RunPending = "pending"
	
	RunRunning = "running"
	
	RunPaused = "paused"
	
	RunSucceeded = "succeeded"
	
	RunFailed = "failed"
)

type NodeResult struct {
	
	NodeID string `json:"node_id"`
	
	Status string `json:"status"`
	
	Output string `json:"output,omitempty"`
	
	Error string `json:"error,omitempty"`
	
	StartedAt time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

type Run struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	TenantID  string    `json:"tenant_id"`
	Status    string    `json:"status"`
	
	Input string `json:"input,omitempty"`
	
	Results []NodeResult `json:"results"`
	
	PausedAt string `json:"paused_at,omitempty"`
	
	PausePrompt string `json:"pause_prompt,omitempty"`
	
	FinalOutput string `json:"final_output,omitempty"`
	
	CreatedBy string `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewRun(id, agentID, tenantID, input string) *Run {
	now := time.Now().UTC()
	return &Run{
		ID:       id,
		AgentID:  agentID,
		TenantID: tenantID,
		Status:   RunPending,
		Input:    input,
		Results:  make([]NodeResult, 0),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (r *Run) recordNode(nr NodeResult) {
	nr.FinishedAt = time.Now().UTC()
	r.Results = append(r.Results, nr)
	r.UpdatedAt = nr.FinishedAt
}

func (r *Run) PauseForHuman(nodeID, prompt string) {
	r.Status = RunPaused
	r.PausedAt = nodeID
	r.PausePrompt = prompt
	r.UpdatedAt = time.Now().UTC()
}

func (r *Run) HumanResume(decision string) error {
	if r.Status != RunPaused {
		return ErrRunNotPaused
	}
	r.recordNode(NodeResult{
		NodeID:  r.PausedAt,
		Status:  "ok",
		Output:  decision,
		StartedAt: r.UpdatedAt,
	})
	r.PausedAt = ""
	r.PausePrompt = ""
	r.Status = RunRunning
	r.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *Run) Fail(nodeID, reason string) {
	r.recordNode(NodeResult{
		NodeID:  nodeID,
		Status:  "failed",
		Error:   reason,
		StartedAt: r.UpdatedAt,
	})
	r.Status = RunFailed
	r.UpdatedAt = time.Now().UTC()
}

func (r *Run) Succeed(final string) {
	if len(r.Results) > 0 {
		last := r.Results[len(r.Results)-1]
		r.recordNode(NodeResult{
			NodeID:  last.NodeID,
			Status:  "ok",
			Output:  final,
			StartedAt: r.UpdatedAt,
		})
	}
	r.FinalOutput = final
	r.Status = RunSucceeded
	r.UpdatedAt = time.Now().UTC()
}

func (r *Run) Start() {
	r.Status = RunRunning
	r.UpdatedAt = time.Now().UTC()
}

func (r *Run) nodeOutput(nodeID string) string {
	
	for i := len(r.Results) - 1; i >= 0; i-- {
		if r.Results[i].NodeID == nodeID {
			return r.Results[i].Output
		}
	}
	return ""
}

func (r *Run) OutputOf(nodeID string) string {
	return r.nodeOutput(nodeID)
}

func (r *Run) RecordCompleted(nodeID, output string) {
	r.recordNode(NodeResult{
		NodeID:   nodeID,
		Status:   "ok",
		Output:   output,
		StartedAt: r.UpdatedAt,
	})
}

func (r *Run) orderedNodeIDs(dag DAG) ([]string, error) {
	order, err := dag.TopoSort()
	if err != nil {
		return nil, err
	}
	
	sort.SliceStable(order, func(i, j int) bool { return order[i] < order[j] })
	return order, nil
}