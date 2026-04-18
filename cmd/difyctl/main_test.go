package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const good = `app: {name: A, mode: workflow, description: ""}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S, variables: [{variable: q, type: string}]}}
      - {id: l, type: llm, data: {title: G, model: {provider: o, name: gpt-4}, prompt_template: [{role: user, text: "{{#s.q#}}"}]}}
      - {id: e, type: end, data: {title: E}}
    edges:
      - {source: s, target: l}
      - {source: l, target: e}
`

const broken = `app: {name: A, mode: workflow, description: ""}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S}}
      - {id: s, type: end, data: {title: E}}
    edges: []
`

func TestRealMain_NoArgs(t *testing.T) {
	code, err := realMain(nil)
	if code != 2 || err == nil {
		t.Fatalf("want (2, err), got (%d, %v)", code, err)
	}
}

func TestRealMain_Unknown(t *testing.T) {
	code, err := realMain([]string{"bogus"})
	if code != 2 || err == nil {
		t.Fatalf("want (2, err), got (%d, %v)", code, err)
	}
}

func TestRealMain_Help(t *testing.T) {
	code, err := realMain([]string{"help"})
	if code != 0 || err != nil {
		t.Fatalf("want (0, nil), got (%d, %v)", code, err)
	}
}

func TestRunLint_Good(t *testing.T) {
	path := writeTemp(t, "w.yml", good)
	var stdout, stderr bytes.Buffer
	code, err := runLint([]string{path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if code != 0 {
		t.Fatalf("want 0, got %d; out=%s", code, stdout.String())
	}
}

func TestRunLint_Bad(t *testing.T) {
	path := writeTemp(t, "w.yml", broken)
	var stdout, stderr bytes.Buffer
	code, err := runLint([]string{path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if code != 1 {
		t.Fatalf("want 1, got %d; out=%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "DIFY") {
		t.Fatalf("expected DIFYXXX in output, got: %s", stdout.String())
	}
}

func TestRunLint_JSONFormat(t *testing.T) {
	path := writeTemp(t, "w.yml", broken)
	var stdout, stderr bytes.Buffer
	_, err := runLint([]string{"--format", "json", path}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"findings"`) {
		t.Fatalf("missing findings key: %s", stdout.String())
	}
}

func TestRunLint_BadFormat(t *testing.T) {
	path := writeTemp(t, "w.yml", good)
	var stdout, stderr bytes.Buffer
	code, err := runLint([]string{"--format", "xml", path}, &stdout, &stderr)
	if code != 2 || err == nil {
		t.Fatalf("want (2, err), got (%d, %v)", code, err)
	}
}

func TestRunLint_MissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := runLint([]string{"/nonexistent/x.yml"}, &stdout, &stderr)
	if code != 3 || err == nil {
		t.Fatalf("want (3, err), got (%d, %v)", code, err)
	}
}

func TestRunLint_ArgCount(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := runLint([]string{}, &stdout, &stderr)
	if code != 2 || err == nil {
		t.Fatalf("want (2, err), got (%d, %v)", code, err)
	}
}

func TestRunDiff_NoChanges(t *testing.T) {
	a := writeTemp(t, "a.yml", good)
	b := writeTemp(t, "b.yml", good)
	var stdout, stderr bytes.Buffer
	code, err := runDiff([]string{a, b}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("want (0, nil), got (%d, %v)", code, err)
	}
}

func TestRunDiff_FailOnBreaking(t *testing.T) {
	// Remove the Start variable 'q' from b so that {{#s.q#}} reference in b is BREAKING.
	bMod := strings.Replace(good,
		"variables: [{variable: q, type: string}]",
		"variables: [{variable: other, type: string}]",
		1)
	a := writeTemp(t, "a.yml", good)
	b := writeTemp(t, "b.yml", bMod)
	var stdout, stderr bytes.Buffer
	code, err := runDiff([]string{"--fail-on-breaking", a, b}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("want 1 with --fail-on-breaking, got %d\n%s", code, stdout.String())
	}
}

func TestRunDiff_JSON(t *testing.T) {
	a := writeTemp(t, "a.yml", good)
	b := writeTemp(t, "b.yml", good)
	var stdout, stderr bytes.Buffer
	_, err := runDiff([]string{"--format", "json", a, b}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout.String()), "[") {
		t.Fatalf("expected JSON array, got %s", stdout.String())
	}
}

func TestRunDiff_ArgCount(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := runDiff([]string{"only-one.yml"}, &stdout, &stderr)
	if code != 2 || err == nil {
		t.Fatalf("want (2, err), got (%d, %v)", code, err)
	}
}

func TestRunDiff_MissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := runDiff([]string{"/nonexistent/a.yml", "/nonexistent/b.yml"}, &stdout, &stderr)
	if code != 3 || err == nil {
		t.Fatalf("want (3, err), got (%d, %v)", code, err)
	}
}

func TestRunFmt_Stdout(t *testing.T) {
	p := writeTemp(t, "w.yml", good)
	var stdout, stderr bytes.Buffer
	code, err := runFmt([]string{p}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("want (0,nil), got (%d, %v)", code, err)
	}
	if !strings.Contains(stdout.String(), "app:") {
		t.Fatalf("expected app in output; got %s", stdout.String())
	}
}

func TestRunFmt_Write(t *testing.T) {
	p := writeTemp(t, "w.yml", good)
	origBytes, _ := os.ReadFile(p)
	var stdout, stderr bytes.Buffer
	if _, err := runFmt([]string{"-w", p}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// File may or may not be byte-identical; but second format should be idempotent.
	var stdout2, stderr2 bytes.Buffer
	p2 := writeTemp(t, "w2.yml", string(got))
	if _, err := runFmt([]string{"-w", p2}, &stdout2, &stderr2); err != nil {
		t.Fatal(err)
	}
	got2, _ := os.ReadFile(p2)
	if !bytes.Equal(got, got2) {
		t.Fatalf("fmt not idempotent:\norig:\n%s\nonce:\n%s\ntwice:\n%s", origBytes, got, got2)
	}
}

func TestRunFmt_ArgCount(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := runFmt([]string{}, &stdout, &stderr)
	if code != 2 || err == nil {
		t.Fatalf("want (2, err), got (%d, %v)", code, err)
	}
}

func TestRunFmt_MissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := runFmt([]string{"/nonexistent/x.yml"}, &stdout, &stderr)
	if code != 3 || err == nil {
		t.Fatalf("want (3, err), got (%d, %v)", code, err)
	}
}

func TestExitCodeFor(t *testing.T) {
	if exitCodeFor(nil) != 0 {
		t.Fatal("nil → 0")
	}
	if exitCodeFor(newExitErr(2, os.ErrInvalid)) != 2 {
		t.Fatal("exitErr preserved")
	}
}
