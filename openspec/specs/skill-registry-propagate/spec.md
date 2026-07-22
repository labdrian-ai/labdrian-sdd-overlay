# Skill Registry Propagate Specification

## Purpose

Define the correctness contract for `propagate`'s verified-write loop
(`runPropagateVerified` in `engine/cmd/main.go`): how it survives a foreign,
uncoordinated writer (`gentle-ai skill-registry refresh`) racing on
`.atl/skill-registry.md` outside this project's own `<registry>.lock`, on
both the write side and the read side, and how it stays distinct from
genuine hard failures that must never be retried.

## Requirements

### Requirement: Verified Write Survives Foreign-Writer Clobber

ID: R-001

WHEN `propagate` writes an updated registry, the system SHALL re-read the
registry after the write and retry the read-decide-write-verify cycle up to
`maxPropagateWriteAttempts` times if the on-disk bytes mismatch what was
written.

#### Scenario: Foreign writer clobbers a successful write

- GIVEN a write succeeds and a foreign writer overwrites it before verification
- WHEN the system re-reads and finds a byte mismatch
- THEN the system retries the cycle instead of reporting success

#### Scenario: Write never persists after exhausting attempts

- GIVEN a foreign writer clobbers every attempt
- WHEN `maxPropagateWriteAttempts` is exhausted
- THEN the system exits non-zero naming the concurrent external writer

### Requirement: Retryable Torn/Empty Registry Read

ID: R-002

IF a registry read returns empty or whitespace-only content, THEN the system
SHALL treat the read as retryable and retry up to `maxPropagateWriteAttempts`
before exiting non-zero.

#### Scenario: Transient torn read recovers within budget

- GIVEN a foreign writer's mid-rewrite produces a transient empty read
- WHEN a retry within budget returns valid content
- THEN the system completes propagate without an immediate exit(1)

#### Scenario: Persistently empty read exhausts the budget

- GIVEN the registry reads empty on every attempt
- WHEN `maxPropagateWriteAttempts` is exhausted
- THEN the system exits non-zero reporting the registry as unreadable

### Requirement: Retryable Transient Absent Registry Read

ID: R-003

IF a registry read fails not-exist and `--require-registry` was not supplied,
THEN the system SHALL retry the read up to `maxPropagateWriteAttempts` before
concluding a persistent no-op.

#### Scenario: Transient absence during a foreign rewrite recovers

- GIVEN a foreign writer briefly removes/replaces the registry
- WHEN a retry within budget finds the registry present
- THEN the system proceeds with propagate instead of a false-success no-op

#### Scenario: Persistently absent registry still no-ops at exit 0

- GIVEN the registry is absent on every attempt, no `--require-registry`
- WHEN the budget is exhausted
- THEN the system exits 0 with the existing "does not use the overlay" message

#### Scenario: --require-registry with absent registry fails immediately, zero retries

- GIVEN the registry is absent, `--require-registry` set
- WHEN `runPropagateCore` reports the require-registry hard-failure exit
- THEN the system forwards that exit immediately on the first attempt (the
  retry budget is never spent — classifyReadRace treats this as a genuine
  hard failure, not a retryable read race) reporting the registry as
  required but missing

### Requirement: Genuine Hard Failures Bypass Retry

ID: R-004

IF a propagate core exit is caused by bad arguments, an unreadable contract,
or broken frontmatter, THEN the system SHALL forward the failure immediately
without retrying.

#### Scenario: Hard failure forwards on first attempt, zero retries

- GIVEN invalid arguments or unparsable contract frontmatter
- WHEN `runPropagateCore` reports a hard-failure exit
- THEN the system forwards that exit code immediately with zero retries

### Requirement: Propagate-vs-Propagate Lock Remains Intact

ID: R-005

WHILE two concurrent `propagate` invocations target the same registry, the
system SHALL serialize their writes via the existing `<registry>.lock` flock
and atomic rename, unregressed by the read-side retry changes.

#### Scenario: Two concurrent propagate runs still serialize

- GIVEN two `propagate` processes target the same registry concurrently
- WHEN both attempt to acquire `<registry>.lock`
- THEN one proceeds while the other waits, and the atomic rename still
  guarantees no torn write is ever observable by a reader

### Requirement: Mixed-Evidence Exhaustion Prefers Fail-Loud Over the Absent No-Op

ID: R-006

