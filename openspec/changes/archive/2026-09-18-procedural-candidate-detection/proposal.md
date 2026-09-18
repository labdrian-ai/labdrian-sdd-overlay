# Proposal: Procedural promotion-candidate detection and storage

## Intent

The overlay has an episodic memory layer (Engram) and a partially built semantic layer (`longterm-mem` vault), but **no procedural layer**. `skills/` is structurally procedural memory, yet every entry lands there only because a human ran `engine skills add <id>`. Nothing observes that the same approach worked three times, and nothing notices that the same failure was recovered the same way three times. The consequence is concrete: a workflow can be rediscovered from scratch every session, and the repo's dominant learning shape — recovery knowledge, which is most of the memory index — is never captured as a reusable skill at all.

This change is the first slice of the umbrella `procedural-memory-skill-promotion` (roadmap v11 item 30 of 30–35). It builds **detection and storage only**: a durable, cross-session promotion-candidate store, repeated-success and failure-and-recovery detection, and rejection of candidates an existing registered skill already covers. Nothing in this slice drafts a skill, asks for approval, or writes under `skills/`.

Success is a candidate record that survives a session boundary with a correct occurrence count and retrievable evidence references, emitted exactly once at the Nth occurrence, and never emitted for something `skills.registry.yaml` already covers.

