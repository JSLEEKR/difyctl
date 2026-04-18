// Package varref holds the shared logic for parsing Dify variable references
// ({{#node_id.var_name#}}) and determining which outputs a node declares.
//
// Both internal/lint (DIFY013 unresolved-var-ref) and internal/diff (BREAKING
// variable-ref detection) MUST agree byte-for-byte on whether a given
// (node, output-name) pair is resolvable. Previously each package carried its
// own copy of the logic, which drifted silently — see the Cycle B regression
// fixture for question-classifier and variable-assigner nodes where lint said
// "fine" and diff said "BREAKING" for the same input. This package exists so
// that drift cannot recur.
package varref

import (
	"regexp"

	"github.com/JSLEEKR/difyctl/internal/model"
)

// Pattern matches Dify variable references: {{#node_id.var_name#}}.
// Exported so rule and diff code share the exact same regex.
var Pattern = regexp.MustCompile(`\{\{#([a-zA-Z0-9_\-]+)\.([a-zA-Z0-9_\-]+)#\}\}`)

// Ref is a captured variable reference.
type Ref struct {
	NodeID  string
	VarName string
}

// Collect walks a generic value and pulls out all {{#x.y#}} occurrences.
func Collect(v any) []Ref {
	var out []Ref
	WalkStrings(v, func(s string) {
		for _, m := range Pattern.FindAllStringSubmatch(s, -1) {
			out = append(out, Ref{NodeID: m[1], VarName: m[2]})
		}
	})
	return out
}

// WalkStrings calls fn on every string value found anywhere inside v.
func WalkStrings(v any, fn func(string)) {
	switch t := v.(type) {
	case string:
		fn(t)
	case map[string]any:
		for _, vv := range t {
			WalkStrings(vv, fn)
		}
	case map[any]any:
		for _, vv := range t {
			WalkStrings(vv, fn)
		}
	case []any:
		for _, vv := range t {
			WalkStrings(vv, fn)
		}
	}
}

// AsMap coerces a YAML-decoded map that might be map[string]any or map[any]any
// into a single canonical map[string]any. Non-string keys are dropped.
func AsMap(v any) map[string]any {
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

// DefaultTypeOutputs returns the built-in output names for a node type —
// before looking at explicit declarations under data.outputs / data.output_variables.
func DefaultTypeOutputs(t string) map[string]bool {
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
	case "question-classifier", "question_classifier":
		// Dify conventionally exposes 'class_name' after classification.
		return map[string]bool{"class_name": true}
	case "code":
		// code declares its outputs via data.outputs only.
		return map[string]bool{}
	}
	return map[string]bool{}
}

// ExtractOutputs supports outputs declared as a list of names, a list of
// objects {name: ...} / {variable: ...}, or a map keyed by name.
func ExtractOutputs(v any) map[string]bool {
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
				if s, ok := x["variable"].(string); ok {
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

// GatherOutputs enumerates all output variable names a node declares — the
// SINGLE source of truth shared by lint DIFY013 and diff BREAKING-var-ref.
//
// Recognized sources (checked in order, accumulating):
//  1. Built-in defaults per node type (DefaultTypeOutputs).
//  2. Explicit data.outputs and data.output_variables declarations.
//  3. start: data.variables[].{variable,name}.
//  4. parameter-extractor: data.parameters[].name.
//  5. variable-assigner: data.items[].variable_selector[-1] (the assigned name).
func GatherOutputs(n *model.Node) map[string]bool {
	out := map[string]bool{}
	if n == nil {
		return out
	}
	for k, ok := range DefaultTypeOutputs(n.Type) {
		if ok {
			out[k] = true
		}
	}
	if n.Data == nil {
		return out
	}
	// Explicit declarations.
	for _, key := range []string{"outputs", "output_variables"} {
		for k := range ExtractOutputs(n.Data[key]) {
			out[k] = true
		}
	}
	// start.variables
	if n.Type == "start" {
		if vars, ok := n.Data["variables"].([]any); ok {
			for _, v := range vars {
				if m := AsMap(v); m != nil {
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
	// parameter-extractor.parameters
	if n.Type == "parameter-extractor" || n.Type == "parameter_extractor" {
		if params, ok := n.Data["parameters"].([]any); ok {
			for _, p := range params {
				if m := AsMap(p); m != nil {
					if s, ok := m["name"].(string); ok {
						out[s] = true
					}
				}
			}
		}
	}
	// variable-assigner.items[*].variable_selector — the tail element is the
	// newly-assigned variable name visible downstream as {{#<va>.<name>#}}.
	if n.Type == "variable-assigner" || n.Type == "variable_assigner" {
		if items, ok := n.Data["items"].([]any); ok {
			for _, it := range items {
				if m := AsMap(it); m != nil {
					if sel, ok := m["variable_selector"].([]any); ok && len(sel) >= 1 {
						if tail, ok := sel[len(sel)-1].(string); ok && tail != "" {
							out[tail] = true
						}
					}
				}
			}
		}
	}
	return out
}

// NodeDeclaresOutput reports whether node n exposes an output variable named name.
func NodeDeclaresOutput(n *model.Node, name string) bool {
	if n == nil || name == "" {
		return false
	}
	return GatherOutputs(n)[name]
}
