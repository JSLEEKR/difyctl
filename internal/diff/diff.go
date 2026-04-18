// Package diff computes a semantic diff between two Dify workflow DSLs.
//
// Unlike `git diff`, we match nodes by id rather than position and categorize
// changes into ADDED / REMOVED / CHANGED / BREAKING. BREAKING is reserved for
// changes that are likely to silently break downstream references
// (e.g. a variable reference whose source no longer exists).
//
// Variable-ref parsing and output-declaration semantics are delegated to
// internal/varref so that lint DIFY013 and diff BREAKING-var-ref can never
// disagree for identical input.
package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/JSLEEKR/difyctl/internal/model"
	"github.com/JSLEEKR/difyctl/internal/varref"
)

// Category constants.
const (
	CategoryAdded    = "ADDED"
	CategoryRemoved  = "REMOVED"
	CategoryChanged  = "CHANGED"
	CategoryBreaking = "BREAKING"
)

// Change is a single delta between two workflows.
type Change struct {
	Category string `json:"category"`
	Kind     string `json:"kind"` // node | edge | variable-ref | app
	ID       string `json:"id"`
	Detail   string `json:"detail,omitempty"`
}

// Compute produces the list of changes going from a -> b.
// The result is sorted by (Category, Kind, ID) for deterministic output.
func Compute(a, b *model.Workflow) []Change {
	var out []Change
	out = append(out, diffApp(a, b)...)
	out = append(out, diffNodes(a, b)...)
	out = append(out, diffEdges(a, b)...)
	out = append(out, diffBreakingVarRefs(a, b)...)
	sortChanges(out)
	return out
}

// sortChanges orders changes deterministically.
func sortChanges(changes []Change) {
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].Category != changes[j].Category {
			return categoryOrder(changes[i].Category) < categoryOrder(changes[j].Category)
		}
		if changes[i].Kind != changes[j].Kind {
			return changes[i].Kind < changes[j].Kind
		}
		if changes[i].ID != changes[j].ID {
			return changes[i].ID < changes[j].ID
		}
		return changes[i].Detail < changes[j].Detail
	})
}

func categoryOrder(c string) int {
	switch c {
	case CategoryBreaking:
		return 0
	case CategoryRemoved:
		return 1
	case CategoryAdded:
		return 2
	case CategoryChanged:
		return 3
	}
	return 4
}

func diffApp(a, b *model.Workflow) []Change {
	var out []Change
	if a.App.Name != b.App.Name {
		out = append(out, Change{Category: CategoryChanged, Kind: "app", ID: "name", Detail: "'" + a.App.Name + "' -> '" + b.App.Name + "'"})
	}
	if a.App.Mode != b.App.Mode {
		out = append(out, Change{Category: CategoryChanged, Kind: "app", ID: "mode", Detail: "'" + a.App.Mode + "' -> '" + b.App.Mode + "'"})
	}
	if a.Version != b.Version {
		out = append(out, Change{Category: CategoryChanged, Kind: "app", ID: "version", Detail: "'" + a.Version + "' -> '" + b.Version + "'"})
	}
	return out
}

func diffNodes(a, b *model.Workflow) []Change {
	aMap := indexNodes(a)
	bMap := indexNodes(b)
	var out []Change
	for id, an := range aMap {
		bn, ok := bMap[id]
		if !ok {
			out = append(out, Change{Category: CategoryRemoved, Kind: "node", ID: id, Detail: "type=" + an.Type})
			continue
		}
		// Same id exists on both sides — compare.
		aHash := nodeDataHash(an)
		bHash := nodeDataHash(bn)
		if aHash != bHash {
			// Is the only change position?
			if dataHashExcludingPosition(an) == dataHashExcludingPosition(bn) {
				out = append(out, Change{Category: CategoryChanged, Kind: "node", ID: id, Detail: "moved"})
			} else {
				out = append(out, Change{Category: CategoryChanged, Kind: "node", ID: id, Detail: "body-changed"})
			}
		}
		if an.Type != bn.Type {
			out = append(out, Change{Category: CategoryChanged, Kind: "node", ID: id, Detail: "type " + an.Type + " -> " + bn.Type})
		}
	}
	for id, bn := range bMap {
		if _, ok := aMap[id]; !ok {
			out = append(out, Change{Category: CategoryAdded, Kind: "node", ID: id, Detail: "type=" + bn.Type})
		}
	}
	return out
}