Requirements: **R-001..R-004** as restated in the change-scoped requirements brief (Engram `project/labdrian-sdd-overlay/requirements/procedural-candidate-detection`, obs #3401), authoritative source obs #3395.

## Scope

### In Scope

- **R-001 — Durable promotion-candidate store.** A stable Engram topic-key shape per candidate carrying candidate id, occurrence count, first/last observation timestamps, and evidence references for every contributing occurrence. Engram is the durability store; no second SQLite store is introduced (I-09 held).
- **R-002 — Repeated-success detection.** Emit exactly one candidate at the Nth distinct success of the same approach, referencing all N occurrences; no second candidate at N+1; nothing at a single non-repeated success.
- **R-003 — Failure-and-recovery detection.** Emit a candidate labelled `failure-recovery` when the same failure→same recovery pattern recurs N times; emit nothing when N failures had N divergent recoveries.
- **R-004 — Duplicate candidate rejection.** Reject a candidate already covered by a registered skill with reason `duplicate`, recording the matched skill path. Backed by a **new small read-only Go entrypoint in `engine/skills`** (shape along the lines of `MatchCandidate(registry, candidateID/path) (matched bool, skillPath string)`), reusing the already-exported pure `ParseRegistry`, with its own unit tests.
- **Candidate identity contract at sub-step / debugging-pattern granularity** (OQ-1, confirmed by the project owner — see Confirmed Decisions), inherited by umbrella items 31–35.

### Out of Scope

- **Any write under `skills/`.** `engine skills add` is not invoked, and no manifest or registry mutation happens in this slice.
- **R-005..R-014** (skill drafting, approval gate, registration, revision, audit, health report, runtime projection, budget, retirement) — roadmap items 31–35.
- **Any Go-side write path into Engram's database.** See Constraints.
- **A Go-side SessionEnd hook** for detection. Ruled out with the approach decision, not deferred as an option.
- **Skill-schema extension** for trigger/pattern/keyword fields, and semantic-similarity matching that would depend on one.
- **OQ-8 evidence scrubbing** (secrets, absolute paths, private project names) — unscoped in this slice.

## Capabilities

### New Capabilities

- `procedural-candidate-detection`: the promotion-candidate record contract (identity, occurrence count, timestamps, evidence refs, persistence topic-key shape), the repeated-success and failure-and-recovery emission rules including the exactly-once-at-N boundary, the `failure-recovery` label, and duplicate rejection with reason `duplicate` plus the matched skill path — including the read-only `engine/skills` match entrypoint that serves it.

### Modified Capabilities

- None. `skill-lifecycle` governs the `skills add`/`AddCore` write surface; this change adds only a new read-only entrypoint beside it and changes no existing requirement there.

## Approach

**Committed direction: Approach 3 (hybrid)** from `exploration.md`, confirmed by the project owner after point-by-point exploration review (Engram `sdd/procedural-candidate-detection/pipeline-state`, obs #3400).

1. **R-001..R-003 are agent-driven through the existing MCP tools** (`mem_search`, `mem_get_observation`, `mem_save`/`mem_update`). Candidate records are read, counted, and persisted by an LLM turn, exactly as every other Engram write in this repo already happens (episodic saves, session summaries, SDD artifacts). No new Go write path, no new hook.
2. **R-004 gets one new small pure read-only Go function in `engine/skills`**, layered on the already-exported `ParseRegistry`, so duplicate checking is a tested seam rather than ad hoc YAML re-parsing inside agent prose. It is a local pure-function call, so it adds no runtime token or latency cost over a fully agent-driven variant.

**Why not the Go SessionEnd hook (Approach 2):** its premise does not hold against this repo. Approach 1 and Approach 3 are cost-equivalent at runtime, and Approach 2's theoretical token saving does not materialize once persistence still needs an agent-driven MCP call and success/failure semantics still need LLM judgment.

**Delivery** follows the three review slices already recorded in the entry contract (informational here; `sdd-tasks` formalizes them): `candidate-store` (R-001) → `repeat-and-recovery-detection` (R-002+R-003) and `duplicate-rejection` (R-004), both depending on `candidate-store`. Strict TDD is enabled for this change.

## Constraints

- **There is no Go-side write path into Engram's database in this repository.** Confirmed by code read, not assumed: the only Go client, `longterm-mem/internal/engram/store.go`, opens the DB strictly read-only (`mode=ro&_query_only=true`). **No design may assume a Go-side background process can write candidates into Engram without an LLM turn in the loop.** `sdd-design` must not reintroduce that premise.
- **The skill registry schema has no trigger/pattern/keyword field.** `engine/skills/types.go` defines exactly `Entry{ID, Path, Source, Install, Lifecycle}`, and the hand-rolled parser recognizes only that fixed key set. R-004 matching can therefore use `id`/`path` today, or off-schema reads of `SKILL.md` frontmatter — never a schema field that does not exist.
- **No existing observer hook to extend.** The repo has exactly one `SessionEnd` hook (`sync-trigger`) and no `SessionStart`, `Stop`, or `PostToolUse` hook. Whatever detection trigger `sdd-design` picks is net-new surface, not reuse of dormant infrastructure.
- **Engram is the only durability store** for candidates (I-09).

## Confirmed Decisions (do not re-litigate)

| Decision | Value | Source |
|---|---|---|
| OQ-1 — detection granularity | Sub-step / debugging-pattern level (fine-grained), not whole-session or whole-feature | Owner-answered 2026-09-15; obs #3400 |
| Approach | Approach 3 (hybrid): agent-driven R-001..R-003 + read-only Go helper for R-004 | Owner-answered after exploration review; obs #3400 |
| Durability store | Engram, no second store | I-09, obs #3401 |

## Open Questions for `sdd-design`

- **OQ-2 — where the detection loop runs.** Agent-invoked skill, a new hook, or both. **Deliberately left open by this proposal**; exploration established only that there is no existing hook to reuse, so every option is net-new surface. Any answer must respect the "no Go-side Engram write path" constraint above.
- **OQ-6 — what evidence the detector reads.** Engram observations, transcripts, or both. Engram observations are confirmed Go-readable read-only, so this is a real named option rather than a guess.
- **Threshold N.** Whether `failure-recovery` (R-003) uses the same N as repeated success (R-002) is open.
- **R-004 matching method.** Trigger overlap, semantic similarity, or explicit declaration — constrained by the absent schema field noted above.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `engine/skills/` (new file beside `parse.go`, `types.go`) | New | Read-only candidate/registry match entrypoint + unit tests (R-004) |
| `engine/skills/parse.go`, `types.go` | Unchanged (reused) | `ParseRegistry` consumed as-is; no schema change |
| Engram topic keys under a procedural-candidate namespace | New | Durable candidate records (R-001) |
| Agent-side skill/prompt surface (location pending OQ-2) | New | Detection and persistence loop (R-001..R-003) |
| `skills/`, `skills.registry.yaml` | Read-only | Never written by this change |
| `openspec/specs/procedural-candidate-detection/spec.md` | New (at archive) | New capability spec |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| A design silently assumes Go-side Engram writes and becomes unbuildable | Med | Stated as a hard constraint above and carried into `sdd-design`; exploration cites the read-only code path |
| Agent-driven detection never self-triggers reliably (no hard hook forces it) | Med | OQ-2 is design-blocking precisely for this; detection idempotency (no duplicate candidate at N+1) limits damage from irregular firing |
| Cross-session occurrence counting depends on `mem_search` recall, which this repo already flags as imperfect | Med | Stable topic key per candidate makes recovery a direct lookup rather than a recall search; R-001's integration test asserts cross-session recovery |
| R-004 matching on `id`/`path` alone under-detects semantically duplicate candidates | Med | Accepted for this slice; schema extension is explicitly out of scope and the rejection record names the matched path so misses are auditable |
| Candidate identity contract defined here is inherited by umbrella items 31–35 and is expensive to change later | Med | Granularity confirmed (OQ-1) before spec; identity contract is the first review slice and gets its own spec requirement |

## Rollback Plan

- The Go side is additive: delete the new `engine/skills` file and its test; nothing existing is modified, so `engine skills add`/`status`/`list` behavior is untouched.
- The agent side is additive: remove the detection instructions from wherever OQ-2 places them; no hook is installed, so there is no `settings.json` state to unwind.
- Persisted candidate records are inert data under their own Engram topic-key namespace. They are read by nothing else in this slice and can be left in place or removed per memory-lifecycle policy; no other memory layer depends on them.
- Per-slice: revert the slice PRs in reverse order (`duplicate-rejection` / `repeat-and-recovery-detection` before `candidate-store`).

## Dependencies

- Engram MCP tools (`mem_search`, `mem_get_observation`, `mem_save`/`mem_update`) available in the agent runtime — already a hard dependency of the rest of this memory stack.
- `engine/skills.ParseRegistry` (already exported and pure).
- No new external dependency, no new binary, no network access.

## Success Criteria

- [ ] R-001: an integration test shows a candidate observed in session A and again in session B reads occurrence count 2 with both evidence references retrievable, and recovers from Engram with counts intact after a cold start; the topic-key shape is asserted.
- [ ] R-002: unit tests over the threshold boundary at N-1, N, and N+1 show exactly one candidate emitted at N, referencing all N occurrences, and none at N+1 or at a single success.
- [ ] R-003: unit tests distinguish same-recovery from divergent-recovery clusters, and the emitted candidate carries the `failure-recovery` label.
- [ ] R-004: unit tests over covered and uncovered candidates show `duplicate` rejection with the matched skill path recorded, and no rejection when nothing matches.
- [ ] Candidate identity is defined and tested at sub-step / debugging-pattern granularity (OQ-1).
- [ ] No file under `skills/` and no Engram-database write occurs from Go code anywhere in this change.
- [ ] `cd engine && go vet ./... && go test ./...` passes.
