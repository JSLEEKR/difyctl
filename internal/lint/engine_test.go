package lint

import (
	"testing"

	"github.com/JSLEEKR/difyctl/internal/model"
	"github.com/JSLEEKR/difyctl/internal/parse"
)

// loadFixture is a test helper that parses a string and returns the Workflow.
func loadFixture(t *testing.T, s string) *model.Workflow {
	t.Helper()
	wf, err := parse.ParseBytes([]byte(s))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return wf
}

const tinyGood = `
app: {name: A, mode: workflow, description: ""}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: s
        type: start
        data: {title: S}
      - id: e
        type: end
        data: {title: E}
    edges:
      - {id: x, source: s, target: e}
`

func TestRun_Deterministic(t *testing.T) {
	wf := loadFixture(t, tinyGood)
	a := Run(DefaultRules(), wf)
	b := Run(DefaultRules(), wf)
	if len(a) != len(b) {
		t.Fatalf("len mismatch: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("finding %d differs: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestHasErrors(t *testing.T) {
	fs := []Finding{
		{Rule: "DIFY012", Severity: SeverityWarning},
	}
	if HasErrors(fs) {
		t.Fatal("warning-only should not be error")
	}
	fs = append(fs, Finding{Rule: "DIFY001", Severity: SeverityError})
	if !HasErrors(fs) {
		t.Fatal("expected error detected")
	}
}

func TestCountBySeverity(t *testing.T) {
	fs := []Finding{
		{Severity: SeverityError},
		{Severity: SeverityError},
		{Severity: SeverityWarning},
	}
	got := CountBySeverity(fs)
	if got[SeverityError] != 2 || got[SeverityWarning] != 1 {
		t.Fatalf("got %v", got)
	}
}

func TestFindingFormat(t *testing.T) {
	f := Finding{Rule: "DIFY006", Severity: SeverityError, Message: "duplicate id 'llm-1'", Path: "a.yml", Line: 42}
	got := f.Format()
	want := "a.yml:42: [DIFY006/error] duplicate id 'llm-1'"
	if got != want {
		t.Fatalf("want %q got %q", want, got)
	}
	f2 := Finding{Rule: "DIFY014", Severity: SeverityError, Message: "cycle"}
	if got := f2.Format(); got != "[DIFY014/error] cycle" {
		t.Fatalf("got %q", got)
	}
}

func TestRuleIDs_CatalogComplete(t *testing.T) {
	ids := RuleIDs()
	if len(ids) < 15 {
		t.Fatalf("expected >= 15 rules, got %d", len(ids))
	}
	// IDs must be unique.
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate rule id: %s", id)
		}
		seen[id] = true
	}
}
