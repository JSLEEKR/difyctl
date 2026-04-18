package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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

// TestRunLint_JSONSuccessEnvelopeHasErrorNull is the Cycle D regression that
// makes lint's JSON envelope match diff's: jq filters like `.error` should
// work uniformly on success (null) and failure (string) paths. Previously the
// success path omitted the key, breaking naive pipelines that check .error.
// Also locks that findings is always an array (never null), so
// `.findings[]` never explodes with "Cannot iterate over null".
func TestRunLint_JSONSuccessEnvelopeHasErrorNull(t *testing.T) {
	path := writeTemp(t, "w.yml", good)
	var stdout, stderr bytes.Buffer
	code, err := runLint([]string{"--format", "json", path}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("want (0,nil), got (%d,%v): %s", code, err, stdout.String())
	}
	var env map[string]any
	if jerr := json.Unmarshal(stdout.Bytes(), &env); jerr != nil {
		t.Fatalf("stdout not JSON: %v\n%s", jerr, stdout.String())
	}
	raw, ok := env["error"]
	if !ok {
		t.Fatalf("expected 'error' key in success envelope, got %v", env)
	}
	if raw != nil {
		t.Fatalf("expected error=null on success, got %v", raw)
	}
	rawF, ok := env["findings"]
	if !ok {
		t.Fatalf("expected 'findings' key, got %v", env)
	}
	if _, isSlice := rawF.([]any); !isSlice {
		t.Fatalf("findings must serialise as [] not null, got %T=%v", rawF, rawF)
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
	var env map[string]any
	if jerr := json.Unmarshal(stdout.Bytes(), &env); jerr != nil {
		t.Fatalf("stdout is not valid JSON object: %v\n%s", jerr, stdout.String())
	}
	if _, ok := env["changes"]; !ok {
		t.Fatalf("expected 'changes' key, got %v", env)
	}
	// Must include 'error' key set to null on success so jq unifies with error case.
	raw, ok := env["error"]
	if !ok {
		t.Fatalf("expected 'error' key, got %v", env)
	}
	if raw != nil {
		t.Fatalf("expected error=null on success, got %v", raw)
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

// TestRunLint_JSONErrorEnvelope ensures that lint --format=json still emits a
// valid JSON document on stdout when the file cannot be opened or parsed.
// Without this, a caller piping into jq silently breaks.
func TestRunLint_JSONErrorEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := runLint([]string{"--format", "json", "/nonexistent/file.yml"}, &stdout, &stderr)
	if code != 3 || err == nil {
		t.Fatalf("want (3, err), got (%d, %v)", code, err)
	}
	if stdout.Len() == 0 {
		t.Fatal("stdout should contain a JSON error envelope")
	}
	var parsed map[string]any
	if jerr := json.Unmarshal(stdout.Bytes(), &parsed); jerr != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", jerr, stdout.String())
	}
	if _, ok := parsed["error"]; !ok {
		t.Fatalf("expected 'error' key in envelope, got %v", parsed)
	}
}

// TestRunLint_JSONErrorEnvelope_MalformedYAML covers the parse-error branch.
func TestRunLint_JSONErrorEnvelope_MalformedYAML(t *testing.T) {
	path := writeTemp(t, "bad.yml", "not:\n  valid: : : yaml")
	var stdout, stderr bytes.Buffer
	code, err := runLint([]string{"--format", "json", path}, &stdout, &stderr)
	if code != 3 || err == nil {
		t.Fatalf("want (3, err), got (%d, %v)", code, err)
	}
	var parsed map[string]any
	if jerr := json.Unmarshal(stdout.Bytes(), &parsed); jerr != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", jerr, stdout.String())
	}
}

