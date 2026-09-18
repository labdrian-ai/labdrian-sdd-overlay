# Procedural Skill Maintenance Specification

## Purpose

Keep a registered or promoted procedural skill honest after it exists:
refuse to touch a skill a human has taken over, revise a skill that keeps
teaching the same lesson after promotion, and surface — without ever
mutating anything — a skill that has drifted from the reality it describes,
so a human or the project-tier agent can decide to retire or consolidate
it.

## Requirements

### Requirement: Ownership-by-Hash Refuses Revision of a Human-Owned Skill

Before any revision write, the system MUST compute the sha256 of the
skill's current on-disk content and compare it to the hash recorded at the
skill's last registration or promotion. When the hashes differ, the system
MUST treat the skill as human-owned, MUST refuse to write a revision, and
MUST report the refusal reason as human ownership rather than a generic
error.

#### Scenario: A hash mismatch refuses revision

- GIVEN a registered skill whose recorded hash no longer matches its
  current on-disk sha256 (a human edited it after registration)
- WHEN a revision is attempted
- THEN the revision write is refused
- AND the refusal reports the skill as human-owned

#### Scenario: A hash match allows revision to proceed

- GIVEN a registered skill whose current on-disk sha256 still matches the
  recorded hash
- WHEN a revision is attempted
- THEN the ownership check passes and revision proceeds to the drafting and
  registration path

#### Scenario: The ownership check reuses the lock-file hash

- GIVEN the project-owned lock file's recorded hash for a skill
- WHEN the ownership-by-hash check runs
- THEN it reads that same recorded hash rather than recomputing or storing
  a second hash elsewhere

### Requirement: OccurrencesSincePromotion Is Derived and Triggers Revision at Either Tier

`OccurrencesSincePromotion` MUST be a derived value, never a stored,
independently incremented counter: it is the count of qualifying
`Occurrences` entries whose timestamp is strictly after the `at:` of the
record's most recent `Registered` or `Promoted` transition line. This
derivation, and the `OccurrencesSincePromotion >= 2` revision trigger,
apply at both tiers — for a `registered` (project-tier) skill it counts
occurrences since registration and opens a project-tier revision draft; for
a `promoted` (global-tier) skill it counts occurrences since promotion and
opens a human-path revision draft. A revision writes a new `Registered` or
`Promoted` `at:` line, which resets the derived count to zero. The system
MUST disclose that this value is a proxy for skill effectiveness, not load
telemetry, since no skill-load instrumentation exists.

#### Scenario: One post-transition occurrence does not trigger revision

- GIVEN a registered or promoted skill whose derived
  `OccurrencesSincePromotion` (occurrences after the latest `Registered` or
  `Promoted` `at:`) equals 1
- WHEN the value is evaluated
- THEN no revision draft is opened

#### Scenario: A second post-transition occurrence triggers revision

- GIVEN a registered or promoted skill whose derived
  `OccurrencesSincePromotion` equals 2
- WHEN the value is evaluated
- THEN a revision draft is opened for that skill, at the tier matching its
  current `Status`

#### Scenario: A revision resets the derived count

- GIVEN a skill whose revision draft was just applied, writing a new
  `Registered` or `Promoted` `at:` line
- WHEN `OccurrencesSincePromotion` is evaluated afterward
- THEN it reads 0, because no `Occurrences` entry yet postdates the new
  `at:` line

### Requirement: Revision Follows the Drafting and Registration Paths Per Tier

A revision draft MUST follow the same do-not-capture, lesson-shape, and
draft-record rules as initial drafting (`procedural-skill-drafting`). At
the project tier, a revision MUST follow the same lint-register-commit
path as initial registration (`procedural-skill-registration`), gated by
the ownership-by-hash check. At the global tier, a revision to a promoted
skill MUST follow the human promotion path; no automated code path revises
a promoted skill without a human decision.

#### Scenario: A project-tier revision produces one scoped commit

- GIVEN a project-tier registered skill that passes the ownership-by-hash
  check and reaches the revision trigger
- WHEN the revision is applied
- THEN it produces exactly one scoped commit, following the same commit
  scope and refusal rules as initial registration

#### Scenario: A promoted skill's revision requires a human

