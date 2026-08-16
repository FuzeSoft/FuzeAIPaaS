package lineage

import "testing"

func TestBuildGraphBasic(t *testing.T) {
	src := Source{
		JobID:          "job-1",
		RunID:          "run-1",
		CodeRepo:       "git@repo:train.git",
		CodeCommit:     "abc123",
		Image:          "registry/train:v1",
		TemplateID:     "tpl-1",
		DatasetID:      "ds-1",
		DatasetName:    "cifar10",
		DatasetVersion: "v3",
		Hyperparameters: `{"lr":0.01}`,
	}
	g := BuildGraph("mv-1", src)

	if len(g.Nodes) != 6 {
		t.Fatalf("expected 6 nodes, got %d: %+v", len(g.Nodes), g.Nodes)
	}
	if g.Root == nil || g.Root.ID != "mv-1" {
		t.Fatalf("root should be mv-1, got %+v", g.Root)
	}

	var codeNode *Node
	for i := range g.Nodes {
		if g.Nodes[i].Type == Code {
			codeNode = &g.Nodes[i]
		}
	}
	if codeNode == nil {
		t.Fatal("code node missing")
	}
	if codeNode.Label != "abc123" {
		t.Fatalf("code label should be commit, got %s", codeNode.Label)
	}
	if codeNode.Attributes["repo"] != "git@repo:train.git" {
		t.Fatalf("code repo attr missing: %+v", codeNode.Attributes)
	}
	if codeNode.Attributes["image"] != "registry/train:v1" {
		t.Fatalf("code image attr missing: %+v", codeNode.Attributes)
	}

	var dataNode *Node
	for i := range g.Nodes {
		if g.Nodes[i].Type == Data {
			dataNode = &g.Nodes[i]
		}
	}
	if dataNode == nil || dataNode.Label != "cifar10:v3" {
		t.Fatalf("data label should be cifar10:v3, got %+v", dataNode)
	}

	wantEdges := []Edge{
		{From: NodeID(Code, "abc123"), To: NodeID(ModelVersion, "mv-1"), Relation: RelUses},
		{From: NodeID(Data, "ds-1"), To: NodeID(ModelVersion, "mv-1"), Relation: RelUses},
		{From: NodeID(Hyperparam, "hp:mv-1"), To: NodeID(ModelVersion, "mv-1"), Relation: RelUses},
		{From: NodeID(Job, "job-1"), To: NodeID(ModelVersion, "mv-1"), Relation: RelProduced},
		{From: NodeID(Run, "run-1"), To: NodeID(ModelVersion, "mv-1"), Relation: RelProduced},
		{From: NodeID(Job, "job-1"), To: NodeID(Run, "run-1"), Relation: RelProduced},
		{From: NodeID(Code, "abc123"), To: NodeID(Job, "job-1"), Relation: RelUses},
		{From: NodeID(Data, "ds-1"), To: NodeID(Job, "job-1"), Relation: RelUses},
		{From: NodeID(Code, "abc123"), To: NodeID(Run, "run-1"), Relation: RelUses},
		{From: NodeID(Data, "ds-1"), To: NodeID(Run, "run-1"), Relation: RelUses},
		{From: NodeID(Hyperparam, "hp:mv-1"), To: NodeID(Run, "run-1"), Relation: RelUses},
	}
	got := map[string]Edge{}
	for _, e := range g.Edges {
		got[e.From+"->"+e.To+":"+e.Relation] = e
	}
	for _, we := range wantEdges {
		k := we.From + "->" + we.To + ":" + we.Relation
		if _, ok := got[k]; !ok {
			t.Errorf("missing edge %s", k)
		}
	}
	if len(g.Edges) != len(wantEdges) {
		t.Fatalf("expected %d edges, got %d: %+v", len(wantEdges), len(g.Edges), g.Edges)
	}
}

func TestBuildGraphSkipsEmptyNodes(t *testing.T) {
	
	g := BuildGraph("mv-2", Source{JobID: "job-9"})
	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(g.Nodes))
	}
	for _, n := range g.Nodes {
		if n.Type == Code || n.Type == Data || n.Type == Run {
			t.Fatalf("should not include empty node %s", n.Type)
		}
	}
}