# Tasks: Close the read-side foreign-writer race in propagate

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~390-420 (main.go ~90, main_test.go ~300-330) |
| 400-line budget risk | Medium |
| Chained PRs recommended | No |
| Suggested split | Single PR — classifier, loop wiring, and tests are one tightly-coupled unit |
| Delivery strategy | ask-on-risk (default, not resolved by caller) |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Sentinels, classifier, loop wiring, all tests | PR 1 | Single PR; borderline 400 lines — flag for `size:exception` if it lands over budget |

## Phase 1: Sentinels & Guard Tests

- [x] 1.1 RED: `main_test.go` — `TestEmptyRegistryMsgPinned`: run core's empty-registry path (main.go ~705), assert stderr equals not-yet-defined `emptyRegistryErrMsg` const (compile fail = RED). Cites R-002.
- [x] 1.2 GREEN: `main.go` — add `emptyRegistryErrMsg` const near ~479, mirroring the exact line at ~706; switch the `Fprintln` at ~706 to use it. Test 1.1 passes.
- [x] 1.3 RED: `TestAbsentNoopMarkerPinned`: run core's absent-no-op path (main.go ~693), assert stdout contains not-yet-defined `registryAbsentNoopMarker` const (compile fail = RED). Cites R-003.
- [x] 1.4 GREEN: add `registryAbsentNoopMarker` const (stable, path-independent substring of ~693's message); embed it in the `Fprintf` at ~693. Test 1.3 passes.

## Phase 2: Classifier

- [x] 2.1 RED: `TestClassifyReadRace` (table-driven): empty-registry stderr → `raceEmptyRegistry`; absent-noop stdout → `raceAbsentRegistry`; unrelated hard-failure stderr w/ `coreExit!=-1` → `raceNone`; `wrote=true` → `raceNone`. Compile fail = RED. Cites R-002, R-003, R-004.
- [x] 2.2 GREEN: add `readRaceKind` type + `raceNone`/`raceEmptyRegistry`/`raceAbsentRegistry` consts near `maxPropagateWriteAttempts` (~488). Implement `classifyReadRace(coreExit int, wrote bool, stdout, stderr string) readRaceKind` per Decision A/B. Test 2.1 passes.

## Phase 3: Wire Into `runPropagateVerified` Loop

- [x] 3.1 RED: `TestRunPropagateVerified_RetriesEmptyRegistryThenSucceeds` — mutable `diskState` mock: empty on attempt 1, valid on attempt 2; assert no `exit` call, success output. Fails today (empty read hits hard-failure branch). Cites R-002.
- [x] 3.2 RED: `TestRunPropagateVerified_EmptyRegistryExhaustsAttemptsFailsLoud` — `diskState` empty every attempt; assert `exit(1)` only after `maxPropagateWriteAttempts` reads, non-empty stderr. Cites R-002.
- [x] 3.3 RED: `TestRunPropagateVerified_RetriesTransientAbsentThenSucceeds` — `readFile` returns `os.ErrNotExist` attempt 1, valid content attempt 2, no `--require-registry`; assert propagate proceeds (not a false-success no-op). Cites R-003.
- [x] 3.4 RED: `TestRunPropagateVerified_PersistentAbsentStillNoOpsExitZero` — `readFile` always `os.ErrNotExist`, no `--require-registry`; assert no `exit` call, exact no-op stdout after exhausting attempts. Cites R-003.
- [x] 3.5 RED: `TestRunPropagateVerified_HardFailureStillNoRetry` — broken-frontmatter hard failure (`coreExit=1`, stderr unrelated to `emptyRegistryErrMsg`); assert `readFile` call count == 1 (zero retries), immediate forwarding. Cites R-004.
- [x] 3.6 GREEN: extend `runPropagateVerified`'s loop (~539-558): call `classifyReadRace(coreExit, wrote, outBuf.String(), errBuf.String())` right after `runPropagateCore` returns, BEFORE the existing `coreExit != -1` branch. `raceEmptyRegistry`: `continue` if `attempt<max`, else fail-loud `exit(1)`. `raceAbsentRegistry`: `continue` if `attempt<max`, else forward the buffered exit-0 no-op unchanged. Discard buffers on `continue`. Tests 3.1-3.5 pass.

## Phase 4: Regression Guard

- [x] 4.1 Run full `engine/cmd` suite — confirm the 4 existing write-side `TestRunPropagateVerified_*` tests and ~20 `TestRunPropagateCore_*` tests stay green (core untouched).
- [x] 4.2 Update `maxPropagateWriteAttempts` doc comment (~479-488) to note it now also bounds read-race retries (R-005 unaffected — lock/rename logic untouched).

## Phase 5: Judgment-Day Review Fixes

- [x] 5.1 WARNING (real): the exhausted `raceEmptyRegistry` exit message claimed "empty on every attempt" even when an earlier attempt in the same run classified as `raceAbsentRegistry`. Added `sawEmptyRegistry`/`sawAbsentRegistry` tracking across the loop in `runPropagateVerified` and split the exhaustion message into an accurate "empty on every attempt" (uniform) vs. "empty or unreadable across N attempts" (mixed) wording. Guard test: `TestRunPropagateVerified_EmptyRegistryExhaustsAfterMixedAbsentThenEmpty`.
- [x] 5.2 WARNING (theoretical): exhausting on the `raceAbsentRegistry` branch after an earlier `raceEmptyRegistry` attempt (direct proof the registry file exists) silently returned the exit-0 no-op, discarding that evidence. Fixed: exhaustion on `raceAbsentRegistry` now checks `sawEmptyRegistry` and prefers the fail-loud empty-registry outcome over the no-op. Guard test: `TestRunPropagateVerified_AbsentExhaustsAfterEarlierEmptyPrefersFailLoud`.
- [x] 5.3 WARNING (theoretical): `--require-registry` + absent registry fails immediately with zero retries; this is intentional (explicit opt-in fail-fast) but R-003's third scenario in spec.md read as "retry until budget exhausted". Added `TestRunPropagateVerified_RequireRegistryAbsentFailsImmediately` (asserts exactly 1 registry read) and corrected the spec.md scenario wording to describe the real zero-retry behavior — no behavior change.
- [x] 5.4 WARNING (theoretical): `maxPropagateWriteAttempts` is a shared budget across read-race and write-race retries. Added `TestRunPropagateVerified_InterleavedReadRaceThenWriteRaceWithinBudget` forcing one read-race retry followed by one write-clobber retry within the existing budget of 3, proving the loop still succeeds at the edge of the budget — no behavior change.
- [x] 5.5 SUGGESTION: added a design.md-referencing comment directly above the `--require-registry` absent-registry branch in `runPropagateCore` noting it gets zero read-race retry protection by design, so a future caller adopting the flag doesn't reintroduce the closed race.
- [x] 5.6 SUGGESTION: introduced `hardFailureExitCode = 1` named constant and used it in `classifyReadRace`'s `coreExit == 1` comparison instead of a bare magic number, making the invariant explicit.

### Round 2 (judge re-review of 5.1-5.6)

- [x] 5.7 WARNING (real): `classifyReadRace` returns `raceNone` immediately whenever `wrote==true`, making a successful-but-clobbered write attempt (attempt 1 writes valid content, gets clobbered, retries; attempts 2-3 read absent) invisible to the `sawEmptyRegistry`/`sawAbsentRegistry` evidence tracking added in 5.1/5.2 — the exhaustion tie-break would silently take the exit-0 no-op path despite that attempt being direct proof the registry exists. Added `sawWroteThisRun` tracking in `runPropagateVerified`, set whenever `wrote==true` for an attempt, and folded it into both the `raceAbsentRegistry` exhaustion tie-break (prefer fail-loud) and the `raceEmptyRegistry` mixed-sequence diagnostic wording condition. Guard test: `TestRunPropagateVerified_ExhaustsAfterEarlierWriteClobberedPrefersFailLoud`.
- [x] 5.8 WARNING (real): companion test proving 5.7 — see above; added alongside the fix.
- [x] 5.9 SUGGESTION: softened `hardFailureExitCode`'s doc comment to state it documents (not enforces) the invariant and is referenced at exactly one comparison site (`classifyReadRace`), instead of implying it is used across all of `runPropagateCore`'s exit(1) call sites.
- [x] 5.10 SUGGESTION: factored the duplicated mixed-evidence stderr message (previously verbatim-identical in the `raceEmptyRegistry` mixed-exhaustion branch and the `raceAbsentRegistry` prefer-fail-loud branch) into one shared helper, `reportInconsistentRegistryState`, called from both sites so a future wording edit cannot drift between the two copies.
- [x] 5.11 WARNING (theoretical): softened `reportInconsistentRegistryState`'s wording from asserting a specific cause ("a concurrent external writer may be tearing reads") to a cause-agnostic "registry state was inconsistent across N attempts — refusing to propagate", since a mixed/inconsistent sequence can also arise from a genuine non-race cause (e.g. a project stopping use of the overlay mid-invocation). Updated `TestRunPropagateVerified_EmptyRegistryExhaustsAfterMixedAbsentThenEmpty`'s wording assertion to match.
- [x] 5.12 SUGGESTION: added an explicit note to design.md's "Out of Scope" section (mirroring the existing corrupt-block note) that `classifyReadRace` only recognizes empty-content and `os.IsNotExist` as retryable read signals; any other transient read error (permission, EIO, etc.) still forwards immediately as a hard failure by design.

### Round 3 (judge re-review of 5.7-5.12)

- [x] 5.13 WARNING (real): design.md's Technical Approach/File Changes claimed `runPropagateCore` was kept "byte-for-byte unchanged"/"untouched", but two inline literals inside it were replaced with references to `emptyRegistryErrMsg`/`registryAbsentNoopMarker` (output-preserving, not a behavior change). Reworded design.md's Technical Approach, Decision A rationale, File Changes table, and Migration/Rollout section to accurately describe an output-preserving const substitution instead of "untouched", and reworded the guard-test rationale to state it now protects against reintroducing an inline literal, not drift between two independent copies.
- [x] 5.14 WARNING (real): the "uniform empty" read-race exhaustion branch (every attempt classified `raceEmptyRegistry`, none absent, none written) still asserted the specific cause "a concurrent external writer may be tearing reads", inconsistent with Round 2's cause-agnostic softening of the mixed-evidence branch — a persistently empty registry from a one-time crash (no live race) would be misdiagnosed as an active race. Removed the specific-cause clause, keeping the accurate "read empty on every attempt" framing but cause-agnostic, matching `reportInconsistentRegistryState`'s rationale.
- [x] 5.15 CONFIRMED: `hardFailureExitCode` was a decorative named constant — referenced at exactly one comparison site, already fully disambiguated by the exact string match on `emptyRegistryErrMsg`, while core's ~10 other `exit(1)` sites remained magic-number literals. Removed the constant; `classifyReadRace` now compares against the literal `1` with a comment explaining the string match already disambiguates and a named constant implied a non-existent enforced invariant.
- [x] 5.16 SUGGESTION: reworded `reportInconsistentRegistryState`'s message from "registry at %s state was inconsistent across %d attempts" to "the registry at %s was in an inconsistent state across %d attempts — refusing to propagate" (grammar fix). Updated `TestRunPropagateVerified_EmptyRegistryExhaustsAfterMixedAbsentThenEmpty`'s substring assertion to match.
- [x] 5.17 SUGGESTION: added a comment at the write-verify-mismatch exhaustion message (the pre-existing #1889 "did not persist after N attempts... appears to be clobbering it" text) explaining why it intentionally keeps cause-specific wording — the on-disk bytes provably mismatching what was just written is direct evidence of an active writer, unlike the read-race branches which have no such evidence. Message text itself unchanged.
- [x] 5.18 SUGGESTION: reworded `emptyRegistryErrMsg`'s doc comment (and the matching comment on `TestEmptyRegistryMsgPinned`) to state core references the const directly, so independent drift is structurally impossible; the guard test instead protects against a future edit reintroducing an inline literal.
- [x] 5.19 SUGGESTION: added the missing `classifyReadRace(-1, false, "", "")` input-domain case to `TestClassifyReadRace`'s table (empty stdout/stderr, no write, no exit — unreachable from real core output but permitted by the signature), asserting `raceNone`.

### Round 4 (judge re-review of 5.13-5.19)

- [x] 5.20 WARNING (real): design.md's "New / Extended Identifiers" table and "Testing Strategy" table were never updated after Round 2/3 added `reportInconsistentRegistryState` and the `sawEmptyRegistry`/`sawAbsentRegistry`/`sawWroteThisRun` evidence tracking; the Data Flow diagram also still implied `raceAbsentRegistry` exhaustion always forwards the exit-0 no-op. Added the missing identifiers to the table, added Testing Strategy rows for the 4 evidence/interleaving tests, and reworked the Data Flow diagram's `raceEmptyRegistry`/`raceAbsentRegistry` exhaustion rows to describe the mixed-evidence tie-break.
- [x] 5.21 WARNING (real): spec.md's R-002/R-003 scenarios only covered the uniform empty-every-attempt / absent-every-attempt cases, with no requirement for the mixed-evidence fail-loud tie-break or `reportInconsistentRegistryState`'s wording. Added `Requirement: Mixed-Evidence Exhaustion Prefers Fail-Loud Over the Absent No-Op` (R-006) with 3 scenarios covering both exhaustion branches.
- [x] 5.22 SUGGESTION: design.md's `emptyRegistryErrMsg` table entry cited a stale line number ("core's line 706 output"). Replaced with a symbolic reference to "the fail-loud empty-registry guard in runPropagateCore".
- [x] 5.23 WARNING (real): the `sawWroteThisRun`-true disjunct in the `raceEmptyRegistry` exhaustion branch's mixed-sequence condition had no dedicated test — only the `raceAbsentRegistry` side was covered. Added `TestRunPropagateVerified_EmptyRegistryExhaustsAfterEarlierWriteClobberedPrefersFailLoud`, mirroring the existing absent-branch test but landing on a final `raceEmptyRegistry` classification.
- [x] 5.24 SUGGESTION: simplified the redundant exhaustion message "read empty on every attempt across %d attempts" to "read empty on all %d attempts".
- [x] 5.25 SUGGESTION: renamed `sawWroteThisRun` to `sawWrote` for naming symmetry with `sawEmptyRegistry`/`sawAbsentRegistry`. Updated all references (main.go) and the one comment reference in main_test.go.
- [x] 5.26 SUGGESTION: added an explicit `case raceNone:` to the `switch classifyReadRace(...)` in `runPropagateVerified`, with a one-line comment noting it falls through to the existing branches below — no behavior change.
- [x] 5.27 SUGGESTION: `TestRunPropagateVerified_InterleavedReadRaceThenWriteRaceWithinBudget` declared/incremented `registryReads` but never asserted on it. Added an exact-count assertion (5 reads) computed from the test's own scripted attempt sequence, matching the file's established convention.
