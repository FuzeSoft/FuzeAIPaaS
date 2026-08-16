package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"fuze-ai-paas/backend/internal/domain/agent"
	"fuze-ai-paas/backend/internal/domain/event"
	"fuze-ai-paas/backend/internal/domain/llm"
)

func linearDAG() agent.DAG {
	return agent.DAG{
		Nodes: []agent.Node{
			{ID: "n1", Type: agent.NodeLLMCall, Ref: "gpt", Config: map[string]any{"prompt": "hi"}},
			{ID: "n2", Type: agent.NodeToolCall, Ref: "search", Config: map[string]any{"url": "x"}},
			{ID: "n3", Type: agent.NodeLLMCall, Ref: "gpt2", Config: map[string]any{"prompt": "done"}},
		},
		Edges: map[string][]string{"n1": {"n2"}, "n2": {"n3"}},
	}
}

type fakeRepo struct {
	mu     sync.Mutex
	agents map[string]*agent.Agent
	runs   map[string]*agent.Run
	
	conflictName string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{agents: map[string]*agent.Agent{}, runs: map[string]*agent.Run{}}
}

func (f *fakeRepo) Create(ctx context.Context, a *agent.Agent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conflictName != "" && a.Name == f.conflictName {
		return errors.New("conflict")
	}
	f.agents[a.ID] = a
	return nil
}
func (f *fakeRepo) Get(ctx context.Context, tenantID, id string) (*agent.Agent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.agents[id]
	if !ok || a.TenantID != tenantID {
		return nil, errors.New("not found")
	}
	return a, nil
}
func (f *fakeRepo) List(ctx context.Context, flt portsAgentFilter) ([]*agent.Agent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*agent.Agent
	for _, a := range f.agents {
		if a.TenantID == flt.TenantID {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeRepo) Update(ctx context.Context, a *agent.Agent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agents[a.ID] = a
	return nil
}
func (f *fakeRepo) Delete(ctx context.Context, tenantID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.agents, id)
	return nil
}
func (f *fakeRepo) CreateRun(ctx context.Context, run *agent.Run) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs[run.ID] = run
	return nil
}
func (f *fakeRepo) GetRun(ctx context.Context, tenantID, id string) (*agent.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.runs[id]
	if !ok || r.TenantID != tenantID {
		return nil, errors.New("run not found")
	}
	return r, nil
}
func (f *fakeRepo) ListRuns(ctx context.Context, flt portsRunFilter) ([]*agent.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*agent.Run
	for _, r := range f.runs {
		if r.TenantID == flt.TenantID && (flt.AgentID == "" || r.AgentID == flt.AgentID) {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeRepo) UpdateRun(ctx context.Context, run *agent.Run) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs[run.ID] = run
	return nil
}

type fakeLLM struct {
	mu    sync.Mutex
	calls []llm.ChatRequest
}

func (c *fakeLLM) Complete(ctx context.Context, req llm.ChatRequest, tenantID, userID, kbID string, topK int) (string, error) {
	c.mu.Lock()
	c.calls = append(c.calls, req)
	c.mu.Unlock()
	return "answer:" + req.Model, nil
}

func seedAgent(t *testing.T, repo *fakeRepo, dag agent.DAG) *agent.Agent {
	t.Helper()
	a, err := agent.NewAgent("ag1", "t1", "demo", dag, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Compile(); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestCompileOK(t *testing.T) {
	repo := newFakeRepo()
	svc, err := NewService(Deps{Agents: repo})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := agent.NewAgent("ag1", "t1", "demo", linearDAG(), "u1")
	if err := repo.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Compile(context.Background(), "t1", "ag1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != agent.AgentStatusCompiled {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestStartRunLLMChain(t *testing.T) {
	repo := newFakeRepo()
	llm := &fakeLLM{}
	svc, _ := NewService(Deps{Agents: repo, LLM: llm})
	dag := agent.DAG{
		Nodes: []agent.Node{
			{ID: "n1", Type: agent.NodeLLMCall, Ref: "gpt", Config: map[string]any{"prompt": "hi"}},
			{ID: "n2", Type: agent.NodeLLMCall, Ref: "gpt2", Config: map[string]any{"prompt": "done"}},
		},
		Edges: map[string][]string{"n1": {"n2"}},
	}
	seedAgent(t, repo, dag)

	run, err := svc.StartRun(context.Background(), "t1", "ag1", "start", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != agent.RunSucceeded {
		t.Fatalf("run status = %s", run.Status)
	}
	if run.FinalOutput != "answer:gpt2" {
		t.Fatalf("final output = %q", run.FinalOutput)
	}
	if len(llm.calls) != 2 {
		t.Fatalf("llm called %d times, want 1", len(llm.calls))
	}
}

func TestStartRun_HumanReview_PauseThenResume(t *testing.T) {
	repo := newFakeRepo()
	llm := &fakeLLM{}
	svc, _ := NewService(Deps{Agents: repo, LLM: llm})
	dag := agent.DAG{
		Nodes: []agent.Node{
			{ID: "n1", Type: agent.NodeLLMCall, Ref: "gpt", Config: map[string]any{"prompt": "hi"}},
			{ID: "n2", Type: agent.NodeHumanReview, Config: map[string]any{"prompt": "approve?"}},
			{ID: "n3", Type: agent.NodeLLMCall, Ref: "gpt2", Config: map[string]any{"prompt": "after {n2}"}},
		},
		Edges: map[string][]string{"n1": {"n2"}, "n2": {"n3"}},
	}
	seedAgent(t, repo, dag)

	run, err := svc.StartRun(context.Background(), "t1", "ag1", "start", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != agent.RunPaused {
		t.Fatalf("expected paused, got %s", run.Status)
	}
	if run.PausedAt != "n2" {
		t.Fatalf("paused at %s", run.PausedAt)
	}
	
	if len(llm.calls) != 1 {
		t.Fatalf("llm called %d times before resume, want 1", len(llm.calls))
	}

	resumed, err := svc.ResumeRun(context.Background(), "t1", run.ID, "approved")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != agent.RunSucceeded {
		t.Fatalf("after resume status = %s", resumed.Status)
	}
	if len(llm.calls) != 2 {
		t.Fatalf("llm called %d times after resume, want 2", len(llm.calls))
	}
	
	if llm.calls[1].Messages[1].Content != "after approved" {
		t.Fatalf("n3 prompt = %q", llm.calls[1].Messages[1].Content)
	}
}

func TestStartRunUncompiled(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := NewService(Deps{Agents: repo})
	a, _ := agent.NewAgent("ag1", "t1", "demo", linearDAG(), "u1")
	if err := repo.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	
	if _, err := svc.StartRun(context.Background(), "t1", "ag1", "x", "u1"); err != ErrAgentNotCompiled {
		t.Fatalf("expected ErrAgentNotCompiled, got %v", err)
	}
}

type fakeRetriever struct {
	lastQuery string
	lastTopK  int
}

func (r *fakeRetriever) Retrieve(ctx context.Context, kbID, query string, topK int) ([]llm.ScoredSegment, error) {
	r.lastQuery = query
	r.lastTopK = topK
	return []llm.ScoredSegment{{Segment: llm.Segment{Content: "kb:" + kbID + "|" + query}}}, nil
}

type fakeToolRepo struct {
	byName map[string]*agent.Tool
}

func (f *fakeToolRepo) GetByName(ctx context.Context, tenantID, name string) (*agent.Tool, error) {
	t, ok := f.byName[name]
	if !ok || t.TenantID != tenantID {
		return nil, errors.New("tool not found")
	}
	return t, nil
}

type fakeSink struct {
	mu     sync.Mutex
	events []event.Event
}

func (s *fakeSink) Notify(_ context.Context, e event.Event) error {
	s.mu.Lock()
	s.events = append(s.events, e)
	s.mu.Unlock()
	return nil
}

func TestStartRun_RagNode_UsesRetriever(t *testing.T) {
	repo := newFakeRepo()
	ret := &fakeRetriever{}
	llm := &fakeLLM{}
	svc, _ := NewService(Deps{Agents: repo, LLM: llm, Retriever: ret})
	dag := agent.DAG{
		Nodes: []agent.Node{
			{ID: "n1", Type: agent.NodeRagRetrieve, Ref: "kb-1", Config: map[string]any{"query": "q:你好", "top_k": float64(7)}},
			{ID: "n2", Type: agent.NodeLLMCall, Ref: "gpt", Config: map[string]any{"prompt": "ctx:{n1}"}},
		},
		Edges: map[string][]string{"n1": {"n2"}},
	}
	seedAgent(t, repo, dag)

	run, err := svc.StartRun(context.Background(), "t1", "ag1", "q:你好", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != agent.RunSucceeded {
		t.Fatalf("status=%s out=%q", run.Status, run.FinalOutput)
	}
	
	if ret.lastQuery != "q:你好" {
		t.Fatalf("retriever query = %q", ret.lastQuery)
	}
	if ret.lastTopK != 7 {
		t.Fatalf("retriever topk = %d", ret.lastTopK)
	}
	
	if len(llm.calls) != 1 {
		t.Fatalf("llm called %d times, want 1", len(llm.calls))
	}
	if !contains(llm.calls[0].Messages[1].Content, "kb:kb-1|q:你好") {
		t.Fatalf("llm prompt missing kb ctx: %q", llm.calls[0].Messages[1].Content)
	}
}

func TestStartRun_ToolNode_HTTP(t *testing.T) {
	repo := newFakeRepo()
	
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"echo": "ok"})
	}))
	defer srv.Close()

	tools := &fakeToolRepo{byName: map[string]*agent.Tool{
		"search": {TenantID: "t1", Name: "search", Kind: agent.ToolKindHTTP,
			HTTP: &agent.HTTPToolSpec{URL: srv.URL, Method: http.MethodPost}},
	}}
	
	prev := ssrfGuard
	ssrfGuard = func(string) error { return nil }
	defer func() { ssrfGuard = prev }()
	exec := NewToolExecutor(tools, nil)
	svc, _ := NewService(Deps{Agents: repo, Tools: exec})
	dag := agent.DAG{
		Nodes: []agent.Node{
			{ID: "n1", Type: agent.NodeToolCall, Ref: "search", Config: map[string]any{}},
		},
		Edges: map[string][]string{},
	}
	seedAgent(t, repo, dag)

	run, err := svc.StartRun(context.Background(), "t1", "ag1", "go", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != agent.RunSucceeded {
		t.Fatalf("status=%s out=%q", run.Status, run.FinalOutput)
	}
	if !contains(run.FinalOutput, `"echo":"ok"`) {
		t.Fatalf("tool output missing: %q", run.FinalOutput)
	}
}

func TestStartRun_ToolNode_SensitiveRejected(t *testing.T) {
	repo := newFakeRepo()
	tools := &fakeToolRepo{byName: map[string]*agent.Tool{
		"danger": {TenantID: "t1", Name: "danger", Kind: agent.ToolKindHTTP, Sensitive: true,
			HTTP: &agent.HTTPToolSpec{URL: "https://example.com/x"}},
	}}
	exec := NewToolExecutor(tools, nil)
	svc, _ := NewService(Deps{Agents: repo, Tools: exec})
	dag := agent.DAG{
		Nodes: []agent.Node{{ID: "n1", Type: agent.NodeToolCall, Ref: "danger"}},
	}
	seedAgent(t, repo, dag)
	run, err := svc.StartRun(context.Background(), "t1", "ag1", "go", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != agent.RunFailed {
		t.Fatalf("expected failed, got %s", run.Status)
	}
}

func TestStartRunSubAgent(t *testing.T) {
	repo := newFakeRepo()
	llm := &fakeLLM{}
	svc, _ := NewService(Deps{Agents: repo, LLM: llm})

	child := agent.DAG{
		Nodes: []agent.Node{{ID: "c1", Type: agent.NodeLLMCall, Ref: "child", Config: map[string]any{"prompt": "child"}}},
	}
	seedAgentNamed(t, repo, "child1", child)

	parent := agent.DAG{
		Nodes: []agent.Node{{ID: "p1", Type: agent.NodeSubAgent, Ref: "child1", Config: map[string]any{"input": "hello"}}},
	}
	seedAgentNamed(t, repo, "ag1", parent)

	run, err := svc.StartRun(context.Background(), "t1", "ag1", "start", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != agent.RunSucceeded {
		t.Fatalf("status=%s out=%q", run.Status, run.FinalOutput)
	}
	if run.FinalOutput != "answer:child" {
		t.Fatalf("sub-agent output = %q", run.FinalOutput)
	}
}

func TestStartRunPublishesRunFinishedEvent(t *testing.T) {
	repo := newFakeRepo()
	llm := &fakeLLM{}
	sink := &fakeSink{}
	svc, _ := NewService(Deps{Agents: repo, LLM: llm, Sink: sink})
	seedAgent(t, repo, agent.DAG{
		Nodes: []agent.Node{
			{ID: "n1", Type: agent.NodeLLMCall, Ref: "gpt", Config: map[string]any{"prompt": "hi"}},
		},
	})

	if _, err := svc.StartRun(context.Background(), "t1", "ag1", "start", "u1"); err != nil {
		t.Fatal(err)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.events) != 1 {
		t.Fatalf("events published = %d, want 1", len(sink.events))
	}
	ev, ok := sink.events[0].(event.AgentRunFinished)
	if !ok {
		t.Fatalf("event type = %T", sink.events[0])
	}
	if ev.Status != agent.RunSucceeded || ev.TenantID != "t1" || ev.AgentID != "ag1" || ev.ActorID != "u1" {
		t.Fatalf("event fields wrong: %+v", ev)
	}
}

func TestStartRun_ConditionNode_Passthrough(t *testing.T) {
	repo := newFakeRepo()
	llm := &fakeLLM{}
	svc, _ := NewService(Deps{Agents: repo, LLM: llm})
	dag := agent.DAG{
		Nodes: []agent.Node{
			{ID: "n1", Type: agent.NodeLLMCall, Ref: "gpt", Config: map[string]any{"prompt": "hi"}},
			{ID: "n2", Type: agent.NodeCondition, Config: map[string]any{"expr": "score > 0.5"}},
			{ID: "n3", Type: agent.NodeLLMCall, Ref: "gpt2", Config: map[string]any{"prompt": "cond:{n2}"}},
		},
		Edges: map[string][]string{"n1": {"n2"}, "n2": {"n3"}},
	}
	seedAgent(t, repo, dag)

	run, err := svc.StartRun(context.Background(), "t1", "ag1", "start", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != agent.RunSucceeded {
		t.Fatalf("status=%s out=%q", run.Status, run.FinalOutput)
	}
	
	if len(llm.calls) != 2 {
		t.Fatalf("llm called %d times, want 2", len(llm.calls))
	}
	if !contains(llm.calls[1].Messages[1].Content, "cond:score > 0.5") {
		t.Fatalf("n3 prompt missing condition output: %q", llm.calls[1].Messages[1].Content)
	}
	if run.FinalOutput != "answer:gpt2" {
		t.Fatalf("final output = %q", run.FinalOutput)
	}
}

func TestCompile_Condition_NoExprCompiles(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := NewService(Deps{Agents: repo})
	a, _ := agent.NewAgent("ag1", "t1", "demo", agent.DAG{
		Nodes: []agent.Node{
			{ID: "n1", Type: agent.NodeCondition, Config: map[string]any{}},
		},
	}, "u1")
	if err := repo.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Compile(context.Background(), "t1", "ag1"); err != nil {
		t.Fatalf("condition node without expr should still compile, got %v", err)
	}
}

func seedAgentNamed(t *testing.T, repo *fakeRepo, id string, dag agent.DAG) *agent.Agent {
	t.Helper()
	a, err := agent.NewAgent(id, "t1", "demo", dag, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Compile(); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	return a
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}