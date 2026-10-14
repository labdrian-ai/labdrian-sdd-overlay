# Tasks: Knowledge ingestion into long-term memory

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1290 total across 4 slices (design forecast: WU-1 ~400 total, WU-2 ~730 total; split below: 1a ~210, 1b ~200, 2a ~400, 2b ~480 — the Extractor use case over the `SourceReader` port ~330 plus the reading side of `internal/sourcefs` and its tests ~150, which the L11 amendment added; the design's ~730 for WU-2 becomes ~880) |
| 400-line budget risk | Medium overall — 1a and 1b individually sit safely under budget; 2a sits right at the 400-line boundary; 2b now sits over it, and splits at its seam if it runs long: the use case (2b.1–2b.4, 2b.7–2b.10) and the directory scan with the reading side of `sourcefs` (2b.5–2b.6a) |
| Chained PRs recommended | Yes |
| Suggested split | PR 1a (contract doc) → PR 1b (SKILL.md + registry) → PR 2a (header/slug/chunker) → PR 2b (Extractor over the `SourceReader` port, directory scan, `sourcefs` reads) |
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
  round-trip, chunk boundary and chunk metadata, ~400 lines) and **2b** (`extract.go`: the
  `Extractor` use case — `Extract`, request validation, per-item skip semantics, directory scan —
  over the `SourceReader` port `internal/ingest` owns, with its tests over an in-memory reader; the
  reading side of the `internal/sourcefs` adapter and its `t.TempDir()` tests; and the one-line
  `net_allowlist_test.go` addition, ~480 lines). 2b depends on 2a (`Extract` calls the chunker and
  the header renderer) and on L11 (Phase 9, batch 42: the port, `ResolveOrigin` and the adapter's
  `WorkingDirectory` landed there).

