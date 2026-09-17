# Proposal: Knowledge ingestion into long-term memory

## Intent

The memory stack can remember what happened in a session, but it cannot take in
knowledge from outside one. Engram captures episodic observations an agent
wrote about its own work, and `longterm-mem` promotes a subset of those into the
vault. Neither has an intake path for external knowledge — a documentation page,
a specification, a pasted report, a directory of local notes. `longterm-mem`'s
MCP surface is exactly three tools (`query`, `get`, `promote` —
`internal/mcpserver/server.go`), and `promote` takes a single `engram_id` that
must already exist. There is no ingestion command, no batch intake, and no
document parsing anywhere in the repository.

This change is the **first slice of a new umbrella**: long-term memory that is
accessible, scalable, able to reason and create new connections, and able to
ingest new knowledge. It builds **ingestion only**. It is not a continuation of
`procedural-memory-skill-promotion` or `procedural-candidate-detection`, which
are unrelated prior changes about the procedural layer.

Success is that an agent asked to ingest external knowledge follows one
documented, repeatable procedure, and that the resulting memory is
distinguishable from an ordinary session observation, carries its origin, and is
reachable by both `mem_search` and `longterm-mem query`.

Change-scoped requirements **R-001..R-007** are defined by this proposal. Final
spec IDs and `Traces to:` keys are settled by `sdd-spec`; no number in the
existing `longterm-mem` R-NNN space is claimed here.

## Scope

### In Scope

**Work unit 1 — the ingestion convention (R-001..R-004).** No new Go code.

- **R-001 — One documented ingestion procedure.** A durable, agent-discoverable
  repository artifact stating that external knowledge enters long-term memory as
  *agent fetch/read* → `mem_save` → `longterm-mem promote`, and that no other
  intake path exists. The default placement is a new skill under `skills/`
  registered through the existing skill lifecycle (`skills.registry.yaml`,
  `engine skills add`); exact placement is OQ-1.
