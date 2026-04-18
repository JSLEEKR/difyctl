package lint

import "testing"

func TestRuleMissingStart(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: e, type: end, data: {title: E}}
    edges: []
`)
	fs := ruleMissingStart{}.Check(wf)
	if len(fs) != 1 {
		t.Fatalf("want DIFY015, got %v", fs)
	}
}

func TestRuleMissingEnd(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S}}
    edges: []
`)
	fs := ruleMissingEnd{}.Check(wf)
	if len(fs) != 1 {
		t.Fatalf("want DIFY016, got %v", fs)
	}
}

func TestRuleMissingStart_NotTriggered(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S}}
      - {id: e, type: end, data: {title: E}}
    edges: []
`)
	if fs := (ruleMissingStart{}).Check(wf); len(fs) != 0 {
		t.Fatalf("start present, no finding expected: %v", fs)
	}
	if fs := (ruleMissingEnd{}).Check(wf); len(fs) != 0 {
		t.Fatalf("end present, no finding expected: %v", fs)
	}
}

func TestRuleMissingEnd_AnswerSatisfies(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: chatflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S}}
      - {id: a, type: answer, data: {title: A, answer: ok}}
    edges: []
`)
	if fs := (ruleMissingEnd{}).Check(wf); len(fs) != 0 {
		t.Fatalf("answer should count as end, got %v", fs)
	}
}
