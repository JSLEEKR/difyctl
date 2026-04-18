package lint

import "testing"

func TestRuleGraphCycle_Simple(t *testing.T) {
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
      - {source: a, target: b}
      - {source: b, target: a}
      - {source: b, target: e}
`)
	fs := ruleGraphCycle{}.Check(wf)
	if len(fs) == 0 {
		t.Fatal("expected cycle detection")
	}
}

func TestRuleGraphCycle_DAG(t *testing.T) {
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
	fs := ruleGraphCycle{}.Check(wf)
	if len(fs) != 0 {
		t.Fatalf("no cycle expected, got %v", fs)
	}
}

func TestRuleGraphCycle_SelfLoop(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: a, type: start, data: {title: A}}
      - {id: b, type: llm, data: {title: B, model: {name: m, provider: o}}}
    edges:
      - {source: a, target: b}
      - {source: b, target: b}
`)
	fs := ruleGraphCycle{}.Check(wf)
	if len(fs) == 0 {
		t.Fatal("self-loop should be detected")
	}
}

func TestRuleGraphCycle_IterationBodyAllowed(t *testing.T) {
	// Back-edge inside an iteration body should NOT trigger.
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S}}
      - {id: it, type: iteration, data: {title: It}}
      - {id: its, type: iteration-start, data: {title: ITS, parent_id: it}}
      - {id: body, type: template-transform, data: {title: B, parent_id: it}}
      - {id: e, type: end, data: {title: E}}
    edges:
      - {source: s, target: it}
      - {source: its, target: body}
      - {source: body, target: its}
      - {source: it, target: e}
`)
	fs := ruleGraphCycle{}.Check(wf)
	if len(fs) != 0 {
		t.Fatalf("iteration internal cycle should be allowed, got %v", fs)
	}
}

func TestRuleGraphCycle_LargeDAGNoStackOverflow(t *testing.T) {
	// Pathological chain; iterative DFS must handle it.
	yml := `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
`
	const N = 500
	for i := 0; i < N; i++ {
		id := "n"
		for c := i; c > 0; c /= 10 {
			id += string(rune('0' + c%10))
		}
		if i == 0 {
			id = "n0"
		}
		yml += "      - {id: " + id + ", type: start, data: {title: X}}\n"
	}
	yml += "    edges:\n"
	for i := 0; i < N-1; i++ {
		src := "n"
		for c := i; c > 0; c /= 10 {
			src += string(rune('0' + c%10))
		}
		if i == 0 {
			src = "n0"
		}
		dst := "n"
		j := i + 1
		for c := j; c > 0; c /= 10 {
			dst += string(rune('0' + c%10))
		}
		if j == 0 {
			dst = "n0"
		}
		yml += "      - {source: " + src + ", target: " + dst + "}\n"
	}
	wf := loadFixture(t, yml)
	// Should complete without panic or stack overflow.
	_ = ruleGraphCycle{}.Check(wf)
}
