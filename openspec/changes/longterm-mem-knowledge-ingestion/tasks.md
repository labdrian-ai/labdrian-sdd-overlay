# Tasks: Knowledge ingestion into long-term memory

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1140 total across 4 slices (design forecast: WU-1 ~400 total, WU-2 ~730 total; split below: 1a ~210, 1b ~200, 2a ~400, 2b ~330) |
| 400-line budget risk | Medium overall — 1a and 1b individually sit safely under budget; 2a sits right at the 400-line boundary and is the slice most likely to need trimming if implementation runs long; 2b is comfortably under |
| Chained PRs recommended | Yes |
| Suggested split | PR 1a (contract doc) → PR 1b (SKILL.md + registry) → PR 2a (header/slug/chunker) → PR 2b (Extract/directory scan) |
| Delivery strategy | ask-on-risk (session preflight) |
| Chain strategy | stacked-to-main (established this session) |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Medium

### Slice-split rationale

The design's own forecast (`design.md`, "Review-budget forecast") already flags both work units at
or over budget and explicitly authorizes `sdd-tasks` to split further:

- **WU-1 `ingestion-convention`** forecast ~400 lines as one slice — at the budget, so it is split
  into **1a** (the normative contract document + its artifact test, ~210 lines) and **1b** (the
  agent-facing `SKILL.md` + the one registry row + its registry-parse test, ~200 lines). 1a is the
  dependency of 1b (the procedure cites the contract as normative) and of both WU-2 slices (the Go
  emitter cites the same contract).
- **WU-2 `ingest-extraction`** forecast ~730 lines as one slice — over budget — and is split into
  **2a** (`header.go`, `chunk.go`, `doc.go` plus their tests: slug, `SourceID`, provenance-block
  round-trip, chunk boundary and chunk metadata, ~400 lines) and **2b** (`extract.go`: `Extract`,
  request validation, per-item skip semantics, directory scan, plus its tests and the one-line
  `net_allowlist_test.go` addition, ~330 lines). 2b depends on 2a (`Extract` calls the chunker and
  the header renderer).

Dependency order: **1a → 1b → 2a → 2b**. 1a must land before 2a can cite a stable contract; 2a must
land before 2b can call it. 1b has no Go dependency on 2a/2b but is kept between them in the stack
because it completes work unit 1 before work unit 2 begins, matching the proposal's own two-work-unit
sequencing.

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1a contract | Normative ingested-observation contract document + artifact test (R-002, R-004) | PR 1a | `cd engine && go test ./skills/...` | Go table/repo-file assertion test, no fixtures | delete `skills/_shared/ingested-observation-contract.md` and `engine/skills/ingested_contract_artifact_test.go` |
| 1b procedure | `SKILL.md` procedure + registry row (R-001, R-003) | PR 1b | `cd engine && go test ./skills/...` | Registry-parse test over the real `skills.registry.yaml`; acceptance checklist executed manually during `sdd-verify` | delete `skills/knowledge-ingestion/`, revert the registry row, re-propagate the registry |
| 2a header/chunker | `Header`/`Render`/`ParseHeader`/`NormalizeSlug`/`SourceID` and the chunk boundary (R-006, part of R-007) | PR 2a | `cd longterm-mem && go test ./internal/ingest/...` | Go table tests + `t.TempDir()` where needed, no fixtures beyond in-memory strings | delete `longterm-mem/internal/ingest/header.go`, `chunk.go`, `doc.go` and their tests; package has no caller yet |
| 2b extract/scan | `Extract`, request validation, skip semantics, directory scan (R-005, R-007) | PR 2b | `cd longterm-mem && go vet ./... && go test ./...` | Go table tests over `t.TempDir()` fixtures; full-module run proves `TestNetImportAllowlist`/`TestNetImportAllowlistStillRefusesOthers` still pass | delete `longterm-mem/internal/ingest/extract.go` and its tests; revert the one-line `net_allowlist_test.go` addition |

