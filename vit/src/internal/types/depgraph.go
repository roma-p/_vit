package types

import (
	"strings"
	"time"
)

type DepGraph struct {
	Edges []DependencyGraphEdge                                 `json:"edges"`
	Nodes map[RepoUID]map[AssetPath]map[ObjectPath]DepGraphNode `json:"nodes,omitempty"`
}

type DepGraphNode struct {
	// PayloadFile: content root hash, empty for branches.
	PayloadFile string    `json:"payload_file,omitempty"`
	Author      string    `json:"author,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

type (
	RepoUID    = string
	AssetPath  = string
	ObjectPath = string
)

type DependencyGraphEdge struct {
	Consumer *Ref                `json:"consumer"`
	RefSpec  *Ref                `json:"ref_spec"`
	Provider *Ref                `json:"provider"`
	Name     string              `json:"name"`
	Type     AssetDependencyType `json:"type"`
}

func NewDepGraph() *DepGraph {
	return &DepGraph{
		Nodes: make(map[RepoUID]map[AssetPath]map[ObjectPath]DepGraphNode),
	}
}

func (d *DepGraph) AddNode(ref *Ref, node DepGraphNode) {
	if _, ok := d.Nodes[ref.RepoUID]; !ok {
		d.Nodes[ref.RepoUID] = make(map[AssetPath]map[ObjectPath]DepGraphNode)
	}
	if _, ok := d.Nodes[ref.RepoUID][ref.AssetPath]; !ok {
		d.Nodes[ref.RepoUID][ref.AssetPath] = make(map[ObjectPath]DepGraphNode)
	}
	d.Nodes[ref.RepoUID][ref.AssetPath][ref.ObjectPath()] = node
}

func (d *DepGraph) Connect(
	consumer, provider, refSpec *Ref,
	name string, edgeType AssetDependencyType,
) error {
	if edgeType == AssetDependencySource {
		if path := d.cyclePath(provider, consumer); path != nil {
			return newCircularDependencyError(path)
		}
	}
	d.Edges = append(
		d.Edges,
		DependencyGraphEdge{
			Consumer: consumer,
			Provider: provider,
			Name:     name,
			Type:     edgeType,
			RefSpec:  refSpec,
		},
	)
	return nil
}

func (d *DepGraph) Merge(other *DepGraph) {
	if other == nil {
		return
	}
	for ru, byAsset := range other.Nodes {
		for ap, byObj := range byAsset {
			for op, n := range byObj {
				if d.Nodes[ru] == nil {
					d.Nodes[ru] = make(map[AssetPath]map[ObjectPath]DepGraphNode)
				}
				if d.Nodes[ru][ap] == nil {
					d.Nodes[ru][ap] = make(map[ObjectPath]DepGraphNode)
				}
				d.Nodes[ru][ap][op] = n
			}
		}
	}
	have := make(map[string]bool, len(d.Edges))
	for _, e := range d.Edges {
		have[edgeKey(e)] = true
	}
	for _, e := range other.Edges {
		if !have[edgeKey(e)] {
			d.Edges = append(d.Edges, e)
			have[edgeKey(e)] = true
		}
	}
}

func edgeKey(e DependencyGraphEdge) string {
	provider := ""
	if e.Provider != nil {
		provider = e.Provider.RelativePath()
	}
	return e.Consumer.RelativePath() + "\x00" + provider + "\x00" +
		e.Name + "\x00" + string(e.Type)
}

// -- FILE LOCAL -------------------------------------------------------------

// CheckAddSourceCycle checks taht adding new branch does not create cycle.
func (d *DepGraph) CheckAddSourceCycle(consumer, target *Ref) error {
	if path := d.cyclePath(target, consumer); path != nil {
		return newCircularDependencyError(path)
	}
	return nil
}

func (d *DepGraph) cyclePath(start, target *Ref) []*Ref {
	// forbid cycle in asset sourcing. A-> B -> A
	sameAsset := func(a, b *Ref) bool {
		return a.RepoUID == b.RepoUID && a.AssetPath == b.AssetPath
	}
	visited := map[Ref]bool{}
	var search func(n *Ref, path []*Ref) []*Ref
	search = func(n *Ref, path []*Ref) []*Ref {
		path = append(path, n)
		if sameAsset(n, target) {
			return append([]*Ref(nil), path...)
		}
		if visited[*n] {
			return nil
		}
		visited[*n] = true
		for i := range d.Edges {
			e := d.Edges[i]
			// TODO: maybe product also?
			if e.Type == AssetDependencySource && sameAsset(e.Consumer, n) {
				if hit := search(e.Provider, path); hit != nil {
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
