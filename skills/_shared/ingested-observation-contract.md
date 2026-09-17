# Ingested-Observation Contract

Normative. This document is the single definition of what an "ingested
observation" is. It is cited, not re-stated, by two consumers that must not
drift: the agent-facing `skills/knowledge-ingestion/SKILL.md` procedure and
the Go package `longterm-mem/internal/ingest` (its `Header.Render` doc
comment cites this file as normative). Neither consumer restates this field
block; both implement it.

This file lives under `skills/_shared/`, which is infra: it takes no
`skills.registry.yaml` row and is excluded from `ManifestView`
(`engine/skills/manifest.go`'s `infraPrefixes`/`isInfraDir`).

## Scope note

This project's ordinary `mem_save` convention — `**What** / **Why** /
**Where** / **Learned**` — does NOT apply to `ingested/` records. That block
describes work performed in a session; an ingested record describes a
document that exists outside one. Do not apply the What/Why/Where/Learned
shape to any record defined by this contract.

## `mem_save` arguments for every ingested record

| Argument | Value |
|---|---|
| `topic_key` | `ingested/{source-kind}/{source-id}` (manifest or standalone) or `ingested/{source-kind}/{source-id}/c{NNNN}` (chunk) |
| `title` | `Ingested: {source title}` for a manifest/standalone; `Ingested: {source title} ({n}/{N})` for a chunk |
| `type` | `discovery` — deliberately non-load-bearing; see "Marker mechanism" below |
| `project` | the resolved project |
| `scope` | `project` |
| `capture_prompt` | `false` (automated artifact) |
| `content` | body → `---` → provenance block, below |

## Marker mechanism (non-load-bearing `type`)

The ingested-vs-session marker is carried three redundant, independently
verifiable ways. None of them depends on the Engram `type` value:

1. The topic-key namespace's first segment, `ingested/`.
2. The title prefix, `Ingested: `.
3. The provenance trailer inside `content` (below) — the only one of the
   three that survives into the promoted vault page body.

## Topic-key grammar

```
ingested/{source-kind}/{source-id}             # manifest, or the whole record when N == 1
ingested/{source-kind}/{source-id}/c{NNNN}     # chunk NNNN of N, when N > 1
```

`{source-kind}` is one of: `url | file | directory | pasted`.

Chunk ordinals are zero-padded to four digits (`c0001`, `c0002`, ...) so a
namespace listing sorts lexically. A source whose chunk count would exceed
`9999` is refused with a named error rather than silently renumbered.

## Provenance block — verbatim shape

Record content is `body` → `---` → provenance block. The block is the LAST
element of `content`, after the body, never before it.

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

### Rules this contract fixes

1. `Source-Id` is a function of the origin only (never the content). See
   "Source-Id derivation" below.
2. A single-chunk source writes exactly ONE record, at the manifest key,
   with `**Chunk**: 1/1`. No separate chunk observation exists for it.
3. `**Chunk-Span**` is a byte range into the normalized source (CRLF folded
   to LF), so any chunk is locatable in the original document.
4. The provenance block is last in `content`, so the embedding window
   (`vecindex.DefaultInputLimit`) covers the body first, and truncation
   falls on the block's high-entropy digests instead of the prose.
5. A record whose body is absent and whose `**Status**` is `retired` is a
   tombstone for a chunk the source no longer has. It is never
   hand-deleted.

## `Source-Id` derivation

`Source-Id = NormalizeSlug(<human part>)[:40] + "-" + sha256(<canonical origin>)[:8]`

The id is a function of the origin only — it never reads the content. If it
did, an edited document would resolve to a new topic key, the upsert would
not fire, and every re-ingestion would fork a second memory of the same
source, which R-004 forbids.

| Source-Kind | Canonical origin | Human part |
|---|---|---|
| `url` | lowercase scheme+host, default port stripped, fragment stripped, query kept, trailing `/` stripped unless the path is `/` | `host + path` |
| `file` | cleaned absolute path, or the caller-supplied `Origin.URI` override when present | base name without extension |
| `directory` | as `file`, applied per contained file | as `file` |
| `pasted` | `pasted:` + the operator-supplied label (required) | the label |

The 8-hex suffix is collision insurance for two origins whose human parts
slug identically; the slug keeps the key readable.

## Re-ingestion decision table (OQ-5)

Re-ingestion resolves to the same topic key and upserts. There is never a
second parallel memory, and never an automatic supersession.

| Prior state at the key | `Source-SHA256` vs stored | Action |
|---|---|---|
| none | — | Create every record, promote each, manifest `**Status**: complete` |
| exists | **equal** | **Hard no-op.** No `mem_save`, no `promote`. Report "unchanged". |
| exists | **differ** | Upsert every chunk whose `Content-SHA256` changed (same key → same `engram_id` → `findPromotedPage`'s `(project, engram_id)` resolves the same page, updated in place); leave byte-identical chunks untouched; upsert the manifest with the new digest and inventory |
| exists, **new N < old N** | — | Surplus keys `c{N+1}…` upsert to a retired tombstone (`**Status**: retired`, trailer preserved, body removed) and are promoted so the vault page updates in place. **Never hand-deleted.** |
| exists, **new N > old N** | — | New chunk keys are created and promoted |
| **origin changed** (new URI ⇒ new `Source-Id`) | — | A different topic key. The old record is NOT touched automatically; `mem_compare(new, old, "supersedes")` is a documented operator step, after which promotion propagates `status: superseded` and the related-link on the next `sync`. |

The unchanged case is a HARD no-op, not an idempotent re-save: an upsert
increments `revision_count` and rewrites the page, which would make a
re-ingestion of unchanged content indistinguishable from a real update to
anything reading revisions, including `sync`'s own re-promotion trigger.

R-004's "no new dedup store" statement: the only identity/dedup mechanisms
in use for re-ingestion are Engram's own `topic_key` upsert and the existing
`(project, engram_id)` vault-page dedup already used by promotion. This
contract introduces no new SQLite database, file-based index, or other
store.

## Failure vocabulary (`Skip.Code`)

Mirrored from the `longterm-mem/internal/ingest` package's contract, so an
agent reading only this document understands both layers. `Extract` returns
a hard `error` only when the request itself cannot be resolved; every
per-item failure below is instead a `Skip` record, and the call still
succeeds (possibly with zero sources):

| `Skip.Code` | Meaning |
|---|---|
| `unreadable` | reading the source failed (permissions, I/O) |
| `binary` | a NUL byte was found within the first 8 KiB |
| `unsupported_extension` | the file extension is outside the text allowlist (`.md`, `.markdown`, `.txt`, `.text`, `.rst`, `.adoc`) |
| `too_large` | the source is above the configured maximum source size |
| `empty` | the source is zero bytes, or whitespace only |
| `symlink` | the entry is a symlink; symlinks are never followed |
| `outside_root` | the resolved path escapes the scan root |

`Skipped` is always rendered in a result, never omitted, so "ten files, ten
skipped" is reportable honestly and an absent list never looks the same as
"nothing was skipped".
