# Procedural Promotion-Candidate Detection

This contract specifies an in-session, agent-invoked procedure for detecting
and durably storing procedural promotion candidates: repeated-success
patterns and recurring failure-recovery patterns. It covers detection and
storage only — it never drafts a skill, never requests approval, and never
writes under `skills/`. Candidate records persist in Engram, the sole
durability store for this capability; no second SQLite store and no Go-side
write path into Engram's database exist anywhere in this contract.

This document is delivered across several review slices. Sections 1-3
shipped in slice 1 (`candidate-store`, R-001). Sections 4-5 shipped in slice
2 (`repeat-and-recovery-detection`, R-002, R-003). Section 6 (duplicate
rejection) shipped in slice 3 (`duplicate-rejection`, R-004). Sections 7-10
shipped with the procedural drafting contract, and section 11 (registration
and commit procedure) ships with `project-register-cli`.

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

## 6. Duplicate candidate rejection (R-004)

This capability MUST reject a promotion candidate whose identity is already
covered by an existing entry in the registered skill registry
(`skills.registry.yaml`). Rejection is backed by a new read-only Go
entrypoint, `skills.MatchCandidate` (`engine/skills/match.go`). The caller
parses `skills.registry.yaml` with the already-exported `ParseRegistry`
function and passes the resulting `Registry` value in; `MatchCandidate`
itself performs no file, network, or other I/O and makes no write to the
registry, to any file under `skills/`, or to any other persisted state.
Matching compares the untruncated normalized form of each identity, never
the form `NormalizeSlug` truncates to 48 bytes, so two distinct identities
that happen to share the same truncated prefix are never treated as
duplicates. Rejection is decided purely on registry contents at check time;
the registry itself is never mutated by this capability.
Comparison always uses the untruncated normalized form, never the truncated form `NormalizeSlug` returns.

### Rejection record fields

Both fields are defined fully here; section 2 introduces them only as
placeholders because in slice 1 and slice 2 no record ever reaches
`Status: rejected`.

- **`RejectionReason`**: present only when `Status: rejected`. In this slice
  the only value is `duplicate` — the candidate's normalized identity exactly
  matches an already-registered skill.
- **`MatchedSkillPath`**: present only when `RejectionReason: duplicate`. It
  holds the `Path` of the registry entry that `MatchCandidate` matched,
  exactly as returned by `MatchCandidate`, so the audit trail names the
  specific registered skill that caused the rejection.

A rejected record is **kept, never discarded**. Because
`skills.MatchCandidate` performs no filesystem or registry write, a rejection
is purely diagnostic: the record documents that a pattern was observed and
recognized as already covered, so every miss and every match stays
auditable.

### `MatchCandidate` call site

`MatchCandidate(registry, slug)` is called at exactly one point in the
emission decision table (section 4): the row where `OccurrenceCount` first
reaches `Threshold`. It is never called per-occurrence, and never before the
threshold is reached — calling it earlier would mark a record `rejected`
before it was ever a real candidate, and calling it on every occurrence would
cost more MCP/registry-read round trips for no additional determinism, since
the registry's answer for the same normalized slug cannot change between one
occurrence and the next within a single detection sweep.
Any per-sweep reuse or memoization of a `MatchCandidate` answer MUST be keyed by the untruncated normalized form, never by the truncated `NormalizeSlug` topic-key slug, because two long candidates sharing a truncated prefix would otherwise share one cached answer and reintroduce the false duplicate this section exists to prevent.

`skills.MatchCandidate(reg Registry, candidate string) (matched bool, skillPath string)`
compares the untruncated normalized form of `candidate` against, for each
registry entry in registry order: the untruncated normalized form of
`entry.ID`, of `entry.Path`, and of `path.Base(entry.Path)`. The comparison
never uses `NormalizeSlug`'s 48-byte-truncated output, because two distinct
identities longer than 48 bytes can share the same truncated prefix without
being the same identity. The first match wins (deterministic).
An empty or all-punctuation candidate never matches. There is **no
substring, prefix, or fuzzy matching** — a candidate like `sdd-spec-review`
never matches a registered skill `sdd-spec`, because under-detection (a
missed duplicate, deferred to a human reviewer) is the accepted, auditable
failure mode, while over-detection (a false duplicate that silently discards
a real candidate before anyone reviews it) is not.

## 7. Do-not-capture list (gates candidate quality and drafting)

