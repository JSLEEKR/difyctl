package lint

import "github.com/JSLEEKR/difyctl/internal/model"

// DIFY009 — edge-dangling-source.
type ruleEdgeDanglingSource struct{}

func (ruleEdgeDanglingSource) ID() string { return "DIFY009" }

func (ruleEdgeDanglingSource) Check(wf *model.Workflow) []Finding {
	ids := nodeIDSet(wf)
	var out []Finding
	for _, e := range wf.Workflow.Graph.Edges {
		if e.Source == "" {
			out = append(out, Finding{
				Rule:     "DIFY009",
				Severity: SeverityError,
				Message:  "edge has empty 'source'",
				Line:     e.Line,
			})
			continue
		}
		if _, ok := ids[e.Source]; !ok {
			out = append(out, Finding{
				Rule:     "DIFY009",
				Severity: SeverityError,
				Message:  "edge source '" + e.Source + "' refers to non-existent node",
				Line:     e.Line,
			})
		}
	}
	return out
}

// DIFY010 — edge-dangling-target.
type ruleEdgeDanglingTarget struct{}

func (ruleEdgeDanglingTarget) ID() string { return "DIFY010" }

func (ruleEdgeDanglingTarget) Check(wf *model.Workflow) []Finding {
	ids := nodeIDSet(wf)
	var out []Finding
	for _, e := range wf.Workflow.Graph.Edges {
		if e.Target == "" {
			out = append(out, Finding{
				Rule:     "DIFY010",
				Severity: SeverityError,
				Message:  "edge has empty 'target'",
				Line:     e.Line,
			})
			continue
		}
		if _, ok := ids[e.Target]; !ok {
			out = append(out, Finding{
				Rule:     "DIFY010",
				Severity: SeverityError,
				Message:  "edge target '" + e.Target + "' refers to non-existent node",
				Line:     e.Line,
			})
		}
	}
	return out
}

// DIFY011 — duplicate-edge: same (source, target, sourceHandle) tuple.
type ruleDuplicateEdge struct{}

func (ruleDuplicateEdge) ID() string { return "DIFY011" }

func (ruleDuplicateEdge) Check(wf *model.Workflow) []Finding {
	seen := map[string]bool{}
	var out []Finding
	for _, e := range wf.Workflow.Graph.Edges {
		key := e.Source + "|" + e.Target + "|" + e.SourceHandle
		if seen[key] {
			out = append(out, Finding{
				Rule:     "DIFY011",
				Severity: SeverityError,
				Message:  "duplicate edge: source='" + e.Source + "' target='" + e.Target + "' handle='" + e.SourceHandle + "'",
				Line:     e.Line,
			})
			continue
		}
		seen[key] = true
	}
	return out
}

// nodeIDSet builds a set of node ids for O(1) lookups.
func nodeIDSet(wf *model.Workflow) map[string]struct{} {
	ids := make(map[string]struct{}, len(wf.Workflow.Graph.Nodes))
	for _, n := range wf.Workflow.Graph.Nodes {
		if n.ID != "" {
			ids[n.ID] = struct{}{}
		}
	}
	return ids
}
