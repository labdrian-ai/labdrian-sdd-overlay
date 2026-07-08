# Apply Progress: anti-generic-design-runtime-wiring (PR-3/4)

**Branch:** `anti-generic-design-runtime-wiring/pr-3-merger-standalone` (base: `anti-generic-design-runtime-wiring/pr-2-wiring`)
**Delivery strategy:** feature-branch-chain (PR 1 -> PR 2 -> PR 3 -> PR 4)
**Chain position:** PR-3 of 4 — Unit 3: "Merger hook pair + isolation/drift guards + standalone copy (Phases 4-5)"
**Status:** Phase 1 COMPLETE (from PR-1). Phase 2 COMPLETE (from PR-2). Phase 3 COMPLETE (from PR-2). Phase 4 COMPLETE (6/6 tasks incl. 4.5). Phase 5 COMPLETE (5/5 tasks). **Phase 6 NOT started — the ONLY remaining phase, reserved for PR-4.**
**Mode:** Strict TDD (RED before GREEN, real failing tests first)

## Scope boundary (explicit)

This batch implements ONLY Phases 4 and 5 of `tasks.md`. It does NOT touch:
- Phase 6: Rebuild `~/.claude/bin/gentle-ai-overlay`, `install-hooks`, live registry/status verification (-> PR-4, Unit 4, the last PR in the chain)

## Tasks completed (Phases 1-3 — carried over from PR-1/PR-2)

- [x] Phase 1 (1.1-1.5, R-101/R-103/R-104): `AntiGenericDesignBeginMarker`/`EndMarker` constants; `engine/assets/anti-generic-design.md` authored + embedded via `//go:embed`.
- [x] Phase 2 (2.1-2.6, R-102): `case "anti-generic-design"` in `embeddedContract()`; `propagate`/`gate-task` resolve the new case; `usage()` updated.
- [x] Phase 3 (3.1-3.4): `checkRegistry` 3-marker accumulator; `HasLabdrianDesignHook`/`LabdrianDesignIdentity`/`embeddedDesignName` added and ANDed into `HasSupportedClaudeLifecycleState`.

(Full PR-1/PR-2 detail preserved in Engram `sdd/anti-generic-design-runtime-wiring/apply-progress` history — this file carries the cumulative summary only to stay readable.)

## Tasks completed (Phase 4 — R-105, Merger hook wiring)

### [x] 4.1 RED — mergeHooks installs a third pair, idempotent
- Extended `TestMerge_Idempotent` in `engine/settings/settings_test.go`: comment updated from "TWO pairs" to "THREE pairs", assertions changed from `!= 2` to `!= 3`, plus new per-key design-entry-count-1 checks via `settings.LabdrianDesignIdentity`.
- Confirmed RED: `UserPromptSubmit: expected exactly 3 entries ... got 2`; `expected exactly 1 design entry, got 0` (both keys).

### [x] 4.2 GREEN — designIdentity / isDesignEntry / buildDesign*Entry wired into mergeHooks
- Added `designIdentity = "--embedded-contract " + embeddedDesignName` const in `engine/settings/settings.go` (mirrors `safetyIdentity`).
- Added `isDesignEntry` method (mirrors `isSafetyEntry`).
- Added `buildDesignUserPromptSubmitEntry` / `buildDesignPreToolUseEntry` (mirror the safety builders exactly): `propagate --embedded-contract anti-generic-design` / `gate-task --embedded-contract anti-generic-design --contract-path "$HOME/.claude/skills/_shared/anti-generic-design.md"`, same `command -v ... &>/dev/null && ... || true` fail-safe guard and `matcher: "Agent"` shape as the precedent pairs.
- Wired both into `mergeHooks` as a third dedup-checked pair.
- 4.1 confirmed GREEN off this wiring.

### [x] 4.3 RED — removeHooks strips the design pair by identity
- Extended `TestUninstall_PreservesSameBinaryThirdPartyHooksWithoutLabdrianIdentity` (TC-SET-6a) with an `ownedDesignCommand` fixture entry added to both `UserPromptSubmit` and `PreToolUse`, plus assertions that `countLabdrianIdentityEntries(..., LabdrianDesignIdentity)` is 0 after `Uninstall()` — mirrors the existing minimalism/safety assertions in the same test, while the pre-existing third-party-survival assertions stay unchanged.
- Confirmed RED: `design-owned UserPromptSubmit hook should be removed by identity` (both keys) — pre-4.4 `removeHooks` did not check `isDesignEntry`, so the hand-injected design entries survived `Uninstall()`.

