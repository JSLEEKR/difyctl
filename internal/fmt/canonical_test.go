package fmt

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const scrambled = `workflow:
  graph:
    edges:
      - {source: a, target: c, id: e2}
      - {source: a, target: b, id: e1}
    nodes:
      - id: c
        data: {title: C}
        type: end
      - data: {title: A}
        type: start
        id: a
      - id: b
        type: llm
        data: {title: B, model: {name: gpt-4, provider: openai}}
version: "0.1"
kind: app
app:
  mode: workflow
  description: ""
  name: Demo
`

func TestFormat_TopLevelOrder(t *testing.T) {
	out, err := Format([]byte(scrambled))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// Order check: app < kind < version < workflow
	iApp := strings.Index(s, "\napp:")
	iKind := strings.Index(s, "\nkind:")
	iVer := strings.Index(s, "\nversion:")
	iWf := strings.Index(s, "\nworkflow:")
	// "app:" on first line will not have leading \n; so also allow index 0.
	if iApp == -1 {
		iApp = strings.Index(s, "app:")
	}
	if iApp > iKind || iKind > iVer || iVer > iWf {
		t.Fatalf("unexpected order app=%d kind=%d ver=%d wf=%d\n%s", iApp, iKind, iVer, iWf, s)
	}
}

func TestFormat_PerNodeOrder(t *testing.T) {
	out, err := Format([]byte(scrambled))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// For each node the id should appear before type, and type before data.
	// Find the first occurrence of "id:" in nodes subtree. We can check per-line positions.
	if !strings.Contains(s, "- id: a") {
		t.Fatalf("node 'a' id first key expected\n%s", s)
	}
}

func TestFormat_NodesSortedByID(t *testing.T) {
	out, err := Format([]byte(scrambled))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// The sequence of id lines under nodes should be a,b,c
	idxA := strings.Index(s, "id: a")
	idxB := strings.Index(s, "id: b")
	idxC := strings.Index(s, "id: c")
	if !(idxA < idxB && idxB < idxC) {
		t.Fatalf("nodes not sorted by id: a=%d b=%d c=%d\n%s", idxA, idxB, idxC, s)
	}
}

func TestFormat_EdgesSortedByID(t *testing.T) {
	out, err := Format([]byte(scrambled))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	idxE1 := strings.Index(s, "id: e1")
	idxE2 := strings.Index(s, "id: e2")
	if !(idxE1 < idxE2) {
		t.Fatalf("edges not sorted: e1=%d e2=%d\n%s", idxE1, idxE2, s)
	}
}

func TestFormat_Idempotent(t *testing.T) {
	once, err := Format([]byte(scrambled))
	if err != nil {
		t.Fatal(err)
	}
	twice, err := Format(once)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(once, twice) {
		t.Fatalf("not idempotent:\nonce=\n%s\ntwice=\n%s", once, twice)
	}
}

func TestFormat_InvalidYAMLReturnsError(t *testing.T) {
	_, err := Format([]byte("not: : yaml"))
	if err == nil {
		t.Fatal("expected error on malformed yaml")
	}
}

// TestFormat_EmptyRejected guards the Cycle C fix: an empty or whitespace-only
// input must NOT be "formatted" into the literal bytes "null\n", which was the
// previous accidental behavior inherited from yaml.v3's marshaling of a zero
// Node. Empty in, ErrEmpty out — the caller (cmd/difyctl/fmt.go) can then
// decide whether to leave the file alone or surface an error.
func TestFormat_EmptyRejected(t *testing.T) {
	for _, src := range []string{"", "   ", "\n\n\t\n"} {
		if _, err := Format([]byte(src)); err == nil {
			t.Fatalf("want ErrEmpty for %q, got nil", src)
		}
	}
}

// TestFormat_NullDocumentRejected covers inputs like "~" or literal "null".
// These decode to a null scalar and have no meaningful canonical form.
func TestFormat_NullDocumentRejected(t *testing.T) {
	for _, src := range []string{"~", "null", "NULL\n"} {
		if _, err := Format([]byte(src)); err == nil {
			t.Fatalf("want error for null scalar %q, got nil", src)
		}
	}
}

