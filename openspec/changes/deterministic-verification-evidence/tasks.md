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

## Slice 2 Ledger Re-split — `runner-registry-and-rows` (mandatory — 200-line-per-objective ledger cap)

**Calibration data point (only one so far):** slice 1's runtime attempt ledger recorded **67** changed lines while its commit range carried 668 insertions and its code-only diff (`git diff --stat` on the 3 edited files) was **33** insertions+deletions. Planning artifacts (openspec docs) were evidently excluded from the ledger count, but the exact counting rule beyond that is undocumented and unverified for net-new Go source files. Do not assume `git diff --stat` on new files equals the ledger count — treat every estimate below as a rough guide, not a guarantee, and leave real margin.

`runner-registry-and-rows` (380-560 raw-line estimate) does not fit one 200-line ledger objective. Split into 4 ledger objectives, ordered, each depending only on the prior one, all landing on PR2 (base: PR1 branch) as separate commits/attempts within that PR:

| Ledger objective | Scope | Depends on | Est. raw lines | Margin to 200 |
|---|---|---|---|---|
| `runner-scaffold-and-discovery` | `go.mod`, minimal `main.go`/`run()` stub, `discoverModules()` (sorted `go.mod` walk, `CALLER_CWD`-normalized, subdir-safe) | — | 130-180 | 20-70 |
| `runner-registry-and-classify` | `check` struct (`name`/`deterministic`/`blocking`/`checkArgv`), hardcoded 4-entry registry, `classify()` | `runner-scaffold-and-discovery` | 90-130 | 70-110 |
| `runner-result-and-outcome` | `result` struct, `selectOutcome()` (6-case precedence table) | `runner-registry-and-classify` | 100-150 | 50-100 |
| `runner-row-emission-and-ci-job` | `emitRows()`, full `run()` wiring (discovery × registry × exec × classify/selectOutcome × emitRows) with placeholder (non-banned-literal) summary text, CI job `test-deterministic-check-runner` | `runner-scaffold-and-discovery`, `runner-registry-and-classify`, `runner-result-and-outcome` | 115-160 | 40-85 |

Every objective targets the 120-150 line zone requested for real margin; the low end of each range still clears 200 with comfortable room even if the ledger over-counts relative to raw diff (as it did in slice 1, ~2x). `check.normalizeArgv` and `check.parse`/`check.failed` are intentionally NOT added yet — they land additively in `runner-mode-separation` (Phase 4) and `runner-summary-rendering` (Phase 3) respectively, so this split introduces no rework, only field growth on an existing struct. Row summaries emitted by `runner-row-emission-and-ci-job` are placeholder text (e.g. `exit=<code>`) — real bounded-summary text (`count=N; top: …`) is Phase 3's job; the placeholder still satisfies the banned-literal guard by construction (no literal PASS/N/A/etc. ever appears) even though the guard itself isn't tested until Phase 3.