### [x] 4.4 GREEN — removeHooks matches isDesignEntry
- Extended the `removeHooks` filter condition from `m.isMinimalismEntry(e) || m.isSafetyEntry(e)` to also include `|| m.isDesignEntry(e)`.
- 4.3 confirmed GREEN off this wiring; third-party-survival assertions in the same test still pass (no regression).

### [x] 4.5 — Reconciled the injectDesignHookPair test-fixture workaround (tracked since PR-2)
- Removed the `injectDesignHookPair` helper function and all 3 call sites:
  - `engine/runtime/claude_test.go`: `TestClaudeInstallWritesLifecycleHooksAndReportsSupportedStatus` (also extended with explicit design-hook-present assertions for both `UserPromptSubmit`/`PreToolUse`, since the real `Install()` now produces the pair) and `TestClaudeUpdateRefreshesLifecycleAndKeepsSupportedStatus`.
  - `engine/cmd/runtime_test.go`: `TestRunRuntimeCore_AllTargetsStatusAllowsCodexPartialWithoutFailing`.
- Removed the now-unused `github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings` import from `engine/cmd/runtime_test.go` (it was ONLY used inside `injectDesignHookPair`); `engine/runtime/claude_test.go` keeps its `settings` import since it is still used elsewhere (minimalism/safety identity checks).
- All 3 tests now exercise the real `Merger.Install()`/`Update()`/`runtime install` path end-to-end for the design pair, with no hand-patched fixture.

## Cross-cutting fallout of 4.2 (documented, not silent)

Wiring the design pair into `mergeHooks` raised the per-key owned-hook count from 2 to 3. This broke 4 pre-existing tests that hardcoded the "2 pairs" count:
- `TestMerge_Idempotent` (`engine/settings/settings_test.go`) — updated `!= 2` -> `!= 3`, comment "TWO pairs" -> "THREE pairs", added design-entry-count assertions.
- `TestUninstall_CountIsZeroAfterInstall` (`engine/settings/settings_test.go`) — precondition `!= 2` -> `!= 3`.
- `TestSchema_InstallTwice_Idempotent` (`engine/settings/settings_test.go`) — `!= 2` -> `!= 3`, comment updated.
- `TestRunMergeSettings_Idempotent` (`engine/cmd/main_test.go`) — `!= 2` -> `!= 3`, comment updated.

This is the same category of fallout PR-2 documented for 3.4 (`HasSupportedClaudeLifecycleState` requiring the design pair): a real, load-bearing behavior change (mergeHooks now installs 3 pairs, not 2) correctly breaks tests that hardcoded the old count. Fixed by updating the counts to match new correct behavior, not by weakening the assertions.

## Tasks completed (Phase 5 — Isolation, drift guard, R-106 verification)

### [x] 5.1 RED/lock — three-block isolation + idempotency
- Added `TestAntiGenericDesignPropagate_ThreeBlockIsolationAndIdempotency` in `engine/propagator/propagator_test.go`: builds a registry with pre-existing, correctly-scoped minimalism-contract and skill-discovery-safety blocks, then runs `Propagate` with the `anti-generic-design` `Config`. Asserts: (a) both pre-existing blocks survive byte-identical (`strings.Contains` on the exact original block text), (b) the new anti-generic-design block is added with its own markers, (c) exactly 3 `BEGIN:` markers coexist, (d) re-propagating the same design block on the first run's output is a no-op (`changed=false`, byte-identical output).
- **This is a LOCK test, not a true RED gate**: `propagator.Propagate()` was already written generically across Phase 1-3 to operate on an arbitrary `Config{BeginMarker, EndMarker, RowLabel}`, so this test PASSED on first run with zero production code changes. It characterizes and locks in already-correct multi-block-coexistence behavior rather than driving new implementation — noted explicitly per the "no silent fallback" TDD rule rather than mislabeling it as a RED-then-GREEN cycle.

### [x] 5.2 — Standalone copy created
- `cp engine/assets/anti-generic-design.md skills/_shared/anti-generic-design.md`; verified via `diff` (zero output — byte-identical).

### [x] 5.3 RED — drift guard test
- Added `TestAntiGenericDesignStandaloneCopyMatchesEmbeddedAsset` in `engine/assets/assets_test.go`, plus a `repoRoot(t)` helper (mirrors the existing pattern in `engine/gadu/gadu_test.go`) to resolve `skills/_shared/anti-generic-design.md` from the repo root. Asserts `os.ReadFile(standalonePath)` content `== assets.AntiGenericDesign` (string equality on the full embedded asset).
- Confirmed RED (run BEFORE 5.2): `read standalone copy .../skills/_shared/anti-generic-design.md: ... no such file or directory`.

