# Tasks: Deterministic Verification Evidence

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines (5 slices) | 1285-2070 (midpoint ~1680) |
| 800-line budget risk | Low (post-split; largest slice tops out ~680, 120-line margin) |
| Chained PRs recommended | Yes |
| Suggested split | PR1 → PR2 → PR3 → PR4 → PR5 (feature-branch-chain) |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
800-line budget risk: Low

## Slice 2 Re-forecast (mandatory — carried risk from every prior phase)

Original forecast: `deterministic-check-runner-core` (R-005..R-008) at 500-780 lines against 800 — a 20-line margin.

**Bottom-up re-forecast, grounded in `tools/entry-contract-validator` (verified: `main.go` 354 lines, `main_test.go` 438 lines, `testdata/` 2 fixtures, `go.mod` ~13 lines = ~943 total).** The new runner is materially more complex than that precedent: 4 distinct checks each with a `parse` function and a `failed(exit,count)` predicate, a `classify()`/`selectOutcome()` decision layer, top-N/bounded-summary rendering, `capPayload`, and golden-file coverage. Itemizing against the design's `Interfaces/Contracts` and `Testing Strategy` tables (registry+classify+selectOutcome+module-discovery+row-emit in `main.go`; 4× parse funcs + summary renderer + `capPayload` also in `main.go`; classify/selectOutcome/module-discovery/row tests + 4× parse-func table tests + summary 0/1/N/N+1 + banned-literals + `capPayload` tests in `main_test.go`; tool-output fixtures + stdout goldens in `testdata/`) yields an unsplit total of roughly **1000-1100 lines** — over the 800 budget.

**Decision: split along the pre-agreed seam.** No approval needed per the design's own D-note.

| New id | Scope | Est. lines |
|---|---|---|
| `runner-registry-and-rows` | `check`/`result` structs, hardcoded registry, `classify()`, `selectOutcome()`, module discovery, row emission, CI job scaffold | 380-560 |
| `runner-summary-rendering` | 4× `parse` funcs, `failed()` predicates, summary renderer (count/top-N/path, 200-char cap), `capPayload`, banned-literal guard, stdout goldens | 420-680 |

Both land comfortably under 800 (120-180 line margin at the high end vs. the original 20-line margin). Chain renumbers to 5 slices; `runner-mode-separation` and `capture-evidence-wiring` are unaffected in scope (their own forecasts, 180-340 and 240-420, already had healthy margin and are unchanged).

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | `severity-policy-and-ci-gates` — CRITICAL severity, staticcheck/deadcode CI gates, `tools` test layer | PR 1 (base: tracker) | `rg` content assertions (see 1.1/1.3/1.5) | N/A — markdown/YAML policy edit, no runtime path | Revert `strict-tdd-verify.md`, `SKILL.md` ordering line, `ci.yml`, `openspec/config.yaml` |
| 2 | `runner-registry-and-rows` — registry, classify, selectOutcome, module discovery, rows | PR 2 (base: PR1 branch) | `cd tools/deterministic-check-runner && go test ./... -run 'TestClassify\|TestSelectOutcome\|TestModuleDiscovery\|TestRowEmission'` | `go run . check` against this repo's own `go.mod` set | Delete `tools/deterministic-check-runner/` module |
| 3 | `runner-summary-rendering` — parse funcs, bounded summaries, capPayload, goldens | PR 3 (base: PR2 branch) | `cd tools/deterministic-check-runner && go test ./... -cover` | `go run . check --top-n 5` against a fixture with 200+ synthetic findings | Revert additive files only (parse funcs, renderer, goldens); registry/rows from PR2 stay intact |
| 4 | `runner-mode-separation` — normalize/check split, byte-neutrality | PR 4 (base: PR3 branch) | `cd tools/deterministic-check-runner && go test ./...` (includes `-short`-skippable integration test) | `check` invoked twice on a dirty `t.TempDir()` git fixture with a 0755 file; diff `git status --porcelain` before/after | Revert subcommand dispatch + out-dir guard; core runner from PR2/3 stays functional single-mode |
| 5 | `capture-evidence-wiring` — CLI dispatch, capture-evidence piping, outcome mapping, R-001/R-019 gates | PR 5 (base: PR4 branch) | `bash -n bin/labdrian-overlay && shellcheck bin/labdrian-overlay`; `cd tools/deterministic-check-runner && go test ./...` | `bin/labdrian-overlay deterministic-checks check` end-to-end on this repo | Revert `cmd_deterministic_checks`, dispatch line, help text, `SKILL.md` wiring |

---

## Phase 1: severity-policy-and-ci-gates (R-001, R-002 policy, R-003, R-004, R-017, R-018)