**Note on TDD applicability.** R-005 and R-006 are pure Go and follow strict RED→GREEN→REFACTOR
throughout WU-2. R-001, R-002, R-003, R-004 are agent-driven prose plus one repo-file assertion test
each in WU-1 — the assertion test itself follows RED→GREEN like any other Go test (assert the
document contains required content, verbatim, before the content exists), but the *procedure's*
end-to-end behavior (R-003 promotion, R-004 re-ingestion) has no Go harness and is exercised through
the numbered acceptance checklist executed during `sdd-verify`, per the design's Testing Strategy
table.

## Phase 1a: ingestion-convention — contract document (PR 1a, R-002, R-004)

- [x] 1a.1 RED `engine/skills/ingested_contract_artifact_test.go` (new, following the
  `oo_quality_contract_artifact_test.go` precedent — `filepath.Abs("..", "..")` + `readRepoFile`):
  assert `skills/_shared/ingested-observation-contract.md` exists and contains, verbatim: the full
  provenance-block field list (`Ingested`, `Source-Kind`, `Source-URI`, `Source-Id`, `Source-Title`,
  `Source-SHA256`, `Content-SHA256`, `Ingested-At`, `Ingested-By`, `Chunk`, `Chunk-Span`,
  `Chunk-Path`, `Split`, `Status`); the manifest-only field additions (`Chunks-Expected`,
  `Chunks-Saved`, `Chunk-Status`); both topic-key shapes
  (`ingested/{source-kind}/{source-id}` and `ingested/{source-kind}/{source-id}/c{NNNN}`); the
  `Source-Id` derivation rule table (per-kind canonical origin and human part); and the OQ-5
  re-ingestion decision table (none/equal/differ/new-N<old-N/new-N>old-N/origin-changed rows)
- [x] 1a.2 GREEN `skills/_shared/ingested-observation-contract.md` (new): write the normative
  contract — the R-002 field block and provenance-block verbatim shape (body → `---` → provenance
  block, per design's Interfaces/Contracts section); the topic-key grammar with `{source-kind}` ∈
  `url | file | directory | pasted` and zero-padded four-digit chunk ordinals; the `Source-Id`
  derivation table; the OQ-5 decision table including the hard-no-op rule on unchanged digest and
  the retired-tombstone rule on shrunk chunk count (never hand-deleted); R-004's "no new dedup store"
  statement (only Engram `topic_key` upsert and existing `(project, engram_id)` page dedup); the
  failure vocabulary (the seven `Skip.Code` values and their meanings, mirrored from WU-2's contract
  so an agent reading only this document understands both layers); and the explicit note that the
  project's `What/Why/Where/Learned` `mem_save` convention does NOT apply to `ingested/` records
- [x] 1a.3 REFACTOR: confirm `skills/_shared/ingested-observation-contract.md` is prose-only (no
  registry row added in this slice — `_shared/` stays excluded from `engine/skills/manifest.go`'s
  `infraPrefixes`/`isInfraDir` check), and confirm `engine/skills/ingested_contract_artifact_test.go`
  follows the exact `readRepoFile`/`repoRoot` pattern of `oo_quality_contract_artifact_test.go` with
  no divergent helper
- [x] 1a.4 Verify: `cd engine && go test ./skills/...`

## Phase 1b: ingestion-convention — procedure and registration (PR 1b, R-001, R-003)

- [x] 1b.1 RED (extend or add an `engine/skills` registry-parse test): assert `ParseRegistry` over
  the real `skills.registry.yaml` returns a `knowledge-ingestion` entry with `source.type: custom`,
  `install.defaultScope: global`, `install.targets` containing exactly `claude`, `opencode`, `codex`,
  `pi`, and `lifecycle.updateStrategy: overlay-only`
- [x] 1b.2 GREEN `skills.registry.yaml` (modify): add the `knowledge-ingestion` row matching the
  `anti-generic-design` precedent shape (id, path, `source.type: custom`, the four install targets,
  `lifecycle.updateStrategy: overlay-only`)
- [x] 1b.3 GREEN `skills/knowledge-ingestion/SKILL.md` (new): write the procedure — activation
  contract (when an agent is asked to ingest external knowledge); the mandatory three-step path
  (agent fetch/read → Engram `mem_save` → `longterm-mem promote`) stated as the only intake path
  (R-001); the manifest-first ordering for a multi-chunk source (save manifest at `Status: pending`,
  promote it, then per-chunk save+promote in order, upsert manifest to `complete`/`partial`) per the
  design's OQ-6 WU-1 agent-layer contract; the re-ingestion gate (search the topic key, compare
  `Source-SHA256` before writing anything, per the contract in 1a.2); the partial-failure reporting
  rule (never report success for a partially ingested document; resuming is simply re-running
  ingestion); the explicit "no other intake path exists" statement (R-001 scenario); and the
  secret/credential warning from the design's Threat Matrix risk row (ingested content is stored and
  promoted verbatim, never scrubbed)
