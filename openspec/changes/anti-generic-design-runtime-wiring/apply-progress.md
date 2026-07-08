# Apply Progress: anti-generic-design-runtime-wiring (PR-2/4)

**Branch:** `anti-generic-design-runtime-wiring/pr-2-wiring` (base: `anti-generic-design-runtime-wiring/pr-1-markers-asset`)
**Delivery strategy:** feature-branch-chain (PR 1 -> PR 2 -> PR 3 -> PR 4)
**Chain position:** PR-2 of 4 — Unit 2: "`embeddedContract()` case + `checkRegistry`/`HasSupportedClaudeLifecycleState` recognition (Phases 2-3)"
**Status:** Phase 1 COMPLETE (from PR-1). Phase 2 COMPLETE (6/6 tasks). Phase 3 COMPLETE (4/4 tasks). Phases 4-6 NOT started — reserved for PR-3, PR-4.
**Mode:** Strict TDD (RED before GREEN, real failing tests first)

## Scope boundary (explicit)

This batch implements ONLY Phases 2 and 3 of `tasks.md`. It does NOT touch:
- Phase 4: `settings.Merger` hook pair wiring (`mergeHooks`/`removeHooks` installing the design pair) (-> PR-3, Unit 3)
- Phase 5: Isolation/drift guard + standalone `skills/_shared/anti-generic-design.md` copy (-> PR-3, Unit 3)
- Phase 6: Rebuild, `install-hooks`, live registry/status verification (-> PR-4, Unit 4)

## Tasks completed (Phase 1 — carried over from PR-1, R-101, R-103, R-104)

- [x] 1.1 RED / [x] 1.2 GREEN — `AntiGenericDesignBeginMarker`/`EndMarker` constants in `engine/propagator/propagator.go` (slug `anti-generic-design-scope`).
- [x] 1.3 — `engine/assets/anti-generic-design.md` authored (frontmatter `applies_to_phases: [sdd-tasks, sdd-apply]`).
- [x] 1.4 RED / [x] 1.5 GREEN — `//go:embed anti-generic-design.md` / `var AntiGenericDesign string` in `engine/assets/assets.go`; frontmatter parse test green.

(Full PR-1 detail preserved in Engram `sdd/anti-generic-design-runtime-wiring/apply-progress` history — this file carries the cumulative summary only to stay readable.)

## Tasks completed (Phase 2 — R-102, embeddedContract() wiring)

### [x] 2.1 RED — direct table test for embeddedContract("anti-generic-design")
- Added `TestEmbeddedContract_AntiGenericDesign` in `engine/cmd/main_test.go`
- Asserts `ok=true`, `content == assets.AntiGenericDesign`, `beginMarker/endMarker ==`
  `propagator.AntiGenericDesignBeginMarker/EndMarker`, `rowLabel == "anti-generic-design"`,
  `defaultPath == "skills/_shared/anti-generic-design.md"`
- Confirmed RED: `embeddedContract("anti-generic-design")` returned `ok=false` (unknown case)

### [x] 2.2 GREEN — `case "anti-generic-design"` in `embeddedContract()`
- Added to `engine/cmd/main.go`, mirroring the `"skill-discovery-safety"` case exactly (content,
  R-101 markers, row label, default path)

### [x] 2.3 RED — propagate resolves the new case
- Added `TestRunPropagateCore_EmbeddedDesignContract_ResolvesAndWritesPath`
- Confirmed RED: stderr contained `unknown embedded contract "anti-generic-design"`, exit 1

### [x] 2.4 RED — gate-task resolves the new case
- Added `TestGateTaskCore_EmbeddedDesignContract_Injects` (subagent_type=`sdd-apply`, in
  `applies_to_phases`)
- Confirmed RED: fail-safe `{}\n` pass-through with a stderr warning about unknown embedded
  contract

### [x] 2.5 GREEN — confirmed off the 2.2 wiring
- Both 2.3 and 2.4 pass once `embeddedContract()` resolves the case; no additional patch needed

### [x] 2.6 — `usage()` updated
- Embedded-contracts line now reads `Embedded contracts: skill-discovery-safety, anti-generic-design`

## Tasks completed (Phase 3 — checkRegistry + lifecycle-state recognition)

### [x] 3.1 RED — checkRegistry three-block coverage
- Added `TestCheckRegistry_ThreeBlockCombinations` (direct table test — no pre-existing direct
  unit test for `checkRegistry` existed; it was previously only exercised indirectly via
  `statusCore` tests)
- Table cases: all three blocks present -> `ok=true, degraded=false, note contains "scoped block"`;
  only `anti-generic-design-scope` missing -> `ok=true, degraded=true, note contains
  "anti-generic-design-scope"`
- Confirmed RED: the "only design missing" case failed — old switch treated 2-of-3 present as fully OK

