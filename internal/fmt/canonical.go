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
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/JSLEEKR/difyctl/internal/fileio"
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

// ErrNotMapping is returned when the document's root is a non-mapping scalar
// (`42`, `true`, `foo`) or a top-level sequence. These are syntactically valid
// YAML but meaningless as a Dify DSL — lint and diff reject them via
// parse.ParseBytes with "root must be a mapping"; fmt must behave identically
// so that all three subcommands agree on what a "valid DSL file" is.
// Previously fmt happily re-emitted `42\n` for input `42`, which is surprising
// and creates a parity gap between lint (reject) and fmt (accept + rewrite
// on -w).
var ErrNotMapping = errors.New("format: root must be a mapping")

// ErrMultiDoc is returned when src contains more than one YAML document
// (separated by `---`). yaml.Unmarshal silently decodes only the first one,
// which for `fmt -w` meant the user's multi-doc file got TRUNCATED on disk to
// doc #1 — classic silent data loss in the same spirit as the Cycle E UTF-16
// bug. parse.ParseBytes rejects the same shape with its own ErrMultiDoc, so
// all three subcommands now refuse identically.
var ErrMultiDoc = errors.New("format: multi-document YAML not supported (Dify DSL is single-document)")

// ErrAnchors is returned when the input uses YAML anchors (`&name`) paired
// with aliases (`*name`). Canonical formatting reorders top-level keys and
// sorts nodes/edges by id; if the anchor-defining element ends up AFTER the
// alias-using element in the emitted stream, the output is invalid YAML
// (`unknown anchor 'name' referenced`). `fmt -w` in that case would silently
// corrupt the user's file — same class of data-loss bug as Cycles E (UTF-16
// ASCII-stripping) and H (multi-doc truncation). Dify's DSL exporter does NOT
// emit anchors/aliases, so the practical cost of refusal is ~zero. Users
// hand-editing a DSL with anchors get a clear error instead of a corrupted
// file.
var ErrAnchors = errors.New("format: YAML anchors/aliases (&name / *name) are not supported — canonical reordering would break them")

// ErrRoundTrip is returned when the canonically re-emitted bytes fail to
// re-parse as YAML. This is the architectural backstop for the class of
// silent-data-loss bugs that Cycles E (UTF-16 ASCII-stripping), H (multi-doc
// truncation), and I (anchor-alias reorder producing invalid YAML) all
// belonged to: each was a case where Format returned (bytes, nil) but the
// returned bytes were NOT a valid Dify DSL on their face. `fmt -w` then
// persisted corrupted bytes to disk. Per-class gates (one per Cycle) kept
// catching the specific symptom; a round-trip self-check catches the whole
// class at once. If Format ever produces bytes yaml.v3 cannot re-parse, we
// refuse the write rather than corrupt the file. The check is strict about
// "re-parses as YAML"; semantic equivalence (idempotence) is a stronger
// property already covered by TestFormat_Idempotent.
var ErrRoundTrip = errors.New("format: round-trip re-parse failed — refusing to emit bytes that are not valid YAML")

// ErrDuplicateKeys is returned when the input mapping contains the same key
// more than once. yaml.Unmarshal into a *yaml.Node tree (which fmt uses to
// preserve structure for canonical reordering) silently accepts duplicate
// mapping keys. lint and diff route through parse.ParseBytes, which strict-
// decodes into a typed struct — that path REJECTS duplicates with a clear
// "mapping key 'x' already defined at line N" error. Without this guard, a
// user running `lint file.yml` would see exit 3 ("parse error: mapping key
// 'x' already defined"), then `fmt -w file.yml` would silently succeed and
// rewrite the (still-duplicated) file in place — exactly the cross-command
// parity gap that Cycle G fixed for non-mapping roots and Cycle E fixed for
// UTF-16. We now refuse fmt on duplicate-key input so all three subcommands
// agree on what a "valid Dify DSL file" is. The yaml.v3 error already names
// the offending line; we keep its wording verbatim for parity with lint.
var ErrDuplicateKeys = errors.New("format: duplicate mapping keys are not supported (lint/diff reject these too)")