// TestFormat_NonUTF8BOMRejected is the Cycle E regression: yaml.v3 does not
// decode UTF-16/UTF-32 input — it silently slurps the ASCII subset and returns
// a bogus document. Before the fix, `fmt -w` on a UTF-16 file would overwrite
// the file with the ASCII-stripped remainder (e.g. UTF-16 LE bytes
// \xff\xfe a\x00 p\x00 p\x00 became the literal three bytes "app\n"). We now
// detect UTF-16/UTF-32 BOMs up-front and refuse.
func TestFormat_NonUTF8BOMRejected(t *testing.T) {
	cases := map[string][]byte{
		"UTF-16 LE": {0xFF, 0xFE, 'a', 0x00, 'p', 0x00, 'p', 0x00},
		"UTF-16 BE": {0xFE, 0xFF, 0x00, 'a', 0x00, 'p', 0x00, 'p'},
		"UTF-32 LE": {0xFF, 0xFE, 0x00, 0x00, 'a', 0x00, 0x00, 0x00},
		"UTF-32 BE": {0x00, 0x00, 0xFE, 0xFF, 0x00, 0x00, 0x00, 'a'},
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Format(src)
			if err == nil {
				t.Fatal("want ErrEncoding, got nil — fmt would silently corrupt the file")
			}
		})
	}
	// UTF-8 BOM + valid content must still work.
	utf8Valid := append([]byte{0xEF, 0xBB, 0xBF}, []byte("app: {name: X, mode: workflow}\n")...)
	if _, err := Format(utf8Valid); err != nil {
		t.Fatalf("UTF-8 BOM + valid content must work, got: %v", err)
	}
}

// TestFormat_CommentOnlyRejected is the Cycle D regression: a file that is
// only YAML comments (`# nothing here`) previously slipped past the Cycle C
// guards because yaml.v3 parses it to a DocumentNode with zero content, the
// ScalarNode check never fires, and the encoder then emitted the literal
// string "null\n" — clobbering a user's comment-only file on `fmt -w`.
func TestFormat_CommentOnlyRejected(t *testing.T) {
	for _, src := range []string{
		"# nothing\n",
		"# one\n# two\n",
		"\n# leading blank then comment\n",
		"# trailing-no-newline",
	} {
		out, err := Format([]byte(src))
		if err == nil {
			t.Fatalf("want error for comment-only %q, got bytes: %q", src, out)
		}
	}
}

// TestFormat_NonMappingRootRejected is Cycle G's parity fix. parse.ParseBytes
// (which backs lint and diff) rejects non-mapping roots with "root must be a
// mapping". Before Cycle G, Format happily re-emitted `42\n` on input `42`
// (or `- a\n- b\n` for a top-level sequence). Users running lint then fmt on
// the same file would see lint exit 3 but `fmt -w` quietly "succeed" and
// rewrite the file in place. Now fmt agrees with lint/diff.
func TestFormat_NonMappingRootRejected(t *testing.T) {
	for _, src := range []string{
		"42\n",       // bare integer scalar
		"true\n",     // bare bool scalar
		"foo\n",      // bare string scalar
		"- a\n- b\n", // top-level sequence
		"[1, 2, 3]\n",
	} {
		out, err := Format([]byte(src))
		if err == nil {
			t.Fatalf("want error for non-mapping root %q, got bytes: %q", src, out)
		}
	}
}