- **R-002 — Ingested-observation contract.** The fields an ingested observation
  MUST carry: its origin (source URL, local path, or an explicit "pasted by the
  operator" marker), the ingestion timestamp, and a marker that distinguishes
  ingested external knowledge from an ordinary session observation. The
  mechanism for that marker — Engram `type`, `topic_key` namespace, or a
  content-body convention — is OQ-2.
- **R-003 — Promotion is part of ingestion, not a later hope.** The procedure
  SHALL promote what it saves, so ingested knowledge reaches the vault
  deterministically rather than depending on whether a later automatic sync
  finds it eligible. `promote` accepts one observation id per call, so a
  multi-observation ingestion promotes each one.
- **R-004 — Re-ingestion of the same source is recognisable.** Ingesting the
  same source twice SHALL be detectable from the stored observations
  (stable identity derived from the origin), so the second ingestion updates or
  is reported, rather than silently creating a parallel second memory of the
  same document. This rides Engram's `topic_key` upsert and the existing
  `(project, engram_id)` page dedup; it introduces no new dedup store.

**Work unit 2 — local extraction helper (R-005..R-007).** New Go, no network.

- **R-005 — Local, non-network extraction entrypoint** inside `longterm-mem`
  that accepts already-fetched text or a local file path and returns structured,
  right-sized ingestion units. It SHALL NOT import `net` or `net/http`, so
  `net_allowlist_test.go`'s allowlist stays at exactly one entry.
- **R-006 — Deterministic chunking with a stated boundary.** A document larger
  than one ingestion unit SHALL be split at a defined, tested boundary, and each
  chunk SHALL carry its position and its shared source identity so the chunks of
  one document are recoverable as one document. The concrete boundary is OQ-3,
  bounded by the evidence in Constraints.
- **R-007 — Output satisfies R-002.** Every unit the helper emits SHALL carry
  the R-002 field set, so an agent can pass it to `mem_save` without inventing
  metadata.

Work unit 2 depends on work unit 1 and ships as its own review slice. It is
included here rather than deferred because R-002's field contract and R-006's
chunk metadata are the same contract, and splitting them across two changes
would settle that contract twice.

### Out of Scope

- **Approach 2 — Go-side ingestion that bypasses Engram.** Rejected, not
  deferred. See Constraints for the evidence.
- **Any network fetch inside `longterm-mem`.** URL fetching stays with the
  agent's own tools. `net_allowlist_test.go`'s allowlist is not widened by this
  change.
- **Connection inference / relation reasoning over ingested knowledge.**
  `mem_compare`/`mem_judge` machinery is Engram's and belongs to a later
  umbrella slice.
- **Multi-agent concurrency and locking** over ingestion or the vault.
- **Batch promotion.** `promote` stays one observation per call; a batch surface
  is a sync-time concern, orthogonal to intake.
- **Scheduled or autonomous ingestion with no agent turn.** Every ingestion in
  this change is triggered by an agent turn, which is the direct cost of
  rejecting Approach 2.
- **PDF, HTML or binary format parsing.** Work unit 2 accepts text and local
  text files; format decoding is not scoped here.
- **Scrubbing ingested content** for secrets, credentials or private paths.

## Capabilities

### New Capabilities

- `knowledge-ingestion`: the ingestion procedure itself (fetch → `mem_save` →
  `promote`), the ingested-observation field contract (origin, ingestion
  timestamp, ingested-vs-session marker), the promotion step, and re-ingestion
  identity. Work unit 1, R-001..R-004.
- `longterm-mem-ingest-extraction`: the local, non-network extraction/chunking
  entrypoint — its input surface (raw text or local path), its chunk boundary
  and chunk metadata, its zero-egress guarantee, and the fact that its output
  satisfies the ingested-observation contract. Work unit 2, R-005..R-007.

### Modified Capabilities

- None. `longterm-mem-promotion` is consumed exactly as specified (R-032
  explicit promote, R-007 eligibility); no requirement there changes.
  `longterm-mem-memory-access` R-002's read-only Engram connection is untouched,
  because every write goes through Engram's own MCP surface, not Go.
  `skill-lifecycle` is used as-is to register a skill; no lifecycle requirement
  changes.

## Approach

**Approach 1 (Engram-first ingestion), with Approach 3 (local-parsing helper) as
a sequenced second work unit in this same change.** Confirmed by the project
owner after exploration review. Approach 2 is rejected (Constraints).

The decisive point is that **Approach 1 requires no new Go code, because the
flow already exists end to end**. An agent fetches or reads the source with its
own tools; it calls `mem_save` with the extracted content; it calls
`longterm-mem promote --id N` (or the `promote` MCP tool);
`promote.ExplicitPromote` resolves the observation through the read-only
`engram.Store.ObservationByID` seam and `Writer.Promote(obs, true)` emits the
vault page and refreshes the index. `Eligible()` always admits `explicit=true`,
so nothing about eligibility blocks an ingested observation.

What is missing is not capability but **convention**. Today an agent asked to
"remember this document" will improvise: some ad hoc `mem_save`, no origin
recorded, no promotion, no way to tell the resulting memory apart from a note
about a bug fix. This change's real work is to pin that procedure down precisely
enough that ingestion is repeatable and its output is identifiable — which is
what the spec and design phases will nail down from R-001..R-004.

Work unit 2 then removes the one place where the agent is genuinely bad at the
job: splitting a large or structured document into units worth storing. That
belongs in tested Go, not in an LLM turn's judgment, and it needs no network to
do it, so it costs nothing at the egress boundary.

**Delivery** is two review slices — `ingestion-convention` (R-001..R-004, no Go)
then `ingest-extraction` (R-005..R-007, Go plus tests), the second depending on
the first. `sdd-tasks` formalizes the split against the 400-changed-line review
budget; work unit 2 is expected to be the larger of the two and may itself need
splitting. Strict TDD is enabled for this change.

## Constraints

- **Approach 2 — direct Go-side ingestion that bypasses Engram — is rejected.**
  Three independent reasons, all confirmed by source read rather than assumed:
  1. **The egress guard.** `longterm-mem/net_allowlist_test.go` (package
     `guard`) statically AST-parses every non-test `.go` file in the module and
     fails if any file other than `internal/embed/client.go` imports `net/http`
     or `net` (R-071). A second test asserts the allowlist holds exactly one
     entry. Any Go-side URL fetch requires deliberately widening it, which that
     test's own doc comment calls "a change to the egress boundary, not a
     convenience."
  2. **Invisibility to Engram.** Engram has no read path into vault pages, so
     content ingested outside Engram is unreachable by `mem_search`,
     `mem_compare` and `mem_judge`. That machinery is Engram's, not
     `longterm-mem`'s, and it is precisely the machinery the umbrella's "reason
     and create new connections" goal depends on. Approach 2 would ingest into
     the one place the reasoning layer cannot see.
  3. **No synthetic-identity design exists.** `findPromotedPage`/`Allocate`
     (`internal/promote/address.go`) key re-promotion on `(project, engram_id)`
     read from page frontmatter, and `EngramID`, the `engram_id` frontmatter
     key and the "Promoted from Engram observation N" footer all assume real
     Engram provenance throughout `internal/promote/*`. A non-Engram observation
     has no non-colliding id space today (Engram autoincrements from 1).

  A future slice may revisit direct ingestion if a concrete case appears for
  content that deliberately must NOT enter Engram's reasoning layer. That would
  be an argued decision with its own proposal, not a default.

- **`longterm-mem` cannot write to Engram.** `internal/engram/store.go` opens
  the database strictly read-only (`mode=ro&_query_only=true`). Every write in
  this change happens through Engram's own MCP tools in an agent turn. `sdd-design`
  must not reintroduce a Go-side Engram write.

- **Automatic promotion eligibility reacts to `topic_key`.**
  `longterm-mem-promotion` R-007 makes an observation automatically eligible
  when its `topic_key`'s first path segment is non-empty and not `sdd`, `review`
  or `delivery`. Any ingestion topic-key namespace this change picks therefore
  changes what a plain `sync` promotes, whether or not that was intended. This
  constrains OQ-4 and must be decided knowingly.

- **Query responses are budgeted.** `internal/query/query.go` caps a response at
  `ResponseTokenCeiling = 2000` tokens (`bytesPerToken = 4`, so ~8000 bytes)
  across all results, with `MinSnippetBudget = 120` bytes per row and a default
  top-N of 5. An ingestion unit far larger than its share of that budget is
  always returned truncated, recoverable only through `get`. This is the real
  evidence the chunk boundary (OQ-3) must be argued against — not a guessed
  token count.

- **`promote` is one observation per call** (`PromoteIn{Project, EngramID}`).
  A chunked document is N promote calls, and a partially promoted document is a
  reachable state design must account for.

- **No fetch or document-parsing code exists anywhere in the repository** to
  reuse. Work unit 2's extraction is net-new surface, though it adds no
  dependency on the network.

## Confirmed Decisions (do not re-litigate)

| Decision | Value | Source |
|---|---|---|
| Approach | Approach 1 (Engram-first), with Approach 3 as a sequenced second work unit | Owner-answered after exploration review, 2026-09-16 |
| Approach 2 | Rejected on R-071, Engram invisibility, and absent synthetic-id design | Owner-answered; evidence in `exploration.md` |
| Slice boundary | Ingestion only; connection inference and multi-agent concurrency are separate future umbrella items | Owner-stated scope for this slice |
| Egress boundary | `net_allowlist_test.go`'s allowlist is not widened by this change | Follows from the approach decision |

## Open Questions for `sdd-design`

- **OQ-1 — where the convention lives.** A new standalone skill under `skills/`
  registered in `skills.registry.yaml`, or a section added to an existing skill.
  The repository has no root `CLAUDE.md`, so "add it to the project instructions"
  is not an available third option; the artifact must be a repository file that
  the skill lifecycle propagates. Default assumption is a new skill, but the
  choice is not settled.
- **OQ-2 — how an ingested observation is marked.** Whether the
  ingested-vs-session distinction needs a new Engram `type`, or is carried by a
  `topic_key` convention plus content-body fields. **Genuinely undecided**: the
  documented Engram type set is `decision | architecture | bugfix | pattern |
  config | discovery | learning | manual | preference`, none of which means
  "external knowledge", and whether Engram accepts a type outside that set is
  not established by anything read during exploration. Design must verify before
  choosing.
- **OQ-3 — the chunk boundary for R-006.** Semantic (heading/section), size, or
  hybrid, and what size. Must be argued against `ResponseTokenCeiling = 2000`
  tokens, `MinSnippetBudget = 120` bytes and default top-N 5 (see Constraints),
  and against whatever the embedding arm's own input limit turns out to be —
  which exploration did not establish. If design cannot ground a concrete
  boundary in evidence, it should say so and defer work unit 2 rather than guess
  a number.
- **OQ-4 — ingestion topic-key namespace.** A dedicated namespace (e.g.
  `ingested/{source}/...`) versus ordinary project-scoped topic keys. Interacts
  directly with promotion eligibility R-007 (see Constraints) and with R-004's
  re-ingestion identity, since `topic_key` is Engram's upsert key.
- **OQ-5 — what R-004 does on re-ingestion of changed content.** Upsert the same
  observation (losing the prior version), or write a new one and supersede the
  old. Engram's supersession machinery exists, and the vault's own supersession
  propagation is specified in `longterm-mem-promotion` R-033, so both are real
  options rather than a guess.
- **OQ-6 — partial-ingestion failure semantics.** What state an N-chunk document
  is left in when chunk K's `mem_save` or `promote` fails, and whether the
  procedure must be resumable. Reachable because promotion is per-observation.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `skills/` (new skill directory, placement pending OQ-1) | New | The ingestion convention artifact (R-001..R-004) |
| `skills.registry.yaml` | Modified | One registry entry for the new skill |
| `longterm-mem/internal/` (new package, name pending design) | New | Local extraction/chunking helper plus tests (R-005..R-007) |
| `longterm-mem/net_allowlist_test.go` | Unchanged (asserted) | Allowlist stays at one entry; work unit 2 must keep this test green unmodified |
| `longterm-mem/internal/promote/*` | Unchanged (consumed) | `ExplicitPromote`/`Writer.Promote` used exactly as specified |
| `longterm-mem/internal/engram/store.go` | Unchanged (read-only) | No Go-side Engram write is introduced |
| `longterm-mem/internal/mcpserver/server.go` | Unchanged | No `ingest` tool; `query`/`get`/`promote` stay the whole surface |
| Engram topic keys under an ingestion namespace (pending OQ-4) | New | Where ingested observations live |
| `openspec/specs/knowledge-ingestion/spec.md`, `openspec/specs/longterm-mem-ingest-extraction/spec.md` | New (at archive) | The two new capability specs |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| A convention with no enforcement is ignored, and agents keep improvising ad hoc saves | High | R-002's field contract makes a non-conforming ingestion identifiable after the fact; work unit 2's helper makes the conforming path the easier path |
| OQ-2 resolves to "a new Engram type" and Engram rejects unknown types | Med | Called out as genuinely undecided; design must verify against Engram before committing, with the topic-key/content-convention route as the fallback |
| An ingestion topic-key namespace silently widens what `sync` auto-promotes (R-007) | Med | Stated as a hard constraint; OQ-4 must be decided with that consequence in view, and the spec should assert the resulting eligibility behavior |
| Chunk boundary is guessed rather than grounded, producing units that are always truncated in query results | Med | OQ-3 names the concrete budget constants to argue against, and explicitly permits deferring work unit 2 over guessing |
| Large documents inflate the vault and degrade `longterm-mem query` signal | Med | Chunking with stated boundaries is the mitigation; ingestion is explicit and agent-triggered, never automatic, so volume stays operator-controlled |
| Every ingestion needs an agent turn — no batch or scheduled intake | Accepted | This is the direct, knowing cost of rejecting Approach 2; revisitable only through a new argued proposal |
| Work unit 2 exceeds the 400-line review budget on its own | Med | It is already a separate slice; `sdd-tasks` forecasts and may split it further |

## Rollback Plan

- **Work unit 1** is a documentation/convention artifact: delete the skill
  directory and its `skills.registry.yaml` entry, then re-propagate the registry.
  No code path changes, so `longterm-mem` behavior is unaffected by the revert.
- **Work unit 2** is additive Go in a new package with no caller inside existing
  flows: delete the package and its tests. `promote`, `sync`, `query` and the
  MCP surface are untouched by construction, and `net_allowlist_test.go` is
  unchanged either way.
- **Already-ingested observations and their vault pages are inert data.** They
  are ordinary Engram observations and ordinary promoted pages; nothing else in
  this change reads them. They may be left in place, or retired through the
  normal memory-lifecycle and vault supersession paths — never by hand-deleting
  vault pages, which the local-edit precedence rules are built to notice.
- Revert the slice PRs in reverse order: `ingest-extraction` before
  `ingestion-convention`.

## Dependencies

- Engram MCP tools (`mem_save`, `mem_search`, `mem_get_observation`) available
  in the agent runtime — already a hard dependency of this memory stack.
- `longterm-mem`'s existing explicit `promote` path (CLI and MCP tool).
- An agent-side fetch/read capability (WebFetch, Read, or equivalent) for any
  source that is not already local. This change deliberately does not provide one.
- The existing skill lifecycle (`skills.registry.yaml`, `engine skills add`) for
  work unit 1's placement.
- No new external dependency, no new binary, and no new network access.

## Success Criteria

- [ ] R-001: the ingestion procedure exists as a durable repository artifact,
      is registered/propagated through the existing skill lifecycle, and names
      fetch → `mem_save` → `promote` as the only intake path.
- [ ] R-002: the ingested-observation field contract is specified and testable —
      given an ingested observation, origin, ingestion timestamp and the
      ingested-vs-session marker are all recoverable from it.
- [ ] R-003: an end-to-end walkthrough shows an external source ingested and its
      observation promoted to a vault page, reachable by both `mem_search` and
      `longterm-mem query`.
- [ ] R-004: re-ingesting the same source is shown not to produce a second
      independent memory of it; the resolved behavior (OQ-5) is asserted.
- [ ] R-005: the extraction entrypoint accepts raw text and a local file path,
      and `go test ./...` in `longterm-mem` — including
      `TestNetImportAllowlist` and `TestNetImportAllowlistStillRefusesOthers`
      unmodified — passes.
- [ ] R-006: unit tests cover the chunk boundary at and around its threshold,
      and show that the chunks of one document carry a shared source identity
      and their position.
- [ ] R-007: every emitted unit carries the R-002 field set, asserted in test.
- [ ] `net_allowlist_test.go`'s allowlist still holds exactly one entry, and no
      Go file in this change imports `net` or `net/http`.
- [ ] No Go-side write to Engram's database is introduced anywhere in this change.
- [ ] `cd longterm-mem && go vet ./... && go test ./...` passes.
