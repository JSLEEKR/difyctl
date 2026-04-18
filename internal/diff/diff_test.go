package diff

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JSLEEKR/difyctl/internal/parse"
)

const baseYAML = `app: {name: A, mode: workflow, description: ""}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S, variables: [{variable: q, type: string}]}}
      - {id: llm-1, type: llm, data: {title: G, model: {provider: o, name: gpt-4}, prompt_template: [{role: user, text: "{{#s.q#}}"}]}}
      - {id: end-1, type: end, data: {title: E}}
    edges:
      - {source: s, target: llm-1}
      - {source: llm-1, target: end-1}
`

func parseS(t *testing.T, s string) (a any) {
	t.Helper()
	wf, err := parse.ParseBytes([]byte(s))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return wf
}

func TestCompute_NoChanges(t *testing.T) {
	a, _ := parse.ParseBytes([]byte(baseYAML))
	b, _ := parse.ParseBytes([]byte(baseYAML))
	if changes := Compute(a, b); len(changes) != 0 {
		t.Fatalf("want 0 changes, got %v", changes)
	}
}

func TestCompute_NodeAdded(t *testing.T) {
	a, _ := parse.ParseBytes([]byte(baseYAML))
	modified := strings.Replace(baseYAML,
		"      - {id: end-1, type: end, data: {title: E}}",
		"      - {id: extra, type: llm, data: {title: X, model: {provider: o, name: m}}}\n      - {id: end-1, type: end, data: {title: E}}",
		1)
	b, _ := parse.ParseBytes([]byte(modified))
	changes := Compute(a, b)
	found := false
	for _, c := range changes {
		if c.Category == CategoryAdded && c.Kind == "node" && c.ID == "extra" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ADDED node extra; got %v", changes)
	}
}

func TestCompute_NodeRemoved(t *testing.T) {
	a, _ := parse.ParseBytes([]byte(baseYAML))
	modified := strings.Replace(baseYAML,
		"      - {id: llm-1, type: llm, data: {title: G, model: {provider: o, name: gpt-4}, prompt_template: [{role: user, text: \"{{#s.q#}}\"}]}}\n",
		"",
		1)
	// Also remove edges that reference llm-1.
	modified = strings.Replace(modified, "      - {source: s, target: llm-1}\n", "", 1)
	modified = strings.Replace(modified, "      - {source: llm-1, target: end-1}\n", "      - {source: s, target: end-1}\n", 1)
	b, _ := parse.ParseBytes([]byte(modified))
	changes := Compute(a, b)
	found := false
	for _, c := range changes {
		if c.Category == CategoryRemoved && c.Kind == "node" && c.ID == "llm-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected REMOVED node llm-1; got %v", changes)
	}
}

func TestCompute_BreakingVarRef(t *testing.T) {
	a, _ := parse.ParseBytes([]byte(baseYAML))
	// rename Start node id from 's' to 's2' — llm-1 still references {{#s.q#}}
	modified := strings.Replace(baseYAML,
		"      - {id: s, type: start, data: {title: S, variables: [{variable: q, type: string}]}}",
		"      - {id: s2, type: start, data: {title: S, variables: [{variable: q, type: string}]}}",
		1)
	modified = strings.Replace(modified, "      - {source: s, target: llm-1}", "      - {source: s2, target: llm-1}", 1)
	b, _ := parse.ParseBytes([]byte(modified))
	changes := Compute(a, b)
	found := false
	for _, c := range changes {
		if c.Category == CategoryBreaking && c.Kind == "variable-ref" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected BREAKING variable-ref; got %v", changes)
	}
}

func TestCompute_BreakingOutputRemoved(t *testing.T) {
	// A references {{#s.q#}}. B removes 'q' from start.
	a, _ := parse.ParseBytes([]byte(baseYAML))
	modified := strings.Replace(baseYAML,
		"variables: [{variable: q, type: string}]",
		"variables: [{variable: other, type: string}]",
		1)
	b, _ := parse.ParseBytes([]byte(modified))
	changes := Compute(a, b)
	found := false
	for _, c := range changes {
		if c.Category == CategoryBreaking && c.Kind == "variable-ref" && strings.Contains(c.Detail, "output") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected BREAKING for output removed; got %v", changes)
	}
}

