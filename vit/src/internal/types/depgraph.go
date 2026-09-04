package types

import "strings"

type DepGraphPool struct {
	GraphPerAsset DepGraphPerAsset
}

func NewDepGraphPool() *DepGraphPool {
	return &DepGraphPool{
		GraphPerAsset: make(DepGraphPerAsset),
	}
}

type DepGraphPerAsset map[string]*DepGraph // <ref.ObjectPath()> -> graph UID

type DepGraph struct {
	Edges []DependencyGraphEdge                                 `json:"edges"`
	Nodes map[RepoUID]map[AssetPath]map[ObjectPath]DepGraphNode `json:"nodes,omitempty"`
}

type DepGraphNode struct {
	PayloadFile string `json:"payload_file,omitempty"`
	CommitID    string `json:"commit_id,omitempty"`
	// edges here?
}

type (
	RepoUID    = string
	AssetPath  = string
	ObjectPath = string
)

type DependencyGraphEdge struct {
	From    *Ref                `json:"from"`
	To      *Ref                `json:"to"`
	Name    string              `json:"name"`
	Type    AssetDependencyType `json:"type"`
	RefSpec *Ref                `json:"ref_spec"`
}

func NewDepGraph() *DepGraph {
	return &DepGraph{
		Nodes: make(map[RepoUID]map[AssetPath]map[ObjectPath]DepGraphNode),
	}
}

func (d *DepGraph) AddNode(ref *Ref, commitID, payloadFile string) {
	if _, ok := d.Nodes[ref.RepoUID]; !ok {
		d.Nodes[ref.RepoUID] = make(map[AssetPath]map[ObjectPath]DepGraphNode)
	}
	if _, ok := d.Nodes[ref.RepoUID][ref.AssetPath]; !ok {
		d.Nodes[ref.RepoUID][ref.AssetPath] = make(map[ObjectPath]DepGraphNode)
	}
	d.Nodes[ref.RepoUID][ref.AssetPath][ref.ObjectPath()] = DepGraphNode{
		PayloadFile: payloadFile,
		CommitID:    commitID,
	}
}

// Connect : refA has a dependancy to refB based on refSpec.
func (d *DepGraph) Connect(refA, refB, refSpec *Ref, name string, edgeType AssetDependencyType) error {
	if edgeType == AssetDependencySource {
		if path := d.cyclePath(refB, refA); path != nil {
			return newCircularDependencyError(path)
		}
	}
	d.Edges = append(
		d.Edges,
		DependencyGraphEdge{
			From:    refA,
			To:      refB,
			Name:    name,
			Type:    edgeType,
			RefSpec: refSpec,
		},
	)
	return nil
}

// -- FILE LOCAL -------------------------------------------------------------

func (d *DepGraph) cyclePath(start, target *Ref) []*Ref {
	visited := map[Ref]bool{}
	var search func(n *Ref, path []*Ref) []*Ref
	search = func(n *Ref, path []*Ref) []*Ref {
		path = append(path, n)
		if *n == *target {
			return append([]*Ref(nil), path...)
		}
		if visited[*n] {
			return nil
		}
		visited[*n] = true
		for i := range d.Edges {
			e := d.Edges[i]
			// TODO: maybe product also?
			if e.Type == AssetDependencySource && *e.From == *n {
				if hit := search(e.To, path); hit != nil {
					return hit
				}
			}
		}
		return nil
	}
	return search(start, nil)
}

func newCircularDependencyError(path []*Ref) error {
	chain := make([]string, len(path))
	for i, r := range path {
		chain[i] = r.RelativePath()
	}
	return NewVitError(
		ErrCircularDependency,
		[]string{"the dependency would create a cycle: " + strings.Join(chain, " -> ")},
		[]any{"cycle", chain},
	)
}
