# Verify Report: fix-propagate-foreign-writer-race

**Verdict: GO-with-findings — 0 CRITICAL, 0 WARNING (blocking), 1 process note**
**Branch:** fix/skill-registry-scope-block-wipe
**Date:** 2026-07-10
**Mode:** Strict TDD (RED before GREEN, per phase)
**Supersedes:** all prior verify-report.md content in this file (two earlier passes
in this same round were themselves found stale by judgment-day Round 5 — this
pass was hand-written directly from fresh command output, not delegated, after
that finding).

This report was produced by running every check myself against the current
working tree — no numbers were carried over from `apply-progress.md`,
`tasks.md`, or a prior verify pass without independently re-deriving them.

---

## Why this change went through 5 rounds of judgment-day

`fix-propagate-foreign-writer-race` closes the READ-side half of a race that a
prior fix (#1889, `runPropagateVerified`) only closed on the WRITE side: a
foreign, uncoordinated writer (`gentle-ai skill-registry refresh`) can clobber
`.atl/skill-registry.md` outside this project's own lock. Apply implemented
all 14 planned tasks cleanly; 5 rounds of dual-blind adversarial review (the
user set a zero-tolerance bar) then found and fixed:
- Round 1: 6 findings (4 WARNING, 2 SUGGESTION) — mostly wording/message
  precision issues in the initial implementation.
- Round 2: a genuine logic gap — a successful write that got clobbered
  (`wrote==true` + mismatch) was invisible to the evidence tracking meant to
  prefer fail-loud over a false no-op.
- Round 3: `design.md` had drifted from claiming "core untouched" to actually
  describing 2 output-preserving const substitutions inside `runPropagateCore`;
  one exhaustion branch still used stale cause-specific wording.
- Round 4: `design.md`/`spec.md` documentation hadn't caught up with Round 2-3
  code changes (missing identifiers, missing R-006 requirement); one real
  branch-coverage gap.
- Round 5: two consecutive `sdd-verify`/regeneration passes on this very
  report were themselves inaccurate (wrong test counts, a non-existent test
  name, missing R-006, and a mistraced `R-001` citation) — this report is the
  result of a third, manually fact-checked pass.

Rounds 3-5 found **zero new logic/correctness bugs** — only documentation and
traceability drift. The classification/retry logic itself has been
independently re-derived as correct by 2 different judges across 3
consecutive rounds.

---

## Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 41 (`tasks.md`: Phases 1-4 = 14 original tasks, Phase 5 Rounds 1-4 review-fix items = 27) |
| Tasks complete | 41 (`rg -c '^- \[x\]'` on tasks.md) |
| Tasks incomplete | 0 (`rg -c '^- \[ \]'` returns 0) |

---

## Build & Tests Execution

**Build**: PASSED
```
cd engine && go build ./...   → clean, no output
cd engine && go vet ./...     → clean, no output
```

**Tests**: PASSED — all 10 packages green, `-count=1` (no cache reuse)

`go test ./cmd/... -v -count=1`: **97/97 subtests PASS, 0 FAIL** (counted via
`rg -c '^--- PASS'` / `^--- FAIL'` on a fresh run, not copied from any prior
report).

**New test functions added by this diff**: **14** (`git diff engine/cmd/main_test.go | rg -c '^\+func Test'`) —
`TestEmptyRegistryMsgPinned`, `TestAbsentNoopMarkerPinned`, `TestClassifyReadRace`,
`TestRunPropagateVerified_RetriesEmptyRegistryThenSucceeds`,
`TestRunPropagateVerified_EmptyRegistryExhaustsAttemptsFailsLoud`,
`TestRunPropagateVerified_RetriesTransientAbsentThenSucceeds`,
`TestRunPropagateVerified_PersistentAbsentStillNoOpsExitZero`,
`TestRunPropagateVerified_HardFailureStillNoRetry`,
`TestRunPropagateVerified_RequireRegistryAbsentFailsImmediately`,
`TestRunPropagateVerified_InterleavedReadRaceThenWriteRaceWithinBudget`,
`TestRunPropagateVerified_EmptyRegistryExhaustsAfterMixedAbsentThenEmpty`,
`TestRunPropagateVerified_AbsentExhaustsAfterEarlierEmptyPrefersFailLoud`,
`TestRunPropagateVerified_ExhaustsAfterEarlierWriteClobberedPrefersFailLoud`,
`TestRunPropagateVerified_EmptyRegistryExhaustsAfterEarlierWriteClobberedPrefersFailLoud`.

**Diff size** (`git diff --numstat`): `engine/cmd/main.go` +221/-6,
`engine/cmd/main_test.go` +759/-3 = **989 changed lines**.

**Coverage** (changed functions, fresh `go test ./cmd/... -coverprofile`):

| Function | Coverage |
|---|---|
| `classifyReadRace` | 100.0% |
| `runPropagateVerified` | 98.2% |
| `runPropagateCore` | 96.5% |
| Package total | 76.6% |

---

## Independent Diff Verification

### 1. New identifiers present and correctly wired
Confirmed via `rg` against the current tree: `emptyRegistryErrMsg`,
`registryAbsentNoopMarker` (sentinel consts), `readRaceKind` type with
`raceNone`/`raceEmptyRegistry`/`raceAbsentRegistry`, `classifyReadRace`,
`sawEmptyRegistry`/`sawAbsentRegistry`/`sawWrote` (evidence-tracking bools,
Round 2/4 — `sawWroteThisRun` was renamed to `sawWrote` in Round 4 for
naming symmetry, confirmed zero remaining references to the old name in
`main.go`/`main_test.go`), `reportInconsistentRegistryState` (Round 2 helper,
deduplicates the mixed-evidence exhaustion message). The decorative
`hardFailureExitCode` constant flagged in Round 3 was removed in favor of a
plain literal `1` with an explanatory comment (Round 3 fix) — confirmed
absent from the current diff.

### 2. `runPropagateCore` byte-for-byte output — VERIFIED
Two call sites inside `runPropagateCore` reference the new sentinel consts
instead of inline literals; both substitutions were confirmed to reproduce
the exact prior literal strings (direct string comparison, not just
re-running the pinned guard tests `TestEmptyRegistryMsgPinned`/
`TestAbsentNoopMarkerPinned`, though those also pass). No other line inside
`runPropagateCore`'s body changed.

### 3. Branch ordering vs. `design.md` Data Flow — VERIFIED
The `classifyReadRace` switch (now with an explicit `case raceNone:` fallthrough,
added in Round 4 for readability) runs entirely before the generic
`coreExit != -1` hard-failure forward, and the mixed-evidence tie-break
(`sawEmptyRegistry`/`sawAbsentRegistry`/`sawWrote`) is consulted on both
exhaustion branches. Traced by hand (this pass and by both judges in Rounds
3-5) against every reachable attempt-sequence permutation, including
compound sequences (write-then-clobbered, then absent or empty) — no
misclassification found in any case.

### 4. Message consistency — VERIFIED
The "uniform empty" and "mixed evidence" exhaustion branches both use
cause-agnostic wording as of Round 3/4 (no longer assert "a concurrent
external writer" when the evidence doesn't support that specific cause). The
pre-existing (#1889) write-verify-mismatch message intentionally keeps
cause-specific wording — it has direct byte-mismatch evidence of an active
writer — and now carries a comment explaining that asymmetry (Round 3 fix).

---

## Spec Compliance Matrix

`specs/skill-registry-propagate/spec.md` defines **6 requirements, 12 scenarios**
(confirmed via `rg -c '^#### Scenario'`).

| R-NNN | Requirement | Test citing this ID |
|-------|-------------|---------------------|
| R-001 | Verified Write Survives Foreign-Writer Clobber | `TestRunPropagateVerified_RetriesWhenWriteClobberedByForeignWriter`, `TestRunPropagateVerified_GivesUpAfterMaxAttempts` |
| R-002 | Retryable Torn/Empty Registry Read | `TestRunPropagateVerified_RetriesEmptyRegistryThenSucceeds`, `TestRunPropagateVerified_EmptyRegistryExhaustsAttemptsFailsLoud` |
| R-003 | Retryable Transient Absent Registry Read | `TestRunPropagateVerified_RetriesTransientAbsentThenSucceeds`, `TestRunPropagateVerified_PersistentAbsentStillNoOpsExitZero`, `TestRunPropagateVerified_RequireRegistryAbsentFailsImmediately` |
| R-004 | Genuine Hard Failures Bypass Retry | `TestRunPropagateVerified_HardFailureStillNoRetry` |
| R-005 | Propagate-vs-Propagate Lock Remains Intact | `TestAcquireRegistryLock_Exclusive`, `TestPropagate_ConcurrentProcesses_RegistryNeverGutted` |
| R-006 | Mixed-Evidence Exhaustion Prefers Fail-Loud Over the Absent No-Op | `TestRunPropagateVerified_EmptyRegistryExhaustsAfterMixedAbsentThenEmpty`, `TestRunPropagateVerified_ExhaustsAfterEarlierWriteClobberedPrefersFailLoud` (+2 siblings) |

All 6 requirements have at least one test whose name or comment cites the
ID — confirmed via `rg -n "R-00[1-6]"` against `engine/cmd/main_test.go`
directly (not inferred). `TestAcquireRegistryLock_Exclusive` is the correct,
existing name (a prior draft of this report cited a non-existent
`TestAcquireRegistryLock_ExclusiveFlock` — corrected here after Round 5 caught
it).

---

## Success Criteria Sweep (proposal.md)

All 5 Success Criteria are checked `[x]` in `proposal.md` and independently
confirmed met: torn/empty reads retry within budget, transient absence
retries before the no-op, genuine hard failures still forward immediately,
14 RED-before-GREEN tests cover both read-side paths (and their mixed-evidence
combinations), and the corrupt-block case remains explicitly documented as
deferred technical debt in all three artifacts' Out-of-Scope sections.

---

## Findings

### CRITICAL
None.

### WARNING
None blocking.

### PROCESS NOTE (not a code defect)

**P-1 — Diff size (989 lines) is more than double this project's ~400-line
review-workload budget and tasks.md's own original forecast (~390-420).**
This grew organically across 5 judgment-day rounds fixing real and marginal
findings on a zero-tolerance bar, not from scope creep in the original
apply. Recommend an explicit size-exception acknowledgment at commit/PR time
(this is a single, cohesive bugfix with no natural split point — the read-race
classifier, its evidence tracking, and its tests are one indivisible unit),
rather than retroactively splitting into chained PRs.

### SUGGESTION
None outstanding — all SUGGESTIONs raised across Rounds 1-5 were fixed
(message wording, naming symmetry, explicit switch case, missing test
assertions, stale line-number references, R-NNN citation placement).

---

## Verdict

**GO for fix-propagate-foreign-writer-race.**
0 CRITICAL | 0 WARNING | 1 process note (diff size, not a defect)

Five rounds of independent dual-judge adversarial review, each re-verified
against a fresh reading of the actual working tree (not trusted from any
prior report), converged on zero remaining logic defects. All 41 tasks
complete, all 6 spec requirements traceable to a passing test, `runPropagateCore`
provably byte-for-byte unchanged, 97/97 tests pass on a fresh run, and
`design.md`/`spec.md`/`proposal.md` are now internally consistent with the
shipped code. Safe to proceed to `sdd-archive`, pending the user's decision
on how to acknowledge the diff-size process note (single PR with a size
exception, vs. splitting).

next_recommended: sdd-archive