// skipAnchorCheck is a test-only hook that bypasses the anchor pre-check so
// the round-trip self-check below can be exercised on anchored input. In
// production this is always false. Keeping the override here (rather than a
// duplicated format-without-anchor helper) avoids drift between the test path
// and the real one.
var skipAnchorCheck = false

// Format parses src YAML and returns canonically ordered YAML bytes. Unknown
// keys keep their original relative order after the ranked keys.
func Format(src []byte) ([]byte, error) {
	// Reject UTF-16 / UTF-32 BOMs BEFORE yaml.Unmarshal. yaml.v3 silently
	// ASCII-strips such input and returns a misleading scalar node, which
	// would cause `fmt -w` to overwrite the user's file with the stripped
	// remainder. A UTF-8 BOM (EF BB BF) is fine — yaml.v3 handles it. This
	// guard is retained as belt-and-suspenders in addition to the CLI's
	// fileio.ReadCapped check so that direct callers of Format(src []byte)
	// — e.g. tests and third-party users — also benefit from the refusal.
	if fileio.HasNonUTF8BOM(src) {
		return nil, ErrEncoding
	}
	if len(bytes.TrimSpace(src)) == 0 {
		return nil, ErrEmpty
	}
	// Reject multi-document input BEFORE Unmarshal. yaml.Unmarshal happily
	// returns only the first doc, so without this guard `fmt -w` would
	// silently truncate a multi-doc file to doc #1 on disk. See ErrMultiDoc.
	if isMultiDoc(src) {
		return nil, ErrMultiDoc
	}
	// Reject duplicate mapping keys BEFORE the *yaml.Node unmarshal below.
	// yaml.Unmarshal into a *yaml.Node tree silently accepts duplicates (it
	// only parses structure). Decoding into a generic `any` triggers yaml.v3's
	// strict map-population path which rejects duplicates with a clear line-
	// numbered error — the same path parse.ParseBytes uses, so lint/diff/fmt
	// now agree on what a valid Dify DSL is. We only flag the duplicate-key
	// shape here; any OTHER yaml.Unmarshal error is left to the *yaml.Node
	// unmarshal below to surface with its existing wording (so we don't lie
	// about, say, malformed YAML being a "duplicate key" issue).
	{
		var probe any
		if perr := yaml.Unmarshal(src, &probe); perr != nil {
			if strings.Contains(perr.Error(), "already defined at line") {
				return nil, fmt.Errorf("%w: %v", ErrDuplicateKeys, perr)
			}
			// Other errors fall through — the *yaml.Node unmarshal below will
			// surface them with the same wording fmt has always returned.
		}
	}
	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		return nil, err
	}
	// Reject anchors/aliases. Canonical reordering can move an anchor AFTER
	// the alias that references it, producing invalid YAML on re-emit. Rather
	// than silently corrupting the user's file on `fmt -w`, we refuse. Dify
	// DSL exports do not use anchors; hand-crafted files must be de-anchored
	// before formatting. See ErrAnchors.
	if !skipAnchorCheck && hasAnchors(&root) {
		return nil, ErrAnchors
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
	// Reject non-mapping roots (bare scalars `42`/`true`/`foo` or top-level
	// sequences `- a\n- b`). parse.ParseBytes — which backs lint and diff —
	// rejects the same inputs with "root must be a mapping". Without this
	// parity check, `difyctl fmt` would silently accept garbage that lint and
	// diff refuse. See TestFormat_NonMappingRootRejected for the regression.
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		c := root.Content[0]
		if c.Kind != yaml.MappingNode {
			return nil, ErrNotMapping
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
	out := buf.Bytes()
	// Architectural self-check: ensure the bytes we are about to return re-parse
	// as YAML. This is the generic backstop for the silent-data-loss cascade
	// (Cycles E / H / I) where Format returned (bytes, nil) but the bytes were
	// not a valid document. Catching "bytes are not valid YAML" once here
	// obviates adding a new per-class gate every time yaml.v3 surprises us with
	// another shape of broken-on-re-emit input. Cost is one extra yaml.Unmarshal
	// per Format call — negligible against the ~50 KB real DSLs and bounded by
	// the 32 MiB file cap. This does NOT guarantee semantic equivalence; that
	// stronger property is covered by the existing idempotence test.
	var roundTrip yaml.Node
	if err := yaml.Unmarshal(out, &roundTrip); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRoundTrip, err)
	}
	return out, nil
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

