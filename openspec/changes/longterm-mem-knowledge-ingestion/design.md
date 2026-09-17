# Design: Knowledge ingestion into long-term memory

## Technical Approach

Approach 1 (Engram-first), with Approach 3 as a sequenced second work unit, exactly as
the proposal committed. Approach 2 is rejected and appears nowhere below, not even as a
fallback.

The design's load-bearing idea is that **ingestion identity is a function of the origin,
never of the content**. A source URL or local path deterministically resolves to one
`Source-Id`, which resolves to one topic-key namespace, which is Engram's own upsert key
and `findPromotedPage`'s route to the same `(project, engram_id)` vault page. Re-ingesting
the same document therefore lands on the same observation and the same page by
construction — R-004 falls out of the identity rule rather than needing a dedup store,
which the proposal explicitly forbids introducing.

The second load-bearing idea is that **the ingested-vs-session marker must not depend on
anything this design cannot verify**. The Engram `type` value is deliberately
non-load-bearing (see OQ-2), because `type` is unverifiable from here, is forbidden as a
promotion-eligibility signal by `longterm-mem-promotion` R-007, and is copied verbatim into
vault page frontmatter (`internal/promote/page.go:62` → `engram_type`), so an unknown value
would leak into the vault. The marker is carried three redundant ways that are all
verifiable: the topic-key namespace, a title prefix, and a fixed provenance trailer inside
`content`.

Two work units, both sliced for review below:

- **WU-1 `ingestion-convention` (R-001..R-004)** — no Go in `longterm-mem`. A new skill
  `skills/knowledge-ingestion/` plus a normative contract at
  `skills/_shared/ingested-observation-contract.md`, registered through the existing skill
  lifecycle, with one repo-file assertion test in `engine/skills/`.
- **WU-2 `ingest-extraction` (R-005..R-007)** — a new `longterm-mem/internal/ingest`
  package: local, zero-network extraction and chunking that emits records ready to hand to
  `mem_save` unchanged.

## Architecture Decisions

### Decision: OQ-1 — a new standalone skill, plus the normative contract in `skills/_shared/`

**Choice**: two artifacts, one registry row.

| Path | Role | Registry |
|---|---|---|
| `skills/knowledge-ingestion/SKILL.md` | The procedure: activation contract, the fetch → `mem_save` → `promote` steps, decision gates, failure handling | One row in `skills.registry.yaml` |
| `skills/_shared/ingested-observation-contract.md` | The normative R-002 field block, topic-key grammar, re-ingestion decision table, failure-state vocabulary | **None** — `_shared/` is infra |

`skills/_shared/` takes no registry row and produces no manifest skill row:
`engine/skills/manifest.go:13` lists `_shared/` in `infraPrefixes`, and
`isInfraDir` excludes those rows from `ManifestView`. The precedent is direct —
`skills/_shared/oo-quality-contract.md`, `engram-convention.md`,
`openspec-convention.md`, and this session's `procedural-candidate-detection.md` all live
there with no registry entry.

The split is not decoration. The contract document is cited by **two** consumers that must
not drift: the agent procedure in WU-1 and the Go emitter in WU-2 (`Header.Render`'s doc
comment cites it as normative). A contract owned by one skill's SKILL.md and re-stated in a
Go comment is a contract with two definitions.

Registry row shape (matching every other `custom` entry, e.g. `anti-generic-design`):

```yaml
  - id: knowledge-ingestion
    path: knowledge-ingestion
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
        - opencode
        - codex
        - pi
    lifecycle:
      updateStrategy: overlay-only
```

**Alternatives considered**:
- *A section inside an existing skill.* Rejected: no existing skill owns memory intake. The
  nearest candidates are the `sdd-*` family (wrong lifecycle — `vendor-merge` from upstream,
  so an overlay-owned convention would be a merge hazard on every sync) and `skill-registry`
  (a different domain entirely). A buried section also loses discoverability: the contextual
  skill-loading rule matches on a skill's own `description:` trigger line, which a section
  inside another skill does not have.
- *Root `CLAUDE.md`.* Closed by the proposal — the repository has none. It would also be
  Claude-only, while a registry row propagates to `claude`, `opencode`, `codex`, and `pi`.
- *Contract inside `SKILL.md` only, no `_shared/` file.* Rejected: WU-2's Go doc comment and
  WU-1's procedure would each hold a copy of the field block, and the artifact test would
  assert a file whose primary content is prose about when to load a skill.

