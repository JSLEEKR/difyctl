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
// a directory succeeds on Unix; fileio.ReadCapped must catch the IsDir case
// before io.ReadAll happily slurps the directory entries.
func TestRunFmt_DirectoryRejected(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code, err := runFmt([]string{dir}, &stdout, &stderr)
	if code != 3 || err == nil {
		t.Fatalf("want (3, err) for directory arg, got (%d, %v)", code, err)
	}
}

// --- Cycle G regressions: cross-command parity ----------------------------

// TestCrossCommand_DirectoryRejected is the Cycle G parity test. Before Cycle
// G, `fmt` rejected directories cleanly (Cycle F fix) but `lint` and `diff`
// — which went through parse.LoadFile and then io.ReadAll — returned a
// double-wrapped "io error: read X: read X: is a directory". Worse, they
// sometimes slipped past and fed directory-listing bytes to yaml.v3 which
// returned an opaque "incompatible YAML document". All three subcommands must
// now refuse directories at the same layer (fileio.ReadCapped) and emit a
// single clean "<path>: is a directory" suffix.
func TestCrossCommand_DirectoryRejected(t *testing.T) {
	dir := t.TempDir()
	good := writeTemp(t, "g.yml", good)
	for _, tc := range []struct {
		name string
		run  func() (int, error)
	}{
		{"lint", func() (int, error) {
			var so, se bytes.Buffer
			return runLint([]string{dir}, &so, &se)
		}},
		{"diff-first-arg", func() (int, error) {
			var so, se bytes.Buffer
			return runDiff([]string{dir, good}, &so, &se)
		}},
		{"diff-second-arg", func() (int, error) {
			var so, se bytes.Buffer
			return runDiff([]string{good, dir}, &so, &se)
		}},
		{"fmt", func() (int, error) {
			var so, se bytes.Buffer
			return runFmt([]string{dir}, &so, &se)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, err := tc.run()
			if code != 3 || err == nil {
				t.Fatalf("want (3, err), got (%d, %v)", code, err)
			}
			if !strings.Contains(err.Error(), "is a directory") {
				t.Fatalf("%s: error should contain 'is a directory', got: %v", tc.name, err)
			}
			// No double-wrap: the message must contain "is a directory" only once.
			if strings.Count(err.Error(), "is a directory") > 1 {
				t.Fatalf("%s: double-wrapped message: %v", tc.name, err)
			}
			// Similarly the path must not appear twice (regression for the
			// "read X: read X: ..." double-wrap).
			if strings.Count(err.Error(), "read ") > 1 {
				t.Fatalf("%s: 'read ' appears multiple times (double-wrap): %v", tc.name, err)
			}
		})
	}
}

// TestCrossCommand_UTF16BOMRejected is the core Cycle G cascade test. Cycle E
// made `fmt` refuse UTF-16/UTF-32 input (because yaml.v3 silently ASCII-strips
// such bytes and `fmt -w` would clobber the file with the remainder). But the
// same decoder backs lint and diff: both would happily run rules over the
// mangled ASCII subset, reporting nonsense. After Cycle G's fileio extraction
// all three refuse at the read layer.
func TestCrossCommand_UTF16BOMRejected(t *testing.T) {
	dir := t.TempDir()
	// UTF-16 LE BOM + a few ASCII-encoded chars.
	bom := []byte{0xFF, 0xFE, 'a', 0x00, 'p', 0x00, 'p', 0x00, ':', 0x00, '\n', 0x00}
	bomPath := filepath.Join(dir, "bom.yml")
	if err := os.WriteFile(bomPath, bom, 0o644); err != nil {
		t.Fatal(err)
	}
	goodPath := writeTemp(t, "g.yml", good)

	for _, tc := range []struct {
		name string
		run  func() (int, error)
	}{
		{"lint", func() (int, error) {
			var so, se bytes.Buffer
			return runLint([]string{bomPath}, &so, &se)
		}},
		{"diff-first-arg", func() (int, error) {
			var so, se bytes.Buffer
			return runDiff([]string{bomPath, goodPath}, &so, &se)
		}},
		{"diff-second-arg", func() (int, error) {
			var so, se bytes.Buffer
			return runDiff([]string{goodPath, bomPath}, &so, &se)
		}},
		{"fmt", func() (int, error) {
			var so, se bytes.Buffer
			return runFmt([]string{bomPath}, &so, &se)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, err := tc.run()
			if code != 3 || err == nil {
				t.Fatalf("want (3, err) for UTF-16 input, got (%d, %v)", code, err)
			}
			if !strings.Contains(err.Error(), "UTF-8") && !strings.Contains(err.Error(), "non-UTF-8") {
				t.Fatalf("%s: error should mention encoding, got: %v", tc.name, err)
			}
		})
	}
}

