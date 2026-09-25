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

### Requirement: Do-Not-Capture List Gates Candidate Quality

The system MUST NOT advance a candidate past `emitted` toward drafting when
its contributing evidence matches a disqualifying category from the
do-not-capture list: an environment-dependent failure, an unverified
negative claim about a tool, a transient error with no recurring cause, a
one-off narrative with no repeatable content, or an unresolved failure
narrated as a completed workflow. This is a candidate-quality gate at the
detection/emission boundary. The do-not-capture list is defined exactly
once, in `skills/_shared/procedural-candidate-detection.md`; this
requirement and drafting's equivalent requirement
(`procedural-skill-drafting`) both enforce that same single list at two
independent points in the lifecycle — emission and draft time — rather than
each maintaining its own copy. Neither point's enforcement supersedes the
other; a candidate that slips past emission is still refused at draft time.

#### Scenario: A disqualified candidate is marked, not silently dropped

- GIVEN a candidate whose evidence matches a documented disqualifying
  category
- WHEN the do-not-capture check runs at emission time
- THEN the candidate's status reflects the disqualification (for example
  `rejected`) rather than disappearing without a record
- AND the disqualifying category is recorded

#### Scenario: A qualifying candidate is unaffected by the list

- GIVEN a candidate whose evidence does not match any disqualifying
  category
- WHEN the do-not-capture check runs
- THEN the candidate's status and progression are unaffected by this check

### Requirement: Extended Status Vocabulary

The candidate record's `Status` field MUST support a vocabulary extended
beyond `observing | emitted | rejected` to include every state introduced
by this change's lifecycle: at minimum a drafted state, a project-tier
`registered` state, a global-tier `promoted` state distinct from
`registered`, and a `retired` state. Every value in the extended vocabulary
MUST be reachable only through the corresponding lifecycle transition
defined by the drafting, registration, and maintenance capabilities, never
set directly.

The draft record MUST have its own, separate `Status` field with its own
vocabulary (`open | registered | abandoned`); a draft reaching `registered`
MUST move its candidate record's `Status` to `registered`, and an
`abandoned` draft MUST leave the candidate record's `Status` at `drafted`
until a new draft is opened or the candidate is retired.

#### Scenario: Status values are mutually exclusive at a point in time

- GIVEN a candidate/draft record at any point in its lifecycle
- WHEN its `Status` is read
- THEN exactly one value from the extended vocabulary is reported

#### Scenario: registered and promoted are distinct and ordered

- GIVEN a skill that has completed project-tier registration but not human
  promotion
- WHEN its `Status` is read
- THEN it reads `registered`, not `promoted`
- AND it can only reach `promoted` through the documented human promotion
  procedure

### Requirement: Disposition Field on the Candidate Record

The candidate record MUST carry a `Disposition` field with value `new` or
`extend:<skill-id>`, computed by the drafting capability and persisted on
the candidate record so that downstream registration, revision, and
retirement logic can read it without recomputing the extend-vs-create
decision.

#### Scenario: Disposition persists from drafting through registration

- GIVEN a candidate whose `Disposition` was computed as `extend:skill-x`
  during drafting
- WHEN the candidate record is read during registration
- THEN `Disposition` still reads `extend:skill-x`

### Requirement: Append-Only History on Candidate and Draft Records

The candidate and draft record schema MUST include a `History` list field
that only grows: every write that changes lifecycle state (emission,
do-not-capture disqualification, drafting, registration, promotion,
revision, retirement, consolidation) MUST append an entry to `History`
rather than replacing it, so the record survives Engram's overwrite-on-
upsert behavior without losing prior transitions.

#### Scenario: An upsert-style write preserves prior History entries

- GIVEN a candidate record with an existing `History` of length N
- WHEN a new lifecycle transition is written via `mem_save`/`mem_update`
  (Engram's upsert path)
- THEN the record read back afterward has `History` of length N+1 or more
- AND all N original entries are present unchanged

### Requirement: Registered/Promoted Hash Line Recorded on the Candidate Record

Upon project-tier registration or global-tier promotion, the candidate
record MUST record the sha256 hash of the exact content written to disk at
that tier, labeled to distinguish a `Registered` hash from a `Promoted`
hash. This hash line is an informational mirror for humans reading the
record; it is never the value the ownership-by-hash check
(`procedural-skill-maintenance`) reads. That check reads only the project
lock file's recorded hash, which is the single source of truth for ownership.

#### Scenario: Registration writes a Registered hash line

- GIVEN a candidate that completes project-tier registration
- WHEN the candidate record is read afterward
- THEN it carries a `Registered` hash line equal to the sha256 of the
  written SKILL.md content

#### Scenario: Promotion writes a distinct Promoted hash line

- GIVEN a candidate that completes global-tier promotion
- WHEN the candidate record is read afterward
- THEN it carries a `Promoted` hash line, distinguishable from any
  `Registered` hash line recorded earlier

### Requirement: OccurrencesSincePromotion Is Derived, Not Stored, and Applies at Either Tier

The candidate record's `OccurrencesSincePromotion` value MUST be derived —
never a stored, independently incremented counter — as the count of
qualifying `Occurrences` entries whose timestamp is strictly after the
`at:` of the record's most recent `Registered` or `Promoted` transition
line in `History`. It applies both to a `registered` (project-tier) skill,
counting since registration, and to a `promoted` (global-tier) skill,
counting since promotion, for the maintenance capability's revision trigger
to read.

#### Scenario: Value reads zero immediately after a Registered or Promoted transition

- GIVEN a candidate whose record just gained a new `Registered` or
  `Promoted` `at:` line
- WHEN `OccurrencesSincePromotion` is read
- THEN it reads 0, because no `Occurrences` entry yet postdates that line

#### Scenario: Value increases with each qualifying post-transition recurrence

- GIVEN a registered or promoted candidate whose derived
  `OccurrencesSincePromotion` currently reads 1
- WHEN the underlying pattern is observed to recur again after the latest
  `Registered` or `Promoted` `at:`
- THEN `OccurrencesSincePromotion` reads 2 when next read, without any
  stored counter field having been incremented

### Requirement: AbsorbedInto Field on the Candidate Record

The candidate record MUST support an `AbsorbedInto: <skill-id>` field, set
only by the retirement/consolidation path defined in
`procedural-skill-maintenance`, and only after that path verifies the named
skill id **exists** at the appropriate tier — in the overlay skill registry
for a global target, or in the project lock file for a project-tier target
— not by `MatchCandidate` coverage of the retired candidate's identity.

#### Scenario: AbsorbedInto is absent until consolidation

- GIVEN a candidate record that has not been retired via consolidation
- WHEN its record is read
- THEN `AbsorbedInto` is unset

#### Scenario: AbsorbedInto is set only by a verified consolidation

- GIVEN a candidate retired via a verified consolidation into skill `B`
- WHEN its record is read afterward
- THEN `AbsorbedInto` reads `B`