### [x] 3.2 GREEN — checkRegistry recognizes the third marker
- Added `hasDesign := containsSubstring(content, propagator.AntiGenericDesignBeginMarker)`
- Replaced the old 4-branch switch with a missing-block accumulator (`missing []string`) covering
  all 8 presence combinations of the three markers
- New note format: `"present but scoped block(s) missing: <comma-separated names> (run 'overlay
  install-hooks' or propagate)"` — preserves the pre-existing "scoped block" + per-name substrings
  that other tests assert on

### [x] 3.3 RED — HasSupportedClaudeLifecycleState requires the design pair
- Added `TestHasSupportedClaudeLifecycleState_RequiresDesignPair` + `buildRootWithPairs` helper in
  `engine/settings/settings_test.go` (raw hook-entry JSON construction, since `Merger` cannot yet
  produce a design pair — that's Phase 4)
- Confirmed RED via compile failure: `undefined: settings.LabdrianDesignIdentity`

### [x] 3.4 GREEN — wired the design pair into the lifecycle-state check
- Added `embeddedDesignName = "anti-generic-design"` const, `LabdrianDesignIdentity =
  "--embedded-contract " + embeddedDesignName` const, and `HasLabdrianDesignHook` function to
  `engine/settings/settings.go`
- ANDed `HasLabdrianDesignHook(...)` into `HasSupportedClaudeLifecycleState` for both
  `UserPromptSubmit` and `PreToolUse`

## Cross-cutting consequence of 3.4 (documented deviation, not silent)

Wiring `HasLabdrianDesignHook` into `HasSupportedClaudeLifecycleState` is a real, load-bearing
contract change: any Claude install lacking the design hook pair now correctly reports `partial`
instead of `supported` (FAIL-LOUD — the new contract is no longer silently ignored, as the user
requested). But `Merger.mergeHooks` does not install that pair yet (Phase 4 is PR-3, explicitly
out of scope for this batch). This broke 3 pre-existing tests that exercise the real
`Merger.Install()` path end-to-end:

- `TestClaudeInstallWritesLifecycleHooksAndReportsSupportedStatus` (`engine/runtime/claude_test.go`)
- `TestClaudeUpdateRefreshesLifecycleAndKeepsSupportedStatus` (`engine/runtime/claude_test.go`)
- `TestRunRuntimeCore_AllTargetsStatusAllowsCodexPartialWithoutFailing` (`engine/cmd/runtime_test.go`)

**Fix applied:** added an `injectDesignHookPair` test helper (implemented independently in both
the `cmd` and `runtime` test packages, since they're separate packages with no shared test-only
import) that hand-patches a test's `settings.json` with a synthetic design hook pair keyed on
`settings.LabdrianDesignIdentity`, called immediately after `Install()`/`runtime install` in those
3 tests. This keeps each test isolated to the behavior it actually verifies (install path, update
path, codex-partial exemption) without depending on Phase 4's not-yet-written `mergeHooks` wiring.
**No Phase 4 production code (`mergeHooks`/`removeHooks`) was touched — only test fixtures.**

Also updated `TestStatusCore_RegistryScopedBlockPresent` (`engine/cmd/main_test.go`) to include
the third marker block in its fixture, since `checkRegistry` now requires all three for the
non-degraded "scoped block present" result.

Flagged as new task **4.5** in `tasks.md` for PR-3 to reconcile: once Phase 4 lands, remove the
`injectDesignHookPair` hand-patch calls so those 3 tests exercise the real `Install()` path again.

## TDD Cycle Evidence

| Task | RED | GREEN | REFACTOR |
|------|-----|-------|----------|
| 2.1/2.2 (embeddedContract case) | `TestEmbeddedContract_AntiGenericDesign` failed — `ok=false` | `case "anti-generic-design"` added; test green | None needed |
| 2.3/2.4/2.5 (propagate + gate-task resolve) | Both tests failed — "unknown embedded contract" / fail-safe `{}` | Green off the 2.2 wiring alone, no separate patch | None needed |
| 2.6 (usage line) | N/A — doc/CLI-help task, no test | Manually verified string change | None needed |
| 3.1/3.2 (checkRegistry) | `TestCheckRegistry_ThreeBlockCombinations/only_anti-generic-design-scope_missing` failed — `degraded=false`, wrong note | Missing-block accumulator added; both table cases green, plus all pre-existing status-level tests re-verified green | Replaced 4-branch switch with an accumulator loop for maintainability across future contracts |
| 3.3/3.4 (HasSupportedClaudeLifecycleState) | `TestHasSupportedClaudeLifecycleState_RequiresDesignPair` failed to compile — `undefined: settings.LabdrianDesignIdentity` | Const + hook function + AND-wiring added; test green | Fixed 3 downstream tests broken by the contract change (see Cross-cutting consequence above) — this is intentional GREEN-phase cleanup of dependent tests, not scope creep into Phase 4 |

## Verification

- `cd engine && go build ./...` — OK
- `cd engine && go vet ./...` — OK
- `cd engine && go test ./...` — ALL PASS (assets, cmd, gadu, gate, installer, prespec,
  propagator, runtime, settings, skills)
- `gofmt -l` on touched files — clean, except `engine/cmd/main_test.go` which carries a
  **pre-existing** unrelated gofmt issue (confirmed via `git stash` diff) already noted as
  out-of-scope in PR-1's apply-progress; not introduced by this batch
- `git diff --stat engine/` confirms only `cmd/main.go`, `cmd/main_test.go`,
  `cmd/runtime_test.go`, `settings/settings.go`, `settings/settings_test.go`,
  `runtime/claude_test.go` touched — no Phase 4-6 production files (`mergeHooks`, `removeHooks`,
  `skills/_shared/anti-generic-design.md`) touched

## Files changed

| File | Action | What Was Done |
|------|--------|----------------|
| `engine/cmd/main.go` | Modified | Added `case "anti-generic-design"` to `embeddedContract()`; updated `usage()`; rewrote `checkRegistry()`'s marker-presence switch to a 3-marker accumulator |
| `engine/cmd/main_test.go` | Modified | Added `TestEmbeddedContract_AntiGenericDesign`, `TestRunPropagateCore_EmbeddedDesignContract_ResolvesAndWritesPath`, `TestGateTaskCore_EmbeddedDesignContract_Injects`, `TestCheckRegistry_ThreeBlockCombinations`; updated `TestStatusCore_RegistryScopedBlockPresent` fixture; added `assets` import |
| `engine/cmd/runtime_test.go` | Modified | Added `injectDesignHookPair` helper; used it in `TestRunRuntimeCore_AllTargetsStatusAllowsCodexPartialWithoutFailing` |
| `engine/settings/settings.go` | Modified | Added `embeddedDesignName`, `LabdrianDesignIdentity` consts; added `HasLabdrianDesignHook`; wired into `HasSupportedClaudeLifecycleState` |
| `engine/settings/settings_test.go` | Modified | Added `TestHasSupportedClaudeLifecycleState_RequiresDesignPair` + `buildRootWithPairs` helper |
| `engine/runtime/claude_test.go` | Modified | Added `injectDesignHookPair` helper; used it in `TestClaudeInstallWritesLifecycleHooksAndReportsSupportedStatus` and `TestClaudeUpdateRefreshesLifecycleAndKeepsSupportedStatus` |
| `openspec/changes/anti-generic-design-runtime-wiring/tasks.md` | Modified | Marked Phase 2 (2.1-2.6) and Phase 3 (3.1-3.4) `[x]`; added new task 4.5 (reconcile test workaround); documented cross-cutting consequence inline |
| `openspec/changes/anti-generic-design-runtime-wiring/apply-progress.md` | Modified | This file — cumulative PR-1 + PR-2 progress record |

## Deviations from design

Design.md's Phase 3 scope ("extend checkRegistry + HasSupportedClaudeLifecycleState for
status/check parity") was implemented literally, per the explicit user instruction. The one
undocumented-in-design consequence — that wiring `HasSupportedClaudeLifecycleState` ahead of
Phase 4's installer creates a real "partial" status window for the whole feature-branch-chain
until PR-3 lands — is expected and safe under feature-branch-chain semantics (this branch is
never merged to `main` on its own; only the aggregate chain lands once all 4 PRs are complete).
Fixed with test-only workarounds (`injectDesignHookPair`) rather than touching Phase 4 production
code, per the explicit "do not touch Phases 4-6" instruction. See "Cross-cutting consequence"
section above for full detail.

## Remaining tasks (PR-3, PR-4 — NOT in this batch)

- [ ] Phase 4 (4.1-4.5): Merger hook wiring + reconcile the `injectDesignHookPair` test workaround — Unit 3, PR-3
- [ ] Phase 5 (5.1-5.5): Isolation, drift guard, standalone copy, R-106 verification — Unit 3, PR-3
- [ ] Phase 6 (6.1-6.3): Rebuild, install-hooks, live verification — Unit 4, PR-4

## Status

Phase 1 (5/5, carried from PR-1) + Phase 2 (6/6) + Phase 3 (4/4) = 15/15 tasks complete across
PR-1 and PR-2. This is PR-2 of 4 in the feature-branch-chain. Next batch (PR-3) targets this
branch (`anti-generic-design-runtime-wiring/pr-2-wiring`) and implements Phase 4 + Phase 5
(Unit 3), including reconciling the `injectDesignHookPair` test workaround once the real Merger
wiring lands. Ready for `sdd-verify` on the Phase 2-3 slice.
