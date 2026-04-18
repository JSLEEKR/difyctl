package lint

import "github.com/JSLEEKR/difyctl/internal/model"

// DIFY019 — iteration-missing-start: every iteration body must contain exactly one iteration-start.
type ruleIterationMissingStart struct{}

func (ruleIterationMissingStart) ID() string { return "DIFY019" }

func (ruleIterationMissingStart) Check(wf *model.Workflow) []Finding {
	// For every iteration node, count iteration-start nodes that declare it as parent.
	iters := map[string]int{}
	iterLines := map[string]int{}
	for _, n := range wf.Workflow.Graph.Nodes {
		if IsIterationType(n.Type) {
			iters[n.ID] = 0
			iterLines[n.ID] = n.Line
		}
	}
	if len(iters) == 0 {
		return nil
	}
	for _, n := range wf.Workflow.Graph.Nodes {
		if !IsIterationStart(n.Type) {
			continue
		}
		if n.Data == nil {
			continue
		}
		parent, _ := n.Data["parent_id"].(string)
		if parent == "" {
			continue
		}
		if _, ok := iters[parent]; ok {
			iters[parent]++
		}
	}
	var out []Finding
	for id, count := range iters {
		switch {
		case count == 0:
			out = append(out, Finding{
				Rule:     "DIFY019",
				Severity: SeverityError,
				Message:  "iteration node '" + id + "' has no iteration-start child",
				Line:     iterLines[id],
			})
		case count > 1:
			out = append(out, Finding{
				Rule:     "DIFY019",
				Severity: SeverityError,
				Message:  "iteration node '" + id + "' has multiple iteration-start children",
				Line:     iterLines[id],
			})
		}
	}
	return out
}
