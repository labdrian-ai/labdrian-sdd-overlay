# Tasks: Runtime-wire the anti-generic-design contract

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~450-520 (2 near-duplicate md assets ~90 lines each + Go source ~120 + Go tests ~175) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 -> PR 2 -> PR 3 -> PR 4 (feature-branch-chain) |
| Delivery strategy | ask-on-risk (default; orchestrator must confirm) |
| Chain strategy | feature-branch-chain (confirmed — PR-1 applied on anti-generic-design-runtime-wiring/pr-1-markers-asset; PR-2 applied on anti-generic-design-runtime-wiring/pr-2-wiring) |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Marker pair + embedded asset + frontmatter test (Phase 1) | PR 1 | base = tracker branch; unused until Unit 2 wires it — COMPLETE |
| 2 | `embeddedContract()` case + `checkRegistry`/`HasSupportedClaudeLifecycleState` recognition (Phases 2-3) | PR 2 | base = PR 1 branch; depends on Unit 1 — COMPLETE |
| 3 | Merger hook pair + isolation/drift guards + standalone copy (Phases 4-5) | PR 3 | base = PR 2 branch; depends on Unit 2 |
| 4 | Rebuild, `install-hooks`, live registry/status verification (Phase 6) | PR 4 | base = PR 3 branch; depends on Unit 3 |

## Phase 1: Foundation — marker pair + embedded asset (R-101, R-103, R-104) — COMPLETE (PR-1)

- [x] 1.1 RED: `engine/propagator/propagator_test.go` — assert `AntiGenericDesignBeginMarker`/`EndMarker` exist and are distinct from `BeginMarker`/`EndMarker` and `DiscoverySafetyBeginMarker`/`EndMarker` (R-101).
- [x] 1.2 GREEN: add the two constants to `engine/propagator/propagator.go` (slug `anti-generic-design-scope`) (R-101).
- [x] 1.3 Author `engine/assets/anti-generic-design.md`: frontmatter `applies_to_phases: [sdd-tasks, sdd-apply]` + `excluded_phases`/`injection_point`, advisory-scope note, body distilled verbatim from `skills/anti-generic-design/SKILL.md` (R-103, R-104).
- [x] 1.4 RED: test asserting `propagator.ParseFrontmatter(assets.AntiGenericDesign)` yields `AppliesTo == ["sdd-tasks","sdd-apply"]`, no error (R-104).
- [x] 1.5 GREEN: add `//go:embed anti-generic-design.md` / `var AntiGenericDesign string` to `engine/assets/assets.go`; confirm 1.4 passes (R-103).

## Phase 2: `embeddedContract()` wiring (R-102) — COMPLETE (PR-2)

- [x] 2.1 RED: `engine/cmd/main_test.go` table test — `embeddedContract("anti-generic-design")` must return `ok=true`, `content=assets.AntiGenericDesign`, R-101 markers, `rowLabel="anti-generic-design"`, `defaultPath="skills/_shared/anti-generic-design.md"` (R-102). Implemented as `TestEmbeddedContract_AntiGenericDesign`.
- [x] 2.2 GREEN: add `case "anti-generic-design"` to `embeddedContract()` in `engine/cmd/main.go` (R-102).
- [x] 2.3 RED: test `propagate --embedded-contract anti-generic-design` does not hit "unknown embedded contract"; row Path cell reads `skills/_shared/anti-generic-design.md` (R-102). Implemented as `TestRunPropagateCore_EmbeddedDesignContract_ResolvesAndWritesPath`.
- [x] 2.4 RED: test `gate-task --embedded-contract anti-generic-design` on a well-formed PreToolUse/Agent payload (subagent_type=sdd-apply) emits an injection referencing the contract path, not the `{}` fail-safe (R-102). Implemented as `TestGateTaskCore_EmbeddedDesignContract_Injects`.
- [x] 2.5 GREEN: confirmed 2.3/2.4 pass off the 2.2 wiring — no additional patch needed.
- [x] 2.6 Updated `usage()` in `engine/cmd/main.go` — embedded-contracts line now lists `skill-discovery-safety, anti-generic-design`.

## Phase 3: `checkRegistry` + lifecycle-state recognition — COMPLETE (PR-2)

