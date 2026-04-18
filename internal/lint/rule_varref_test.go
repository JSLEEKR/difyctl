package lint

import (
	"testing"

	"github.com/JSLEEKR/difyctl/internal/varref"
)

func TestVarRef_Good(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: start-1
        type: start
        data:
          title: Start
          variables:
            - variable: query
              type: string
      - id: llm-1
        type: llm
        data:
          title: Gen
          model: {provider: openai, name: gpt-4}
          prompt_template:
            - role: user
              text: "ask: {{#start-1.query#}}"
      - id: e
        type: end
        data: {title: E}
    edges:
      - {source: start-1, target: llm-1}
      - {source: llm-1, target: e}
`)
	fs := ruleUnresolvedVarRef{}.Check(wf)
	if len(fs) != 0 {
		t.Fatalf("no findings expected, got %v", fs)
	}
}

func TestVarRef_MissingNode(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: start-1
        type: start
        data: {title: S, variables: [{variable: q, type: string}]}
      - id: llm-1
        type: llm
        data:
          title: Gen
          model: {provider: openai, name: gpt-4}
          prompt_template:
            - role: user
              text: "ask: {{#ghost.x#}}"
    edges: []
`)
	fs := ruleUnresolvedVarRef{}.Check(wf)
	if len(fs) != 1 {
		t.Fatalf("want 1 DIFY013, got %v", fs)
	}
}

func TestVarRef_MissingOutput(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: start-1
        type: start
        data: {title: S, variables: [{variable: q, type: string}]}
      - id: llm-1
        type: llm
        data:
          title: G
          model: {provider: o, name: g}
          prompt_template:
            - role: user
              text: "x: {{#start-1.nonexistent#}}"
    edges: []
`)
	fs := ruleUnresolvedVarRef{}.Check(wf)
	if len(fs) != 1 {
		t.Fatalf("want 1 DIFY013, got %v", fs)
	}
}

func TestVarRef_LLMDefaultText(t *testing.T) {
	// Referencing {{#llm-1.text#}} should be fine because llm nodes emit text by default.
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S, variables: [{variable: q, type: string}]}}
      - {id: llm-1, type: llm, data: {title: G, model: {name: m, provider: o}}}
      - id: e
        type: end
        data:
          title: E
          outputs:
            - variable: ans
              value: "{{#llm-1.text#}}"
    edges: []
`)
	fs := ruleUnresolvedVarRef{}.Check(wf)
	if len(fs) != 0 {
		t.Fatalf("want 0, got %v", fs)
	}
}

func TestVarRef_CodeOutputsDeclared(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S, variables: [{variable: q, type: string}]}}
      - id: code-1
        type: code
        data:
          title: C
          code_language: python3
          code: "def main(): return {'n': 1}"
          outputs:
            - name: n
      - id: llm-1
        type: llm
        data:
          title: G
          model: {name: m, provider: o}
          prompt_template:
            - role: user
              text: "use {{#code-1.n#}}"
    edges: []
`)
	fs := ruleUnresolvedVarRef{}.Check(wf)
	if len(fs) != 0 {
		t.Fatalf("want 0, got %v", fs)
	}
}

func TestCollectVarRefs_NestedStrings(t *testing.T) {
	data := map[string]any{
		"a": "hello {{#n.x#}} there",
		"b": []any{"ok", "{{#m.y#}}", map[string]any{"deep": "{{#d.z#}}"}},
	}
	refs := varref.Collect(data)
	if len(refs) != 3 {
		t.Fatalf("want 3 refs, got %v", refs)
	}
	// all refs should be captured
	seen := map[string]bool{}
	for _, r := range refs {
		seen[r.NodeID+"."+r.VarName] = true
	}
	for _, want := range []string{"n.x", "m.y", "d.z"} {
		if !seen[want] {
			t.Errorf("missing %q", want)
		}
	}
}

// TestVarRef_DedupesRepeatedRefs is the Cycle D regression for DIFY013 spam:
// one node mentioning the same `{{#ghost.q#}}` N times used to emit N identical
// findings. We now dedup per (referrer, target, var) so a giant prompt template
// with 1000 copies of a typo produces exactly one finding, not 1000 lines of
// noise.
func TestVarRef_DedupesRepeatedRefs(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S, variables: [{variable: q}]}}
      - id: l
        type: llm
        data:
          title: G
          model: {name: m, provider: o}
          prompt_template:
            - role: user
              text: "{{#ghost.q#}} {{#ghost.q#}} {{#ghost.q#}} {{#ghost.q#}}"
            - role: system
              text: "also {{#ghost.q#}}"
    edges: []
`)
	fs := ruleUnresolvedVarRef{}.Check(wf)
	if len(fs) != 1 {
		t.Fatalf("want exactly 1 dedup'd finding, got %d: %v", len(fs), fs)
	}
}

func TestVarRefPattern_Boundary(t *testing.T) {
	// Missing closing should not match.
	if varref.Pattern.MatchString("{{#x.y}}") {
		t.Error("should not match without closing #")
	}
	// Leading # missing.
	if varref.Pattern.MatchString("{{x.y#}}") {
		t.Error("should not match without leading #")
	}
}
