package lint

import "testing"

func TestRuleLLMMissingModel_Missing(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S}}
      - {id: llm-1, type: llm, data: {title: X}}
    edges: []
`)
	fs := ruleLLMMissingModel{}.Check(wf)
	if len(fs) != 1 || fs[0].Rule != "DIFY017" {
		t.Fatalf("want DIFY017, got %v", fs)
	}
}

func TestRuleLLMMissingModel_Empty(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: llm-1, type: llm, data: {title: X, model: {}}}
    edges: []
`)
	fs := ruleLLMMissingModel{}.Check(wf)
	if len(fs) != 1 {
		t.Fatalf("want 1, got %v", fs)
	}
}

func TestRuleLLMMissingModel_OK(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: llm-1, type: llm, data: {title: X, model: {provider: openai, name: gpt-4}}}
    edges: []
`)
	if fs := (ruleLLMMissingModel{}).Check(wf); len(fs) != 0 {
		t.Fatalf("want 0, got %v", fs)
	}
}

func TestRuleCodeMissingCode_Missing(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: c, type: code, data: {title: X, code_language: python3}}
    edges: []
`)
	fs := ruleCodeMissingCode{}.Check(wf)
	if len(fs) != 1 {
		t.Fatalf("want 1, got %v", fs)
	}
}

func TestRuleCodeMissingCode_LangMissing(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: c, type: code, data: {title: X, code: "return 1"}}
    edges: []
`)
	fs := ruleCodeMissingCode{}.Check(wf)
	if len(fs) != 1 {
		t.Fatalf("want 1, got %v", fs)
	}
}

func TestRuleCodeMissingCode_OK(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: c, type: code, data: {title: X, code_language: python3, code: "def main(): return 1"}}
    edges: []
`)
	if fs := (ruleCodeMissingCode{}).Check(wf); len(fs) != 0 {
		t.Fatalf("want 0, got %v", fs)
	}
}
