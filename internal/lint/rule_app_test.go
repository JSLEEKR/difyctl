package lint

import "testing"

func TestRuleMissingApp(t *testing.T) {
	wf := loadFixture(t, `kind: app
version: "0.1"
workflow:
  graph:
    nodes: []
    edges: []
`)
	fs := ruleMissingApp{}.Check(wf)
	if len(fs) != 1 || fs[0].Rule != "DIFY001" {
		t.Fatalf("want DIFY001, got %v", fs)
	}
}

func TestRuleMissingApp_Present(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes: []
    edges: []
`)
	if fs := (ruleMissingApp{}).Check(wf); len(fs) != 0 {
		t.Fatalf("expected no findings, got %v", fs)
	}
}

func TestRuleUnknownAppMode(t *testing.T) {
	cases := []struct {
		name string
		mode string
		want int
	}{
		{"workflow", "workflow", 0},
		{"chatflow", "chatflow", 0},
		{"agent-chat", "agent-chat", 0},
		{"bogus", "bogus", 1},
		{"empty", "", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			yml := `app: {name: A, mode: "` + tc.mode + `", description: x}
kind: app
version: "0.1"
workflow: {graph: {nodes: [], edges: []}}
`
			wf := loadFixture(t, yml)
			fs := ruleUnknownAppMode{}.Check(wf)
			if len(fs) != tc.want {
				t.Fatalf("want %d, got %v", tc.want, fs)
			}
		})
	}
}

func TestRuleKindMismatch(t *testing.T) {
	cases := []struct {
		name    string
		kind    string
		want    int
		wantSub string
	}{
		{"ok", "app", 0, ""},
		{"missing", "", 1, "missing"},
		{"wrong", "workflow", 1, "expected 'app'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			yml := "app: {name: A, mode: workflow}\nkind: " + tc.kind + "\nversion: \"0.1\"\nworkflow: {graph: {nodes: [], edges: []}}\n"
			wf := loadFixture(t, yml)
			fs := ruleKindMismatch{}.Check(wf)
			if len(fs) != tc.want {
				t.Fatalf("want %d findings, got %v", tc.want, fs)
			}
		})
	}
}

func TestRuleMissingVersion(t *testing.T) {
	wf := loadFixture(t, `app: {name: A, mode: workflow}
kind: app
workflow: {graph: {nodes: [], edges: []}}
`)
	fs := ruleMissingVersion{}.Check(wf)
	if len(fs) != 1 {
		t.Fatalf("want 1, got %v", fs)
	}
}
