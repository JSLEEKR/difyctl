package parse

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const basicYAML = `app:
  name: Demo
  mode: workflow
  description: ""
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: start-1
        type: start
        data:
          title: Start
      - id: llm-1
        type: llm
        data:
          title: Gen
    edges:
      - id: e1
        source: start-1
        target: llm-1
`

func TestParseBytes_Good(t *testing.T) {
	wf, err := ParseBytes([]byte(basicYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.App.Mode != "workflow" {
		t.Fatalf("want mode=workflow, got %q", wf.App.Mode)
	}
	if len(wf.Workflow.Graph.Nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(wf.Workflow.Graph.Nodes))
	}
	if wf.Workflow.Graph.Nodes[0].Line == 0 {
		t.Fatalf("expected node line annotation, got 0")
	}
	if wf.Workflow.Graph.Edges[0].Line == 0 {
		t.Fatalf("expected edge line annotation, got 0")
	}
}

func TestParseBytes_Empty(t *testing.T) {
	_, err := ParseBytes(nil)
	if !errors.Is(err, ErrParse) {
		t.Fatalf("want ErrParse, got %v", err)
	}
}

func TestParseBytes_Malformed(t *testing.T) {
	_, err := ParseBytes([]byte("not:\n  valid: : : yaml"))
	if !errors.Is(err, ErrParse) {
		t.Fatalf("want ErrParse, got %v", err)
	}
}

func TestParseBytes_RootNotMapping(t *testing.T) {
	_, err := ParseBytes([]byte("- a\n- b\n"))
	if !errors.Is(err, ErrParse) {
		t.Fatalf("want ErrParse, got %v", err)
	}
}

func TestLoadFile_Missing(t *testing.T) {
	_, err := LoadFile("/nonexistent/does-not-exist.yml")
	if !errors.Is(err, ErrIO) {
		t.Fatalf("want ErrIO, got %v", err)
	}
}

func TestLoadFile_Good(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "w.yml")
	if err := os.WriteFile(path, []byte(basicYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	wf, err := LoadFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Path != path {
		t.Fatalf("expected path set, got %q", wf.Path)
	}
}

func TestParseBytes_NoPanicOnGarbage(t *testing.T) {
	// Fuzz-style: arbitrary bytes must return an error (not panic).
	inputs := [][]byte{
		[]byte("\x00\x01\x02"),
		[]byte("%!%!%!"),
		[]byte("- [[["),
	}
	for _, in := range inputs {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panicked on %q: %v", in, r)
			}
		}()
		_, _ = ParseBytes(in)
	}
}

// TestLoadFile_ErrorMessageNotDuplicated guards against a regression where
// the open error was double-wrapped, producing "io error: open X: open X: ...".
func TestLoadFile_ErrorMessageNotDuplicated(t *testing.T) {
	_, err := LoadFile("/nonexistent/definitely-not-here.yml")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	// Expect a single "open /...:" segment, not two.
	if strings.Count(msg, "open ") > 1 {
		t.Fatalf("error message contains 'open ' more than once (double-wrap): %q", msg)
	}
	if !errors.Is(err, ErrIO) {
		t.Fatalf("expected ErrIO, got %v", err)
	}
}

// TestParseBytes_MultiDocRejected is the Cycle H regression: yaml.Unmarshal
// silently consumes only the first document when the input contains multiple
// YAML documents separated by `---`. Before the fix, lint/diff would rule
// against doc #1 only (ignoring doc #2..N with NO warning) and — worse — a
// follow-up `fmt -w` would rewrite the user's file to just doc #1, silently
// truncating the rest. We now detect multi-doc up-front and refuse.
func TestParseBytes_MultiDocRejected(t *testing.T) {
	cases := map[string]string{
		"two-docs": "app: {name: A, mode: workflow}\n---\napp: {name: B, mode: workflow}\n",
		"three-docs": "app: {name: A, mode: workflow}\n---\napp: {name: B, mode: workflow}\n" +
			"---\napp: {name: C, mode: workflow}\n",
		"leading-doc-marker":   "---\napp: {name: A, mode: workflow}\n---\napp: {name: B, mode: workflow}\n",
		"second-doc-malformed": "app: {name: A, mode: workflow}\n---\nnot:\n  valid: : : yaml\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseBytes([]byte(src))
			if err == nil {
				t.Fatal("want error on multi-doc input, got nil")
			}
			if !errors.Is(err, ErrParse) {
				t.Fatalf("want ErrParse, got %v", err)
			}
			if !errors.Is(err, ErrMultiDoc) {
				t.Fatalf("errors.Is(err, ErrMultiDoc) = false — callers using errors.Is on the sentinel are broken. Got err=%v", err)
			}
			// Belt-and-suspenders: the human-readable message must also
			// mention multi-document so CLI users get a clear answer.
			if !strings.Contains(err.Error(), "multi-document") {
				t.Fatalf("error should mention multi-document, got %v", err)
			}
		})
	}
}

