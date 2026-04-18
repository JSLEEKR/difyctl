# difyctl

[![Go](https://img.shields.io/badge/go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-yellow?style=for-the-badge)](./LICENSE)
[![Status](https://img.shields.io/badge/status-v1.0.0-brightgreen?style=for-the-badge)](./CHANGELOG.md)
[![Tests](https://img.shields.io/badge/tests-140_passing-success?style=for-the-badge)](#testing)
[![Rules](https://img.shields.io/badge/lint_rules-20-blue?style=for-the-badge)](#rule-catalog)
[![Build](https://img.shields.io/badge/build-go_build_clean-success?style=for-the-badge)](#building)

**`difyctl`** is a single-binary Go CLI that lints, diffs, and canonically formats [Dify](https://github.com/langgenius/dify) workflow DSL (YAML) files — so git-driven workflow teams catch schema breakage, variable-path regressions, and silent graph rewrites **before** they ship.

---

## Table of Contents

- [Why This Exists](#why-this-exists)
- [Features](#features)
- [Install](#install)
- [Quick Start](#quick-start)
- [Usage](#usage)
  - [`difyctl lint`](#difyctl-lint)
  - [`difyctl diff`](#difyctl-diff)
  - [`difyctl fmt`](#difyctl-fmt)
- [Rule Catalog](#rule-catalog)
- [Exit Codes](#exit-codes)
- [`git diff` vs `difyctl diff`](#git-diff-vs-difyctl-diff)
- [CI Recipe](#ci-recipe)
- [Design](#design)
- [Building](#building)
- [Testing](#testing)
- [Scope](#scope)
- [Contributing](#contributing)
- [License](#license)

---

## Why This Exists

Dify is a fast-growing LLM-workflow platform (92k+ stars; ranked #2 in the RAG-momentum report with 305 commits/week as of 2026-04-17). Most teams:

1. Author workflows in Dify's web editor.
2. Export a YAML DSL.
3. Check the DSL into Git.
4. Pray `git diff` reviewers catch mistakes.

The problem: **this last step doesn't work**.

- Dify's UI happily saves DSLs with **dangling `{{#node.var#}}` references** — you only discover the broken path when a user hits production. ([dify#19114](https://github.com/langgenius/dify/issues/19114))
- There is **no local linter**, no offline validator.
- `git diff` on YAML is dominated by key-order churn and list-reordering noise — a real "node renamed" looks identical to a trivial edit. ([dify#26158](https://github.com/langgenius/dify/issues/26158), [dify#7966](https://github.com/langgenius/dify/issues/7966))

`difyctl` is a **git-first** linter and semantic differ. Not a replacement for Dify's editor — a safety net for the 30-second gap between "hit Export" and "merge to `main`".

### What it catches that `git diff` can't

| Change                                      | `git diff` signal                     | `difyctl diff` signal                                              |
|---------------------------------------------|---------------------------------------|--------------------------------------------------------------------|
| Node renamed (`llm-1` → `rewrite`)          | ~40 line-level diff lines             | `REMOVED node llm-1` + `ADDED node rewrite` + `BREAKING variable-ref …` if anything downstream still references `{{#llm-1.text#}}` |
| Position-only move (cosmetic drag)          | 2 changed lines                       | `CHANGED node llm-1 moved` (one line)                              |
| Variable output removed from Start          | Maybe 1 line, easy to miss            | `BREAKING variable-ref: {{#start-1.query#}} output 'query' removed`|
| Edge rewired                                | Messy reordered list                  | `ADDED edge`, `REMOVED edge`                                       |

---

## Features

- **`lint`** — 20 deterministic rules (`DIFY001`..`DIFY020`) with stable IDs, severity (error / warning), file path, and line number.
- **`diff`** — semantic graph diff, categorized into `BREAKING`, `REMOVED`, `ADDED`, `CHANGED`. Detects renamed nodes, body-changed vs position-only moves, rewired edges, and — most importantly — **broken variable references** that silently kill runtime flows.
- **`fmt`** — canonical YAML re-emit. Top-level keys ordered (`app` → `kind` → `version` → `workflow`). Nodes sorted by `id`, edges sorted by `id`. Idempotent: `fmt(fmt(x)) == fmt(x)`.
- **`--format json`** — machine-readable output on every subcommand.
- **Exit codes for CI** — 0 OK, 1 issues, 2 usage, 3 IO/parse.
- **No panics on malformed input** — returns structured errors.
- **Iterative DFS cycle detection** — safe on 10k-node graphs.
- **Iteration-body aware** — back-edges inside an `iteration` subgraph do NOT trigger the cycle rule.
- **Zero required external deps beyond `gopkg.in/yaml.v3`** — single binary, no Python/Node runtime.

---

## Install

### From source (requires Go 1.22+)

```bash
go install github.com/JSLEEKR/difyctl/cmd/difyctl@latest
```

### Clone & build

```bash
git clone https://github.com/JSLEEKR/difyctl.git
cd difyctl
go build -o difyctl ./cmd/difyctl
sudo mv difyctl /usr/local/bin/
```

### Verify

```bash
difyctl version
# v1.0.0
```

---

## Quick Start

```bash
# 1. Lint a single workflow file.
difyctl lint my-workflow.yml

# 2. Compare two versions of a workflow.
difyctl diff old-workflow.yml new-workflow.yml

# 3. Normalize keys so git diffs stay readable.
difyctl fmt -w my-workflow.yml
```

Output of `difyctl lint` on a broken file:

```text
my-workflow.yml:17: [DIFY013/error] node 'llm-1' references '{{#ghost.question#}}' but node 'ghost' does not exist
my-workflow.yml:21: [DIFY006/error] duplicate node id 'llm-1'

2 errors, 0 warnings
```

---

## Usage

### `difyctl lint`

```bash
difyctl lint <file.yml> [--format text|json] [--dify-version 1.0]
```

Flags:

- `--format text|json` (default `text`) — output format. `json` produces a structured report with `path`, `findings`, and severity summary.
- `--dify-version` — informational flag (v1.0 accepts it without applying version-gated rules; v1.1 will.)

Text output example:

```text
workflow.yml:23: [DIFY017/error] llm node 'llm-1' is missing 'data.model'
workflow.yml:45: [DIFY012/warning] orphan node 'floater' has no incoming or outgoing edges

1 errors, 1 warnings
```

JSON output example:

```json
{
  "path": "workflow.yml",
  "findings": [
    { "rule": "DIFY017", "severity": "error", "message": "llm node 'llm-1' is missing 'data.model'", "path": "workflow.yml", "line": 23 },
    { "rule": "DIFY012", "severity": "warning", "message": "orphan node 'floater' has no incoming or outgoing edges", "path": "workflow.yml", "line": 45 }
  ],
  "summary": { "error": 1, "warning": 1 }
}
```

### `difyctl diff`

```bash
difyctl diff <a.yml> <b.yml> [--format text|json] [--fail-on-breaking]
```

Flags:

- `--format text|json`
- `--fail-on-breaking` — exit 1 when any `BREAKING` change is detected. Use this in CI for pull-request gating.

Text output categorizes changes:

```text
[BREAKING]
  variable-ref llm-1: reference to {{#start-1.query#}} broken: output 'query' removed from 'start-1'
[REMOVED]
  node floater: type=llm
  edge e-3: llm-1 -> end-1
[ADDED]
  node answer: type=answer
  edge e-4: llm-1 -> answer
[CHANGED]
  app version: '0.1' -> '0.2'
  node llm-1: body-changed
  node start-1: moved

summary: 1 breaking, 2 removed, 2 added, 3 changed
```

JSON output uses a single envelope shape on both success and error so `jq`
filters like `.changes[]` work without branching on the exit code:

```json
{
  "changes": [
    { "category": "BREAKING", "kind": "variable-ref", "id": "llm-1", "detail": "reference to {{#start-1.query#}} broken: output 'query' removed from 'start-1'" },
    { "category": "ADDED",    "kind": "node",         "id": "answer", "detail": "type=answer" }
  ],
  "error": null
}
```

On IO/parse failure the shape is identical except `error` is a string and
`changes` is `[]` — consumers never see empty stdout.

### `difyctl fmt`

```bash
difyctl fmt <file.yml> [-w]
```

Flags:

- `-w` — write canonical form back to the file in-place. Without it, writes to stdout.

Recommended pre-commit usage:

```bash
difyctl fmt -w workflows/*.yml
```

---

## Rule Catalog

| ID        | Severity | Name                        | Description                                                                                 |
|-----------|----------|-----------------------------|---------------------------------------------------------------------------------------------|
| DIFY001   | error    | missing-app                 | Top-level `app` block is empty or missing.                                                  |
| DIFY002   | error    | unknown-app-mode            | `app.mode` not in `{workflow, chatflow, agent-chat}`.                                       |
| DIFY003   | error    | kind-mismatch               | Top-level `kind` is missing or not `app`.                                                   |
| DIFY004   | error    | missing-version             | Top-level `version` is missing.                                                             |
| DIFY005   | error    | missing-node-id             | Node has no `id` field.                                                                     |
| DIFY006   | error    | duplicate-node-id           | Two nodes share the same `id`.                                                              |
| DIFY007   | error    | unknown-node-type           | Node `type` not in known set.                                                               |
| DIFY008   | error    | missing-node-data           | Node has no `data` map.                                                                     |
| DIFY009   | error    | edge-dangling-source        | Edge `source` references non-existent node.                                                 |
| DIFY010   | error    | edge-dangling-target        | Edge `target` references non-existent node.                                                 |
| DIFY011   | error    | duplicate-edge              | Two edges share the same `(source, target, sourceHandle)` tuple.                            |
| DIFY012   | warning  | orphan-node                 | Non-start node with no incoming AND no outgoing edges.                                      |
| DIFY013   | error    | unresolved-var-ref          | `{{#node.var#}}` references a missing node or a variable the source node does not declare. |
| DIFY014   | error    | graph-cycle                 | DFS cycle in the graph (back-edges inside iteration bodies are allowed).                    |
| DIFY015   | error    | missing-start               | Workflow has no `start` node.                                                               |
| DIFY016   | error    | missing-end                 | Workflow has no `end` or `answer` node.                                                     |
| DIFY017   | error    | llm-missing-model           | `type: llm` node has no `data.model`, or `data.model` lacks `provider`/`name`.              |
| DIFY018   | error    | code-missing-code           | `type: code` node is missing `data.code` or `data.code_language`.                           |
| DIFY019   | error    | iteration-missing-start     | `iteration` node does not have exactly one `iteration-start` child (via `parent_id`).       |
| DIFY020   | warning  | unreachable-from-start      | Node is not reachable from any `start` node via forward edge traversal (iteration bodies are silenced). |

### Known node types

`start`, `end`, `answer`, `llm`, `code`, `http-request`, `if-else`, `iteration`, `iteration-start`, `knowledge-retrieval`, `parameter-extractor`, `question-classifier`, `template-transform`, `variable-aggregator`, `variable-assigner`, `tool`. Both hyphen and underscore forms of each name are accepted.

### Default declared outputs per node type

Variable references are resolved when the source node declares an output with that name. In addition to whatever the node explicitly declares under `data.outputs` / `data.output_variables`, difyctl treats the following as built-in defaults:

| Node type              | Default outputs                         |
|------------------------|-----------------------------------------|
| `llm`                  | `text`, `usage`                         |
| `knowledge-retrieval`  | `result`                                |
| `http-request`         | `body`, `status_code`, `headers`        |
| `template-transform`   | `output`                                |
| `iteration`            | `output`                                |
| `iteration-start`      | `item`, `index`                         |
| `variable-aggregator`  | `output`                                |
| `tool`                 | `text`, `files`                         |
| `question-classifier`  | `class_name`                            |

`start` nodes expose whatever they declare under `data.variables[].variable`. `parameter-extractor` exposes `data.parameters[].name`. `variable-assigner` exposes each `data.items[].variable_selector[-1]` (the tail of the assigned path). `code` declares outputs under `data.outputs`.

> Lint (`DIFY013`) and diff (`BREAKING variable-ref`) share a single source of truth for this table — see `internal/varref`. If the two commands ever disagree on whether a given `{{#node.var#}}` resolves, it is a bug in that package.

---

## Exit Codes

| Code | Meaning                                                        |
|------|----------------------------------------------------------------|
| 0    | OK (lint clean, diff clean — or diff had changes but no `--fail-on-breaking`, fmt succeeded) |
| 1    | Lint found at least one `error` finding; or `diff --fail-on-breaking` found `BREAKING` |
| 2    | Usage / argument error (wrong number of args, unknown flag, unknown format) |
| 3    | IO / parse error (file not found, malformed YAML, root not a mapping) |

`warning` findings do NOT fail lint by default.

---

## `git diff` vs `difyctl diff`

Suppose you rename a workflow node from `llm-1` to `rewriter` and update the one `{{#llm-1.text#}}` reference downstream. A typical Dify workflow has ~8 keys per node block.

- `git diff` shows: the id changed in one place, `data` keys may have re-serialized in different order, the edges list may have reordered items, a variable-ref string changed, position fields may differ if the UI moved things. **20-40 diff lines.**
- `difyctl diff` shows:

  ```text
  [REMOVED]
    node llm-1: type=llm
  [ADDED]
    node rewriter: type=llm
  [CHANGED]
    node downstream: body-changed
  ```

  **Four lines. No false positives.**

Miss a downstream reference?

```text
[BREAKING]
  variable-ref end-1: reference to {{#llm-1.text#}} broken: node 'llm-1' removed
```

You catch it in CI, not at 3am.

---

## CI Recipe

### GitHub Actions

```yaml
name: Workflow Lint

on:
  pull_request:
    paths:
      - 'workflows/**.yml'

jobs:
  difyctl:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }

      - name: Install difyctl
        run: go install github.com/JSLEEKR/difyctl/cmd/difyctl@latest

      - name: Lint all workflows
        run: |
          set -e
          for f in workflows/*.yml; do
            difyctl lint "$f"
          done

      - name: Semantic diff vs base branch
        run: |
          git fetch origin "${{ github.base_ref }}"
          for f in workflows/*.yml; do
            if git show "origin/${{ github.base_ref }}:$f" > /tmp/old.yml 2>/dev/null; then
              difyctl diff --fail-on-breaking /tmp/old.yml "$f"
            fi
          done
```

### Pre-commit hook (`.git/hooks/pre-commit`)

```bash
#!/bin/sh
changed=$(git diff --cached --name-only --diff-filter=ACM | grep '^workflows/.*\.yml$' || true)
[ -z "$changed" ] && exit 0
for f in $changed; do
  difyctl fmt -w "$f"
  difyctl lint "$f" || exit 1
  git add "$f"
done
```

---

## Design

See [`docs/superpowers/specs/2026-04-19-difyctl-design.md`](../../docs/superpowers/specs/2026-04-19-difyctl-design.md) for the full design spec: data model, rule engine, diff algorithm, canonical-format algorithm, exit-code contract, test strategy.

High-level notes:

- **One binary, stdlib `flag`.** Cobra would have added 3 MB of deps and saved about six lines of code. Not worth it.
- **`yaml.v3` with `*yaml.Node`.** Parsed structs for fast field access, raw node tree retained for accurate line numbers in lint output and for canonical-order formatting.
- **Rule engine is a slice of `Rule` interface values**, not reflection. Each rule is one file (`rule_cycle.go`, `rule_varref.go`, etc.) — trivial to add #21.
- **Iterative DFS** for cycle detection. The 305-commits-per-week upstream repo could ship a 10k-node workflow and we still don't stack-overflow.
- **Generator ≠ Evaluator.** This package was built in Phase 2 of the daily-challenge pipeline and will be audited by an independent evaluator in Phase 3.

---

## Building

```bash
go build ./...        # build everything (cmd + all internal packages)
go vet ./...          # static checks — must be clean
go test ./...         # run the full test suite
```

Binaries land at `./difyctl` if you build with:

```bash
go build -o difyctl ./cmd/difyctl
```

Cross-compile:

```bash
GOOS=linux   GOARCH=amd64  go build -o dist/difyctl-linux-amd64   ./cmd/difyctl
GOOS=darwin  GOARCH=arm64  go build -o dist/difyctl-darwin-arm64  ./cmd/difyctl
GOOS=windows GOARCH=amd64  go build -o dist/difyctl-windows-amd64.exe ./cmd/difyctl
```

---

## Testing

```
$ go test ./...
ok   github.com/JSLEEKR/difyctl/cmd/difyctl         0.016s
ok   github.com/JSLEEKR/difyctl/internal/diff       0.005s
ok   github.com/JSLEEKR/difyctl/internal/fmt        0.004s
ok   github.com/JSLEEKR/difyctl/internal/lint       0.023s
ok   github.com/JSLEEKR/difyctl/internal/model      0.003s
ok   github.com/JSLEEKR/difyctl/internal/parse      0.004s
ok   github.com/JSLEEKR/difyctl/internal/varref     0.002s
```

- **140 tests** across 7 packages.
- Rule tests are table-driven — one test file per rule, each exercising the happy path and at least one failure case.
- `internal/fmt` has an idempotence test (`fmt(fmt(x)) == fmt(x)`) that will catch ANY accidental key re-ordering drift.
- `internal/parse` has a "no-panic on garbage bytes" test covering binary noise and malformed YAML.

---

## Scope

### v1.0 — shipped ✅

- `lint`, `diff`, `fmt`, `version`.
- 20 rules, JSON output, CI-ready exit codes.
- No HTTP, no side effects, no writing to remote Dify servers.

### v1.1 — planned

- Version-gated rules (`--dify-version` stops being informational).
- Extended node-type coverage as upstream Dify adds types.
- Suppression comments (`# difyctl:disable=DIFY012`).

### v2.0 — planned

- `difyctl export --api-key … --host …` — bulk pull DSLs from a live Dify instance ([dify#26158](https://github.com/langgenius/dify/issues/26158)).
- `difyctl import` — push validated DSLs back.
- Version migration (`v1.0 → v1.1`).

---

## Contributing

Bug reports and PRs are welcome. Please:

1. Open an issue first for anything larger than a typo.
2. Include a failing test case.
3. Run `go test ./... && go vet ./... && gofmt -l .` before committing.

If you're adding a new lint rule:

1. Add a file `internal/lint/rule_<name>.go` with a single type implementing `Rule`.
2. Register it in `internal/lint/rules.go` in ID order.
3. Add a table-driven test in `internal/lint/rule_<name>_test.go`.
4. Document it in the Rule Catalog section of this README.

---

## License

MIT © 2026 JSLEEKR. See [LICENSE](./LICENSE).
