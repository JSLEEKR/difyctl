package lint

import (
	"fmt"
	"regexp"

	"github.com/JSLEEKR/difyctl/internal/model"
)

// varRefPattern matches Dify variable references: {{#node_id.var_name#}}.
var varRefPattern = regexp.MustCompile(`\{\{#([a-zA-Z0-9_\-]+)\.([a-zA-Z0-9_\-]+)#\}\}`)

// DIFY013 — unresolved-var-ref.
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
		refs := collectVarRefs(n.Data)
		for _, ref := range refs {
			src, ok := idToNode[ref.nodeID]
			if !ok {
				out = append(out, Finding{
					Rule:     "DIFY013",
					Severity: SeverityError,
					Message:  fmt.Sprintf("node '%s' references '{{#%s.%s#}}' but node '%s' does not exist", n.ID, ref.nodeID, ref.varName, ref.nodeID),
					Line:     n.Line,
				})
				continue
			}
			if !nodeDeclaresOutput(src, ref.varName) {
				out = append(out, Finding{
					Rule:     "DIFY013",
					Severity: SeverityError,
					Message:  fmt.Sprintf("node '%s' references '{{#%s.%s#}}' but node '%s' (type=%s) does not declare output '%s'", n.ID, ref.nodeID, ref.varName, ref.nodeID, src.Type, ref.varName),
					Line:     n.Line,
				})
			}
		}
	}
	return out
}

// varRef is a captured {{#node.var#}} reference.
type varRef struct {
	nodeID  string
	varName string
}

// collectVarRefs walks a generic value and pulls out all {{#x.y#}} occurrences.
func collectVarRefs(v any) []varRef {
	var out []varRef
	walkStrings(v, func(s string) {
		for _, m := range varRefPattern.FindAllStringSubmatch(s, -1) {
			out = append(out, varRef{nodeID: m[1], varName: m[2]})
		}
	})
	return out
}

// walkStrings calls fn on every string value found anywhere inside v.
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

// nodeDeclaresOutput reports whether the node declares a variable that downstream
// refs can resolve to. We accept common Dify conventions:
//   - Start node: list under data.variables[] with item.variable matching name
//   - Explicit: data.outputs / data.output_variables as a list of names or map
//   - Known types: llm exposes "text"; code exposes whatever outputs lists;
//     knowledge-retrieval exposes "result"; http-request exposes "body"/"status_code"
//     by default.
//   - parameter-extractor: data.parameters[].name
func nodeDeclaresOutput(n *model.Node, name string) bool {
	if n == nil {
		return false
	}
	// Built-in default outputs by node type.
	if defaultOutputs(n.Type)[name] {
		return true
	}
	data := n.Data
	if data == nil {
		return false
	}

	// Start variables.
	if IsStartType(n.Type) {
		if vars, ok := data["variables"].([]any); ok {
			for _, v := range vars {
				if m := asMap(v); m != nil {
					if s, ok := m["variable"].(string); ok && s == name {
						return true
					}
					if s, ok := m["name"].(string); ok && s == name {
						return true
					}
				}
			}
		}
	}

	// outputs / output_variables
	if decl := extractOutputs(data["outputs"]); decl[name] {
		return true
	}
	if decl := extractOutputs(data["output_variables"]); decl[name] {
		return true
	}

	// parameter-extractor specific.
	if n.Type == "parameter-extractor" || n.Type == "parameter_extractor" {
		if params, ok := data["parameters"].([]any); ok {
			for _, p := range params {
				if m := asMap(p); m != nil {
					if s, ok := m["name"].(string); ok && s == name {
						return true
					}
				}
			}
		}
	}

	// question-classifier exposes 'class_name' conventionally.
	if n.Type == "question-classifier" || n.Type == "question_classifier" {
		if name == "class_name" {
			return true
		}
	}

	// variable-assigner exposes the assigned var.
	if n.Type == "variable-assigner" || n.Type == "variable_assigner" {
		if items, ok := data["items"].([]any); ok {
			for _, it := range items {
				if m := asMap(it); m != nil {
					if s, ok := m["variable_selector"].([]any); ok && len(s) >= 2 {
						if tail, ok := s[len(s)-1].(string); ok && tail == name {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func defaultOutputs(t string) map[string]bool {
	switch t {
	case "llm":
		return map[string]bool{"text": true, "usage": true}
	case "knowledge-retrieval", "knowledge_retrieval":
		return map[string]bool{"result": true}
	case "http-request", "http_request":
		return map[string]bool{"body": true, "status_code": true, "headers": true}
	case "template-transform", "template_transform":
		return map[string]bool{"output": true}
	case "code":
		return map[string]bool{} // code declares its own outputs via data.outputs
	case "iteration":
		return map[string]bool{"output": true}
	case "variable-aggregator", "variable_aggregator":
		return map[string]bool{"output": true}
	case "tool":
		return map[string]bool{"text": true, "files": true}
	case "iteration-start", "iteration_start":
		// Dify conventionally exposes "item" and "index" inside an iteration body.
		return map[string]bool{"item": true, "index": true}
	}
	return map[string]bool{}
}

// extractOutputs supports outputs declared as a list of names, a list of
// objects {name: ...}, or a map keyed by name.
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

// asMap coerces a YAML-decoded map that might be map[string]any or map[any]any.
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