- GIVEN a promoted skill that reaches the revision trigger
- WHEN the revision draft is ready
- THEN no automated path writes the revision into `skills/`
- AND the documented human promotion procedure is the only path that can
  apply it

### Requirement: Retirement Detection Is Report-Only

The system MUST provide a retirement detector that inspects, without
mutating, the paths and commands a procedural skill names — reusing
`longterm-mem/internal/staleness` for path staleness — plus the
candidate's `LastObserved` age and whether a newer skill's
`MatchCandidate` result supersedes it. The detector MUST NOT write, delete,
or modify any file, record, or registry entry; it MUST only produce a
report identifying candidate-for-retirement skills and the reason.

#### Scenario: A skill naming a git-removed path is flagged

- GIVEN a procedural skill whose body names a file path that was removed in
  the repository's git history
- WHEN the retirement detector runs
- THEN the skill is flagged as a retirement candidate with the removed-path
  reason
- AND no file, record, or registry entry is modified by the detector

#### Scenario: A skill with no staleness signal is not flagged

- GIVEN a procedural skill whose named paths and commands are all still
  valid, whose `LastObserved` is recent, and which is not superseded by
  another skill
- WHEN the retirement detector runs
- THEN it is not flagged as a retirement candidate

#### Scenario: The detector performs zero mutation regardless of outcome

- GIVEN a repository containing both flagged and unflagged skills
- WHEN the retirement detector runs and produces its report
- THEN no file under `skills/`, `.claude/skills/`, or `.agents/skills/` is
  modified
- AND no Engram record is modified by the detector itself

### Requirement: Retirement Removal Is a Decision, Scoped by Tier

Acting on a retirement report MUST be a separate decision from producing
the report. At the project tier, retirement removal MUST be agent-owned,
performed as a single commit that is revertable with `git revert`. At the
global tier, retirement removal MUST go through the existing human
`engine skills remove` (`RemoveCore`) path; the retirement detector itself
MUST NOT invoke removal.

#### Scenario: A project-tier retirement is one revertable commit

- GIVEN a project-tier skill flagged for retirement and a decision to
  remove it
- WHEN the agent performs the removal
- THEN it produces exactly one commit removing that skill's files and lock
  entry
- AND `git revert` on that commit restores the skill

#### Scenario: A global-tier retirement requires RemoveCore

- GIVEN a promoted skill flagged for retirement
- WHEN removal is decided
- THEN the removal is performed through `engine skills remove`
  (`RemoveCore`), not by any automated project-tier path

#### Scenario: The detector never triggers removal by itself

- GIVEN a retirement report naming one or more flagged skills
- WHEN the report is produced
- THEN no removal, commit, or record mutation happens as a direct effect of
  producing that report

### Requirement: Consolidation Records an Existence-Verified AbsorbedInto Reference

When a skill is retired because its content has been consolidated into
another skill, the system MUST record `AbsorbedInto: <skill-id>` on the
retired skill's record only after verifying that `<skill-id>` **exists** as
a registered skill at the appropriate tier: for a global (overlay) target,
existence in the overlay skill registry; for a project-tier target,
existence as an entry in the project lock file. This is an existence check,
not a `MatchCandidate` coverage check — exact-identity matching cannot
prove that B covers A's procedural content, so `MatchCandidate` (where used
to look the id up in the registry) serves only as the existence lookup
mechanism, never as proof of coverage. The system MUST refuse to record an
`AbsorbedInto` reference to a skill id that does not exist at the target
tier.

#### Scenario: A verified consolidation into a global skill records AbsorbedInto

- GIVEN a retirement decision that consolidates skill A into existing
  global skill B, where B exists in the overlay skill registry
- WHEN A is retired
- THEN A's record gains `AbsorbedInto: B`

#### Scenario: A verified consolidation into a project skill records AbsorbedInto

- GIVEN a retirement decision that consolidates skill A into existing
  project-tier skill B, where B is an entry in the project lock file
- WHEN A is retired
- THEN A's record gains `AbsorbedInto: B`

#### Scenario: A nonexistent target refuses the AbsorbedInto write

- GIVEN a proposed `AbsorbedInto` target id that exists in neither the
  overlay skill registry nor the project lock file
- WHEN the write is attempted
- THEN the `AbsorbedInto` field is not written
- AND the refusal names the unverified target
