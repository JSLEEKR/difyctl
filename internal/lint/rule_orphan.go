package lint

import "github.com/JSLEEKR/difyctl/internal/model"

// DIFY012 — orphan-node: non-Start node with no incoming AND no outgoing edges.
type ruleOrphanNode struct{}

func (ruleOrphanNode) ID() string { return "DIFY012" }

func (ruleOrphanNode) Check(wf *model.Workflow) []Finding {
	inDeg := map[string]int{}
	outDeg := map[string]int{}
	for _, e := range wf.Workflow.Graph.Edges {
		if e.Target != "" {
			inDeg[e.Target]++
		}
		if e.Source != "" {
			outDeg[e.Source]++
		}
	}
	var out []Finding
	for _, n := range wf.Workflow.Graph.Nodes {
		if n.ID == "" {
			continue
		}
		if IsStartType(n.Type) {
			continue
		}
		if inDeg[n.ID] == 0 && outDeg[n.ID] == 0 {
			out = append(out, Finding{
				Rule:     "DIFY012",
				Severity: SeverityWarning,
				Message:  "orphan node '" + n.ID + "' has no incoming or outgoing edges",
				Line:     n.Line,
			})
		}
	}
	return out
}