// TestRunDiff_JSONErrorEnvelope ensures diff --format=json emits valid JSON
// even when a file cannot be opened.
func TestRunDiff_JSONErrorEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := runDiff([]string{"--format", "json", "/nonexistent/a.yml", "/nonexistent/b.yml"}, &stdout, &stderr)
	if code != 3 || err == nil {
		t.Fatalf("want (3, err), got (%d, %v)", code, err)
	}
	var parsed map[string]any
	if jerr := json.Unmarshal(stdout.Bytes(), &parsed); jerr != nil {
		t.Fatalf("stdout not valid JSON on diff error: %v\n%s", jerr, stdout.String())
	}
	if _, ok := parsed["error"]; !ok {
		t.Fatalf("expected 'error' key, got %v", parsed)
	}
}

// TestRunDiff_QuestionClassifierSelfDiff is the end-to-end regression test
// for the Cycle B drift bug: before the fix, `diff x x` for a workflow that
// referenced {{#qc.class_name#}} reported a spurious BREAKING change even
// though the two files were byte-identical, because lint and diff carried
// duplicated output-declaration helpers that disagreed on question-classifier.
func TestRunDiff_QuestionClassifierSelfDiff(t *testing.T) {
	const src = `app: {name: A, mode: workflow, description: ""}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S, variables: [{variable: q, type: string}]}}
      - {id: qc, type: question-classifier, data: {title: Q}}
      - {id: l, type: llm, data: {title: L, model: {provider: o, name: g}, prompt_template: [{role: user, text: "{{#qc.class_name#}}"}]}}
      - {id: e, type: end, data: {title: E}}
    edges:
      - {source: s, target: qc}
      - {source: qc, target: l}
      - {source: l, target: e}
`
	p := writeTemp(t, "qc.yml", src)
	var stdout, stderr bytes.Buffer
	code, err := runDiff([]string{p, p}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("want (0,nil), got (%d, %v); stdout=%s", code, err, stdout.String())
	}
	if strings.Contains(stdout.String(), "BREAKING") {
		t.Fatalf("self-diff should not report BREAKING: %s", stdout.String())
	}

	// Same fixture must also pass lint clean — this asserts that the SINGLE
	// source of truth (internal/varref) agrees for both operations.
	var lintOut, lintErr bytes.Buffer
	lc, lerr := runLint([]string{p}, &lintOut, &lintErr)
	if lerr != nil || lc != 0 {
		t.Fatalf("lint on same fixture: want (0,nil) got (%d,%v): %s", lc, lerr, lintOut.String())
	}
}

// TestRunDiff_VariableAssignerSelfDiff mirrors the above but for variable-assigner
// nodes, whose outputs are declared via data.items[].variable_selector tails.
func TestRunDiff_VariableAssignerSelfDiff(t *testing.T) {
	const src = `app: {name: A, mode: workflow, description: ""}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: s, type: start, data: {title: S, variables: [{variable: q, type: string}]}}
      - {id: va, type: variable-assigner, data: {title: V, items: [{variable_selector: ["va", "assigned"]}]}}
      - {id: l, type: llm, data: {title: L, model: {provider: o, name: g}, prompt_template: [{role: user, text: "{{#va.assigned#}}"}]}}
      - {id: e, type: end, data: {title: E}}
    edges:
      - {source: s, target: va}
      - {source: va, target: l}
      - {source: l, target: e}
`
	p := writeTemp(t, "va.yml", src)
	var stdout, stderr bytes.Buffer
	code, err := runDiff([]string{p, p}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("want (0,nil), got (%d, %v); stdout=%s", code, err, stdout.String())
	}
	if strings.Contains(stdout.String(), "BREAKING") {
		t.Fatalf("self-diff should not report BREAKING: %s", stdout.String())
	}
}

// TestRunFmt_PreservesPermissions ensures fmt -w keeps the file's original mode
// bits. A previous implementation blindly used 0o644 on WriteFile which would
// demote a 0o755 script or promote a 0o600 private file.
func TestRunFmt_PreservesPermissions(t *testing.T) {
	p := writeTemp(t, "w.yml", good)
	// Set an unusual mode.
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if _, err := runFmt([]string{"-w", p}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("fmt -w clobbered permissions: want 0600, got %o", info.Mode().Perm())
	}
}