// TestCrossCommand_OversizeRejected asserts that the 32 MiB cap applies
// uniformly. Cycle F added the cap to fmt; this locks that lint and diff also
// use the same limit so future guards land in one place.
func TestCrossCommand_OversizeRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates ~32 MiB")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "big.yml")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte("a"), 1024)
	for written := int64(0); written <= 32*1024*1024; written += int64(len(chunk)) {
		if _, werr := f.Write(chunk); werr != nil {
			t.Fatal(werr)
		}
	}
	_ = f.Close()
	goodPath := writeTemp(t, "g.yml", good)
	for _, tc := range []struct {
		name string
		run  func() (int, error)
	}{
		{"lint", func() (int, error) {
			var so, se bytes.Buffer
			return runLint([]string{p}, &so, &se)
		}},
		{"diff", func() (int, error) {
			var so, se bytes.Buffer
			return runDiff([]string{p, goodPath}, &so, &se)
		}},
		{"fmt", func() (int, error) {
			var so, se bytes.Buffer
			return runFmt([]string{p}, &so, &se)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, err := tc.run()
			if code != 3 || err == nil {
				t.Fatalf("want (3, err) for oversize input, got (%d, %v)", code, err)
			}
			if !strings.Contains(err.Error(), "cap") && !strings.Contains(err.Error(), "exceeds") {
				t.Fatalf("%s: expected cap/exceeds in error, got: %v", tc.name, err)
			}
		})
	}
}

// TestCrossCommand_MultiDocRejected is the Cycle H parity regression. yaml.v3
// silently decodes only the first document of a multi-doc stream, so before
// this fix:
//   - `lint` would rule against doc #1 and ignore doc #2..N with no warning,
//   - `diff` would compare doc #1 vs doc #1 and miss any breaking change
//     hidden in doc #2,
//   - `fmt -w` would REWRITE the user's multi-doc file with just doc #1,
//     silently truncating on disk — classic data loss, same spirit as the
//     Cycle E UTF-16 ASCII-stripping bug.
//
// All three subcommands must now refuse multi-doc input with exit code 3.
func TestCrossCommand_MultiDocRejected(t *testing.T) {
	dir := t.TempDir()
	multi := "app: {name: A, mode: workflow}\n---\napp: {name: B, mode: workflow}\n"
	multiPath := filepath.Join(dir, "multi.yml")
	if err := os.WriteFile(multiPath, []byte(multi), 0o644); err != nil {
		t.Fatal(err)
	}
	goodPath := writeTemp(t, "g.yml", good)
	origBytes, _ := os.ReadFile(multiPath)

	for _, tc := range []struct {
		name string
		run  func() (int, error)
	}{
		{"lint", func() (int, error) {
			var so, se bytes.Buffer
			return runLint([]string{multiPath}, &so, &se)
		}},
		{"diff-first-arg", func() (int, error) {
			var so, se bytes.Buffer
			return runDiff([]string{multiPath, goodPath}, &so, &se)
		}},
		{"diff-second-arg", func() (int, error) {
			var so, se bytes.Buffer
			return runDiff([]string{goodPath, multiPath}, &so, &se)
		}},
		{"fmt", func() (int, error) {
			var so, se bytes.Buffer
			return runFmt([]string{multiPath}, &so, &se)
		}},
		{"fmt-w", func() (int, error) {
			var so, se bytes.Buffer
			return runFmt([]string{"-w", multiPath}, &so, &se)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, err := tc.run()
			if code != 3 || err == nil {
				t.Fatalf("want (3, err) for multi-doc input, got (%d, %v)", code, err)
			}
			if !strings.Contains(err.Error(), "multi-document") {
				t.Fatalf("%s: error should mention multi-document, got %v", tc.name, err)
			}
		})
	}
	// Belt-and-suspenders: the file on disk MUST be untouched — this is the
	// actual user-visible data-loss bug we are guarding against.
	now, _ := os.ReadFile(multiPath)
	if !bytes.Equal(origBytes, now) {
		t.Fatalf("multi-doc file was mutated on disk:\nbefore:\n%s\nafter:\n%s", origBytes, now)
	}
}

