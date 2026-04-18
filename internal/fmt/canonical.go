// Package fmt re-emits a Dify workflow DSL with canonical key ordering so
// that git diffs remain minimal.
//
// The approach is to parse the document as a *yaml.Node tree, reorder the
// content of specific mapping nodes in-place according to a rank table keyed
// by the parent path, and re-serialize. Unknown keys preserve their original
// order (appended after the ranked keys).
package fmt

import (
	"bytes"
	"errors"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrEmpty is returned when Format is called on an empty or whitespace-only
// document. yaml.v3 would otherwise silently marshal such input to the string
// "null\n", which is surprising for `fmt -w` (it clobbers the user's empty
// file with literal "null") and inconsistent with parse.ParseBytes which
// rejects empty documents.
var ErrEmpty = errors.New("format: empty document")

// ErrEncoding is returned when Format detects a byte-order-mark for a YAML
// encoding other than UTF-8. yaml.v3 does not actually decode UTF-16 / UTF-32
// bytes — it happily slurps the ASCII subset and returns a bogus document.
// If we then `fmt -w`, we would silently overwrite the user's UTF-16 file
// with the ASCII-stripped remainder — catastrophic data loss. Detect the
// common BOMs up-front and refuse.
var ErrEncoding = errors.New("format: non-UTF-8 input detected (yaml.v3 only decodes UTF-8)")

// Format parses src YAML and returns canonically ordered YAML bytes. Unknown
// keys keep their original relative order after the ranked keys.
func Format(src []byte) ([]byte, error) {
	// Reject UTF-16 / UTF-32 BOMs BEFORE yaml.Unmarshal. yaml.v3 silently
	// ASCII-strips such input and returns a misleading scalar node, which
	// would cause `fmt -w` to overwrite the user's file with the stripped
	// remainder. A UTF-8 BOM (EF BB BF) is fine — yaml.v3 handles it.
	if hasNonUTF8BOM(src) {
		return nil, ErrEncoding
	}
	if len(bytes.TrimSpace(src)) == 0 {
		return nil, ErrEmpty
	}
	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		return nil, err
	}
	// Reject cases where the document has no content — e.g. a file that is
	// only YAML comments like `# nothing here`. yaml.v3 parses these to a
	// DocumentNode with zero children and would otherwise marshal back as the
	// literal string "null\n", clobbering the user's comment-only file.
	if root.Kind == 0 || (root.Kind == yaml.DocumentNode && len(root.Content) == 0) {
		return nil, ErrEmpty
	}
	// Reject cases where the document decoded to a null scalar (e.g. input
	// like "~" or "null"): we have nothing meaningful to re-emit.
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		c := root.Content[0]
		if c.Kind == yaml.ScalarNode && (c.Tag == "!!null" || strings.EqualFold(c.Value, "null") || c.Value == "~" || c.Value == "") {
			return nil, ErrEmpty
		}
	}
	doc := &root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		reorder(doc.Content[0], "")
		sortEdgesSeq(doc.Content[0])
		sortNodesSeq(doc.Content[0])
	} else {
		reorder(doc, "")
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// order is the canonical key order per parent path. A parent path is a
// slash-joined sequence of keys from the document root. Inside arrays the path
// does not include the index.
var order = map[string][]string{
	"":                     {"app", "kind", "version", "workflow"},
	"app":                  {"name", "mode", "description"},
	"workflow":             {"graph"},
	"workflow/graph":       {"nodes", "edges"},
	"workflow/graph/nodes": {"id", "type", "data", "position"},
	"workflow/graph/edges": {"id", "source", "target", "sourceHandle", "targetHandle"},
}

// reorder walks a mapping/sequence tree and reorders mapping children per the
// `order` table. parentPath is the dotted key path leading to this node.
func reorder(n *yaml.Node, parentPath string) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.MappingNode:
		reorderMapping(n, parentPath)
		// Recurse into children.
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i].Value
			val := n.Content[i+1]
			child := key
			if parentPath != "" {
				child = parentPath + "/" + key
			}
			reorder(val, child)
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			// Inside sequences, the parent path stays the same (keyed by collection).
			reorder(c, parentPath)
		}
	}
}

// reorderMapping reorders n.Content according to the rank table at parentPath.
// Keys not in the rank table keep their original order, appended after ranked keys.
func reorderMapping(n *yaml.Node, parentPath string) {
	rank := order[parentPath]
	if len(rank) == 0 {
		// Default: sort alphabetically to get deterministic fmt output for unknown maps.
		sortMappingAlpha(n)
		return
	}

	rankIndex := make(map[string]int, len(rank))
	for i, k := range rank {
		rankIndex[k] = i
	}
	type pair struct {
		key   *yaml.Node
		value *yaml.Node
		origI int
	}
	var pairs []pair
	for i := 0; i+1 < len(n.Content); i += 2 {
		pairs = append(pairs, pair{key: n.Content[i], value: n.Content[i+1], origI: i})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		ri, iOK := rankIndex[pairs[i].key.Value]
		rj, jOK := rankIndex[pairs[j].key.Value]
		switch {
		case iOK && jOK:
			return ri < rj
		case iOK && !jOK:
			return true
		case !iOK && jOK:
			return false
		default:
			return pairs[i].origI < pairs[j].origI
		}
	})
	out := make([]*yaml.Node, 0, len(pairs)*2)
	for _, p := range pairs {
		out = append(out, p.key, p.value)
	}
	n.Content = out
}

