package fmt_test

// Parity tests for Cycle L's architectural change. fmt now routes its input
// validation through parse.Validate — the same path lint and diff use — so any
// shape lint REJECTS, fmt must now also REJECT. Cycles E/G/H/I/K each found
// one such asymmetry and patched it with a per-class gate; this file is the
// regression suite that proves the architectural fix kills the WHOLE class.
//
// Why an _test package: this file imports both internal/fmt and internal/parse
// to compare their behaviour side-by-side. Living in package fmt_test (not
// package fmt) keeps the import graph clean and exercises the public API.

import (
	"errors"
	"strings"
	"testing"

	difyfmt "github.com/JSLEEKR/difyctl/internal/fmt"
	"github.com/JSLEEKR/difyctl/internal/parse"
)

// TestParity_FmtRejectsWhatLintRejects locks the architectural invariant that
// every shape parse.Validate rejects, fmt.Format ALSO rejects. If a future
// strict-decode rule is added to parse, this test will fail loudly the first
// time someone forgets to also patch fmt — which is exactly the regression
// Cycles E/G/H/I/K kept being bitten by.
func TestParity_FmtRejectsWhatLintRejects(t *testing.T) {
	cases := map[string]string{
		// Cycle L discovery: complex YAML keys (`? [a, b]: c`) inside a node
		// data field. parse strict-decodes data into map[string]any, which
		// rejects sequence-as-key. fmt previously took the *yaml.Node path
		// which is structure-only and accepted these.
		"complex-key-in-node-data": `app:
  name: A
  mode: workflow
  description: ""
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: s
        type: start
        data:
          ? [a, b]
          : c
    edges: []
`,
		// Cycle L discovery: node.data as a sequence rather than a mapping.
		// parse rejects (`cannot unmarshal !!seq into map[string]interface {}`),
		// fmt previously accepted and re-emitted the sequence verbatim.
		"sequence-as-node-data": `app:
  name: A
  mode: workflow
  description: ""
kind: app
version: "0.1"
workflow:
  graph:
    nodes:
      - id: s
        type: start
        data:
          - 1
          - 2
    edges: []
`,
		// Cycle L discovery: app block as a bare scalar. parse rejects
		// (`cannot unmarshal !!str into model.App`), fmt previously accepted.
		"scalar-as-app-block": `app: not-a-map
kind: app
version: "0.1"
workflow:
  graph: {nodes: [], edges: []}
`,
		// Cycle L discovery: workflow as a sequence. parse rejects
		// (`cannot unmarshal !!seq into model.GraphWrapper`), fmt accepted.
		"sequence-as-workflow": `app:
  name: A
  mode: workflow
  description: ""
kind: app
version: "0.1"
workflow:
  - 1
  - 2
`,
		// Cycle L discovery: nodes is a scalar. parse rejects
		// (`cannot unmarshal !!str into []model.Node`), fmt accepted.
		"scalar-as-nodes": `app:
  name: A
  mode: workflow
  description: ""
kind: app
version: "0.1"
workflow:
  graph:
    nodes: not-a-seq
    edges: []
`,
		// Cycle L discovery: version is a mapping. parse rejects
		// (`cannot unmarshal !!map into string`), fmt accepted.
		"mapping-as-version": `app:
  name: A
  mode: workflow
  description: ""
kind: app
version:
  major: 0
  minor: 1
workflow:
  graph: {nodes: [], edges: []}
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			parseErr := parse.Validate([]byte(src))
			_, fmtErr := difyfmt.Format([]byte(src))
			if parseErr == nil {
				t.Fatalf("test bug: parse accepted %q — pick a different case", name)
			}
			if fmtErr == nil {
				t.Fatalf("PARITY VIOLATION: parse rejects %q with %v but fmt accepted it — fmt -w would silently rewrite a file lint refuses", name, parseErr)
			}
		})
	}
}

// TestParity_KnownLinterErrorsKeepFmtSentinels locks that the well-known fmt
// sentinel errors (ErrMultiDoc, ErrDuplicateKeys, ErrEmpty, ErrNotMapping)
// still surface even though Format now delegates to parse.Validate. Without
// this test, a future cleanup that simplified the error mapping could silently
// regress callers who use errors.Is on the documented sentinels.
func TestParity_KnownLinterErrorsKeepFmtSentinels(t *testing.T) {
	type want struct {
		src      string
		contains string
	}
	cases := map[string]want{
		"multi-doc": {
			src:      "app: {name: A, mode: workflow}\n---\napp: {name: B, mode: workflow}\n",
			contains: "multi-document",
		},
		"dup-key": {
			src:      "app:\n  name: A\n  name: B\n  mode: workflow\nkind: app\nversion: \"0.1\"\nworkflow: {graph: {nodes: [], edges: []}}\n",
			contains: "already defined at line",
		},
		"non-mapping": {
			src:      "42\n",
			contains: "root must be a mapping",
		},
		"empty": {
			src:      "",
			contains: "empty",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := difyfmt.Format([]byte(tc.src))
			if err == nil {
				t.Fatalf("want error for %q, got nil", name)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("want error containing %q, got %v", tc.contains, err)
			}
		})
	}
}

// TestParity_SentinelErrorsUnwrapViaErrorsIs locks the stronger contract that
// callers can use errors.Is on the fmt sentinel for every documented class.
// Cycle L refactored input validation through parse.Validate, and the original
// mapping table had a dead branch (`errors.Is(vErr, parse.ErrMultiDoc)` — which
// is always false because parse wraps ErrParse via %w and formats ErrMultiDoc
// via %v, so it is NOT in the error chain). The multi-doc case fell through to
// the default branch, returning the raw parse error instead of the documented
// fmt.ErrMultiDoc sentinel — a silent contract break invisible to the old
// string-contains test. This test asserts the contract directly via errors.Is
// so any future regression of the mapping is caught immediately.
func TestParity_SentinelErrorsUnwrapViaErrorsIs(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		sentinel error
	}{
		{
			name:     "multi-doc",
			src:      "app: {name: A, mode: workflow}\n---\napp: {name: B, mode: workflow}\n",
			sentinel: difyfmt.ErrMultiDoc,
		},
		{
			name:     "dup-key",
			src:      "app:\n  name: A\n  name: B\n  mode: workflow\nkind: app\nversion: \"0.1\"\nworkflow: {graph: {nodes: [], edges: []}}\n",
			sentinel: difyfmt.ErrDuplicateKeys,
		},
		{
			name:     "non-mapping",
			src:      "42\n",
			sentinel: difyfmt.ErrNotMapping,
		},
		{
			name:     "empty",
			src:      "",
			sentinel: difyfmt.ErrEmpty,
		},
		{
			name:     "utf16-bom",
			src:      string([]byte{0xFF, 0xFE, 0x61, 0x00}),
			sentinel: difyfmt.ErrEncoding,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := difyfmt.Format([]byte(tc.src))
			if err == nil {
				t.Fatalf("want error for %q, got nil", tc.name)
			}
			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("errors.Is(err, %v) = false — callers using errors.Is on the documented sentinel are broken. Got err=%v (type %T)", tc.sentinel, err, err)
			}
		})
	}
}
