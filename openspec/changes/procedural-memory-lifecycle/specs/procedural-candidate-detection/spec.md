# Delta for Procedural Candidate Detection

> **Base spec**: this delta is written against the promoted main spec
> `openspec/specs/procedural-candidate-detection/spec.md` (item 30, archived
> 2026-09-18 via PR #351 to
> `openspec/changes/archive/2026-09-18-procedural-candidate-detection/`). It
> extends that capability's candidate record schema and detection-time
> gating with the additional fields and gates this change introduces:
> the do-not-capture list at emission, an extended `Status` vocabulary,
> `Disposition`, append-only `History`, the `Registered`/`Promoted` hash
> line, a derived `OccurrencesSincePromotion`, and a verified `AbsorbedInto`.

## ADDED Requirements

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

The candidate/draft record's `Status` field MUST support a vocabulary
extended beyond `observing | emitted | rejected` to include every state
introduced by this change's lifecycle: at minimum a drafted state, a
project-tier `registered` state, a global-tier `promoted` state distinct
from `registered`, and a `retired` state. Every value in the extended
vocabulary MUST be reachable only through the corresponding lifecycle
transition defined by the drafting, registration, and maintenance
capabilities, never set directly.

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
hash. This hash line is the value the ownership-by-hash check
(`procedural-skill-maintenance`) reads back for comparison against the
current on-disk content.

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