**Rationale**: it is the placement the proposal already assumed (`R-001`'s "registered
through the existing skill lifecycle"), it makes the contract independently testable, and
it costs exactly one registry row.

### Decision: OQ-2 — the marker is the topic-key namespace, a title prefix, and a provenance trailer. The Engram `type` is deliberately non-load-bearing.

**Choice**: `type: "discovery"` — an in-set value that carries **no** ingestion semantics.
The distinction is carried by three redundant, verifiable markers:

| Marker | Where it survives | Machine-readable by |
|---|---|---|
| Topic key first segment `ingested/` | Engram `observations.topic_key` | `engram.Observation.TopicKey` (Go, read-only), `mem_search` on the key |
| Title prefix `Ingested: ` | Engram title; vault page `title` frontmatter | `mem_search`, vault FTS, `query.ResultRow.Title` |
| Provenance trailer inside `content` | Engram content; **and the vault page body**, since `EmitPage` writes the observation body | `mem_get_observation`, `longterm-mem get`, any reader of the page |

Only the third survives into the vault page body, which is why the trailer — not the topic
key — is the authoritative record of origin.

**Why not a new Engram `type`.** Three reasons, in decreasing order of strength:

1. **It is unverifiable from this phase.** This agent has no shell and cannot probe Engram.
   The MCP `mem_save` schema text available here describes `type` as
   `"Category: decision, architecture, bugfix, pattern, config, discovery, learning
   (default: manual)"` — a prose description with a default, not a declared closed enum —
   but whether the server *validates* it is exactly what the proposal flagged as
   unestablished, and a prose description is not evidence of acceptance. Designing the
   identity contract on an unverified write is designing it on a guess.
2. **`type` is forbidden as a promotion signal.** `longterm-mem-promotion` R-007 states
   plainly: "An observation's `type` value and its `revision_count` SHALL NOT be used as
   automatic eligibility criteria", and `Eligible` (`internal/promote/eligible.go:25`) reads
   only `explicit`, `Pinned`, and `TopicKey`. A marker in `type` would therefore be invisible
   to the one Go predicate that decides what reaches the vault.
3. **An unknown `type` leaks into the vault.** `page.go:62` sets `EngramType: obs.Type`, and
   `frontmatter.go:148/189` writes it as the `engram_type` field of every promoted page
   (`testdata/pages/architecture.golden.md:18`). An out-of-set value would become permanent
   frontmatter in git-tracked vault pages before anyone confirmed Engram accepts it.

`discovery` is chosen over `manual` as a positive in-set choice rather than the schema
default, and over `architecture`/`decision` because an ingested document records neither.

**Named extension point**: because no marker depends on `type`, adding a dedicated
`ingested` type later is a *compatible* extension — it would buy one real thing
(`query.Request.ExcludeTypes` could then filter ingested rows out of a query, which today it
cannot do for ingested content specifically) and would invalidate nothing already stored.
That upgrade requires an empirical check that Engram accepts the value, which this phase
cannot perform. It is recorded as an open question, not silently taken.

**Alternatives considered**: a topic-key-only marker (rejected — the topic key does not
reach the vault page body, so a promoted ingested page would carry no origin a reader can
see); a content marker only (rejected — not addressable, so a namespace listing of every
ingested source becomes a full-text search); Engram `tags` (no tag field exists on
`engram.Observation` — `store.go:38-51` is `ID, SyncID, Type, Title, Content, Project,
RevisionCount, Pinned, CreatedAt, UpdatedAt, DeletedAt, TopicKey`).

### Decision: the trailer goes **after** the body, not before it

**Choice**: record content is `body` → `---` → provenance block. The manifest record is
`summary` → inventory → `---` → provenance block.

**Rationale, measured against real constants**: `vecindex.EmbedInput(title, content,
inputLimit)` embeds `title + "\x00" + content[:inputLimit]`, and
`vecindex.DefaultInputLimit = 2000` (`internal/vecindex/index.go:27`, the default behind
`--embed-input-limit`). Content past that offset is **never embedded** — `fingerprint.go`'s
own comment and `TestFingerprintIgnoresContentBeyondInputLimit` state it: an edit past the
limit does not even change the fingerprint. The provenance block is ~440 bytes, of which
160 bytes are two 64-hex digests, i.e. high-entropy noise. Putting it first would spend 22%
of every chunk's embedding window on metadata that is near-identical across sibling chunks
of the same document — inflating their mutual similarity and displacing real text. Putting
it last spends the truncation on the digests instead, which is where the loss belongs.

(Note for implementers: `EmbedInput` slices with `len()`, so `inputLimit` is **bytes**
despite the doc comment saying "characters". The chunk bounds below are stated in bytes
accordingly. This is pre-existing behaviour and is not changed here.)

### Decision: OQ-3 — chunk bounds are 1600 bytes max, 480 bytes min, derived from three independent measured constants

**Choice**: `DefaultMaxChunkBytes = 1600`, `DefaultMinChunkBytes = 480`, both overridable
per call. WU-2 is **not** deferred; the number is grounded, not guessed.

| Bound | Value | Derivation |
|---|---|---|
| Max | **1600 bytes** | `query.ResponseByteCeiling` (8000 = `ResponseTokenCeiling` 2000 × `bytesPerToken` 4) ÷ `DefaultTopN` (5) = 1600. One chunk is worth at most one row's share of one whole query response — so an agent can `get` a full top-5 result set and still land inside the same budget one query answer is allowed. |
| Max, cross-check | **1600 ≤ 2000 − headroom** | `vecindex.DefaultInputLimit` = 2000 bytes. Body (≤1600) + separator + title fit inside the embedding window, so **no part of a chunk's text is semantically invisible**. The ~440-byte trailer is what gets truncated out of the window, by design (previous decision). |
| Min | **480 bytes** | `engram.SnippetBudget` = 480, cited in `query.go:521` as "derived from 2 sources × 5 rows" and used as the hard cap on any rendered snippet. A chunk at the floor is therefore *at most one snippet long* — the smallest unit that a query row can still show essentially whole. |
| Min, cross-check | **480 = 4 × 120** | `query.MinSnippetBudget` = 120 is documented as the point below which "a snippet cannot carry the sentence a match sits in". Four times that floor is the smallest chunk that survives snippet rendering with context. |

Two independent constants — the response budget and the embedding input limit — converge on
~1600, which is the strongest form this evidence takes. The proposal's off-ramp (defer WU-2
rather than guess) is therefore **not** taken.

