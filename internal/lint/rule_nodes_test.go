package lint

import "testing"

func TestRuleMissingNodeID(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - type: start
        data: {title: S}
    edges: []
`)
	fs := ruleMissingNodeID{}.Check(wf)
	if len(fs) != 1 || fs[0].Rule != "DIFY005" {
		t.Fatalf("want 1 DIFY005, got %v", fs)
	}
}

func TestRuleDuplicateNodeID(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: a
        type: start
        data: {title: S}
      - id: a
        type: end
        data: {title: E}
    edges: []
`)
	fs := ruleDuplicateNodeID{}.Check(wf)
	if len(fs) != 1 || fs[0].Rule != "DIFY006" {
		t.Fatalf("want 1 DIFY006, got %v", fs)
	}
}

func TestRuleUnknownNodeType(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: s
        type: start
        data: {title: S}
      - id: x
        type: mystery
        data: {title: X}
    edges: []
`)
	fs := ruleUnknownNodeType{}.Check(wf)
	if len(fs) != 1 || fs[0].Rule != "DIFY007" {
		t.Fatalf("want 1 DIFY007, got %v", fs)
	}
}

func TestRuleUnknownNodeType_EmptyType(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: s
        type: ""
        data: {title: S}
    edges: []
`)
	fs := ruleUnknownNodeType{}.Check(wf)
	if len(fs) != 1 {
		t.Fatalf("want 1 finding, got %v", fs)
	}
}

func TestRuleMissingNodeData(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: s
        type: start
      - id: e
        type: end
        data: {title: E}
    edges: []
`)
	fs := ruleMissingNodeData{}.Check(wf)
	if len(fs) != 1 || fs[0].Rule != "DIFY008" {
		t.Fatalf("want 1 DIFY008, got %v", fs)
	}
}

func TestKnownNodeTypes_Comprehensive(t *testing.T) {
	// ensure the set includes both forms for hyphen/underscore variants we care about.
	cases := []string{"llm", "code", "http-request", "http_request", "if-else", "if_else", "iteration",
		"iteration-start", "iteration_start", "tool", "start", "end", "answer",
		"knowledge-retrieval", "parameter-extractor", "question-classifier",
		"template-transform", "variable-aggregator", "variable-assigner",
	}
	for _, c := range cases {
		if !IsKnownNodeType(c) {
			t.Errorf("expected %q to be known", c)
		}
	}
	if IsKnownNodeType("bogus-type") {
		t.Error("bogus-type should not be known")
	}
}