**Forward assessment (executed) — `runner-summary-rendering` (Phase 3, PR3) was re-split before implementation started.** See "Phase 3 Ledger Re-split" below for the executed 6-objective split (grown from the originally-sketched 3-objective seam to absorb two audit findings and the invocation-strategy gap discovered while running Phase 2, obs #2714).

## Phase 3 Ledger Re-split — `runner-summary-rendering` (mandatory — 200-line-per-objective ledger cap)

**Calibration data — four completed Phase 2 objectives (obs #2713/#2714):** raw → ledger: 174→184, 120→132, 91→99, 161→173. The ledger consistently counts the code diff plus roughly 8-12 lines; the first Phase 2 unit landed at 184/200 (16-line margin). Target **≤160 raw** per sub-unit here, not the full 200, to keep real margin.

`runner-summary-rendering` (420-680 raw-line original estimate) does not fit one 200-line ledger objective — its upper bound is higher than `runner-registry-and-rows`'s pre-split upper bound (560) was. The design's own seam (`runner-parse-functions` / `runner-summary-and-cap` / `runner-golden-wiring`) does not fit either once the two audit findings (staticcheck toolchain mismatch, deadcode exit-code untrustworthiness) and the invocation-strategy gap (obs #2714) are itemized: parsing four distinct tools' real output, two of them with a dedicated edge-case fixture each, is denser than one "parse functions" objective can hold at ≤160 raw lines. Split into 6 ledger objectives, ordered, all landing on PR3 (base: PR2 branch) as separate commits/attempts within that PR:

| Ledger objective | Scope | Depends on | Est. raw lines | Margin to 200 |
|---|---|---|---|---|
| `runner-pinned-invocation-parity` | Align `staticcheck`/`deadcode` `checkArgv` with CI's pinned `go run <module>@<version>` invocation (audit finding, obs #2714) | Phase 2D | 40-70 | 130-160 |
| `runner-parse-simple-checks` | `parseGofmt`, `parseGoVet`, their `failed(exit,count)` predicates, fixtures | `runner-pinned-invocation-parity` | 80-120 | 80-120 |
| `runner-parse-staticcheck` | `parseStaticcheck` + `failed()`, toolchain-mismatch edge case (audit finding, obs #2711) | `runner-pinned-invocation-parity` | 90-130 | 70-110 |
| `runner-parse-deadcode` | `parseDeadcode` + `failed()`, stdout-vs-exit-code divergence (audit finding, obs #2709/#2711/#2712) | `runner-pinned-invocation-parity` | 70-110 | 90-130 |
| `runner-summary-and-cap` | Bounded renderer (count/top-N/path, 200-char cap), `--top-n` flag, `capPayload`, banned-literal guard | `runner-parse-simple-checks`, `runner-parse-staticcheck`, `runner-parse-deadcode` | 130-160 | 40-70 |
| `runner-golden-wiring` | Wire real stdout/stderr capture into `runCheck`, replace Phase-2 placeholder summary in `run()` with the real renderer, stdout goldens | `runner-summary-and-cap` | 90-140 | 60-110 |

Total range 500-730 raw — higher than the original 420-680 forecast because the invocation-parity fix (previously undiscovered) and the split-by-tool granularity for staticcheck/deadcode add real, honest scope; still comfortably under the 800-line PR review budget for PR3. Why 6 and not the sketched 3: combining `parseStaticcheck` and `parseDeadcode` into one "parse functions" objective sums to 160-240 raw lines — the low end already meets the 160 target and the high end blows past it, and both audit findings deserve isolated rollback/test boundaries given they were the two things the audit flagged by name. `runner-pinned-invocation-parity` is its own objective (not folded into `runner-parse-staticcheck`/`runner-parse-deadcode`) because it changes existing Phase-2-delivered registry data via a content-parity test against `ci.yml`, is independently revertible, and gates the other two parse objectives' fixtures (fixture stderr/stdout must be captured from the actual pinned invocation, not a bare-binary invocation that may format differently).

**Why `runner-pinned-invocation-parity` belongs in Phase 3, not Phase 4:** Phase 4 (`runner-mode-separation`) is about the `normalize`/`check` subcommand split and byte-neutrality — orthogonal to which binary a check invokes. Phase 3 is the phase where check output becomes real (parse functions consume actual tool stdout/stderr), and the two richest parse objectives here (`runner-parse-staticcheck`, `runner-parse-deadcode`) are only trustworthy if their fixtures come from the same invocation CI actually runs — a bare `staticcheck ./...` binary run and a pinned `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...` run could in principle format diagnostics differently, and CI is the ground truth this whole change exists to match (obs #2714). Deferring the fix to Phase 4 would mean writing Phase 3's staticcheck/deadcode fixtures against the wrong invocation and reworking them later.

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | `severity-policy-and-ci-gates` — CRITICAL severity, staticcheck/deadcode CI gates, `tools` test layer | PR 1 (base: tracker) | `rg` content assertions (see 1.1/1.3/1.5) | N/A — markdown/YAML policy edit, no runtime path | Revert `strict-tdd-verify.md`, `SKILL.md` ordering line, `ci.yml`, `openspec/config.yaml` |
| 2 | `runner-registry-and-rows` — 4 ledger objectives (`runner-scaffold-and-discovery` → `runner-registry-and-classify` → `runner-result-and-outcome` → `runner-row-emission-and-ci-job`; see Slice 2 Ledger Re-split) | PR 2 (base: PR1 branch) | `cd tools/deterministic-check-runner && go test ./... -run 'TestClassify\|TestSelectOutcome\|TestModuleDiscovery\|TestRowEmission'` | `go run . check` against this repo's own `go.mod` set | Delete `tools/deterministic-check-runner/` module |
| 3 | `runner-summary-rendering` — 6 ledger objectives (`runner-pinned-invocation-parity` → `runner-parse-simple-checks`/`runner-parse-staticcheck`/`runner-parse-deadcode` → `runner-summary-and-cap` → `runner-golden-wiring`; see Phase 3 Ledger Re-split) | PR 3 (base: PR2 branch) | `cd tools/deterministic-check-runner && go test ./... -cover` | `go run . check --top-n 5` against a fixture with 200+ synthetic findings | Revert additive files only (parse funcs, renderer, goldens, pinned checkArgv); registry structure/rows from PR2 stay intact |
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

PR 2 (base: PR1 branch). Split into 4 ledger objectives — each its own RED→GREEN attempt under the 200-line-per-objective ledger cap (see Slice 2 Ledger Re-split above). Dependencies name prior objectives only; all 4 land as separate commits on the PR2 branch.

### 2A. runner-scaffold-and-discovery (R-005; threat matrix: git repository selection) — est. 130-180 lines

- [x] 2A.1 RED: create `tools/deterministic-check-runner/go.mod` (module path mirrors `entry-contract-validator`, `go 1.21`) and `tools/deterministic-check-runner/main_test.go` with failing `TestModuleShape` — asserts `go.mod`/`main.go`/`main_test.go`/`testdata/` parity with `tools/entry-contract-validator`.
- [x] 2A.2 GREEN: create `main.go` — `package main`, `func main()` calling `run(os.Args[1:], os.Stdout, os.Stderr)`, `run(args []string, stdout, stderr io.Writer) int` stub returning 0; add `testdata/.gitkeep`. `TestModuleShape` passes.
- [x] 2A.3 RED: add `TestModuleDiscovery` — sorted `go.mod` walk over a synthetic `t.TempDir()` module tree; asserts deterministic sorted order and correct discovery when invoked from a subdirectory.
- [x] 2A.4 GREEN: implement `discoverModules(root string) ([]string, error)` rooted at `CALLER_CWD`-normalized cwd, sorted `go.mod` walk. Test passes.
- [x] 2A.5 Run `cd tools/deterministic-check-runner && go test ./... -cover` — GREEN.

### 2B. runner-registry-and-classify (R-002 test half, R-006 partial) — est. 90-130 lines — depends on 2A — COMPLETE

- [x] 2B.1 RED: add `TestCheckRegistry` — exactly 4 checks; each declares `deterministic` and `blocking`; `checkArgv` is non-empty for all 4.
- [x] 2B.2 GREEN: add `check` struct (`name`, `deterministic`, `blocking`, `checkArgv` fields — `normalizeArgv` lands in Phase 4, `parse`/`failed` land in Phase 3) and hardcoded `[]check` registry: `gofmt` (`checkArgv: []string{"gofmt","-l","."}`, deterministic+blocking), `go vet` (deterministic+blocking), `staticcheck` (deterministic+blocking), `deadcode` (`blocking: false`). Test passes.
- [x] 2B.3 RED: add `TestClassify` — a check without `deterministic=true` never classifies CRITICAL regardless of `blocking` (R-002).
- [x] 2B.4 GREEN: implement `classify(c check) bool { return c.blocking && c.deterministic }` — the sole enforcement point. Test passes.
- [x] 2B.5 Run `cd tools/deterministic-check-runner && go test ./... -cover` — GREEN.

### 2C. runner-result-and-outcome (R-006 core) — est. 100-150 lines — depends on 2B — COMPLETE

- [x] 2C.1 RED: add `TestSelectOutcomePrecedence` — 6 cases: BLOCKING-set check unavailable alone → 3; `deadcode` unavailable alone → 0; `deadcode` unavailable + a blocking check failed → 1; runner-internal error → 3; non-blocking (`deadcode`) red alone → 0; all clean → 0.
- [x] 2C.2 GREEN: add `result` struct (`check check`, `exitCode int`, `unavailable bool`, `runnerErr bool` — `count`/`top`/`logPath` land with the Phase-3 renderer) and `selectOutcome([]result) int` per D4 precedence. Test passes.
- [x] 2C.3 Run `cd tools/deterministic-check-runner && go test ./... -cover` — GREEN.

### 2D. runner-row-emission-and-ci-job (R-006 core remainder, R-005 CI acceptance) — est. 115-160 lines — depends on 2A, 2B, 2C

- [x] 2D.1 RED: add `TestRowEmission` — `%s | %d | %s\n` format, one row per configured check, stable declared registry order, `exit_code` field is a typed integer (R-006).
- [x] 2D.2 GREEN: implement `emitRows(w io.Writer, results []result)`. Test passes.
- [x] 2D.3 RED: add `TestRunEndToEnd` — `run(args, stdout, stderr)` executes each configured check's `checkArgv` per discovered module and emits exactly 4 rows in registry order against this repo's own module set; summary text is a placeholder (`exit=<code>`) pending the Phase-3 renderer and must not be a banned literal.
- [x] 2D.4 GREEN: wire `run()` — `discoverModules` × registry × `exec.Command(checkArgv...)` per module × `classify`/`selectOutcome` × `emitRows`. Test passes.
- [x] 2D.5 Add CI job `test-deterministic-check-runner` to `.github/workflows/ci.yml` mirroring `test-entry-contract-validator` (`go test ./... -cover`) — R-005 acceptance.
- [x] 2D.6 Run `cd tools/deterministic-check-runner && go test ./... -cover` — GREEN.

## Phase 3: runner-summary-rendering (R-006 remainder, R-007, R-008)

PR 3 (base: PR2 branch). Re-split into 6 ledger objectives — each its own RED→GREEN attempt under the 200-line-per-objective ledger cap (see "Phase 3 Ledger Re-split" above). Dependencies name prior objectives only; all 6 land as separate commits on the PR3 branch. `runner-pinned-invocation-parity` is a mandatory prerequisite for the two audit-finding objectives (`runner-parse-staticcheck`, `runner-parse-deadcode`) because their fixtures must be captured from the actual pinned invocation, not a bare-binary run.

### 3A. runner-pinned-invocation-parity (audit finding — invocation-strategy gap, obs #2714) — est. 40-70 lines — depends on Phase 2D

- [x] 3A.1 RED: add `TestCheckArgvPinnedToCIInvocation` — parses `.github/workflows/ci.yml`'s pinned `go run honnef.co/go/tools/cmd/staticcheck@vX.Y.Z ./...` and `go run golang.org/x/tools/cmd/deadcode@vX.Y.Z ./...` invocation strings and asserts the registry's `staticcheck` and `deadcode` `checkArgv` match them exactly (module path + pinned version), so a future CI version bump that isn't mirrored in the runner fails loud. Currently fails: the Phase-2 registry resolves bare `staticcheck`/`deadcode` from `PATH`, producing exit 127 on any machine without those binaries installed while CI (which uses pinned `go run`) stays green on the same commit.
- [x] 3A.2 GREEN: change the `registry`'s `staticcheck` and `deadcode` entries' `checkArgv` to `[]string{"go", "run", "honnef.co/go/tools/cmd/staticcheck@v0.7.0", "./..."}` and `[]string{"go", "run", "golang.org/x/tools/cmd/deadcode@v0.48.0", "./..."}` respectively (`gofmt`/`go vet` unchanged — no separate CI pin exists for them). Test passes.
- [x] 3A.3 Run `cd tools/deterministic-check-runner && go test ./... -run 'TestCheckArgv|TestCheckRegistry|TestSelectOutcomePrecedence|TestRunEndToEnd'` — GREEN, confirming the Phase 2 registry/outcome/row tests still pass unmodified against the new argv values.

### 3B. runner-parse-simple-checks (R-006 remainder, R-007, R-008) — est. 80-120 lines — depends on 3A

- [x] 3B.1 RED: add table-driven `TestParseGofmt` and `TestParseGoVet` against `testdata/` fixtures; assert `gofmt -l` non-empty file-list output at exit 0 still yields `count>0` via `failed(exit,count)` (D3 — `gofmt`'s own exit code is not authoritative), and `go vet`'s count derives from parsed diagnostic lines with `failed` following its exit code directly.
- [x] 3B.2 GREEN: implement `parseGofmt`, `parseGoVet`, and their `failed(exit,count)` predicates. Tests pass.

### 3C. runner-parse-staticcheck (R-006 remainder, R-007 — audit finding, obs #2711) — est. 90-130 lines — depends on 3A

- [ ] 3C.1 RED: add table-driven `TestParseStaticcheck` (clean/N-findings cases) against `testdata/` fixtures captured from the pinned `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...` invocation (3A). Add `TestParseStaticcheckToolchainMismatch` — fixture reproduced against `tui/go.mod`'s `go 1.26.1` directive, error text `requires newer Go version`; asserts the result is marked `unavailable`/unexecutable, never a counted failure.
- [ ] 3C.2 GREEN: implement `parseStaticcheck` + its `failed(exit,count)` predicate. `parseStaticcheck` recognizes the toolchain-mismatch stderr signature and marks the result `unavailable` instead of counting it — `selectOutcome` (Phase 2C, unchanged) then routes it to `procedural_tooling_failed`, never `verification_failed`, per D4/R-016. Tests pass.

### 3D. runner-parse-deadcode (R-006 remainder, R-007 — audit finding, obs #2709/#2711/#2712) — est. 70-110 lines — depends on 3A

- [ ] 3D.1 RED: add table-driven `TestParseDeadcode` against `testdata/` fixtures captured from the pinned `go run golang.org/x/tools/cmd/deadcode@v0.48.0 ./...` invocation (3A). Assert findings are counted from parsed **stdout only**, never from `deadcode`'s exit code — confirmed empirically: 20-21 real findings, exit 0. Stdout must be captured separately from stderr: the Go toolchain-switch message lands on stderr and inflates the count if merged.
- [ ] 3D.2 GREEN: implement `parseDeadcode` (stdout-only counting) + its `failed(exit,count)` predicate (`count>0`, exit code ignored — `deadcode` stays WARNING-only via `classify()` regardless of this predicate). Tests pass.

### 3E. runner-summary-and-cap (R-007, R-008) — est. 130-160 lines — depends on 3B, 3C, 3D

- [ ] 3E.1 RED: add `TestSummaryRendering` over 0/1/N/N+1 findings — `count=N; top: …; full: <path>`, 200-char excerpt cap, `--top-n` honored (default 5), zero renders literal `0`.
- [ ] 3E.2 GREEN: implement the summary renderer + `--top-n` flag + stable out-dir `${TMPDIR}/labdrian-deterministic-checks/<tool>.log` (D6, overwritten on rerun). Test passes.
- [ ] 3E.3 RED: add `TestBannedLiterals` scanning every emitted summary for PASS/PASSED/SUCCESS/N/A/NA/NONE/TODO/TBD/PLACEHOLDER as a standalone value, including the zero-findings case.
- [ ] 3E.4 GREEN: confirm/guard the renderer never emits a banned literal standalone. Test passes.
- [ ] 3E.5 RED: add `TestCapPayload` — payload over 4 MiB truncates to counts+paths, never rejects.
- [ ] 3E.6 GREEN: implement `capPayload` (D6). Test passes.
- [ ] 3E.7 Run `cd tools/deterministic-check-runner && go test ./... -run 'TestSummaryRendering|TestBannedLiterals|TestCapPayload'` — GREEN.

### 3F. runner-golden-wiring (R-006 remainder, R-007 goldens) — est. 90-140 lines — depends on 3E

- [ ] 3F.1 RED: extend the `runCheck`-facing test to require real per-module stdout/stderr capture (currently discarded — only the exit code is kept), asserting `result.count`/`result.top`/`result.logPath` are populated by wiring the Phase-3 `parse` funcs into `runCheck` in place of the Phase-2 placeholder summary.
- [ ] 3F.2 GREEN: wire `runCheck` to capture stdout and stderr separately per module (`cmd.Stdout`/`cmd.Stderr` buffers, per D3/obs #2712), call the matching `parse` func, populate `result.count`/`result.top`/`result.logPath`, and replace `emitRows`'s Phase-2 placeholder `exit=%d` text with the real renderer output from 3E. `TestRunEndToEnd`'s banned-literal assertion (Phase 2D, unchanged) still passes.
- [ ] 3F.3 RED: add a golden test for the full stdout row block via the `-update` path (R-006).
- [ ] 3F.4 GREEN: generate goldens with `-update`, rerun without — byte-identical.
- [ ] 3F.5 Run `cd tools/deterministic-check-runner && go test ./... -cover` — full Phase 3 GREEN.

## Phase 4: runner-mode-separation (R-009, R-010)

> Forward assessment (not executed in this pass — scope is out of bounds for the current Phase-3-only re-split): original estimate 180-340 raw lines. Tasks 4.1-4.4 (dispatch + fixer-flag guard) and 4.5-4.10 (byte-neutral integration test, out-dir guard, missing-tool stubbed-PATH test) are two natural seams; the 340-line upper bound alone is unlikely to clear the 200-line ledger cap as one objective, and even the two-seam split's upper half (byte-neutral + out-dir + stubbed-PATH combined) risks landing near or over 200 given the ledger's ~8-12-line overcount observed across all four Phase 2 objectives (obs #2713). Recommend a dedicated `sdd-tasks` re-split pass before `sdd-apply` begins Phase 4, following the same pattern used here for Phase 3.

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

> Forward assessment (not executed in this pass — scope is out of bounds for the current Phase-3-only re-split): original estimate 240-420 raw lines, comparable in magnitude to Phase 2's pre-split `runner-registry-and-rows` (380-560). Much of this phase is shell/`SKILL.md` prose rather than Go (5.1-5.6, 5.11-5.12), which the ledger appears to undercount relative to code (slice 1 recorded 67 against 668 insertions of mostly-planning-doc content, per obs #2713) — so Phase 5 may fit in fewer, larger objectives than a pure line-count read suggests. Still, 5.9-5.10 (outcome-mapping tests + implementation) and 5.13 (full regression) are Go/shell-heavy and should not be assumed to fit one 200-line objective without measurement. Recommend a dedicated `sdd-tasks` re-split pass before `sdd-apply` begins Phase 5, informed by the actual ledger counts Phases 3 and 4 produce.

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
