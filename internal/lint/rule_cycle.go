package lint

import (
	"sort"

	"github.com/JSLEEKR/difyctl/internal/model"
)

// DIFY014 — graph-cycle: iterative DFS on edges that are NOT inside an
// iteration body. Iteration bodies are expected to loop by construction.
type ruleGraphCycle struct{}

func (ruleGraphCycle) ID() string { return "DIFY014" }

func (ruleGraphCycle) Check(wf *model.Workflow) []Finding {
	// Build adjacency, skipping edges inside iteration bodies.
	iterationMembers := iterationBodyNodes(wf)
	adj := map[string][]string{}
	allNodes := []string{}
	for _, n := range wf.Workflow.Graph.Nodes {
		if n.ID == "" {
			continue
		}
		adj[n.ID] = nil
		allNodes = append(allNodes, n.ID)
	}
	for _, e := range wf.Workflow.Graph.Edges {
		if iterationMembers[e.Source] && iterationMembers[e.Target] {
			// edge inside an iteration body; skip from cycle check.
			continue
		}
		if _, ok := adj[e.Source]; !ok {
			continue // dangling source — covered by DIFY009
		}
		if _, ok := adj[e.Target]; !ok {
			continue
		}
		adj[e.Source] = append(adj[e.Source], e.Target)
	}
	sort.Strings(allNodes)

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var out []Finding
	var cycleReported = map[string]bool{}

	for _, start := range allNodes {
		if color[start] != white {
			continue
		}
		// Iterative DFS using an explicit stack. Each frame records the node and
		// the index of the next outgoing edge to explore.
		type frame struct {
			node string
			idx  int
		}
		stack := []frame{{node: start, idx: 0}}
		color[start] = gray
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			nbrs := adj[top.node]
			if top.idx >= len(nbrs) {
				color[top.node] = black
				stack = stack[:len(stack)-1]
				continue
			}
			next := nbrs[top.idx]
			top.idx++
			switch color[next] {
			case white:
				color[next] = gray
				stack = append(stack, frame{node: next, idx: 0})
			case gray:
				// Found a back-edge -> cycle. Report once per (from->to) pair.
				key := top.node + "->" + next
				if !cycleReported[key] {
					cycleReported[key] = true
					out = append(out, Finding{
						Rule:     "DIFY014",
						Severity: SeverityError,
						Message:  "cycle detected: back-edge '" + top.node + "' -> '" + next + "'",
					})
				}
			}
		}
	}
	return out
}

// iterationBodyNodes returns a set of node ids that are inside an iteration
// body. Dify marks this on a node with data.parent_id pointing at an iteration
// node, or by nesting iteration subgraphs.
func iterationBodyNodes(wf *model.Workflow) map[string]bool {
	iters := map[string]bool{}
	for _, n := range wf.Workflow.Graph.Nodes {
		if IsIterationType(n.Type) {
			iters[n.ID] = true
		}
	}
	members := map[string]bool{}
	for _, n := range wf.Workflow.Graph.Nodes {
		if n.Data == nil {
			continue
		}
		if parent, ok := n.Data["parent_id"].(string); ok && iters[parent] {
			members[n.ID] = true
		}
		// The iteration-start node is also part of the body.
		if IsIterationStart(n.Type) {
			members[n.ID] = true
		}
	}
	// The iteration container itself is not part of its own body.
	return members
}
