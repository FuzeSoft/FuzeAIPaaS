package ports

import (
	"context"
	"errors"

	"fuze-ai-paas/backend/internal/domain/agent"
)

var (
	
	ErrAgentNotFound = errors.New("ports: agent not found")
	
	ErrAgentConflict = errors.New("ports: agent name conflict")
	
	ErrRunNotFound = errors.New("ports: agent run not found")
)

type AgentFilter struct {
	
	TenantID string
	
	Name string
	
	Status string
}

type RunFilter struct {
	
	TenantID string
	
	AgentID string
	
	Status string
}

type AgentRepository interface {
	
	Create(ctx context.Context, a *agent.Agent) error
	
	Get(ctx context.Context, tenantID, id string) (*agent.Agent, error)
	
	List(ctx context.Context, f AgentFilter) ([]*agent.Agent, error)
	
	Update(ctx context.Context, a *agent.Agent) error
	
	Delete(ctx context.Context, tenantID, id string) error

	CreateRun(ctx context.Context, run *agent.Run) error
	
	GetRun(ctx context.Context, tenantID, id string) (*agent.Run, error)
	
	ListRuns(ctx context.Context, f RunFilter) ([]*agent.Run, error)
	
	UpdateRun(ctx context.Context, run *agent.Run) error
}

type ToolRepository interface {
	
	Create(ctx context.Context, t *agent.Tool) error
	
	Get(ctx context.Context, tenantID, id string) (*agent.Tool, error)
	
	GetByName(ctx context.Context, tenantID, name string) (*agent.Tool, error)
	
	List(ctx context.Context, tenantID string) ([]*agent.Tool, error)
	
	Update(ctx context.Context, t *agent.Tool) error
	
	Delete(ctx context.Context, tenantID, id string) error
}

var (
	
	ErrToolNotFound = errors.New("ports: tool not found")
	
	ErrToolConflict = errors.New("ports: tool name conflict")
)