// TestCrossCommand_AnchorsHandled is the Cycle I parity assertion. A YAML
// anchor/alias pair — common in hand-crafted DSLs using `<<: *base` merges —
// is safe for lint and diff (they only read, never re-emit) but unsafe for
// fmt: canonical reordering can move the anchor-defining element AFTER its
// alias in the output, producing invalid YAML. `fmt -w` would have silently
// written the broken bytes to disk; now it refuses. Lint and diff are still
// expected to accept the input, since the user gets no useful feedback from
// refusing a read-only operation.
func TestCrossCommand_AnchorsHandled(t *testing.T) {
	const anchored = `app: {name: X, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: s
        type: start
        data: &a {title: S, x: 1}
      - id: e
        type: end
        data: *a
    edges:
      - {id: e-1, source: s, target: e}
`
	dir := t.TempDir()
	p := filepath.Join(dir, "a.yml")
	if err := os.WriteFile(p, []byte(anchored), 0o644); err != nil {
		t.Fatal(err)
	}
	origBytes, _ := os.ReadFile(p)

	// lint: should accept (DIFY warnings fine, but no IO/parse error).
	t.Run("lint-accepts", func(t *testing.T) {
		var so, se bytes.Buffer
		code, err := runLint([]string{p}, &so, &se)
		if err != nil {
			t.Fatalf("lint should accept anchors, got err: %v", err)
		}
		if code == 3 {
			t.Fatalf("lint should NOT return 3 for anchored input, got %d", code)
		}
	})

	// diff: should accept.
	t.Run("diff-accepts", func(t *testing.T) {
		var so, se bytes.Buffer
		code, err := runDiff([]string{p, p}, &so, &se)
		if err != nil || code != 0 {
			t.Fatalf("diff should accept anchors, got (%d, %v)", code, err)
		}
	})

	// fmt: MUST refuse to avoid silent file corruption.
	t.Run("fmt-refuses", func(t *testing.T) {
		var so, se bytes.Buffer
		code, err := runFmt([]string{p}, &so, &se)
		if code != 3 || err == nil {
			t.Fatalf("fmt should refuse anchored input, got (%d, %v)", code, err)
		}
		if !strings.Contains(err.Error(), "anchor") {
			t.Fatalf("fmt error should mention anchors, got: %v", err)
		}
	})

	// fmt -w: MUST refuse AND leave file untouched on disk.
	t.Run("fmt-w-refuses-and-preserves", func(t *testing.T) {
		var so, se bytes.Buffer
		code, err := runFmt([]string{"-w", p}, &so, &se)
		if code != 3 || err == nil {
			t.Fatalf("fmt -w should refuse anchored input, got (%d, %v)", code, err)
		}
		now, _ := os.ReadFile(p)
		if !bytes.Equal(origBytes, now) {
			t.Fatalf("fmt -w mutated an anchored file on disk — data loss!\nbefore:\n%s\nafter:\n%s", origBytes, now)
		}
	})
}

// TestCrossCommand_BareScalarRejected is Cycle G's answer to the Cycle F
// open question (b): `fmt` used to happily serialise `42\n` for input `42`
// while lint/diff rejected the same input with "root must be a mapping".
// The new ErrNotMapping in internal/fmt closes that gap.
func TestCrossCommand_BareScalarRejected(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []string{"42\n", "true\n", "foo\n", "- a\n- b\n"} {
		p := filepath.Join(dir, "s.yml")
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		goodPath := writeTemp(t, "g.yml", good)
		// All three must reject.
		{
			var so, se bytes.Buffer
			code, err := runLint([]string{p}, &so, &se)
			if code != 3 || err == nil {
				t.Fatalf("lint %q: want (3, err), got (%d, %v)", src, code, err)
			}
		}
		{
			var so, se bytes.Buffer
			code, err := runDiff([]string{p, goodPath}, &so, &se)
			if code != 3 || err == nil {
				t.Fatalf("diff %q: want (3, err), got (%d, %v)", src, code, err)
			}
		}
		{
			var so, se bytes.Buffer
			code, err := runFmt([]string{p}, &so, &se)
			if code != 3 || err == nil {
				t.Fatalf("fmt %q: want (3, err), got (%d, %v)", src, code, err)
			}
		}
	}
}