- [x] 3.1 RED: extended `checkRegistry` coverage in `main_test.go` via new `TestCheckRegistry_ThreeBlockCombinations` (direct table test — no pre-existing direct table test for `checkRegistry` existed) — all three blocks present -> ok, not degraded; only `anti-generic-design-scope` missing -> ok, degraded, note names it.
- [x] 3.2 GREEN: in `engine/cmd/main.go` `checkRegistry()`, added `hasDesign` check on `propagator.AntiGenericDesignBeginMarker`; replaced the old 4-branch switch with a missing-block accumulator covering all 8 presence combinations, note format `"present but scoped block(s) missing: <names> (run 'overlay install-hooks' or propagate)"`.
- [x] 3.3 RED: `engine/settings/settings_test.go` — added `TestHasSupportedClaudeLifecycleState_RequiresDesignPair` (+ `buildRootWithPairs` helper) asserting `HasSupportedClaudeLifecycleState` returns false when the design pair is absent even with minimalism+safety present; true only when all three pairs exist. Confirmed RED via compile failure `undefined: settings.LabdrianDesignIdentity`.
- [x] 3.4 GREEN: added `embeddedDesignName` const, `LabdrianDesignIdentity` const, `HasLabdrianDesignHook` in `engine/settings/settings.go`; ANDed it into `HasSupportedClaudeLifecycleState` for both hook keys.

**Cross-cutting consequence (documented, not silent):** wiring `HasLabdrianDesignHook` into
`HasSupportedClaudeLifecycleState` broke 3 pre-existing tests that exercise the real
`Merger.Install()` path (`TestClaudeInstallWritesLifecycleHooksAndReportsSupportedStatus`,
`TestClaudeUpdateRefreshesLifecycleAndKeepsSupportedStatus`,
`TestRunRuntimeCore_AllTargetsStatusAllowsCodexPartialWithoutFailing`) because `mergeHooks` does
not install the design pair yet (that's Phase 4, PR-3). Fixed with a local
`injectDesignHookPair` test helper (added independently in both `engine/cmd/runtime_test.go` and
`engine/runtime/claude_test.go`) that hand-patches `settings.json` with a synthetic design pair so
those 3 tests stay isolated to the behavior they actually verify. No Phase 4 production code was
touched. See new task 4.5 below to reconcile once Phase 4 lands.

## Phase 4: Merger hook wiring (R-105)

- [ ] 4.1 RED: `settings_test.go` — `mergeHooks` installs a third UserPromptSubmit/PreToolUse pair (`gate-task`/`propagate --embedded-contract anti-generic-design`, same `command -v ... || true` shape), idempotent on re-run.
- [ ] 4.2 GREEN: add `designIdentity`, `isDesignEntry`, `buildDesignUserPromptSubmitEntry`, `buildDesignPreToolUseEntry` to `engine/settings/settings.go` (reuse the `embeddedDesignName` const already added in Phase 3); wire the pair into `mergeHooks`.
- [ ] 4.3 RED: `settings_test.go` — `removeHooks` strips the design pair by identity, leaves minimalism+safety entries unchanged.
- [ ] 4.4 GREEN: extend `removeHooks` to also match `isDesignEntry`.
- [ ] 4.5 (new, discovered during PR-2 apply) Reconcile the `injectDesignHookPair` test-fixture workaround in `engine/cmd/runtime_test.go` and `engine/runtime/claude_test.go`: once `mergeHooks` installs the real pair, remove the hand-patch calls from the 3 tests listed above so they exercise the real `Install()` path end-to-end again.

## Phase 5: Isolation, drift guard, R-106 verification

- [ ] 5.1 RED/lock: `propagator_test.go` — propagating `anti-generic-design` after minimalism+safety already exist leaves both pre-existing blocks byte-identical; re-propagate is idempotent (R-101 scenarios).
- [ ] 5.2 Create `skills/_shared/anti-generic-design.md` — deployed copy, byte-identical to `engine/assets/anti-generic-design.md`.
- [ ] 5.3 RED: add a Go test (e.g. `engine/assets/assets_test.go`) asserting `assets.AntiGenericDesign` and the file contents of `skills/_shared/anti-generic-design.md` are byte-identical — drift guard for the two-copy design risk.
- [ ] 5.4 GREEN: reconcile 5.2 against 5.3 until the guard passes.
- [ ] 5.5 Verify (no code change): `skills/anti-generic-design/SKILL.md` is byte-identical to its pre-change git ref (R-106).

## Phase 6: Build, install, live verification

- [ ] 6.1 Rebuild `~/.claude/bin/gentle-ai-overlay` and run `merge-settings` (`overlay install-hooks`) — Merger installs the third hook pair; no manual `settings.json` edits.
- [ ] 6.2 Run `propagate --embedded-contract anti-generic-design` (or `overlay apply`) against the live registry; confirm `anti-generic-design-scope` block appears in `.atl/skill-registry.md`.
- [ ] 6.3 Run `overlay status`/`overlay check` — new block reported healthy; run `go test ./... && go vet ./...` in `engine/`.
