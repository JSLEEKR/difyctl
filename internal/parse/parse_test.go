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
