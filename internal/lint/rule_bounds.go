package lint

import "github.com/JSLEEKR/difyctl/internal/model"

// DIFY015 — missing-start: at least one start node required.
type ruleMissingStart struct{}

func (ruleMissingStart) ID() string { return "DIFY015" }

func (ruleMissingStart) Check(wf *model.Workflow) []Finding {
	for _, n := range wf.Workflow.Graph.Nodes {
		if IsStartType(n.Type) {
			return nil
		}
	}
	return []Finding{{
		Rule:     "DIFY015",
		Severity: SeverityError,
		Message:  "workflow has no 'start' node",
	}}
}

// DIFY016 — missing-end: at least one end or answer node required.
type ruleMissingEnd struct{}

func (ruleMissingEnd) ID() string { return "DIFY016" }

func (ruleMissingEnd) Check(wf *model.Workflow) []Finding {
	for _, n := range wf.Workflow.Graph.Nodes {
		if IsEndType(n.Type) {
			return nil
		}
	}
	return []Finding{{
		Rule:     "DIFY016",
		Severity: SeverityError,
		Message:  "workflow has no 'end' or 'answer' node",
	}}
}
