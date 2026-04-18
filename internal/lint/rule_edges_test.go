package lint

import "testing"

func TestRuleEdgeDanglingSource(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: a, type: start, data: {title: A}}
      - {id: b, type: end, data: {title: B}}
    edges:
      - {source: zzz, target: b}
      - {source: "", target: b}
`)
	fs := ruleEdgeDanglingSource{}.Check(wf)
	if len(fs) != 2 {
		t.Fatalf("want 2 findings, got %v", fs)
	}
	for _, f := range fs {
		if f.Rule != "DIFY009" {
			t.Fatalf("rule should be DIFY009, got %v", f)
		}
	}
}

func TestRuleEdgeDanglingTarget(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: a, type: start, data: {title: A}}
      - {id: b, type: end, data: {title: B}}
    edges:
      - {source: a, target: zzz}
      - {source: a, target: ""}
`)
	fs := ruleEdgeDanglingTarget{}.Check(wf)
	if len(fs) != 2 {
		t.Fatalf("want 2 findings, got %v", fs)
	}
}

func TestRuleEdgeDanglingSource_NoFalsePositive(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: a, type: start, data: {title: A}}
      - {id: b, type: end, data: {title: B}}
    edges:
      - {source: a, target: b}
`)
	if fs := (ruleEdgeDanglingSource{}).Check(wf); len(fs) != 0 {
		t.Fatalf("want 0, got %v", fs)
	}
	if fs := (ruleEdgeDanglingTarget{}).Check(wf); len(fs) != 0 {
		t.Fatalf("want 0, got %v", fs)
	}
}

func TestRuleDuplicateEdge(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: a, type: start, data: {title: A}}
      - {id: b, type: end, data: {title: B}}
    edges:
      - {source: a, target: b}
      - {source: a, target: b}
`)
	fs := ruleDuplicateEdge{}.Check(wf)
	if len(fs) != 1 || fs[0].Rule != "DIFY011" {
		t.Fatalf("want 1 DIFY011, got %v", fs)
	}
}

func TestRuleDuplicateEdge_HandlesMatter(t *testing.T) {
	// Same source/target but different handles should NOT be duplicates.
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: a, type: start, data: {title: A}}
      - {id: b, type: end, data: {title: B}}
    edges:
      - {source: a, target: b, sourceHandle: left}
      - {source: a, target: b, sourceHandle: right}
`)
	fs := ruleDuplicateEdge{}.Check(wf)
	if len(fs) != 0 {
		t.Fatalf("want 0, got %v", fs)
	}
}
