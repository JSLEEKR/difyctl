package parse

import (
	"errors"
	"os"
	"path/filepath"
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
