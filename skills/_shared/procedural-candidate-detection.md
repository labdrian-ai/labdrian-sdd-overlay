# Procedural Promotion-Candidate Detection

This contract specifies an in-session, agent-invoked procedure for detecting
and durably storing procedural promotion candidates: repeated-success
patterns and recurring failure-recovery patterns. It covers detection and
storage only — it never drafts a skill, never requests approval, and never
writes under `skills/`. Candidate records persist in Engram, the sole
durability store for this capability; no second SQLite store and no Go-side
write path into Engram's database exist anywhere in this contract.

This document is delivered across three review slices. Sections 1-3 shipped
in slice 1 (`candidate-store`, R-001). Sections 4-5 below ship in slice 2
(`repeat-and-recovery-detection`, R-002, R-003). Section 6 (duplicate
rejection) ships in slice 3.

## 1. Candidate identity and topic-key contract

Every promotion candidate is identified by one of two topic-key shapes.
`{kind}` is part of identity: the same slug seen under both kinds is two
distinct candidates by design.

```
procedural/candidates/repeated-success/{approach-slug}
procedural/candidates/failure-recovery/{failure-slug}/{recovery-slug}
```

Candidate identity is defined at **sub-step / debugging-pattern granularity**,
not whole-session or whole-feature granularity. A candidate id is never
derived from a whole-session or whole-feature identifier alone.

Every segment (`{approach-slug}`, `{failure-slug}`, `{recovery-slug}`) is
produced by `skills.NormalizeSlug` (`engine/skills/match.go`), the single,
exported, machine-verified definition of identity normalization: lowercase
ASCII, every run of characters outside `[a-z0-9]` collapsed to a single `-`,
leading/trailing `-` trimmed, truncated to at most 48 bytes at the last `-`
boundary at or before 48. This document cites `NormalizeSlug` rather than
restating the rule; every segment matches `[a-z0-9]([a-z0-9-]*[a-z0-9])?`,
max 48 characters.

### Slug grammar

| Segment | Grammar | Example |
|---|---|---|
| `{approach-slug}` | `{verb}-{object}[-{qualifier}]` | `probe-engram-with-home-override` |
| `{failure-slug}` | `{subject}-{symptom}` | `sqlite-attempt-to-write-readonly` |
| `{recovery-slug}` | `{verb}-{object}[-{qualifier}]` | `override-home-not-database-url` |

### Aliases: drift-prevention rule

Before minting a new slug, the detector runs one namespace search over
`procedural/candidates/{kind}` and MUST reuse an existing record whose slug
or alias already covers the same pattern, appending the new phrasing to
`Aliases` instead of creating a sibling record. This is a required step, not
an optimization: two sessions can independently pick different but
equivalent phrasings (for example `rebase-worktree-onto-main` vs.
`rebase-branch-onto-main`) for the same underlying pattern, and the alias
search is what keeps the first session's naming choice sticky and auditable.

## 2. Candidate record — Engram observation

Each candidate is one Engram observation. `title` and `topic_key` are both
the exact topic key from section 1. `type: pattern`. `project`: the resolved
project. `capture_prompt: false` (this is an automated pipeline artifact, not
a human/proactive memory save).

```markdown
**Kind**: repeated-success | failure-recovery
**Candidate**: <approach-slug>              # failure-recovery: <failure-slug>/<recovery-slug>
**Aliases**: <phrasing>; <phrasing>         # alternate phrasings folded into this candidate; "none" if empty
**Status**: observing | emitted | rejected
**RejectionReason**: duplicate              # present only when Status=rejected
**MatchedSkillPath**: <registry entry path> # present only when RejectionReason=duplicate
**Threshold**: 3
**OccurrenceCount**: 2
**FirstObserved**: 2026-09-15T10:04:00Z
**LastObserved**: 2026-09-16T08:12:00Z
**Occurrences**:
- engram:3401 @ 2026-09-15T10:04:00Z — <one-line summary of this occurrence>
- engram:3407 @ 2026-09-16T08:12:00Z — <one-line summary of this occurrence>
**Summary**: <what the approach is, and what it is good for, in one or two sentences>
```

`RejectionReason` and `MatchedSkillPath` are defined fully in section 6
(duplicate rejection, slice 3); in slice 1 and slice 2 no record reaches
`Status: rejected`, so these fields are simply absent.

### Threshold

`Threshold` is declared per-record, written at creation time, and defaults
to **3**. Writing the threshold into the record (rather than relying solely
on a shared constant) means a record is always interpretable under the
threshold it was emitted against, even if the shared default value changes
later.

