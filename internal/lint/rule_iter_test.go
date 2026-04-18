package lint

import "testing"

func TestRuleIterationMissingStart_NoBody(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: it, type: iteration, data: {title: IT}}
    edges: []
`)
	fs := ruleIterationMissingStart{}.Check(wf)
	if len(fs) != 1 || fs[0].Rule != "DIFY019" {
		t.Fatalf("want 1 DIFY019, got %v", fs)
	}
}

func TestRuleIterationMissingStart_Multiple(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: it, type: iteration, data: {title: IT}}
      - {id: its1, type: iteration-start, data: {title: S, parent_id: it}}
      - {id: its2, type: iteration-start, data: {title: S2, parent_id: it}}
    edges: []
`)
	fs := ruleIterationMissingStart{}.Check(wf)
	if len(fs) != 1 {
		t.Fatalf("want 1, got %v", fs)
	}
}

func TestRuleIterationMissingStart_Ok(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: it, type: iteration, data: {title: IT}}
      - {id: its, type: iteration-start, data: {title: S, parent_id: it}}
    edges: []
`)
	if fs := (ruleIterationMissingStart{}).Check(wf); len(fs) != 0 {
		t.Fatalf("want 0, got %v", fs)
	}
}

func TestRuleIterationMissingStart_NoIteration(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S}}
    edges: []
`)
	if fs := (ruleIterationMissingStart{}).Check(wf); len(fs) != 0 {
		t.Fatalf("want 0, got %v", fs)
	}
}
