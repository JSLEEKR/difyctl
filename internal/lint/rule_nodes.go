package lint

import (
	"fmt"

	"github.com/JSLEEKR/difyctl/internal/model"
)

// DIFY005 — missing-node-id.
type ruleMissingNodeID struct{}

func (ruleMissingNodeID) ID() string { return "DIFY005" }

func (ruleMissingNodeID) Check(wf *model.Workflow) []Finding {
	var out []Finding
	for _, n := range wf.Workflow.Graph.Nodes {
		if n.ID == "" {
			out = append(out, Finding{
				Rule:     "DIFY005",
				Severity: SeverityError,
				Message:  "node is missing required 'id' field",
				Line:     n.Line,
			})
		}
	}
	return out
}

// DIFY006 — duplicate-node-id.
type ruleDuplicateNodeID struct{}

func (ruleDuplicateNodeID) ID() string { return "DIFY006" }

func (ruleDuplicateNodeID) Check(wf *model.Workflow) []Finding {
	seen := make(map[string]int)
	var out []Finding
	for _, n := range wf.Workflow.Graph.Nodes {
		if n.ID == "" {
			continue
		}
		if firstLine, ok := seen[n.ID]; ok {
			msg := "duplicate node id '" + n.ID + "'"
			if firstLine > 0 {
				msg += fmt.Sprintf(" (first defined at line %d)", firstLine)
			}
			out = append(out, Finding{
				Rule:     "DIFY006",
				Severity: SeverityError,
				Message:  msg,
				Line:     n.Line,
			})
		} else {
			seen[n.ID] = n.Line
		}
	}
	return out
}

// DIFY007 — unknown-node-type.
type ruleUnknownNodeType struct{}

func (ruleUnknownNodeType) ID() string { return "DIFY007" }

func (ruleUnknownNodeType) Check(wf *model.Workflow) []Finding {
	var out []Finding
	for _, n := range wf.Workflow.Graph.Nodes {
		if n.Type == "" {
			out = append(out, Finding{
				Rule:     "DIFY007",
				Severity: SeverityError,
				Message:  "node '" + n.ID + "' is missing required 'type'",
				Line:     n.Line,
			})
			continue
		}
		if !IsKnownNodeType(n.Type) {
			out = append(out, Finding{
				Rule:     "DIFY007",
				Severity: SeverityError,
				Message:  "node '" + n.ID + "' has unknown type '" + n.Type + "'",
				Line:     n.Line,
			})
		}
	}
	return out
}

// DIFY008 — missing-node-data.
type ruleMissingNodeData struct{}

func (ruleMissingNodeData) ID() string { return "DIFY008" }

func (ruleMissingNodeData) Check(wf *model.Workflow) []Finding {
	var out []Finding
	for _, n := range wf.Workflow.Graph.Nodes {
		if n.Data == nil {
			out = append(out, Finding{
				Rule:     "DIFY008",
				Severity: SeverityError,
				Message:  "node '" + n.ID + "' has no 'data' map",
				Line:     n.Line,
			})
		}
	}
	return out
}