// sortMappingAlpha sorts mapping keys alphabetically, used for generic maps
// where no explicit rank is defined (e.g. inside node.data).
func sortMappingAlpha(n *yaml.Node) {
	type pair struct {
		key   *yaml.Node
		value *yaml.Node
	}
	var pairs []pair
	for i := 0; i+1 < len(n.Content); i += 2 {
		pairs = append(pairs, pair{key: n.Content[i], value: n.Content[i+1]})
	}
	// Place "title" first if present, then alphabetical.
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].key.Value == "title" {
			return true
		}
		if pairs[j].key.Value == "title" {
			return false
		}
		return pairs[i].key.Value < pairs[j].key.Value
	})
	out := make([]*yaml.Node, 0, len(pairs)*2)
	for _, p := range pairs {
		out = append(out, p.key, p.value)
	}
	n.Content = out
}

// sortEdgesSeq stable-sorts the edges sequence by edge id (or src->dst if id empty).
func sortEdgesSeq(root *yaml.Node) {
	edges := findSeq(root, []string{"workflow", "graph", "edges"})
	if edges == nil {
		return
	}
	type item struct {
		node *yaml.Node
		key  string
	}
	var items []item
	for _, c := range edges.Content {
		key := edgeKeyFromMap(c)
		items = append(items, item{node: c, key: key})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].key < items[j].key })
	out := make([]*yaml.Node, len(items))
	for i, it := range items {
		out[i] = it.node
	}
	edges.Content = out
}

// sortNodesSeq stable-sorts nodes sequence by id (falls back to original order
// for nodes with no id — but we preserve empty-id at end).
func sortNodesSeq(root *yaml.Node) {
	nodes := findSeq(root, []string{"workflow", "graph", "nodes"})
	if nodes == nil {
		return
	}
	type item struct {
		node *yaml.Node
		id   string
		orig int
	}
	var items []item
	for i, c := range nodes.Content {
		id := mapStringField(c, "id")
		items = append(items, item{node: c, id: id, orig: i})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].id == "" || items[j].id == "" {
			return items[i].orig < items[j].orig
		}
		return items[i].id < items[j].id
	})
	out := make([]*yaml.Node, len(items))
	for i, it := range items {
		out[i] = it.node
	}
	nodes.Content = out
}

func edgeKeyFromMap(m *yaml.Node) string {
	id := mapStringField(m, "id")
	if id != "" {
		return id
	}
	src := mapStringField(m, "source")
	tgt := mapStringField(m, "target")
	h := mapStringField(m, "sourceHandle")
	return src + "->" + tgt + "#" + h
}

func mapStringField(m *yaml.Node, key string) string {
	if m == nil || m.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1].Value
		}
	}
	return ""
}

// hasNonUTF8BOM reports whether src starts with a UTF-16 or UTF-32 BOM. The
// UTF-8 BOM (EF BB BF) is accepted — yaml.v3 handles it natively.
func hasNonUTF8BOM(src []byte) bool {
	// UTF-32 BE: 00 00 FE FF
	if len(src) >= 4 && src[0] == 0x00 && src[1] == 0x00 && src[2] == 0xFE && src[3] == 0xFF {
		return true
	}
	// UTF-32 LE: FF FE 00 00
	if len(src) >= 4 && src[0] == 0xFF && src[1] == 0xFE && src[2] == 0x00 && src[3] == 0x00 {
		return true
	}
	// UTF-16 BE: FE FF
	if len(src) >= 2 && src[0] == 0xFE && src[1] == 0xFF {
		return true
	}
	// UTF-16 LE: FF FE  (check AFTER UTF-32 LE since they overlap in prefix)
	if len(src) >= 2 && src[0] == 0xFF && src[1] == 0xFE {
		return true
	}
	return false
}

func findSeq(root *yaml.Node, path []string) *yaml.Node {
	cur := root
	for _, p := range path {
		if cur == nil || cur.Kind != yaml.MappingNode {
			return nil
		}
		next := (*yaml.Node)(nil)
		for i := 0; i+1 < len(cur.Content); i += 2 {
			if cur.Content[i].Value == p {
				next = cur.Content[i+1]
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	if cur == nil || cur.Kind != yaml.SequenceNode {
		return nil
	}
	return cur
}