// sortNodesSeq stable-sorts nodes sequence by id. Nodes with no id are kept in
// their original relative order and placed after all id'd nodes — DIFY005 will
// already be complaining about them anyway. The previous implementation mixed
// "orig-order" and "id-order" in a single Less function, which produced a
// non-transitive comparator for inputs like [b, "", a] (A<B because one side
// was empty; B<C because empty; but A<C required "b"<"a", which is false).
// The resulting order was silently unsorted. We now partition first and sort
// each half independently.
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
	var withID, noID []item
	for i, c := range nodes.Content {
		id := mapStringField(c, "id")
		if id == "" {
			noID = append(noID, item{node: c, id: id, orig: i})
		} else {
			withID = append(withID, item{node: c, id: id, orig: i})
		}
	}
	sort.SliceStable(withID, func(i, j int) bool { return withID[i].id < withID[j].id })
	// noID stays in insertion order via the stable partition above.
	out := make([]*yaml.Node, 0, len(withID)+len(noID))
	for _, it := range withID {
		out = append(out, it.node)
	}
	for _, it := range noID {
		out = append(out, it.node)
	}
	nodes.Content = out
}

// isMultiDoc reports whether src contains more than one YAML document with
// actual content. Implemented locally rather than delegating to
// internal/parse to avoid an fmt→parse dep (parse already depends on fmt's
// sibling fileio; keeping fmt parse-free makes the dep graph a DAG). The
// detection is a small yaml.NewDecoder probe — cost of duplication is
// trivial. See parse.IsMultiDoc for the sibling implementation with the same
// semantics, including the trailing-`---`-carve-out.
func isMultiDoc(src []byte) bool {
	dec := yaml.NewDecoder(bytes.NewReader(src))
	var first yaml.Node
	if err := dec.Decode(&first); err != nil {
		// Empty or malformed; caller's Unmarshal will surface the error.
		return false
	}
	for {
		var next yaml.Node
		err := dec.Decode(&next)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return false
			}
			// Unparseable second doc — still multi-doc.
			return true
		}
		if docIsEmpty(&next) {
			// Trailing `---\n` with no content — skip.
			continue
		}
		return true
	}
}

// hasAnchors reports whether the node tree contains any YAML anchor (&name)
// or alias (*name). Canonical reordering — top-level keys, node id sort,
// edge id sort — can move an anchor AFTER its alias in the emitted stream,
// producing output that yaml fails to re-parse (`unknown anchor 'x'
// referenced`). `fmt -w` in that scenario silently corrupts the user's file;
// we refuse up-front instead. Also covers yaml merge keys (`<<: *base`)
// which are the most common anchor pattern in hand-written YAML.
func hasAnchors(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	if n.Anchor != "" || n.Kind == yaml.AliasNode {
		return true
	}
	for _, c := range n.Content {
		if hasAnchors(c) {
			return true
		}
	}
	return false
}

// docIsEmpty mirrors parse.docIsEmpty — duplicated here because fmt
// intentionally does not import parse (keeps the dep graph a DAG). See the
// comment on isMultiDoc for the rationale.
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