// TestRunFmt_SymlinkPreserved is the Cycle C regression for the fmt-on-symlink
// bug. Previously `fmt -w link.yml` would os.Rename over the symlink, leaving
// the user with a regular file where the symlink had been — while the original
// target was untouched. Now we follow the symlink and rewrite the target, so
// the symlink stays a symlink and the target's bytes get the canonical form.
func TestRunFmt_SymlinkPreserved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks on Windows require privileges; skip in CI")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.yml")
	link := filepath.Join(dir, "link.yml")
	if err := os.WriteFile(target, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	origTargetBytes, _ := os.ReadFile(target)

	var stdout, stderr bytes.Buffer
	if _, err := runFmt([]string{"-w", link}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}

	// 1. link.yml must still be a symlink.
	li, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if li.Mode()&os.ModeSymlink == 0 {
		t.Fatal("fmt -w clobbered the symlink into a regular file")
	}
	// 2. target file must have been rewritten (or remain byte-equal if already canonical).
	newTargetBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	_ = origTargetBytes
	if len(newTargetBytes) == 0 {
		t.Fatal("target rewritten to empty bytes")
	}
}

// TestRunFmt_EmptyFile guards that `fmt` refuses to rewrite an empty file
// as the literal string "null", which was the previous accidental behavior
// (yaml.v3 Marshal(zero) -> "null\n"). Empty in, empty document error out.
func TestRunFmt_EmptyFile(t *testing.T) {
	p := writeTemp(t, "empty.yml", "")
	var stdout, stderr bytes.Buffer
	code, err := runFmt([]string{"-w", p}, &stdout, &stderr)
	if code != 3 || err == nil {
		t.Fatalf("want (3, err) for empty input, got (%d, %v)", code, err)
	}
	// The file on disk must be untouched.
	b, _ := os.ReadFile(p)
	if len(b) != 0 {
		t.Fatalf("fmt -w on empty wrote bytes: %q", b)
	}
}

// TestRunFmt_FileSizeCap is the Cycle F regression: Cycle A added a 32 MiB
// file-size cap to parse.LoadFile so lint/diff cannot be OOM'd by a hostile
// input, but `fmt` used os.ReadFile directly and silently accepted files of
// any size. A 40 MiB file was happily loaded and then re-serialised. The cap
// now applies to `fmt` too, via cmd/difyctl.readFileCapped. We write a file
// one byte past the cap and assert refusal; we also assert that `-w` did not
// corrupt the original bytes.
func TestRunFmt_FileSizeCap(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates ~32 MiB")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "huge.yml")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte("a"), 1024)
	// Write MaxFileSize+1024 bytes — comfortably past the cap.
	for written := int64(0); written <= 32*1024*1024; written += int64(len(chunk)) {
		if _, werr := f.Write(chunk); werr != nil {
			t.Fatal(werr)
		}
	}
	if cerr := f.Close(); cerr != nil {
		t.Fatal(cerr)
	}
	origSize := int64(0)
	if fi, serr := os.Stat(p); serr == nil {
		origSize = fi.Size()
	}

	var stdout, stderr bytes.Buffer
	code, err := runFmt([]string{"-w", p}, &stdout, &stderr)
	if code != 3 || err == nil {
		t.Fatalf("want (3, err) for oversize input, got (%d, %v)", code, err)
	}
	// -w must not have rewritten the (capped) file with the truncated prefix.
	fi, _ := os.Stat(p)
	if fi.Size() != origSize {
		t.Fatalf("fmt -w on oversize file rewrote bytes: orig=%d now=%d", origSize, fi.Size())
	}
}

// TestRunFmt_DirectoryRejected guards that passing a directory yields a clean
// IO error (exit 3) rather than a confusing yaml.Unmarshal failure. os.Open of
// a directory succeeds on Unix; the readFileCapped helper must catch the
// IsDir case before io.ReadAll happily slurps the directory entries.
func TestRunFmt_DirectoryRejected(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code, err := runFmt([]string{dir}, &stdout, &stderr)
	if code != 3 || err == nil {
		t.Fatalf("want (3, err) for directory arg, got (%d, %v)", code, err)
	}
}
