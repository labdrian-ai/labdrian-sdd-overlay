# Design: Close the read-side foreign-writer race in propagate

## Technical Approach

Extend `runPropagateVerified` (engine/cmd/main.go) primarily. `runPropagateCore`'s
~20 stateless-mock tests forbid adding verification reads inside it (same
constraint #1889 hit), so its behavior and output are unchanged — but two of its
inline stderr/stdout literals were replaced with references to the new shared
`emptyRegistryErrMsg`/`registryAbsentNoopMarker` consts (an output-preserving
substitution, not a behavior change: the emitted bytes are byte-identical to
before). The wrapper already owns the bounded
`maxPropagateWriteAttempts=3` loop and intercepts core's stdout, stderr, exit code,
and a `wrote` flag. We add a classifier that recognizes two READ-side race signals
in those already-available buffers and folds them into the SAME loop as retryable,
distinct from genuine hard failures (which still forward immediately).

## Architecture Decisions

### Decision: How to distinguish "empty registry" exit(1) from a genuine hard failure

| Option | Touches core? | Tradeoff | Decision |
|--------|---------------|----------|----------|
| (a) Sentinel stderr match | No signature/behavior change | "Fragile" if the string drifts | **CHOSEN (hardened)** |
| (b) Richer result channel | Changes core's signature | Breaks all ~20 core tests | Rejected |

**Choice**: Match core's exact empty-registry stderr line against a package-level
sentinel const, with core's own emission of that line replaced to reference the
same const directly (output-preserving substitution — same bytes, no signature
change), and PIN the coupling with a guard test that runs core's empty path and
asserts its output equals the sentinel.
**Rationale**: (b) is the invasive path the whole wrapper exists to avoid — any new
parameter or exit-fn type change forces touching every core test call site. (a)'s
remaining weakness, now that core references the shared const directly, is not
drift between two independent copies (structurally impossible once both sides
share the same identifier) but a future edit reintroducing an inline literal in
either place; the guard test (`TestEmptyRegistryMsgPinned`) converts that into a
loud CI failure. The match is a FULL trimmed-equality on `emptyRegistryErrMsg`
(core prints exactly one line then exits), not a loose substring, so no other error
can be misclassified as retryable.

### Decision: Transient absent registry — retryable in the wrapper, NOT --require-registry at call sites

**Verified**: none of the 3 hooks pass `--require-registry` (engine/settings/settings.go
lines 452, 500, 540); all append `|| true`. They fire GLOBALLY on every project.
**Choice**: Detect the absent-no-op via its stdout marker and retry it inside the
bounded loop; after attempts are exhausted with the file still absent, keep the exact
exit-0 no-op. Do NOT add `--require-registry` to the hooks.
**Alternatives considered**: forcing `--require-registry` at the 3 call sites.
**Rationale**: `--require-registry` would print `error: registry required but not
found` to stderr on EVERY UserPromptSubmit in every NON-overlay project (`|| true`
swallows the exit code, not the stderr noise), breaking the graceful "project does
not use the overlay" degradation. Bounded-retry-then-no-op closes the mid-refresh
race window (foreign tool unlink-then-recreate) without that collateral. A
persistently absent registry still resolves to the fast exit-0 no-op.
`--require-registry` stays available as an explicit opt-in (CI, project-scoped installs).

### Decision: No backoff sleep; reuse maxPropagateWriteAttempts

**Choice**: Immediate bounded retries, no sleep, reusing the existing constant and loop.
**Rationale**: Matches the judge-validated write-side shape; zero added latency on the
non-overlay hot path (3× ENOENT stat is microseconds). When a torn/empty window cannot
be won within bounds, the correct terminal behavior is fail-loud, which the bounded loop
already gives. Backoff is deferred; revisit only if field data shows retries losing the race.

## New / Extended Identifiers (engine/cmd/main.go)

| Name | Kind | Purpose |
|------|------|---------|
| `emptyRegistryErrMsg` | const string | Sentinel = the fail-loud empty-registry guard's output in `runPropagateCore`; pinned by guard test |
| `registryAbsentNoopMarker` | const string | Stable substring of core's absent-no-op stdout (path-independent) |
| `readRaceKind` | type + `raceNone`/`raceEmptyRegistry`/`raceAbsentRegistry` | Classification enum |
| `classifyReadRace(coreExit int, wrote bool, stdout, stderr string) readRaceKind` | func | Unit-testable classifier |
| `sawEmptyRegistry`, `sawAbsentRegistry`, `sawWrote` | local `bool`s in `runPropagateVerified` | Track which evidence kinds were observed across every attempt in the run, so exhaustion messages/tie-breaks can reason about a mixed sequence instead of only the terminal attempt's kind |
| `reportInconsistentRegistryState(stderr io.Writer, registryPath string, attempts int)` | func | Shared, cause-agnostic fail-loud diagnostic for the two mixed-evidence exhaustion outcomes (`raceEmptyRegistry` mixed exhaustion, `raceAbsentRegistry` prefer-fail-loud tie-break) so the wording cannot drift between the two call sites |

`maxPropagateWriteAttempts` is KEPT (not renamed) to avoid churn on the landed
write-side fix and its tests; its doc comment gains a note that it now also bounds
read-race retries.

## Data Flow (loop, one attempt)

    runPropagateCore ──→ buffers(out,err) + wrote + coreExit
         │
         ▼
    classifyReadRace()
     ├─ raceEmptyRegistry → attempt<max? continue
     │                      : exhausted → sawAbsentRegistry||sawWrote?
     │                        reportInconsistentRegistryState + exit(1) [mixed]
     │                        : uniform "read empty on all N attempts" + exit(1)
     ├─ coreExit!=-1      → genuine hard failure: forward + exit(coreExit)   [unchanged]
     ├─ raceAbsentRegistry→ attempt<max? continue
     │                      : exhausted → sawEmptyRegistry||sawWrote?
     │                        reportInconsistentRegistryState + exit(1) [prefer fail-loud]
     │                        : forward exit-0 no-op [persistently absent, no contrary evidence]
     ├─ !wrote            → already-correct no-op: forward + return          [unchanged]
     └─ wrote             → read back; bytes match? return : retry/fail-loud [unchanged]

Retryable `continue` DISCARDS that attempt's buffers (per-iteration allocation);
only the final/terminal branches forward output. Order matters: the empty-registry
check runs BEFORE the generic `coreExit != -1` forward so it is intercepted.

`sawEmptyRegistry`/`sawAbsentRegistry`/`sawWrote` accumulate across every
attempt in the run (not just the current one). Exhaustion is NOT always the
uniform exit-0 no-op / uniform fail-loud shown above: if an earlier attempt in
the SAME run supplied contrary evidence that the registry file exists (a
genuine empty read, or a write that later verification found clobbered), the
tie-break prefers the fail-loud diagnostic over discarding that evidence —
`raceAbsentRegistry` exhaustion is no longer guaranteed to forward the exit-0
no-op, and `raceEmptyRegistry` exhaustion's wording switches from the uniform
"read empty on all N attempts" to the shared, cause-agnostic
`reportInconsistentRegistryState` message when the sequence was mixed.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `engine/cmd/main.go` | Modify | Add sentinels + `classifyReadRace`; extend the loop's branches. Core's two matching literals replaced with references to the shared consts (output-preserving; no behavior change). |
| `engine/cmd/main_test.go` | Modify | RED-before-GREEN: empty-read retry-then-succeed, empty-read exhaust-then-fail-loud, absent-read retry-then-succeed, absent-read exhaust-then-exit-0-noop, hard-failure-still-immediate, already-correct-never-retried, `TestEmptyRegistryMsgPinned`, `TestAbsentNoopMarkerPinned`. |

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `classifyReadRace` mapping of each signal combo | Table-driven, pure function |
| Unit | Loop retries empty/absent, forwards hard fails immediately | Shared mutable `diskState` mock (per #1889), NOT a read-call counter |
| Guard | Core's actual output still matches both sentinels | Run core's empty/absent paths, assert equality — fails loud on drift |
| Unit | Shared retry budget survives one read-race retry + one write-race retry within the same run | `TestRunPropagateVerified_InterleavedReadRaceThenWriteRaceWithinBudget` — scripted attempt sequence (torn read, clobbered write, clean write), asserts success and the exact registry-read count consumed |
| Unit | Exhaustion wording stays accurate for a mixed absent-then-empty sequence, instead of overclaiming "empty on every attempt" | `TestRunPropagateVerified_EmptyRegistryExhaustsAfterMixedAbsentThenEmpty` |
| Unit | Exhausting on `raceAbsentRegistry` after an earlier `raceEmptyRegistry` attempt prefers fail-loud over the exit-0 no-op (earlier attempt is direct proof the registry exists) | `TestRunPropagateVerified_AbsentExhaustsAfterEarlierEmptyPrefersFailLoud` |
| Unit | Exhausting after an earlier clobbered write (`sawWrote` true, `sawEmptyRegistry`/`sawAbsentRegistry` alone would miss it) prefers fail-loud on both the `raceAbsentRegistry` and `raceEmptyRegistry` exhaustion branches | `TestRunPropagateVerified_ExhaustsAfterEarlierWriteClobberedPrefersFailLoud` (absent branch) and `TestRunPropagateVerified_EmptyRegistryExhaustsAfterEarlierWriteClobberedPrefersFailLoud` (empty branch) |

Reuse #1889's mock discipline: model the foreign clobber as one mutable `diskState`
both the write and the simulated race mutate, so reads stay physically consistent.

## Out of Scope (explicit deferred debt)

Corrupt `BEGIN`-without-matching-`END` block (propagator.go `replaceBlock`, treated as
"already correct" forever). A torn read landing as a non-empty half-written block is seen
by `Propagate` as changed=false → already-correct no-op → the wrapper does NOT retry it.
This residual gap is NOT closed here; only empty-read and absent-read races are covered.
Filed as separate technical debt unless the user opts to fold it in.

`classifyReadRace` only recognizes empty-content (`emptyRegistryErrMsg`) and
`os.IsNotExist` (`registryAbsentNoopMarker`) as retryable read signals. Any
other transient read error on the registry (permission denied, EIO, or any
other OS-level I/O error) still forwards immediately as a genuine hard failure
on the first attempt, with none of this change's retry protection. This is the
intentional, documented scope of this change, not a regression — mirroring the
corrupt-block note above.

## Migration / Rollout

No migration. Additive and reversible: revert the diff in the two files, rebuild
`gentle-ai-overlay`, redistribute to `~/.claude/bin/`. `runPropagateCore`'s
behavior and output are unchanged (only two literals became const references),
and the #1889 write-side fix is untouched.

## Open Questions

- [ ] None blocking. Optional: whether to fold the corrupt-block case into this change
      (currently deferred) — user decision, not a design blocker.
