// Package parse loads Dify workflow DSL YAML files into model.Workflow.
//
// It retains the yaml.Node tree so downstream code can report line numbers.
// Malformed input returns a structured error — never panics.
package parse

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/JSLEEKR/difyctl/internal/model"
	"gopkg.in/yaml.v3"
)

// ErrIO signals a filesystem-level failure.
var ErrIO = errors.New("io error")

// ErrParse signals malformed or unreadable YAML.
var ErrParse = errors.New("parse error")

// MaxFileSize caps how large a Dify DSL file may be. Hard-wired at 32 MiB —
// well above any realistic real-world export (seen in the wild: ~50 KB) and
// small enough that a hostile or corrupted file cannot OOM the linter.
const MaxFileSize = 32 * 1024 * 1024

// LoadFile reads and parses a workflow DSL at the given path.
// Reads are capped at MaxFileSize so that a pathological input cannot OOM.
func LoadFile(path string) (*model.Workflow, error) {
	f, err := os.Open(path)
	if err != nil {
		// err is typically *fs.PathError whose Error() already contains
		// "open <path>: <reason>". Don't re-print the path; just wrap.
		return nil, fmt.Errorf("%w: %v", ErrIO, err)
	}
	defer f.Close()
	// Stat to reject obviously oversized files before any read.
	if fi, statErr := f.Stat(); statErr == nil && fi.Size() > MaxFileSize {
		return nil, fmt.Errorf("%w: %s: file is %d bytes, exceeds cap of %d", ErrIO, path, fi.Size(), MaxFileSize)
	}
	// Hard cap with LimitReader in case Stat was unreliable (pipes, special files).
	b, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %v", ErrIO, path, err)
	}
	if int64(len(b)) > MaxFileSize {
		return nil, fmt.Errorf("%w: %s: file exceeds %d-byte cap", ErrIO, path, MaxFileSize)
	}
	wf, err := ParseBytes(b)
	if err != nil {
		return nil, err
	}
	wf.Path = path
	return wf, nil
}

// ParseBytes parses a YAML byte slice into a Workflow.
func ParseBytes(b []byte) (*model.Workflow, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("%w: empty document", ErrParse)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParse, err)
	}
	// Extract the mapping node.
	doc := &root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w: root must be a mapping", ErrParse)
	}

	wf := &model.Workflow{}
	if err := doc.Decode(wf); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrParse, err)
	}
	wf.RawRoot = doc
	annotateLines(doc, wf)
	return wf, nil
}

// annotateLines walks the yaml.Node tree and attaches line numbers to Node/Edge.
func annotateLines(root *yaml.Node, wf *model.Workflow) {
	if root == nil {
		return
	}
	// workflow -> graph -> nodes/edges.
	workflowNode := lookupKey(root, "workflow")
	if workflowNode == nil {
		return
	}
	graphNode := lookupKey(workflowNode, "graph")
	if graphNode == nil {
		return
	}
	nodesSeq := lookupKey(graphNode, "nodes")
	if nodesSeq != nil && nodesSeq.Kind == yaml.SequenceNode {
		for i, item := range nodesSeq.Content {
			if i >= len(wf.Workflow.Graph.Nodes) {
				break
			}
			wf.Workflow.Graph.Nodes[i].Line = item.Line
		}
	}
	edgesSeq := lookupKey(graphNode, "edges")
	if edgesSeq != nil && edgesSeq.Kind == yaml.SequenceNode {
		for i, item := range edgesSeq.Content {
			if i >= len(wf.Workflow.Graph.Edges) {
				break
			}
			wf.Workflow.Graph.Edges[i].Line = item.Line
		}
	}
}

// lookupKey finds the value node for a given string key inside a mapping.
func lookupKey(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := m.Content[i]
		v := m.Content[i+1]
		if k.Value == key {
			return v
		}
	}
	return nil
}
