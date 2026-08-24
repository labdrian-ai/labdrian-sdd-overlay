```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:b91be4cb3c87940ec770a5403e3f15a25b292040c8bfac7e9faafbad8e30ac07
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 7/7
scenarios: 10/10
test_command: 'cd tui && go test ./... -v -count=1'
test_exit_code: 0
test_output_hash: sha256:3ba7761646eb27f3020ea5861646d7d1ee26295ada0701b2e68a84ba4101aeb8
build_command: 'out="$(mktemp -d)"; (cd tui && go build -o "$out/" ./...)'
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# Verification Report — tui-self-update-offer

**Change**: `tui-self-update-offer` | **Phase**: verify | **Store**: hybrid | **Mode**: Strict TDD
**Date**: 2026-08-22 | **Branch**: `feat/tui-self-update-3-action` | **HEAD**: `4babe40` | **Tree**: clean

Verified against the full three-PR chain tip, which contains PR1 (`cdb1858`, backend subcommand),
PR2 (`ec234c3`, probe + banner) and PR3 (`4babe40`, action entry + re-probe). This is an independent
verification: every requirement was re-derived from the current on-disk source and from a freshly
executed suite, not from the apply batches' self-reports.

## How `evidence_revision` was derived (re-derivable)

SHA-256 of a manifest containing, in this order: `head 4babe4029067262a2def09bd5d3547a9deb340d7`,
then one `<sha256>  <path>` line for each of the ten inspected artifacts in this order —
`bin/labdrian-overlay`, `tui/model.go`, `tui/run.go`, `tui/view.go`, `tui/main_test.go`,
`tui/view_test.go`, `tui/selfupdate_backend_test.go`,
`openspec/changes/tui-self-update-offer/specs/tui-self-update/spec.md`,
`openspec/changes/tui-self-update-offer/design.md`,
`openspec/changes/tui-self-update-offer/tasks.md` — then a line for `test_output_hash` and one for
`build_output_hash` in the same `<sha256>  <label>` shape. Anyone can rebuild it.

## Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 25 |
| Tasks complete | 25 |
| Tasks incomplete | 0 |

Counted directly from `openspec/changes/tui-self-update-offer/tasks.md`: Phase 1 (5) + Phase 2 (4) +
Phase 3 (6) + Phase 4 (2) + Phase 5 (5) + Phase 6 (3) = 25, all `[x]`. See WARNING-1: the Engram
mirror of this artifact is stale and still shows Phases 5-6 unchecked.

## Build & Tests Execution

**Build**: PASSED

```text
out="$(mktemp -d)"; (cd tui && go build -o "$out/" ./...)   -> exit 0, empty output
```

**Tests**: 64 top-level PASSED, 31 subtests PASSED, 0 failed, 0 skipped

```text
cd tui && go test ./... -v -count=1   -> exit 0
ok  github.com/labdrian-ai/labdrian-sdd-overlay/tui  0.709s
=== RUN lines: 95   --- PASS (top-level): 64   --- PASS (subtest): 31   --- FAIL: 0   SKIP: 0
```

Test files: `logo_test.go` (1), `main_test.go` (50), `selfupdate_backend_test.go` (9),
`view_test.go` (4) = 64.

**Coverage**: not available — no coverage tool configured for this project; skipped, not a failure.

## Spec Compliance Matrix

Authoritative counts read from `specs/tui-self-update/spec.md`: 7 requirements, 10 scenarios.

| Requirement | Scenario | Test (runtime evidence) | Result |
|---|---|---|---|
| R-001 Launch-time cached-only origin probe | Probe is cached-only and async | `main_test.go > TestProbeBehindOriginCmd` (stub backend records argv+cwd: asserts neither `--fetch` nor `--check-origin` is passed, and `cmd.Dir == root`); `TestInit_ReturnsNonNilCmd`; `TestUpdate_ProbeDoneMsg_SetsBehindOrigin`; `TestInitialRenderShowsTargets` (teatest drives the real program: the target screen renders while the probe is in flight) | COMPLIANT |
| R-002 Dismissible behind-origin banner, no new screen | Banner shown for nonzero, hidden for zero/NA | `view_test.go > TestRepoLine_BehindOriginBannerStates` subtests `positive count, not dismissed -> shown`, `zero -> hidden`, `NA -> hidden` | COMPLIANT |
| R-002 | Dismissal suppresses the banner for the session | `main_test.go > TestGlobalXKey_DismissesBannerOnlyWhenVisible` (both directions); `view_test.go > TestRepoLine_BehindOriginBannerStates/positive but dismissed -> hidden` | COMPLIANT |
| R-003 Update-repository action entry | Target-agnostic, mutating, and scoped confirm copy | `main_test.go > TestSelfUpdateActionRegistered` (Mutating+TargetAgnostic true, placed immediately after `apply` and before `install-hooks`); `TestSelfUpdateConfirmScreen` (sources the real `Actions()` entry, renders `screenConfirm`, asserts no target list and that the copy names `main`); `TestActionMenuShapeAndOrder` | COMPLIANT |
| R-004 self-update fast-forwards main only | Clean tree converges main, original branch restored | `selfupdate_backend_test.go > TestSelfUpdateBackend_BehindFastForwardsAndReturns` (exit 0, output mentions fast-forward, current branch back to `feature-x`, `main == origin/main`) | COMPLIANT |
| R-004 | Only main moves | same test — `featureHeadBefore == featureHeadAfter` and `main HEAD == origin/main HEAD`; plus `TestSelfUpdateBackend_UpToDateNoCheckout` proves no checkout occurs at all when already converged (HEAD reflog byte-identical) | COMPLIANT |
| R-005 Hard refusal on a dirty tracked tree | Dirty tracked tree blocks the update | `TestSelfUpdateBackend_DirtyTrackedTreeBlocks` (nonzero exit, `ERROR:` prefix, names the refusal, branch unchanged, `main` HEAD unchanged, dirty content preserved) | COMPLIANT |
| R-005 | Untracked-only changes do not block | `TestSelfUpdateBackend_UntrackedOnlyProceeds` (exit 0, `main` converged, untracked file intact) | COMPLIANT |
| R-006 Hard refusal on local-ahead divergence | Local-ahead main blocks the update | `TestSelfUpdateBackend_LocalAheadBlocks` (nonzero exit, `ERROR:` prefix, message names `ahead of origin/main`, branch unchanged, `main` HEAD unchanged) | COMPLIANT |
| R-007 Successful update observable via REPO_BEHIND_ORIGIN | Post-update convergence | `TestSelfUpdateBackend_SyncCheckConvergesToZero` (runs the real `sync-check` after a real `self-update`, asserts `REPO_BEHIND_ORIGIN=0`) | COMPLIANT |

**Compliance summary**: 10/10 scenarios compliant, 7/7 requirements covered.

### Independent source confirmation of R-007's negative clause

R-007 also requires that this capability MUST NOT alter `sync-check-verdicts`' detection logic, VERDICT
fields, or exit codes. Confirmed from the diff, not from the apply report: `git diff main...HEAD --
bin/labdrian-overlay` contains exactly three hunks — the `usage()` entry (`@@ -102,6 +102,11 @@`), the new
`cmd_self_update` block appended after `cmd_apply` (`@@ -814,6 +819,78 @@`), and the dispatcher row
(`@@ -1799,6 +1876,7 @@`). `compute_repo_behind_origin` and the VERDICT emitter are untouched.

Also confirmed at source: `sync-check`'s `git fetch origin` is gated behind `if [[ "$check_origin" == true ]]`,
and `--check-origin|--fetch` is the only flag that sets it. The probe passes neither flag, so R-001's
"MUST NOT invoke git fetch" holds by construction of the callee, not only by the stub-recorded argv.

## Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| R-001 | Implemented | `tui/run.go:301 probeBehindOriginCmd` — `exec.Command(bin, "sync-check")`, `cmd.Dir = root`, CombinedOutput fed to `probeBehind` even on nonzero exit. `tui/run.go:287 probeBehind` returns `RepoBehindOriginNA` on zero verdicts. `tui/model.go:86 Init()` returns that cmd. |
| R-002 | Implemented | `tui/view.go:127 repoLine()` — `rootErr` keeps precedence, then `bannerVisible()` gates the amber line. `tui/model.go:141 bannerVisible()` = `rootErr == nil && behindOrigin > 0 && !bannerDismissed`. Screen enum still holds exactly the original five values (`screenTargets`..`screenResult`, `tui/model.go:13-17`) — no new screen was added. |
| R-003 | Implemented | `tui/run.go:67-74` — the entry matches design D7's struct literal verbatim (Name, Command, Mutating, TargetAgnostic, ConfirmMessage naming `main`, Hint), positioned between `apply` and the hooks block. |
| R-004 | Implemented | `bin/labdrian-overlay:874-891` — pre-expanded `trap "git checkout '${current_branch}' ..." EXIT`, `git checkout main`, `git merge --ff-only origin/main`, checkout back, `trap - EXIT`. |
| R-005 | Implemented | `bin/labdrian-overlay:843-844` — `[[ -z "$(git status --porcelain --untracked-files=no)" ]] || die ...`, fired before any checkout. `die()` at `:49` is `echo "ERROR: $*" >&2; exit 1`, so the `ERROR: ` prefix and exit 1 are structural. |
| R-006 | Implemented | `bin/labdrian-overlay:861-864` — `git rev-list --count origin/main..main`, `die` when nonzero, also before any checkout. |
| R-007 | Implemented | Consumer-only. Verified by diff scope above. |

## Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| D1 — `self-update`, flagless, dispatcher + usage | Yes | `[[ $# -eq 0 ]] || die "Unknown option: $1"`; dispatcher row at `:1879`; usage entry at `:105`. |
| D2 — every refusal fires before any checkout | Yes | Implemented in the exact prescribed order: (1) no origin, (2) dirty tracked, (3) bounded fetch with the `timeout`-optional fallback, (4) local-ahead, (5) up-to-date short-circuit, (6) trap + checkout + ff merge. |
| D3 — exit code + plain lines, no TUI special-casing | Yes | `info "Updated main: <old>..<new> (fast-forward)"` / `info "main is already up to date with origin/main"`; refusals are one-line `die` to stderr with exit 1. No VERDICT lines, never exit 2. |
| D4 — probe reuses sync-check + ParseSyncCheck | Yes | `probeBehind` delegates to `ParseSyncCheck` and reads `verdicts[0].RepoBehindOrigin`; no duplicated detection. |
| D5 — `probeDoneMsg`, NA-initialized field, re-probe tail | Yes | `newModel()` sets `behindOrigin: RepoBehindOriginNA` (`model.go:79`); `case probeDoneMsg` at `:193`; re-probe gate at `:188` is exactly `msg.result.action.Command == "self-update" && msg.result.err == nil`. |
| D6 — amber banner copy + global `x` dismissal | Yes | Copy matches D6 verbatim; `x` intercept sits beside `ctrl+c` in the `tea.KeyMsg` case (`model.go:215`), guarded by `bannerVisible()`. |
| D7 — action entry placement | Yes | Verbatim struct literal, placed after `apply` and before the hooks block. |

**Deviations**: none.

## TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | PASS | TDD Cycle Evidence tables present in apply-progress for all three batches |
| All tasks have tests | PASS | Every RED task maps to a named test that exists on disk |
| RED confirmed (tests exist) | PASS | All 19 named tests located in `main_test.go`, `view_test.go`, `selfupdate_backend_test.go` |
| GREEN confirmed (tests pass) | PASS | 19/19 re-executed by this verify run; all pass |
| Triangulation adequate | PASS | `TestProbeBehind` 4 cases; `TestRepoLine_BehindOriginBannerStates` 5 cases; `TestUpdate_SelfUpdateSuccess_RefiresProbe` 3 cases including two negatives that pin the gate on BOTH command and error; `TestGlobalXKey_...` 2 cases |
| Safety Net for modified files | PASS | apply-progress records pre-batch baselines 54/54 then 61/61; end state 64/64 — monotone, consistent with additive-only work |

**TDD Compliance**: 6/6 checks passed.

### Tests added by this change (independently enumerated from the diff)

`TestInit_ReturnsNonNilCmd`, `TestNewModel_BehindOriginDefaultsToNA`,
`TestUpdate_ProbeDoneMsg_SetsBehindOrigin`, `TestGlobalXKey_DismissesBannerOnlyWhenVisible`,
`TestProbeBehind`, `TestProbeBehindOriginCmd`, `TestSelfUpdateActionRegistered`,
`TestSelfUpdateConfirmScreen`, `TestUpdate_SelfUpdateSuccess_RefiresProbe`,
`TestRepoLine_BehindOriginBannerStates` (10 in `tui/main_test.go` + `tui/view_test.go`), plus the 9
backend integration tests in the new `tui/selfupdate_backend_test.go`. Total 19 new tests.

## Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 54 | `main_test.go` (pure funcs + model/view state), `view_test.go`, `logo_test.go` | Go stdlib `testing` |
| Integration | 10 | `selfupdate_backend_test.go` (9, Go->bash against real scratch git repos via `OVERLAY_DIR`), `main_test.go` (1, `teatest` program-level render) | `os/exec` + real `git`; `x/exp/teatest` |
| E2E | 0 in suite | — | Performed once out-of-suite: pty-driven run of the compiled binary against a real scratch clone (apply-progress task 6.2) |
| **Total** | **64** | **4** | |

## Changed File Coverage

Coverage analysis skipped — no coverage tool is configured for this project. Not a failure.

## Assertion Quality

Audited all 19 tests added by this change, plus the 3 pre-existing tests it modified.

No tautologies, no assertions that skip production code, no ghost loops (every table loop iterates a
non-empty literal slice), no smoke-test-only assertions, no mock-heavy tests, no CSS/implementation-detail
coupling. Specific strengths worth recording:

- `TestUpdate_ProbeDoneMsg_SetsBehindOrigin` asserts `behind: 5` — a value that is neither the zero value
  nor the `RepoBehindOriginNA` sentinel, so it cannot pass by accident against the default.
- `TestProbeBehindOriginCmd` uses a recording stub backend and asserts on the actual recorded argv and cwd,
  making R-001's "no fetch" clause a behavioral assertion rather than a code-reading claim.
- `TestSelfUpdateConfirmScreen` sources the real entry from `Actions()` instead of constructing a
  stand-in, so it cannot pass while the real menu lacks the entry.
- `TestSelfUpdateBackend_UpToDateNoCheckout` asserts the HEAD reflog is byte-identical before and after —
  a genuine proof that no checkout occurred, not merely that the branch name ended up the same.
- The refusal tests assert branch, `main` HEAD, and file content are all untouched, not just the exit code.

**Assertion quality**: all assertions verify real behavior. 0 CRITICAL, 0 WARNING.

## Quality Metrics

**gofmt**: clean (`gofmt -l .` -> no output)
**go vet**: clean (exit 0)
**staticcheck**: clean (0 findings)
**deadcode**: clean (0 findings)
**bash -n bin/labdrian-overlay**: clean (exit 0)

Run via the project's own gate: `bin/labdrian-overlay deterministic-checks check` -> `gofmt | 0 | 0`,
`go vet | 0 | 0`, `staticcheck | 0 | 0`, `deadcode | 0 | 0`.

## Threat Matrix Re-Confirmation

Every applicable row of design.md's threat matrix was re-checked against a named, currently passing test:

| Row | Covering test | Status |
|---|---|---|
| Git repository selection | all 9 `TestSelfUpdateBackend_*` (scoped via `OVERLAY_DIR`); `TestProbeBehindOriginCmd` (asserts `cmd.Dir == root`) | Covered |
| Commit state (dirty refusal / untracked ignored) | `TestSelfUpdateBackend_DirtyTrackedTreeBlocks`, `TestSelfUpdateBackend_UntrackedOnlyProceeds` | Covered |
| Local-ahead divergence | `TestSelfUpdateBackend_LocalAheadBlocks` | Covered |
| No origin remote | `TestSelfUpdateBackend_NoOriginRemoteBlocks`; `TestProbeBehind/explicit NA`; `TestRepoLine_.../NA -> hidden` | Covered |
| Never-fetched cache / malformed probe | `TestProbeBehind/zero verdicts`, `TestProbeBehind/garbage output`, `TestNewModel_BehindOriginDefaultsToNA` | Covered |
| `main` checkout fails mid-op | `TestSelfUpdateBackend_BlockedCheckoutRestoresBranch` | Covered |
| Concurrent external git op | `TestSelfUpdateBackend_HeldIndexLockBlocksBranchIntact` | Covered |
| Documentation-like paths / push state / PR commands | — | N/A per design |

## Review Workload

| Slice | Range | Changed lines (add+del) | Within 400-line budget |
|---|---|---|---|
| PR1 — backend | `main..cdb1858` | 862 | No |
| PR2 — probe + banner | `cdb1858..ec234c3` | 355 | Yes |
| PR3 — action + re-probe | `ec234c3..4babe40` | 181 | Yes |
| Whole chain | `main...4babe40` | 1358 | n/a |

The `auto-chain` delivery strategy and the planned three-way split were followed. PR1 nonetheless exceeds
the budget — see WARNING-2.

## Issues Found

**CRITICAL**: None.

**WARNING**:

1. **Engram `sdd/tui-self-update-offer/tasks` is stale and contradicts the filesystem.** The Engram copy
   (obs #3039, revision 4) still shows Phases 5 and 6 as `[ ]` — 8 tasks unchecked — while the
   authoritative `openspec/changes/tui-self-update-offer/tasks.md` shows all 25 as `[x]`. Under the hybrid
   store both mirrors are supposed to agree. A later phase or session that reads only Engram would
   conclude this change is incomplete. This is a bookkeeping defect, not a code defect: the filesystem
   copy matches the code state, which is what this report verified against. Recommend re-saving the tasks
   artifact to Engram from the current file before archive.

2. **PR1's slice is 862 changed lines — over twice the 400-line review budget.** The chain was executed as
   planned, but the guard's purpose (bounded reviewer load per PR) is not met for PR1. Two contributing
   factors: the 419-line `tui/selfupdate_backend_test.go` harness and 365 lines of OpenSpec planning
   artifacts landed in the same slice. Related: `tasks.md`'s forecast estimated ~500-560 changed lines for
   the *entire* change; the actual total is 1358, a 2.4x underestimate. If PRs have not been opened yet,
   splitting the planning artifacts (or the test harness) into their own slice would bring PR1 under
   budget. This does not block archive — it is a delivery-shaping observation, not a spec or test failure.

**SUGGESTION**:

1. The orchestrator's handoff described the spec as having 11 scenarios. The authoritative spec file has
   10 (7 requirements). This report's envelope uses the counted values, per the rule that envelope totals
   are derived from the retrieved specs and never asserted.
2. R-005 and R-006 say "exit 1", but the backend refusal tests assert `code != 0` rather than `code == 1`.
   The behavior is correct — `die()` is `echo "ERROR: $*" >&2; exit 1`, so exit 1 is structural — but the
   assertions under-constrain the spec's literal exit code and would not catch a future refusal path that
   exited 2.
3. R-002's "no value was added to the screen enum" clause is verified statically here (the enum still
   holds exactly its original five values) and is implicitly protected by the compiler, but no test pins
   the enum's arity. A one-line assertion that `screenResult == 4` would make that clause runtime-checked
   like the rest of the scenario.

## Verdict

**PASS WITH WARNINGS** — 0 CRITICAL, 2 WARNING, 3 SUGGESTION. **Archive-ready: YES.**

All 7 requirements and all 10 scenarios have passing runtime coverage; the full suite, build, and every
deterministic check are green on the chain tip; the implementation matches design D1-D7 with no
deviations; and the change's negative clause — that `sync-check-verdicts`' detection logic is untouched —
was independently confirmed from the diff. Neither warning is a defect in the shipped behavior: one is an
Engram/filesystem mirror drift in the tasks artifact, the other is a reviewer-load observation about how
the work was sliced.
