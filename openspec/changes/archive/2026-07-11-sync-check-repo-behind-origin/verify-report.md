# Verification Report: sync-check-repo-behind-origin

**Change**: `sync-check-repo-behind-origin`
**Mode**: Full artifacts (proposal, specs, design, tasks — all present)
**Verified on**: `main` @ HEAD (commits `5641c00`, `73107a9`), post-merge of PR #94 and PR #95
**Verdict**: **PASS**

## Completeness (tasks.md)

20/20 tasks marked `[x]`. Cross-checked each against actual code/tests — all checkmarks are honest; no task claims evidence that isn't present in the repo.

| Phase | Tasks | Status |
|---|---|---|
| 1 — Bash git-fixture harness (RED) | 1.1–1.7 | Confirmed: `engine/installer/sync_check_test.go` exists with all 7 named tests |
| 2 — Bash implementation (GREEN) | 2.1–2.4 | Confirmed: `compute_repo_behind_origin()`, flag parsing, VERDICT append all present in `bin/labdrian-overlay`; shellcheck-unavailable note verified accurate (`which shellcheck` empty in this sandbox too) |
| 3 — Go parsing/classification | 3.1–3.4 | Confirmed: `classify()` 3-arg, `SyncBehindOrigin` enum, `RepoBehindOrigin` field, dedicated NA pre-check in `ParseSyncCheck` |
| 4 — TUI rendering | 4.1–4.2 | Confirmed: `tui/view_test.go` (3 tests) + `view.go`'s `SyncBehindOrigin` case and conditional counts append |
| 5 — Integration & regression | 5.1–5.2 | Confirmed: full suites pass; dogfood claim plausible and consistent with current repo state |
| 6 — Documentation | 6.1 | Confirmed: `usage()` heredoc documents `--check-origin`/`--fetch` and `REPO_BEHIND_ORIGIN` |

## Test Execution (real runs, this session)

```
cd tui && go clean -testcache && go test ./...
  ok  github.com/labdrian-ai/labdrian-sdd-overlay/tui  0.082s

cd engine && go clean -testcache && go test ./...
  ok  .../engine/assets       ok  .../engine/gate         ok  .../engine/prespec
  ok  .../engine/cmd          ok  .../engine/installer     ok  .../engine/propagator
  ok  .../engine/gadu         ok  .../engine/runtime        ok  .../engine/settings
                                                             ok  .../engine/skills
```

`go vet ./...` clean on both modules. All 8 sync-check-specific tests (7 bash-fixture + `TestSyncCheck_BehindOriginOnly_ActionHintsGitPull`) and the 3 new TUI rendering tests pass individually with `-v`. No skipped/flaky output observed.

CI on both merged PRs independently confirms `Shell Lint` passing (not runnable in this sandbox — `shellcheck` binary absent here too, same gap PR1/PR2 flagged):

- PR #94 (`feat(sync-check): detect commits behind origin/main`): Engine Tests / Shell Lint / TUI Tests — all `pass`.
- PR #95 (`feat(sync-check): render origin-behind status and fix action hint`): Engine Tests / Shell Lint / TUI Tests — all `pass`.

## Spec Compliance Matrix (R-001..R-006)

| Req | Requirement | Test(s) citing ID | Status |
|---|---|---|---|
| R-001 | Default cached-ref comparison, no fetch | `TestSyncCheck_ReportsRepoBehindOrigin_CachedRef` (proves cached value survives origin advancing further, i.e. no live fetch) | PASS |
| R-002 | `REPO_BEHIND_ORIGIN` always present, `<count>` or `NA` | `TestSyncCheck_ReportsRepoBehindOrigin_CachedRef`, `TestSyncCheck_EvenWithOrigin_ReportsZero`, `TestParseSyncCheckDashboard` (R-002/R-004 comment) | PASS |
| R-003 | `--check-origin`/`--fetch` triggers live fetch; fetch failure degrades to scoped warning | `TestSyncCheck_CheckOriginFlag_FetchesLive`, `TestSyncCheck_CheckOriginFlag_FetchFailure_DegradesToNA` | PASS |
| R-004 | Graceful degrade, no remote / no cached ref | `TestSyncCheck_NoOriginRemote_ReportsNA`, `TestSyncCheck_NoCachedRef_ReportsNA` | PASS |
| R-005 | TUI renders distinct indicator when count > 0, none when 0 | `TestViewDashboard_ShowsOriginBehindIndicator`, `TestViewDashboard_ZeroOriginBehind_NoIndicator` | PASS |
| R-006 | Never silently healthy while behind; exit code unaffected; additive precedence (both verdicts shown together) | `TestClassifyPrecedence`, `TestViewDashboard_NeverHealthyWhileBehindOrigin` cover "never healthy". No CLI exit-code assertion found, but `compute_repo_behind_origin`/VERDICT line have zero interaction with any `exit`/`die` call in `cmd_sync_check` — code-trace confirms the "exit code unaffected" scenario; low risk. See WARNING-1 for the "additive precedence" rendering scenario. | PASS (with WARNING-1 noted) |

## Design Coherence

