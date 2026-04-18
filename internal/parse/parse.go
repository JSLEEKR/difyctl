// Package parse loads Dify workflow DSL YAML files into model.Workflow.
//
// It retains the yaml.Node tree so downstream code can report line numbers.
// Malformed input returns a structured error — never panics.
//
// All file I/O (size cap, directory rejection, non-UTF-8 BOM rejection) is
// delegated to internal/fileio so that lint, diff, and fmt agree byte-for-byte
// on what they accept. See internal/fileio/fileio.go for the rationale.
package parse

import (
	"errors"
	"fmt"

	"github.com/JSLEEKR/difyctl/internal/fileio"
	"github.com/JSLEEKR/difyctl/internal/model"
	"gopkg.in/yaml.v3"
)

// ErrIO signals a filesystem-level failure.
var ErrIO = errors.New("io error")

// ErrParse signals malformed or unreadable YAML.
var ErrParse = errors.New("parse error")

// MaxFileSize re-exports fileio.MaxFileSize so existing callers and tests keep
// working. The authoritative value lives in internal/fileio.
const MaxFileSize = fileio.MaxFileSize

// LoadFile reads and parses a workflow DSL at the given path.
// Reads are capped at MaxFileSize (32 MiB), directories are rejected, and
// UTF-16/UTF-32 byte-order marks are refused — see internal/fileio.
func LoadFile(path string) (*model.Workflow, error) {
	b, err := fileio.ReadCapped(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIO, err)
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