WHILE `maxPropagateWriteAttempts` are exhausted with a NON-uniform sequence of
read-race outcomes across the run (some attempts classified as a torn/empty
read, some as absent, or an earlier attempt wrote successfully before later
verification found it clobbered), the system SHALL treat any earlier
attempt's direct evidence that the registry file exists as taking priority
over the persistently-absent no-op, and SHALL report the exhaustion with a
cause-agnostic diagnostic describing the registry as being in an inconsistent
state, rather than asserting a specific cause.

#### Scenario: Exhausting on absent after an earlier empty read prefers fail-loud

- GIVEN attempt 1 reads the registry as torn/empty (direct proof the file
  exists) and every subsequent attempt reads it as absent
- WHEN `maxPropagateWriteAttempts` is exhausted on the absent classification
- THEN the system exits non-zero with the shared inconsistent-state
  diagnostic instead of silently forwarding the exit-0 absent no-op

#### Scenario: Exhausting on absent after an earlier clobbered write prefers fail-loud

- GIVEN attempt 1 writes valid content but a foreign writer clobbers it before
  verification, and every subsequent attempt reads the registry as absent
- WHEN `maxPropagateWriteAttempts` is exhausted on the absent classification
- THEN the system exits non-zero with the shared inconsistent-state
  diagnostic instead of silently forwarding the exit-0 absent no-op, because
  the earlier successful write is direct proof the registry exists

#### Scenario: Exhausting on empty after an earlier absent or clobbered-write attempt reports mixed evidence

- GIVEN at least one attempt in the run classified as absent, or wrote
  successfully before being clobbered, and the run exhausts on a torn/empty
  read classification
- WHEN `maxPropagateWriteAttempts` is exhausted
- THEN the system exits non-zero using the shared, cause-agnostic
  inconsistent-state diagnostic (`reportInconsistentRegistryState`) instead of
  the uniform "read empty on all N attempts" wording, since the sequence was
  not uniformly empty

## Out of Scope

- Corrupt `BEGIN`-without-matching-`END` block in `propagator.go`'s
  `replaceBlock` (treated as "already correct" forever). Preexisting bug, not
  introduced by this change, flagged at lower severity by the judges. NOT a
  requirement here — deferred as separate technical debt.

## Requirements

### Requirement: Authoritative Scoped Block Restoration

The repair MUST use the existing authoritative propagator and shared contracts to generate exactly one marker-delimited block and exactly one generated row for each of `minimalism-contract-scope`, `skill-discovery-safety-scope`, and `anti-generic-design-scope`. Generated rows MUST NOT be hand-authored. (Traceability: AC-001)

#### Scenario: Restore exactly three unique scoped blocks

- GIVEN `.atl/skill-registry.md` lacks the three required scope blocks
- WHEN the authoritative propagation mechanism runs for the three corresponding contracts
- THEN each named BEGIN/END marker pair and its generated row occurs exactly once

### Requirement: Registry Preservation and Idempotence

The repair MUST preserve all registry content unrelated to the three generated marker ranges, and a second identical propagation pass MUST produce no registry change. (Traceability: AC-002)

#### Scenario: Preserve unrelated registry content

- GIVEN the registry content captured before restoration
- WHEN the restored registry is compared with that capture
- THEN content outside the three generated marker ranges is unchanged

#### Scenario: Repeat restoration without changes

- GIVEN all three required blocks and rows occur exactly once
- WHEN the same three authoritative propagations run again
- THEN `.atl/skill-registry.md` has no resulting diff and no duplicate marker or row

### Requirement: Healthy Hook Status

After restoration, `bin/labdrian-overlay status-hooks` MUST exit `0` and MUST NOT emit a missing-scope warning for any restored block. (Traceability: AC-003)

#### Scenario: Report healthy status

- GIVEN the three required generated blocks are present exactly once
- WHEN `bin/labdrian-overlay status-hooks` runs
- THEN it exits `0` without a missing-scope warning

### Requirement: Healthy Surfaces Remain Untouched

The repair MUST modify only `.atl/skill-registry.md`; the existing binary, hooks, shared contracts, propagation and status-check code, TUI, and Claude, OpenCode, and Codex targets MUST remain unchanged. No rebuild, restart, rewrite, installation, or synchronization repair SHALL occur. (Traceability: AC-004)

#### Scenario: Prove excluded surfaces are unchanged

- GIVEN the binary, hooks, contracts, and synchronized targets are healthy before restoration
- WHEN the repair diff and executed operations are reviewed
- THEN only the registry restoration is present and no excluded surface or workflow was changed
