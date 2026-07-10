# Delta for skill-registry-propagate

First spec for this domain — written as ADDED requirements (no prior
`openspec/specs/skill-registry-propagate/spec.md` exists to delta against).

## ADDED Requirements

### Requirement: Verified Write Survives Foreign-Writer Clobber

ID: R-001

WHEN `propagate` writes an updated registry, the system SHALL re-read the
registry after the write and retry the read-decide-write-verify cycle up to
`maxPropagateWriteAttempts` times if the on-disk bytes mismatch what was
written.

(Already implemented in `runPropagateVerified`, fix #1889; documented here as
the domain's first formal requirement.)

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

## Out of Scope

- Corrupt `BEGIN`-without-matching-`END` block in `propagator.go`'s
  `replaceBlock` (treated as "already correct" forever). Preexisting bug, not
  introduced by this change, flagged at lower severity by the judges. NOT a
  requirement here — deferred as separate technical debt.