func diffEdges(a, b *model.Workflow) []Change {
	aMap := indexEdges(a)
	bMap := indexEdges(b)
	var out []Change
	for key, ae := range aMap {
		if _, ok := bMap[key]; !ok {
			out = append(out, Change{Category: CategoryRemoved, Kind: "edge", ID: edgeDisplayID(ae), Detail: ae.Source + " -> " + ae.Target})
		}
	}
	for key, be := range bMap {
		if _, ok := aMap[key]; !ok {
			out = append(out, Change{Category: CategoryAdded, Kind: "edge", ID: edgeDisplayID(be), Detail: be.Source + " -> " + be.Target})
		}
	}
	return out
}

// diffBreakingVarRefs finds references in `a` whose source (node or variable)
// is missing or renamed in `b`.
//
// A ref is flagged BREAKING only if the SAME ref was resolvable in a (the
// "before" state). This avoids reporting pre-existing unresolved refs — those
// are already caught by lint DIFY013 and are not introduced by this diff.
func diffBreakingVarRefs(a, b *model.Workflow) []Change {
	aNodes := indexNodes(a)
	bNodes := indexNodes(b)

	var out []Change
	seen := map[string]bool{}
	for _, n := range a.Workflow.Graph.Nodes {
		refs := varref.Collect(n.Data)
		for _, r := range refs {
			key := n.ID + "|" + r.NodeID + "." + r.VarName
			if seen[key] {
				continue
			}
			seen[key] = true

			// If the reference was ALREADY broken in a, it is not a change we introduced.
			aSrc, aExisted := aNodes[r.NodeID]
			aResolved := aExisted && varref.NodeDeclaresOutput(aSrc, r.VarName)
			if !aResolved {
				continue
			}

			bSrc, bExists := bNodes[r.NodeID]
			if !bExists {
				out = append(out, Change{
					Category: CategoryBreaking,
					Kind:     "variable-ref",
					ID:       n.ID,
					Detail:   "reference to {{#" + r.NodeID + "." + r.VarName + "#}} broken: node '" + r.NodeID + "' removed",
				})
				continue
			}
			if !varref.NodeDeclaresOutput(bSrc, r.VarName) {
				out = append(out, Change{
					Category: CategoryBreaking,
					Kind:     "variable-ref",
					ID:       n.ID,
					Detail:   "reference to {{#" + r.NodeID + "." + r.VarName + "#}} broken: output '" + r.VarName + "' removed from '" + r.NodeID + "'",
				})
			}
		}
	}
	return out
}

// --- helpers ---

func indexNodes(wf *model.Workflow) map[string]*model.Node {
	out := map[string]*model.Node{}
	for i := range wf.Workflow.Graph.Nodes {
		n := &wf.Workflow.Graph.Nodes[i]
		if n.ID != "" {
			out[n.ID] = n
		}
	}
	return out
}

func indexEdges(wf *model.Workflow) map[string]model.Edge {
	out := map[string]model.Edge{}
	for _, e := range wf.Workflow.Graph.Edges {
		out[e.Source+"|"+e.Target+"|"+e.SourceHandle] = e
	}
	return out
}

func edgeDisplayID(e model.Edge) string {
	if e.ID != "" {
		return e.ID
	}
	return e.Source + "->" + e.Target
}

// nodeDataHash digests a node's {type, data, position} deterministically.
func nodeDataHash(n *model.Node) string {
	return hashOf(map[string]any{
		"type":     n.Type,
		"data":     normalize(n.Data),
		"position": normalize(n.Position),
	})
}

// dataHashExcludingPosition is used to detect "moved-only" changes.
func dataHashExcludingPosition(n *model.Node) string {
	return hashOf(map[string]any{
		"type": n.Type,
		"data": normalize(n.Data),
	})
}

// hashOf canonical-JSON-encodes v then SHA-256s.
func hashOf(v any) string {
	buf, _ := json.Marshal(v)
	h := sha256.Sum256(buf)
	return hex.EncodeToString(h[:])
}

// normalize converts map[any]any (from yaml.v3) to map[string]any recursively
// and sorts map keys for deterministic JSON encoding.
func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[k] = normalize(vv)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			if s, ok := k.(string); ok {
				out[s] = normalize(vv)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, vv := range t {
			out[i] = normalize(vv)
		}
		return out
	}
	return v
}
