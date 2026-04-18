package fmt

import (
	"bytes"
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