## 3. Occurrence identity and dedupe rules

1. **Occurrence identity is the Engram observation id.** Every occurrence
   reference in `Occurrences` is `engram:<id>`, the id of the ordinary
   episodic observation that anchors that occurrence. A pattern with no
   Engram observation is not an occurrence — the live conversation is only
   the detector's signal; before an occurrence may be counted, the agent
   must have saved (or must save) an episodic observation for it and
   reference that id here.
2. **Appending an already-listed occurrence id is a no-op.** Re-running the
   detector against the same occurrence, including within the same session,
   is therefore safe by construction and never inflates the count.
3. **`OccurrenceCount` is always derived**, equal to the number of distinct
   ids currently listed in `Occurrences`. It is never blindly incremented.
4. Evidence references MUST remain retrievable after a cold session start:
   `mem_get_observation(id)` for every id listed in `Occurrences` must
   resolve, in any session, to the original occurrence content.
5. The topic-key shape used to store and read back a candidate is identical
   across every session that observes it — the key is derived deterministically
   from candidate identity (section 1), never from session or transcript
   state, so the same pattern observed in session B resolves to the same key
   used in session A and counting is a direct lookup rather than a recall
   search.

## 4. Emission decision table and the `Status` latch (R-002, R-003)

This section, and section 5 below, are agent-driven prose, not Go code:
Go-sense TDD does not apply to R-002/R-003 because there is no compiled
logic to make RED/GREEN. Contract-artifact content (this section's verbatim
text) is still Go-testable and covered by
`engine/skills/procedural_candidate_contract_test.go`. The equivalent gate
for the emission and clustering behavior itself is the fixture-driven
acceptance checklist at the end of this section, executed against real
Engram records during `sdd-verify` — this is a deliberate, disclosed
deviation from the proposal's literal "unit tests over the threshold
boundary" wording (see design.md, "R-002/R-003 boundary coverage is an
executable acceptance checklist, not a Go unit test").

`Threshold: 3` is the shared default `N` for both R-002 and R-003, written
into each record's `Threshold` field at creation (section 2) so a record
stays interpretable under the threshold it was emitted against even if the
default changes later.

### Emission decision table (normative)

| Current `Status` | Occurrence id already listed | `OccurrenceCount` after append | `MatchCandidate` | Resulting `Status` | Emitted this turn |
|---|---|---|---|---|---|
| — (no record) | — | 1 | not called | `observing` | No |
| `observing` | yes | unchanged | not called | `observing` | No |
| `observing` | no | < N | not called | `observing` | No |
| `observing` | no | = N | no match | `emitted` | **Yes, once** |
| `observing` | no | = N | match | `rejected` | No |
| `emitted` | no | > N | not called | `emitted` | No |
| `rejected` | no | > N | not called | `rejected` | No |

"Emitted" in this slice means exactly: the record reaches `Status: emitted`
and the agent reports it once in that turn's reply. Nothing drafts,
approves, or registers a skill — a future consumer subscribes by reading
`Status: emitted` records.

### `Status` latch rule

`Status` transitions `observing → emitted` exactly once, on the single
update where `OccurrenceCount` first reaches `Threshold`. Once `emitted`,
later occurrences still append to `Occurrences` (evidence stays complete)
but the record never re-emits. Because the latch lives in the durable
record rather than in agent memory, "no second candidate at N+1" survives a
session boundary.

`MatchCandidate` (section 6, slice 3) is called only at the row where
`OccurrenceCount` first reaches `Threshold` — never on every occurrence, and
never before the threshold is reached.

### Failure-recovery two-segment clustering

`failure-recovery` candidates use the two-segment topic key from section 1:
`procedural/candidates/failure-recovery/{failure-slug}/{recovery-slug}`.
Clustering falls directly out of that key shape, with no separate algorithm:

- **Same failure, same recovery, observed repeatedly**: every occurrence
  resolves to the same two-segment key, so the count accumulates on one
  record and the emission decision table above applies normally.
- **Same failure, divergent recoveries**: each distinct recovery produces a
  sibling key under the same `{failure-slug}` parent (`{failure-slug}/{recovery-slug-a}`,
  `{failure-slug}/{recovery-slug-b}`, ...). Each sibling's own count stays at
  1 (or whatever its own occurrence count is), so none of them reaches
  `Threshold` from occurrences that belong to a different sibling. A
  divergent recovery never contributes to another recovery's count.
- **Mixed cluster**: when more than `Threshold` occurrences share the same
  failure but only a subset share the same recovery, only the occurrences
  matching that shared recovery accumulate on that recovery's key. A
  candidate emits only when one specific `{failure-slug}/{recovery-slug}`
  key reaches `Threshold`; occurrences under a different sibling key never
  count toward it.

