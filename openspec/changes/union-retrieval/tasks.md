# Tasks: Union Retrieval Over Engram's Own Rows

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | PR-1 250–350 · PR-2 ~420 · PR-3 ~450 · PR-4 ~400 + generated fixture |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR-1 merge → PR-2 embed client → PR-3 vecindex/index/ops → PR-4 embedding arm (two shapes, see Phase 4) |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending — ask the user: stacked-to-main or feature-branch-chain |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 0 | Blind gate queries + adjudication + check-1/check-3 prerequisites | pre-PR-1 | `go test ./internal/... -run Snippet` (check 3 shape) | `python evaluate.py` arm-D simulation (check 1) | delete `split/blind/*`; no production code touched |
| 1 | Merge, `sources`, budget-before-render, quota cap, R-006 | PR-1 | `go test ./internal/query/... ./internal/engram/...` | `longterm-mem query` against dev DB, inspect merged order | revert PR-1 commit; `sources` is additive+omitempty |
| 2 | `internal/embed` client + egress guard | PR-2 | `go test ./internal/embed/... ./longterm-mem/...` | loopback ollama round trip; refused non-loopback attempt | revert PR-2 commit; nothing reads the package yet |
| 3 | `internal/vecindex` + `index --embeddings` + doctor/status | PR-3 (gated by check 1) | `go test ./internal/vecindex/... ./internal/ops/...` | `longterm-mem index --embeddings` on dev DB; `doctor`/`status` output | delete `<state-dir>/index/<project>/`; revert PR-3 commit |
| 4 | Embedding arm + coverage + golden harness (branch A or B) | PR-4 | `go test ./internal/query/... -run Union` | `longterm-mem query --sources engram-fts,engram-embed` | flip default `sources` back to `["engram-fts"]`; revert PR-4 commit |

If PR-3 or PR-4 measures over 800 lines once check-1/check-3 evidence lands, split PR-3 at `internal/vecindex` (fingerprint+manifest) vs. `index --embeddings`+ops wiring, and split PR-4 at "embedding arm + coverage" vs. "golden harness port."

## Phase 0: Pre-Implementation Validation (blocks PR-1 and gates PR-3/PR-4)

