package lint

import (
	"fmt"

	"github.com/JSLEEKR/difyctl/internal/model"
	"github.com/JSLEEKR/difyctl/internal/varref"
)

// DIFY013 — unresolved-var-ref. Flags references like `{{#node.var#}}` whose
// target node either does not exist or does not declare the named output.
//
// The decision of "does node X declare output Y" is delegated to internal/varref
// so that lint and diff can never disagree for the same input — see the
// regression fixture in varref/varref_test.go.
type ruleUnresolvedVarRef struct{}

func (ruleUnresolvedVarRef) ID() string { return "DIFY013" }

func (ruleUnresolvedVarRef) Check(wf *model.Workflow) []Finding {
	idToNode := map[string]*model.Node{}
	for i := range wf.Workflow.Graph.Nodes {
		n := &wf.Workflow.Graph.Nodes[i]
		if n.ID != "" {
			idToNode[n.ID] = n
		}
	}

	var out []Finding
	for i := range wf.Workflow.Graph.Nodes {
		n := &wf.Workflow.Graph.Nodes[i]
		refs := varref.Collect(n.Data)
		for _, ref := range refs {
			src, ok := idToNode[ref.NodeID]
			if !ok {
				out = append(out, Finding{
					Rule:     "DIFY013",
					Severity: SeverityError,
					Message:  fmt.Sprintf("node '%s' references '{{#%s.%s#}}' but node '%s' does not exist", n.ID, ref.NodeID, ref.VarName, ref.NodeID),
					Line:     n.Line,
				})
				continue
			}
			if !varref.NodeDeclaresOutput(src, ref.VarName) {
				out = append(out, Finding{
					Rule:     "DIFY013",
					Severity: SeverityError,
					Message:  fmt.Sprintf("node '%s' references '{{#%s.%s#}}' but node '%s' (type=%s) does not declare output '%s'", n.ID, ref.NodeID, ref.VarName, ref.NodeID, src.Type, ref.VarName),
					Line:     n.Line,
				})
			}
		}
	}
	return out
}