## 5. Trigger contract (T1, T2)

The detection loop runs in-session and agent-invoked, at exactly two
trigger points. No hook is added for either trigger (see design.md, OQ-2).

| Trigger | When it fires | Role |
|---|---|---|
| **T1 — occurrence anchor** | Immediately after any proactive `mem_save` of type `bugfix`, `pattern`, `discovery`, or `decision` | That save **is** the occurrence evidence: the observation id it just created is the occurrence reference appended to `Occurrences` (section 3). No separate scan is needed — anchor and trigger are one event. |
| **T2 — session sweep** | During the mandatory session-close protocol, beside `mem_session_summary` | Backstop for occurrences whose episodic save happened before this contract was loaded in the session, or that were otherwise not swept by T1. |

An irregularly-firing detector is late, never wrong: the `Status` latch
(section 4) means a missed T1/T2 firing costs a delayed candidate, not a
duplicate emission or a miscounted `OccurrenceCount`.

## Acceptance checklist (R-002, R-003, executed during `sdd-verify`)

This section is agent-driven prose, not Go code; it is verified by a
fixture-driven procedure run against real Engram records during
`sdd-verify`, not by `go test`. Contract-artifact content (the emission
decision table and `Threshold: 3` above) is still Go-testable and covered by
`engine/skills/procedural_candidate_contract_test.go`.

1. **N-1 (no emission)**: GIVEN the same approach has succeeded N-1 times
   (2 occurrences, `Threshold: 3`), WHEN detection runs after the N-1th
   success, THEN the record's `Status` stays `observing` and nothing is
   emitted.
2. **N (exactly one emission referencing all N occurrence ids)**: GIVEN the
   same approach now succeeds for the Nth time, WHEN detection runs, THEN
   the record transitions to `Status: emitted` exactly once, and
   `Occurrences` lists all N distinct `engram:<id>` references.
3. **N+1 (no second emission)**: GIVEN a candidate already reached
   `Status: emitted` at N, WHEN that approach succeeds an (N+1)th time,
   THEN no second candidate is emitted, the existing record MAY be updated
   with the new occurrence (appended to `Occurrences`, `OccurrenceCount`
   incremented), and no new candidate identity is created.
4. **A single non-repeated success (nothing emitted)**: GIVEN an approach
   has succeeded exactly once with no prior occurrences, WHEN detection
   runs, THEN no candidate record reaches `Status: emitted` and no emission
   is reported.
5. **Same-failure/same-recovery repeated N times (one candidate)**: GIVEN
   the same failure is resolved by the same recovery action N times, WHEN
   detection runs after the Nth matching occurrence, THEN exactly one
   candidate is emitted, it carries `Kind: failure-recovery`, and it
   references all N contributing occurrences.
6. **Same-failure/divergent-recovery N times (nothing emitted)**: GIVEN the
   same failure occurs N times but each occurrence is resolved by a
   different recovery action, WHEN detection runs after the Nth occurrence,
   THEN no candidate is emitted for that failure — each sibling
   `{failure-slug}/{recovery-slug}` key's own count stays below `Threshold`.
7. **Mixed cluster (matching subset only)**: GIVEN the same failure occurs
   more than N times and exactly N of those occurrences share the same
   recovery action while the rest diverge, WHEN detection runs, THEN a
   `failure-recovery` candidate is emitted referencing only the matching
   subset of N occurrences, and the divergent occurrences are excluded from
   that candidate's `OccurrenceCount`.

## Acceptance checklist (R-001, executed during `sdd-verify`)

This section is agent-driven prose, not Go code; it is verified by a
fixture-driven procedure run against real Engram records during
`sdd-verify`, not by `go test`. Contract-artifact content (this section's
verbatim text) is still Go-testable and covered by
`engine/skills/procedural_candidate_contract_test.go`.

1. **Cross-session cold-start scenario**:
   - GIVEN a candidate is first observed in session A and persisted via
     `mem_save` with `OccurrenceCount: 1` and one occurrence reference
     (`engram:<id-A>`)
   - WHEN the same candidate is observed again in session B, after a full
     cold start (no residual session state)
   - THEN `mem_search` for the exact topic key from section 1 finds the
     record, and `mem_get_observation` reads back `OccurrenceCount: 2`
   - AND both `engram:<id-A>` (session A) and `engram:<id-B>` (session B)
     resolve via `mem_get_observation`
   - AND the topic-key shape used in session B is byte-identical to the one
     used in session A