**What this does not claim**: a 1600-byte chunk is still truncated in a query *response*.
`allocateSnippetBudget` caps any rendered snippet at `engram.SnippetBudget` = 480 regardless
of budget headroom, so nothing above 480 bytes is ever returned whole by `query`. That is
unavoidable and is not a chunking failure; the chunk bound instead guarantees that the
`get` needed to recover the rest costs ~400 tokens rather than an unbounded amount, and that
`SnippetTruncated`/`FullLength` tell the caller honestly that there is more.

**Boundary algorithm (deterministic, hybrid semantic-then-size)**:

1. Normalize CRLF → LF. No other byte is altered; the body is stored verbatim.
2. Segment into **blocks**: an ATX heading (`#`..`######`) starts a new block; a fenced code
   block (` ``` ` or `~~~`) is atomic; a contiguous run of `|`-leading lines (table) is
   atomic; otherwise a block is a blank-line-delimited paragraph.
3. Accumulate blocks greedily while the running size ≤ `MaxChunkBytes`. A chunk is not
   closed below `MinChunkBytes` unless the input is exhausted or the next block cannot fit.
4. A single block larger than `MaxChunkBytes` is force-split, in this order: last sentence
   boundary (`.`/`!`/`?` followed by whitespace) at or before max → last `\n` at or before
   max → hard cut at max, moved back to the nearest UTF-8 rune boundary. The resulting pieces
   record `**Split**: forced-sentence | forced-line | forced-byte`; every other chunk records
   `**Split**: none`.
5. Each chunk records the heading breadcrumb in effect at its first byte as `Chunk-Path`.

**Alternatives considered**: pure size chunking (rejected — cuts mid-table and mid-code
block, producing units that are individually unreadable and therefore unjudgeable in a
search result); pure semantic chunking at headings (rejected — a document with one heading
and 40 KB of prose produces one unit that violates every bound above); a token-based bound
(rejected — this module has no tokenizer, and `query.go:205-213` explicitly refuses to
invent a precise-looking one, stating its ceiling in the same 4-bytes-per-token units the
evidence was measured in; a chunker inventing a second, finer estimate would contradict that).

### Decision: OQ-4 — namespace `ingested/`, which knowingly makes every ingested observation `sync`-eligible

**Choice**:

```
ingested/{source-kind}/{source-id}             # manifest, or the whole record when N == 1
ingested/{source-kind}/{source-id}/c0001       # chunk 1 of N, when N > 1
ingested/{source-kind}/{source-id}/c0002       # ...
```

`{source-kind}` ∈ `url | file | directory | pasted`. Chunk ordinals are zero-padded to four
digits so a namespace listing sorts lexically; a source exceeding 9999 chunks (≈16 MB at the
max bound) is refused with a named error rather than silently renumbered.

**The R-007 consequence, stated exactly**: `curatedTopicKey` (`eligible.go:38`) splits on the
first `/`; the head is `ingested`, which is non-empty and is not `sdd`, `review`, or
`delivery`. **Every ingested observation is therefore automatically eligible for `sync`
promotion**, not only for the explicit `promote` R-003 requires. This is a real behavioural
change to what a plain `sync` promotes and it is accepted deliberately, for three reasons:

1. It is **convergent, not additive**. R-003 already promotes every ingested observation
   explicitly at ingestion time. A later `sync` finds the page already written at the current
   revision and does nothing — `longterm-mem-promotion` R-030's own scenario, "Unchanged
   eligible observation is a no-op".
2. It is the **repair path for OQ-6's partial state**. When an ingestion dies between
   `mem_save` and `promote`, auto-eligibility means the next `sync` finishes the job instead
   of leaving an orphaned observation. Choosing an excluded prefix would forfeit that.
3. Ingestion is **explicit and operator-triggered** (proposal, Out of Scope: no scheduled or
   autonomous intake), so the volume entering auto-eligibility is bounded by deliberate acts,
   not by background accumulation.

The spec MUST assert this behaviour rather than leave it implied: an `ingested/`-keyed
observation that was never explicitly promoted is promoted by the next `sync`.

**Alternatives considered**:
- *Ordinary project-scoped keys with the marker elsewhere.* Rejected: it forfeits namespace
  listing (`mem_search "ingested/url/"` as the inventory of every ingested URL), and it
  forfeits the deterministic upsert address that R-004 is built on. It also does **not**
  avoid the eligibility question — any non-excluded first segment is auto-eligible, so the
  only way to opt out would be to key ingested content under `sdd/`, `review/`, or
  `delivery/`, which would be an outright lie about what the observation is.
- *Content-addressed chunk keys* (`.../sha256-prefix`). Rejected in the re-ingestion decision
  below.
- *A separate `ingested-manifest/` namespace for manifests.* Rejected: it doubles the
  namespaces to list and breaks the parent/child relationship that makes
  `mem_search "ingested/url/{source-id}"` return a document and all of its parts together.

### Decision: `Source-Id` is a function of the origin, never of the content

**Choice**: `Source-Id = NormalizeSlug(<human part>)[:40] + "-" + sha256(<canonical origin>)[:8]`

| Kind | Canonical origin | Human part |
|---|---|---|
| `url` | lowercase scheme+host, default port stripped, fragment stripped, query kept, trailing `/` stripped unless the path is `/` | `host + path` |
| `file` | cleaned absolute path, or the caller-supplied `Origin.URI` override when present | base name without extension |
| `directory` | as `file`, for each contained file | as `file` |
| `pasted` | `pasted:` + the operator-supplied label (required) | the label |

The 8-hex suffix is collision insurance for two origins whose human parts slug identically;
the slug keeps the key readable.

**The rule that makes OQ-5 work**: the id never reads the content. If it did, an edited
document would resolve to a new topic key, upsert would not fire, and every re-ingestion
would fork a second memory of the same source — precisely what R-004 forbids.

**Known limitation, disclosed**: a `file` origin's canonical form is an absolute path, so
the same document ingested on two machines gets two ids. `Origin.URI` exists as the override
(e.g. `repo://openspec/specs/x.md`) for content that should have a machine-independent
identity. This is the correct place for that decision — the operator knows whether a path is
incidental — and it is a documented step in the skill, not a silent default.

### Decision: OQ-5 — content-hash-gated upsert; supersession is reserved for a changed *origin* and stays operator-driven

**Choice**: re-ingestion resolves to the same topic key and upserts. Never a second parallel
memory; never an automatic supersession.

| Prior state at the key | `Source-SHA256` vs stored | Action |
|---|---|---|
| none | — | Create every record, promote each, manifest `Status: complete` |
| exists | **equal** | **No-op.** No `mem_save`, no `promote`. Report "unchanged". |
| exists | different | Upsert every chunk whose `Content-SHA256` changed (same key → same `engram_id` → `findPromotedPage`'s `(project, engram_id)` resolves the same page, which is updated in place); leave byte-identical chunks untouched; upsert the manifest with the new digest and inventory |
| exists, new N < old N | — | Surplus keys `c{N+1}…` upsert to a retired stub (`**Status**: retired`, trailer preserved, body removed) and are promoted so the vault page is updated in place. **Never hand-deleted** — the rollback plan and the local-edit precedence rules both depend on pages not vanishing. |
| exists, new N > old N | — | New chunk keys created and promoted |
| origin changed (new URI ⇒ new `Source-Id`) | — | A different topic key. The old record is **not** touched automatically; `mem_compare(new, old, "supersedes")` is a documented operator step, after which `longterm-mem-promotion` R-033 propagates `status: superseded` and the related-link to the old page on the next `sync`. |

**Why the unchanged case is a hard no-op rather than an idempotent re-save**: an upsert
increments `revision_count` and rewrites the page, which makes a re-ingestion of unchanged
content indistinguishable from a real update to anything reading revisions — including
`sync`'s own re-promotion trigger. A no-op also makes the whole procedure cheaply resumable
(OQ-6): re-running a failed ingestion costs nothing for the parts that already landed.

**Why chunk identity is positional, not content-addressed**: inserting a paragraph early in a
document shifts every later boundary, so positional identity rewrites most chunks. The cost
is real and accepted, because the alternative is worse: content-addressed chunk keys would
orphan and re-create a vault page for every shifted chunk on every edit, breaking the
`(project, engram_id)` page-reuse path and growing the vault monotonically with edit history.
Stable keys with churning bodies beat churning keys with stable bodies when the pages are
git-tracked.

**Alternatives considered**: supersede-on-every-change (rejected — a document edited weekly
would accumulate 52 superseded observations and 52 vault pages a year, and `mem_compare` is
explicitly out of this slice's scope); reject-as-duplicate (rejected — it makes the memory
permanently stale and gives the operator no path forward short of manual deletion).

### Decision: OQ-6 — skip-and-report inside the extractor; manifest-first, resumable, never-claim-success at the agent layer

**Choice**: two different contracts, because the two layers fail differently.

**WU-2 (Go).** `Extract` returns a hard `error` **only** when the request itself cannot be
resolved: `Text` and `Path` both set or both empty, `Path` does not exist or is neither file
nor directory, `Origin.Kind` invalid, `pasted` without a label, or a source exceeding the
9999-chunk cap. Every **per-item** failure is a `Skip` record with a stable code, and the
call still succeeds:

| `Skip.Code` | Meaning |
|---|---|
| `unreadable` | `os.ReadFile` failed (permissions, I/O) |
| `binary` | NUL byte within the first 8 KiB |
| `unsupported_extension` | not in the text allowlist (`.md`, `.markdown`, `.txt`, `.text`, `.rst`, `.adoc`) |
| `too_large` | above `MaxSourceBytes` (default 4 MiB) |
| `empty` | zero bytes, or whitespace only |
| `symlink` | entry is a symlink; never followed |
| `outside_root` | resolved path escapes the scan root |

Zero units plus a non-empty `Skipped` list is a **success with zero sources**, not an error —
the caller must be able to report "ten files, ten skipped" honestly. `Result.Skipped` is
always rendered, never `omitempty`, following the rule `query.Result.Coverage` already states
for itself: an absent field and "nothing was skipped" must never look alike.

**WU-1 (agent).** Manifest-first, so a crash is always readable:

1. `Extract` → records.
2. Save the manifest with `Status: pending`, `Chunks-Expected: N`, `Chunks-Saved: 0`.
3. Promote the manifest.
4. For each chunk in order: `mem_save`, then `promote`.
5. On success for all N: upsert the manifest to `Status: complete`, `Chunks-Saved: N`.
6. On any failure at chunk K: stop that source, upsert the manifest to `Status: partial`
   listing exactly which chunk keys are saved and promoted and which are missing, and report
   `ingested K of N` plus the skip list. **Never report success for a partially ingested
   document.**
7. **Resume is simply re-running the same ingestion.** The OQ-5 hash gate makes every
   already-landed chunk a no-op, so the second run does only the missing work. No separate
   resume command exists, and none is needed.
8. A `promote` refusal from the local-edit precedence rule (`longterm-mem-promotion`'s
   registration-conflict exit, R-032's neighbouring refusal requirement) is reported as a
   **refusal**, not as a retryable failure — retrying refuses identically, and the operator
   has to reconcile the hand-edited page.

**Alternatives considered**: all-or-nothing with rollback (rejected — there is no transaction
across N `mem_save` and N `promote` calls, and "rollback" would mean deleting vault pages,
which the rollback plan forbids); silent partial success (rejected outright — a document
that is 4 chunks of 12 and *looks* complete is worse than no ingestion at all, because a
later reader has no way to know the rest exists).

## Data Flow

Ingestion of one source, end to end. Every arrow into Engram is an MCP call inside an agent
turn; there is no arrow from Go to Engram's database anywhere in this design.

    operator asks to ingest a source
             │
             ▼
    agent fetch/read  (WebFetch | Read)          ← the ONLY network step, agent-owned
             │  raw text
             ▼
    longterm-mem internal/ingest.Extract          ← local only; imports no net, no os/exec
             │  Result{ Sources[]{ Records[] }, Skipped[] }
             ▼
    mem_search("ingested/{kind}/{source-id}") ──▶ mem_get_observation ──▶ stored Source-SHA256
             │
             ├── digests equal ──▶ no-op, report "unchanged"            (OQ-5)
             │
             └── differ or absent
                     │
                     ▼
             mem_save(manifest, Status: pending) ──▶ promote(id)
                     │
                     ▼
             for c0001..cNNNN:  mem_save(chunk) ──▶ promote(id)
                     │                   │
                     │                   └── failure ──▶ manifest Status: partial, report K of N
                     ▼
             mem_save(manifest, Status: complete)

    ┌──────────────────────────────────────────────────────────────────┐
    │ reachability after ingestion                                     │
    │   mem_search / mem_get_observation   ← Engram observation        │
    │   longterm-mem query --sources engram-fts,engram-embed,vault     │
    │   longterm-mem get                   ← whole chunk, ~400 tokens  │
    │   wiki/memory/*.md                   ← promoted page, body holds │
    │                                        the provenance trailer    │
    └──────────────────────────────────────────────────────────────────┘

Sizing, against the constants each bound came from:

    source document (any size)
      └─ chunk       ≤ 1600 B  = ResponseByteCeiling(8000) / DefaultTopN(5)
           ├─ body   ≤ 1600 B  ⊂ vecindex input window (2000 B) → fully embedded
           └─ trailer  ~440 B  → falls outside the window by design
      └─ chunk floor  = 480 B  = engram.SnippetBudget = 4 × MinSnippetBudget(120)

## File Changes

| Slice | File | Action | Description |
|---|---|---|---|
| WU-1 | `skills/_shared/ingested-observation-contract.md` | Create | Normative R-002 field block, topic-key grammar, `Source-Id` derivation, OQ-5 decision table, failure vocabulary |
| WU-1 | `skills/knowledge-ingestion/SKILL.md` | Create | The procedure (R-001, R-003, R-004): activation contract, fetch → `mem_save` → `promote`, manifest-first ordering, re-ingestion gate, partial-failure reporting |
| WU-1 | `skills.registry.yaml` | Modify | One `knowledge-ingestion` row (`custom` / `overlay-only`, four targets) |
| WU-1 | `engine/skills/ingested_contract_artifact_test.go` | Create | Repo-file assertion that the contract document states the field block, key grammar, and decision table verbatim — the `oo_quality_contract_artifact_test.go` precedent (`repoRoot = ../..`, `readRepoFile`) |
| WU-2 | `longterm-mem/internal/ingest/doc.go` | Create | Package doc: zero-egress statement, citation of the `_shared` contract as normative |
| WU-2 | `longterm-mem/internal/ingest/header.go` | Create | `Header`, `Header.Render`, `ParseHeader`, `NormalizeSlug`, `SourceID` |
| WU-2 | `longterm-mem/internal/ingest/chunk.go` | Create | Block segmentation and the chunk boundary (R-006) |
| WU-2 | `longterm-mem/internal/ingest/extract.go` | Create | `Extract`, `Request`/`Result`/`Source`/`Record`/`Skip`, directory scan (R-005, R-007) |
| WU-2 | `longterm-mem/internal/ingest/*_test.go` | Create | Boundary tables, slug determinism, header round-trip, scan ordering and skip codes |
| WU-2 | `longterm-mem/net_allowlist_test.go` | Modify (one line) | Add `internal/ingest/extract.go` to `TestNetImportAllowlistStillRefusesOthers`' must-never-be-allowlisted list. **`allowedNetImporters` is not touched and the `len == 1` assertion stands.** |
| — | `longterm-mem/internal/promote/*` | Unchanged | `ExplicitPromote` / `Writer.Promote` / `Eligible` / `findPromotedPage` consumed exactly as specified |
| — | `longterm-mem/internal/engram/store.go` | Unchanged | Read-only; no Go-side Engram write is introduced |
| — | `longterm-mem/internal/mcpserver/server.go` | Unchanged | No `ingest` tool; `query`/`get`/`promote` stay the whole MCP surface |
| — | `longterm-mem/internal/query/query.go` | Unchanged | Read for its constants only |
| — | `openspec/specs/knowledge-ingestion/spec.md`, `openspec/specs/longterm-mem-ingest-extraction/spec.md` | New at archive | The two capability specs |

Note on the one-line `net_allowlist_test.go` change: the guard's walk already covers every
non-test `.go` file under the module root, so `internal/ingest` is protected **without** any
edit. The added row is belt-and-braces in the style the test already uses (it names three
plausible offenders), documenting the intent for this package. If `sdd-tasks` prefers a
strictly zero-diff guard, dropping this line costs no coverage — the success criterion "the
allowlist still holds exactly one entry" is satisfied either way.

## Interfaces / Contracts

### The ingested-observation contract (R-002) — normative

`mem_save` arguments for every ingested record:

| Argument | Value |
|---|---|
| `topic_key` | `ingested/{kind}/{source-id}` (manifest or standalone) or `.../c{NNNN}` (chunk) |
| `title` | `Ingested: {source title}` for a manifest/standalone; `Ingested: {source title} ({n}/{N})` for a chunk |
| `type` | `discovery` — deliberately non-load-bearing (OQ-2) |
| `project` | the resolved project |
| `scope` | `project` |
| `capture_prompt` | `false` (automated artifact) |
| `content` | body → `---` → provenance block, below |

Provenance block, verbatim shape (the **last** element of `content`):

```markdown
---
**Ingested**: true
**Source-Kind**: url | file | directory | pasted
**Source-URI**: https://example.com/docs/config
**Source-Id**: example-com-docs-config-1a2b3c4d
**Source-Title**: Configuration reference
**Source-SHA256**: <64 hex of the whole normalized source>
**Content-SHA256**: <64 hex of this record's body>
**Ingested-At**: 2026-09-17T10:04:00Z
**Ingested-By**: <runtime/agent identifier>
**Chunk**: 3/12
**Chunk-Span**: 4096-5312
**Chunk-Path**: Configuration > Environment variables
**Split**: none | forced-sentence | forced-line | forced-byte
**Status**: complete
```

Manifest-only additions (a manifest carries no source body):

```markdown
**Chunks-Expected**: 12
**Chunks-Saved**: 12
**Status**: pending | complete | partial | retired
**Chunk-Status**:
- c0001 saved promoted
- c0002 saved promoted
- c0003 pending
```

Rules the contract fixes:
1. `Source-Id` is a function of the origin only (never the content).
2. A single-chunk source writes **one** record at the manifest key, with `Chunk: 1/1`; no
   separate chunk observation exists.
3. `**Chunk-Span**` is a byte range into the normalized source, so any chunk is locatable in
   the original.
4. The block is last, so the embedding window covers the body (see the trailer decision).
5. A record whose body is absent and whose `Status` is `retired` is a tombstone for a chunk
   the source no longer has; it is never deleted.

This project's `What / Why / Where / Learned` `mem_save` convention does **not** apply to
`ingested/` records, and the contract says so explicitly: that block describes work performed
in a session, while an ingested record describes a document that exists outside one.

### Go (`longterm-mem/internal/ingest`) — local only, no network, no subprocess

```go
// Package ingest turns already-fetched text and already-local files into
// records ready to hand to Engram's mem_save unchanged (R-005..R-007).
//
// It never fetches anything. It imports neither net nor net/http (R-071)
// nor os/exec (R-021); the module-wide static guards in
// net_allowlist_test.go and exec_allowlist_test.go cover this package
// without amendment. The record shape it emits is defined normatively in
// skills/_shared/ingested-observation-contract.md, which this package
// implements rather than re-specifies.
package ingest

const (
    // DefaultMaxChunkBytes is query.ResponseByteCeiling / query.DefaultTopN:
    // one chunk is worth at most one row's share of one whole query response,
    // and it sits inside vecindex.DefaultInputLimit so no part of a chunk is
    // invisible to the embedding arm.
    DefaultMaxChunkBytes = 1600
    // DefaultMinChunkBytes is engram.SnippetBudget: the floor at which a chunk
    // is still at most one rendered snippet, four times query.MinSnippetBudget.
    DefaultMinChunkBytes = 480
    // DefaultMaxSourceBytes bounds one source document.
    DefaultMaxSourceBytes = 4 << 20
    // MaxChunks bounds one source's chunk count so ordinals stay four digits.
    MaxChunks = 9999
)

type SourceKind string

const (
    KindURL       SourceKind = "url"
    KindFile      SourceKind = "file"
    KindDirectory SourceKind = "directory"
    KindPasted    SourceKind = "pasted"
)

// Origin is what a record records as its source of record. The agent supplies
// it: this package never learns an origin by reaching for it.
type Origin struct {
    Kind  SourceKind
    URI   string // canonical origin; for KindFile it overrides the absolute path
    Title string
    Label string // required for KindPasted, ignored otherwise
}

// Request is one Extract call. Text and Path are mutually exclusive.
type Request struct {
    Text          string
    Path          string
    Origin        Origin
    Recursive     bool          // directory scans are shallow unless set
    Now           func() time.Time
    IngestedBy    string
    MaxChunkBytes int // 0 -> DefaultMaxChunkBytes
    MinChunkBytes int // 0 -> DefaultMinChunkBytes
    MaxSourceBytes int // 0 -> DefaultMaxSourceBytes
}

type RecordRole string

const (
    RoleStandalone RecordRole = "standalone" // N == 1: manifest and content in one
    RoleManifest   RecordRole = "manifest"
    RoleChunk      RecordRole = "chunk"
)

// Record is exactly one mem_save call's arguments. A caller passes Content
// through unchanged; inventing or amending metadata here is the failure R-007
// exists to prevent.
type Record struct {
    TopicKey string
    Title    string
    Type     string // always "discovery"
    Role     RecordRole
    Content  string // body -> "---" -> provenance block
    Header   Header
    Bytes    int
}

type Source struct {
    Origin  Origin
    ID      string
    Digest  string   // Source-SHA256
    Records []Record // [0] is the manifest when len > 1
}

type Skip struct {
    Path   string
    Code   string // unreadable|binary|unsupported_extension|too_large|empty|symlink|outside_root
    Detail string
}

type Result struct {
    Sources []Source
    Skipped []Skip // always rendered; absent and "nothing skipped" must not look alike
}

// Extract resolves req into records. It returns an error only when the request
// itself cannot be resolved; every per-item failure is a Skip and the call
// still succeeds, possibly with zero Sources.
func Extract(req Request) (Result, error)

// Header is the provenance block's field set.
type Header struct { /* one field per contract line */ }

// Render writes Header as the contract's verbatim markdown block.
func (h Header) Render() string

// ParseHeader reads a rendered block back, so a re-ingestion can compare
// digests without re-deriving them.
func ParseHeader(content string) (Header, bool)

// NormalizeSlug is the single definition of slug normalization for ingestion
// identity: lowercase ASCII, runs outside [a-z0-9] collapsed to one '-',
// leading/trailing '-' trimmed, truncated at the last '-' boundary at or
// before the limit.
func NormalizeSlug(s string, limit int) string

// SourceID derives the origin-only identity described in the contract.
func SourceID(o Origin) string
```

**Deliberate duplication, disclosed**: `NormalizeSlug` also exists (with a different
signature and a fixed 48-byte limit) in the `procedural-candidate-detection` design's
`engine/skills/match.go`. `engine` and `longterm-mem` are separate Go modules, so sharing it
would mean a module dependency between them purely for a string function. Two small
independent implementations of a stated rule is the cheaper mistake; both cite the same
normalization rule in their doc comments, and both are table-tested.

## Testing Strategy

Strict TDD is enabled for this change: every row is RED before GREEN.
Commands: `cd longterm-mem && go vet ./... && go test ./...` and
`cd engine && go vet ./... && go test ./...`.

| Slice | Layer | What to test | Approach |
|---|---|---|---|
| WU-1 | Contract artifact (Go, `engine`) | `skills/_shared/ingested-observation-contract.md` states the provenance block, both topic-key shapes, the `Source-Id` rule, and the OQ-5 decision table verbatim | Repo-file assertion following `engine/skills/oo_quality_contract_artifact_test.go` (`filepath.Abs("..", "..")` + `readRepoFile`) |
| WU-1 | Registry (Go, `engine`) | `skills.registry.yaml` parses and carries a `knowledge-ingestion` row with `source.type: custom`, `lifecycle.updateStrategy: overlay-only`, four targets | `ParseRegistry` over the real registry file |
| WU-1 | Acceptance (executed in `sdd-verify`) | R-003 end to end: ingest one real external source, then show the observation via `mem_search`+`mem_get_observation` **and** the promoted page via `longterm-mem query --sources vault,engram-fts`, with the provenance block present in the page body | Numbered checklist inside `SKILL.md`, results recorded honestly in the verify report |
| WU-1 | Acceptance (`sdd-verify`) | R-004/OQ-5: re-ingest the identical source → reported unchanged, `revision_count` not incremented, no second topic key. Re-ingest after an edit → same key, revision incremented, same vault page path | Checklist rows |
| WU-1 | Acceptance (`sdd-verify`) | OQ-4: an `ingested/`-keyed observation that was never explicitly promoted is promoted by the next `sync` | Checklist row |
| WU-2 | Unit | Chunk boundary table: sizes 479 / 480 / 481 / 1599 / 1600 / 1601; atomic code fence larger than max; markdown table larger than max; document with no blank lines; CRLF input; multibyte rune straddling a forced byte cut; empty input; whitespace-only input; single heading with 40 KB of prose | Table test, `chunk_test.go` |
| WU-2 | Unit | Every emitted record carries the full contract field set (R-007), and `ParseHeader(Render(h)) == h` round-trips for every kind | Table test + property-style round-trip, `header_test.go` |
| WU-2 | Unit | `SourceID` determinism: same origin → same id across calls; URL canonicalization (case, default port, fragment, trailing slash); two origins with identical human parts get distinct ids; `pasted` without a label errors; content change does **not** change the id | Table test |
| WU-2 | Unit | Chunk metadata: `Chunk: n/N` monotone, `Chunk-Span` ranges contiguous and covering the whole normalized source, shared `Source-Id` and `Source-SHA256` across all chunks of one document (R-006's "recoverable as one document") | Table test |
| WU-2 | Unit | Directory scan: lexical ordering by relative path; every `Skip.Code` reachable; shallow by default and recursive on request; symlink never followed; zero readable files → success with zero sources and a populated `Skipped` | `t.TempDir()` fixtures, per `skills/go-testing` |
| WU-2 | Unit | Request validation errors: `Text` and `Path` both set / both empty; nonexistent path; invalid `Origin.Kind`; source exceeding `MaxChunks` | Table test |
| WU-2 | Guard (unchanged) | `TestNetImportAllowlist` and `TestNetImportAllowlistStillRefusesOthers` pass with `allowedNetImporters` still holding exactly one entry | Existing module-wide tests, run as-is |

No golden files, no network, no subprocess, and no new dependency are introduced. All
filesystem tests use `t.TempDir()`.

## Threat Matrix

| Boundary | Applicability | Reason / required behaviour |
|---|---|---|
| Routing / dispatch | N/A | No router, dispatcher, or verb selection changes. `internal/ingest` is a leaf package with no caller inside the binary in this change; the MCP surface stays `query`/`get`/`promote`. |
| Shell commands | N/A | No shell string is constructed, parsed, or executed. |
| Subprocess spawn / timeout / orphans | N/A | No process is spawned. `os/exec` is not imported; `exec_allowlist_test.go` (R-021) covers the package unmodified. |
| Network egress | **Applicable** | `internal/ingest` must import neither `net` nor `net/http`. Enforced by the existing module-wide `TestNetImportAllowlist` with `allowedNetImporters` unchanged at one entry (R-071). RED test: the guard is run before and after the package lands. |
| Filesystem traversal | **Applicable** | A directory scan must not follow symlinks (`os.Lstat`, `Skip{Code: "symlink"}`) and must not read outside the resolved scan root (`Skip{Code: "outside_root"}`). Shallow by default. RED tests: a symlink pointing outside the root, and a `..` component in a relative entry. |
| Resource exhaustion | **Applicable** | `MaxSourceBytes` (4 MiB) and `MaxChunks` (9999) bound one call. RED tests: an oversized file is skipped with `too_large`; a source that would exceed `MaxChunks` errors rather than truncating silently. |
| VCS / PR automation | N/A | No `git` or `gh` invocation. Vault pages are written by the existing `promote` path, not by this change. |
| Executable-file classification | N/A | Files are classified only as text-or-skip for reading. Nothing is executed, deployed, or mode-changed. |
| Process integration (hooks, settings.json) | N/A | No hook is installed or removed; `engine/settings/settings.go` is untouched. |

Recorded as a **risk, not a threat row**: ingested content is stored verbatim and is **not**
scrubbed for secrets, credentials, or private paths — explicitly out of scope per the
proposal. An operator ingesting a document containing a credential stores that credential in
Engram and, after promotion, in a git-tracked vault page. `SKILL.md` must carry this warning
at the point of use; it is a documented limitation, not a mitigation.

## Migration / Rollout

No data migration; nothing existing reads `ingested/` records.

Rollout is additive and per-slice:

- **WU-1** ships the contract and the procedure. From this point ingestion is repeatable by
  hand, chunking is the agent's judgment, and large documents are the known weak case.
- **WU-2** ships the extractor. The conforming path becomes the easy path; the contract does
  not change, because WU-2 implements the block WU-1 already fixed.

Rollback:

- **WU-2**: delete `longterm-mem/internal/ingest/` and revert the one-line
  `net_allowlist_test.go` addition. Nothing imports the package, so `promote`, `sync`,
  `query`, and the MCP surface are untouched by construction.
- **WU-1**: delete `skills/knowledge-ingestion/`, `skills/_shared/ingested-observation-contract.md`,
  the registry row and the `engine/skills` artifact test, then re-propagate the registry.
- **Already-ingested records** are inert data: ordinary Engram observations and ordinary
  promoted pages. Leave them, or retire them through the normal memory-lifecycle and vault
  supersession paths. Never hand-delete a vault page.
- Revert slice PRs in reverse order: `ingest-extraction` before `ingestion-convention`.

**Review-budget forecast (informational — `sdd-tasks` owns the guard lines).** Rough authored
counts, additions plus deletions:

| Slice | Estimate | Note |
|---|---|---|
| WU-1 `ingestion-convention` | ~400 | At the budget. Splitting into `1a` contract document + artifact test (~210) and `1b` SKILL.md + registry row (~200) is the safer shape, and `1a` is the dependency of both `1b` and WU-2. |
| WU-2 `ingest-extraction` | ~730 | **Exceeds the budget as one slice.** Split into `2a` header/slug/chunker + tests (~400) and `2b` `Extract`, directory scan, skip semantics + tests (~330). |

The proposal already forecast WU-2 as the larger slice needing a further split; this design
does not change that and does not silently design past it.

## Open Questions

- [ ] Whether Engram accepts an out-of-set `type` value such as `ingested` remains
      **unverified** — this phase has no shell and cannot probe it. The design is built so
      that the answer does not matter: no marker depends on `type`. If a later session
      verifies acceptance empirically, switching `type` to `ingested` is a compatible
      upgrade that buys `query.Request.ExcludeTypes` filtering of ingested rows, and it
      invalidates nothing already stored. Do not take it on the strength of the MCP schema's
      prose description alone.
- [ ] `skills/_shared/ingested-observation-contract.md` is **not** added to `gateContractFiles`
      (`engine/pipkg/pipkg.go`), so it is not copied into the Pi package, while the skill
      itself targets `pi`. That asymmetry is deliberate for this slice (packaging is a Go
      change outside scope) and should be revisited if a packaged Pi runtime needs the
      contract locally.
- [ ] Secret/credential scrubbing of ingested content is unscoped (proposal, Out of Scope).
      `SKILL.md` warns; nothing enforces.
- [ ] `Chunk-Span` positions are byte offsets into the **normalized** source (CRLF folded to
      LF). For a CRLF original they therefore do not index the original file's bytes. Stated
      in the contract; revisit only if a consumer needs original-file offsets.
