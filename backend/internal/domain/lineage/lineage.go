
package lineage

import "sort"

type NodeType string

const (
	Code         NodeType = "code"
	Data         NodeType = "data"
	Hyperparam   NodeType = "hyperparam"
	Job          NodeType = "job"
	Run          NodeType = "run"
	ModelVersion NodeType = "model-version"
)

const (
	RelUses     = "uses"
	RelProduced = "produced"
	RelDerived  = "derived"
)

type Node struct {
	Type NodeType `json:"type"`
	ID   string   `json:"id"`
	
	Label string `json:"label,omitempty"`
	
	Attributes map[string]string `json:"attributes,omitempty"`
}

type Edge struct {
	From   string `json:"from"`   
	To     string `json:"to"`     
	Relation string `json:"relation"`
}

type Graph struct {
	Root *Node  `json:"root"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

func NodeID(t NodeType, id string) string {
	return string(t) + ":" + id
}

type Source struct {
	JobID      string
	RunID      string
	CodeRepo   string
	CodeCommit string
	Image      string
	TemplateID string
	DatasetID      string
	DatasetName    string
	DatasetVersion string
	Hyperparameters string 
}

func BuildGraph(modelVersionID string, src Source) *Graph {
	nodes := map[string]Node{}
	edges := map[string]Edge{}

	addNode := func(t NodeType, id, label string, attrs map[string]string) {
		if id == "" {
			return
		}
		k := NodeID(t, id)
		if _, ok := nodes[k]; !ok {
			nodes[k] = Node{Type: t, ID: id, Label: label, Attributes: attrs}
		}
	}
	addEdge := func(fromT NodeType, fromID string, toT NodeType, toID string, rel string) {
		if fromID == "" || toID == "" {
			return
		}
		fk, tk := NodeID(fromT, fromID), NodeID(toT, toID)
		ek := fk + "->" + tk + ":" + rel
		if _, ok := edges[ek]; !ok {
			edges[ek] = Edge{From: fk, To: tk, Relation: rel}
		}
	}

	mvID := NodeID(ModelVersion, modelVersionID)
	root := &Node{Type: ModelVersion, ID: modelVersionID}
	nodes[mvID] = *root

	codeID := src.CodeCommit
	if codeID == "" {
		codeID = src.CodeRepo 
	}
	addNode(Code, codeID, codeLabel(src), codeAttrs(src))
	
	dataID := src.DatasetID
	if dataID == "" {
		dataID = src.DatasetName
	}
	addNode(Data, dataID, dataLabel(src), dataAttrs(src))
	
	var hpID string
	if src.Hyperparameters != "" {
		hpID = "hp:" + modelVersionID
		addNode(Hyperparam, hpID, "hyperparameters", map[string]string{"params": src.Hyperparameters})
	}
	
	addNode(Job, src.JobID, "job "+src.JobID, nil)
	addNode(Run, src.RunID, "run "+src.RunID, nil)

	addEdge(Code, codeID, ModelVersion, modelVersionID, RelUses)
	addEdge(Data, dataID, ModelVersion, modelVersionID, RelUses)
	addEdge(Hyperparam, hpID, ModelVersion, modelVersionID, RelUses)
	addEdge(Job, src.JobID, ModelVersion, modelVersionID, RelProduced)
	addEdge(Run, src.RunID, ModelVersion, modelVersionID, RelProduced)
	addEdge(Job, src.JobID, Run, src.RunID, RelProduced)
	addEdge(Code, codeID, Job, src.JobID, RelUses)
	addEdge(Data, dataID, Job, src.JobID, RelUses)
	addEdge(Code, codeID, Run, src.RunID, RelUses)
	addEdge(Data, dataID, Run, src.RunID, RelUses)
	addEdge(Hyperparam, hpID, Run, src.RunID, RelUses)

	out := &Graph{Root: root}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, n)
	}
	for _, e := range edges {
		out.Edges = append(out.Edges, e)
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].ID < out.Nodes[j].ID })
	sort.Slice(out.Edges, func(i, j int) bool {
		if out.Edges[i].From != out.Edges[j].From {
			return out.Edges[i].From < out.Edges[j].From
		}
		return out.Edges[i].To < out.Edges[j].To
	})
	return out
}

func codeLabel(src Source) string {
	if src.CodeCommit != "" {
		return src.CodeCommit
	}
	return src.CodeRepo
}

func codeAttrs(src Source) map[string]string {
	a := map[string]string{}
	if src.CodeRepo != "" {
		a["repo"] = src.CodeRepo
	}
	if src.CodeCommit != "" {
		a["commit"] = src.CodeCommit
	}
	if src.Image != "" {
		a["image"] = src.Image
	}
	if src.TemplateID != "" {
		a["template_id"] = src.TemplateID
	}
	return a
}

func dataLabel(src Source) string {
	if src.DatasetName == "" {
		return src.DatasetID
	}
	if src.DatasetVersion != "" {
		return src.DatasetName + ":" + src.DatasetVersion
	}
	return src.DatasetName
}

func dataAttrs(src Source) map[string]string {
	a := map[string]string{}
	if src.DatasetID != "" {
		a["dataset_id"] = src.DatasetID
	}
	if src.DatasetName != "" {
		a["name"] = src.DatasetName
	}
	if src.DatasetVersion != "" {
		a["version"] = src.DatasetVersion
	}
	return a
}