func TestCompute_EdgeAddedAndRemoved(t *testing.T) {
	a, _ := parse.ParseBytes([]byte(baseYAML))
	modified := strings.Replace(baseYAML,
		"      - {source: s, target: llm-1}\n",
		"      - {source: s, target: end-1}\n",
		1)
	b, _ := parse.ParseBytes([]byte(modified))
	changes := Compute(a, b)
	added, removed := 0, 0
	for _, c := range changes {
		if c.Kind != "edge" {
			continue
		}
		if c.Category == CategoryAdded {
			added++
		}
		if c.Category == CategoryRemoved {
			removed++
		}
	}
	if added != 1 || removed != 1 {
		t.Fatalf("want 1 added and 1 removed edges, got added=%d removed=%d", added, removed)
	}
}

func TestCompute_MovedOnly(t *testing.T) {
	a, _ := parse.ParseBytes([]byte(baseYAML))
	// Add position to llm-1 only in b.
	modified := strings.Replace(baseYAML,
		"      - {id: llm-1, type: llm, data: {title: G, model: {provider: o, name: gpt-4}, prompt_template: [{role: user, text: \"{{#s.q#}}\"}]}}",
		"      - {id: llm-1, type: llm, data: {title: G, model: {provider: o, name: gpt-4}, prompt_template: [{role: user, text: \"{{#s.q#}}\"}]}, position: {x: 10, y: 20}}",
		1)
	b, _ := parse.ParseBytes([]byte(modified))
	changes := Compute(a, b)
	foundMoved := false
	for _, c := range changes {
		if c.Kind == "node" && c.ID == "llm-1" && c.Detail == "moved" {
			foundMoved = true
		}
	}
	if !foundMoved {
		t.Fatalf("expected CHANGED moved; got %v", changes)
	}
}

func TestCompute_BodyChanged(t *testing.T) {
	a, _ := parse.ParseBytes([]byte(baseYAML))
	modified := strings.Replace(baseYAML, "gpt-4", "gpt-4o", 1)
	b, _ := parse.ParseBytes([]byte(modified))
	changes := Compute(a, b)
	found := false
	for _, c := range changes {
		if c.Kind == "node" && c.ID == "llm-1" && c.Detail == "body-changed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected body-changed; got %v", changes)
	}
}

// TestCompute_PreExistingBrokenRefNotBreaking verifies that a reference that
// was ALREADY broken in a (pre-existing lint DIFY013) is not surfaced again
// as BREAKING in diff a->b. BREAKING should only reflect changes this diff
// introduced — pre-existing bugs are noise at this layer.
func TestCompute_PreExistingBrokenRefNotBreaking(t *testing.T) {
	// Both a and b reference {{#ghost.x#}} which does not exist in either.
	src := `app: {name: A, mode: workflow, description: ""}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S, variables: [{variable: q, type: string}]}}
      - {id: l, type: llm, data: {title: L, model: {provider: o, name: g}, prompt_template: [{role: user, text: "hi {{#ghost.x#}}"}]}}
      - {id: e, type: end, data: {title: E}}
    edges:
      - {source: s, target: l}
      - {source: l, target: e}
`
	a, _ := parse.ParseBytes([]byte(src))
	b, _ := parse.ParseBytes([]byte(src))
	for _, c := range Compute(a, b) {
		if c.Category == CategoryBreaking {
			t.Fatalf("pre-existing broken ref should not be BREAKING: %+v", c)
		}
	}
}

// TestCompute_IdenticalQuestionClassifierNoBreaking guards the Cycle B drift
// fix at the diff layer: see internal/varref for the root cause.
func TestCompute_IdenticalQuestionClassifierNoBreaking(t *testing.T) {
	src := `app: {name: A, mode: workflow, description: ""}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S, variables: [{variable: q, type: string}]}}
      - {id: qc, type: question-classifier, data: {title: Q}}
      - {id: l, type: llm, data: {title: L, model: {provider: o, name: g}, prompt_template: [{role: user, text: "{{#qc.class_name#}}"}]}}
      - {id: e, type: end, data: {title: E}}
    edges:
      - {source: s, target: qc}
      - {source: qc, target: l}
      - {source: l, target: e}
`
	a, _ := parse.ParseBytes([]byte(src))
	b, _ := parse.ParseBytes([]byte(src))
	for _, c := range Compute(a, b) {
		if c.Category == CategoryBreaking {
			t.Fatalf("identical workflow with QC must not be BREAKING: %+v", c)
		}
	}
}