// TestFormat_AnchorsRejected is the Cycle I regression. A Dify DSL that uses
// YAML anchors (&name) paired with aliases (*name) is rare but legal, and
// Format's canonical reordering can move the anchor-defining element AFTER
// its alias in the emitted stream. The result is invalid YAML: yaml.v3's
// re-parser fails with `unknown anchor 'name' referenced`. Critically,
// `fmt -w` on such a file used to SUCCEED — writing the invalid bytes to
// disk — because Format returned (bytes, nil) and the re-parse never ran.
// Lint on the written file would then fail with exit 3, leaving the user
// with a corrupted source file. Same class of silent-data-loss bug as
// Cycles E (UTF-16) and H (multi-doc truncation). We now refuse up-front.
func TestFormat_AnchorsRejected(t *testing.T) {
	cases := map[string]string{
		// Anchor-in-data triggers the sort-by-id hazard: after Format sorts
		// nodes by id, "e" (alias user) appears before "s" (anchor owner).
		"anchor-in-node-data": `app: {name: X, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: s
        type: start
        data: &a
          x: 1
      - id: e
        type: end
        data: *a
    edges: []
`,
		// Top-level anchor triggers the unknown-keys-after-ranked hazard: the
		// anchor definition gets moved past workflow:, appearing after its use.
		"anchor-at-top-level": `anchors: &a
  x: 1
app:
  name: X
  mode: workflow
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: s
        type: start
        data: *a
    edges: []
`,
		// YAML merge keys (<<: *base) are the most common anchor pattern in
		// hand-written YAML and hit the same hazard.
		"yaml-merge-key": `base: &b
  title: Base
app:
  name: X
  mode: workflow
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - <<: *b
        id: s
        type: start
        data: {title: S}
    edges: []
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := Format([]byte(src))
			if err == nil {
				t.Fatalf("want ErrAnchors, got bytes: %q — fmt -w would risk corrupting the file", out)
			}
			if err != ErrAnchors {
				t.Fatalf("want ErrAnchors, got %v", err)
			}
		})
	}
}

// TestFormat_MultiDocRejected is the Cycle H regression: yaml.Unmarshal silently
// returns only the first document on a multi-doc stream. Before the fix, `fmt`
// happily re-emitted the (canonicalised) doc #1 and `fmt -w` would clobber the
// user's multi-doc file with just doc #1 on disk — silent truncation, same
// class of data-loss bug as Cycle E's UTF-16 ASCII-stripping. We now refuse.
func TestFormat_MultiDocRejected(t *testing.T) {
	cases := map[string]string{
		"two-docs":           "app: {name: A, mode: workflow}\n---\napp: {name: B, mode: workflow}\n",
		"leading-doc-marker": "---\napp: {name: A, mode: workflow}\n---\napp: {name: B, mode: workflow}\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := Format([]byte(src))
			if err == nil {
				t.Fatalf("want ErrMultiDoc, got bytes: %q — fmt -w would truncate this file", out)
			}
			if err != ErrMultiDoc && !strings.Contains(err.Error(), "multi-document") {
				t.Fatalf("want ErrMultiDoc, got %v", err)
			}
		})
	}
}

// TestFormat_TrailingDocMarkerStillAcceptedAsSingleDoc ensures we did not
// over-trigger: a stream with a single doc followed by a `---` separator is
// still a single document.
func TestFormat_TrailingDocMarkerStillAcceptedAsSingleDoc(t *testing.T) {
	src := "app: {name: X, mode: workflow, description: \"\"}\n" +
		"kind: app\n" +
		"version: \"0.1\"\n" +
		"workflow: {graph: {nodes: [], edges: []}}\n" +
		"---\n"
	if _, err := Format([]byte(src)); err != nil {
		t.Fatalf("trailing --- should be accepted as single-doc, got %v", err)
	}
}

func TestFormat_AppBlockKeyOrder(t *testing.T) {
	out, err := Format([]byte(scrambled))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	iName := strings.Index(s, "name: Demo")
	iMode := strings.Index(s, "mode: workflow")
	if !(iName < iMode) {
		t.Fatalf("app block key order wrong: name=%d mode=%d\n%s", iName, iMode, s)
	}
}

func TestFormat_PreservesUnknownKeys(t *testing.T) {
	input := `app:
  mode: workflow
  custom_key: hello
  name: X
  description: y
kind: app
version: "0.1"
workflow: {graph: {nodes: [], edges: []}}
`
	out, err := Format([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "custom_key: hello") {
		t.Fatalf("custom_key lost:\n%s", out)
	}
}

func TestFormat_UnknownKeysAfterRanked(t *testing.T) {
	input := `app:
  custom_key: hello
  mode: workflow
  name: X
  description: y
kind: app
version: "0.1"
workflow: {graph: {nodes: [], edges: []}}
`
	out, err := Format([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// ranked name/mode/description come before custom_key.
	iDesc := strings.Index(s, "description:")
	iCustom := strings.Index(s, "custom_key:")
	if !(iDesc < iCustom) {
		t.Fatalf("custom_key should come after description: desc=%d custom=%d\n%s", iDesc, iCustom, s)
	}
}

// TestFormat_NodesSortedWithEmptyID is the Cycle F regression. The previous
// sort-Less for nodes fell back to "orig-index" whenever EITHER side had an
// empty id, which is non-transitive and silently left nodes unsorted for
// inputs like [c, "", a, b]. The expected behaviour: id'd nodes sort
// alphabetically, empty-id nodes (DIFY005 will flag them) appear after and
// keep their relative insertion order.
func TestFormat_NodesSortedWithEmptyID(t *testing.T) {
	input := `app: {name: X, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - {id: c, type: llm, data: {title: C, model: {provider: o, name: g}}}
      - {type: end, data: {title: empty1}}
      - {id: a, type: start, data: {title: A}}
      - {id: b, type: llm, data: {title: B, model: {provider: o, name: g}}}
    edges: []
`
	out, err := Format([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	iA := strings.Index(s, "id: a")
	iB := strings.Index(s, "id: b")
	iC := strings.Index(s, "id: c")
	iEmpty := strings.Index(s, "title: empty1")
	if !(iA > 0 && iB > iA && iC > iB) {
		t.Fatalf("id-bearing nodes not sorted a<b<c: a=%d b=%d c=%d\n%s", iA, iB, iC, s)
	}
	if !(iEmpty > iC) {
		t.Fatalf("empty-id node must appear after all id'd nodes: c=%d empty=%d\n%s", iC, iEmpty, s)
	}
}

// TestFormat_RoundTripSelfCheck is Cycle J's architectural regression. The
// previous cycles E (UTF-16), H (multi-doc), and I (anchors) each patched one
// shape of "Format produced bytes that aren't valid Dify DSL on re-parse"
// with a dedicated per-class gate. A round-trip self-check at the end of
// Format catches the entire class in one place: if the canonically
// re-emitted bytes fail to yaml.Unmarshal, we refuse to return them so that
// `fmt -w` never writes a corrupted file to disk.
//
// This test bypasses the anchor pre-check to construct a case where Format
// WOULD emit invalid YAML (anchor-owner sorted after alias-user), and asserts
// that the round-trip check fires. If this test ever regresses, the
// architectural backstop is broken and the N=4 cascade that Cycles E/H/I/J
// avoided will return the next time yaml.v3 surprises us.
func TestFormat_RoundTripSelfCheck(t *testing.T) {
	// Input: the "anchor-in-node-data" shape from TestFormat_AnchorsRejected.
	// After canonical sort-by-id, node 'e' (alias user) appears BEFORE node 's'
	// (anchor owner), which yaml.v3 re-parses as "unknown anchor 'a'".
	const anchored = `app: {name: X, mode: workflow}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: s
        type: start
        data: &a
          x: 1
      - id: e
        type: end
        data: *a
    edges: []
`
	// Sanity: the anchor pre-check rejects this in normal operation. We want
	// to prove the round-trip check ALSO rejects it if the anchor pre-check
	// were ever removed or bypassed.
	prev := skipAnchorCheck
	skipAnchorCheck = true
	defer func() { skipAnchorCheck = prev }()

	_, err := Format([]byte(anchored))
	if err == nil {
		t.Fatal("want round-trip error when anchor check is bypassed, got nil — architectural backstop is broken")
	}
	if !errors.Is(err, ErrRoundTrip) {
		t.Fatalf("want ErrRoundTrip, got %v", err)
	}
}

// TestFormat_RoundTripDoesNotSpuriouslyFail is the negative side of the
// round-trip self-check. Every shape of valid Dify DSL that the prior cycles
// locked in must pass through Format WITHOUT tripping the round-trip gate.
// If this regresses, the architectural change has broken backwards
// compatibility — any of the 200+ existing tests could have caught it, but we
// want an explicit assertion here so the intent is clear.
func TestFormat_RoundTripDoesNotSpuriouslyFail(t *testing.T) {
	cases := map[string]string{
		"basic-scrambled": scrambled,
		"utf8-bom-plus-good": "\xef\xbb\xbfapp: {name: X, mode: workflow, description: \"\"}\n" +
			"kind: app\nversion: \"0.1\"\nworkflow: {graph: {nodes: [], edges: []}}\n",
		"trailing-doc-marker": "app: {name: X, mode: workflow, description: \"\"}\n" +
			"kind: app\nversion: \"0.1\"\nworkflow: {graph: {nodes: [], edges: []}}\n---\n",
		"unicode-u2028-in-string": "app:\n  name: A\n  mode: workflow\n  description: \"a\u2028b\"\n" +
			"kind: app\nversion: \"0.1\"\nworkflow: {graph: {nodes: [], edges: []}}\n",
		"timestamp-looking-id": "app: {name: A, mode: workflow}\nkind: app\nversion: \"0.1\"\n" +
			"workflow:\n  graph:\n    nodes:\n      - {id: 2024-01-01, type: start, data: {title: X}}\n    edges: []\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := Format([]byte(src))
			if err != nil {
				t.Fatalf("round-trip check spuriously rejected %q: %v", name, err)
			}
			// And re-formatting the output must also succeed — tightens the
			// invariant that round-trip is not order-dependent.
			if _, err := Format(out); err != nil {
				t.Fatalf("second Format of %q output failed: %v", name, err)
			}
		})
	}
}

func TestFormat_DataAlphabeticalExceptTitle(t *testing.T) {
	input := `app: {name: X, mode: workflow, description: ""}
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: n
        type: llm
        data:
          z_field: 1
          title: Gen
          a_field: 2
          model: {provider: o, name: gpt-4}
    edges: []
`
	out, err := Format([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	iTitle := strings.Index(s, "title: Gen")
	iA := strings.Index(s, "a_field: 2")
	iModel := strings.Index(s, "model:")
	iZ := strings.Index(s, "z_field: 1")
	if !(iTitle < iA && iA < iModel && iModel < iZ) {
		t.Fatalf("data order wrong: title=%d a=%d model=%d z=%d\n%s", iTitle, iA, iModel, iZ, s)
	}
}