- [x] 1.1 RED: record failing `rg` assertions against current text — no CRITICAL scoping at `strict-tdd-verify.md:121/:127`, `:266` not scoped to coverage/dead-code only.
- [x] 1.2 GREEN: edit `skills/sdd-verify/strict-tdd-verify.md` :121/:127/:266 — deterministic binary checks (`go vet`, `gofmt`, `staticcheck`) CRITICAL; scope :266 to coverage/quality metrics only. Rerun `rg` — pass. (Green `go test` is BANNED as evidence here.)
- [x] 1.3 RED: record failing `rg`/grep assertion — `.github/workflows/ci.yml` has no `staticcheck ./...` step in `test-engine`/`test-tui`.
- [x] 1.4 GREEN: add blocking `staticcheck ./...` step to both jobs; add `go run golang.org/x/tools/cmd/deadcode ./...` with `continue-on-error: true` (R-017, R-018). Rerun assertion — pass.
- [x] 1.5 RED: record failing assertion — `openspec/config.yaml` has no `name: tools` testing layer.
- [x] 1.6 GREEN: add 4th `testing.layers` entry `tools` with `for m in tools/*/go.mod; do (cd "$(dirname "$m")" && go test ./...) || exit 1; done`; append the same loop to both `apply.test_command` and `verify.test_command` (D1). Rerun — pass.
- [x] 1.7 Verify R-001: `git diff main -- bin/overlay` returns empty; confirm no vendored/upstream path in the branch diff.

## Phase 2: runner-registry-and-rows (R-005, R-006 core, R-002 test half)

- [ ] 2.1 RED: create `tools/deterministic-check-runner/{go.mod,main_test.go}` with failing `TestModuleShape` (go.mod/main.go/testdata parity with `entry-contract-validator`), `TestCheckRegistry` (exactly 4 checks, each declares `deterministic`+`blocking`), `TestClassify` (a check without `deterministic=true` never classifies CRITICAL regardless of `blocking` — R-002).
- [ ] 2.2 GREEN: create `main.go` — `check` struct, hardcoded `[]check` registry (gofmt/go vet/staticcheck: deterministic+blocking; deadcode: not blocking), `classify(c check) bool` = `blocking && deterministic`, the sole enforcement point. Tests pass.
- [ ] 2.3 RED: add `TestSelectOutcomePrecedence` (6 cases: BLOCKING-set tool unavailable alone→3; deadcode unavailable alone→0; deadcode unavailable + blocking failed→1; runner error→3; non-blocking red alone→0; all clean→0).
- [ ] 2.4 GREEN: implement `result` struct + `selectOutcome([]result) int` per D4. Tests pass.
- [ ] 2.5 RED: add `TestModuleDiscovery` — sorted `go.mod` walk, deterministic order, correct when invoked from a subdirectory (threat matrix: git repository selection).
- [ ] 2.6 GREEN: implement discovery rooted at `CALLER_CWD`-normalized cwd. Test passes.
- [ ] 2.7 RED: add `TestRowEmission` — `%s | %d | %s\n` format, one row per configured check, stable declared order (R-006).
- [ ] 2.8 GREEN: wire `run(args, stdout, stderr) int` executing each check per module, emitting rows in registry order. Test passes.
- [ ] 2.9 Add CI job `test-deterministic-check-runner` to `.github/workflows/ci.yml` mirroring `test-entry-contract-validator` (`go test ./... -cover`) — R-005 acceptance.
- [ ] 2.10 Run `cd tools/deterministic-check-runner && go test ./... -cover` — GREEN.

## Phase 3: runner-summary-rendering (R-006 remainder, R-007, R-008)

- [ ] 3.1 RED: add table-driven `TestParseGofmt`/`TestParseGoVet`/`TestParseStaticcheck`/`TestParseDeadcode` against `testdata/` fixtures; assert `gofmt -l` non-empty output at exit 0 still yields `count>0` via `failed(exit,count)` (D3).
- [ ] 3.2 GREEN: implement the 4 `parse` funcs + per-check `failed(exit,count)` predicates. Tests pass.
- [ ] 3.3 RED: add `TestSummaryRendering` over 0/1/N/N+1 findings — `count=N; top: …; full: <path>`, 200-char excerpt cap, `--top-n` honored (default 5), zero renders literal `0`.
- [ ] 3.4 GREEN: implement renderer + `--top-n` flag + stable out-dir `${TMPDIR}/labdrian-deterministic-checks/<tool>.log` (D6). Test passes.
- [ ] 3.5 RED: add `TestBannedLiterals` scanning every emitted summary for PASS/PASSED/SUCCESS/N/A/NA/NONE/TODO/TBD/PLACEHOLDER as a standalone value, including the zero-findings case.
- [ ] 3.6 GREEN: confirm/guard renderer never emits a banned literal standalone. Test passes.
- [ ] 3.7 RED: add `TestCapPayload` — payload over 4 MiB truncates to counts+paths, never rejects.
- [ ] 3.8 GREEN: implement `capPayload` (D6). Test passes.
- [ ] 3.9 RED: add golden test for the full stdout row block via the `-update` path.
- [ ] 3.10 GREEN: generate goldens with `-update`, rerun without — byte-identical.
- [ ] 3.11 Run `cd tools/deterministic-check-runner && go test ./... -cover` — GREEN.

