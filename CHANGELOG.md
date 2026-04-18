# Changelog

All notable changes to this project will be documented in this file. The format is loosely based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] — 2026-04-19

### Added
- `difyctl lint` — 20 rules (DIFY001..DIFY020) over Dify workflow DSL: schema errors, dangling edges, variable-ref resolution, graph cycles (outside iteration bodies), iteration-body invariants, reachability, type-specific shape checks for LLM and Code nodes.
- `difyctl diff` — semantic graph diff producing categorized output (BREAKING / REMOVED / ADDED / CHANGED). Detects added/removed nodes & edges, body-changed, moved-only (position-only), and broken variable references.
- `difyctl fmt` — canonical YAML re-emit with stable top-level / per-node / per-edge key ordering so `git diff` only surfaces meaningful changes. Idempotent.
- `--format json` output on all subcommands.
- Deterministic exit codes: 0 OK, 1 lint/diff issues, 2 usage error, 3 IO/parse error.
- 9 realistic testdata fixtures (4 good, 5 broken — one per top-level failure mode).
- Iterative DFS cycle detection (no recursion; safe on 10k-node graphs).
- `{{#node.var#}}` resolver that understands start-node variables, declared `outputs`/`output_variables`, parameter-extractor `parameters`, question-classifier `class_name`, variable-assigner `items`, and per-type defaults for llm/code/http-request/template-transform/iteration/tool/iteration-start.

### Known limits (v1)
- Export / import from a live Dify server is intentionally deferred to v2.
- `--dify-version` flag is accepted but currently informational; per-version rule gating arrives in v1.1.