| Design decision | Verified against code | Match |
|---|---|---|
| Compute once per invocation, before target loop | `repo_behind_origin="$(compute_repo_behind_origin ...)"` at line 869, before `for t in resolve_targets` at 871 | Yes |
| `REPO_BEHIND_ORIGIN=<n>` / `=NA` sentinel, never omitted | Confirmed at VERDICT echo (line 960); `compute_repo_behind_origin` always echoes one of the two forms | Yes |
| `--check-origin`/`--fetch` as boolean aliases | `case "$1" in --check-origin|--fetch) check_origin=true ;;` | Yes |
| Fetch failure → NA + scoped `SYNC_CHECK:` warning on stderr, no silent fallback to stale cache | Lines 826–831; warning written to stderr (verified not stdout-visible in the fixture test's `out` capture pattern, matching design's explicit stdout/stderr separation) | Yes |
| Guard idiom for every new git call under `set -euo pipefail` | `git remote get-url origin`, `git fetch origin`, `git rev-parse --verify -q refs/remotes/origin/main`, `git rev-list ...` — every one is inside an `if ...; then/else` or `if ! ...; then return; fi` guard; none left bare | Yes |
| Go `classify()` 3-arg, precedence `UPSTREAM_CHANGED > OVERLAY_NOT_DEPLOYED > REPO_BEHIND_ORIGIN > healthy` | `tui/run.go:135-146` matches exactly | Yes |
| `RepoBehindOrigin int`, `-1` = NA sentinel (Go side) | `TargetVerdict.RepoBehindOrigin`, default `-1` in `get()`, dedicated `val == "NA"` pre-check before `strconv.Atoi` | Yes |
| View: distinct badge (`colorAmber`, `~`, "Detrás de origin") + counts line shown only when `RepoBehindOrigin > 0` | `tui/view.go:435-436, 448-449` | Yes — deviation from design's File Changes table prose (">=0") is intentional and explicitly documented in tasks.md 4.2 as correctly following the spec (R-005 "zero renders no indicator") over the looser design prose; not a defect |
| No new `runtime.Action` | Confirmed — no changes to `engine/runtime` for this capability | Yes |
| Exit code unchanged (informational only) | No `exit`/`die` call reads `repo_behind_origin` anywhere in `cmd_sync_check` | Yes |

No design deviations found that break a spec requirement. The one documented deviation (view.go's `>0` vs. design table's `>=0` prose) is a correction toward the spec, not away from it, and is already called out honestly in tasks.md.

## Issues

### CRITICAL
None.

### WARNING
1. **R-006 "additive precedence" scenario has no dedicated rendering-layer test.** The spec's third R-006 scenario ("`UPSTREAM_CHANGED=1` and `REPO_BEHIND_ORIGIN=2` simultaneously → both shown, neither hides the other") is verified at the **parsing** layer (`TestParseSyncCheckDashboard`'s `codex` case: `UPSTREAM_CHANGED=3`, `REPO_BEHIND_ORIGIN=5`, and `v.RepoBehindOrigin` stays `5` even though `Status == SyncNeedsCapture`) but is **not** verified at the **rendering** layer — no test sets `Status: SyncNeedsCapture` (or calls `classify(1,0,3)`) together with `RepoBehindOrigin: 3` and asserts `viewDashboard()` shows both the "Requiere capture + apply" label AND the "detrás de origin: 3" count in the same render. Code trace confirms correctness: `tui/view.go`'s `if v.RepoBehindOrigin > 0 { countsText += ... }` (line 448) is unconditional on `v.Status`, so the combination is provably safe by inspection — but per this skill's hard rule ("a spec scenario is compliant only when a covering test passed at runtime"), this specific scenario remains technically UNTESTED at the rendering layer. Low risk (trivial, already-isolated append), but should be closed with one more test case before this is considered fully proven rather than trace-verified. This matches the residual gap flagged in the task brief — confirmed real.
2. **`shellcheck` cannot be run in this sandbox** (binary absent), so the bash lint claim in tasks.md 2.4/5.1 could not be independently re-verified locally. Mitigated: confirmed via `gh pr checks` that CI's `Shell Lint` job passed on both PR #94 and PR #95. Not a defect, just an unverifiable-locally note (matches the task brief's known-accepted-gap list).
3. **No dedicated test for R-006's "exit code unaffected" scenario.** Verified only by code trace (no code path reads `repo_behind_origin` before any `exit`/`die` call). A cheap integration assertion (`sync-check` exit code identical for `REPO_BEHIND_ORIGIN=0` vs. `>0` fixtures) would close this gap; currently trace-only, matching the same "low risk, worth flagging" pattern as WARNING-1.

### SUGGESTION
1. Consider adding the R-001 "no network connectivity" scenario as an explicit fixture (e.g. via a firewalled/unreachable remote) rather than relying on "no live fetch was invoked" as an implicit proxy — current coverage is a reasonable proxy but doesn't simulate an actual network-down condition.
2. `TestSyncCheck_BehindOriginOnly_ActionHintsGitPull` (the PR2 ACTION-hint fix for the judgment-day-found gap) is a good regression pin; consider citing R-002/R-006 explicitly in its doc comment for traceability, since it currently only references the informal "judgment-day gap" framing rather than a requirement ID.

## Final Verdict

**PASS** — all 6 requirements (R-001..R-006) are implemented and covered by passing tests; both bash and Go test suites are fully green (`go vet` clean); design decisions (guard idiom, NA sentinel on both bash and Go sides, additive classify() precedence) match the merged code exactly; tasks.md's 20/20 checklist is honest — every checked task has verifiable evidence in the repo, including its own accurately-self-reported gaps (shellcheck unavailability). No CRITICAL issues found. Two WARNINGs are residual test-coverage gaps for provably-safe-by-trace code paths (R-006's additive-rendering combo, R-006's exit-code-unaffected claim) — recommended as follow-up hardening, not blockers. Safe to proceed to `sdd-archive`.
