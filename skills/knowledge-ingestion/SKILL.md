---
name: knowledge-ingestion
description: "The one sanctioned procedure for bringing external knowledge — a documentation
  page, a specification, a pasted report, a local file — into long-term memory. Trigger: when
  an agent is asked to ingest, remember, save, or add an external document/URL/file into
  long-term memory or the vault."
license: MIT
metadata:
  author: labdrian
  version: "1.0"
---

## Activation Contract

Use this skill whenever an agent is asked to bring external knowledge — a URL, a
documentation page, a specification, a pasted report, or a local file/directory — into
long-term memory. It does not apply to ordinary session observations an agent writes about
its own work (decisions, bug fixes, discoveries); those follow this project's normal
`mem_save` `What/Why/Where/Learned` convention, unchanged.

The normative field contract this procedure produces is defined once, in
`skills/_shared/ingested-observation-contract.md`. This skill is the procedure that uses that
contract; it does not restate it. Read the contract document before ingesting anything —
it is the single definition of the provenance block, the topic-key grammar, the `Source-Id`
derivation rule, and the re-ingestion decision table this procedure depends on.

## The only intake path

There is exactly one sanctioned way for external knowledge to enter long-term memory:

1. **Fetch or read** the source with the agent's own tools (`WebFetch` for a URL, `Read` for
   a local file, or the operator's pasted text taken verbatim). This is the only step that
   ever touches the network, and it is agent-owned — no Go code in this repository fetches
   anything.
2. **Save** the extracted content with memorycard's `mem_save`, using exactly the field and
   metadata shape defined in `skills/_shared/ingested-observation-contract.md`
   (`topic_key`, `title`, `type: "discovery"`, `body` = body → `---` → provenance block;
   for a manifest, the provenance block first).
3. **Promote** the resulting observation with `longterm-mem`'s `promote` (the `promote` MCP
   tool with `source: "memorycard"` and the memorycard id in `memory_id`, or CLI
   `promote --source memorycard --id N`, N being the memorycard id).

No other intake path exists. Content that was fetched or read but never passed through
`mem_save` is not "ingested" by this capability, regardless of whether it still sits in the
agent's own conversational context — closing the session loses it. There is no batch
ingestion command, no Go-side document parser wired into any tool surface, and no scheduled
or autonomous intake: every ingestion happens inside one agent turn, triggered by an explicit
request.

**Promotion is part of ingestion, not a separate later step.** Do not save an observation
and defer `promote` to "whenever `sync` gets to it" — `promote` accepts exactly one
observation id per call, so a multi-chunk source requires one `promote` call per chunk,
all inside the same ingestion turn. Reaching the vault must not depend on whether the
resulting `topic_key` happens to be automatically sync-eligible.

## Manifest-first ordering for a multi-chunk source

When the source is small enough for one record (`N == 1`), there is no separate manifest:
save and promote the single record at the manifest key with `Chunk: 1/1`, per the contract's
rule 2.

**Every chunk, with its provenance block, stays at most 64 KB.** Cut the source into smaller
chunks otherwise: memorycard stores a larger body as parts, and the block, last in a chunk,
would land outside part 1, where the re-ingestion gate and `promote` read. memorycard also
warns on a save above 16 KB, its soft target: the save still succeeds, and a chunk that
triggers the warning is a hint to cut the source smaller, not a failure.

When the source produces more than one chunk (`N > 1`), order matters so a crash mid-ingestion
is always readable and resumable:

1. Save the manifest observation with `**Status**: pending`, `**Chunks-Expected**: N`,
   `**Chunks-Saved**: 0`.
2. Promote the manifest.
3. For each chunk `c0001..cNNNN`, in order: `mem_save` the chunk, then `promote` it.
4. If every chunk succeeds: upsert the manifest to `**Status**: complete`,
   `**Chunks-Saved**: N`.
5. If a chunk fails: stop that source, upsert the manifest to `**Status**: partial` with the
   `**Chunk-Status**` list showing exactly which chunk keys are saved+promoted and which are
   missing, and report `ingested K of N` plus any skip detail.