### [x] 5.4 GREEN — reconciled
- 5.2's `cp` made the guard pass; confirmed via `go test ./assets/... -run TestAntiGenericDesignStandaloneCopyMatchesEmbeddedAsset -v` → PASS.

### [x] 5.5 — R-106 verification (no code change)
- `git log --oneline -- skills/anti-generic-design/SKILL.md` → exactly one commit (`64165e5 feat(skills): add anti-generic-design skill`), its creation.
- `git diff --stat 64165e5 -- skills/anti-generic-design/SKILL.md` → empty (no changes since creation, across all of PR-1/PR-2/PR-3).
- `git status --short skills/anti-generic-design/` → empty (no uncommitted changes).
- File confirmed present on disk and unchanged: `skills/anti-generic-design/SKILL.md` remains invocable by trigger/name, independent of the new embedded contract.

## TDD Cycle Evidence

| Task | RED | GREEN | REFACTOR |
|------|-----|-------|----------|
| 4.1/4.2 (mergeHooks design pair) | `TestMerge_Idempotent` extended — failed: got 2 entries (want 3), 0 design entries (want 1) | `designIdentity`/`isDesignEntry`/`buildDesign*Entry` added, wired into `mergeHooks`; test green | None needed — mirrors existing safety-pair pattern exactly |
| 4.3/4.4 (removeHooks design pair) | `TestUninstall_PreservesSameBinaryThirdPartyHooksWithoutLabdrianIdentity` extended — failed: design entries survived Uninstall | `removeHooks` filter extended with `\|\| m.isDesignEntry(e)`; test green, third-party-survival assertions unaffected | None needed |
| 4.5 (test workaround reconciliation) | N/A — cleanup task, not a new-behavior RED/GREEN cycle | Removed `injectDesignHookPair` + 3 call sites; removed now-unused `settings` import in `cmd/runtime_test.go`; re-ran full suite green | Extended the Claude-install test with explicit design-hook assertions instead of leaving it as a bare pass-through |
| (fallout) 4 pre-existing count-2 tests | N/A — pre-existing tests broken by real Phase-4 behavior change (documented above) | Updated `!= 2` → `!= 3` in `TestMerge_Idempotent`, `TestUninstall_CountIsZeroAfterInstall`, `TestSchema_InstallTwice_Idempotent`, `TestRunMergeSettings_Idempotent`; all green | Comments updated to describe "THREE pairs" / "Three pairs install" |
| 5.1 (propagator 3-block isolation) | Written as RED/lock per task label; PASSED immediately (no code change) — `propagator.Propagate()` already generic since Phase 1-3 | N/A — no implementation needed | None needed |
| 5.2/5.3/5.4 (standalone copy + drift guard) | `TestAntiGenericDesignStandaloneCopyMatchesEmbeddedAsset` — failed: file not found | `cp` created the byte-identical standalone copy; test green | None needed |
| 5.5 (R-106 verification) | N/A — verification-only task, no test | `git log`/`git diff`/`git status` confirm zero drift since the skill's creation commit | N/A |

