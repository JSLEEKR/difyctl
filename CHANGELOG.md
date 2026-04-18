# Changelog

All notable changes to this project will be documented in this file. The format is loosely based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.1] — 2026-04-19 (Cycle C / D / E / F / G / H / I / J hardening)

### Added (Cycle J — architectural round-trip self-check)
- **`fmt` now runs a round-trip re-parse on its own output before returning.** Cycles E (UTF-16 ASCII-stripping), H (multi-doc truncation), and I (anchor/alias reorder) were each a per-class fix for the same architectural gap: `Format` returned `(bytes, nil)` even when the bytes were not a valid Dify DSL on their face, and `fmt -w` then persisted the corrupted bytes to disk. Each Cycle added a new pre-check; the next surprise yaml.v3 shape would have required Cycle K. The new `ErrRoundTrip` is a generic backstop: after canonical re-emit, `Format` re-parses its own output via `yaml.Unmarshal`; on failure it returns an error and the bytes are never written. The check does NOT replace the per-class pre-checks (which give clearer diagnostics for the common shapes) — it is the safety net for whatever shape comes next. Cost: one extra `yaml.Unmarshal` per `Format` call; bounded by the 32 MiB file cap. Test count: 224 → 231.

### Fixed (Cycle I — anchor/alias data-loss fix)
- **HIGH: `fmt -w` on a YAML file using anchors (`&name`) paired with aliases (`*name`) no longer silently corrupts the file.** Canonical reordering — top-level key sort, node-id sort, edge-id sort — could move an anchor AFTER its alias in the emitted stream, producing output that yaml.v3 fails to re-parse (`unknown anchor 'name' referenced`). Before the fix, `Format` returned the invalid bytes with `err == nil`, `fmt -w` wrote them to disk, and a follow-up `lint` on the rewritten file exited 3 — leaving the user with a corrupted source file. Same class of silent-data-loss bug as Cycles E (UTF-16 ASCII-stripping) and H (multi-doc truncation). We now detect any anchor/alias in the parsed node tree and refuse with `format: YAML anchors/aliases … not supported`. `lint` and `diff` are unchanged — they only read, never re-emit, so there is no corruption risk. Dify's DSL exporter does not emit anchors; hand-crafted files using `<<: *base` merges must be de-anchored before `fmt -w`.

### Fixed (Cycle H — multi-document YAML data-loss fix)
- **HIGH: `fmt -w` on a multi-document YAML file no longer silently truncates the file.** yaml.v3's `Unmarshal` consumes only the first document of a `---`-separated stream and drops the rest; before the fix, `difyctl fmt -w multi.yml` would quietly rewrite the user's file to just doc #1 on disk — the exact class of silent-data-loss bug Cycle E fixed for UTF-16 inputs. Multi-doc streams now cause lint / diff / fmt to exit 3 with `multi-document YAML not supported (Dify DSL is single-document)`, and the file on disk is untouched.
- **MED: `lint` / `diff` no longer silently ignore docs #2..N of a multi-document file.** Previously they ruled against doc #1 only, giving a misleading "clean" signal for files whose later documents violated rules. A trailing bare `---\n` (a common editor artifact following a single doc) is still accepted — only substantive additional documents trigger the reject.

### Fixed (Cycle G — cross-command parity cascade fix)
- **HIGH: `lint` and `diff` now refuse UTF-16 / UTF-32 BOM input.** Cycle E fixed the same class of bug in `fmt` (yaml.v3 silently ASCII-strips non-UTF-8 input), but the identical decoder backs `lint` and `diff`. Both commands were happily running rules against the ASCII subset of a UTF-16 file and reporting confident garbage. All three subcommands now route file reads through `internal/fileio.ReadCapped` so future input guards land in one place.
- **MED: `fmt` now rejects non-mapping document roots** (`42`, `true`, `foo`, top-level sequences `- a\n- b`) with `format: root must be a mapping`. `lint` and `diff` already rejected such input via `parse.ParseBytes`; before the fix, a user running `difyctl fmt -w file.yml` on a file that lint/diff rejected could silently get the file "formatted" in place. Now all three agree.
- **LOW: Cleaned up double-wrapped IO error messages.** Previously a directory argument to `lint`/`diff` produced `io error: read /tmp/x: read /tmp/x: is a directory`; now it is `io error: /tmp/x: is a directory`, matching the Cycle A intent for the "open" branch.

