# Procedural Candidate Detection Specification

## Purpose

Detect and durably store procedural promotion candidates — repeated-success
patterns and recurring failure-recovery patterns — as a first slice of the
umbrella procedural-memory-skill-promotion effort. This capability covers
detection and storage only: it never drafts a skill, never requests approval,
and never writes under `skills/`. Candidate records persist in Engram, the
sole durability store for this change (no second SQLite store, no Go-side
write path into Engram's database).

## Requirements

### Requirement: Durable Promotion-Candidate Store

The system MUST persist each promotion candidate as a stable, cross-session
Engram record under a dedicated procedural-candidate topic-key namespace,
carrying at minimum: a stable candidate id, an occurrence count, first- and
last-observation timestamps, and a retrievable evidence reference for every
contributing occurrence. The candidate identity MUST be defined at
sub-step / debugging-pattern granularity, not whole-session or
whole-feature granularity.

#### Scenario: Candidate record survives a session boundary

- GIVEN a candidate is first observed in session A and persisted with
  occurrence count 1 and one evidence reference
- WHEN the same candidate is observed again in session B
- THEN the record read back after the cold start of session B reports
  occurrence count 2
- AND both evidence references from session A and session B are retrievable
- AND the topic-key shape used to store and retrieve the record is the same
  in both sessions

#### Scenario: Candidate identity is sub-step granularity

- GIVEN two occurrences belong to the same debugging pattern but occur in
  different overall sessions or features
- WHEN their candidate identity is computed
- THEN both occurrences resolve to the same candidate id
- AND a candidate id is never derived from a whole-session or whole-feature
  identifier alone

#### Scenario: No second durability store is introduced

- GIVEN a promotion candidate is persisted
- WHEN its storage location is inspected
- THEN the only durable record lives in Engram
- AND no SQLite database or other local store outside Engram holds candidate
  state

### Requirement: Repeated-Success Detection

The system MUST emit exactly one promotion candidate the moment the same
approach is observed to succeed for the Nth distinct time, and that
candidate MUST reference all N contributing occurrences. The system MUST NOT
emit a second candidate for the same approach at occurrence N+1, and MUST
NOT emit any candidate for a single, non-repeated success.

#### Scenario: No candidate before the threshold (N-1)

- GIVEN the same approach has succeeded N-1 times, one short of the
  detection threshold
- WHEN detection runs after the N-1th success
- THEN no candidate is emitted for that approach

#### Scenario: Exactly one candidate emitted at the threshold (N)

- GIVEN the same approach has now succeeded N times
- WHEN detection runs after the Nth success
- THEN exactly one candidate is emitted for that approach
- AND the emitted candidate references all N contributing occurrences

#### Scenario: No duplicate candidate after the threshold (N+1)

- GIVEN a candidate was already emitted at the Nth success for an approach
- WHEN that same approach succeeds again for an (N+1)th time
- THEN no second candidate is emitted for that approach
- AND the existing candidate record MAY be updated with the new occurrence,
  but no new candidate identity is created

#### Scenario: A single non-repeated success emits nothing

- GIVEN an approach has succeeded exactly once and has no prior occurrences
- WHEN detection runs after that single success
- THEN no candidate is emitted

### Requirement: Failure-and-Recovery Detection

The system MUST emit a promotion candidate labelled `failure-recovery` when
the same failure is observed to be resolved by the same recovery action N
times. The system MUST NOT emit a candidate when N occurrences of the same
failure are each resolved by a divergent recovery action.

#### Scenario: Same failure, same recovery, repeated N times

- GIVEN the same failure has occurred N times
- AND each occurrence was resolved by the same recovery action
- WHEN detection runs after the Nth matching occurrence
- THEN exactly one candidate is emitted
- AND that candidate carries the `failure-recovery` label
- AND it references all N contributing failure/recovery occurrences

#### Scenario: Same failure, divergent recoveries, nothing emitted

- GIVEN the same failure has occurred N times
- AND each occurrence was resolved by a different recovery action
- WHEN detection runs after the Nth occurrence
- THEN no candidate is emitted for that failure

#### Scenario: Mixed cluster only counts the matching subset

- GIVEN the same failure has occurred more than N times
- AND only a subset of exactly N of those occurrences share the same
  recovery action, while the rest diverge
- WHEN detection runs
- THEN a `failure-recovery` candidate is emitted referencing only the
  matching subset of N occurrences
- AND the divergent occurrences are not counted toward that candidate's
  occurrence count

### Requirement: Duplicate Candidate Rejection

The system MUST reject a promotion candidate whose identity is already
covered by an existing entry in the registered skill registry
(`skills.registry.yaml`), recording the rejection reason `duplicate` and the
matched skill path. This rejection MUST be backed by a new read-only Go
entrypoint in `engine/skills` that reuses the existing exported
`ParseRegistry` function and performs no write to the registry or to
`skills/`. Rejection is decided purely on registry contents at check time;
the registry itself is never mutated by this capability.

#### Scenario: Candidate already covered by a registered skill is rejected

- GIVEN a registered skill entry in `skills.registry.yaml` covers the same
  candidate identity (by id or path)
- WHEN the candidate is checked against the registry
- THEN the candidate is rejected with reason `duplicate`
- AND the rejection record includes the matched skill's path

#### Scenario: Candidate with no matching registered skill is not rejected

- GIVEN no entry in `skills.registry.yaml` covers the candidate's identity
- WHEN the candidate is checked against the registry
- THEN the candidate is not rejected as a duplicate
- AND no matched skill path is recorded

#### Scenario: Match entrypoint is read-only

- GIVEN the new `engine/skills` match entrypoint is invoked with a parsed
  registry and a candidate identity
- WHEN it evaluates a match
- THEN it performs no write to `skills.registry.yaml`, to any file under
  `skills/`, or to any other persisted state
- AND it reuses the existing exported `ParseRegistry` function rather than
  re-parsing the registry YAML independently

### Requirement: No Go-Side Engram Write Path

The system MUST NOT introduce any Go-side code path that writes to Engram's
database. All persistence of candidate records (creation and updates to
occurrence count, timestamps, and evidence references) MUST occur through an
agent-driven turn using the existing Engram MCP tools
(`mem_save`/`mem_update`), never through a Go process opening Engram's
database for writing.

#### Scenario: No Go code writes to Engram's database

- GIVEN the full set of source changes introduced by this capability
- WHEN the Go source tree is inspected for Engram database access
- THEN no Go code path opens Engram's database in a writable mode
- AND all Engram-database connections in Go remain read-only where they
  exist at all

#### Scenario: Candidate persistence happens through an agent-driven MCP call

- GIVEN a candidate must be created or updated in Engram
- WHEN that persistence happens
- THEN it happens through an LLM-turn-driven MCP call (`mem_save` or
  `mem_update`)
- AND no background Go process performs the write independently of an agent
  turn
