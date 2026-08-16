
package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"fuze-ai-paas/backend/internal/domain/agent"
	"fuze-ai-paas/backend/internal/domain/event"
	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/ports"
)

type portsAgentRepo = ports.AgentRepository

var ErrAgentNotCompiled = agent.ErrAgentNotCompiled

type LLMCaller interface {
	
	Complete(ctx context.Context, req llm.ChatRequest, tenantID, userID, kbID string, topK int) (string, error)
}

type Retriever interface {
	Retrieve(ctx context.Context, baseID, query string, topK int) ([]llm.ScoredSegment, error)
}

type ToolExecutor = agent.ToolExecutor

type IDGen func() string

type Deps struct {
	
	Agents portsAgentRepo
	
	LLM LLMCaller
	
	Retriever Retriever
	
	Tools ToolExecutor
	
	Sink EventPublisher
	
	NewID IDGen
}

type EventPublisher interface {
	
	Notify(ctx context.Context, e event.Event) error
}

type (
	portsAgentFilter = ports.AgentFilter
	portsRunFilter   = ports.RunFilter
)

type Service struct {
	deps Deps
	newID IDGen
}

func NewService(deps Deps) (*Service, error) {
	if deps.Agents == nil {
		return nil, errors.New("agent: Agents repository is required")
	}
	gen := deps.NewID
	if gen == nil {
		gen = defaultIDGen()
	}
	return &Service{deps: deps, newID: gen}, nil
}

func (s *Service) SaveAgent(ctx context.Context, a *agent.Agent) error {
	if a.ID == "" {
		return errors.New("agent: id required")
	}
	existing, err := s.deps.Agents.Get(ctx, a.TenantID, a.ID)
	if err != nil {
		
		return s.deps.Agents.Create(ctx, a)
	}
	
	_ = existing
	return s.deps.Agents.Update(ctx, a)
}

func (s *Service) List(ctx context.Context, tenantID, name, status string) ([]*agent.Agent, error) {
	return s.deps.Agents.List(ctx, ports.AgentFilter{TenantID: tenantID, Name: name, Status: status})
}

func (s *Service) Get(ctx context.Context, tenantID, id string) (*agent.Agent, error) {
	return s.deps.Agents.Get(ctx, tenantID, id)
}

func (s *Service) Delete(ctx context.Context, tenantID, id string) error {
	return s.deps.Agents.Delete(ctx, tenantID, id)
}

func (s *Service) ListRuns(ctx context.Context, tenantID, agentID, status string) ([]*agent.Run, error) {
	return s.deps.Agents.ListRuns(ctx, ports.RunFilter{TenantID: tenantID, AgentID: agentID, Status: status})
}

func (s *Service) GetRun(ctx context.Context, tenantID, runID string) (*agent.Run, error) {
	return s.deps.Agents.GetRun(ctx, tenantID, runID)
}

func (s *Service) Compile(ctx context.Context, tenantID, id string) (*agent.Agent, error) {
	a, err := s.deps.Agents.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if err := a.Compile(); err != nil {
		return nil, err
	}
	if err := s.deps.Agents.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Service) StartRun(ctx context.Context, tenantID, agentID, input, userID string) (*agent.Run, error) {
	a, err := s.deps.Agents.Get(ctx, tenantID, agentID)
	if err != nil {
		return nil, err
	}
	if a.Status != agent.AgentStatusCompiled && a.Status != agent.AgentStatusPublished {
		return nil, ErrAgentNotCompiled
	}
	run := agent.NewRun(s.newID(), agentID, tenantID, input)
	run.CreatedBy = userID
	run.Start()
	if err := s.deps.Agents.CreateRun(ctx, run); err != nil {
		return nil, err
	}
	
	if err := s.drive(ctx, a, run); err != nil {
		
		log.Printf("[agent] run %s drive error: %v", run.ID, err)
	}
	
	s.publishRunFinished(run, a)
	return run, nil
}

func (s *Service) ResumeRun(ctx context.Context, tenantID, runID, decision string) (*agent.Run, error) {
	run, err := s.deps.Agents.GetRun(ctx, tenantID, runID)
	if err != nil {
		return nil, err
	}
	a, err := s.deps.Agents.Get(ctx, tenantID, run.AgentID)
	if err != nil {
		return nil, err
	}
	if err := run.HumanResume(decision); err != nil {
		return nil, err
	}
	if err := s.deps.Agents.UpdateRun(ctx, run); err != nil {
		return nil, err
	}
	if err := s.drive(ctx, a, run); err != nil {
		log.Printf("[agent] run %s resume drive error: %v", run.ID, err)
	}
	
	s.publishRunFinished(run, a)
	return run, nil
}

func (s *Service) drive(ctx context.Context, a *agent.Agent, run *agent.Run) error {
	order, err := a.DAG.TopoSort()
	if err != nil {
		run.Fail("", err.Error())
		return s.deps.Agents.UpdateRun(ctx, run)
	}
	byID := a.DAG.NodeByID()
	done := make(map[string]bool, len(run.Results))
	for _, r := range run.Results {
		done[r.NodeID] = true
	}

	for _, nodeID := range order {
		if done[nodeID] {
			continue
		}
		n := byID[nodeID]
		out, nerr := s.execNode(ctx, a, &n, run)
		if nerr != nil {
			run.Fail(nodeID, nerr.Error())
			_ = s.deps.Agents.UpdateRun(ctx, run)
			return nerr
		}
		if run.Status == agent.RunPaused {
			
			_ = s.deps.Agents.UpdateRun(ctx, run)
			return nil
		}
		run.RecordCompleted(nodeID, out)
		_ = s.deps.Agents.UpdateRun(ctx, run)
	}
	
	final := ""
	if len(run.Results) > 0 {
		final = run.Results[len(run.Results)-1].Output
	}
	run.Succeed(final)
	return s.deps.Agents.UpdateRun(ctx, run)
}