Dependency order: **1a → 1b → 2a → 2b**. 1a must land before 2a can cite a stable contract; 2a must
land before 2b can call it. 1b has no Go dependency on 2a/2b but is kept between them in the stack
because it completes work unit 1 before work unit 2 begins, matching the proposal's own two-work-unit
sequencing.

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1a contract | Normative ingested-observation contract document + artifact test (R-002, R-004) | PR 1a | `cd engine && go test ./skills/...` | Go table/repo-file assertion test, no fixtures | delete `skills/_shared/ingested-observation-contract.md` and `engine/skills/ingested_contract_artifact_test.go` |
| 1b procedure | `SKILL.md` procedure + registry row (R-001, R-003) | PR 1b | `cd engine && go test ./skills/...` | Registry-parse test over the real `skills.registry.yaml`; acceptance checklist executed manually during `sdd-verify` | delete `skills/knowledge-ingestion/`, revert the registry row, re-propagate the registry |
| 2a header/chunker | `Header`/`Render`/`ParseHeader`/`NormalizeSlug`/`SourceID` and the chunk boundary (R-006, part of R-007) | PR 2a | `cd longterm-mem && go test ./internal/ingest/...` | Go table tests, no fixtures beyond in-memory strings (no `t.TempDir()` since L11: `ingest_purity_guard_test.go` keeps `internal/ingest`'s tests off the file system) | delete `longterm-mem/internal/ingest/header.go`, `chunk.go`, `doc.go` and their tests; package has no caller yet |
| 2b extract/scan | The `Extractor` use case over the `SourceReader` port: `Extract`, request validation, skip semantics, directory scan; the reading side of `internal/sourcefs` (R-005, R-007) | PR 2b | `cd longterm-mem && go vet ./... && go test ./...` | Go table tests over an in-memory `SourceReader` for the use case (no file is touched); `t.TempDir()` fixtures only in `internal/sourcefs`'s tests; full-module run proves `TestNetImportAllowlist`/`TestNetImportAllowlistStillRefusesOthers` and the L11 guards (empty `knownDebt`, `ingest_purity_guard_test.go`, `sourcefs_adapter_guard_test.go`) still pass | delete `longterm-mem/internal/ingest/extract.go` and its tests, and the reading methods of `internal/sourcefs` with their tests (the port's `WorkingDirectory`, `ResolveOrigin` and the guards are L11's and stay); revert the one-line `net_allowlist_test.go` addition |

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

**Amended by L11 (Phase 9, batch 42).** 2a.4 delivered `SourceID` calling `filepath.Abs`, so the id of a
file or directory origin depended on the directory the process was in, and `internal/ingest` was the
last package on the architecture guard's debt table. L11 moved that behind the `SourceReader` port
`internal/ingest` owns (`source.go`): `ResolveOrigin(reader, origin)` begins a relative file or
directory path at `reader.WorkingDirectory()`, and `SourceID` derives the id of an absolute path
exactly as before and refuses a relative one. The ids 2a.3 and 2a.4 specify are unchanged
(`origin_file_test.go` holds them); `internal/sourcefs` answers the port on the local file system.
The 2a tasks below stay as they were delivered; 2b is where the amendment bites.

- [x] 2a.1 RED `longterm-mem/internal/ingest/header_test.go`: `NormalizeSlug(s string, limit int)`
  table — lowercase ASCII, runs of non-`[a-z0-9]` collapsed to one `-`, leading/trailing `-` trimmed,
  truncation at the last `-` boundary at or before `limit`, empty string, all-punctuation string,
  unicode input, idempotence (`NormalizeSlug(NormalizeSlug(x, n), n) == NormalizeSlug(x, n)`)
- [x] 2a.2 GREEN `longterm-mem/internal/ingest/header.go` (new): implement `NormalizeSlug`
- [x] 2a.3 RED `header_test.go`: `SourceID(o Origin) string` determinism table — same origin returns
  the same id across repeated calls; URL canonicalization (lowercase scheme+host, default port
  stripped, fragment stripped, query kept, trailing `/` stripped unless path is `/`); two origins
  with identical human parts (slug collision) get distinct ids via the 8-hex suffix; `KindPasted`
  with an empty `Label` errors; changing only `Text`/file content (not the origin) does NOT change
  the id
- [x] 2a.4 GREEN `header.go`: implement `Origin`, `SourceKind` constants, and `SourceID` per the
  per-kind canonical-origin table in the design (url/file/directory/pasted)
- [x] 2a.5 RED `header_test.go`: `Header`/`Render`/`ParseHeader` round-trip — for every
  `SourceKind` and every `Status` value (`complete`, `partial`, `retired`), assert
  `ParseHeader(h.Render()) == h`; assert `Render()` places the provenance block after the body
  separator `---`; assert manifest-only fields (`Chunks-Expected`, `Chunks-Saved`, `Chunk-Status`)
  round-trip only when present
- [x] 2a.6 GREEN `header.go`: implement `Header` struct (one field per contract line from 1a.2),
  `Render()`, and `ParseHeader()`
- [x] 2a.7 RED `longterm-mem/internal/ingest/chunk_test.go`: chunk boundary table at and around the
  480/1600-byte bounds — sizes 479, 480, 481, 1599, 1600, 1601 bytes; an atomic fenced code block
  larger than `DefaultMaxChunkBytes`; a markdown table larger than `DefaultMaxChunkBytes`; a document
  with no blank lines; CRLF input (normalized to LF, verbatim body otherwise); a multibyte UTF-8 rune
  straddling a forced-byte cut point (must move back to the nearest rune boundary); empty input;
  whitespace-only input; a single heading followed by 40 KB of prose (must still respect
  `MaxChunkBytes`, not collapse to one oversized unit)
- [x] 2a.8 GREEN `longterm-mem/internal/ingest/chunk.go` (new): implement block segmentation (ATX
  heading, fenced code block, table run, paragraph) and the boundary algorithm — CRLF normalization,
  greedy accumulation respecting `MinChunkBytes`/`MaxChunkBytes`, forced split in the documented
  order (sentence boundary → line boundary → hard byte cut moved to the nearest rune boundary), and
  the `Split` field (`none | forced-sentence | forced-line | forced-byte`) recorded per chunk
- [x] 2a.9 RED `chunk_test.go`: chunk metadata table — `Chunk: n/N` monotonically increasing; `Chunk-
  Span` ranges contiguous and together covering the whole normalized source with no gap or overlap;
  all chunks of one document share the same `Source-Id` and `Source-SHA256`; `Chunk-Path` reflects
  the heading breadcrumb in effect at each chunk's first byte
- [x] 2a.10 GREEN `chunk.go`: wire chunk metadata emission into `header.go`'s `Header` fields, so
  `chunk.go`'s output is ready for `Header.Render()`
- [x] 2a.11 Create `longterm-mem/internal/ingest/doc.go` (new): package doc comment stating the
  zero-egress guarantee, citing `skills/_shared/ingested-observation-contract.md` as the normative
  contract this package implements (not re-specifies), and declaring
  `DefaultMaxChunkBytes = 1600`, `DefaultMinChunkBytes = 480`, `DefaultMaxSourceBytes = 4 << 20`,
  `MaxChunks = 9999` with the derivation comments from the design (`ResponseByteCeiling /
  DefaultTopN`, `engram.SnippetBudget`)
- [x] 2a.12 REFACTOR: confirm `header.go`, `chunk.go`, and `doc.go` import neither `net` nor
  `net/http` nor `os/exec`
- [x] 2a.13 Verify: `cd longterm-mem && go vet ./internal/ingest/... && go test ./internal/ingest/...`
- [x] 2a.14 Verify the module-wide network guard is unaffected by the new package before it has any
  caller: `cd longterm-mem && go test -run 'TestNetImportAllowlist|TestNetImportAllowlistStillRefusesOthers' ./...`
  — both pass unmodified, confirming the guard's existing whole-module AST walk already covers
  `internal/ingest` without any edit to `net_allowlist_test.go` in this slice

## Phase 2b: ingest-extraction — Extract, skip semantics, and directory scan (PR 2b, R-005, R-007)

Depends on Phase 2a (`Extract` calls `chunk.go` and `header.go`) and on L11 (Phase 9, batch 42).

**Amended by L11 (Phase 9, batch 42): `internal/ingest` stays pure.** `extract.go` reads no file and no
directory itself. Extraction is an ingestion use case, `ingest.Extractor{Reader SourceReader}`, over the
`SourceReader` port that `internal/ingest` owns (`source.go`, where L11 put `WorkingDirectory` and
`ResolveOrigin`); `internal/sourcefs` answers the port on the local file system with `os`; and
`cmd/longterm-mem`, the composition root, is the only package that may name `sourcefs`
(`sourcefs_adapter_guard_test.go`). This change adds no command and no MCP tool (Threat Matrix, routing),
so it wires nothing: the change that adds the first command that ingests builds
`ingest.Extractor{Reader: sourcefs.Reader{}}` in `cmd`. Three consequences for the tasks below:

- `internal/ingest` imports no `os`, `os/exec`, `net` or `net/http`, uses no `path/filepath` member that
  reaches the disk (`Abs`, `EvalSymlinks`, `Glob`, `Walk`, `WalkDir`), reads no wall clock, and names no
  adapter, in its tests as in its code. The architecture test (empty `knownDebt`) and
  `ingest_purity_guard_test.go` hold that; a new test of the use case does not use `t.TempDir()`,
  `t.Chdir` or `t.Setenv`, and does not import `sourcefs`.
- The use case is tested over an in-memory `SourceReader` that the case builds (entries, bytes, listings
  and failures). Cases that need a real directory (a permission-denied file, a real link, a missing
  path) are the adapter's, in `internal/sourcefs`'s own tests with `t.TempDir()`.
- The port stays the use case's vocabulary: its methods return `ingest`'s own types and report a missing
  entry as an error that satisfies `errors.Is(err, fs.ErrNotExist)`, never by its words.

- [ ] 2b.1 RED `longterm-mem/internal/ingest/extract_test.go` (new): `Extract` request-validation
  table, over an in-memory `SourceReader` (no file is touched) — `Text` and `Path` both set errors;
  both empty errors; `Origin.Kind` invalid errors; `KindPasted` with empty `Label` errors; a source
  whose chunk count would exceed `MaxChunks` (9999) errors rather than silently truncating or
  renumbering
- [ ] 2b.2 GREEN `longterm-mem/internal/ingest/extract.go` (new): define `Request`, `Result`,
  `Source`, `Record`, `Skip`, `RecordRole` types per the design's Interfaces/Contracts section; add the
  reading side to the `SourceReader` port in `source.go`, beside `WorkingDirectory`, in `ingest`'s own
  types — `Lstat` (the kind of an entry and its size, without following a link), `ReadFile` (an entry's
  bytes) and `ReadDir` (the names in a directory); define the `Extractor` use case, a struct with a
  `Reader SourceReader` field and the method `Extract(req Request) (Result, error)` (there is no
  package-level `Extract` that reaches for the machine), and implement `Extract`'s request-validation
  branch — returns a hard `error` only for these cases. Every touch of the machine is a call on
  `Extractor.Reader`; `Text` requests never ask it anything
- [ ] 2b.3 RED `extract_test.go`: per-item skip-code table, one case per code, over an in-memory
  `SourceReader` whose entries, bytes and failures the case sets — `unreadable` (the reader's `ReadFile`
  fails), `binary` (NUL byte within the first 8 KiB of the bytes it returns), `unsupported_extension`
  (extension outside `.md`, `.markdown`, `.txt`, `.text`, `.rst`, `.adoc`), `too_large` (the size
  `Lstat` reports is above `MaxSourceBytes`; the bytes are not read), `empty` (zero bytes and
  whitespace-only), `symlink` (`Lstat` reports a link; it is never followed, and the case fails if the
  use case asks the reader to read it), `outside_root` (an entry whose cleaned absolute path is not
  below the scan root's); assert a call producing zero units plus a non-empty `Skipped` list is
  success, not error
- [ ] 2b.4 GREEN `extract.go`: implement single-source extraction for `Text` and for `Path` pointing
  at one file — the path made absolute with `ResolveOrigin` over the reader (so the id is a function
  of the origin, never of the process), the entry's kind and size asked through `Lstat` and its bytes
  read through `ReadFile`; produce `Skip` records for each reachable failure code above, and for a
  readable source call `chunk.go`'s boundary function and `header.go`'s `SourceID`/`Header.Render` to
  build `Record`s; assert `Result.Skipped` is always rendered (never `omitempty`)
- [ ] 2b.5 RED `extract_test.go`: directory-scan table over an in-memory tree — lexical ordering by
  relative path (whatever order the reader lists them in); shallow scan by default; recursive scan only
  when `Request.Recursive` is set; a symlinked entry is skipped and never followed (the reader reports a
  link from `Lstat`; the case fails if the use case lists or reads through it); a directory with zero
  readable files returns success with zero sources and a fully populated `Skipped` list
- [ ] 2b.5a RED `longterm-mem/internal/sourcefs/sourcefs_test.go` (extend, `t.TempDir()` fixtures):
  the reading side of the adapter on a real directory — `Lstat` tells a file, a directory, a link and
  anything else apart and reports a file's size, and does not follow a link (a link to a file outside
  the directory is a link, and the target is never opened); an entry that is not there fails with an
  error that satisfies `errors.Is(err, fs.ErrNotExist)`; `ReadFile` returns the bytes, and a file the
  process may not read fails (the case is skipped when the process can read it anyway, as root can);
  `ReadDir` lists the names in a directory without `.` and `..`, in no promised order (the use case
  sorts)
- [ ] 2b.6 GREEN `extract.go`: implement the directory-scan path (`Path` pointing at a directory) —
  lexical ordering (the use case sorts the names the reader lists), shallow/recursive switch, symlink
  detection from the kind `Lstat` reports (a link is a `symlink` skip: never followed, never read), and
  the `outside_root` guard for any entry whose cleaned absolute path is not below the scan root's,
  decided by the use case on the paths (no link is ever resolved, so the port has no `EvalSymlinks`)
- [ ] 2b.6a GREEN `longterm-mem/internal/sourcefs/sourcefs.go`: implement `Lstat`, `ReadFile` and
  `ReadDir` on `Reader` with `os.Lstat`, `os.ReadFile` and `os.ReadDir`, answering in `ingest`'s own
  types (the adapter imports `ingest`, never the other way). The adapter is the only place in the
  extraction that names `os`; its assertion that it satisfies `ingest.SourceReader` stays
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
- [ ] 2b.10 REFACTOR: confirm `extract.go` imports neither `net` nor `net/http` nor `os/exec` nor `os`,
  uses no `path/filepath` member that reaches the disk, and names no adapter (`internal/sourcefs`
  included); confirm `doc.go`'s citation of the contract document (2a.11) still matches `extract.go`'s
  actual `Record` field population, and that `doc.go` and `source.go` describe the port as it now is
- [ ] 2b.11 Verify: `cd longterm-mem && go vet ./... && go test ./...` — full module run, confirming
  `TestNetImportAllowlist` and `TestNetImportAllowlistStillRefusesOthers` both pass with the
  allowlist still holding exactly one entry (`internal/embed/client.go`), and that the L11 guards
  still pass with `knownDebt` empty: `TestArchitectureFollowsTheDependencyRule`,
  `TestIngestNamesNoAdapter`, `TestIngestReachesNothingAmbient`,
  `TestOnlyTheCompositionRootImportsTheSourceFSAdapter` and
  `TestEveryCallerOfSourceIDResolvesTheOriginFirst` (the `Extractor` of 2b.4 is the first caller of
  `SourceID`, and its file must also use `ingest.ResolveOrigin`: the guard fails the build otherwise)
- [ ] 2b.12 Verify threat-matrix coverage explicitly (cross-reference, no new tests expected beyond
  2b.1/2b.3/2b.5/2b.5a): filesystem traversal — a symlink pointing outside the scan root, and a
  relative entry containing a `..` component, are covered by the `symlink`/`outside_root` cases in
  2b.5 (the use case, over an in-memory reader) and by 2b.5a (the adapter reports a real link as a link
  and never opens its target); resource exhaustion — an oversized file skip (`too_large`) is covered by
  2b.3, and a source exceeding `MaxChunks` is covered by 2b.1

## Phase 3: Full Verification (after all four slices land)

- [ ] 3.1 `cd longterm-mem && go vet ./... && go test ./...`
- [ ] 3.2 `cd engine && go vet ./... && go test ./...`
- [ ] 3.3 Confirm `net_allowlist_test.go`'s allowlist still holds exactly one entry and no `.go` file
  added or modified by this change imports `net` or `net/http` (re-run the two allowlist tests named
  in 2a.14/2b.11 as the final confirmation after all slices are merged)
- [ ] 3.3a Confirm the L11 invariants after all slices are merged: `knownDebt` in
  `longterm-mem/architecture_test.go` is still empty; `internal/ingest` still reaches no file, process,
  clock or adapter, tests included (`TestIngestReachesNothingAmbient`); `cmd/longterm-mem` is still the
  only package that imports `internal/sourcefs`; every caller of `ingest.SourceID` still resolves the
  origin first (`TestEveryCallerOfSourceIDResolvesTheOriginFirst`); and no `.go` file added by this
  change outside `internal/sourcefs` names `os`
- [ ] 3.4 Confirm no Go source path introduced by this change opens Engram's database in a writable
  mode — `longterm-mem/internal/engram/store.go` stays untouched and read-only; no new `sql.Open`/DSN
  construction touching Engram appears anywhere in `longterm-mem/internal/ingest`
- [ ] 3.5 Run the full R-001/R-002/R-003/R-004 acceptance checklist (added in 1b.4) against real
  Engram records during `sdd-verify`; record results honestly in the verify report, including any
  scenario that could not be exercised
- [ ] 3.6 Confirm `skills/_shared/ingested-observation-contract.md` has no `skills.registry.yaml` row
  and is excluded from `ManifestView` (per `engine/skills/manifest.go`'s `infraPrefixes`), and that
  `skills/knowledge-ingestion/` has exactly the one row added in 1b.2