- [x] 1b.4 Acceptance checklist addition (non-Go, executed during `sdd-verify`, add to `SKILL.md`):
  numbered scenarios — (a) R-003 end-to-end: ingest one real external source, show the resulting
  observation via `mem_search` + `mem_get_observation` and the promoted page via
  `longterm-mem query --sources vault,engram-fts`, with the provenance block present in the page
  body; (b) R-004/OQ-5 unchanged re-ingestion: re-ingest the identical source, confirm "unchanged"
  is reported, `revision_count` did not increment, no second topic key was created; (c) R-004/OQ-5
  changed re-ingestion: re-ingest after an edit, confirm same key, incremented revision, same vault
  page path; (d) OQ-4 sync-eligibility: an `ingested/`-keyed observation that was never explicitly
  promoted is promoted by the next `sync`
- [x] 1b.5 Verify: `cd engine && go test ./skills/...`

## Phase 2a: ingest-extraction — header, slug, and chunk boundary (PR 2a, R-006, part of R-007)

Depends on Phase 1a (the contract document `header.go`'s doc comment cites as normative).

- [ ] 2a.1 RED `longterm-mem/internal/ingest/header_test.go`: `NormalizeSlug(s string, limit int)`
  table — lowercase ASCII, runs of non-`[a-z0-9]` collapsed to one `-`, leading/trailing `-` trimmed,
  truncation at the last `-` boundary at or before `limit`, empty string, all-punctuation string,
  unicode input, idempotence (`NormalizeSlug(NormalizeSlug(x, n), n) == NormalizeSlug(x, n)`)
- [ ] 2a.2 GREEN `longterm-mem/internal/ingest/header.go` (new): implement `NormalizeSlug`
- [ ] 2a.3 RED `header_test.go`: `SourceID(o Origin) string` determinism table — same origin returns
  the same id across repeated calls; URL canonicalization (lowercase scheme+host, default port
  stripped, fragment stripped, query kept, trailing `/` stripped unless path is `/`); two origins
  with identical human parts (slug collision) get distinct ids via the 8-hex suffix; `KindPasted`
  with an empty `Label` errors; changing only `Text`/file content (not the origin) does NOT change
  the id
- [ ] 2a.4 GREEN `header.go`: implement `Origin`, `SourceKind` constants, and `SourceID` per the
  per-kind canonical-origin table in the design (url/file/directory/pasted)
- [ ] 2a.5 RED `header_test.go`: `Header`/`Render`/`ParseHeader` round-trip — for every
  `SourceKind` and every `Status` value (`complete`, `partial`, `retired`), assert
  `ParseHeader(h.Render()) == h`; assert `Render()` places the provenance block after the body
  separator `---`; assert manifest-only fields (`Chunks-Expected`, `Chunks-Saved`, `Chunk-Status`)
  round-trip only when present
- [ ] 2a.6 GREEN `header.go`: implement `Header` struct (one field per contract line from 1a.2),
  `Render()`, and `ParseHeader()`
- [ ] 2a.7 RED `longterm-mem/internal/ingest/chunk_test.go`: chunk boundary table at and around the
  480/1600-byte bounds — sizes 479, 480, 481, 1599, 1600, 1601 bytes; an atomic fenced code block
  larger than `DefaultMaxChunkBytes`; a markdown table larger than `DefaultMaxChunkBytes`; a document
  with no blank lines; CRLF input (normalized to LF, verbatim body otherwise); a multibyte UTF-8 rune
  straddling a forced-byte cut point (must move back to the nearest rune boundary); empty input;
  whitespace-only input; a single heading followed by 40 KB of prose (must still respect
  `MaxChunkBytes`, not collapse to one oversized unit)
- [ ] 2a.8 GREEN `longterm-mem/internal/ingest/chunk.go` (new): implement block segmentation (ATX
  heading, fenced code block, table run, paragraph) and the boundary algorithm — CRLF normalization,
  greedy accumulation respecting `MinChunkBytes`/`MaxChunkBytes`, forced split in the documented
  order (sentence boundary → line boundary → hard byte cut moved to the nearest rune boundary), and
  the `Split` field (`none | forced-sentence | forced-line | forced-byte`) recorded per chunk
- [ ] 2a.9 RED `chunk_test.go`: chunk metadata table — `Chunk: n/N` monotonically increasing; `Chunk-
  Span` ranges contiguous and together covering the whole normalized source with no gap or overlap;
  all chunks of one document share the same `Source-Id` and `Source-SHA256`; `Chunk-Path` reflects
  the heading breadcrumb in effect at each chunk's first byte
- [ ] 2a.10 GREEN `chunk.go`: wire chunk metadata emission into `header.go`'s `Header` fields, so
  `chunk.go`'s output is ready for `Header.Render()`
- [ ] 2a.11 Create `longterm-mem/internal/ingest/doc.go` (new): package doc comment stating the
  zero-egress guarantee, citing `skills/_shared/ingested-observation-contract.md` as the normative
  contract this package implements (not re-specifies), and declaring
  `DefaultMaxChunkBytes = 1600`, `DefaultMinChunkBytes = 480`, `DefaultMaxSourceBytes = 4 << 20`,
  `MaxChunks = 9999` with the derivation comments from the design (`ResponseByteCeiling /
  DefaultTopN`, `engram.SnippetBudget`)
- [ ] 2a.12 REFACTOR: confirm `header.go`, `chunk.go`, and `doc.go` import neither `net` nor
  `net/http` nor `os/exec`
- [ ] 2a.13 Verify: `cd longterm-mem && go vet ./internal/ingest/... && go test ./internal/ingest/...`
- [ ] 2a.14 Verify the module-wide network guard is unaffected by the new package before it has any
  caller: `cd longterm-mem && go test -run 'TestNetImportAllowlist|TestNetImportAllowlistStillRefusesOthers' ./...`
  — both pass unmodified, confirming the guard's existing whole-module AST walk already covers
  `internal/ingest` without any edit to `net_allowlist_test.go` in this slice

## Phase 2b: ingest-extraction — Extract, skip semantics, and directory scan (PR 2b, R-005, R-007)

Depends on Phase 2a (`Extract` calls `chunk.go` and `header.go`).

- [ ] 2b.1 RED `longterm-mem/internal/ingest/extract_test.go` (new): `Extract` request-validation
  table — `Text` and `Path` both set errors; both empty errors; `Origin.Kind` invalid errors;
  `KindPasted` with empty `Label` errors; a source whose chunk count would exceed `MaxChunks` (9999)
  errors rather than silently truncating or renumbering
- [ ] 2b.2 GREEN `longterm-mem/internal/ingest/extract.go` (new): define `Request`, `Result`,
  `Source`, `Record`, `Skip`, `RecordRole` types per the design's Interfaces/Contracts section, and
  implement `Extract`'s request-validation branch — returns a hard `error` only for these cases
- [ ] 2b.3 RED `extract_test.go`: per-item skip-code table, one case per code — `unreadable`
  (permission-denied fixture), `binary` (NUL byte within the first 8 KiB), `unsupported_extension`
  (extension outside `.md`, `.markdown`, `.txt`, `.text`, `.rst`, `.adoc`), `too_large` (above
  `MaxSourceBytes`), `empty` (zero bytes and whitespace-only), `symlink` (entry is a symlink, never
  followed), `outside_root` (resolved path escapes the scan root); assert a call producing zero
  units plus a non-empty `Skipped` list is success, not error
- [ ] 2b.4 GREEN `extract.go`: implement single-source extraction for `Text` and for `Path` pointing
  at one file — produce `Skip` records for each reachable failure code above, and for a readable
  source call `chunk.go`'s boundary function and `header.go`'s `SourceID`/`Header.Render` to build
  `Record`s; assert `Result.Skipped` is always rendered (never `omitempty`)
- [ ] 2b.5 RED `extract_test.go`: directory-scan table using `t.TempDir()` fixtures — lexical
  ordering by relative path; shallow scan by default; recursive scan only when `Request.Recursive`
  is set; a symlinked entry is skipped and never followed (`os.Lstat`); a directory with zero
  readable files returns success with zero sources and a fully populated `Skipped` list
- [ ] 2b.6 GREEN `extract.go`: implement the directory-scan path (`Path` pointing at a directory) —
  lexical ordering, shallow/recursive switch, `os.Lstat`-based symlink detection, and the
  `outside_root` guard for any resolved entry escaping the scan root
- [ ] 2b.7 RED `extract_test.go`: R-007 field-completeness table — a single-unit extraction's one
  `Record` carries origin, ingestion timestamp, and the ingested-vs-session marker; a multi-chunk
  extraction's every `Record` independently carries the same three fields, identical across all N
  chunks, differing only in chunk-specific fields (position, `Chunk-Span`, content)
- [ ] 2b.8 GREEN `extract.go`: ensure every emitted `Record` sets `Type: "discovery"`, the correct
  `TopicKey` shape (manifest/standalone vs `.../c{NNNN}` chunk, per 1a.2's grammar), and `Title`
  (`Ingested: {source title}` or `Ingested: {source title} ({n}/{N})` for a chunk)
- [ ] 2b.9 Modify `longterm-mem/net_allowlist_test.go`: add `internal/ingest/extract.go` to
  `TestNetImportAllowlistStillRefusesOthers`'s must-never-be-allowlisted list (one line); do not
  touch `allowedNetImporters` and do not change its `len == 1` assertion
- [ ] 2b.10 REFACTOR: confirm `extract.go` imports neither `net` nor `net/http` nor `os/exec`; confirm
  `doc.go`'s citation of the contract document (2a.11) still matches `extract.go`'s actual `Record`
  field population
- [ ] 2b.11 Verify: `cd longterm-mem && go vet ./... && go test ./...` — full module run, confirming
  `TestNetImportAllowlist` and `TestNetImportAllowlistStillRefusesOthers` both pass with the
  allowlist still holding exactly one entry (`internal/embed/client.go`)
- [ ] 2b.12 Verify threat-matrix coverage explicitly (cross-reference, no new tests expected beyond
  2b.1/2b.3/2b.5): filesystem traversal — a symlink pointing outside the scan root, and a relative
  entry containing a `..` component, are both covered by the `symlink`/`outside_root` cases in 2b.5;
  resource exhaustion — an oversized file skip (`too_large`) is covered by 2b.3, and a source
  exceeding `MaxChunks` is covered by 2b.1

## Phase 3: Full Verification (after all four slices land)

- [ ] 3.1 `cd longterm-mem && go vet ./... && go test ./...`
- [ ] 3.2 `cd engine && go vet ./... && go test ./...`
- [ ] 3.3 Confirm `net_allowlist_test.go`'s allowlist still holds exactly one entry and no `.go` file
  added or modified by this change imports `net` or `net/http` (re-run the two allowlist tests named
  in 2a.14/2b.11 as the final confirmation after all slices are merged)
- [ ] 3.4 Confirm no Go source path introduced by this change opens Engram's database in a writable
  mode — `longterm-mem/internal/engram/store.go` stays untouched and read-only; no new `sql.Open`/DSN
  construction touching Engram appears anywhere in `longterm-mem/internal/ingest`
- [ ] 3.5 Run the full R-001/R-002/R-003/R-004 acceptance checklist (added in 1b.4) against real
  Engram records during `sdd-verify`; record results honestly in the verify report, including any
  scenario that could not be exercised
- [ ] 3.6 Confirm `skills/_shared/ingested-observation-contract.md` has no `skills.registry.yaml` row
  and is excluded from `ManifestView` (per `engine/skills/manifest.go`'s `infraPrefixes`), and that
  `skills/knowledge-ingestion/` has exactly the one row added in 1b.2
