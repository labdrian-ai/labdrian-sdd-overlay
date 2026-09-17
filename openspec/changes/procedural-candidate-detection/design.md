# Design: Procedural promotion-candidate detection and storage

## Technical Approach

Approach 3 (hybrid), as committed by the proposal. The detection loop is an **in-session, agent-invoked procedure** — no hook, no daemon, no Go-side write path — specified once as a shared contract document at `skills/_shared/procedural-candidate-detection.md`. Persistence is ordinary Engram MCP traffic (`mem_search` → `mem_get_observation` → `mem_save`/`mem_update`), exactly like every other Engram write in this repository.

The design's load-bearing idea is that **the Engram topic key is the clustering mechanism, not just an address**. A candidate's identity is derived deterministically from the pattern, so the same pattern observed in session B resolves to the same key as in session A, and counting becomes a direct lookup rather than a recall search. For failure-recovery, the key is two-segment (`{failure}/{recovery}`), so "same failure, same recovery" and "same failure, divergent recoveries" are structurally different key sets — R-003's discrimination falls out of the identity contract instead of needing separate logic.

One new pure, read-only Go file, `engine/skills/match.go`, carries the two functions that must be machine-deterministic: `NormalizeSlug` (the single definition of identity normalization, used by both the record contract and the match) and `MatchCandidate` (R-004). Both are layered on the already-exported `ParseRegistry` and touch neither the filesystem nor Engram.

Three review slices: `candidate-store` (R-001) → `repeat-and-recovery-detection` (R-002+R-003) and `duplicate-rejection` (R-004).

## Architecture Decisions

### Decision: OQ-2 — the detection loop runs in-session, agent-invoked, with no new hook

**Choice**: A shared contract document at `skills/_shared/procedural-candidate-detection.md`, invoked by the agent at two in-session trigger points. **No hook is added in this slice.**

| Trigger | When | Why it is the right point |
|---|---|---|
| T1 — occurrence anchor | Immediately after any proactive `mem_save` of type `bugfix`, `pattern`, `discovery`, or `decision` | That save **is** the occurrence evidence. Detection at this point needs no scan: the observation id it just created is the occurrence reference. Anchor and trigger are one event. |
| T2 — session sweep | During the mandatory session-close protocol, beside `mem_session_summary` | Backstop for occurrences saved before the contract was loaded, and the only point in the repo's agent protocol that is already declared mandatory. |

**Alternatives considered**:
- *A new `SessionEnd` hook* (modeled on `sync-trigger`). Rejected on a structural, not stylistic, ground: a `SessionEnd` hook fires when the conversation is over, so **there is no LLM turn left to perform the `mem_save`**. With no Go-side write path into Engram's database (`longterm-mem/internal/engram/store.go` is `mode=ro&_query_only=true` by construction), a `SessionEnd` hook can read and log but can never persist a candidate. It would be a detector that cannot record what it detected.
- *A new `SessionStart` hook* injecting a reminder. Rejected: it can only nudge the next session, which is strictly weaker than T1/T2, while opening a net-new process-integration boundary (settings.json merge/remove, identity token, idempotency tests, uninstall path) for zero persistence capability.
- *Both hook and skill.* Rejected as the union of both costs with none of the extra capability.

**Rationale**: Every option here is net-new surface — the repo has exactly one hook (`SessionEnd`/`sync-trigger`) and no `SessionStart`/`Stop`/`PostToolUse` hook to extend. Given that, pick the surface that can actually complete the write. The known weakness (nothing *forces* an agent-invoked loop to fire) is mitigated structurally rather than by hoping: T1 rides an already-mandatory save, and the record's `Status` latch (see below) makes an irregularly-firing detector *late*, never *wrong* — a missed firing costs a delayed candidate, not a duplicate or a miscount.

**Placement note**: `skills/_shared/` is infra, not a skill. `engine/skills/manifest.go:13` lists `_shared/` in `infraPrefixes`, so files there are excluded from skill manifest rows. Adding this document creates **no registry entry, no manifest skill row, and no `engine skills add` invocation** — the proposal's out-of-scope ban on writes under `skills/` targets the skill-promotion write path (registry/manifest mutation), which this design does not touch. Siblings in the same class: `skills/_shared/engram-convention.md`, `skills/_shared/openspec-convention.md`, `skills/_shared/skill-resolver.md`.

### Decision: OQ-6 — evidence is Engram observations only; every occurrence must be anchored by an observation id