- [ ] 0.1 Fresh agent (no exposure to §4.2/§4.3 of `openspec/decisions/union-retrieval.md` or `evaluate.py`'s gate code, no routing result) writes 20 paraphrase + 20 multi-token identifier queries plus expected `engram_id`s into `split/blind/{paraphrase,identifier}_queries.json`; commit and record its sha in `openspec/decisions/union-retrieval-gate-validation.md`.
- [ ] 0.2 Third-party adjudicator (not the query author, not the design author) reviews all 40 ground-truth judgements before scoring; drop disagreements, record surviving `n`.
- [ ] 0.3 Run `evaluate.py` against the committed query file; record both shas, per-class routing accuracy, and the ≥90% / 80–89% / <80% / <34-scoreable outcome in `openspec/decisions/union-retrieval-gate-validation.md`. This determines which Phase 4 branch ships.
- [ ] 0.4 RED: `TestSnippetBudgetIsAllocatedBeforeRender` fixture using real encoded `Result` values (full diagnostic set + `Standing` objects present) — this is design's "check 3," a prerequisite for fixing `MinSnippetBudget=120` in Phase 1.
- [ ] 0.5 Python simulation: port arm D against the Go tokenizer/fingerprint shape and confirm it still reproduces `93/100 · 88/94 · 40/70` before any `internal/vecindex` code is written — design's "check 1," a merge prerequisite for PR-3. If it fails to reproduce, stop before Phase 3.

## Phase 1: Merge, Sources, Budget, Cap (PR-1) — R-006, R-058–R-063

- [ ] 1.1 RED `TestSearchTokensSplitsOnFieldsNotPunctuation` + `TestUnionGoldenUsesProductionTokenizer` (`internal/engram/tokens_test.go`) — want `"search.go:181"` → one token, `"a/b c"` → `["a/b","c"]`.
- [ ] 1.2 GREEN: extract `engram.SearchTokens` into `internal/engram/tokens.go` from `search.go:272`.
- [ ] 1.3 RED `TestUnknownSourceIsRefusedNotIgnored`, `TestOmittedSourcesQueriesBothEngramArmsNotVault`, `TestNamingVaultInvokesIt` (`internal/query/query_test.go`) — R-060.
- [ ] 1.4 GREEN: add `sources` param, `SourceEngramFTS/SourceEngramEmbed/SourceVault` consts, default `["engram-fts","engram-embed"]`, refuse unknown names, in `internal/query/query.go`.
- [ ] 1.5 RED: round-robin interleave + dedup property tests — subsequence-per-source invariant, `TestRowFoundByBothSourcesEmittedOnceAtEarliestRank`, `TestVaultFirstAppliesOnlyWhenVaultRequested`, `TestLinkedPairEmittedOnce` — R-006.
- [ ] 1.6 GREEN: rewrite `mergeResults` per amended R-006; update `Sources []string` on `ResultRow` (list, not scalar).
- [ ] 1.7 RED `TestGateRoutesIdentifierShapesToLexicalArm`, `TestGateDoesNotFireOnHyphenatedEnglish`, `TestAnIncorrectRank1RoutingDoesNotShrinkTheGuarantee` (`internal/query/gate_test.go`) — R-058/R-059. Wire against Phase 0's committed blind queries once scored.
- [ ] 1.8 GREEN: create `internal/query/gate.go` with `routeRank1(tokens, matchMode)`; no score computed or compared (D8).
- [ ] 1.9 RED `TestSnippetShareIsPerRowNotPerSource`, `TestUnusedSnippetShareIsRedistributedExactlyOnce`, `TestCapResponseDropsFromTheLargestSourceNotTheTail` (`internal/query/budget_test.go`) — R-062/R-063.
- [ ] 1.10 GREEN: add `Row.MatchOffset int` and exported `engram.SnippetAt(content, offset, budget)` in `internal/engram/search.go`; embedding-arm rows use offset 0.
- [ ] 1.11 GREEN: implement budget-before-render allocation (`available/n`, clamp `MinSnippetBudget=120`..`SnippetBudget=480`, one redistribution pass) and quota-aware `capResponse` in `internal/query/query.go`.
- [ ] 1.12 Update `openspec/specs/longterm-mem-query/spec.md` R-006 scenarios to match shipped behavior (spec already amended; verify no drift).
- [ ] 1.13 Update `internal/mcpserver/server.go` with `sources` field mirroring `ExcludeTypes`.

## Phase 2: Embedding Client + Egress Guard (PR-2) — R-071

- [ ] 2.1 RED `TestNewClientRefusesNonLoopbackEndpoint`, `TestNewClientRefusesHostnameEvenWhenItResolvesToLoopback`, `TestClientRefusesRedirectAndNeverRequestsTheTarget` (httptest 302), `TestClientTimeoutIsExplicit`, `TestUnreachableBackendAndMissingModelAreDistinctErrors` (`internal/embed/client_test.go`).
- [ ] 2.2 GREEN: create `internal/embed/client.go` — one `*http.Client`, explicit timeout, literal-loopback-or-`localhost` check via `netip.Addr.IsLoopback()`, `CheckRedirect` errors, `--allow-remote-embedder` opt-in per invocation, default `http://127.0.0.1:11434`.
- [ ] 2.3 RED `TestOSExecImportAllowlistCatchesTestdataPackage` stays green; add `TestNetImportAllowlist`, `TestNetImportAllowlistStillRefusesOthers` (`longterm-mem/net_allowlist_test.go`) asserting `len(allowedNetImporters)==1` and refusing `internal/vecindex/build.go`, `internal/query/query.go`, `cmd/longterm-mem/main.go`.
- [ ] 2.4 GREEN: generalize `findOSExecImporters` into `findImporters(root, importPath)`; add `allowedNetImporters` map and `guardedImports = []string{"net/http","net"}`.

## Phase 3: Vector Index + Ops Wiring (PR-3, blocked on Phase 0.5) — R-064–R-069

- [ ] 3.1 RED `TestFingerprintIgnoresContentBeyondInputLimit`, corruption-detection test for a manifest revision mismatch (R-066).
- [ ] 3.2 GREEN: create `internal/vecindex/{index,fingerprint,build}.go` — fixed-stride blob + self-digesting JSON manifest at `<state-dir>/index/<project>/`.
- [ ] 3.3 RED: incremental build test — re-embeds only missing/changed fingerprints, removes entries for rows no longer live (R-069).
- [ ] 3.4 GREEN: implement incremental `index --embeddings` build path in `internal/vecindex/build.go` and wire into `cmd/longterm-mem/`.
- [ ] 3.5 RED: three `ops.Check` tests — `embedding-index-present`, `embedding-index-fresh` (names N), `embedding-backend-reachable` distinct from missing-model (R-064).
- [ ] 3.6 GREEN: add the three checks to `internal/ops/doctor.go`.
- [ ] 3.7 RED: `status` reports `embedding_index_built_at` or literal `never` (R-065).
- [ ] 3.8 GREEN: add the field to `internal/ops/status.go`.
- [ ] 3.9 Add `--allow-remote-embedder` flag to `cmd/longterm-mem/` `index` subcommand only (open question resolved: index-only, never `query`).

## Phase 4: Embedding Arm + Coverage + Golden Harness (PR-4) — R-058, R-061, R-068, R-070

- [ ] 4.1 RED `TestSoftDeletedObservationNeverSurfacesFromTheIndex` (`internal/query/embedarm_test.go`) — index a row, soft-delete it, query, want absent and `Unindexed` unchanged.
- [ ] 4.2 GREEN: add `Store.LiveObservationsByID(project string, ids []int64)` with `project = ? AND deleted_at IS NULL` in `internal/engram/store.go`; embedding arm calls this, never `ObservationByID`.
- [ ] 4.3 RED `TestResponseCarriesEmbeddingCoverageWhenSourceRequested`, `TestCoverageIsPresentEvenWhenIndexIsComplete` (no `omitempty`), `TestIncompleteCoverageDetailNamesTheRebuildCommand` (`internal/query/coverage_test.go`).
- [ ] 4.4 GREEN: add `Coverage` struct and `Coverage []Coverage` field (not omitempty) on `Result`; populate one entry for `engram-embed` when requested.
- [ ] 4.5 RED: `embedding_backend_unreachable` vs `embedding_model_missing` diagnostics are distinct and always present on degradation (R-070).
- [ ] 4.6 GREEN: wire named degradation into `internal/query/query.go`'s embedding-arm path.
- [ ] 4.7 RED `TestUnionGoldenFixtureUsesLiveFTSSchema` — recreate `observations_fts` from recorded `sqlite_master.sql`, not the default tokenizer.
- [ ] 4.8 GREEN: build golden fixture generator (`internal/query/testdata/union/*.gz`, gzipped, untruncated content + float32 vectors, ~250–350 rows, ground-truth + top-10-per-arm rows) with a `-update` path.
- [ ] 4.9 RED/GREEN: `TestUnionArmDReproducesPublishedTable` — golden test asserting `93/100 · 88/94 · 40/70`.
- [ ] 4.10 Branch on Phase 0.3's result, exactly one:
  - **Branch A (≥80% both classes)**: wire `routeRank1` (built in Phase 1.8) live into the embedding-arm query path; publish the routing accuracy number in `openspec/specs/longterm-mem-embedding-index/spec.md` if 80–89%, decision doc only if ≥90%.
  - **Branch B (<80% in either class, or <34/40 scoreable)**: do NOT wire `routeRank1` into production — delete the call site, keep `gate.go` unused or remove it; ship fixed FTS-first rank-1 order; publish the paraphrase@1 loss (40%→~10%) in `openspec/specs/longterm-mem-embedding-index/spec.md`.
- [ ] 4.11 Property test: merged set ⊇ each requested source's top-`top` rows, regardless of which Phase 4.10 branch shipped (R-058).
- [ ] 4.12 Update `openspec/decisions/union-retrieval-gate-validation.md` with final routing numbers and the shipped branch.

## Phase 5: Cleanup / Cross-Cutting

- [ ] 5.1 Confirm PR-1's default `sources=["engram-fts"]` (pre-Phase-4) takes the vault cold on the default path; document in PR-1's description.
- [ ] 5.2 Confirm `openspec/changes/shared-project-vault/` untouched across all four PRs.
- [ ] 5.3 Re-run full suite (`go test ./...`) plus `net_allowlist_test.go` and `exec_allowlist_test.go` together after PR-4 lands.
