package lint

import "testing"

func TestRuleUnreachableFromStart(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S}}
      - {id: a, type: llm, data: {title: A, model: {name: m, provider: o}}}
      - {id: b, type: llm, data: {title: B, model: {name: m, provider: o}}}
      - {id: e, type: end, data: {title: E}}
    edges:
      - {source: s, target: a}
      - {source: a, target: e}
      - {source: b, target: e}
`)
	fs := ruleUnreachableFromStart{}.Check(wf)
	if len(fs) != 1 || fs[0].Rule != "DIFY020" {
		t.Fatalf("want 1 DIFY020 (b unreachable), got %v", fs)
	}
	if fs[0].Severity != SeverityWarning {
		t.Fatalf("want warning, got %s", fs[0].Severity)
	}
}

func TestRuleUnreachableFromStart_AllReachable(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S}}
      - {id: a, type: llm, data: {title: A, model: {name: m, provider: o}}}
      - {id: e, type: end, data: {title: E}}
    edges:
      - {source: s, target: a}
      - {source: a, target: e}
`)
	if fs := (ruleUnreachableFromStart{}).Check(wf); len(fs) != 0 {
		t.Fatalf("want 0, got %v", fs)
	}
}

func TestRuleUnreachableFromStart_IgnoresIterationBody(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S}}
      - {id: it, type: iteration, data: {title: IT}}
      - {id: its, type: iteration-start, data: {title: ITS, parent_id: it}}
      - {id: body, type: template-transform, data: {title: B, parent_id: it}}
      - {id: e, type: end, data: {title: E}}
    edges:
      - {source: s, target: it}
      - {source: its, target: body}
      - {source: it, target: e}
`)
	// iteration body nodes are not directly edge-reachable from start in our adjacency,
	// but they should be silenced because they are inside an iteration.
	if fs := (ruleUnreachableFromStart{}).Check(wf); len(fs) != 0 {
		t.Fatalf("iteration body should be silenced, got %v", fs)
	}
}
