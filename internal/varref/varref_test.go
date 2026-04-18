package varref

import (
	"testing"

	"github.com/JSLEEKR/difyctl/internal/model"
)

// TestQuestionClassifierClassName guards the Cycle B regression where lint
// said "{{#qc.class_name#}} is fine" but diff said "BREAKING: output removed"
// for the same input — because the two packages carried duplicated helpers
// and drifted. This package is now the single source of truth; if lint and
// diff ever re-diverge, it is a test-time failure here.
func TestQuestionClassifierClassName(t *testing.T) {
	n := &model.Node{ID: "qc", Type: "question-classifier"}
	if !NodeDeclaresOutput(n, "class_name") {
		t.Fatal("question-classifier must expose 'class_name'")
	}
	// Underscore variant.
	n2 := &model.Node{ID: "qc", Type: "question_classifier"}
	if !NodeDeclaresOutput(n2, "class_name") {
		t.Fatal("question_classifier (underscore) must expose 'class_name'")
	}
}

func TestVariableAssignerItems(t *testing.T) {
	n := &model.Node{
		ID:   "va",
		Type: "variable-assigner",
		Data: map[string]any{
			"items": []any{
				map[string]any{
					"variable_selector": []any{"va", "assigned"},
				},
			},
		},
	}
	if !NodeDeclaresOutput(n, "assigned") {
		t.Fatalf("variable-assigner must expose item tail, got outputs=%v", GatherOutputs(n))
	}
}

func TestStartVariables(t *testing.T) {
	n := &model.Node{
		ID:   "s",
		Type: "start",
		Data: map[string]any{
			"variables": []any{
				map[string]any{"variable": "q", "type": "string"},
				map[string]any{"name": "extra"},
			},
		},
	}
	if !NodeDeclaresOutput(n, "q") {
		t.Fatal("start.variables[].variable should be recognized")
	}
	if !NodeDeclaresOutput(n, "extra") {
		t.Fatal("start.variables[].name should be recognized")
	}
}

func TestLLMDefaults(t *testing.T) {
	n := &model.Node{ID: "l", Type: "llm"}
	for _, name := range []string{"text", "usage"} {
		if !NodeDeclaresOutput(n, name) {
			t.Fatalf("llm must expose %q by default", name)
		}
	}
	if NodeDeclaresOutput(n, "made_up") {
		t.Fatal("llm does not expose 'made_up'")
	}
}

func TestParameterExtractor(t *testing.T) {
	n := &model.Node{
		ID:   "p",
		Type: "parameter-extractor",
		Data: map[string]any{
			"parameters": []any{
				map[string]any{"name": "amount"},
			},
		},
	}
	if !NodeDeclaresOutput(n, "amount") {
		t.Fatal("parameter-extractor.parameters[].name must be recognized")
	}
}

func TestExplicitOutputs(t *testing.T) {
	// Outputs as list-of-strings.
	n1 := &model.Node{ID: "c", Type: "code", Data: map[string]any{
		"outputs": []any{"a", "b"},
	}}
	if !NodeDeclaresOutput(n1, "a") || !NodeDeclaresOutput(n1, "b") {
		t.Fatal("explicit list-of-strings outputs missed")
	}
	// Outputs as list-of-objects with 'name'.
	n2 := &model.Node{ID: "c", Type: "code", Data: map[string]any{
		"outputs": []any{
			map[string]any{"name": "x"},
			map[string]any{"variable": "y"},
		},
	}}
	if !NodeDeclaresOutput(n2, "x") || !NodeDeclaresOutput(n2, "y") {
		t.Fatal("list-of-objects outputs missed")
	}
	// Outputs as map.
	n3 := &model.Node{ID: "c", Type: "code", Data: map[string]any{
		"outputs": map[string]any{"r": "int"},
	}}
	if !NodeDeclaresOutput(n3, "r") {
		t.Fatal("map-shaped outputs missed")
	}
}

func TestOutputVariablesKey(t *testing.T) {
	n := &model.Node{ID: "c", Type: "code", Data: map[string]any{
		"output_variables": []any{"o1"},
	}}
	if !NodeDeclaresOutput(n, "o1") {
		t.Fatal("output_variables key must be read alongside outputs")
	}
}

func TestCollect(t *testing.T) {
	data := map[string]any{
		"text": "hi {{#a.b#}} {{#c.d#}}",
		"nested": []any{
			map[string]any{"k": "{{#e.f#}}"},
		},
	}
	refs := Collect(data)
	if len(refs) != 3 {
		t.Fatalf("want 3 refs, got %d: %v", len(refs), refs)
	}
}

func TestCollectNil(t *testing.T) {
	if refs := Collect(nil); len(refs) != 0 {
		t.Fatalf("want no refs on nil, got %v", refs)
	}
}

func TestAsMapStringKeys(t *testing.T) {
	in := map[string]any{"k": "v"}
	out := AsMap(in)
	if out["k"] != "v" {
		t.Fatalf("AsMap lost value: %v", out)
	}
}

func TestAsMapAnyKeys(t *testing.T) {
	in := map[any]any{"k": "v", 42: "dropped"}
	out := AsMap(in)
	if out["k"] != "v" {
		t.Fatalf("AsMap(map[any]any) lost string key: %v", out)
	}
	if _, ok := out["42"]; ok {
		t.Fatal("AsMap should drop non-string keys")
	}
}

func TestNodeDeclaresOutputEmptyName(t *testing.T) {
	n := &model.Node{ID: "l", Type: "llm"}
	if NodeDeclaresOutput(n, "") {
		t.Fatal("empty name must not resolve")
	}
}

func TestNodeDeclaresOutputNilNode(t *testing.T) {
	if NodeDeclaresOutput(nil, "x") {
		t.Fatal("nil node must not resolve")
	}
}

// TestLintDiffParity is the canary test: if ever a new node type is added
// with default outputs, both lint and diff pick it up automatically through
// this package. If someone re-introduces per-package duplication this test
// will still pass (by definition) — the guard is that lint/rule_varref.go
// and diff/diff.go import this package and no longer define their own
// GatherOutputs. A linter could be added, but the grep-friendly contract is:
//
//	// PARITY: outputs-resolution lives in internal/varref
//
// ...in both files.
func TestLintDiffParity(t *testing.T) {
	// Exhaustive type table. If a new type declares defaults, add it here.
	cases := []struct {
		typ     string
		outputs []string
	}{
		{"llm", []string{"text", "usage"}},
		{"knowledge-retrieval", []string{"result"}},
		{"knowledge_retrieval", []string{"result"}},
		{"http-request", []string{"body", "status_code", "headers"}},
		{"http_request", []string{"body", "status_code", "headers"}},
		{"template-transform", []string{"output"}},
		{"template_transform", []string{"output"}},
		{"iteration", []string{"output"}},
		{"iteration-start", []string{"item", "index"}},
		{"iteration_start", []string{"item", "index"}},
		{"variable-aggregator", []string{"output"}},
		{"variable_aggregator", []string{"output"}},
		{"tool", []string{"text", "files"}},
		{"question-classifier", []string{"class_name"}},
		{"question_classifier", []string{"class_name"}},
	}
	for _, c := range cases {
		n := &model.Node{ID: "x", Type: c.typ}
		for _, o := range c.outputs {
			if !NodeDeclaresOutput(n, o) {
				t.Errorf("%q must declare %q", c.typ, o)
			}
		}
	}
}
