package lint

import "testing"

func TestRuleOrphanNode(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: a, type: start, data: {title: A}}
      - {id: b, type: end, data: {title: B}}
      - {id: floater, type: llm, data: {title: Lonely, model: {provider: o, name: g}}}
    edges:
      - {source: a, target: b}
`)
	fs := ruleOrphanNode{}.Check(wf)
	if len(fs) != 1 || fs[0].Rule != "DIFY012" {
		t.Fatalf("want 1 DIFY012, got %v", fs)
	}
	if fs[0].Severity != SeverityWarning {
		t.Fatalf("want warning, got %s", fs[0].Severity)
	}
}

func TestRuleOrphanNode_StartNodesAllowed(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S}}
    edges: []
`)
	fs := ruleOrphanNode{}.Check(wf)
	if len(fs) != 0 {
		t.Fatalf("start nodes should not trigger orphan; got %v", fs)
	}
}

func TestRuleOrphanNode_NoFalseIfConnected(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S}}
      - {id: m, type: llm, data: {title: M, model: {name: g, provider: o}}}
      - {id: e, type: end, data: {title: E}}
    edges:
      - {source: s, target: m}
      - {source: m, target: e}
`)
	if fs := (ruleOrphanNode{}).Check(wf); len(fs) != 0 {
		t.Fatalf("want 0, got %v", fs)
	}
}
