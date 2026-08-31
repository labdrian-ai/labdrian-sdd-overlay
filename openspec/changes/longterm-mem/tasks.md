# Tasks: longterm-mem — Packaged Mid/Long-Term Memory Layer

Sources: specs (9 files, `openspec/changes/longterm-mem/specs/*/spec.md`) · design `design.md` + long-form `design-notes` (Engram #3133) · entry `entry.json` (Engram #3120) · delivery plan #3119.

## Review Workload Forecast

| # | Slice (PR) | Requirements | Lines (low–high) | Budget risk |
|---|---|---|---|---|
| 1 | longterm-mem-scaffold-module | R-001, R-002, R-020, R-021 | 330–370 | Medium |
| 2a | longterm-mem-scaffold-vault-2a-registry | R-003, R-022, R-023 | 150–180 | Low |
| 2b | longterm-mem-scaffold-vault-2b-runner-retrieve | R-004, R-024, R-021(guard) | 220–260 | Medium |
| 3a | longterm-mem-query-3a-index | R-005, R-025 | 170–200 | Low |
| 3b | longterm-mem-query-3b-merge | R-006, R-026 | 210–250 | Medium |
| 4 | longterm-mem-promotion-writer | R-007, R-027 | 360–410 | **Medium (near cap — see note)** |
| 5 | longterm-mem-promotion-registration | R-028, R-029 | 280–330 | Low |
| 6 | longterm-mem-promotion-mutability | R-008, R-030 | 330–380 | Medium |
| 7 | longterm-mem-promotion-sync | R-009, R-031, R-033 | 340–400 | Medium |
| 8a | longterm-mem-ops-8a-status-doctor | R-010, R-011 | 190–220 | Low |
| 8b | longterm-mem-ops-8b-mcp-promote | R-012, R-032, R-034 | 240–270 | Medium |
| 9 | longterm-mem-overlay-route | R-013, R-035 | 180–230 | Low |
| 10a | longterm-mem-overlay-dispatch-10a-engine | R-014(engine half) | 300–330 | Medium |
| 10b | longterm-mem-overlay-dispatch-10b-shell | R-014(shell half), R-015 | 180–210 | Low |
| 11a | longterm-mem-mcp-registration-json-11a-splice-install-state | R-016(shared), R-017(shared) | 170–200 | Low |
| 11b | longterm-mem-mcp-registration-json-11b-writers | R-016, R-017 | 260–300 | Medium |
| 12a | longterm-mem-mcp-registration-toml-uninstall-12a-toml | R-018 | 200–230 | Low |
| 12b | longterm-mem-mcp-registration-toml-uninstall-12b-uninstall | R-019 | 180–210 | Low |

**Decision needed before apply: No**
**Chained PRs recommended: Yes**
**Chain strategy: feature-branch-chain** (tracker `feat/longterm-mem`)
**400-line budget risk (overall): Medium** — driven by slice 4's upper estimate; see note below.

Note on slice 4: design.md classifies this slice Medium (not one of the six High-risk slices the entry contract names for splitting: 2, 3, 8, 10, 11, 12) and its own mitigation is explicit — keep `LintPage()` to its documented 6 rules rather than growing it. Its high-end estimate (410) brushes the 400-line cap by 10 lines. This is a known, already-designed-for risk, not an open decision: if actual authored lines trend past 400 during apply, split `promote/lint.go` + its golden tests into a `4b` follow-on PR (parent id + `-4b-lint`) based on `4`'s branch, keeping `page.go`/`frontmatter.go` as `4`. No other single (non-split) slice's high estimate reaches 400 (7 reaches exactly 400, 1/5/6 stay comfortably under).

### Chain Topology

Tracker branch `feat/longterm-mem` (draft, no-merge until every child is in). Each PR branch is `feat/longterm-mem/<slice-id>`, PR #1 targets the tracker, every later PR targets the immediately preceding slice's branch (feature-branch-chain). 18 PRs total (6 unsplit slices + 12 children from the 6 split High-risk slices per entry #3120 / design-notes split points).

```
feat/longterm-mem (tracker, draft)
 └─ 1  longterm-mem-scaffold-module
     └─ 2a longterm-mem-scaffold-vault-2a-registry
         └─ 2b longterm-mem-scaffold-vault-2b-runner-retrieve
             └─ 3a longterm-mem-query-3a-index
                 └─ 3b longterm-mem-query-3b-merge
                     └─ 4  longterm-mem-promotion-writer
                         └─ 5  longterm-mem-promotion-registration
                             └─ 6  longterm-mem-promotion-mutability
                                 └─ 7  longterm-mem-promotion-sync
                                     └─ 8a longterm-mem-ops-8a-status-doctor
                                         └─ 8b longterm-mem-ops-8b-mcp-promote
                                             └─ 9  longterm-mem-overlay-route
                                                 └─ 10a longterm-mem-overlay-dispatch-10a-engine
                                                     └─ 10b longterm-mem-overlay-dispatch-10b-shell
                                                         └─ 11a longterm-mem-mcp-registration-json-11a-splice-install-state
                                                             └─ 11b longterm-mem-mcp-registration-json-11b-writers
                                                                 └─ 12a longterm-mem-mcp-registration-toml-uninstall-12a-toml
                                                                     └─ 12b longterm-mem-mcp-registration-toml-uninstall-12b-uninstall → main (via tracker)
```

Ordering note: slice 9 has no prior dependency (`dependencies: []` in entry.json) and slice 10 depends on both 8 and 9. In a single linear feature-branch-chain, the number of predecessor slices before 10 is identical whether 9 sits at position 9 (this plan, matching entry.json's given order) or is moved earlier — 10 must still follow the later of {8, 9} in the sequence either way. This plan keeps entry.json's order for direct traceability; sdd-apply MAY reorder slice 9 earlier in the chain (e.g. right after slice 1) without violating any dependency, since 9 depends on nothing and nothing before 10 depends on 9.

## Traceability (R-001..R-035 = 35/35)

| Req | Slice | Task ids |
|---|---|---|
| R-001 | 1 | 1.1–1.3 |
| R-002 | 1 | 1.6–1.8 |
| R-003 | 2a | 2a.1, 2a.4 |
| R-004 | 2b | 2b.5–2b.6, 2b.8 |
| R-005 | 3a | 3a.1–3a.2, 3a.4–3a.5 |
| R-006 | 3b | 3b.1–3b.3, 3b.5–3b.7 |
| R-007 | 4 | 4.1–4.2 |
| R-008 | 6 | 6.3–6.4, 6.7 |
| R-009 | 7 | 7.1–7.2 |
| R-010 | 8a | 8a.1–8a.3 |
| R-011 | 8a | 8a.4–8a.6 |
| R-012 | 8b | 8b.1–8b.3, 8b.7 |
| R-013 | 9 | 9.1–9.4, 9.6–9.9 |
| R-014 | 10a, 10b | 10a.1–10a.3, 10a.5, 10a.8; 10b.1–10b.2, 10b.5–10b.6 |
| R-015 | 10b | 10b.3–10b.5 |
| R-016 | 11a, 11b | 11a.1–11a.5; 11b.1–11b.4 |
| R-017 | 11a, 11b | 11a.1–11a.5; 11b.5–11b.6 |
| R-018 | 12a | 12a.1–12a.5 |
| R-019 | 12b | 12b.1–12b.4 |
| R-020 | 1 | 1.9–1.10 |
| R-021 | 1, 2b | 1.4–1.5; 2b.10 (re-verify) |
| R-022 | 2a | 2a.2 |
| R-023 | 2a | 2a.3 |
| R-024 | 2b | 2b.7–2b.8 |
| R-025 | 3a | 3a.3–3a.4 |
| R-026 | 3b | 3b.4, 3b.6 |
| R-027 | 4 | 4.5–4.11 |
| R-028 | 5 | 5.1–5.3 |
| R-029 | 5, 7 | 5.4–5.5, 7.10 |
| R-030 | 6 | 6.5–6.7 |
| R-031 | 7 | 7.3–7.4 |
| R-032 | 8b | 8b.4–8b.6 |
| R-033 | 7 | 7.5–7.6, 7.8 |
| R-034 | 8b | 8b.8–8b.10 |
| R-035 | 9 | 9.5–9.6 |

## Anchors (design-notes #3133, D1–D13)

Module: `github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem`, `go 1.26.1` (matches `tui/go.mod`). Deps: `modernc.org/sqlite v1.57.0`, `github.com/modelcontextprotocol/go-sdk v1.7.0`, `github.com/pelletier/go-toml/v2 v2.4.3` (validator-only, no deps).
Binary: `~/.labdrian-overlay/bin/longterm-mem` (`$STATE_DIR/bin`).
Read-only Engram DSN: `file:<db>?mode=ro&_txlock=deferred&_pragma=query_only(1)&_pragma=busy_timeout(2000)` (default `~/.engram/engram.db`).
CLI: `longterm-mem query|index|sync|promote|status|doctor|mcp|vaults|register|unregister`, global flags `--project --vault --state-dir --engram-db --json`. Env: `LONGTERM_MEM_STATE_DIR`, `LONGTERM_MEM_VAULTS_FILE`, `LONGTERM_MEM_VAULT`, `LONGTERM_MEM_ENGRAM_DB` (flag > env > file).
Exit codes: `0` ok · `1` internal · `2` usage · `3` vault_not_configured · `4` engram_unavailable · `5` vault_subprocess_failed · `6` registration_conflict · `7` not_found.
Package layout: `internal/{engram,vaultreg,vault,query,promote,ops,mcpserver,register}`, `cmd/longterm-mem/` one file per subcommand. Sole `os/exec` importer: `internal/vault/runner.go` (R-021).
State files (schema 1, tmp+fsync+rename): `~/.labdrian-overlay/vaults.json`, `~/.labdrian-overlay/longterm-mem/install-state.json` (module-owned), `~/.labdrian-overlay/longterm-mem/registration.json` (engine-owned), `<vault>/.raw/.longterm-mem-manifest.json` (precedence store), `<vault>/.vault-meta/longterm-mem-sync-state.json`.
Route domain (`bin/labdrian-overlay` `route_resolve`, `engine/skills/ondisk.go` `nonSkillRoutes`): `{skill, agent, opencode-agent, mcp}`.

---

## Slice 1 — longterm-mem-scaffold-module (R-001, R-002, R-020, R-021) — PR1 (base: tracker `feat/longterm-mem`)

### Bootstrap

- [x] 1.0.1 Create `longterm-mem/go.mod` (`module github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem`, `go 1.26.1`); `go get modernc.org/sqlite@v1.57.0 github.com/modelcontextprotocol/go-sdk@v1.7.0 github.com/pelletier/go-toml/v2@v2.4.3` to generate `go.sum` (pinned upfront so version drift cannot appear mid-chain).
- [x] 1.0.2 Add `.github/workflows/ci.yml` job `test-longterm-mem`, cloned from `test-tui` (:273): checkout, `actions/setup-go@v5` with `go-version-file: longterm-mem/go.mod`, gofmt (`test -z "$(gofmt -l .)"`), `go vet ./...`, `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...`, `go test ./... -cover`, all `working-directory: longterm-mem`.
- [x] 1.0.3 Extend `openspec/config.yaml`: add a `longterm-mem` entry under `testing.layers` (`command: cd longterm-mem && go test ./...`, `vet: cd longterm-mem && go vet ./...`) and prepend `cd longterm-mem && go test ./... && ` to the `apply.tdd.test_command` and `verify.test_command`/`verify.build_command` chains.
- [x] 1.0.4 Create `longterm-mem/README.md` stub: per-project component description, fixed binary install path, module boundary note (R-001; full CLI/MCP refresh lands at 10b.8 per CHK-05).

### R-001 — Standalone Module Outside engine/

- [x] 1.1 RED `longterm-mem/cmd/longterm-mem/main_test.go::TestMain_BuildsIndependentModule` — asserts `go build ./...` succeeds from `longterm-mem/` as an independent module capable of declaring third-party deps. Scenario: "Component builds as an independent module".
- [x] 1.2 GREEN `longterm-mem/cmd/longterm-mem/main.go` — minimal subcommand dispatcher (usage text, exit 2 on unknown subcommand) so the build succeeds.
- [x] 1.3 RED→GREEN re-run `engine/skills/zero_fetch_test.go::TestZeroFetchImportAllowlist` unmodified after 1.0.1–1.2 land — asserts it still passes and `engine/go.mod` carries no third-party requirement despite `longterm-mem/go.mod` adding sqlite/mcp-sdk/go-toml. Scenario: "engine/'s zero-dependency gate stays green". Command: `cd engine && go test ./... -run TestZeroFetchImportAllowlist`.

### R-021 — No CLI Shelling to Engram (guard, enforced further in 2b)

- [x] 1.4 RED `longterm-mem/exec_allowlist_test.go::TestOSExecImportAllowlist` — statically parses (go/parser+ast) every non-test `.go` file under `longterm-mem/`, modeled on `engine/skills/zero_fetch_test.go`, and fails if any file other than `internal/vault/runner.go` imports `os/exec`. Scenario: "No subprocess call to Engram's CLI".
- [x] 1.5 GREEN — no production code required yet (only `main.go` exists); confirm the guard test passes vacuously. Command: `cd longterm-mem && go test ./... -run TestOSExecImportAllowlist`.

### R-002 — Read-Only Engram Connection

- [x] 1.6 RED `longterm-mem/internal/engram/store_test.go::TestOpen_DefaultIsReadOnly` + `TestOpen_OverridePathStaysReadOnly` — build `testdata/schema.sql` (Engram DDL, live schema #3129) into a `t.TempDir()` SQLite file via modernc; assert an `INSERT`/`UPDATE` on the opened handle fails and the no-override path resolves to `~/.engram/engram.db`. Scenarios: "Default connection is read-only", "Overridden connection stays read-only".
- [x] 1.7 GREEN `longterm-mem/internal/engram/store.go` — `Open(dbPath string) (*Store, error)` using the read-only DSN (D1); default path via `$HOME/.engram/engram.db`.
- [x] 1.8 REFACTOR — extract `readOnlyDSN(path string) string` helper for reuse by later fixture setups.
- Command: `cd longterm-mem && go test ./... -run TestOpen`

### R-020 — Mid-Term Query Scoping

- [x] 1.9 RED `longterm-mem/internal/engram/store_test.go::TestListObservations_ScopesProjectAndExcludesSoftDeleted` — fixture rows: active/P, soft-deleted/P, active/other-project; assert only active/P returned. Scenario: "Soft-deleted and other-project observations are excluded".
- [x] 1.10 GREEN `longterm-mem/internal/engram/store.go` — `ListObservations(project string) ([]Observation, error)` with `WHERE project = ? AND deleted_at IS NULL`.
- Command: `cd longterm-mem && go test ./... -run TestListObservations`

- [x] 1.11 Slice verification — `cd longterm-mem && go test ./...`; `bash -n bin/labdrian-overlay` (strict-TDD focused commands; unaffected but must stay green).

---

## Slice 2a — longterm-mem-scaffold-vault-2a-registry (R-003, R-022, R-023) — PR2a (base: PR1)

- [x] 2a.1 RED `longterm-mem/internal/vaultreg/registry_test.go::TestResolve_ConfiguredOverrideWins` — fixture `vaults.json` row for `some-other-project`; assert `Resolve` returns that configured path. Scenario: "Configured override is used" (R-003).
- [x] 2a.2 RED (same file) `TestResolve_DefaultSeedEntryForOverlayProject` — no override row for `labdrian-sdd-overlay`; assert resolution succeeds via the pre-seeded default row, itself an ordinary editable/deletable JSON row, not a code constant. Scenario: "Default resolves without a code-level constant" (R-022).
- [x] 2a.3 RED (same file) `TestResolve_UnconfiguredNonDefaultProjectRejected` — project `some-new-project`, no row, not `labdrian-sdd-overlay`; assert `ErrVaultNotConfigured` (exit 3 semantics) instead of a guessed path. Scenario: "Unconfigured, non-default project is rejected, not guessed" (R-023).
- [x] 2a.4 RED (same file) `TestPrecedence_FlagEnvFile` — `--vault` flag > `LONGTERM_MEM_VAULT` env > registry row (D5).
- [x] 2a.5 RED (same file) `TestSeed_OnlyWhenFileAbsent` — a `vaults.json` that exists but lacks the `labdrian-sdd-overlay` row is never auto-seeded (deleting the seed row must mean "not configured").
- [x] 2a.6 GREEN `longterm-mem/internal/vaultreg/registry.go` — `Registry{schema, vaults}` JSON model; `Load(path)`; `Seed(path)` (writes the seed row only when the file is absent); `Resolve(project, flagVault string) (string, error)` implementing flag>env>row precedence, `~` expansion, absolute-path validation, `ErrVaultNotConfigured`.
- [x] 2a.7 REFACTOR — extract the tmp+fsync+rename JSON write helper for reuse by later sidecar writers (D6).
- Command: `cd longterm-mem && go test ./... -run TestResolve|TestSeed|TestPrecedence`

---

## Slice 2b — longterm-mem-scaffold-vault-2b-runner-retrieve (R-004, R-024) — PR2b (base: PR2a)

- [x] 2b.1 RED `longterm-mem/internal/vault/runner_test.go::TestRunner_RefusesOutsideVaultRoot` — `Runner{Root: tmpVault}` called with a script path outside the root (after `EvalSymlinks`); assert refusal, no subprocess spawned. (Threat matrix: subprocess execution — outside-root refusal.)
- [x] 2b.2 RED (same file) `TestRunner_ArgvOnly_NoShellMetacharacters` — fixture script echoing `"$@"`; a query argument containing `; rm -rf /` arrives as one literal argv element, never shell-interpreted. (Threat matrix: metacharacter spy.)
- [x] 2b.3 RED (same file) `TestRunner_TimeoutSurfacesExitAndStderr` — fixture script sleeping past a short `context.WithTimeout`; assert a synthetic non-zero exit and captured stderr.
- [x] 2b.4 GREEN `longterm-mem/internal/vault/runner.go` — the sole `os/exec` importer: `Runner{Root string}`, `Run(ctx, script string, args ...string) (stdout, stderr []byte, exitCode int, err error)`; cwd = vault root; env limited to `PATH, HOME, LANG`; per-call timeout.
- [x] 2b.5 RED `longterm-mem/internal/vault/retrieve_test.go::TestRetrieve_DefaultTopNAndFullFieldParse` — fixture `retrieve.py` stdout with N rows; assert `Retrieve` invokes with the default top-N (5) and parses page address, absolute path, BM25 score, rerank score, and snippet for every row. Scenario: "Default top-N and full field parse" (R-004).
- [x] 2b.6 RED (same file) `TestRetrieve_ExplicitTopNOverride` — explicit top-N passed through as the `--top` argv value. Scenario: "Explicit top-N override" (R-004).
- [x] 2b.7 RED (same file) `TestRetrieve_NotProvisionedExitTenMapsToStatus` — fixture script exits 10; assert `Retrieve` returns `vault_status: not_provisioned` rather than a generic error. Scenario: "Never-indexed vault maps to not_provisioned, not a generic error" (R-024).
- [x] 2b.8 GREEN `longterm-mem/internal/vault/retrieve.go` — `Retrieve(ctx, runner, project, query string, top int) (Result, error)` invoking `python3 scripts/retrieve.py "<q>" --top N`, parsing the five fields, mapping exit 10 → `not_provisioned`.
- [x] 2b.9 REFACTOR — factor the exit-code-to-status mapping into a shared `vault` package function reused by the index path in slice 3a.
- [x] 2b.10 Slice verification (2a+2b) — `cd longterm-mem && go test ./...`; re-run `TestOSExecImportAllowlist` (1.4) to confirm only `runner.go` imports `os/exec` (R-021 re-verified).
- Command: `cd longterm-mem && go test ./... -run TestRunner|TestRetrieve`

---

## Slice 3a — longterm-mem-query-3a-index (R-005, R-025) — PR3a (base: PR2b)

- [x] 3a.1 RED `longterm-mem/internal/vault/index_test.go::TestIndex_AlreadyProvisionedRefresh` — fixture vault already provisioned (`.vault-meta/bm25/index.json` present, non-empty `.vault-meta/chunks/`), new/changed pages; assert `Rebuild` runs `contextual-prefix.py --all --no-llm` then `bm25-index.py build` and reports success. Scenario: "Already-provisioned vault refresh" (R-005).
- [x] 3a.2 RED (same file) `TestIndex_NeverIndexedIsProvisionedFirst` — no `.vault-meta/bm25/index.json`; assert `setup-retrieve.sh --no-llm` runs first, then the refresh, and the vault is queryable afterward. Scenario: "Never-indexed vault is provisioned first" (R-005).
- [x] 3a.3 RED (same file) `TestIndex_RebuildStepFailureReportsFailure` — fixture `bm25-index.py` forced to exit non-zero (`FAKE_RC`); assert an error is returned and no "rebuilt" wording appears (exit 5). Scenario: "Failing rebuild step reports failure, not false success" (R-025).
- [x] 3a.4 GREEN `longterm-mem/internal/vault/index.go` — `Provisioned(vaultRoot string) bool`; `Rebuild(ctx, runner) error` implementing provision-then-refresh, always `--no-llm` (D12), non-zero rc → wrapped error.
- [x] 3a.5 GREEN `longterm-mem/cmd/longterm-mem/cmd_index.go` — `index --project P [--vault DIR] [--rebuild]` wiring `vaultreg.Resolve` → `vault.Rebuild`; exit 5 on subprocess failure.
- Command: `cd longterm-mem && go test ./... -run TestIndex`

---

## Slice 3b — longterm-mem-query-3b-merge (R-006, R-026) — PR3b (base: PR3a)

- [x] 3b.1 RED `longterm-mem/internal/query/query_test.go::TestQuery_GroupedBySourceInNativeRankOrder` — fake vault + Engram rows; assert output is vault rows (vault order) then Engram rows (Engram order), each tagged `source`. Scenario: "Results are grouped by source in native rank order" (R-006).
- [x] 3b.2 RED (same file) `TestQuery_LinkedPairEmittedOnce` — a vault row and an Engram row sharing a promotion link; assert one `source:"linked"` row carrying both references. Scenario: "Linked pair is emitted once" (R-006).
- [x] 3b.3 RED (same file) `TestQuery_MissingProjectRejected` — empty `Request.Project`; assert rejection (exit 2), no inferred project. Scenario: "Missing project argument is rejected" (R-006).
- [x] 3b.4 RED (same file) `TestQuery_NotProvisionedDegradesToEngramOnly` — vault retrieve reports `not_provisioned`; assert Engram-only results plus `vault_status: not_provisioned`, no error. Scenario: "Unprovisioned vault degrades instead of failing the whole call" (R-026).
- [x] 3b.5 GREEN `longterm-mem/internal/engram/search.go` — `Search(project, query string, limit int) ([]Row, error)` via `observations_fts MATCH` (tokens double-quoted, AND), `project=? AND deleted_at IS NULL`, `ORDER BY rank LIMIT top`.
- [x] 3b.6 GREEN `longterm-mem/internal/query/query.go` — `Run(ctx, Deps, Request{Project, Query, Top}) (Result, error)` implementing D8's merge/degrade/linked-pair rules.
- [x] 3b.7 GREEN `longterm-mem/cmd/longterm-mem/cmd_query.go` — `query --project P "<text>" [--top N] [--vault DIR] [--json]` wiring.
- [x] 3b.8 REFACTOR — extract the linked-pair matcher (precedence-store `engram_id` lookup) for unchanged reuse by promote (slice 4+) and MCP `query` (slice 8b).
- [x] 3b.9 Slice verification (3a+3b) — `cd longterm-mem && go test ./...`.
- Command: `cd longterm-mem && go test ./... -run TestQuery`

---

## Slice 4 — longterm-mem-promotion-writer (R-007, R-027) — PR4 (base: PR3b)

- [x] 4.1 RED `longterm-mem/internal/promote/eligibility_test.go::TestEligible` (table-driven) — pinned→eligible; type `discovery`/rev 4/unpinned→eligible; type `discovery`/rev 1/unpinned/not explicit→not eligible; explicit promote call overrides type/pin/revision. Scenarios: "Pinned observation is eligible", "High-revision, untyped, unpinned observation is eligible", "Low-revision, untyped, unpinned observation is not eligible", "Explicit promote call overrides the automatic criteria" (R-007).
- [x] 4.2 GREEN `longterm-mem/internal/promote/eligibility.go` — `Eligible(obs engram.Observation, explicit bool) bool`.
- [x] 4.3 RED `longterm-mem/internal/engram/relations_test.go::TestRelatedEdges_AcceptedOnly` — fixture edges; assert only `judgment_status='judged'`, `superseded_at IS NULL`, relation ∈ {related, compatible, scoped, supersedes, conflicts_with} are returned.
- [x] 4.4 GREEN `longterm-mem/internal/engram/relations.go` — `RelatedEdges(observationID int) ([]Edge, error)` (D7 filter).
- [x] 4.5 RED `longterm-mem/internal/promote/page_test.go::TestEmitPage_TypeMappedOntoVaultEnum` — eligible `decision`-typed observation; assert frontmatter `type` is inside the vault's own contract enum (`concept`), with `engram_type`, `engram_id`, `project` as flat extras. Scenario: "Type is mapped onto the vault's contract enum" (R-027).
- [x] 4.6 RED (same file) `TestEmitPage_RelatedLinksResolve` — one relation edge to an already-promoted observation; assert `related` includes a link resolving to an existing file. Scenario: "Related links resolve" (R-027).
- [x] 4.7 RED (same file) `TestEmitPage_FilenameSurvivesRetitle` — address-derived filename; retitle in Engram; assert filename unchanged. Scenario: "Filename survives a retitle" (R-027).
- [x] 4.8 RED `longterm-mem/internal/promote/lint_test.go::TestLintPage_FreshlyPromotedPagePasses` — freshly emitted page through `LintPage`; assert pass. Scenario: "Freshly promoted page passes the vault's own lint" (R-027).
- [x] 4.9 GREEN `longterm-mem/internal/promote/frontmatter.go` — flat-YAML emitter, fixed field order (`type, title, address, aliases, created, updated, tags, status, related, sources, engram_id, engram_sync_id, engram_type, engram_revision, project`); golden files `testdata/pages/*.golden.md` (`-update` flag, go-testing pattern).
- [x] 4.10 GREEN `longterm-mem/internal/promote/page.go` — `EmitPage(obs, address string, related []Link) (Page, error)`: frontmatter + `# Title` (unless body already starts with H1) + body + footer line naming Engram id/revision.
- [x] 4.11 GREEN `longterm-mem/internal/promote/lint.go` — `LintPage(page) []Diagnostic`, kept to exactly 6 rules (required fields, enum, `^c-\d{6}$`, address_map consistency, wikilink resolvability, inbound `index.md` link) per the design's line-budget mitigation for this slice.
- [x] 4.12 REFACTOR — extract the wikilink-alias formatter (`[[c-NNNNNN|Title]]`) shared by `related` emission and `LintPage`'s resolvability check.
- [x] 4.13 Slice verification — `cd longterm-mem && go test ./...`; confirm authored diff stays ≤400 lines (see forecast note); if it trends over, split `lint.go` + its goldens into a follow-on `4b` PR based on this branch.
- Command: `cd longterm-mem && go test ./... -run TestEligible|TestEmitPage|TestLintPage|TestRelatedEdges`

---

## Slice 5 — longterm-mem-promotion-registration (R-028, R-029) — PR5 (base: PR4)

- [x] 5.1 RED `longterm-mem/internal/promote/address_test.go::TestAllocate_FirstPromotionAllocatesNewAddress` — newly promoted observation; assert a new address is allocated and referenced in `.raw/.manifest.json` `address_map`. Scenario: "First promotion allocates a new address" (R-028).
- [x] 5.2 RED (same file) `TestAllocate_RePromotionReusesExistingAddress` — re-promotion of an already-addressed observation; assert no new allocation, existing reused. Scenario: "Re-promotion reuses the existing address" (R-028).
- [x] 5.3 GREEN `longterm-mem/internal/promote/address.go` — `Allocate(vaultRoot, project string, engramID int) (address string, err error)` invoking `scripts/allocate-address.sh` (flock-safe, via `internal/vault.Runner`) plus `.raw/.manifest.json` `address_map[path]` mutation (decode → mutate → 2-space encode; `sources` untouched).
- [x] 5.4 RED `longterm-mem/internal/promote/register_test.go::TestRegister_NewPageDiscoverableAndLogged` — newly promoted page; assert the vault's master catalog lists it and the append-only log records the promotion event. Scenario: "New page is discoverable and logged" (R-029).
- [x] 5.5 GREEN `longterm-mem/internal/promote/register.go` — `RegisterIndex(indexMdPath, addr, title string) error` (idempotent marker-block append/replace, sorted by address); `RegisterLog(logMdPath, addr, title string, at time.Time) error` (newest-first insert before the first `^## \[` line).
- [x] 5.6 REFACTOR — extract the idempotent marker-block writer so `sync` (slice 7) invokes it once per run rather than per-page.
- [x] 5.7 Slice verification — `cd longterm-mem && go test ./...`.
- Command: `cd longterm-mem && go test ./... -run TestAllocate|TestRegister`

---

## Slice 6 — longterm-mem-promotion-mutability (R-008, R-030) — PR6 (base: PR5)

- [x] 6.1 RED `longterm-mem/internal/promote/store_test.go::TestPrecedenceStore_LoadSaveRoundTrip` — `.raw/.longterm-mem-manifest.json` keyed by `page_address`, storing `body_hash`+`frontmatter_hash` separately (D6), tmp+fsync+rename.
- [x] 6.2 GREEN `longterm-mem/internal/promote/store.go` — `PrecedenceStore` load/save, `Get(address)`, `Set(address, entry)`.
- [x] 6.3 RED `longterm-mem/internal/promote/update_test.go::TestUpdate_UnmodifiedPageUpdatesInPlace` — observation X previously promoted to page V, not locally modified; revision increases; assert V's content and updated timestamp refresh, no second page. Scenario: "Unmodified page updates in place on revision" (R-008).
- [x] 6.4 RED (same file) `TestUpdate_RetitleKeepsSameFile` — X's title changed, id unchanged; assert same on-disk page updated, no new file, no orphan. Scenario: "Retitle keeps the same file" (R-008).
- [x] 6.5 RED (same file) `TestUpdate_LocallyEditedPageSkippedWithDiagnostic` — on-disk content diverges from stored `body_hash`/`frontmatter_hash`; assert content update skipped, a diagnostic names the page. Scenario: "Locally edited page is skipped with a diagnostic" (R-030).
- [x] 6.6 RED (same file) `TestUpdate_UnmodifiedPageUpdatesNormally` — page not locally modified since last write; assert normal update-in-place, no skip. Scenario: "Unmodified page updates normally" (R-030).
- [x] 6.7 GREEN `longterm-mem/internal/promote/update.go` — `UpdateInPlace(store, page, existingPath) (action Action, err error)` implementing hash-divergence detection vs. update-in-place.
- [x] 6.8 REFACTOR — consolidate `EmitPage` (create) and `UpdateInPlace` (update) behind one `promote.Writer.Promote(obs, explicit bool) (Result, error)` entrypoint reused by `sync` (slice 7) and the explicit promote surface (slice 8b).
- [x] 6.9 Slice verification — `cd longterm-mem && go test ./...`.
- Command: `cd longterm-mem && go test ./... -run TestPrecedenceStore|TestUpdate`

---

## Slice 7 — longterm-mem-promotion-sync (R-009, R-031, R-033) — PR7 (base: PR6)

- [x] 7.1 RED `longterm-mem/internal/promote/sync_test.go::TestSync` (table-driven, 3 cases) — never-promoted eligible observation is promoted; already-promoted-at-rev-2-now-rev-3 is re-promoted; already-promoted-at-current-revision is a no-op. Scenarios: "Never-promoted eligible observation is promoted", "Revised eligible observation is re-promoted", "Unchanged eligible observation is a no-op" (R-009).
- [x] 7.2 GREEN `longterm-mem/internal/promote/sync.go` — `Sync(ctx, Deps, project string) (SyncReport, error)` iterating eligible observations, calling `Writer.Promote` only for unpromoted-or-revised ones.
- [x] 7.3 RED (same file) `TestSync_IndexAndSyncStateReflectCompletion` — sync run promoting three observations; assert the vault index reflects the new pages and `.vault-meta/longterm-mem-sync-state.json` carries the completion timestamp. Scenario: "Index and sync-state both reflect the completed sync" (R-031).
- [x] 7.4 GREEN — wire `Sync` to call `vault.Rebuild` (3a.4) after promotion and write the sync-state record (tmp+fsync+rename).
- [x] 7.5 RED `longterm-mem/internal/promote/propagate_test.go::TestPropagate` (table-driven, 4 cases) — supersession → `status: superseded` + `related` to successor, body unchanged; soft-delete with no successor → `status: archived`; untouched observation keeps its status; a status patch on a locally edited page still lands, body not rewritten, and the precedence store's `frontmatter_hash` updates so a later sync does not misread the patch as a human edit. Scenarios: "Supersession updates status and related, body untouched", "Soft-delete with no successor archives the page", "Untouched observation keeps its status", "Status patch on a locally edited page still lands (canon wins)" (R-033).
- [x] 7.6 GREEN `longterm-mem/internal/promote/propagate.go` — `Propagate(ctx, Deps, project string) (PropagateReport, error)`: D11 successor rule (newer observation by `created_at` wins on a `supersedes` edge, independent of edge direction); soft-deleted with no accepted `supersedes` edge → `archived`; frontmatter-only patch of `status:`/`related:`, body never rewritten, `frontmatter_hash` updated (`body_hash` untouched). Deviation: `superseded_by:` was not implemented — see apply-progress.md.
- [x] 7.7 GREEN `longterm-mem/cmd/longterm-mem/cmd_sync.go` — `sync --project P [--vault DIR]` wiring `Sync` → `Propagate` → index rebuild → sync-state write.
- [x] 7.8 REFACTOR — moved the frontmatter-only line patcher into `frontmatter.go` (slice 4) as `PatchStatusFields`, so it is one shared editor. Landed alongside 7.6 (propagate.go depends on it directly), not deferred after 7.7 as originally sequenced.
- [x] 7.9 Slice verification — `cd longterm-mem && go test ./...`.
- Command: `cd longterm-mem && go test ./... -run TestSync|TestPropagate`
- [x] 7.10 RED/GREEN `longterm-mem/internal/promote/writer.go` + `writer_test.go` — wire `RegisterIndex`/`RegisterLog` (register.go, Slice 5) into `Writer.Promote` so R-029 ("register it in both the vault's master catalog and its append-only promotion log") is satisfied end to end: a promotion that actually writes a page (create or update) registers it; a skip (`ActionSkippedLocalEdit`) or an ineligible observation registers nothing. **This task did not exist in the original task list** — 7.1–7.9 never mention wiring `RegisterIndex`/`RegisterLog` in, and Slice 7's own apply-progress.md explicitly flagged the gap ("`RegisterIndex`/`RegisterLog` (Slice 5) remain unwired into `Writer.Promote`/`Sync`") without a task to close it. Added retroactively because R-029 is unmet without it. Scenario: "New page is discoverable and logged" (R-029).

---

## Slice 8a — longterm-mem-ops-8a-status-doctor (R-010, R-011) — PR8a (base: PR7)

- [x] 8a.1 RED `longterm-mem/internal/ops/status_test.go::TestStatus` (table-driven, 3 cases) — Engram reachable + vault provisioned + prior sync recorded → all healthy plus timestamp; never-provisioned vault → reported, not an error; never-synced project → "never", not a fabricated timestamp. Scenarios: "Healthy status reports all three fields", "Never-provisioned vault is reported, not an error", "Never-synced project reports never, not a fabricated timestamp" (R-010).
- [x] 8a.2 GREEN `longterm-mem/internal/ops/status.go` — `Status(ctx, Deps, project string) (Report, error)` composing Engram reachability, `vault.Provisioned`, and the sync-state file's `last_sync_completed_at`. Implemented as `StatusDeps`/`StatusReport` (see apply-progress.md for why the generic `Deps`/`Report` wording resolved to distinct types).
- [x] 8a.3 GREEN `longterm-mem/cmd/longterm-mem/cmd_status.go` — `status --project P` wiring.
- [x] 8a.4 RED `longterm-mem/internal/ops/doctor_test.go::TestDoctor` (table-driven, 4 named checks) — unresolvable vault-registry path → vault-config-resolvable check names it; promoted page missing its address-map entry → address-map-integrity check names it; promoted page absent from catalog/log → wiki-registration-consistency check names it; missing runtime prerequisite → runtime-prerequisites check reports missing rather than a later generic failure. Scenarios: "Unresolvable vault config is named", "Corrupted address-map entry is named", "Unregistered promoted page is named", "Missing runtime prerequisite is named" (R-011).
- [x] 8a.5 GREEN `longterm-mem/internal/ops/doctor.go` — `Doctor(ctx, Deps, project string) (Report, error)` running the four read-only checks independently (address-map/registration checks reuse `promote.LintPage`'s rules, slice 4); exit 1 if any FAILs. Implemented as `DoctorDeps`/`DoctorReport`; runtime-prerequisites check wired to new `vault.PrerequisitePresent` (runner.go, the sole `os/exec` importer, R-021).
- [x] 8a.6 GREEN `longterm-mem/cmd/longterm-mem/cmd_doctor.go` — `doctor --project P` wiring.
- [x] 8a.7 REFACTOR — extract the shared registry/vault/precedence-store/catalog fixture builder into `internal/ops/testdata` helpers reused by both test files.
- [x] 8a.8 Slice verification — `cd longterm-mem && go test ./...`.
- Command: `cd longterm-mem && go test ./... -run TestStatus|TestDoctor`

---

## Slice 8b — longterm-mem-ops-8b-mcp-promote (R-012, R-032, R-034) — PR8b (base: PR8a)

- [x] 8b.1 RED `longterm-mem/internal/mcpserver/server_test.go::TestServer_ToolListingListsQueryAndPromote` — `mcp.NewInMemoryTransports()` client tool-listing handshake; assert both `query` and `promote` are listed. Scenario: "Tool-listing handshake lists both tools" (R-012).
- [x] 8b.2 RED (same file) `TestServer_QueryRoundTripsOverStdio` — connected client calls `query` with a valid project/query string; assert the grouped result list returns over the same connection. Scenario: "Query round-trips over stdio" (R-012).
- [x] 8b.3 GREEN `longterm-mem/internal/mcpserver/server.go` — `mcp.NewServer` + `mcp.AddTool[QueryIn,QueryOut]` / `AddTool[PromoteIn,PromoteOut]` (D3), tools wired to `query.Run` and `promote.Writer.Promote`. Implemented via `Deps.Query`/`Deps.Promote` function seams (matching `query.Deps`/`promote.Deps`'s own convention) so both tools were wired from this GREEN step; production wiring to the real `Writer.Promote` call lands in 8b.10/8b.11's `cmd_mcp.go` — see apply-progress.md.
- [x] 8b.4 RED `longterm-mem/internal/promote/eligible_test.go::TestPromote_ExplicitCallOverridesAutomaticEligibility` (extends 4.1's file; the task's stated filename `eligibility_test.go` does not exist in this codebase — 4.1 itself landed as `eligible.go`/`eligible_test.go`, see apply-progress.md) — observation not pinned/not eligible type/below revision threshold, named by an explicit promote call; assert it is promoted through the same page-emission/addressing/registration path as any other eligible observation. Scenario: "Below-threshold observation is promoted via explicit call" (R-032).
- [x] 8b.5 RED (same file) `TestPromote_InvalidObservationIdRejected` — invalid/nonexistent id; assert rejection with a clear error (exit 7 `not_found`), not a silent no-op. Scenario: "Invalid observation id is rejected" (R-032).
- [x] 8b.6 GREEN `longterm-mem/cmd/longterm-mem/cmd_promote.go` — `promote --project P --id <engram_id> [--vault DIR]` calling `promote.ExplicitPromote` (new `internal/promote/explicit.go`, an id-lookup wrapper around `Writer.Promote(obs, explicit=true)`) via `engram.Store.ObservationByID` (new); exit 7 on invalid id.
- [x] 8b.7 GREEN — wire the MCP `promote` tool to the same `Writer.Promote` call as 8b.6 so R-012 and R-032 share one code path. Realized through `mcpserver.Deps.Promote` calling `cmd_mcp.go`'s `runPromote` helper (rundeps.go, 8b.11), the same function `cmdPromote` calls.
- [x] 8b.8 RED `longterm-mem/internal/mcpserver/server_test.go::TestServer_ExitsWhenStdinCloses` (`-short`-skippable integration) — built binary `mcp` subprocess; close stdin; assert the process exits and no residual child remains (`cmd.Wait` + empty `pgrep -P`). Scenario: "MCP server exits with its session" (R-034).
- [x] 8b.9 RED `longterm-mem/cmd/longterm-mem/main_test.go::TestCLI_NoResidualProcessAfterAnySubcommand` (`-short`-skippable integration) — run each CLI subcommand against a fixture project; assert the process list is unchanged afterward. Scenario: "No CLI subcommand leaves a residual process" (R-034).
- [x] 8b.10 GREEN — confirm `server.Run(ctx, &mcp.StdioTransport{})` returns on stdin EOF; `signal.NotifyContext(os.Interrupt, syscall.SIGTERM)` cancels in-flight subprocess contexts (D3); no subcommand spawns anything outside a bounded, awaited `Runner` call. New `cmd_mcp.go` + `mcp` dispatch case in `main.go`.
- [x] 8b.11 REFACTOR — collapse `cmd_query.go`/`cmd_promote.go` request construction and the MCP tool handlers onto one shared call path so the CLI and MCP surfaces cannot drift. New `cmd/longterm-mem/rundeps.go` (`runQuery`, `runPromote`); `cmd_query.go`/`cmd_promote.go`/`cmd_mcp.go` all call these two functions instead of separately reconstructing `query.Deps`/`promote.Writer`.
- [x] 8b.12 Slice verification (8a+8b) — `cd longterm-mem && go test ./... -short`, then once more without `-short` for the R-034 integration scenarios. Both pass; see apply-progress.md for exact commands/output and coverage.
- Command: `cd longterm-mem && go test ./... -run TestServer|TestPromote|TestCLI_NoResidualProcess`

---

## Slice 9 — longterm-mem-overlay-route (R-013, R-035) — PR9 (base: PR8b)

Note: this slice touches only `engine/` and `bin/labdrian-overlay`, not `longterm-mem/`; its meaningful verification commands are the ones listed per item, not the module-focused command (which would pass vacuously).

- [x] 9.1 RED `engine/skills/ondisk_test.go::TestDeployableManifestPaths_ExcludesMcpRoute` — manifest row `longterm-mem/go.mod  custom  mcp`; assert `DeployableManifestPaths` excludes it. Scenarios: "mcp-routed row is not required to exist under the skills directory", "mcp-routed row does not falsely register as an unregistered skill file" (skills-ondisk-validation R-012, traces R-013).
- [x] 9.2 GREEN `engine/skills/ondisk.go` — add `"mcp": true` to `nonSkillRoutes`; update the doc comment to name the four-value domain.
- Command: `cd engine && go test ./... -run TestDeployableManifestPaths`
- [x] 9.3 RED `engine/installer/route_test.go::TestRouteResolve_McpRow` — fixture manifest row with route `mcp`; assert bash `route_resolve` emits `route=mcp` with the longterm-mem repo source path and zero copy targets. Scenario: "Bash dispatch recognizes the mcp route" / "Go route handling recognizes the mcp route" (overlay-agent-route R-006, traces R-013).
- [x] 9.4 RED (same file) `TestRouteResolve_OpencodeAgentUnaffected` — regression: an existing `opencode-agent`-routed row still resolves/deploys exactly as before. Scenario: "opencode-agent route is unaffected".
- [x] 9.5 RED (same file) `TestRouteResolve_UnroutedLongtermMemRowRejected` + `TestRouteResolve_UnrecognizedRouteLongtermMemRowRejected` — a `longterm-mem/**` row with a missing third column, and one with an unrecognized route value; assert `route_resolve` rejects both (exit 1, explicit stderr) instead of silently falling through to the skill route. Scenario: "Unrouted longterm-mem row is rejected by both parsers" (overlay-agent-route R-012, traces R-035).
- [x] 9.6 GREEN `bin/labdrian-overlay` — `route_resolve()`: new `mcp` branch (emits `route=mcp`, repo source, zero targets) alongside `agent`/`opencode-agent`; a guard ahead of the `skill` fallthrough that rejects any `longterm-mem/**` row whose third column is absent or not in `{skill,agent,opencode-agent,mcp}` (`exit 1`, explicit stderr naming the row). Implemented directly as the `route_reject_unrouted_longterm_mem()` helper (9.11's target shape) rather than as an inline guard later hoisted out — see apply-progress.md.
- [x] 9.7 GREEN `bin/labdrian-overlay` — `route_repo_rel()`: add `mcp) printf '%s' "$manifest_path"` case.
- Command: `cd engine && go test ./... -run TestRouteResolve`
- [x] 9.8 RED `engine/skills/ondisk_test.go::TestRouteDomain_MatchesBashAndGo` — parity fixture asserting both bash `route_resolve` and Go `nonSkillRoutes` recognize the identical four-value set `{skill, agent, opencode-agent, mcp}`.
- [x] 9.9 GREEN — no further production code beyond 9.2/9.6/9.7; this closes the parity assertion.
- Command: `cd engine && go test ./... -run TestRouteDomain`
- [x] 9.10 Add the manifest sentinel row `longterm-mem/go.mod  custom  mcp` to `overlay.manifest` (D13).
- [x] 9.11 REFACTOR — hoist the `longterm-mem/**`-prefix rejection guard from 9.6 into a `route_reject_unrouted_longterm_mem()` helper so 10b's `cmd_apply` hook can reuse it. Landed as part of 9.6's GREEN commit (the guard was written as its own function from the start); no separate refactor diff was needed — see apply-progress.md.
- [x] 9.12 Slice verification — `bash -n bin/labdrian-overlay`; `cd engine && go test ./...`; `cd longterm-mem && go test ./...` (unaffected, must stay green).

---

## Slice 10a — longterm-mem-overlay-dispatch-10a-engine (R-014 engine half) — PR10a (base: PR9)

- [ ] 10a.1 RED `engine/runtime/longtermmem_test.go::TestLongtermMemAdapter_StatusMatrix` (table-driven) — supported (binary executable ∧ install-state record ∧ entry present with matching fingerprint); partial ×4 named reasons (record without entry, entry without record/unmanaged, fingerprint drift, missing binary); unsupported (config root unresolvable).
- [ ] 10a.2 RED (same file) `TestLongtermMemAdapter_InstallRecordsRegistrationAndReportsStatus` — GIVEN the binary already built/copied, WHEN `Install()` runs, THEN `registration.json` is written and a per-runtime status is reported for claude/opencode/codex. Scenario: "Install records registration and reports per-runtime status" (runtime-lifecycle ADDED requirement, R-014).
- [ ] 10a.3 RED (same file) `TestLongtermMemAdapter_StatusAndUninstallRequireNoBuild` — `Status()`/`Uninstall()` read/remove the registration directly, no build step. Scenario: "Status and uninstall report without requiring a build".
- [ ] 10a.4 RED (same file) `TestLongtermMemAdapter_UpdateAndRollbackRefused` — `Update()`/`Rollback()` return a refusal rather than attempting an action. Scenario: "No update or rollback surface is offered".
- [ ] 10a.5 GREEN `engine/runtime/longtermmem.go` — `LongtermMemAdapter` implementing `runtime.Adapter` (D4): stdlib-only, read-only config inspection (JSON decode for claude/opencode, TOML header/`command =` line scan for codex); `registration.json` record read/write; the status vocabulary above; `Update()`/`Rollback()` return `CapabilityUnsupported` with an explicit reason.
- [ ] 10a.6 RED `engine/cmd/main_test.go::TestComponentFlag_LongtermMemRefusesUpdateRollback` — `--component longterm-mem update` / `... rollback` parsed at the CLI layer; assert exit 1 before any adapter call (D4 parse-time refusal).
- [ ] 10a.7 RED (same file) `TestComponentFlag_DefaultIsRuntimeParity` — no `--component` flag behaves exactly as today (regression guard).
- [ ] 10a.8 GREEN `engine/cmd/main.go` — `--component {runtime-parity|longterm-mem}` and `--state-dir` flags; component dispatch selecting `LongtermMemAdapter` vs. the existing parity adapters; usage text update.
- [ ] 10a.9 REFACTOR — factor shared claude/opencode/codex config-inspection primitives between `LongtermMemAdapter` and the existing parity adapters where the schema overlaps.
- [ ] 10a.10 Slice verification — `cd engine && go test ./...`.
- Command: `cd engine && go test ./... -run TestLongtermMemAdapter|TestComponentFlag`

---

## Slice 10b — longterm-mem-overlay-dispatch-10b-shell (R-014 shell half, R-015) — PR10b (base: PR10a)

- [ ] 10b.1 RED `engine/installer/route_test.go::TestInstall_BuildsCopiesThenReportsPerRuntimeStatus` (integration, `-short`-skippable, sandbox HOME/OVERLAY_DIR/STATE_DIR) — `longterm-mem install --target all`; assert a binary exists at `$STATE_DIR/bin/longterm-mem` afterward and a per-runtime status is reported for claude/opencode/codex. Scenario: "Install builds, copies, then reports per-runtime status" (install spec R-014).
- [ ] 10b.2 RED (same file) `TestStatusUninstall_SkipBuildStep` — `longterm-mem status` / `longterm-mem uninstall`; assert no build invocation occurs. Scenario: "Status and uninstall skip the build step" (R-014).
- [ ] 10b.3 RED (same file) `TestInstall_BinaryPersistsAfterProcessExits` — fresh install; installing process exits; assert the binary still exists at the documented fixed path and is invocable. Scenario: "Binary persists after the installing process exits" (R-015).
- [ ] 10b.4 RED (same file) `TestInstall_BinaryPathStableAcrossInspections` — no install/uninstall in progress; inspect the path at two points; assert unchanged. Scenario: "Binary path is stable absent install/uninstall activity" (R-015).
- [ ] 10b.5 GREEN `bin/labdrian-overlay` — `cmd_longterm_mem()` dispatcher: `install|status|uninstall [--target claude|opencode|codex|all] [--purge]`; `install` = `go build ./longterm-mem/cmd/longterm-mem` → copy to `LONGTERM_MEM_BINARY="$STATE_DIR/bin/longterm-mem"` → `longterm-mem vaults seed` → per target (`longterm-mem register --target t` then `engine runtime install --component longterm-mem --target t`); `status`/`uninstall` skip the build step; binary removed only when `--target all` leaves zero install-state targets, or `--purge`.
- [ ] 10b.6 GREEN `bin/labdrian-overlay` — usage text + top-level dispatcher case for `longterm-mem`; `LONGTERM_MEM_BINARY="$STATE_DIR/bin/longterm-mem"` constant.
- [ ] 10b.7 GREEN `bin/labdrian-overlay` — `cmd_apply()` hook: after the existing copy loop, any manifest row whose `route_resolve` result is `mcp` sets a flag; `cmd_apply` calls `cmd_longterm_mem install --target "$TARGET"` once, after the copy loop (D13), reusing `route_reject_unrouted_longterm_mem()` (9.11) for the same-pass rejection.
- [ ] 10b.8 REFACTOR `longterm-mem/README.md` — refresh (CHK-05): per-project wording, fixed install path, full CLI/MCP surface now that `install`/`status`/`uninstall` exist end-to-end.
- [ ] 10b.9 Slice verification (10a+10b) — `cd engine && go test ./... && cd tui && go test ./... && bash -n bin/labdrian-overlay && bash -n bin/overlay`; `cd longterm-mem && go test ./...`.
- Command: `cd engine && go test ./... -run TestInstall|TestStatusUninstall`

---

## Slice 11a — longterm-mem-mcp-registration-json-11a-splice-install-state (R-016 shared, R-017 shared) — PR11a (base: PR10b)

- [ ] 11a.1 RED `longterm-mem/internal/register/jsonsplice_test.go::TestJSONSplice_LocatesAndReplacesMemberSpan` — fixture JSON with an existing `mcpServers.longterm-mem`-shaped member among unrelated members; assert the splice locates the exact byte span via `json.Decoder.InputOffset()` and replaces only it, every other byte identical.
- [ ] 11a.2 RED (same file) `TestJSONSplice_InsertsWhenAbsent` — no existing member; assert insertion, `json.Valid` afterward.
- [ ] 11a.3 GREEN `longterm-mem/internal/register/jsonsplice.go` — JSON member byte-splice editor (D9): locate/insert/replace span, `.bak` write, tmp+rename in the same dir, `json.Valid` post-write validation before rename.
- [ ] 11a.4 RED `longterm-mem/internal/register/installstate_test.go::TestInstallState_FingerprintRoundTrip` — write `fingerprint = sha256(entry bytes)` for a target, reload, assert match; assert the tag never appears as an unknown key inside the runtime's own config schema.
- [ ] 11a.5 GREEN `longterm-mem/internal/register/installstate.go` — `install-state.json` schema, load/save (tmp+fsync+rename), `Get(target)`/`Set(target, record)`.
- [ ] 11a.6 REFACTOR — extract `register.Decide(entryPresent, recordPresent, fingerprintMatches bool) Action{insert|replace|refuse|noop}` (D9 semantics table) as a pure function shared by every runtime writer in 11b/12a.
- [ ] 11a.7 Slice verification — `cd longterm-mem && go test ./...`.
- Command: `cd longterm-mem && go test ./... -run TestJSONSplice|TestInstallState`

---

## Slice 11b — longterm-mem-mcp-registration-json-11b-writers (R-016, R-017) — PR11b (base: PR11a)

- [ ] 11b.1 RED `longterm-mem/internal/register/claude_test.go::TestClaude_UnrelatedEntriesPreserved` — golden fixture `~/.claude.json` with unrelated `mcpServers` entries; assert install adds only a new ownership-tagged `longterm-mem` entry, all pre-existing entries byte-identical. Scenario: "Unrelated entries are preserved" (R-016).
- [ ] 11b.2 RED (same file) `TestClaude_ReinstallIsIdempotent` — existing tagged entry; re-install; assert in-place replace, not a duplicate. Scenario: "Reinstall is idempotent" (R-016).
- [ ] 11b.3 RED (same file) `TestClaude_UntaggedSameNamedEntryRefused` — an untagged `longterm-mem`-named entry install-state does not own; assert refusal + reported conflict (exit 6), nothing written. Scenario: "Untagged same-named entry is refused, not overwritten" (R-016).
- [ ] 11b.4 GREEN `longterm-mem/internal/register/claude.go` — `RegisterClaude(configRoot, binary string) error` writing `"longterm-mem": {"type":"stdio","command":"<bin>","args":["mcp"]}` under `mcpServers` via `jsonsplice.go` + `installstate.go`; golden fixtures `testdata/claude/*`.
- [ ] 11b.5 RED `longterm-mem/internal/register/opencode_test.go::TestOpencode_UnrelatedEntriesPreserved` / `TestOpencode_ReinstallIsIdempotent` / `TestOpencode_UntaggedSameNamedEntryRefused` — same three scenarios against opencode's `opencode.json` `mcp` map. Scenarios: "Unrelated entries are preserved", "Reinstall is idempotent", "Untagged same-named entry is refused, not overwritten" (R-017).
- [ ] 11b.6 GREEN `longterm-mem/internal/register/opencode.go` — `RegisterOpencode(configRoot, binary string) error` writing `"longterm-mem": {"type":"local","command":["<bin>","mcp"],"enabled":true}`; golden fixtures `testdata/opencode/*`.
- [ ] 11b.7 GREEN `longterm-mem/cmd/longterm-mem/cmd_register.go` — `register --target claude|opencode|codex|all [--config-root DIR] [--state-dir DIR] [--binary PATH]` wiring (codex target wired in 12a.6).
- [ ] 11b.8 REFACTOR — extract the golden-fixture harness (`testdata/<runtime>/before.json`, `after-install.json`, `after-reinstall.json`, `after-uninstall.json`) for reuse by codex in slice 12a.
- [ ] 11b.9 Slice verification — `cd longterm-mem && go test ./...`.
- Command: `cd longterm-mem && go test ./... -run TestClaude|TestOpencode`

---

## Slice 12a — longterm-mem-mcp-registration-toml-uninstall-12a-toml (R-018) — PR12a (base: PR11b)

- [ ] 12a.1 RED `longterm-mem/internal/register/tomlsplice_test.go::TestTOMLSplice_LocatesTableSpan` — fixture `config.toml` with an existing unrelated `[mcp_servers.other]` section and unrelated top-level keys; assert the header regex `^\s*\[mcp_servers\.("longterm-mem"|longterm-mem)\]` locates the span to the next `^\s*\[` or EOF, replaces only that span, all else unchanged, file stays valid via `go-toml/v2` parse-only validation.
- [ ] 12a.2 GREEN `longterm-mem/internal/register/tomlsplice.go` — TOML table byte-splice editor (D9), `.bak`, tmp+rename, post-write `pelletier/go-toml/v2 Unmarshal` + `mcp_servers.longterm-mem.command == binary` validation before rename.
- [ ] 12a.3 RED `longterm-mem/internal/register/codex_test.go::TestCodex_UnrelatedSectionsAndOrderingPreserved` — golden fixture; assert only a new `longterm-mem` section is added, all other sections/key ordering unchanged, file stays valid. Scenario: "Unrelated sections and ordering are preserved" (R-018).
- [ ] 12a.4 RED (same file) `TestCodex_ReinstallIsIdempotent` — existing tagged section; re-install; assert in-place replace. Scenario: "Reinstall is idempotent" (R-018).
- [ ] 12a.5 GREEN `longterm-mem/internal/register/codex.go` — `RegisterCodex(configRoot, binary string) error` writing `[mcp_servers.longterm-mem]\ncommand = "<bin>"\nargs = ["mcp"]` via `tomlsplice.go` + `installstate.go`; golden fixtures `testdata/codex/*`.
- [ ] 12a.6 GREEN — wire `codex` into `cmd_register.go` (11b.7) target dispatch.
- [ ] 12a.7 REFACTOR — confirm `internal/register` package doc names all three runtimes' semantics table (D9) in one place for `doctor`/`status` reuse.
- [ ] 12a.8 Slice verification — `cd longterm-mem && go test ./...`.
- Command: `cd longterm-mem && go test ./... -run TestTOMLSplice|TestCodex`

---

## Slice 12b — longterm-mem-mcp-registration-toml-uninstall-12b-uninstall (R-019) — PR12b (base: PR12a)

- [ ] 12b.1 RED `longterm-mem/internal/register/unregister_test.go::TestUnregister_SelectiveRemovalAcrossAllThreeRuntimes` — claude/opencode/codex configs each carry a longterm-mem entry plus unrelated entries; uninstall one runtime; assert only that runtime's entry is removed, all other entries in all three files untouched. Scenario: "Selective removal across all three runtimes" (R-019).
- [ ] 12b.2 RED (same file) `TestUnregister_UntaggedEntryPreservedAndReported` — untagged `longterm-mem`-named entry install-state does not own; uninstall; assert left in place, reported `unmanaged`, not removed. Scenario: "Untagged entry is preserved and reported, not removed" (R-019).
- [ ] 12b.3 RED (same file) `TestUnregister_PartialUninstallKeepsSharedBinary` — uninstall from only one of the three runtimes; assert the shared binary is not removed. Scenario: "Partial uninstall does not remove the shared binary" (R-019).
- [ ] 12b.4 GREEN `longterm-mem/internal/register/unregister.go` — `Unregister(target string, ...) error`: record present → remove span (and any dangling comma/blank line) via the same splice editors; no record → leave + report `unmanaged`; updates `install-state.json`.
- [ ] 12b.5 GREEN `longterm-mem/cmd/longterm-mem/cmd_unregister.go` — `unregister --target claude|opencode|codex|all` wiring.
- [ ] 12b.6 GREEN `bin/labdrian-overlay` — `cmd_longterm_mem uninstall`: `unregister` then `engine runtime uninstall --component longterm-mem`; `--purge` flag glue; binary removed only when `--target all` leaves zero install-state targets or `--purge`.
- [ ] 12b.7 REFACTOR — final pass so `claude.go`/`opencode.go`/`codex.go` share one `Decide` call site (11a.6) for both install and uninstall paths.
- [ ] 12b.8 Slice verification (12a+12b, full change) — `cd longterm-mem && go test ./...`; broad: `cd longterm-mem && go test ./... && cd engine && go test ./... && cd tui && go test ./... && bash -n bin/labdrian-overlay && bash -n bin/overlay`.
- Command: `cd longterm-mem && go test ./... -run TestUnregister`
