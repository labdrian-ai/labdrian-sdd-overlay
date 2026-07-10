# Apply Progress: fix-propagate-foreign-writer-race

**Branch:** `fix/skill-registry-scope-block-wipe`
**Status:** COMPLETE — 14 / 14 tasks
**Mode:** Strict TDD (RED before GREEN, per phase)

This supersedes the prior apply-progress content in this file, which recorded
a NOT-STARTED / 0-of-12-tasks attempt. That finding is now stale: this pass
implemented the full read-side race classifier on top of the already-landed
write-side fix (#1889, `runPropagateVerified` + `maxPropagateWriteAttempts`),
per `design.md`.

## Summary

Extended `runPropagateVerified` (engine/cmd/main.go) with a `classifyReadRace`
classifier that recognizes two READ-side race signals inside the existing
bounded `maxPropagateWriteAttempts` loop:

- A torn/empty registry read (core's fail-loud `exit(1)` guard) is now
  retried instead of forwarded as an immediate hard failure.
- A transient absent-registry read (`os.IsNotExist`, no `--require-registry`)
  is now retried instead of forwarded as a false-success no-op.
- Genuine hard failures (bad args, unreadable contract, broken frontmatter)
  still forward immediately with zero retries — unchanged.
- `runPropagateCore` was NOT touched beyond two message-string call sites
  (see below), preserving its ~20 stateless-mock tests untouched in behavior.

## TDD Cycle Evidence

| Task | RED | GREEN | REFACTOR | Notes |
|------|-----|-------|----------|-------|
| 1.1/1.2 `emptyRegistryErrMsg` + `TestEmptyRegistryMsgPinned` | Compile fail confirmed (`undefined: emptyRegistryErrMsg`) | Const added; `runPropagateCore`'s `Fprintln` at the empty-registry guard now uses it; test passes | n/a (single const) | Guard test runs core's real empty-registry path and asserts exact (trimmed) string equality |
| 1.3/1.4 `registryAbsentNoopMarker` + `TestAbsentNoopMarkerPinned` | Compile fail confirmed (`undefined: registryAbsentNoopMarker`) | Const added; `runPropagateCore`'s absent-registry `Fprintf` reconstructed byte-identically via `%s` substitution; test passes | n/a | Guard test runs core's real absent-registry path and asserts substring containment |
| 2.1/2.2 `readRaceKind` + `classifyReadRace` + `TestClassifyReadRace` | Compile fail confirmed (5 undefined identifiers) | Type, 3 consts, and pure classifier function added; all 5 table cases pass | Function kept small/pure, no refactor needed | Table covers: empty→raceEmptyRegistry, absent→raceAbsentRegistry, unrelated hard-failure w/ coreExit!=-1→raceNone, wrote=true→raceNone (even with empty stderr), genuine no-op→raceNone |
| 3.1 `TestRunPropagateVerified_RetriesEmptyRegistryThenSucceeds` | Failed for the right reason: `exit=1` (torn read hit the still-unwired hard-failure branch) | Passes after 3.6 wiring | n/a | Shared mutable `diskState` + one-shot `tornOnce` flag models the foreign writer's rewrite window physically, not a call-count mock |
| 3.2 `TestRunPropagateVerified_EmptyRegistryExhaustsAttemptsFailsLoud` | Failed for the right reason: registry read count was 1, not `maxPropagateWriteAttempts` (no retry loop entered) | Passes after 3.6 wiring | n/a | Registry always torn; asserts exactly 3 reads before fail-loud `exit(1)` with non-empty stderr |
| 3.3 `TestRunPropagateVerified_RetriesTransientAbsentThenSucceeds` | Failed for the right reason: reported false-success no-op immediately, `wroteAny=false` | Passes after 3.6 wiring | n/a | Shared mutable `diskState` + one-shot `absentOnce` flag; proves propagate PROCEEDS (writes) once the registry reappears, not a silent no-op |
| 3.4 `TestRunPropagateVerified_PersistentAbsentStillNoOpsExitZero` | Failed for the right reason: registry read count was 1, not 3 | Passes after 3.6 wiring | n/a | Registry always absent; asserts exactly 3 reads, then the *existing* exit-0 no-op message, unchanged |
| 3.5 `TestRunPropagateVerified_HardFailureStillNoRetry` | Already GREEN before 3.6 (regression guard, proves baseline behavior for a same-exit-code-1-but-unrelated hard failure) | Stayed GREEN after 3.6 wiring | n/a | Broken contract frontmatter fails before the registry is ever read (registry read count = 0), proving the classifier's exact-match sentinel comparison does not accidentally net-classify unrelated `exit(1)`s as retryable |
| 3.6 Wire `classifyReadRace` into the loop | — (implementation task) | `go build`, `go vet`, full `engine/cmd` suite green; all of 3.1-3.5 pass | Switch statement placed immediately after `runPropagateCore` returns, before the existing `coreExit != -1` forward, per design.md's exact branch order | `continue` naturally discards that iteration's `outBuf`/`errBuf` (declared fresh per loop iteration) |
| 4.1 Regression suite | n/a | `go test ./cmd/...` → 91/91 subtests pass, 0 failures (includes the 4 pre-existing write-side tests and ~20 `TestRunPropagateCore_*` tests) | n/a | `go build ./...`, `go vet ./...`, `go test ./...` all green across every package |
| 4.2 Doc comment update | n/a | `maxPropagateWriteAttempts`'s doc comment now notes it also bounds read-race retries; new sentinel/classifier doc comments added above it | n/a | Comment-only change, no behavior impact |

## Files Changed

| File | Action | What Was Done |
|------|--------|---------------|
| `engine/cmd/main.go` | Modified | Added `emptyRegistryErrMsg`, `registryAbsentNoopMarker` sentinel consts; added `readRaceKind` type + `raceNone`/`raceEmptyRegistry`/`raceAbsentRegistry` consts; added `classifyReadRace(coreExit int, wrote bool, stdout, stderr string) readRaceKind`; wired the classifier into `runPropagateVerified`'s loop (empty-registry check before the generic `coreExit != -1` forward; absent-registry check before the generic `!wrote` forward); updated `maxPropagateWriteAttempts`'s doc comment; rewrote the two message call sites in `runPropagateCore` to use the sentinel/marker consts (byte-identical output, same message text) |
| `engine/cmd/main_test.go` | Modified | Added `TestEmptyRegistryMsgPinned`, `TestAbsentNoopMarkerPinned`, `TestClassifyReadRace` (5-case table), `TestRunPropagateVerified_RetriesEmptyRegistryThenSucceeds`, `TestRunPropagateVerified_EmptyRegistryExhaustsAttemptsFailsLoud`, `TestRunPropagateVerified_RetriesTransientAbsentThenSucceeds`, `TestRunPropagateVerified_PersistentAbsentStillNoOpsExitZero`, `TestRunPropagateVerified_HardFailureStillNoRetry` |

## `runPropagateCore` Change Note

Per the design's hard constraint, `runPropagateCore`'s **signature, control flow,
and observable behavior** are unchanged. The only edits inside it are two
string-construction call sites (`Fprintln`/`Fprintf`) switched to reference the
new package-level sentinel consts instead of inline string literals — the
actual bytes written to stdout/stderr are byte-identical to before (proven by
the pre-existing `TestRunPropagateCore_EmptyRegistry_FailLoudNoWrite` and
`TestRunPropagateCore_RegistryAbsent_NoOp` staying green unmodified).

## Deviations from Design

None functionally. One cosmetic deviation from the tasks.md wording: task 3.5
describes asserting "`readFile` call count == 1"; the implemented test asserts
the **registry-specific** read count is 0 (broken contract frontmatter fails
inside `runPropagateCore` before the registry is ever read — only the
contract file is read once). Both describe the same "zero retries" invariant;
the test's variable name/count reflects registry reads specifically for
clarity against the other four tests in the same block, which all count
registry reads.

## Verification Results

```
cd engine && go build ./... && go vet ./... && go test ./...
```

- `go build ./...` — OK
- `go vet ./...` — OK, no warnings
- `go test ./...` — all packages OK; `engine/cmd` 91/91 subtests pass, 0 failures

## Out of Scope (per design.md, unchanged)

Corrupt `BEGIN`-without-matching-`END` block in `propagator.go`'s
`replaceBlock` remains explicit deferred technical debt, not addressed here.

## Judgment-Day Review Fixes (Phase 5, post-14/14)

A judgment-day pass raised 2 WARNING (real/theoretical) findings against the
exhausted read-race branches, 2 further WARNING (theoretical) findings about
`--require-registry` and the shared retry budget, and 2 SUGGESTIONs. All 6
were fixed:

| # | Finding | Fix | New/updated test |
|---|---------|-----|-------------------|
| 1 | Exhausted `raceEmptyRegistry` message claimed "empty on every attempt" even when an earlier attempt was `raceAbsentRegistry` | Track `sawEmptyRegistry`/`sawAbsentRegistry` across the loop; use "empty or unreadable across N attempts" wording when kinds varied, keep the precise wording only when uniform | `TestRunPropagateVerified_EmptyRegistryExhaustsAfterMixedAbsentThenEmpty` |
| 2 | Exhausting on `raceAbsentRegistry` after an earlier `raceEmptyRegistry` attempt silently no-op'd, discarding proof the registry exists | Exhaustion on `raceAbsentRegistry` now checks `sawEmptyRegistry` and prefers the fail-loud empty-registry outcome over the no-op | `TestRunPropagateVerified_AbsentExhaustsAfterEarlierEmptyPrefersFailLoud` |
| 3 | `--require-registry` + absent registry fails with zero retries, but spec.md R-003's third scenario read as "retry until budget exhausted" | No behavior change (fail-fast is intentional); added a wrapper-level test pinning the zero-retry behavior and corrected the spec.md scenario wording | `TestRunPropagateVerified_RequireRegistryAbsentFailsImmediately` |
| 4 | `maxPropagateWriteAttempts` is a shared budget across read-race and write-race retries — a compounded case leaves no margin | No behavior change; added a test forcing exactly one read-race retry then one write-clobber retry within the existing budget of 3, proving it still succeeds | `TestRunPropagateVerified_InterleavedReadRaceThenWriteRaceWithinBudget` |
| 5 | No comment flagging that `--require-registry` gets zero read-race retry protection by design | Added a design.md-referencing comment above the `--require-registry` absent branch in `runPropagateCore` | n/a (comment only) |
| 6 | `classifyReadRace`'s `coreExit == 1` check relied on an unenforced invariant | Introduced `hardFailureExitCode = 1` named constant, used in the comparison | n/a (naming only, covered by existing `TestClassifyReadRace`) |

Verification after Phase 5: `go build ./...`, `go vet ./...`, `go test ./...`
all green; `engine/cmd` package passes with the 4 new tests included.

## Judgment-Day Review Fixes, Round 2 (Phase 5.7-5.12)

A second judgment-day pass on the Phase 5 fixes found one more real gap plus
three suggestions/theoretical findings. All 6 fixed:

| # | Finding | Fix | New/updated test |
|---|---------|-----|-------------------|
| 1 | `classifyReadRace` returns `raceNone` immediately whenever `wrote==true`, so a successful-but-clobbered write attempt (attempt 1 writes, gets clobbered, retries; attempts 2-3 read absent) was invisible to `sawEmptyRegistry`/`sawAbsentRegistry`, and the exhaustion tie-break silently took the exit-0 no-op path despite that attempt proving the registry exists | Added `sawWroteThisRun`, set whenever an attempt's `wrote==true`; folded into both the `raceAbsentRegistry` exhaustion tie-break and the `raceEmptyRegistry` mixed-sequence diagnostic condition | `TestRunPropagateVerified_ExhaustsAfterEarlierWriteClobberedPrefersFailLoud` |
| 2 | No test proved #1's scenario fails loud | Added the test above alongside the fix | (same) |
| 3 | `hardFailureExitCode`'s doc comment overclaimed it is used across all of `runPropagateCore`'s exit(1) call sites | Softened the comment: documents, not enforces; used at exactly one comparison site (`classifyReadRace`) | n/a (comment only) |
| 4 | The mixed-evidence stderr message was duplicated verbatim in two branches (`raceEmptyRegistry` mixed-exhaustion, `raceAbsentRegistry` prefer-fail-loud) | Factored into one shared helper `reportInconsistentRegistryState`, called from both sites | Existing mixed-sequence tests still pass against the shared helper |
| 5 | The "prefer fail-loud" tie-break message asserted a specific cause ("a concurrent external writer may be tearing reads") even when the true cause could be a genuine non-race event (e.g. mid-invocation uninstall) | Softened `reportInconsistentRegistryState`'s wording to cause-agnostic: "registry state was inconsistent across N attempts — refusing to propagate" | `TestRunPropagateVerified_EmptyRegistryExhaustsAfterMixedAbsentThenEmpty` wording assertion updated to match |
| 6 | design.md's "Out of Scope" section didn't note that `classifyReadRace` only recognizes empty-content and absent as retryable read signals (other transient read errors still hard-fail immediately) | Added an explicit out-of-scope note mirroring the existing corrupt-block note | n/a (docs only) |

Verification after Round 2: `go build ./...`, `go vet ./...`, `go test ./...`
all green across every package; `engine/cmd` passes with the new test
included alongside all pre-existing tests.

## Judgment-Day Review Fixes, Round 3 (Phase 5.13-5.19)

A third judgment-day pass on the Phase 5 Round 2 fixes found two real
findings, one confirmed-decorative-constant finding, and four suggestions.
All 7 fixed:

| # | Finding | Fix | New/updated test |
|---|---------|-----|-------------------|
| 1 | design.md claimed `runPropagateCore` was "byte-for-byte unchanged"/"untouched", but two inline literals inside it were replaced with references to the shared sentinel consts (output-preserving, not a behavior change); the guard-test rationale also still framed the tests as protecting against drift between two independent literals | Reworded design.md's Technical Approach, Decision A rationale, File Changes table, and Migration/Rollout section to accurately describe the const substitution; reworded the guard-test rationale to state it protects against reintroducing an inline literal, not drift | n/a (docs only) |
| 2 | The "uniform empty" read-race exhaustion branch kept the old specific-cause wording ("a concurrent external writer may be tearing reads"), inconsistent with Round 2's cause-agnostic softening of the mixed-evidence branch — a persistently empty registry with no live race would be misdiagnosed as an active race | Removed the specific-cause clause from that branch's message, keeping the accurate "read empty on every attempt" framing but cause-agnostic | `TestRunPropagateVerified_EmptyRegistryExhaustsAttemptsFailsLoud` (existing, non-empty-stderr assertion still passes) |
| 3 | `hardFailureExitCode` was decorative: referenced at exactly one comparison site, already fully disambiguated by the exact string match on `emptyRegistryErrMsg`, while core's other `exit(1)` sites remained magic-number literals | Removed the constant; `classifyReadRace` now compares against literal `1` with a comment explaining why | `TestClassifyReadRace` (existing, unaffected) |
| 4 | `reportInconsistentRegistryState`'s message had a grammar nit: "registry at %s state was inconsistent across %d attempts" | Reworded to "the registry at %s was in an inconsistent state across %d attempts — refusing to propagate" | `TestRunPropagateVerified_EmptyRegistryExhaustsAfterMixedAbsentThenEmpty` substring assertion updated |
| 5 | The pre-existing write-verify-mismatch exhaustion message keeps cause-specific wording with no comment explaining why it's excluded from the cause-agnostic `reportInconsistentRegistryState` helper | Added a comment explaining the asymmetry is intentional (direct evidence of a writer vs. none); message text unchanged | n/a (comment only) |
| 6 | `emptyRegistryErrMsg`'s doc comment (and `TestEmptyRegistryMsgPinned`'s comment) overstated the guard test as protecting against wording drift, when core now references the const directly, making independent drift structurally impossible | Reworded both comments to state the guard test protects against reintroducing an inline literal, not drift | n/a (comment only) |
| 7 | `TestClassifyReadRace`'s table omitted the `classifyReadRace(-1, false, "", "")` input-domain case (permitted by the signature, unreachable from real core output) | Added the case, asserting `raceNone` | `TestClassifyReadRace` (case added) |

Verification after Round 3: `go build ./...`, `go vet ./...`, `go test ./...`
all green across every package.

## Judgment-Day Review Fixes, Round 4 (Phase 5.20-5.27)

A fourth judgment-day pass on the Phase 5 Round 3 fixes found two real
documentation-drift findings, one real branch-coverage gap, and five
suggestions. All 8 fixed (verify-report.md intentionally excluded — left for
a fresh, independent `sdd-verify` pass, per this round's explicit
instructions):

| # | Finding | Fix | New/updated test |
|---|---------|-----|-------------------|
| 1 | design.md's "New / Extended Identifiers" and "Testing Strategy" tables were never updated after Round 2/3 added `reportInconsistentRegistryState` and the `sawEmptyRegistry`/`sawAbsentRegistry`/`sawWroteThisRun` (now `sawWrote`) evidence tracking; the Data Flow diagram also implied `raceAbsentRegistry` exhaustion always forwards the exit-0 no-op | Added the missing identifiers to the table; added Testing Strategy rows for the 4 evidence/interleaving tests; reworked the Data Flow diagram's exhaustion rows to describe the mixed-evidence tie-break | n/a (docs only) |
| 2 | spec.md's R-002/R-003 scenarios only covered the uniform empty-every-attempt / absent-every-attempt cases, with nothing for the mixed-evidence fail-loud tie-break or `reportInconsistentRegistryState`'s wording | Added `Requirement: Mixed-Evidence Exhaustion Prefers Fail-Loud Over the Absent No-Op` (R-006), 3 scenarios | n/a (docs only, existing tests already cover the behavior) |
| 3 | design.md's `emptyRegistryErrMsg` table entry cited a stale line number ("core's line 706 output") | Replaced with a symbolic reference to the fail-loud empty-registry guard in `runPropagateCore` | n/a (docs only) |
| 4 | The `sawWroteThisRun`-true disjunct in the `raceEmptyRegistry` exhaustion branch's mixed-sequence condition had no dedicated test — only the `raceAbsentRegistry` side was covered | Added `TestRunPropagateVerified_EmptyRegistryExhaustsAfterEarlierWriteClobberedPrefersFailLoud`, mirroring the existing absent-branch test but landing on `raceEmptyRegistry` | `TestRunPropagateVerified_EmptyRegistryExhaustsAfterEarlierWriteClobberedPrefersFailLoud` |
| 5 | Redundant exhaustion message: "read empty on every attempt across %d attempts" | Simplified to "read empty on all %d attempts" | n/a (no test asserted the old exact phrasing) |
| 6 | `sawWroteThisRun` had a naming asymmetry vs. its siblings `sawEmptyRegistry`/`sawAbsentRegistry` | Renamed to `sawWrote`; updated all references in main.go and the one comment reference in main_test.go | n/a (rename only) |
| 7 | `switch classifyReadRace(...)` had no explicit `case raceNone:`, relying on silent fallthrough | Added an explicit `case raceNone:` with a one-line comment; no behavior change | n/a (existing tests cover raceNone paths) |
| 8 | `TestRunPropagateVerified_InterleavedReadRaceThenWriteRaceWithinBudget` declared/incremented `registryReads` but never asserted on it | Added an exact-count assertion (5 reads), matching the file's established convention | `TestRunPropagateVerified_InterleavedReadRaceThenWriteRaceWithinBudget` (assertion added) |

Verification after Round 4: `go build ./...`, `go vet ./...`, `go test ./...`
all green across every package; `engine/cmd` passes 97/97 subtests (includes
the 1 new test and the 1 new assertion added this round).

## Status

14/14 tasks complete, plus 6/6 Phase 5 review fixes, 6/6 Phase 5 Round-2
review fixes, 7/7 Phase 5 Round-3 review fixes, and 8/8 Phase 5 Round-4
review fixes applied. Ready for `sdd-verify`. Working tree left uncommitted
per instructions.
