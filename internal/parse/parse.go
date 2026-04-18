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
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/JSLEEKR/difyctl/internal/fileio"
	"github.com/JSLEEKR/difyctl/internal/model"
	"gopkg.in/yaml.v3"
)

// ErrIO signals a filesystem-level failure.
var ErrIO = errors.New("io error")

// ErrParse signals malformed or unreadable YAML.
var ErrParse = errors.New("parse error")

// ErrMultiDoc is returned when input contains more than one YAML document
// (separated by `---`). yaml.Unmarshal silently decodes only the first and
// drops the rest, which is a data-loss footgun for `fmt -w` (user's file ends
// up truncated to just doc #1 on disk) and a confusing half-answer for
// lint/diff (they rule against doc #1 only, silently ignoring doc #2..N).
// Dify workflow DSL files are single-document by convention; reject the rest.
var ErrMultiDoc = errors.New("multi-document YAML not supported (Dify DSL is single-document)")

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

// Validate runs the same syntactic and shape checks as ParseBytes but discards
// the decoded result. Use this when you only need a "is this valid Dify DSL?"
// answer without paying for the line-number annotation pass.
//
// Why it exists: prior cycles E, G, H, I, K all patched the same class of bug —
// fmt's input acceptance diverged from parse's strict-decode rules, so a user
// could `lint file.yml` (exit 3) then `fmt -w file.yml` (silently rewrite) on
// the same file. Each cycle added one more per-shape gate to fmt to close the
// gap. This helper lets fmt route input through the SAME validator lint uses,
// so any future strict-decode rule added here is automatically inherited by
// fmt — eliminating the recurring drift.
func Validate(b []byte) error {
	_, err := ParseBytes(b)
	return err
}

// ParseBytes parses a YAML byte slice into a Workflow.
func ParseBytes(b []byte) (*model.Workflow, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("%w: empty document", ErrParse)
	}
	if IsMultiDoc(b) {
		return nil, fmt.Errorf("%w: %v", ErrParse, ErrMultiDoc)
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

// IsMultiDoc reports whether src contains more than one YAML document with
// actual content. yaml.Unmarshal silently consumes only the first document on
// multi-doc input — a data-loss footgun for `fmt -w` and a confusing
// half-answer for lint/diff.
//
// A trailing bare `---\n` (single doc followed by a document separator and
// nothing else) is NOT multi-doc: yaml.v3 reports the tail as a null document
// which carries no information. We treat only substantive extra documents as
// multi-doc so that e.g. `app: {..}\n---\n` (a common editor artifact) still
// passes through cleanly.
//
// Implementation note: we accept the small cost of parsing the whole file
// twice (once here, once in the caller's Unmarshal) to keep the detection
// absolutely simple and to avoid threading the decoded first node through.
// For realistic DSL files (~50 KB) the overhead is negligible; the 32 MiB cap
// bounds the worst case.
func IsMultiDoc(src []byte) bool {
	dec := yaml.NewDecoder(bytes.NewReader(src))
	var first yaml.Node
	if err := dec.Decode(&first); err != nil {
		// Either the stream is empty or doc #1 is malformed; the caller's
		// Unmarshal will surface the parse error. Not multi-doc.
		return false
	}
	// Walk the rest of the stream looking for a second document with actual
	// content. A trailing `---\n` with nothing after is decoded as an empty
	// node (Kind=0) — ignore it. An explicit null scalar (`null` / `~`) is
	// still content-bearing, so we DO flag `doc1\n---\nnull\n` as multi-doc,
	// since the user wrote a deliberate second document.
	for {
		var next yaml.Node
		err := dec.Decode(&next)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return false
			}
			// Any decode error on doc #2 that is NOT the clean end-of-stream
			// means there IS a second document, it's just unparseable. Treat
			// as multi-doc so we reject with a clear message rather than
			// letting the caller's Unmarshal silently succeed on doc #1.
			return true
		}
		if docIsEmpty(&next) {
			// A trailing `---\n` with no actual content after decodes as a
			// DocumentNode whose child is an empty-valued !!null scalar. Skip
			// these — they carry no information and are a common editor
			// artifact we must not misclassify.
			continue
		}
		return true
	}
}

// docIsEmpty reports whether a decoded yaml.Node (typically a DocumentNode)
// contains no actual content. Used by IsMultiDoc to ignore bare trailing
// `---\n` markers which yaml.v3 reports as an extra document containing a
// tag=!!null, value="" scalar.
func docIsEmpty(n *yaml.Node) bool {
	if n == nil || n.Kind == 0 {
		return true
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return true
		}
		c := n.Content[0]
		if c.Kind == yaml.ScalarNode && c.Tag == "!!null" && c.Value == "" {
			return true
		}
	}
	return false
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