### Test Summary
- **Total tests written/extended**: 6 (2 extended existing settings tests, 1 new propagator lock test, 1 new assets drift-guard test, 2 runtime/cmd tests de-workaround'ed and strengthened with real assertions)
- **Total tests passing**: all (see Verification below)
- **Layers used**: Unit (all — Go `testing` package, no integration/E2E layer in this repo)
- **Approval tests** (refactoring): None — no refactoring-of-existing-behavior tasks in this batch beyond the documented count-2→3 fallout, which are correctness fixes, not approval tests
- **Pure functions created**: `isDesignEntry`, `buildDesignUserPromptSubmitEntry`, `buildDesignPreToolUseEntry` (all pure — same style as their `isSafetyEntry`/`buildSafety*Entry` precedents)

## Verification

- `cd engine && go build ./...` — OK
- `cd engine && go vet ./...` — OK
- `cd engine && go test ./...` — ALL PASS (assets, cmd, gadu, gate, installer, prespec, propagator, runtime, settings, skills)
- `gofmt -l` on touched files — clean, except `engine/cmd/main_test.go`, which carries the SAME pre-existing unrelated gofmt issue already noted out-of-scope in PR-1's and PR-2's apply-progress (confirmed via `git stash` + `gofmt -l` before this batch's changes) — not introduced by this batch
- `git diff --stat -- engine/ skills/` confirms only Phase 4-5 files touched: `engine/settings/settings.go`, `engine/settings/settings_test.go`, `engine/cmd/main_test.go`, `engine/cmd/runtime_test.go`, `engine/runtime/claude_test.go`, `engine/propagator/propagator_test.go`, `engine/assets/assets_test.go` (all modified), plus `skills/_shared/anti-generic-design.md` (new file) — no Phase 6 production files (build scripts, live `~/.claude/bin/gentle-ai-overlay`, live `install-hooks` invocation) touched
- Total diff: 266 insertions / 127 deletions across 7 modified files + 1 new file — within the chained-PR review budget

## Files changed

| File | Action | What Was Done |
|------|--------|----------------|
| `engine/settings/settings.go` | Modified | Added `designIdentity` const, `isDesignEntry` method, `buildDesignUserPromptSubmitEntry`, `buildDesignPreToolUseEntry`; wired all into `mergeHooks`/`removeHooks` |
| `engine/settings/settings_test.go` | Modified | Extended `TestMerge_Idempotent` and `TestUninstall_PreservesSameBinaryThirdPartyHooksWithoutLabdrianIdentity` with design-pair RED assertions; fixed count-2→3 fallout in `TestUninstall_CountIsZeroAfterInstall` and `TestSchema_InstallTwice_Idempotent` |
| `engine/cmd/main_test.go` | Modified | Fixed count-2→3 fallout in `TestRunMergeSettings_Idempotent` |
| `engine/cmd/runtime_test.go` | Modified | Removed `injectDesignHookPair` helper + call site; removed unused `settings` import |
| `engine/runtime/claude_test.go` | Modified | Removed `injectDesignHookPair` helper + 2 call sites; extended install test with explicit design-hook assertions |
| `engine/propagator/propagator_test.go` | Modified | Added `TestAntiGenericDesignPropagate_ThreeBlockIsolationAndIdempotency` (lock test) |
| `engine/assets/assets_test.go` | Modified | Added `repoRoot` helper + `TestAntiGenericDesignStandaloneCopyMatchesEmbeddedAsset` drift guard |
| `skills/_shared/anti-generic-design.md` | Created | Standalone deployed copy, byte-identical to `engine/assets/anti-generic-design.md` |
| `openspec/changes/anti-generic-design-runtime-wiring/tasks.md` | Modified | Marked Phase 4 (4.1-4.5) and Phase 5 (5.1-5.5) `[x]`; documented cross-cutting fallout inline; marked Unit 3 COMPLETE |
| `openspec/changes/anti-generic-design-runtime-wiring/apply-progress.md` | Modified | This file — cumulative PR-1 + PR-2 + PR-3 progress record |

## Deviations from design

None beyond the documented, expected fallout above (count-2→3 test updates — a direct, necessary consequence of the design's own Phase 4 scope, not a deviation from it). Design.md's Phase 4 hook command lines (`propagate ... --embedded-contract anti-generic-design`, `gate-task --embedded-contract anti-generic-design --contract-path "$HOME/.claude/skills/_shared/anti-generic-design.md"`) were implemented verbatim, mirroring the skill-discovery-safety precedent exactly as specified.

The 5.1 test being a "lock" rather than a true RED gate is noted explicitly rather than silently reported as a standard RED→GREEN cycle — the underlying `Propagate()` generality was already delivered in Phase 1-3, so Phase 5's marginal contribution here is a regression guard, not new behavior.

## Remaining tasks (PR-4 — NOT in this batch)

- [ ] Phase 6 (6.1-6.3): Rebuild `~/.claude/bin/gentle-ai-overlay`, run `merge-settings`/`install-hooks`, run `propagate --embedded-contract anti-generic-design` against the live registry, run `overlay status`/`overlay check`, run `go test ./... && go vet ./...` in `engine/` — Unit 4, PR-4 (the LAST PR in the feature-branch-chain; this is the only phase left in the whole `anti-generic-design-runtime-wiring` change)

## Status

Phase 1 (5/5) + Phase 2 (6/6) + Phase 3 (4/4) + Phase 4 (6/6, incl. 4.5) + Phase 5 (5/5) = 26/29 tasks complete across PR-1, PR-2, and PR-3. This is PR-3 of 4 in the feature-branch-chain. **Only Phase 6 (3 tasks) remains**, reserved for PR-4 — the final PR in the chain, which rebuilds the binary, runs the real installer/propagator against a live registry, and confirms `overlay status`/`overlay check` report the new contract healthy. Ready for `sdd-verify` on the Phase 4-5 slice.