**Choice**: The candidate record's evidence references are **always Engram observation ids** (`engram:<id>`). A pattern with no Engram observation is not an occurrence. The live conversation is the detector's *signal*, but before an occurrence may be counted, the agent must have saved (or must save) an ordinary episodic observation for it, then reference that id.

**Alternatives considered**:
- *Transcripts (session JSONL) as evidence.* Rejected: transcript layout is per-runtime (Claude Code, Codex, Pi all differ — see the memory index's "Agent runtimes in use"), has no stable addressable identity for a sub-step, is not readable by any Go code in this repo, and is subject to rotation/compaction. R-001 requires evidence that is *retrievable* after a cold start; a transcript offset is not.
- *Both.* Rejected: a two-source evidence model needs a merge/dedupe rule between two address spaces with no common key, which is strictly more contract for no new recall.

**Rationale**: Engram observation ids are stable, cross-session, project-scoped, and confirmed Go-readable read-only (`Store.ObservationByID`, `Store.ListObservations`), which keeps a future read-side consumer (umbrella items 31–35: drafting, audit, health report) buildable in Go without revisiting this contract. It also collapses the "how do we count across sessions" problem into the dedupe rule in the next decision.

### Decision: Candidate identity and topic-key contract

**Choice**: Two topic-key shapes, at OQ-1's confirmed sub-step / debugging-pattern granularity:

```
procedural/candidates/repeated-success/{approach-slug}
procedural/candidates/failure-recovery/{failure-slug}/{recovery-slug}
```

`{kind}` is part of identity; the same slug seen under both kinds is two candidates by design. Every segment is produced by `skills.NormalizeSlug` (below) and matches `[a-z0-9]([a-z0-9-]*[a-z0-9])?`, max 48 characters per segment.

Slug grammar (the doc fixes it so two sessions converge on the same text):

| Segment | Grammar | Example |
|---|---|---|
| `{approach-slug}` | `{verb}-{object}[-{qualifier}]` | `probe-engram-with-home-override` |
| `{failure-slug}` | `{subject}-{symptom}` | `sqlite-attempt-to-write-readonly` |
| `{recovery-slug}` | `{verb}-{object}[-{qualifier}]` | `override-home-not-database-url` |

**Alternatives considered**:
- *`procedural/{project}/candidates/{id}`* (exploration's sketch). Rejected: Engram observations already carry a `project` column and `mem_save` takes `project` explicitly, so the segment is redundant and would silently fork a project's candidates if the resolved project name ever changes (a real failure mode here — see "Engram in worktrees needs explicit project" in the memory index).
- *Flat `procedural/candidates/{slug}` with `kind` as a body field.* Rejected: it loses the free namespace listing (`mem_search "procedural/candidates/failure-recovery"`) and, more importantly, loses the two-segment failure-recovery shape that makes R-003's divergent-recovery case structural.

**Rationale**: The two-segment failure-recovery key is what makes R-003 cheap and correct. Same failure recovered the same way N times → **one** key, count N → emit. Same failure recovered N different ways → **N sibling keys** under the same `{failure-slug}` parent, each count 1 → emit nothing. No separate clustering algorithm exists to get wrong.

### Decision: Candidate record body — a fixed field block with a `Status` latch

**Choice**: The observation content is a fixed field block (see Interfaces / Contracts). Four rules govern it:

1. **Occurrence identity is the Engram observation id.** Appending an occurrence whose id is already listed is a no-op. Re-running the detector in the same session is therefore safe by construction.
2. **`OccurrenceCount` is derived**, always equal to the number of distinct ids in `Occurrences`. It is never blindly incremented.
3. **`Status` is the exactly-once latch.** `observing → emitted` happens on the single update where `OccurrenceCount` first reaches `Threshold`. Once `emitted`, later occurrences still append (evidence stays complete) but never re-emit. Because the latch lives *in the durable record*, "no second candidate at N+1" survives a session boundary rather than depending on an agent remembering.
4. **Duplicate rejection runs at the emission boundary only.** When the count first reaches `Threshold`, call `MatchCandidate`; on a match the record becomes `Status: rejected`, `RejectionReason: duplicate`, `MatchedSkillPath: <path>`, and nothing is emitted. The record is kept, so every miss is auditable.

**Alternatives considered**: an ephemeral in-session counter with emission decided per turn (rejected — cannot satisfy R-001's cold-start criterion and cannot suppress a second emission at N+1 across sessions); running `MatchCandidate` on every occurrence (rejected — same verdict, more calls, and it would mark a record rejected before it was ever a candidate).

**Rationale**: Every idempotency guarantee R-001/R-002 asks for is expressed as a property of the stored record, not as agent discipline. That is the only form of idempotency an agent-driven loop can actually keep.

### Decision: `Aliases` field to counter slug drift

**Choice**: The record carries `**Aliases**`, a list of alternate phrasings already folded into this candidate. Before minting a new slug, the detector runs one namespace search over `procedural/candidates/{kind}` and MUST reuse an existing record whose slug or alias covers the same pattern, appending the new phrasing to `Aliases` instead of creating a sibling.

**Alternatives considered**: relying on the slug grammar alone (rejected — grammar makes formatting deterministic, but two sessions can still pick `rebase-worktree-onto-main` and `rebase-branch-onto-main` for the same thing); embedding-based similarity (rejected — no embedding surface is available to the agent loop here, and it would be unverifiable).

**Rationale**: Slug drift is the single biggest threat to cross-session counting, and it is the one part of identity an LLM decides freely. Aliases make the *first* session's naming choice sticky and auditable, at the cost of one extra search per new candidate.

### Decision: Threshold N — one shared default of 3, recorded per candidate

**Choice**: `N = 3` for both R-002 and R-003. The value is declared once in the contract document and **written into each record's `Threshold` field at creation**, so a record is always interpretable under the threshold it was emitted against.

**Alternatives considered**: a lower N (2) for `failure-recovery`, on the argument that recovery knowledge is this repo's dominant learning shape and three recurrences of the same failure is expensive. Rejected **for now** as unevidenced: nothing measured here says a two-occurrence recovery is worth promoting, and diverging the constants would double the boundary test matrix before any data justifies it. Also rejected: a single global constant with no per-record copy (a later change to N would silently re-interpret every historical record).

**Rationale**: The boundary logic is identical for both kinds, so one constant keeps the rule and its acceptance table uniform. The per-record `Threshold` field is the cheap option that keeps independent configurability open: umbrella items 31–35 can lower `failure-recovery`'s N without invalidating anything already stored.

### Decision: R-004 matching is exact identity match after normalization, with a named extension point

**Choice**: `MatchCandidate(reg Registry, candidate string) (matched bool, skillPath string)` compares `NormalizeSlug(candidate)` against, for each entry in registry order: `NormalizeSlug(entry.ID)`, `NormalizeSlug(entry.Path)`, and `NormalizeSlug(path.Base(entry.Path))`. First match wins (deterministic). Empty or all-punctuation candidate → `(false, "")`. **No substring, prefix, or fuzzy matching.**

**Alternatives considered**:
- *Trigger/keyword overlap.* Not available: `engine/skills/types.go` defines exactly `Entry{ID, Path, Source, Install, Lifecycle}` and the hand-rolled parser rejects any unknown key (`parse.go:400`). There is no field to read.
- *Semantic similarity over `SKILL.md` frontmatter/description.* Rejected for this slice: it requires filesystem reads inside a package that is deliberately filesystem-free for validation (`validateEntry`'s doc comment, `parse.go:599-603`), it is non-deterministic, and its false positives are the expensive direction — wrongly rejecting a real candidate as `duplicate` silently deletes knowledge, while a miss merely defers it.
- *Substring matching* (e.g. candidate `sdd-spec-review` matching skill `sdd-spec`). Explicitly rejected and given a RED test: it manufactures false duplicates.

**Extension point (documented, not built)**: when the registry schema grows a trigger/keyword field, add `MatchCandidateBy(reg, candidate, fields ...MatchField)` **beside** `MatchCandidate`; `MatchCandidate`'s contract stays "exact identity match after normalization" forever, so existing rejection records keep their meaning.

**Rationale**: Under-detection is the accepted, auditable failure mode (the rejection record names the matched path, so the audit trail exists either way). Over-detection is not auditable, because a candidate rejected as `duplicate` never becomes a record anyone reviews.

### Decision: `NormalizeSlug` is exported and is the single definition of identity normalization

**Choice**: `NormalizeSlug` lives in `engine/skills/match.go`, is exported, and is cited by the contract document as the normative definition of slug formatting. Rule: lowercase ASCII; every run of characters outside `[a-z0-9]` becomes a single `-`; leading/trailing `-` trimmed; truncated to 48 characters at the last `-` boundary at or before 48.

**Alternatives considered**: keeping normalization as prose in the contract document only. Rejected — cross-session identity stability is the most fragile property in this design, and prose is not testable. Exporting it means the part most likely to drift has a Go table test.

**Rationale**: This is how R-001's identity determinism gets machine-verified despite R-001 being otherwise agent-driven.

### Decision: R-002/R-003 boundary coverage is an executable acceptance checklist, not a Go unit test

**Choice**: The N-1 / N / N+1 boundary and the divergent-recovery case are specified as a decision table in the contract document and verified by a **fixture-driven acceptance procedure executed against real Engram records during `sdd-verify`**, not by Go unit tests. Go unit tests cover `NormalizeSlug` and `MatchCandidate` only.

**Alternatives considered**: a new `engine/procedural` package plus a CLI verb (`procedural observe`) taking the current record markdown on stdin and emitting the updated record plus an emission verdict on stdout. This would make counting, dedupe, the boundary, and rejection fully deterministic and unit-testable while *still* honouring the read-only-Engram constraint (the LLM turn does both the read and the write; Go only transforms text). **Rejected here** because it exceeds the committed Approach 3 scope ("one new small pure read-only Go function in `engine/skills`") and would roughly double the slice. It is recorded as the natural first extension if items 31–35 need machine-determinism at the boundary.

**Rationale, stated plainly**: this is a deliberate, disclosed deviation from the proposal's literal "unit tests over the threshold boundary" wording in the R-002/R-003 success criteria. Agent-driven logic cannot be unit-tested by Go; pretending otherwise would produce a test that asserts the document's text rather than the behaviour. `sdd-tasks` and `sdd-verify` must plan for an acceptance procedure, not a `go test` row, for these two requirements.

## Data Flow

Occurrence path (T1), from a proactive episodic save to a persisted candidate:

    agent turn ──▶ mem_save(episodic)  ──▶ obs #NNNN  (the occurrence anchor)
         │
         ▼
    derive kind + slug (NormalizeSlug grammar)
         │
         ▼
    mem_search("procedural/candidates/{kind}")   ── alias/namespace check (new slugs only)
         │
         ▼
    mem_search("<exact topic key>") ──▶ mem_get_observation(id)   ── direct lookup, not recall
         │
         ├── no record ─────▶ create: Status=observing, count=1, Threshold=3
         └── record found ──▶ append occurrence if id not already listed
                                   │
                                   ▼
                         count < N ──▶ Status stays observing
                         count = N ──▶ skills.MatchCandidate(registry, slug)
                                          ├── matched ──▶ Status=rejected, reason=duplicate,
                                          │                MatchedSkillPath=<path>   (no emission)
                                          └── no match ─▶ Status=emitted  (report once)
                         count > N ──▶ append only; Status unchanged (latch holds)
                                   │
                                   ▼
                         mem_save(topic_key=<exact key>)   ── upsert

Sequence, across a session boundary (R-001's cold-start criterion):

    Session A          Contract doc        Engram (MCP)          engine/skills
       │                    │                   │                     │
       │── occurrence 1 ───▶│                   │                     │
       │                    │── search key ────▶│  (miss)             │
       │                    │── mem_save ──────▶│  count=1, observing │
       │                                                              │
    ══ session boundary ═══════════════════════════════════════════════
       │                                                              │
    Session B               │                   │                     │
       │── occurrence 2 ───▶│                   │                     │
       │                    │── search key ────▶│  (hit, count=1)     │
       │                    │── get_observation▶│  full record        │
       │                    │── mem_save ──────▶│  count=2, observing │
       │                                                              │
       │── occurrence 3 ───▶│                   │                     │
       │                    │── search key ────▶│  (hit, count=2)     │
       │                    │─────── MatchCandidate(registry, slug) ─▶│
       │                    │◀────── (false, "") ─────────────────────│
       │                    │── mem_save ──────▶│  count=3, EMITTED   │
       │◀── report once ────│                   │                     │

`engine/skills` is called as a pure local function and never touches Engram; Engram is reached only through MCP tools inside an LLM turn. There is no arrow from Go to the Engram database anywhere in this design.

## File Changes

| Slice | File | Action | Description |
|---|---|---|---|
| 1 `candidate-store` | `engine/skills/match.go` | Create | `NormalizeSlug` (identity normalization, exported as the single definition) |
| 1 | `engine/skills/match_test.go` | Create | `NormalizeSlug` table tests |
| 1 | `skills/_shared/procedural-candidate-detection.md` | Create | Sections 1–3: record schema, topic-key shapes, occurrence anchoring and dedupe rules (R-001) |
| 2 `repeat-and-recovery-detection` | `skills/_shared/procedural-candidate-detection.md` | Modify | Sections 4–5: emission decision table, `Status` latch, failure-recovery two-segment clustering, T1/T2 trigger contract, acceptance checklist (R-002, R-003) |
| 3 `duplicate-rejection` | `engine/skills/match.go` | Modify | `MatchCandidate` on top of `ParseRegistry`'s `Registry` |
| 3 | `engine/skills/match_test.go` | Modify | `MatchCandidate` table tests including the substring near-miss guard |
| 3 | `skills/_shared/procedural-candidate-detection.md` | Modify | Section 6: rejection record fields and the `MatchCandidate` call site |
| — | `engine/skills/parse.go`, `engine/skills/types.go` | Unchanged | `ParseRegistry` and `Registry` consumed as-is; no schema change |
| — | `skills.registry.yaml`, `skills/<id>/**` | Untouched | No registry row, no manifest row, no `engine skills add` |
| — | `openspec/specs/procedural-candidate-detection/spec.md` | New at archive | Capability spec (written by `sdd-spec`, landed by `sdd-archive`) |

## Interfaces / Contracts

### Go (`engine/skills/match.go`) — pure, read-only, no filesystem, no network

```go
// NormalizeSlug returns the canonical identity form of s: lowercase ASCII,
// every run of characters outside [a-z0-9] collapsed to a single '-', leading
// and trailing '-' trimmed, truncated to at most 48 bytes at the last '-'
// boundary at or before 48. It is the single definition of identity
// normalization for procedural promotion candidates; the contract document
// cites it rather than restating the rule.
func NormalizeSlug(s string) string

// MatchCandidate reports whether a registered skill already covers candidate,
// and returns that skill's path when it does. Matching is exact identity
// match after NormalizeSlug, against each entry's ID, Path, and the base
// segment of Path, in registry order; the first match wins. There is no
// substring, prefix, or similarity matching: a false duplicate silently
// discards knowledge, a miss only defers it. An empty or all-punctuation
// candidate never matches.
func MatchCandidate(reg Registry, candidate string) (matched bool, skillPath string)
```

### Candidate record — Engram observation

`title` and `topic_key` are both the exact topic key. `type: pattern`. `project`: the resolved project. `capture_prompt: false` (automated artifact).

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

### Emission decision table (normative for R-002/R-003)

| Current `Status` | Occurrence id already listed | `OccurrenceCount` after append | `MatchCandidate` | Resulting `Status` | Emitted this turn |
|---|---|---|---|---|---|
| — (no record) | — | 1 | not called | `observing` | No |
| `observing` | yes | unchanged | not called | `observing` | No |
| `observing` | no | < N | not called | `observing` | No |
| `observing` | no | = N | no match | `emitted` | **Yes, once** |
| `observing` | no | = N | match | `rejected` | No |
| `emitted` | no | > N | not called | `emitted` | No |
| `rejected` | no | > N | not called | `rejected` | No |

"Emitted" in this slice means exactly: the record reaches `Status: emitted` and the agent reports it once in that turn's reply. Nothing drafts, approves, or registers a skill — umbrella items 31–35 subscribe by reading `Status: emitted` records.

## Testing Strategy

Strict TDD: every row below is RED first, then GREEN. Focused command `cd engine && go test ./skills/... ./cmd/...`; broad `cd engine && go vet ./... && go test ./...`.

| Slice | Layer | What to test | Approach |
|---|---|---|---|
| 1 | Unit (Go) | `NormalizeSlug`: mixed case, spaces, underscores, punctuation runs, leading/trailing separators, unicode, empty string, all-punctuation input, 48-byte truncation at a `-` boundary, idempotence (`NormalizeSlug(NormalizeSlug(x)) == NormalizeSlug(x)`) | Table test in `engine/skills/match_test.go` |
| 1 | Contract artifact | `skills/_shared/procedural-candidate-detection.md` declares both topic-key shapes and the record field block verbatim | Repo-file assertion test, following the existing precedent in `engine/skills/oo_quality_contract_artifact_test.go` |
| 2 | Contract artifact | The emission decision table and `Threshold: 3` appear verbatim in the contract document | Same precedent |
| 2 | Acceptance (Engram, executed in `sdd-verify`) | N-1 / N / N+1: exactly one emission at N referencing all N occurrence ids, none at N+1, none at a single success. Cross-session: record created in session A reads count 2 in session B with both `engram:<id>` references resolvable via `mem_get_observation`; topic key asserted verbatim. Divergent recovery: three same-failure/different-recovery occurrences produce three sibling keys, each count 1, zero emissions. Same recovery three times produces one key, count 3, `Status: emitted`, `Kind: failure-recovery` | Numbered checklist in the contract document, run against real Engram records; results recorded honestly in the verify report (see the disclosed deviation above) |
| 3 | Unit (Go) | `MatchCandidate`: exact id hit; path hit; `path.Base` hit; case/underscore/space variants of a real id; no match on an unrelated candidate; **no match on a substring near-miss (`sdd-spec-review` must not match `sdd-spec`)**; empty candidate; all-punctuation candidate; empty registry; deterministic first-match when two entries both match | Table test in `engine/skills/match_test.go`, registry built via `ParseRegistry` over a fixture and via a literal `Registry` value |
| 3 | Acceptance (Engram) | A candidate slug equal to a registered skill id yields `Status: rejected`, `RejectionReason: duplicate`, `MatchedSkillPath` set, and no emission; an uncovered candidate emits normally | Checklist rows in the contract document |

No golden files, no network, no subprocess, no temp-directory fixtures are introduced.

## Threat Matrix

| Boundary | Applicability | Reason |
|---|---|---|
| Routing / dispatch | N/A | No router, dispatcher, or verb selection changes. `MatchCandidate` is a leaf function with no caller inside the engine binary in this slice. |
| Shell commands | N/A | No shell string is constructed, parsed, or executed. |
| Subprocess spawn / timeout / orphans | N/A | No process is spawned. The `synctrigger` precedent is explicitly **not** followed, because the hook option was rejected (OQ-2). |
| VCS / PR automation | N/A | No git or `gh` invocation. |
| Executable-file classification | N/A | No file is classified, deployed, or marked executable. `MatchCandidate` performs no filesystem access; `Entry.Path` is compared as a string only. |
| Process integration (hooks, settings.json) | N/A | No hook is installed or removed; `engine/settings/settings.go` is untouched. This is a direct consequence of the OQ-2 decision, not an omission. |

Not applicable overall — no task is generated from this matrix. One adjacent concern is recorded as a risk rather than a threat row: candidate records embed free-text summaries into Engram, and evidence scrubbing (OQ-8) is explicitly out of this slice's scope.

## Migration / Rollout

No data migration. Nothing reads candidate records in this slice, so there is no consumer to upgrade.

Rollout is purely additive and per-slice:
- **Slice 1** ships `NormalizeSlug` and the record/identity sections of the contract document. The detector can create and count records from this point; no emission rule exists yet, so `Status` stays `observing`.
- **Slice 2** ships the emission rule and triggers. First emissions become possible.
- **Slice 3** ships `MatchCandidate` and rejection. Between slice 2 and slice 3, a candidate already covered by a registered skill can emit — accepted, because emission in this slice is a single reported line with no downstream consumer, and slice 3 lands in the same change.

Rollback: delete `engine/skills/match.go` and `engine/skills/match_test.go` (nothing else imports them) and delete `skills/_shared/procedural-candidate-detection.md`. No `settings.json` state, no registry row, no manifest row to unwind. Persisted candidate records are inert data under their own namespace and may be left in place or removed under ordinary memory-lifecycle policy. Per-slice, revert in reverse order: `duplicate-rejection` / `repeat-and-recovery-detection` before `candidate-store`.

Review-budget forecast (informational; `sdd-tasks` owns the guard lines): slice 1 ≈ 250 authored lines, slice 2 ≈ 180, slice 3 ≈ 200. Each is comfortably under the 400-line budget, and the three-slice chain already matches the entry contract.

## Open Questions

- [ ] `skills/_shared/procedural-candidate-detection.md` is **not** added to `gateContractFiles` (`engine/pipkg/pipkg.go:673`), so it is not copied into the Pi package. That is a deliberate omission — packaging it is a Go change outside this slice's scope, and R-001..R-004 do not require it. Revisit when umbrella item 31 needs the contract available inside a packaged runtime.
- [ ] Evidence scrubbing (OQ-8 — secrets, absolute paths, private project names in `Summary` and occurrence one-liners) remains unscoped. Candidate records are ordinary project-scoped observations and inherit whatever hygiene the episodic layer has; no additional guarantee is made here.
- [ ] Whether `failure-recovery` should eventually use a lower threshold than `repeated-success`. Deliberately deferred with the mechanism in place (per-record `Threshold`), to be answered from observed data rather than intuition.