### Refactor (Cycle G)
- Extracted `internal/fileio` package. All file reads (`MaxFileSize`, directory rejection, BOM detection) live here. `parse.LoadFile`, `cmd/difyctl/fmt.go`, and `internal/fmt.Format` delegate to it. Prevents the recurring "feature X added to subcommand Y but not Z" cascade that Cycles A (lint cap), F (fmt cap), and G (BOM in lint/diff) all had to patch separately.

### Fixed (Cycle F)
- `fmt` now enforces the 32 MiB file-size cap that Cycle A added to `lint`/`diff`. Previously `difyctl fmt` used `os.ReadFile` directly, which happily slurped files of arbitrary size — a hostile 40 MiB YAML would OOM the process. The cap now applies to all three subcommands uniformly.
- `fmt` on a directory argument now fails with a clean `read X: is a directory` error (exit 3) instead of letting `io.ReadAll` slurp raw directory bytes into `yaml.Unmarshal`.
- `fmt` node-sort is now transitive for mixed id / no-id inputs. The previous `Less` fell back to original-index comparison whenever either side had an empty id, which is non-transitive (e.g. `[c, "", a, b]` stayed in insertion order). Id-bearing nodes now sort alphabetically; empty-id nodes keep their relative insertion order after.


### Changed
- `diff --format json` now emits a uniform `{changes: [...], error: null | "..."}` envelope on BOTH success and error paths. Previously success emitted a bare JSON array while error emitted an object, so `jq` filters like `.changes[]` could not unify the two without branching on exit code. Consumers that parsed `.changes[]` already keep working; consumers that assumed a top-level array must switch to `.changes[]`.
- `lint --format json` envelope now always includes `"error": null` on success, mirroring the diff envelope. `jq` filters that branch on `.error` behave uniformly across success and IO/parse-error paths. `findings` is always a JSON array (never `null`), so `.findings[]` cannot explode with "Cannot iterate over null".
- `DIFY006` (duplicate-node-id) now reports the first-defined source line: `duplicate node id 'llm-1' (first defined at line 14)`. Makes renames and copy-paste bugs easier to locate.
- `DIFY013` (unresolved-var-ref) now dedupes on `(referrer, target, var)` — a single node mentioning `{{#ghost.q#}}` N times yields one finding, not N.

### Fixed
- `fmt` no longer corrupts UTF-16 / UTF-32 input. yaml.v3 silently ASCII-strips such files and returns a bogus scalar; `fmt -w` would then rewrite the file with the stripped bytes (catastrophic data loss). We now detect the four common non-UTF-8 BOMs (UTF-16 LE/BE, UTF-32 LE/BE) up-front and error out. UTF-8 BOM (EF BB BF) continues to work.
- `fmt` on a file that is only YAML comments (e.g. `# nothing here\n`) now errors out instead of silently clobbering the file with the literal string `null\n`. Previously slipped past the Cycle C guards because yaml.v3 parses comment-only input to a DocumentNode with zero content.
- `fmt -w link.yml` on a symlink no longer replaces the symlink with a regular file. We now `EvalSymlinks` to resolve the target, then atomically rename over the TARGET — the symlink stays a symlink, just pointing at the newly-rewritten bytes.
- `fmt` on an empty or whitespace-only document now errors out (`format: empty document`) instead of silently clobbering the file with the literal string `null\n`. Likewise for inputs that decode to a bare null scalar (`~`, `null`).

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