func (s *Service) execNode(ctx context.Context, a *agent.Agent, n *agent.Node, run *agent.Run) (string, error) {
	
	upstream := func(id string) string { return run.OutputOf(id) }

	switch n.Type {
	case agent.NodeLLMCall:
		if s.deps.LLM == nil {
			return "", errors.New("llm executor not configured")
		}
		spec := agent.ExtractLLMCall(n.Ref, n.Config)
		prompt := agent.RenderPrompt(spec.Prompt, upstream)
		req := llm.ChatRequest{
			Model: spec.Model,
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: spec.System},
				{Role: llm.RoleUser, Content: prompt},
			},
		}
		if err := req.Validate(); err != nil {
			return "", err
		}
		return s.deps.LLM.Complete(ctx, req, run.TenantID, run.CreatedBy, spec.KnowledgeBaseID, 0)

	case agent.NodeRagRetrieve:
		if s.deps.Retriever == nil {
			return "", errors.New("retriever not configured")
		}
		q := agent.ExtractRagQuery(n.Ref, n.Config)
		query := agent.RenderPrompt(q.Query, upstream)
		hits, err := s.deps.Retriever.Retrieve(ctx, q.KnowledgeBaseID, query, q.TopK)
		if err != nil {
			return "", err
		}
		return formatSegments(hits), nil

	case agent.NodeToolCall:
		if s.deps.Tools == nil {
			return "", errors.New("tool executor not configured")
		}
		res, err := s.deps.Tools.Execute(agent.ToolCall{
			Tool:     n.Ref,
			Args:     n.Config,
			TenantID: run.TenantID,
		})
		if err != nil {
			return "", err
		}
		if !res.OK {
			return "", fmt.Errorf("tool %s failed: %s", n.Ref, res.Error)
		}
		return res.Data, nil

	case agent.NodeCondition:
		
		if expr, ok := n.Config["expr"].(string); ok {
			return expr, nil
		}
		return "", nil

	case agent.NodeSubAgent:
		
		subID := n.Ref
		if subID == "" {
			return "", errors.New("subagent: node ref (sub-agent id) is required")
		}
		childInput := agent.RenderPrompt(fmt.Sprintf("%v", n.Config["input"]), upstream)
		if childInput == "" {
			
			childInput = run.Input
		}
		return s.runSubAgent(ctx, run.TenantID, subID, childInput, run.CreatedBy)

	case agent.NodeHumanReview:
		prompt, _ := n.Config["prompt"].(string)
		run.PauseForHuman(n.ID, prompt)
		return "", nil

	default:
		return "", fmt.Errorf("unsupported node type %q", n.Type)
	}
}

func formatSegments(hits []llm.ScoredSegment) string {
	out := ""
	for i, h := range hits {
		out += fmt.Sprintf("[%d] %s\n", i+1, h.Segment.Content)
	}
	return out
}

func (s *Service) runSubAgent(ctx context.Context, tenantID, subID, input, userID string) (string, error) {
	sub, err := s.deps.Agents.Get(ctx, tenantID, subID)
	if err != nil {
		return "", fmt.Errorf("subagent: load %q: %w", subID, err)
	}
	if sub.Status != agent.AgentStatusCompiled && sub.Status != agent.AgentStatusPublished {
		return "", fmt.Errorf("subagent: %q is not compiled (status=%s)", subID, sub.Status)
	}
	child := agent.NewRun(s.newID(), subID, tenantID, input)
	child.CreatedBy = userID
	child.Start()
	if err := s.deps.Agents.CreateRun(ctx, child); err != nil {
		return "", fmt.Errorf("subagent: create run: %w", err)
	}
	if err := s.drive(ctx, sub, child); err != nil {
		
		if child.Status == agent.RunPaused {
			return "", fmt.Errorf("subagent: %q paused on HumanReview, not supported in sub-agent", subID)
		}
		return "", fmt.Errorf("subagent: %q failed: %s", subID, child.FinalOutput)
	}
	return child.FinalOutput, nil
}

func defaultIDGen() IDGen {
	var seq int64
	return func() string {
		seq++
		return fmt.Sprintf("run-%d-%d", time.Now().UnixNano(), seq)
	}
}

func (s *Service) publishRunFinished(run *agent.Run, a *agent.Agent) {
	if s.deps.Sink == nil {
		return
	}
	if run.Status != agent.RunSucceeded && run.Status != agent.RunFailed {
		return
	}
	status := run.Status
	var errMsg string
	if run.Status == agent.RunFailed && len(run.Results) > 0 {
		errMsg = run.Results[len(run.Results)-1].Error
	}
	ev := event.NewAgentRunFinished(
		run.TenantID, a.ID, run.ID, status,
		run.CreatedBy, run.CreatedBy, len(a.DAG.Nodes),
		run.UpdatedAt.Sub(run.CreatedAt).Milliseconds(),
		run.FinalOutput, errMsg,
	)
	if err := s.deps.Sink.Notify(context.Background(), ev); err != nil {
		log.Printf("[agent] publish AgentRunFinished failed run=%s: %v", run.ID, err)
	}
}