# Tasks: Detect Local Repo Behind origin/main in sync-check

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~430-470 (bash ~55, run.go ~40, view.go ~20, main_test.go ~45, view_test.go ~80 new, sync_check_test.go ~200 new, docs ~10) |
| 400-line budget risk | Medium |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 -> PR 2 (stacked) |
| Delivery strategy | ask-on-risk (default; entry's single-pr was advisory) |
| Chain strategy | stacked-to-main |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Bash verdict field + Go parsing/classification (Ph. 1-3) | PR 1 | base=main; ~340 lines; includes git-fixture harness |
| 2 | TUI rendering + docs/regression (Ph. 4-6) | PR 2 | base=PR 1 branch; ~110 lines; depends on PR 1's Go types |

Revises entry's `single-pr` advisory: combined total is near/over budget, driven by the new bash git-fixture harness (highest-risk item, per estimate #2071). Split above keeps each slice under 400 lines.

## Phase 1: Bash Git-Fixture Test Harness (RED)

- [x] 1.1 New `engine/installer/sync_check_test.go`: add `setupSandboxOverlayWithOrigin(t, home)` — bare `origin` repo, configurable ahead-commit count, fetched/unfetched toggle.
- [x] 1.2 RED `TestSyncCheck_ReportsRepoBehindOrigin_CachedRef` (R-001/R-002): HEAD 3 behind, cached ref, asserts `REPO_BEHIND_ORIGIN=3`, no fetch call.
- [x] 1.3 RED `TestSyncCheck_EvenWithOrigin_ReportsZero` (R-002): asserts `REPO_BEHIND_ORIGIN=0`.
- [x] 1.4 RED `TestSyncCheck_NoOriginRemote_ReportsNA` (R-004): no `origin`, asserts literal `REPO_BEHIND_ORIGIN=NA`, other checks complete.
- [x] 1.5 RED `TestSyncCheck_NoCachedRef_ReportsNA` (R-004): origin configured, ref never fetched, asserts `NA`.
- [x] 1.6 RED `TestSyncCheck_CheckOriginFlag_FetchesLive` (R-003): `--check-origin` fetches, count reflects fresh state.
- [x] 1.7 RED `TestSyncCheck_CheckOriginFlag_FetchFailure_DegradesToNA` (R-003): fetch fails, asserts `NA` + scoped warning, other checks still complete.

## Phase 2: Bash Implementation (GREEN)

- [x] 2.1 `bin/labdrian-overlay` `cmd_sync_check`: parse `--check-origin`/`--fetch` as boolean aliases.
- [x] 2.2 Add `compute_repo_behind_origin()`, called once before the target loop; wrap `git fetch origin` and `git rev-list HEAD..origin/main --count` in the `if <cmd> 2>/dev/null; then ... else NA; fi` guard idiom (precedent line 854).
- [x] 2.3 Append `REPO_BEHIND_ORIGIN=<n|NA>` to each target's `VERDICT:` line.
- [x] 2.4 Run `go test ./engine/installer/...` (Phase 1 -> GREEN) and `shellcheck bin/labdrian-overlay`. **Note**: `go test ./engine/installer/...` passes (all 7 sync-check tests GREEN, full package suite green). `shellcheck` is not installed in this environment (`which shellcheck` empty, no apt/snap package present) — could not run it; `bash -n bin/labdrian-overlay` passes as a syntax-only substitute. Flagged as a risk for CI/verify to run shellcheck where available.

## Phase 3: Go Parsing/Classification (RED -> GREEN)

- [x] 3.1 RED `tui/main_test.go`: update 3 existing `classify()` call sites to 3-arg form; add `classify(0,0,3) == SyncBehindOrigin` (R-006) — breaks compile until 3.2.
- [x] 3.2 GREEN `tui/run.go`: add `SyncBehindOrigin` enum, `TargetVerdict.RepoBehindOrigin int` (-1=NA), `classify()`'s 3rd param, precedence `UPSTREAM_CHANGED > OVERLAY_NOT_DEPLOYED > REPO_BEHIND_ORIGIN > healthy` (R-006).
- [x] 3.3 RED `TestParseSyncCheckDashboard`: extend sample lines with `REPO_BEHIND_ORIGIN=<n>` and `=NA`, assert values `n` and `-1` (R-002/R-004).
- [x] 3.4 GREEN `ParseSyncCheck`: add `case "REPO_BEHIND_ORIGIN":` checking `val == "NA"` before `strconv.Atoi`.

## Phase 4: TUI Rendering (RED -> GREEN)

- [x] 4.1 RED new `tui/view_test.go`: `TestViewDashboard_ShowsOriginBehindIndicator` (R-005, n>0), `TestViewDashboard_ZeroOriginBehind_NoIndicator` (R-005, n=0), `TestViewDashboard_NeverHealthyWhileBehindOrigin` (R-006). **Note**: production code (`SyncBehindOrigin` case + counts append) was already present on this branch from PR1's defensive fix. RED was proven by temporarily reverting that case/append, confirming 2/3 new tests failed (`TestViewDashboard_ShowsOriginBehindIndicator`, `TestViewDashboard_NeverHealthyWhileBehindOrigin`), then restoring the exact original code (`git diff --stat` empty after restore) before re-running GREEN.
- [x] 4.2 GREEN `tui/view.go` `viewDashboard()`: `SyncBehindOrigin` switch case (colorAmber, "~", "Detrás de origin") and counts-line append confirmed already correct against spec R-005 (only shown when `RepoBehindOrigin > 0`, per the "Zero count renders no origin indicator" scenario — this correctly follows the spec over design.md's looser ">=0" prose in the File Changes table, noted as a deviation below). No production code changes needed; all 3 new tests pass.

## Phase 5: Integration & Regression

- [x] 5.1 Run `go test ./...` + `shellcheck`; confirm legacy `TestClassifyPrecedence`/`TestParseSyncCheckDashboard` cases unchanged. **Note**: `cd engine && go test ./...` and `cd tui && go test ./...` both pass in full (all packages `ok`, `go vet` clean on both modules). `TestClassifyPrecedence` and `TestParseSyncCheckDashboard` (both already extended in Phase 3) pass unchanged. `shellcheck` remains not installed in this environment (same pre-existing gap flagged in task 2.4); `bash -n bin/labdrian-overlay` passes as the syntax-only substitute. Flagged again as a risk for CI/verify to run shellcheck where available.
- [x] 5.2 Dogfood: run `sync-check` on this repo's own clone; confirm `REPO_BEHIND_ORIGIN` appears (success criteria). **Result**: `bash bin/labdrian-overlay sync-check --target claude` emits `VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0` — field present, no network call, no error. `REPO_BEHIND_ORIGIN=0` correctly reflects this clone's actual state (`git rev-list HEAD..origin/main --count` == 0 on `feat/sync-check-repo-behind-origin-view`, confirmed independently). The n>0 detection path itself is covered by Phase 1's git-fixture integration tests (`TestSyncCheck_ReportsRepoBehindOrigin_CachedRef`), which construct a real behind-by-N fixture.

## Phase 6: Documentation

- [x] 6.1 Update `sync-check` help block (`bin/labdrian-overlay` ~line 104) for `--check-origin`/`--fetch` and `REPO_BEHIND_ORIGIN`. **Result**: `usage()` heredoc now documents the `--check-origin`/`--fetch` flag, the `REPO_BEHIND_ORIGIN=<count|NA>` VERDICT field, the cached-vs-fetch default behavior, and the `NA` degrade condition. Verified with `bash -n` (syntax OK) and by running `bin/labdrian-overlay` with no args to confirm the heredoc renders correctly. This closes the WARNING both PR1 judgment-day judges flagged.
