// Package diff computes a semantic diff between two Dify workflow DSLs.
//
// Unlike `git diff`, we match nodes by id rather than position and categorize
// changes into ADDED / REMOVED / CHANGED / BREAKING. BREAKING is reserved for
// changes that are likely to silently break downstream references
// (e.g. a variable reference whose source no longer exists).
package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"

	"github.com/JSLEEKR/difyctl/internal/model"
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
func diffBreakingVarRefs(a, b *model.Workflow) []Change {
	bNodes := indexNodes(b)
	bOutputs := map[string]map[string]bool{}
	for id, n := range bNodes {
		bOutputs[id] = gatherOutputs(n)
	}

	var out []Change
	seen := map[string]bool{}
	for _, n := range a.Workflow.Graph.Nodes {
		refs := collectVarRefs(n.Data)
		for _, r := range refs {
			key := n.ID + "|" + r.NodeID + "." + r.VarName
			if seen[key] {
				continue
			}
			seen[key] = true
			bNode, exists := bNodes[r.NodeID]
			if !exists {
				out = append(out, Change{
					Category: CategoryBreaking,
					Kind:     "variable-ref",
					ID:       n.ID,
					Detail:   "reference to {{#" + r.NodeID + "." + r.VarName + "#}} broken: node '" + r.NodeID + "' removed",
				})
				continue
			}
			_ = bNode
			if !bOutputs[r.NodeID][r.VarName] {
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

// VarRef is a captured variable reference.
type VarRef struct {
	NodeID  string
	VarName string
}

var varRefPattern = regexp.MustCompile(`\{\{#([a-zA-Z0-9_\-]+)\.([a-zA-Z0-9_\-]+)#\}\}`)

func collectVarRefs(v any) []VarRef {
	var out []VarRef
	walkStrings(v, func(s string) {
		for _, m := range varRefPattern.FindAllStringSubmatch(s, -1) {
			out = append(out, VarRef{NodeID: m[1], VarName: m[2]})
		}
	})
	return out
}

func walkStrings(v any, fn func(string)) {
	switch t := v.(type) {
	case string:
		fn(t)
	case map[string]any:
		for _, vv := range t {
			walkStrings(vv, fn)
		}
	case map[any]any:
		for _, vv := range t {
			walkStrings(vv, fn)
		}
	case []any:
		for _, vv := range t {
			walkStrings(vv, fn)
		}
	}
}

// gatherOutputs enumerates the output variable names a node declares.
func gatherOutputs(n *model.Node) map[string]bool {
	out := map[string]bool{}
	if n == nil {
		return out
	}
	// default outputs per type
	for k, ok := range defaultTypeOutputs(n.Type) {
		if ok {
			out[k] = true
		}
	}
	if n.Data == nil {
		return out
	}
	// declared outputs / output_variables.
	for _, key := range []string{"outputs", "output_variables"} {
		merge(out, extractOutputs(n.Data[key]))
	}
	// start.variables
	if n.Type == "start" {
		if vars, ok := n.Data["variables"].([]any); ok {
			for _, v := range vars {
				if m := asMap(v); m != nil {
					if s, ok := m["variable"].(string); ok {
						out[s] = true
					}
					if s, ok := m["name"].(string); ok {
						out[s] = true
					}
				}
			}
		}
	}
	// parameter-extractor
	if n.Type == "parameter-extractor" || n.Type == "parameter_extractor" {
		if params, ok := n.Data["parameters"].([]any); ok {
			for _, p := range params {
				if m := asMap(p); m != nil {
					if s, ok := m["name"].(string); ok {
						out[s] = true
					}
				}
			}
		}
	}
	return out
}

func defaultTypeOutputs(t string) map[string]bool {
	switch t {
	case "llm":
		return map[string]bool{"text": true, "usage": true}
	case "knowledge-retrieval", "knowledge_retrieval":
		return map[string]bool{"result": true}
	case "http-request", "http_request":
		return map[string]bool{"body": true, "status_code": true, "headers": true}
	case "template-transform", "template_transform":
		return map[string]bool{"output": true}
	case "iteration":
		return map[string]bool{"output": true}
	case "variable-aggregator", "variable_aggregator":
		return map[string]bool{"output": true}
	case "tool":
		return map[string]bool{"text": true, "files": true}
	case "iteration-start", "iteration_start":
		return map[string]bool{"item": true, "index": true}
	}
	return map[string]bool{}
}

func extractOutputs(v any) map[string]bool {
	out := map[string]bool{}
	switch t := v.(type) {
	case []any:
		for _, el := range t {
			switch x := el.(type) {
			case string:
				out[x] = true
			case map[string]any:
				if s, ok := x["name"].(string); ok {
					out[s] = true
				}
				if s, ok := x["variable"].(string); ok {
					out[s] = true
				}
			case map[any]any:
				if s, ok := x["name"].(string); ok {
					out[s] = true
				}
			}
		}
	case map[string]any:
		for k := range t {
			out[k] = true
		}
	case map[any]any:
		for k := range t {
			if s, ok := k.(string); ok {
				out[s] = true
			}
		}
	}
	return out
}

func asMap(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, vv := range t {
			if s, ok := k.(string); ok {
				m[s] = vv
			}
		}
		return m
	}
	return nil
}

func merge(dst, src map[string]bool) {
	for k, v := range src {
		if v {
			dst[k] = true
		}
	}
}