// TestIsMultiDoc_SingleDocs locks that ordinary single-doc YAML — including
// streams that happen to contain a leading `---` marker before a single doc —
// is NOT misclassified as multi-doc.
func TestIsMultiDoc_SingleDocs(t *testing.T) {
	cases := map[string]string{
		"plain":            "app: {name: A, mode: workflow}\n",
		"leading-marker":   "---\napp: {name: A, mode: workflow}\n",
		"trailing-marker":  "app: {name: A, mode: workflow}\n---\n",
		"empty":            "",
		"only-whitespace":  "   \n\n",
		"comment-only":     "# just a comment\n",
		"trailing-newline": "app: {name: A, mode: workflow}\n\n\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if IsMultiDoc([]byte(src)) {
				t.Fatalf("IsMultiDoc classified single-doc %q as multi", src)
			}
		})
	}
}

// TestParseBytes_SentinelErrorsChained is Cycle N's architectural regression:
// every documented shape-sentinel in this package must be in the error chain
// for the input that triggers it, so callers using errors.Is on the sentinel
// keep working regardless of yaml.v3's wording changes. Cycle M fixed this
// for ErrMultiDoc; Cycle N extends it to ErrEmpty, ErrNotMapping, and
// ErrDupKey. If this test regresses, fmt/canonical.go's errors.Is switch arms
// will silently fall through to the default branch — the same latent bug
// shape Cycle M discovered for multi-doc.
func TestParseBytes_SentinelErrorsChained(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		sentinel error
	}{
		{"empty", "", ErrEmpty},
		{"multi-doc", "app: {name: A, mode: workflow}\n---\napp: {name: B, mode: workflow}\n", ErrMultiDoc},
		{"non-mapping-scalar", "42\n", ErrNotMapping},
		{"non-mapping-sequence", "- a\n- b\n", ErrNotMapping},
		{"non-mapping-null", "~\n", ErrNotMapping},
		{"non-mapping-whitespace", "   \n\n", ErrNotMapping},
		{"non-mapping-comment", "# hello\n", ErrNotMapping},
		{"dup-key", "a: 1\na: 2\n", ErrDupKey},
		{"dup-key-nested", "app:\n  name: A\n  name: B\n  mode: workflow\nkind: app\nversion: \"0.1\"\nworkflow: {graph: {nodes: [], edges: []}}\n", ErrDupKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseBytes([]byte(tc.src))
			if err == nil {
				t.Fatalf("want error for %q, got nil", tc.name)
			}
			if !errors.Is(err, ErrParse) {
				t.Fatalf("errors.Is(err, ErrParse) = false for %q — the top-level parse sentinel must always be in the chain. Got err=%v", tc.name, err)
			}
			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("errors.Is(err, %v) = false for %q — callers using errors.Is on the shape sentinel are broken. Got err=%v", tc.sentinel, tc.name, err)
			}
		})
	}
}

// TestParseBytes_DupKeyPreservesLineNumber locks that the yaml.v3 line-number
// context ("already defined at line N") survives the typed-sentinel wrap. The
// line number is the only way users can find the offending key in a large file,
// so losing it would be a regression equivalent to the Cycle K bug where fmt
// swallowed lint's wording.
func TestParseBytes_DupKeyPreservesLineNumber(t *testing.T) {
	src := "a: 1\na: 2\n"
	_, err := ParseBytes([]byte(src))
	if err == nil {
		t.Fatal("want dup-key error, got nil")
	}
	if !errors.Is(err, ErrDupKey) {
		t.Fatalf("want ErrDupKey in chain, got %v", err)
	}
	if !strings.Contains(err.Error(), "already defined at line") {
		t.Fatalf("error lost line-number context from yaml.v3: %v", err)
	}
}

// TestLoadFile_TooLarge verifies that the MaxFileSize cap is enforced.
func TestLoadFile_TooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.yml")
	// Write MaxFileSize+1 bytes of harmless content.
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Fill with 'a' bytes (valid YAML scalar), just over the cap.
	chunk := bytes.Repeat([]byte("a"), 1024)
	for written := int64(0); written <= MaxFileSize; written += int64(len(chunk)) {
		if _, werr := f.Write(chunk); werr != nil {
			t.Fatal(werr)
		}
	}
	if cerr := f.Close(); cerr != nil {
		t.Fatal(cerr)
	}
	_, err = LoadFile(path)
	if err == nil {
		t.Fatal("expected oversize error, got nil")
	}
	if !errors.Is(err, ErrIO) {
		t.Fatalf("expected ErrIO, got %v", err)
	}
	if !strings.Contains(err.Error(), "cap") && !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error should mention cap/exceeds, got %q", err.Error())
	}
}
