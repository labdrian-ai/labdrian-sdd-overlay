# Procedural Skill Drafting Specification

## Purpose

Turn an item-30 promotion candidate into a draft SKILL.md (or a diff against
an existing one) that a human never had to write by hand, while keeping
low-quality or log-shaped material out of the procedural-memory pipeline
before it ever reaches a project's `skills/` tree. Drafts are Engram records
only; no draft file is ever written under `skills/`.

## Requirements

### Requirement: Do-Not-Capture List Refuses Drafting Disqualified Candidates

The system MUST refuse to create a draft for a candidate whose evidence
matches any disqualifying category from the do-not-capture list: an
environment-dependent failure (fails only under one local misconfiguration,
not a general lesson); a negative claim about a tool with no independent
verification; a transient error (network blip, rate limit, timeout) with no
recurring cause; a one-off narrative with no repeatable procedural content;
or an unresolved failure presented as if it were a completed workflow. A
refusal MUST be recorded rather than silently dropped. This is the same
single do-not-capture list defined once in
`skills/_shared/procedural-candidate-detection.md` that
`procedural-candidate-detection` enforces at emission time — this
requirement is drafting's independent second enforcement point over that
one list, not a separately maintained copy of it, and it applies even to a
candidate that already passed the emission-time check.

#### Scenario: Environment-dependent failure is refused

- GIVEN a candidate whose contributing occurrences all describe a failure
  that reproduces only under one occurrence's local misconfiguration
- WHEN drafting is attempted for that candidate
- THEN no draft record is created
- AND the refusal and its do-not-capture category are recorded

#### Scenario: Unresolved failure presented as workflow is refused

- GIVEN a candidate whose evidence shows the failure was never actually
  resolved, only narrated as if a workflow completed
- WHEN drafting is attempted for that candidate
- THEN no draft record is created

#### Scenario: A qualifying candidate is not refused

- GIVEN a candidate whose evidence shows a repeated, verifiable, non-transient
  pattern with a repeatable procedural lesson
- WHEN drafting is attempted for that candidate
- THEN drafting proceeds and is not blocked by the do-not-capture list

### Requirement: Lesson Shape for Summary

When drafting produces or updates a skill's summary content, the system
MUST phrase it as an imperative rule followed by exactly one clause
explaining why, and MUST NOT include observation ids, dates, or PR/issue
numbers in that summary text.

#### Scenario: Drafted summary is an imperative rule plus one why-clause

- GIVEN a qualifying candidate ready for drafting
- WHEN the draft's summary text is generated
- THEN the summary opens with an imperative instruction
- AND it is followed by exactly one clause stating why
- AND it contains no observation id, date, or PR/issue number

#### Scenario: A summary containing an observation id is rejected

- GIVEN a candidate summary draft that embeds a literal observation id
- WHEN the draft is validated against the lesson-shape rule
- THEN the draft is rejected before it is persisted as a draft record

### Requirement: Disposition Decides Extend-Before-Create

The system MUST compute a `Disposition` of either `new` or
`extend:<skill-id>` for every candidate before drafting, and MUST prefer
`extend:<skill-id>` over `new` whenever an existing registered or promoted
skill already covers materially the same procedural ground as the
candidate. When `Disposition` is `extend:<skill-id>`, the draft MUST be
expressed as a diff against the named existing skill, not as a new,
unrelated SKILL.md.

#### Scenario: A candidate overlapping an existing skill extends it

- GIVEN a candidate whose procedural content materially overlaps an
  existing registered skill `skill-id`
- WHEN `Disposition` is computed
- THEN the result is `extend:skill-id`
- AND the resulting draft is a diff against that skill's current content

#### Scenario: A candidate with no overlapping skill creates new

- GIVEN a candidate with no materially overlapping existing skill
- WHEN `Disposition` is computed
- THEN the result is `new`
- AND the resulting draft holds a complete SKILL.md text, not a diff

### Requirement: Draft Records Live Only in Engram

The system MUST store every draft as an Engram record under the topic key
shape `procedural/drafts/{kind}/{slug}`, holding either the complete
SKILL.md text (`Disposition: new`) or a unified diff (`Disposition:
extend:<skill-id>`). The system MUST NOT write any draft file under
`skills/` at any point in the drafting lifecycle.

#### Scenario: A new-disposition draft holds complete SKILL.md text

- GIVEN a candidate with `Disposition: new`
- WHEN its draft record is created
- THEN the record is stored under `procedural/drafts/{kind}/{slug}`
- AND its body is the complete proposed SKILL.md text

#### Scenario: An extend-disposition draft holds a unified diff

- GIVEN a candidate with `Disposition: extend:<skill-id>`
- WHEN its draft record is created
- THEN the record is stored under `procedural/drafts/{kind}/{slug}`
- AND its body is a unified diff against the named skill's current content

#### Scenario: No draft file appears under skills/

- GIVEN any draft created by this capability, in either disposition
- WHEN the `skills/` tree is inspected
- THEN no file corresponding to that draft exists under `skills/`

### Requirement: Extended Status Vocabulary Reflects Drafting Progress

The system MUST extend the candidate/draft `Status` vocabulary beyond
`observing | emitted | rejected` to include the drafting-and-beyond states
needed by this change (at minimum a drafted state distinct from emitted,
and the registration/promotion/retirement states defined by the
registration and maintenance capabilities). A `Status` transition MUST only
move a record forward through this vocabulary in response to an actual
lifecycle event; it MUST NOT be set arbitrarily.

#### Scenario: A drafted candidate reaches drafted status

- GIVEN a candidate that passed the do-not-capture check and produced a
  draft record
- WHEN its status is read after drafting
- THEN it reports a status representing "drafted", distinct from `emitted`

#### Scenario: A refused candidate does not advance past rejected

- GIVEN a candidate refused by the do-not-capture list
- WHEN its status is read after the refusal
- THEN it reports `rejected` and never advances to a drafted state

### Requirement: History Is Append-Only

The system MUST append every status transition and drafting decision (draft
created, draft refused, disposition computed) to the candidate or draft
record's `History` list, and MUST NOT remove, reorder, or overwrite any
existing `History` entry on a subsequent write. This survives Engram's
overwrite-on-upsert behavior because the write always reads the current
`History`, appends, and writes the full list back.

#### Scenario: A second drafting-related write only grows History

- GIVEN a candidate record whose `History` already has one entry from
  detection
- WHEN a drafting decision is recorded for that candidate
- THEN the record's `History` after the write has two entries
- AND the original first entry is unchanged

#### Scenario: History never shrinks across transitions

- GIVEN a candidate/draft record with N `History` entries after some number
  of lifecycle transitions
- WHEN any further lifecycle transition is recorded (draft, registration,
  revision, retirement)
- THEN the record's `History` after that write has at least N+1 entries