## Phase 4: runner-mode-separation (R-009, R-010)

- [ ] 4.1 RED: add `TestSubcommandUsage` — no subcommand → non-zero exit, usage names both `normalize` and `check`.
- [ ] 4.2 GREEN: add subcommand dispatch; wire `checkArgv`/`normalizeArgv` per check. Test passes.
- [ ] 4.3 RED: add `TestCheckNeverMutates` — `checkArgv` allowlist contains no fixer flags.
- [ ] 4.4 GREEN: enforce no fixer flag in any `checkArgv` (no `gofmt -w`, no `staticcheck -fix`). Test passes.
- [ ] 4.5 RED: add `TestCheckByteNeutral` (integration, skip in `-short`) — `t.TempDir()` git fixture, dirty tree + a 0755-mode file; snapshot `git status --porcelain` + per-file `(mode, size, sha256)` before/after `check`.
- [ ] 4.6 GREEN: guarantee `check` runs only read-only argv. Integration test passes.
- [ ] 4.7 RED: add `TestOutDirGuard` — `check --out-dir <path inside repo root>` → usage error (threat matrix: git repository selection).
- [ ] 4.8 GREEN: implement the out-dir guard. Test passes.
- [ ] 4.9 RED: add `TestMissingToolStubbedPATH` (integration) — stub `PATH` so a BLOCKING-set tool is unavailable (assert row `unavailable=true`, `selectOutcome`→3); separately stub only `deadcode` unavailable (assert WARNING row, `selectOutcome` unaffected when all else green).
- [ ] 4.10 GREEN: implement exit-127 unavailable-tool detection feeding `result.unavailable`; confirm precedence holds. Test passes.
- [ ] 4.11 Run `cd tools/deterministic-check-runner && go test ./...` (full suite) — GREEN.

## Phase 5: capture-evidence-wiring (R-011, R-012, R-013, R-014, R-015, R-016, R-019)

- [ ] 5.1 RED: record failing `rg` assertion — `skills/sdd-verify/SKILL.md` does not yet declare `normalize` pre-`review start` / `check` as sole post-freeze step.
- [ ] 5.2 GREEN: edit `SKILL.md` to declare the ordering (R-011). Rerun — pass.
- [ ] 5.3 RED: record failing dispatch-smoke assertion — `bin/labdrian-overlay` has no `deterministic-checks` subcommand.
- [ ] 5.4 GREEN: add `cmd_deterministic_checks()` mirroring `cmd_validate_entry_contract` (lines 1559-1615): two exit-3 guards (missing runner source / missing `go`), `CALLER_CWD` normalization, temp-binary build, exit propagation; add dispatch case (mirrors line 1651) and help entry (R-012). `bash -n` + `shellcheck` pass.
- [ ] 5.5 RED: record failing smoke assertion — missing-tool and missing-`go` guards do not yet exit 3 with explicit stderr.
- [ ] 5.6 GREEN: confirm both guards fire (built in 5.4); add the smoke-test script. Pass.
- [ ] 5.7 RED: add payload-size-boundary unit test on the evidence-piping contract — empty run still satisfies `minLength 1`; payload over 4194304 bytes truncates, never rejected (R-013).
- [ ] 5.8 GREEN: document `gentle-ai review capture-evidence --input -` piping the runner's `check` stdout in `SKILL.md`; test passes.
- [ ] 5.9 RED: add unit tests asserting exit-code→`--outcome` mapping is mechanical: 0→`passed`, 1→`verification_failed`, 3→`procedural_tooling_failed`, including both precedence scenarios (BLOCKING-set unavailable wins over a failing check; missing WARNING-only tool never masks/escalates).
- [ ] 5.10 GREEN: implement/document the mapping; add `command -v labdrian-overlay` guard — absent CLI → `procedural_tooling_failed`, never silent pass, never `verification_failed` (D8). Tests pass.
- [ ] 5.11 Verify R-019: confirm no new file under `skills/` (edits only touch `strict-tdd-verify.md`/`SKILL.md`, rows 22/41). If a new `skills/` file is ever added, add its `overlay.manifest` row and run `skills validate` — exit 0.
- [ ] 5.12 Re-verify R-001: `git diff main -- bin/overlay` still empty.
- [ ] 5.13 Run full regression: `cd engine && go test ./...`, `cd tui && go test ./...`, `cd tools/deterministic-check-runner && go test ./... -cover`; confirm green CI (staticcheck gate, deadcode visible+green, runner job).
