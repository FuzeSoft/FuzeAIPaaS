package agent

import (
	"errors"
	"testing"
)

func linearDAG() DAG {
	return DAG{
		Nodes: []Node{
			{ID: "n1", Type: NodeLLMCall, Ref: "gpt", Config: map[string]any{"prompt": "hi"}},
			{ID: "n2", Type: NodeToolCall, Ref: "search"},
			{ID: "n3", Type: NodeHumanReview},
		},
		Edges: map[string][]string{"n1": {"n2"}, "n2": {"n3"}},
	}
}

func TestDAG_Validate_OK(t *testing.T) {
	if err := linearDAG().Validate(); err != nil {
		t.Fatalf("expected valid dag, got %v", err)
	}
}

func TestDAG_TopoSort_Order(t *testing.T) {
	order, err := linearDAG().TopoSort()
	if err != nil {
		t.Fatalf("topo failed: %v", err)
	}
	want := []string{"n1", "n2", "n3"}
	if len(order) != 3 || order[0] != want[0] || order[2] != want[2] {
		t.Fatalf("unexpected order %v", order)
	}
}

func TestDAG_Validate_Cycle(t *testing.T) {
	d := DAG{
		Nodes: []Node{{ID: "a", Type: NodeLLMCall}, {ID: "b", Type: NodeLLMCall}},
		Edges: map[string][]string{"a": {"b"}, "b": {"a"}},
	}
	if err := d.Validate(); err != ErrCycle {
		t.Fatalf("expected ErrCycle, got %v", err)
	}
}

func TestDAG_Validate_DupNode(t *testing.T) {
	d := DAG{
		Nodes: []Node{{ID: "a", Type: NodeLLMCall}, {ID: "a", Type: NodeLLMCall}},
		Edges: map[string][]string{},
	}
	if err := d.Validate(); !errors.Is(err, ErrDuplicateNode) {
		t.Fatalf("expected ErrDuplicateNode, got %v", err)
	}
}

func TestDAG_Validate_NoEntry(t *testing.T) {
	d := DAG{
		Nodes: []Node{{ID: "a", Type: NodeLLMCall}, {ID: "b", Type: NodeLLMCall}},
		Edges: map[string][]string{"a": {"b"}, "b": {"a"}}, 
	}
	if err := d.Validate(); err == nil {
		t.Fatalf("expected error for cyclic (no entry) dag")
	}
}

func TestDAG_Validate_MissingEdgeTarget(t *testing.T) {
	d := DAG{
		Nodes: []Node{{ID: "a", Type: NodeLLMCall}},
		Edges: map[string][]string{"a": {"ghost"}},
	}
	if err := d.Validate(); !errors.Is(err, ErrMissingNode) {
		t.Fatalf("expected ErrMissingNode, got %v", err)
	}
}

func TestNode_Validate_RefRequired(t *testing.T) {
	toolNode := Node{ID: "x", Type: NodeToolCall}
	if err := toolNode.Validate(); err == nil {
		t.Fatal("tool_call without ref should fail")
	}
	ragNode := Node{ID: "x", Type: NodeRagRetrieve}
	if err := ragNode.Validate(); err == nil {
		t.Fatal("rag_retrieve without ref should fail")
	}
}

func TestAgentCompile(t *testing.T) {
	a, err := NewAgent("ag1", "t1", "demo", linearDAG(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Compile(); err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if a.Status != AgentStatusCompiled {
		t.Fatalf("status = %s, want compiled", a.Status)
	}
}

func TestRunHumanPauseResume(t *testing.T) {
	r := NewRun("run1", "ag1", "t1", "start")
	r.Start()
	if r.Status != RunRunning {
		t.Fatal("should be running after Start")
	}
	r.PauseForHuman("n3", "approve?")
	if r.Status != RunPaused || r.PausedAt != "n3" {
		t.Fatalf("should be paused at n3, got %s/%s", r.Status, r.PausedAt)
	}
	if err := r.HumanResume("approved"); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if r.Status != RunRunning || r.PausedAt != "" {
		t.Fatalf("after resume should run, got %s/%s", r.Status, r.PausedAt)
	}
	
	if got := r.nodeOutput("n3"); got != "approved" {
		t.Fatalf("human decision not recorded: %q", got)
	}
}

func TestRunResumeWhenNotPaused(t *testing.T) {
	r := NewRun("r", "a", "t", "")
	if err := r.HumanResume("x"); err != ErrRunNotPaused {
		t.Fatalf("expected ErrRunNotPaused, got %v", err)
	}
}

func TestRunSucceed(t *testing.T) {
	r := NewRun("r", "a", "t", "")
	r.Start()
	r.Succeed("final answer")
	if r.Status != RunSucceeded || r.FinalOutput != "final answer" {
		t.Fatalf("succeed broken: %s/%q", r.Status, r.FinalOutput)
	}
}

func TestRenderPrompt(t *testing.T) {
	tpl := "use {n1} then {n2}"
	got := RenderPrompt(tpl, func(id string) string {
		if id == "n1" {
			return "A"
		}
		return ""
	})
	if got != "use A then {n2}" {
		t.Fatalf("render = %q", got)
	}
}

func TestExtractRagQuery(t *testing.T) {
	q := ExtractRagQuery("kb-1", map[string]any{"query": "天气", "top_k": float64(8)})
	if q.KnowledgeBaseID != "kb-1" || q.Query != "天气" || q.TopK != 8 {
		t.Fatalf("rag query broken: %+v", q)
	}
}