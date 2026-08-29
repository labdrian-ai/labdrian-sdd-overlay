# Bridge probe: `engram obsidian-export` vs claude-obsidian wiki contract

Date: 2026-08-28. Follows `feasibility.md`. Two experiments run in parallel, then
two independent contract judges over the export output.

## Experiment 1 — provision retrieval on the real vault

`cd ~/labdrian-brain && bash bin/setup-retrieve.sh --no-llm` → exit 0, 15 s.

- 49 wiki pages → 131 chunks (all tier `synthetic`), BM25 vocab 4376.
- Rerank active: ollama `nomic-embed-text` reachable, `strategy:
  bm25+rerank:cosine:nomic-embed-text`.
- Created only under `.vault-meta/` (`chunks/`, `bm25/index.json` 566 KB,
  `embed-cache.json` 611 KB, two lock files) — all gitignored. No `wiki/` file
  touched. `git status` identical before/after.
- Test query `"DragonScale deterministic address" --top 3` → 2 candidates,
  correct page first (`c-000001`). Fewer than `--top` is rerank threshold
  filtering, not an error.
- Only one page has a real DragonScale address (`c-000001`); the other 48 got
  `syn-*` synthetic addresses because they predate address rollout.

**Result: the vault is now queryable headless.** `labdrian-brain` retrieval works.

## Experiment 2 — `engram obsidian-export` into a scratch copy

`engram obsidian-export --vault <copy> --project labdrian-sdd-overlay --limit 30
--graph-config skip` → exit 0.

Observed:
- **`--limit 30` was not honored**: 453 observation notes + 64 hub notes
  created (518 files). Upstream engram bug candidate.
- Output lands in a new vault-root folder `engram/`:
  `engram/<project>/<engram-type>/<slug60>-<id>.md`, `engram/_sessions/<uuid>.md`,
  `engram/_topics/<key>.md`, plus `engram/.engram-sync-state.json` (id → path
  map, incremental state).
- `.obsidian/` untouched (`--graph-config skip` respected).
- `.raw/.manifest.json` untouched: no `address_map` entries, no `sources`.
- Re-indexing the copy: BM25 doc_count stayed 131, **zero `engram/` chunks** —
  `contextual-prefix.py` hardcodes `WIKI_DIR = VAULT_ROOT / "wiki"`.
- Retrieval over export content: `retrieve.py "identity taste vault"` returned
  only `wiki/` pages. **`export_notes_retrievable = false`.**

## Contract judges (both: `conforms: no`, `postprocess_needed: true`)

Lens 1 — frontmatter/schema:
- BLOCKER: engram-native keys only (`id, type, project, scope, topic_key,
  session_id, created_at, updated_at, revision_count, tags, aliases`). None of
  the contract's universal keys (`title, created, updated, status, related,
  sources`) in any of 517 files.
- BLOCKER: every `type` value is outside the contract enum
  (`architecture 170, discovery 80, session_summary 57, bugfix 40, ...`).
- MAJOR: all 727 footer wikilinks dead — notes link `[[session-<uuid>]]` /
  `[[topic-<key>]]`, hub files are named `<uuid>.md` / `<key>.md` with no
  `aliases`.
- MAJOR: 45 untitled notes (`# ` empty H1, `- ""` alias, `observation-<id>.md`).
- MAJOR: timestamps quoted `"YYYY-MM-DD HH:MM:SS"` under non-contract keys.
- MINOR: 224 empty-string frontmatter values; tags don't follow
  `<domain>+<type>` pairing; slugs truncate at 60 chars mid-word and replace
  accented chars with `-`; SDD-artifact bodies leak a second H1 + bare
  `status:/created:` lines.
- Passes: flat YAML everywhere, balanced fences, deterministic filename
  `slug(title)[:60]-<id>` (but title-derived, so a retitled observation moves).

Lens 2 — ingest/links/addresses:
- BLOCKER: out-of-contract folder `engram/` — invisible to wiki-lint, RAG index,
  hot cache, `index.md`.
- BLOCKER: zero DragonScale addresses, `address_map` not updated, counter still
  `3`; all 453 pages are post-rollout → each would be a lint error if in scope.
- MAJOR: disconnected island — zero links to or from any existing `wiki/` page,
  64 hub notes are orphans, `index.md/log.md/hot.md/overview.md` never mention
  the export. 25 topic keys referenced have no hub at all.
- INFO: export bypasses wiki-ingest bookkeeping entirely; its only state is the
  private `.engram-sync-state.json`.

## Conclusion

`engram obsidian-export` is a **dump, not an ingest**. It is useful as a
read-only mirror for humans browsing in Obsidian, but it cannot be the
mid-term → long-term promotion bridge: nothing it writes is reachable by the
wiki's retrieval, lint, index, or address system, and fixing that would mean
rewriting frontmatter, types, links, filenames, addresses and the manifest for
every note — at which point we own the writer anyway.

Also, a wholesale dump is the wrong model for the goal. 1848 raw observations
are not "core knowledge"; the long-term layer is supposed to be curated
meta-cognition. Promotion must be **selective**.

## Design consequences for `longterm-mem`

1. **Own the promotion writer.** Read Engram via SQLite (read-only), select
   candidates (pinned, `decision`/`architecture`/`pattern` types, high
   `revision_count`, or explicit `promote` call), and emit contract-conformant
   `wiki/` pages: universal frontmatter, `type` mapped onto the contract enum
   with `engram_type` + `engram_id` as extra flat fields, DragonScale address
   via `scripts/allocate-address.sh`, `address_map` + `sources` entries in
   `.raw/.manifest.json`, `index.md`/`log.md` registration, resolvable
   wikilinks. Filename id-first (`<address> <Title>.md` or similar) so retitles
   don't relocate.
2. **Own re-indexing.** After any promotion, run `contextual-prefix.py --all` +
   `bm25-index.py build` (or watch mtimes). The plugin never does it.
3. **Unified query, two sources.** The MCP `query` tool fans out to Engram FTS5
   (mid-term, project-scoped) and vault BM25+rerank (long-term), returning one
   ranked list tagged by source. The join key is `engram_id` ↔ `page_address`.
4. **Don't depend on `obsidian-export`.** Optionally offer it as a separate
   "human mirror" command, never as the source of truth path.

## Side effects of this probe

- `~/labdrian-brain/.vault-meta/` now contains a live retrieval index
  (gitignored). Intended and kept.
- Scratch copy with the export lives at the session scratchpad
  (`brain-copy/`); disposable.
- Upstream candidate: `engram obsidian-export --limit N` ignored (v1.20.0).
