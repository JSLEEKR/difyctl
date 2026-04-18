package lint

import "github.com/JSLEEKR/difyctl/internal/model"

// DIFY020 — unreachable-from-start: non-start nodes that cannot be reached
// from any start node via forward edge traversal. Warning-severity since some
// workflows legitimately split flows.
type ruleUnreachableFromStart struct{}

func (ruleUnreachableFromStart) ID() string { return "DIFY020" }

func (ruleUnreachableFromStart) Check(wf *model.Workflow) []Finding {
	adj := map[string][]string{}
	for _, n := range wf.Workflow.Graph.Nodes {
		if n.ID != "" {
			adj[n.ID] = nil
		}
	}
	for _, e := range wf.Workflow.Graph.Edges {
		if _, ok := adj[e.Source]; !ok {
			continue
		}
		if _, ok := adj[e.Target]; !ok {
			continue
		}
		adj[e.Source] = append(adj[e.Source], e.Target)
	}

	reached := map[string]bool{}
	var stack []string
	for _, n := range wf.Workflow.Graph.Nodes {
		if IsStartType(n.Type) && n.ID != "" {
			stack = append(stack, n.ID)
			reached[n.ID] = true
		}
	}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, nxt := range adj[cur] {
			if !reached[nxt] {
				reached[nxt] = true
				stack = append(stack, nxt)
			}
		}
	}

	var out []Finding
	// Nodes inside an iteration body are reached via the iteration node conceptually;
	// skip them to reduce noise.
	iterMembers := iterationBodyNodes(wf)
	for _, n := range wf.Workflow.Graph.Nodes {
		if n.ID == "" || IsStartType(n.Type) {
			continue
		}
		if iterMembers[n.ID] {
			continue
		}
		if !reached[n.ID] {
			out = append(out, Finding{
				Rule:     "DIFY020",
				Severity: SeverityWarning,
				Message:  "node '" + n.ID + "' is not reachable from any 'start' node",
				Line:     n.Line,
			})
		}
	}
	return out
}
