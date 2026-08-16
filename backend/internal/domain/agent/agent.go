
package agent

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

var (
	
	ErrCycle = errors.New("agent: dag has a cycle")
	
	ErrNoEntry = errors.New("agent: dag has no entry node")
	
	ErrDuplicateNode = errors.New("agent: duplicate node id")
	
	ErrMissingNode = errors.New("agent: edge references missing node")
	
	ErrInvalidNode = errors.New("agent: invalid node config")
	
	ErrRunNotPaused = errors.New("agent: run is not paused for human review")
	
	ErrAgentNotCompiled = errors.New("agent: cannot execute uncompiled agent")
)

type NodeType string

const (
	
	NodeLLMCall NodeType = "llm_call"
	
	NodeToolCall NodeType = "tool_call"
	
	NodeRagRetrieve NodeType = "rag_retrieve"
	
	NodeCondition NodeType = "condition"
	
	NodeHumanReview NodeType = "human_review"
	
	NodeSubAgent NodeType = "sub_agent"
)

type Node struct {
	
	ID string `json:"id"`
	
	Type NodeType `json:"type"`
	
	Ref string `json:"ref,omitempty"`
	
	Config map[string]any `json:"config,omitempty"`
}

func (n Node) Validate() error {
	if n.ID == "" {
		return fmt.Errorf("%w: node id is empty", ErrInvalidNode)
	}
	switch n.Type {
	case NodeLLMCall, NodeToolCall, NodeRagRetrieve, NodeCondition, NodeHumanReview, NodeSubAgent:
		
	default:
		return fmt.Errorf("%w: unknown node type %q", ErrInvalidNode, n.Type)
	}
	if n.Type == NodeToolCall && n.Ref == "" {
		return fmt.Errorf("%w: tool_call node requires ref", ErrInvalidNode)
	}
	if n.Type == NodeRagRetrieve && n.Ref == "" {
		return fmt.Errorf("%w: rag_retrieve node requires ref (knowledge base id)", ErrInvalidNode)
	}
	if n.Type == NodeSubAgent && n.Ref == "" {
		return fmt.Errorf("%w: sub_agent node requires ref (sub-agent id)", ErrInvalidNode)
	}
	return nil
}

type DAG struct {
	
	Nodes []Node `json:"nodes"`
	
	Edges map[string][]string `json:"edges"`
}

func (d DAG) NodeByID() map[string]Node {
	m := make(map[string]Node, len(d.Nodes))
	for _, n := range d.Nodes {
		m[n.ID] = n
	}
	return m
}

func (d DAG) entryNode() (string, error) {
	indeg := make(map[string]int, len(d.Nodes))
	for _, n := range d.Nodes {
		indeg[n.ID] = 0
	}
	for _, tos := range d.Edges {
		for _, to := range tos {
			indeg[to]++
		}
	}
	var entries []string
	for id, deg := range indeg {
		if deg == 0 {
			entries = append(entries, id)
		}
	}
	if len(entries) == 0 {
		return "", ErrNoEntry
	}
	if len(entries) > 1 {
		sort.Strings(entries)
		return "", fmt.Errorf("%w: found %d entries: %v", ErrNoEntry, len(entries), entries)
	}
	return entries[0], nil
}

func (d DAG) TopoSort() ([]string, error) {
	byID := d.NodeByID()
	indeg := make(map[string]int, len(d.Nodes))
	adj := make(map[string][]string, len(d.Edges))
	for _, n := range d.Nodes {
		indeg[n.ID] = 0
	}
	for from, tos := range d.Edges {
		if _, ok := byID[from]; !ok {
			return nil, fmt.Errorf("%w: edge source %q", ErrMissingNode, from)
		}
		for _, to := range tos {
			if _, ok := byID[to]; !ok {
				return nil, fmt.Errorf("%w: edge target %q", ErrMissingNode, to)
			}
			adj[from] = append(adj[from], to)
			indeg[to]++
		}
	}
	
	queue := make([]string, 0)
	for id, deg := range indeg {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)

	order := make([]string, 0, len(d.Nodes))
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		
		succ := append([]string(nil), adj[cur]...)
		sort.Strings(succ)
		for _, nxt := range succ {
			indeg[nxt]--
			if indeg[nxt] == 0 {
				queue = append(queue, nxt)
			}
		}
	}
	if len(order) != len(d.Nodes) {
		return nil, ErrCycle
	}
	return order, nil
}

func (d DAG) Validate() error {
	seen := make(map[string]struct{}, len(d.Nodes))
	for _, n := range d.Nodes {
		if err := n.Validate(); err != nil {
			return err
		}
		if _, dup := seen[n.ID]; dup {
			return fmt.Errorf("%w: %s", ErrDuplicateNode, n.ID)
		}
		seen[n.ID] = struct{}{}
	}
	
	if _, err := d.TopoSort(); err != nil {
		return err
	}
	
	if _, err := d.entryNode(); err != nil {
		return err
	}
	return nil
}

type Agent struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	
	Description string `json:"description,omitempty"`
	
	DAG       DAG       `json:"dag"`
	
	Status    string    `json:"status"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const (
	AgentStatusDraft    = "draft"
	AgentStatusCompiled = "compiled"
	AgentStatusPublished = "published"
)

func NewAgent(id, tenantID, name string, dag DAG, createdBy string) (*Agent, error) {
	if id == "" || tenantID == "" || name == "" {
		return nil, fmt.Errorf("%w: id/tenant/name required", ErrInvalidNode)
	}
	now := time.Now().UTC()
	return &Agent{
		ID:        id,
		TenantID:  tenantID,
		Name:      name,
		DAG:       dag,
		Status:    AgentStatusDraft,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (a *Agent) Compile() error {
	if err := a.DAG.Validate(); err != nil {
		return fmt.Errorf("agent %s compile failed: %w", a.ID, err)
	}
	a.Status = AgentStatusCompiled
	a.UpdatedAt = time.Now().UTC()
	return nil
}