func TestCompute_AppMetadata(t *testing.T) {
	a, _ := parse.ParseBytes([]byte(baseYAML))
	modified := strings.Replace(baseYAML, "name: A", "name: Renamed", 1)
	modified = strings.Replace(modified, "version: \"0.1\"", "version: \"0.2\"", 1)
	b, _ := parse.ParseBytes([]byte(modified))
	changes := Compute(a, b)
	kinds := map[string]bool{}
	for _, c := range changes {
		if c.Kind == "app" {
			kinds[c.ID] = true
		}
	}
	if !kinds["name"] || !kinds["version"] {
		t.Fatalf("want app name + version changes, got %v", changes)
	}
}

func TestCompute_Sorted(t *testing.T) {
	a, _ := parse.ParseBytes([]byte(baseYAML))
	// Make multiple changes.
	modified := strings.Replace(baseYAML, "gpt-4", "gpt-4o", 1)
	modified = strings.Replace(modified,
		"      - {id: end-1, type: end, data: {title: E}}",
		"      - {id: zzz, type: llm, data: {title: Z, model: {provider: o, name: m}}}\n      - {id: end-1, type: end, data: {title: E}}",
		1)
	b, _ := parse.ParseBytes([]byte(modified))
	changes := Compute(a, b)
	for i := 1; i < len(changes); i++ {
		if categoryOrder(changes[i-1].Category) > categoryOrder(changes[i].Category) {
			t.Fatalf("not sorted by category: %+v", changes)
		}
	}
}

func TestRenderText_Categorizes(t *testing.T) {
	changes := []Change{
		{Category: CategoryAdded, Kind: "node", ID: "x"},
		{Category: CategoryBreaking, Kind: "variable-ref", ID: "y", Detail: "removed"},
	}
	var buf bytes.Buffer
	RenderText(&buf, changes)
	s := buf.String()
	if !strings.Contains(s, "[BREAKING]") || !strings.Contains(s, "[ADDED]") {
		t.Fatalf("missing categories: %s", s)
	}
	// BREAKING must appear before ADDED.
	if strings.Index(s, "[BREAKING]") > strings.Index(s, "[ADDED]") {
		t.Fatalf("wrong order: %s", s)
	}
}

func TestRenderText_Empty(t *testing.T) {
	var buf bytes.Buffer
	RenderText(&buf, nil)
	if !strings.Contains(buf.String(), "no semantic changes") {
		t.Fatalf("got %q", buf.String())
	}
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderJSON(&buf, []Change{{Category: "ADDED", Kind: "node", ID: "x"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"ADDED"`) {
		t.Fatalf("got %q", buf.String())
	}
}

func TestRenderJSON_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderJSON(&buf, nil); err != nil {
		t.Fatal(err)
	}
	// Should be "[]" not "null".
	s := strings.TrimSpace(buf.String())
	if s != "[]" {
		t.Fatalf("want '[]', got %q", s)
	}
}

func TestHasBreaking(t *testing.T) {
	cs := []Change{{Category: CategoryAdded}}
	if HasBreaking(cs) {
		t.Fatal("no breaking in set")
	}
	cs = append(cs, Change{Category: CategoryBreaking})
	if !HasBreaking(cs) {
		t.Fatal("breaking present")
	}
}

func TestSummary(t *testing.T) {
	cs := []Change{
		{Category: CategoryAdded}, {Category: CategoryAdded}, {Category: CategoryBreaking},
	}
	s := Summary(cs)
	if !strings.Contains(s, "2 added") || !strings.Contains(s, "1 breaking") {
		t.Fatalf("got %q", s)
	}
	if Summary(nil) != "no changes" {
		t.Fatal("empty summary wrong")
	}
}