**Resuming is simply re-running the same ingestion.** The re-ingestion gate below makes every
already-landed chunk a hard no-op, so a second run only does the missing work. There is no
separate resume command.

**Never report success for a partially ingested document.** A document that looks complete
but is 4 of 12 chunks is worse than no ingestion at all — the next reader has no way to know
the rest exists. Report the partial state honestly.

## Re-ingestion gate — check before writing anything

Before saving anything for a source, derive its `Source-Id` (per the contract's derivation
rule — a function of the origin only, never the content) and search its topic key:
`mem_search("ingested/{source-kind}/{source-id}")`, then `mem_get` to read the
stored `**Source-SHA256**` and, for a manifest record, `**Status**`. Read the record in full
before deciding: when memorycard stored it as parts, each part's title ends `(part i/N)` and
the rest sits in the memories at `<key>/part-2` … `<key>/part-N` (or `<key>/part-i~<n>` when
an import moved a part). A record stored before this procedure moved to memorycard may hold
its block, or only its `**Status**`, in one of those later parts.

The hard no-op only applies to a manifest whose `**Status**` is already `complete` — a
matching digest never short-circuits a `pending` or `partial` manifest, since an unchanged
source can still have unfinished chunks and a no-op there would silently report "unchanged"
for a document that is not fully ingested, breaking the "never report success for a partial
document" rule above.

- If the digest matches AND `**Status**: complete` (a single-chunk record at the manifest
  key carries `**Status**` too, `complete` while live, per the contract's chunk `**Status**`
  rule): this is a **hard no-op**. Make no `mem_save` call and no `promote` call. Report
  "unchanged".
- If the digest matches but `**Status**` is `pending` or `partial`: this is a **resume**, not
  a no-op. Re-run the manifest-first ordering above starting from step 3, using
  `**Chunk-Status**`, read in full (when memorycard stored the manifest as parts, its list
  continues in the memories at `<manifest key>/part-2`, `/part-3`, …, which `mem_search` and
  `mem_get` read like any other), to skip chunks already `saved promoted` and land only the
  missing ones.
- If the digest differs, or no record exists yet: proceed through the manifest-first ordering
  above. The full re-ingestion decision table (including the shrink/grow/origin-changed rows)
  lives in the contract document — consult it before writing, do not improvise the shrink or
  origin-changed cases.

## Secret and credential warning

Ingested content is stored in memorycard and, once promoted, in a git-tracked vault page,
**verbatim and unscrubbed**. This procedure performs no secret or credential scrubbing of any
kind — that is explicitly out of scope for this capability. Before ingesting a document,
consider whether it contains credentials, private paths, or other sensitive material that
should not become permanent, git-tracked memory. If in doubt, ask the operator before
ingesting.

## Acceptance checklist (executed during `sdd-verify`)

These scenarios are non-Go and are exercised by hand against real memorycard/`longterm-mem`
state, not by a unit test. Record results honestly in the verify report, including any
scenario that could not be exercised.

1. **R-003 end-to-end.** Ingest one real external source through this procedure. Show the
   resulting observation via `mem_search` + `mem_get`, and show the promoted page
   via `longterm-mem`'s `query` MCP tool with `sources: ["vault"]`. Confirm the provenance block is
   present in the page body.
2. **R-004 / OQ-5 unchanged re-ingestion.** Re-ingest the identical source a second time.
   Confirm the procedure reports "unchanged", the version history `mem_get` returns shows no
   new version, and no second topic key was created.
3. **R-004 / OQ-5 changed re-ingestion.** Edit the source, then re-ingest. Confirm it resolves
   to the same topic key, the version history `mem_get` returns shows one new version, and
   the vault page path is unchanged.
4. **OQ-4 sync-eligibility.** Take an `ingested/`-keyed observation that was saved but never
   explicitly promoted (e.g. a deliberately interrupted ingestion). Confirm the next `sync`
   promotes it.