This is the single do-not-capture list for this contract. It is defined exactly once, here, and enforced at two independent points in the lifecycle: at emission (this document's own section 4, before a candidate that reaches `Threshold` is allowed to advance past `emitted`) and at draft time (`procedural-skill-drafting`, before any draft record is saved). Neither enforcement point supersedes the other: a candidate that slips past emission-time enforcement is still refused at draft time.

A candidate whose contributing evidence matches any of the following
categories MUST be refused rather than advanced or drafted, and the refusal
MUST be recorded (never silently dropped):

1. **Environment-dependent failure**: a failure that reproduces only under
   one occurrence's local misconfiguration, not a general lesson.
2. **Negative claim about a tool with no independent verification**: an
   assertion that a tool does not work or lacks a capability, with no
   independent confirmation beyond the one occurrence.
3. **Transient error**: a network blip, rate limit, or timeout with no
   recurring cause.
4. **One-off narrative**: a one-off narrative with no repeatable procedural
   content.
5. **Unresolved failure presented as workflow**: a failure narrated as if a
   workflow completed, when the failure was never actually resolved.

A candidate refused by this list transitions to `Status: rejected` with
`RejectionReason: do-not-capture:<class>`, where `<class>` is one of
`environment-dependent`, `unverified-negative-claim`, `transient-error`,
`one-off-narrative`, or `unresolved-presented-as-workflow`. The refusal is
kept, never discarded, exactly like a duplicate rejection (section 6): every
miss and every match stays auditable.

The advisory `incident-log-shape` lint warning (`skill-lint`) is a separate,
non-blocking signal on the rendered SKILL.md body. It never gates emission
or drafting; it only flags a later-stage authoring smell after a draft
already exists.

## 8. Lesson shape, Disposition, and draft records

### Lesson shape for Summary

When drafting produces or updates a skill's summary content, the summary MUST be phrased as an imperative rule followed by exactly one clause explaining why, and MUST NOT include observation ids, dates, or PR/issue numbers. This is enforced by the agent-driven drafting procedure itself,
before any draft record is saved — not by Go code — and is verified by the
acceptance checklist at the end of this document, not a unit test. It is
independent of the advisory `incident-log-shape` lint warning (section 7),
which flags a different, later-stage signal and never blocks drafting.

Example: "Override `HOME`, not `ENGRAM_DATABASE_URL`, because the Engram CLI
and its MCP server never read the latter." is an acceptable summary shape:
one imperative clause, one why-clause, no ids, no dates, no PR/issue
numbers.

### Disposition: new | extend:<skill-id>

Every candidate MUST have a `Disposition` of exactly `new` or `extend:<skill-id>`, computed before drafting. `extend:<skill-id>` MUST be
preferred over `new` whenever an existing registered or promoted skill
already covers materially the same procedural ground as the candidate
(extend-before-create). When `Disposition` is `extend:<skill-id>`, the draft
MUST be a unified diff against the named skill's current content, not an
unrelated new SKILL.md. `Disposition` persists on the candidate record
(section 9) so registration, revision, and retirement can read it without
recomputing the extend-vs-create decision.

### Draft records live only in Engram

Every draft is stored as an Engram observation under the topic-key shape:

    procedural/drafts/{kind}/{slug}

where `{kind}` and `{slug...}` mirror the candidate's own kind and slug tail
(section 1). `type: pattern`, `capture_prompt: false`. No draft file is ever written under `skills/` at any point in the drafting lifecycle — a draft
that reached the `skills/` tree would fail the ondisk gate's
`UNREGISTERED_ON_DISK` check, and registration (section 11) is the only path
that ever writes there.

The draft record's field block:

`````
**Candidate**: <candidate topic key>
**Disposition**: new | extend:<skill-id>
**Status**: open | registered | abandoned
**ForRevision**: <n>
**Lint**: hard 0, warnings <k> (<rule-ids>)
**History**:
- ...
**Body**:
````markdown
<complete SKILL.md text>      (Disposition new)
````
`````

or a ` ```diff ` fenced unified diff when `Disposition` is
`extend:<skill-id>`. A five-backtick outer fence wraps a four-backtick body
fence so the inner fence closes before the outer one does, and an
embedded SKILL.md's own triple-backtick fences do not terminate it early.
`ForRevision` is the latch: at most one open draft exists per revision
number.

## 9. Extended Status vocabulary, transition table, and History

### Status vocabulary

The candidate record's `Status` field takes exactly one value from:

    observing | emitted | rejected | drafted | registered | promoted | retired

The draft record (section 8) has its own, separate `Status` field with its
own vocabulary, `open | registered | abandoned`, defined in section 8's
field block. A draft reaching `registered` moves its candidate record's
`Status` to `registered`; an `abandoned` draft leaves the candidate
record's `Status` at `drafted` until a new draft is opened or the candidate
is retired.

Every value beyond `observing | emitted | rejected` (item 30, sections 4-6)
is reachable only through the corresponding lifecycle transition defined by
this document and by `procedural-skill-registration` /
`procedural-skill-maintenance`; none is ever set directly.

### Transition table (normative)

| From | To | Actor | Tier | Event | Required `History` evidence |
|---|---|---|---|---|---|
| none | `observing` | agent | none | first occurrence (item 30) | `engram:<id>` |
| `observing` | `emitted` | agent | none | count reaches `Threshold`, no `MatchCandidate` match (item 30) | count |
| `observing` | `rejected` | agent | none | `MatchCandidate` match (item 30) | `MatchedSkillPath` |
| `observing` or `emitted` | `rejected` | agent | none | do-not-capture class matched at emission or at draft time | `RejectionReason: do-not-capture:<class>` |
| `emitted` | `drafted` | agent | project | draft record written, `LintSkill` hard = 0 (new) or diff applies cleanly (extend) | draft topic key, `Disposition`, lint counts |
| `drafted` | `registered` | agent | project | `project-register` and commit (new), or `project-revise` of an agent-owned target (extend) | skill id, `sha256`, commit, rev |
| `registered` | `registered` | agent | project | revision (ownership OK, trigger reached) | `sha256`, commit, rev |
| `drafted` or `registered` | `promoted` | human | global | `engine skills add` merged in the overlay | overlay commit, `sha256` |
| `registered` | `retired` | agent | project | `project-retire` and commit | `RetirementReason`, commit |
| `promoted` | `retired` | human | global | `engine skills remove` merged | `RetirementReason`, overlay commit |

Non-transition events append a same-state `History` line (for example
`registered -> registered`, or `promoted -> promoted`) without changing
`Status`: ownership lost, disposition computed, a revision draft opened, a
recurrence after retirement, and the agent's post-promotion removal of the
now-redundant project-tier copies. That last case deserves its own note:
once a human sets `Status: promoted`, the agent MAY run
`project-retire --reason promoted`, which deletes the project-tier files and
their project lock entry and commits, but it is NOT the `registered -> retired` transition in the table above — `Status` stays `promoted`, and only a `promoted -> promoted` `History` line records the removal. Retirement never reopens automatically.

Only a human sets `Status: promoted`. No engine verb ever writes `Status`,
and every project-tier procedure in this document ends at `registered` or
`retired`.

### History format and write rule

One line per entry, append-only, always the last field in the record block:

```
**History**:
- 2026-09-20T10:00:00Z | observing -> emitted | agent | count 3/3, no registry match
- 2026-09-20T10:05:00Z | emitted -> drafted | agent | draft procedural/drafts/repeated-success/probe-engram-with-home-override; Disposition new; lint hard 0 warnings 1 (body-recommended)
- 2026-09-20T10:09:00Z | drafted -> registered | agent | skill probe-engram-with-home-override rev 1 sha256:3f2a...c1 commit a1b2c3d
```

**Write rule**: read the current record with `mem_get_observation`, copy the
existing `History` lines byte-for-byte, append one or more new lines, and
upsert (`mem_save`/`mem_update`). After the write, the old lines MUST be a prefix of the new ones — a subsequent read MUST NOT remove, reorder, or
overwrite any existing `History` entry. This is how a `History` field
survives Engram's overwrite-on-upsert behavior. Records that item 30 created
with no `History` get a backfilled first line on their first write under
this document: `<FirstObserved> | none -> observing | agent | backfilled
from FirstObserved`.

### New candidate-record fields

Appended to item 30's field block (section 2):

- `**Disposition**: new | extend:<skill-id>`
- `**Draft**: procedural/drafts/...`
- `**Registered**: <id> rev:<n> sha256:<hex> commit:<sha> at:<RFC3339>` —
  replaced on revision; the old values survive in `History`.
- `**Promoted**: <id> path:skills/<id>/SKILL.md sha256:<hex> commit:<sha> at:<RFC3339>`
- `**OccurrencesSincePromotion**: <n>` — derived, never a stored,
  independently incremented counter. It counts `Occurrences` entries whose
  timestamp is strictly after the `at:` of the most recent `Registered` or
  `Promoted` line, and applies at either tier (registered project-tier or
  promoted global-tier).
- `**RetirementReason**: stale-reference | superseded | absorbed | promoted | quiet | human-request`
- `**AbsorbedInto**: <skill-id>` — set only after the retirement/
  consolidation path verifies the named id exists (in the overlay registry
  for a global target, or in the project lock file for a project-tier
  target), never by `MatchCandidate` coverage of the retired candidate's own
  identity.

The `Registered`/`Promoted` hash line above is an informational mirror for
humans reading the record. It is never the value the ownership-by-hash check
(`procedural-skill-maintenance`) reads — that check reads only the project
lock file's recorded hash, which is the single source of truth for
ownership.

## 10. Runtime targets

Project-tier registration (`procedural-skill-registration`) writes exactly
two fixed target directories, one `SKILL.md` per directory, and never a
third. `.pi/skills/` is never written by this capability.

| Directory | Runtime(s) | Status | Evidence |
|---|---|---|---|
| `.claude/skills/` | Claude Code | verified | Claude Code loads project skills from `.claude/skills/<id>/SKILL.md` directly; no trust prompt applies. |
| `.agents/skills/` | Pi (always, after project trust); Codex (discovery only, on this host) | verified | Pi: installed Pi docs (`docs/security.md`, `docs/skills.md`) list project `.agents/skills` as a trust-requiring resource; project skills load only after trust. Codex: `codex-smoke.md` (2026-09-18, Codex 0.148.0) recorded verdict `PASS`, scoped to discovery of `.agents/skills/<id>/` (name and description exposed in Codex's skill list); loading the skill body is unverified on this host, because the body-read probe hit a host filesystem-sandbox limitation, not a discovery failure. |

**Pi trust note**: writing to `.agents/skills/` in a project Pi has not yet
trusted makes Pi prompt for project trust once, on its next start, under the
default `defaultProjectTrust: ask`; a non-interactive Pi run ignores the
skill until the project is trusted. This is the accepted, disclosed cost of
writing one Pi-visible copy (revised decision (e)); it also means Pi never
sees a duplicate skill name for the same procedural skill. Every successful
`project-register` run prints a `note:` line disclosing this (section 11).

## 11. Registration and commit procedure

Registration is a two-actor procedure with a hard boundary between them.
The engine prints the exact path set and never runs git; the agent runs every git command as `git -C <project-root>`.
`R` below is that absolute project root, the same value passed as
`--project-root`. The agent performs the ten steps in this order and stops
at the first refusal.

1. **Prove the root.** `git -C R rev-parse --show-toplevel` must equal `R`.
   Anything else means the command was aimed at a subdirectory, a nested
   repository or a submodule, and registering there would write into the
   wrong repository.
2. **Prove the branch.** `git -C R symbolic-ref -q HEAD` must succeed, so a
   detached `HEAD` refuses. Refuse as well if any of `MERGE_HEAD`,
   `CHERRY_PICK_HEAD`, `REVERT_HEAD`, `rebase-merge` or `rebase-apply`
   exists under `git -C R rev-parse --git-dir`: a commit made during one of
   those operations is not the single revertable commit this procedure
   promises.
3. **Prove the index is empty.** `git -C R diff --cached --name-only` must
   print nothing. Any staged path refuses, is named in the refusal, and the
   index is left exactly as it was.
4. **Plan.** `labdrian skills project-register --dry-run --project-root R --candidate <key> <draft-file>`
   prints one `plan: <rel>` line per planned path and writes nothing.
5. **Prove the targets are not ignored.** `git -C R check-ignore -- <plan paths>`
   must print nothing. An ignored target refuses, and `git add -f` is
   forbidden: a skill git cannot see is a skill no reviewer can see.
6. **Write.** The same command without `--dry-run` prints `wrote: <rel>` per
   path, then `sha256: <hex>`, then `revision: <n>`, and finally the Pi
   trust `note:` line. The `wrote:` set is the pathspec for every step
   below.
7. **Stage with an explicit pathspec.** `git -C R add -- <wrote paths>`, then `git -C R diff --cached --name-only` must equal the wrote set exactly. On a mismatch, run `git -C R restore --staged -- <wrote paths>` and refuse.
8. **Commit with an explicit pathspec.** `git -C R commit -m "<conventional message>" -- <wrote paths>`.
   Messages: `feat(skills): register project skill <id>`, `feat(skills): revise project skill <id>` or `chore(skills): retire project skill <id>`.
   No AI attribution, no `-a`, no `--no-verify`, no amend, no push.
9. **Confirm ownership.** `labdrian skills project-status --project-root R <id>`
   must report `owner:agent`. A commit hook that rewrote the committed bytes
   makes the skill human-owned; that is reported to the user, never
   auto-corrected.
10. **Record the commit.** Record `git -C R rev-parse --short HEAD` in the
    candidate record's `Registered` line and in `History`.

The pathspec on steps 7 and 8, together with the empty-index precondition of
step 3, are two independent protections against sweeping unrelated work into
this commit. Neither one is dropped when the other holds.

### Refusals (normative)

Every row refuses without writing anything and without leaving the
repository in a state the operator did not ask for.

| Refusal condition | Detected by | What the agent does |
|---|---|---|
| The command runs from a subdirectory, a nested repository or a submodule | `git -C R rev-parse --show-toplevel` does not equal `R` | Refuse before step 4; nothing is planned, read or written |
| `HEAD` is detached | `git -C R symbolic-ref -q HEAD` fails | Refuse before step 4 |
| A merge, cherry-pick, revert or rebase is in progress | `MERGE_HEAD`, `CHERRY_PICK_HEAD`, `REVERT_HEAD`, `rebase-merge` or `rebase-apply` exists under `git -C R rev-parse --git-dir` | Refuse before step 4 |
| Unrelated changes are already staged | `git -C R diff --cached --name-only` is non-empty | Refuse, naming every staged path, and leave the index untouched |
| A planned target is gitignored | `git -C R check-ignore -- <plan paths>` prints a path | Refuse; `git add -f` is forbidden |
| The staged set does not equal the wrote set | `git -C R diff --cached --name-only` after `git -C R add` differs | Unstage with `git -C R restore --staged -- <wrote paths>` and refuse |

### Crash-window recovery

There is one window the engine cannot close for the agent: after step 6 has
printed and before the commit of step 8 lands. Recovery in that window is
deliberately narrow, because the repository may hold work that has nothing
to do with this registration.

The newly written SKILL.md files are always untracked (each lives under a brand-new skill directory), and are deleted only when their bytes hash to the sha256 the failed run printed.
A file whose hash differs was touched by someone else and is left alone for
a human. The lock file needs separate handling because it may already be
tracked with other skills' entries: when it existed in `HEAD` before this run, restore it byte-for-byte with `git -C R restore --source=HEAD -- <lock path>`; when this run created it for the first time in the repository (so it is untracked), delete it instead, after the same hash check.
A revision or retirement runs `git -C R restore --source=HEAD --staged --worktree -- <wrote paths>`. `git clean` and `git reset --hard` are never used.

### The Pi trust note

Every successful run prints `note: Pi loads .agents/skills only after the project is trusted; Pi may prompt once for project trust.` as its last line,
the disclosure section 10 requires. The agent relays every `note:` line to the user and never uses one as a pathspec.
A refusal and a `--dry-run` both print no `note:` line, because neither one
creates the consequence it discloses.

## Acceptance checklist (procedural-skill-registration, executed during `sdd-verify`)

This section is agent-driven prose, not Go code; it is verified by a
fixture-driven procedure run in a temporary git repository with real Engram
records during `sdd-verify`, not by `go test`. Contract-artifact content
(section 11 above) is still Go-testable and covered by
`engine/skills/procedural_candidate_contract_test.go`.

1. **Clean repository gives exactly one commit**: GIVEN a clean project
   repository on a branch, WHEN the ten steps run for a valid draft, THEN
   exactly one commit is created and its changed paths equal the `wrote:`
   set exactly.
2. **An unrelated staged file refuses with the index untouched**: GIVEN an
   unrelated path is already staged, WHEN step 3 runs, THEN the procedure
   refuses naming that path, and `git diff --cached --name-only` afterwards
   is byte-identical to what it was before.
3. **An ignored target refuses**: GIVEN `.claude/skills/` is gitignored,
   WHEN step 5 runs, THEN the procedure refuses and no `git add -f` is
   attempted.
4. **Detached `HEAD` refuses**: GIVEN the repository is on a detached
   `HEAD`, WHEN step 2 runs, THEN the procedure refuses before anything is
   planned.
5. **A merge or rebase in progress refuses**: GIVEN `MERGE_HEAD` or
   `rebase-merge`/`rebase-apply` exists, WHEN step 2 runs, THEN the
   procedure refuses before anything is planned.
6. **A subdirectory or nested repository never registers against the wrong
   root**: GIVEN the procedure is invoked from a subdirectory of the project
   or from a nested repository root, WHEN step 1 runs, THEN the root is
   either resolved to `R` or refused, and never silently registered against
   another repository.
7. **A staged-set mismatch unstages and refuses**: GIVEN an unexpected path
   is staged alongside the wrote set after step 7's `git add`, WHEN the
   staged set is compared, THEN the procedure runs
   `git restore --staged -- <wrote paths>` and refuses.
8. **`git revert` restores the files and the lock**: GIVEN the registration
   commit exists, WHEN it is reverted, THEN every written `SKILL.md` is gone
   and the lock file is byte-identical to its pre-registration state.
9. **A hook that rewrites the bytes is reported, not fixed**: GIVEN a commit
   hook rewrites the committed `SKILL.md`, WHEN step 9 runs, THEN
   `project-status` reports the skill human-owned and the procedure reports
   that rather than re-registering.
10. **A crash between `wrote:` and the commit deletes only this run's
    files**: GIVEN a second registration fails after its `wrote:`/`sha256:`
    output but before its own commit, WHEN crash-window recovery runs, THEN
    only that run's untracked, hash-verified `SKILL.md` files are deleted,
    the lock is restored from `HEAD`, and a previously registered skill's
    lock entry and committed files survive unchanged.
11. **`History` only grows**: GIVEN a candidate record that moved
    `drafted -> registered`, WHEN the record is read back, THEN the
    pre-registration `History` lines are a prefix of the post-registration
    ones.

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

## Acceptance checklist (R-004, executed during `sdd-verify`)

This section is agent-driven prose paired with a Go-testable entrypoint: the
`skills.MatchCandidate` function itself is covered by table tests in
`engine/skills/match_test.go` (exact ID/Path/`path.Base` hits, the substring
near-miss guard, empty/all-punctuation candidates, and deterministic
first-match-wins). The two scenarios below are the acceptance-level checks,
run against real Engram records and a real `skills.registry.yaml` during
`sdd-verify`, not by `go test`.

1. **Candidate slug equal to a registered skill id/path is rejected**: GIVEN
   a candidate's normalized slug exactly matches a registered skill's `ID`,
   `Path`, or `path.Base(Path)` in `skills.registry.yaml`, WHEN the candidate
   reaches `OccurrenceCount: Threshold` and `MatchCandidate` runs, THEN the
   record's `Status` becomes `rejected`, `RejectionReason: duplicate` is set,
   `MatchedSkillPath` is set to the matched entry's `Path`, and no candidate
   is emitted.
2. **Uncovered candidate emits normally**: GIVEN a candidate's normalized
   slug matches no entry in `skills.registry.yaml`, WHEN the candidate
   reaches `OccurrenceCount: Threshold` and `MatchCandidate` runs, THEN the
   record's `Status` becomes `emitted` as in the ordinary emission decision
   table (section 4), and neither `RejectionReason` nor `MatchedSkillPath`
   is set.

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

## Acceptance checklist (procedural-skill-drafting, executed during `sdd-verify`)

This section is agent-driven prose, not Go code; it is verified by a
fixture-driven procedure run against real Engram records during
`sdd-verify`, not by `go test`. Contract-artifact content (sections 7-10
above) is still Go-testable and covered by
`engine/skills/procedural_candidate_contract_test.go`.

1. **Summary with an observation id is rejected**: GIVEN a candidate
   summary draft that embeds a literal `engram:<id>` reference, WHEN the
   draft is validated against the lesson-shape rule (section 8), THEN the
   draft is rejected before any draft record is saved.
2. **Summary with a literal date is rejected**: GIVEN a candidate summary
   draft that embeds a literal date, WHEN the draft is validated against the
   lesson-shape rule, THEN the draft is rejected before any draft record is
   saved.
3. **Summary with a PR/issue number is rejected**: GIVEN a candidate summary
   draft that embeds a PR or issue number, WHEN the draft is validated
   against the lesson-shape rule, THEN the draft is rejected before any
   draft record is saved.
4. **A clean lesson-shaped summary passes**: GIVEN a candidate summary draft
   phrased as an imperative rule followed by exactly one why-clause, with no
   observation id, date, or PR/issue number, WHEN the draft is validated,
   THEN validation passes and its draft record is saved under
   `procedural/drafts/{kind}/{slug}`.
5. **`incident-log-shape` never gates drafting**: GIVEN a rendered SKILL.md
   body that triggers the advisory `incident-log-shape` lint warning
   (`skill-lint`), WHEN drafting or registration is attempted, THEN the
   warning is reported as a separate, non-blocking signal and never refuses
   the draft or the registration.
