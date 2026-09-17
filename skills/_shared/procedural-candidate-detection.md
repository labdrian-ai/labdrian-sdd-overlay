# Procedural Promotion-Candidate Detection

This contract specifies an in-session, agent-invoked procedure for detecting
and durably storing procedural promotion candidates: repeated-success
patterns and recurring failure-recovery patterns. It covers detection and
storage only — it never drafts a skill, never requests approval, and never
writes under `skills/`. Candidate records persist in Engram, the sole
durability store for this capability; no second SQLite store and no Go-side
write path into Engram's database exist anywhere in this contract.

This document is delivered across three review slices. Sections 1-3 below
ship in slice 1 (`candidate-store`, R-001). Sections 4-5 (emission decision
table, `Status` latch, trigger contract) and section 6 (duplicate rejection)
ship in later slices.

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
