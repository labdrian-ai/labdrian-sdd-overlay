```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:c40a52783514fcd2c24ad987e88ae593c927252392fa89233f21a5215ed95928
verdict: fail
blockers: 2
critical_findings: 2
requirements: 33/35
scenarios: 80/82
test_command: cd longterm-mem && go test ./... && cd ../engine && go test ./... && cd ../tui && go test ./...
test_exit_code: 0
test_output_hash: sha256:6a74eff6b5131fbe814bc5c8664c405815462ca85dece1d86185d38bd3745faa
build_command: bash -n bin/labdrian-overlay && bash -n bin/overlay && cd longterm-mem && go vet ./... && gofmt -l .
build_exit_code: 0
build_output_hash: sha256:79749754995caa191076db349b5ce5dd0fa48aea4abe5dc05d9a07f391a5880c
```

## Verification Report

**Change**: longterm-mem
**Version**: 9 delta specs, R-001 through R-035
**Mode**: Strict TDD
**Artifact store**: openspec (`openspec/changes/longterm-mem/`)
**Worktree**: `/home/labdrian/labdrian-sdd-overlay-worktrees/longterm-mem`
**Branch**: `feat/lm/longterm-mem-unregister-12b8-record` @ `0ee595d`

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 168 |
| Tasks complete | 168 |
| Tasks incomplete | 0 |
| Requirements (delta specs) | 35 |
| Scenarios (delta specs) | 82 |
| Requirements fully satisfied | 33 |
| Scenarios compliant | 80 |

Native `sdd-status` reports `apply: all_done`, `taskProgress 168/168`, `verify: ready`.
Every task checkbox is ticked; two ticked tasks overstate what shipped (see CRITICAL-2
and WARNING-1).

### Build & Tests Execution

**Tests**: all green.

```text
cd longterm-mem && go test ./...                                   exit 0
  longterm-mem                    ok  0.004s
  longterm-mem/cmd/longterm-mem   ok  1.160s
  longterm-mem/internal/engram    ok  0.409s
  longterm-mem/internal/mcpserver ok  0.693s
  longterm-mem/internal/ops       ok  0.019s
  longterm-mem/internal/promote   ok  0.640s
  longterm-mem/internal/query     ok  0.176s
  longterm-mem/internal/register  ok  0.084s
  longterm-mem/internal/vault     ok  0.479s
  longterm-mem/internal/vaultreg  ok  (cached)

cd engine && go test ./...                                          exit 0
  assets, cmd, gadu, gate, installer (12.700s), prespec,
  propagator, runtime, settings, skills  -- all ok

cd tui && go test ./...                                             exit 0
  tui  ok  5.160s

bash -n bin/labdrian-overlay                                        exit 0
bash -n bin/overlay                                                 exit 0
```

**Static analysis**:

```text
cd longterm-mem && gofmt -l .    exit 0, no output (all files formatted)
cd longterm-mem && go vet ./...  exit 0, no diagnostics
```

**Standing import guards** (both executed individually, both PASS):

| Guard | Location | Result |
|---|---|---|
| `TestOSExecImportAllowlist` | `longterm-mem/exec_allowlist_test.go` | PASS -- only `internal/vault/runner.go` imports `os/exec` |
| `TestZeroFetchImportAllowlist` | `engine/skills/zero_fetch_test.go` | PASS -- `engine/go.mod` still declares zero third-party requires |

`longterm-mem/go.mod` carries `pelletier/go-toml/v2 v2.4.3` (slice 12a),
`modelcontextprotocol/go-sdk v1.7.0` and `modernc.org/sqlite v1.57.0`.
`engine/go.mod` remains `go 1.21` with no `require` block at all -- the
slice-12a dependency did not leak across the module boundary.

**Coverage** (per package, `go test -cover`):

| Package | Coverage | Rating |
|---|---|---|
| `internal/ops` | 89.0% | Excellent |
| `internal/query` | 85.1% | Excellent |
| `internal/promote` | 84.6% | Excellent |
| `internal/vault` | 84.1% | Excellent |
| `internal/engram` | 83.0% | Excellent |
| `internal/mcpserver` | 77.8% | Acceptable |
| `internal/register` | 77.3% | Acceptable |
| `internal/vaultreg` | 67.2% | Low |
| `cmd/longterm-mem` | 55.2% | Low |
| root `guard` package | no statements | n/a |

Average across measurable packages: 78.0%. The two low packages are the
effectful CLI shell and the registry loader; both have their pure logic
pinned separately (`register_paths.go` rules, `Resolve` precedence table).
Coverage is informational and does not gate this verdict.

### Spec Compliance Matrix

Every row below was checked against production code AND a test observed
passing at runtime. Test names are exact; sub-test names are the literal
spec scenario titles wherever the implementation used table-driven tests.

#### longterm-mem-memory-access (12 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-001 | Component builds as an independent module | `longterm-mem/go.mod`, `cmd/longterm-mem/main.go` | `main_test.go > TestMain_BuildsIndependentModule` | COMPLIANT |
| R-001 | engine/'s zero-dependency gate stays green | `engine/go.mod` (no require block) | `engine/skills/zero_fetch_test.go > TestZeroFetchImportAllowlist` | COMPLIANT |
| R-002 | Default connection is read-only | `internal/engram/store.go > Open`, `readOnlyDSN` | `store_test.go > TestOpen_DefaultIsReadOnly` | COMPLIANT |
| R-002 | Overridden connection stays read-only | same | `store_test.go > TestOpen_OverridePathStaysReadOnly` | COMPLIANT |
| R-020 | Soft-deleted and other-project observations excluded | `internal/engram/store.go > ListObservations` | `store_test.go > TestListObservations_ScopesProjectAndExcludesSoftDeleted` | COMPLIANT |
| R-021 | No subprocess call to Engram's CLI | `internal/vault/runner.go` (sole `os/exec` importer) | `exec_allowlist_test.go > TestOSExecImportAllowlist` | COMPLIANT |
| R-003 | Configured override is used | `internal/vaultreg/registry.go > Resolve` | `registry_test.go > TestResolve_ConfiguredOverrideWins` | COMPLIANT |
| R-022 | Default resolves without a code-level constant | `registry.go > Seed`, `DefaultProject` | `registry_test.go > TestResolve_DefaultSeedEntryForOverlayProject`, `TestSeed_OnlyWhenFileAbsent` | COMPLIANT |
| R-023 | Unconfigured, non-default project rejected | `registry.go > ErrVaultNotConfigured` | `registry_test.go > TestResolve_UnconfiguredNonDefaultProjectRejected` | COMPLIANT |
| R-004 | Default top-N and full field parse | `internal/vault/retrieve.go` | `retrieve_test.go > TestRetrieve_DefaultTopNAndFullFieldParse` | COMPLIANT |
| R-004 | Explicit top-N override | same | `retrieve_test.go > TestRetrieve_ExplicitTopNOverride` | COMPLIANT |
| R-024 | Never-indexed vault maps to not_provisioned | `internal/vault/status.go > statusForExitCode`, `retrieve.go` | `retrieve_test.go > TestRetrieve_NotProvisionedExitTenMapsToStatus` | COMPLIANT |

#### longterm-mem-query (7 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-005 | Already-provisioned vault refresh | `internal/vault/index.go` | `index_test.go > TestIndex_AlreadyProvisionedRefresh` | COMPLIANT |
| R-005 | Never-indexed vault is provisioned first | same | `index_test.go > TestIndex_NeverIndexedIsProvisionedFirst` | COMPLIANT |
| R-025 | Failing rebuild step reports failure | same | `index_test.go > TestIndex_RebuildStepFailureReportsFailure` | COMPLIANT |
| R-006 | Results grouped by source in native rank order | `internal/query/query.go > Query` | `query_test.go > TestQuery_GroupedBySourceInNativeRankOrder` | COMPLIANT |
| R-006 | Linked pair emitted once | same | `query_test.go > TestQuery_LinkedPairEmittedOnce` | COMPLIANT |
| R-006 | Missing project argument rejected | same | `query_test.go > TestQuery_MissingProjectRejected` | COMPLIANT |
| R-026 | Unprovisioned vault degrades, no error | same | `query_test.go > TestQuery_NotProvisionedDegradesToEngramOnly` | COMPLIANT |

#### longterm-mem-promotion (25 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-007 | Pinned observation is eligible | `internal/promote/eligible.go` | `TestEligible/pinned_observation_is_eligible` | COMPLIANT |
| R-007 | High-revision, untyped, unpinned is eligible | same | `TestEligible/high-revision,_untyped,_unpinned_observation_is_eligible` | COMPLIANT |
| R-007 | Low-revision, untyped, unpinned not eligible | same | `TestEligible/low-revision,_untyped,_unpinned_observation_is_not_eligible` | COMPLIANT |
| R-007 | Explicit promote overrides automatic criteria | `internal/promote/explicit.go > ExplicitPromote` | `TestEligible/explicit_promote_call_overrides_the_automatic_criteria` | COMPLIANT |
| R-027 | Type mapped onto the vault's contract enum | `internal/promote/page.go`, `frontmatter.go` | `page_test.go > TestEmitPage_TypeMappedOntoVaultEnum` | COMPLIANT |
| R-027 | Related links resolve | `page.go`, `internal/engram/relations.go` | `page_test.go > TestEmitPage_RelatedLinksResolve` | COMPLIANT |
| R-027 | Filename survives a retitle | `internal/promote/address.go` | `page_test.go > TestEmitPage_FilenameSurvivesRetitle` | COMPLIANT |
| R-027 | Freshly promoted page passes the vault's lint | `internal/promote/lint.go > LintPage` | `lint_test.go > TestLintPage_FreshlyPromotedPagePasses` | COMPLIANT |
| R-028 | First promotion allocates a new address | `address.go > Allocate` | `address_test.go > TestAllocate_FirstPromotionAllocatesNewAddress` | COMPLIANT |
| R-028 | Re-promotion reuses the existing address | same | `address_test.go > TestAllocate_RePromotionReusesExistingAddress` | COMPLIANT |
| R-029 | New page is discoverable and logged | `internal/promote/register.go` | `register_test.go > TestRegister_NewPageDiscoverableAndLogged` | COMPLIANT |
| R-008 | Unmodified page updates in place on revision | `internal/promote/update.go` | `update_test.go > TestUpdate_UnmodifiedPageUpdatesInPlace` | COMPLIANT |
| R-008 | Retitle keeps the same file | same | `update_test.go > TestUpdate_RetitleKeepsSameFile` | COMPLIANT |
| R-030 | Locally edited page skipped with a diagnostic | `update.go`, `internal/promote/store.go` | `update_test.go > TestUpdate_LocallyEditedPageSkippedWithDiagnostic` | COMPLIANT |
| R-030 | Unmodified page updates normally | same | `update_test.go > TestUpdate_UnmodifiedPageUpdatesNormally` | COMPLIANT |
| R-009 | Never-promoted eligible observation is promoted | `internal/promote/sync.go` | `TestSync/Never-promoted_eligible_observation_is_promoted` | COMPLIANT |
| R-009 | Revised eligible observation is re-promoted | same | `TestSync/Revised_eligible_observation_is_re-promoted` | COMPLIANT |
| R-009 | Unchanged eligible observation is a no-op | same | `TestSync/Unchanged_eligible_observation_is_a_no-op` | COMPLIANT |
| R-031 | Index and sync-state both reflect the sync | `sync.go` | `sync_test.go > TestSync_IndexAndSyncStateReflectCompletion` | COMPLIANT |
| R-033 | Supersession updates status and related | `internal/promote/propagate.go`, `frontmatter.go > PatchStatusFields` | `TestPropagate/Supersession_updates_status_and_related,_body_untouched` | COMPLIANT |
| R-033 | Soft-delete with no successor archives the page | same | `TestPropagate/Soft-delete_with_no_successor_archives_the_page` | COMPLIANT |
| R-033 | Untouched observation keeps its status | same | `TestPropagate/Untouched_observation_keeps_its_status` | COMPLIANT |
| R-033 | Status patch on a locally edited page still lands | same | `TestPropagate/Status_patch_on_a_locally_edited_page_still_lands_(canon_wins)` | COMPLIANT |
| R-032 | Below-threshold observation promoted via explicit call | `explicit.go`, `cmd_promote.go` | `eligible_test.go > TestPromote_ExplicitCallOverridesAutomaticEligibility` | COMPLIANT |
| R-032 | Invalid observation id is rejected | same | `eligible_test.go > TestPromote_InvalidObservationIdRejected`, `main_test.go > TestCmdPromote_InvalidIdExits7` | COMPLIANT |

#### longterm-mem-ops (11 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-010 | Healthy status reports all three fields | `internal/ops/status.go > Status` | `TestStatus/Healthy_status_reports_all_three_fields` | COMPLIANT |
| R-010 | Never-provisioned vault reported, not an error | same | `TestStatus/Never-provisioned_vault_is_reported,_not_an_error` | COMPLIANT |
| R-010 | Never-synced reports never, not a fabricated timestamp | `status.go > readLastSyncCompletedAt`, `neverSynced` | `TestStatus/Never-synced_project_reports_never,_not_a_fabricated_timestamp` | COMPLIANT |
| R-011 | Unresolvable vault config is named | `internal/ops/doctor.go` | `TestDoctor/Unresolvable_vault_config_is_named` | COMPLIANT |
| R-011 | Corrupted address-map entry is named | same | `TestDoctor/Corrupted_address-map_entry_is_named` | COMPLIANT |
| R-011 | Unregistered promoted page is named | same | `TestDoctor/Unregistered_promoted_page_is_named` | COMPLIANT |
| R-011 | Missing runtime prerequisite is named | `doctor.go`, `internal/vault/runner.go > PrerequisitePresent` | `TestDoctor/Missing_runtime_prerequisite_is_named` | COMPLIANT |
| R-012 | Tool-listing handshake lists both tools | `internal/mcpserver/server.go` | `server_test.go > TestServer_ToolListingListsQueryAndPromote` | COMPLIANT |
| R-012 | Query round-trips over stdio | same | `server_test.go > TestServer_QueryRoundTripsOverStdio` | COMPLIANT |
| R-034 | MCP server exits with its session | `server.go`, `cmd_mcp.go` | `server_test.go > TestServer_ExitsWhenStdinCloses` (executed, not skipped) | COMPLIANT |
| R-034 | No CLI subcommand leaves a residual process | `cmd/longterm-mem/*` | `main_test.go > TestCLI_NoResidualProcessAfterAnySubcommand` (executed, not skipped) | COMPLIANT |

#### longterm-mem-install (4 scenarios) and runtime-lifecycle (3 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-014 | Install builds, copies, then reports per-runtime status | `bin/labdrian-overlay > cmd_longterm_mem`, `engine/runtime/longtermmem.go > Install` | `engine/installer/route_test.go > TestInstall_BuildsCopiesThenReportsPerRuntimeStatus` | COMPLIANT |
| R-014 | Status and uninstall skip the build step | `cmd_longterm_mem`, `LongtermMemAdapter.Status/Uninstall` | `route_test.go > TestStatusUninstall_SkipBuildStep` | COMPLIANT |
| R-014 (rt) | Install records registration and reports per-runtime status | `engine/runtime/longtermmem.go > writeRegistration`, `evaluateAll` | `engine/runtime/longtermmem_test.go > TestLongtermMemAdapter_InstallRecordsRegistrationAndReportsStatus` | COMPLIANT |
| R-014 (rt) | Status and uninstall report without requiring a build | same | `longtermmem_test.go > TestLongtermMemAdapter_StatusAndUninstallRequireNoBuild` | COMPLIANT |
| R-014 (rt) | No update or rollback surface is offered | `longtermmem.go > Update`, `Rollback` (both refuse) | `longtermmem_test.go > TestLongtermMemAdapter_UpdateAndRollbackRefused` | COMPLIANT |
| R-015 | Binary persists after the installing process exits | `bin/labdrian-overlay > LONGTERM_MEM_BINARY` | `route_test.go > TestInstall_BinaryPersistsAfterProcessExits` | COMPLIANT |
| R-015 | Binary path is stable absent install/uninstall | same | `route_test.go > TestInstall_BinaryPathStableAcrossInspections` | COMPLIANT |

#### longterm-mem-mcp-registration (11 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-016 | Unrelated entries are preserved | `internal/register/claude.go`, `jsonsplice.go`, `jsonwrite.go` | `claude_test.go > TestClaude_UnrelatedEntriesPreserved` | COMPLIANT |
| R-016 | Reinstall is idempotent | `claude.go`, `decide.go > Decide` | `claude_test.go > TestClaude_ReinstallIsIdempotent` | COMPLIANT |
| R-016 | Untagged same-named entry refused | `writer.go > ErrConflict`, `Decide > ActionRefuse` | `claude_test.go > TestClaude_UntaggedSameNamedEntryRefused` | COMPLIANT |
| R-017 | Unrelated entries are preserved | `internal/register/opencode.go` | `opencode_test.go > TestOpencode_UnrelatedEntriesPreserved` | COMPLIANT |
| R-017 | Reinstall is idempotent | same | `opencode_test.go > TestOpencode_ReinstallIsIdempotent` | COMPLIANT |
| R-017 | Untagged same-named entry refused | same | `opencode_test.go > TestOpencode_UntaggedSameNamedEntryRefused` | COMPLIANT |
| R-018 | Unrelated sections and ordering preserved | `internal/register/codex.go`, `tomlsplice.go`, `tomlwrite.go` | `codex_test.go > TestCodex_UnrelatedSectionsAndOrderingPreserved` | COMPLIANT |
| R-018 | Reinstall is idempotent | same | `codex_test.go > TestCodex_ReinstallIsIdempotent` | COMPLIANT |
| **R-019** | **Selective removal across all three runtimes** | `internal/register/unregister.go` (correct); `bin/labdrian-overlay:3020` (WRONG `--state-dir`) | `unregister_test.go > TestUnregister_SelectiveRemovalAcrossAllThreeRuntimes` passes at package level; **no test covers the overlay entrypoint, and the shipped entrypoint fails this scenario** | **FAILING** |
| R-019 | Untagged entry preserved and reported, not removed | `unregister.go > UnregisterUnmanaged`, `cmd_unregister.go` (exit 6) | `unregister_test.go > TestUnregister_UntaggedEntryPreservedAndReported` (claude/opencode/codex) | COMPLIANT |
| R-019 | Partial uninstall does not remove the shared binary | `installstate.go > Delete`, `bin/labdrian-overlay > longtermmem_maybe_remove_binary` | `unregister_test.go > TestUnregister_PartialUninstallKeepsSharedBinary`; `route_test.go > TestStatusUninstall_SkipBuildStep` | COMPLIANT |

#### overlay-agent-route (7 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-013 | Agent-routed file deployed to claude agents only | `bin/labdrian-overlay > route_resolve` | `engine/installer/route_test.go > TestRouteResolve_GADUAgentRow` | COMPLIANT |
| R-013 | Skill-routed file deployed to three skills destinations | same | `route_test.go > TestRouteResolve_GADUSkillRow` | COMPLIANT |
| R-013 | Existing two-column rows default to skill route | same | `route_test.go > TestRouteResolve_LegacySkillRow` | COMPLIANT |
| R-013 | Bash dispatch recognizes the mcp route | `route_resolve` mcp branch, `route_repo_rel` | `route_test.go > TestRouteResolve_McpRow`, `TestApply_InvokesLongtermMemInstallOnceForMcpRow` | COMPLIANT |
| R-013 | Go route handling recognizes the mcp route | `engine/skills/ondisk.go > nonSkillRoutes` | `ondisk_test.go > TestDeployableManifestPaths_ExcludesMcpRoute`, `TestRouteDomain_MatchesBashAndGo` | COMPLIANT |
| R-013 | opencode-agent route is unaffected | `route_resolve` | `route_test.go > TestRouteResolve_OpencodeAgentUnaffected` | COMPLIANT |
| **R-035** | **Unrouted longterm-mem row is rejected by both parsers** | bash `route_reject_unrouted_longterm_mem` (correct); **Go `DeployableManifestPaths` has no such guard** | `route_test.go > TestRouteResolve_UnroutedLongtermMemRowRejected` / `..._UnrecognizedRouteLongtermMemRowRejected` **exercise only the bash parser** | **FAILING** |

#### skills-ondisk-validation (2 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-013 (ondisk) | mcp-routed row not required under skills dir | `ondisk.go > DeployableManifestPaths`, `nonSkillRoutes["mcp"]` | `ondisk_test.go > TestDeployableManifestPaths_ExcludesMcpRoute`; `TestRepositorySkillsAreFullyRegistered` (real `overlay.manifest`, zero divergences) | COMPLIANT |
| R-013 (ondisk) | mcp-routed row not a false UNREGISTERED_ON_DISK | `ondisk.go > ScanSkillFiles`/`DiffOnDisk` | `ondisk_test.go > TestRepositorySkillsAreFullyRegistered` (asserts zero divergences over the real manifest containing `longterm-mem/go.mod custom mcp`) | COMPLIANT |

**Compliance summary**: 80/82 scenarios compliant, 2 FAILING, 0 UNTESTED.
33/35 requirements fully satisfied; R-019 and R-035 each have one failing
scenario.

### Reachability Audit (dead behaviour behind ticked tasks)

`golang.org/x/tools/cmd/deadcode` (RTA from `cmd/longterm-mem`) over the
whole module reports exactly three functions unreachable from any
user-invocable entrypoint:

```text
internal/engram/store.go:98:17:  unreachable func: Store.Degraded
internal/engram/store.go:130:17: unreachable func: Store.Path
internal/register/decide.go:31:17: unreachable func: Action.String
```

- `Store.Degraded` and `Store.Path` are called only from `store_test.go`.
  `Store.Degraded` is the more interesting one: `Open` has a real
  `immutable=1` fallback path whose result nothing in production ever
  reads, so `longterm-mem status` prints `engram: reachable=true` with no
  detail even when the connection is the degraded, possibly-stale fallback
  (`cmd_status.go:45-52` opens the store and returns `true, ""`
  unconditionally). Neither `degraded` nor the fallback appears anywhere in
  `tasks.md`, so this is unplanned surface introduced during apply, not a
  ticked task without behaviour.
- `Action.String` has no caller at all -- not even a test. Its doc comment
  claims it is "the name used throughout D9's documentation and writer
  error/status messages (e.g. reporting an `ActionRefuse` conflict)", but
  the writers report conflicts through `ErrConflict`, never through this
  method. Dead code plus an inaccurate comment.

The slice-7 (`RegisterIndex`/`RegisterLog` with zero production callers)
and slice-12b (harness scenario with no call site) shapes did NOT recur:
`promote.Register*` are reachable from `Writer.Promote`, and all five
`goldenWriterCase` harness methods -- including
`testUninstallUntaggedEntryPreservedAndReported` and
`testUninstallRemovesOwnedEntry` -- have call sites in
`claude_test.go`/`opencode_test.go`/`codex_test.go`/`unregister_test.go`.

One further dead call site exists on the shell side, outside Go's reach:
see WARNING-1.

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| D1 read-only Engram DSN | Yes | `readOnlyDSN` uses `mode=ro` + `_query_only=true` + `_txlock=deferred`. `apply-progress.md:41` still documents the earlier `_pragma=query_only(1)` form -- doc drift, code is correct. |
| D4 one writer per file (engine records, module registers) | Yes | `LongtermMemAdapter` writes only `registration.json`; runtime configs are written only by `internal/register`. |
| D5 vault registry, lazy seed, flag>env>row | Yes | `vaultreg.Resolve`. |
| D6 sidecar precedence store, tmp+fsync+rename | Yes | `promote/store.go`, `vaultreg.writeJSONAtomic`, `installstate.Save`. |
| D9 one `Decide` semantics table for install and uninstall | Yes | `jsonInstall`/`tomlInstall`/`jsonUninstall`/`tomlUninstall` all call `Decide`. |
| D13 four-value route domain shared by bash and Go | Partially | Both recognize `{skill, agent, opencode-agent, mcp}`, but only bash rejects an unrouted `longterm-mem/**` row (CRITICAL-2). |
| State-file paths from `tasks.md:111` | Partially | `install-state.json` and `vaults.json` land where documented. The engine registration record is `$STATE_DIR/longterm-mem-registration.json`, not the documented `$STATE_DIR/longterm-mem/registration.json` -- internally consistent across install/status/uninstall, so harmless, but the documented contract is stale. |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD evidence reported | Partial | 10 of 18 slices carry a `### TDD Cycle Evidence` table (slices 6-12b). Slices 1-5 record equivalent per-requirement RED/GREEN narrative plus a `### Verification` block with the exact commands; substantive evidence is present for every slice, only the format differs. |
| All tasks have tests | Yes | Every test file named in `apply-progress.md` and `tasks.md` exists on disk. |
| RED confirmed (test files exist) | Yes | 169 `Test*` functions across the module; every named file verified present. |
| GREEN confirmed (tests pass now) | Yes | Full suite re-executed at verify time, exit 0 in all three modules. |
| Triangulation adequate | Yes | Scenario-shaped table-driven sub-tests (`TestEligible`, `TestSync`, `TestPropagate`, `TestDoctor`, `TestStatus`, `TestDecide_SemanticsTableIsExhaustive`) name each spec scenario literally; `Decide`'s table is pinned exhaustively across all 8 input combinations. |
| Safety net for modified files | Yes | Each slice's `### Verification` block records re-running the prior slices' suites plus the two standing import guards. |

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit | ~150 | 24 | `go test` |
| Integration (subprocess: real binary, real `bin/labdrian-overlay`, real vault fixtures) | ~19 | 5 (`main_test.go`, `mcpserver/server_test.go`, `engine/installer/route_test.go`, `engine/skills/ondisk_test.go`, `runtime/longtermmem_test.go`) | `go test` + `bash` + `python3` + `pgrep` |
| E2E (browser) | 0 | 0 | not applicable |
| **Total** | **169 (longterm-mem) + engine/tui suites** | **29** | |

Environment-gated skips (`-short`, missing `pgrep`, missing `python3`,
running as root) were all inactive in this run: the R-034 and MCP
subprocess tests were confirmed EXECUTED, not skipped.

### Assertion Quality

938 `t.Error`/`t.Fatal` assertions across the module. No tautologies
(`if true`, `expect(1).toBe(1)` equivalents), no assertion-free tests, no
ghost loops over possibly-empty collections, and no orphan empty-collection
assertions were found. Negative cases are paired with positive ones
throughout (`TestDecide_RefuseIsDistinctFromNoop`,
`TestUpdate_UnmodifiedPageUpdatesNormally` alongside
`TestUpdate_LocallyEditedPageSkippedWithDiagnostic`,
`TestUnregister_OutcomeAndOnDiskEffect`'s removed/left-alone/quiet triple).

**Assertion quality**: all assertions verify real behaviour. One structural
caveat, not an assertion defect -- see WARNING-3 (a real Go package lives
under `internal/ops/testdata/`, which the `os/exec` allowlist walk skips).

### Quality Metrics

**Formatter**: `gofmt -l longterm-mem` -- no output, clean.
**Vet**: `go vet ./...` -- no diagnostics.
**Shell syntax**: `bash -n bin/labdrian-overlay`, `bash -n bin/overlay` -- both clean.

### Issues Found

#### CRITICAL

**CRITICAL-1 -- `longterm-mem uninstall` never removes any MCP entry (R-019 violated end to end).**

`bin/labdrian-overlay` installs and uninstalls against two different
install-state directories:

```text
bin/labdrian-overlay:3001  "$LONGTERM_MEM_BINARY" register   --target "$t"
                           # no --state-dir -> defaultRegisterStateDir()
                           #                -> $HOME/.labdrian-overlay/longterm-mem

bin/labdrian-overlay:3020  "$LONGTERM_MEM_BINARY" unregister --target "$t" --state-dir "$STATE_DIR"
                           # STATE_DIR (line 24) = $HOME/.labdrian-overlay
```

`register.jsonUninstall`/`tomlUninstall` compute
`statePath := filepath.Join(stateDir, "install-state.json")`, so install
writes `~/.labdrian-overlay/longterm-mem/install-state.json` while
uninstall reads `~/.labdrian-overlay/install-state.json` -- a file install
never creates. `Decide(entryPresent=true, recordPresent=false, _)` returns
`ActionRefuse`, which `Unregister` maps to `UnregisterUnmanaged`: the entry
is left in place and `cmd_unregister` exits 6.

`tasks.md:111` names the intended path explicitly:
`~/.labdrian-overlay/longterm-mem/install-state.json (module-owned)`. The
`--state-dir "$STATE_DIR"` argument is the defect.

Reproduced at verify time with the real binary built from this tree
(sandbox HOME, the real `~/.claude.json` untouched):

```text
$ longterm-mem register   --target claude --config-root SB --state-dir SB/.labdrian-overlay/longterm-mem --binary ...
longterm-mem: register: claude: ok                                  exit 0
  -> .claude.json now has "unrelated" + ownership-tagged "longterm-mem"

$ longterm-mem unregister --target claude --config-root SB --state-dir SB/.labdrian-overlay
  (this is exactly what bin/labdrian-overlay:3020 does)
longterm-mem: unregister: claude: unmanaged (an entry exists that longterm-mem does not own; left untouched)
                                                                    exit 6
  -> .claude.json UNCHANGED, longterm-mem entry still present

$ longterm-mem unregister --target claude --config-root SB --state-dir SB/.labdrian-overlay/longterm-mem
  (control: the path install actually wrote to)
longterm-mem: unregister: claude: removed                           exit 0
  -> .claude.json back to just "unrelated"
```

The control proves the `internal/register` implementation is correct; only
the shell call site is wrong.

Blast radius is worse than a no-op. `bin/labdrian-overlay:3020` swallows
exit 6 with `|| warn ... continuing`, then line 3022 removes the target
from bash-level tracking anyway. After uninstalling all three targets the
tracking file is empty, so lines 3036-3047 wipe the engine registration
record and delete the shared binary -- leaving three orphaned, still-active
MCP server entries in `~/.claude.json`, `opencode.json` and codex
`config.toml` pointing at a binary that no longer exists.

Why the suite is green: `engine/installer/route_test.go >
TestStatusUninstall_SkipBuildStep` substitutes a two-line
`#!/bin/sh` + `echo dummy` shell script for the longterm-mem binary and
asserts only that the file's mtime/size/content did not change. It never
seeds a runtime config and never asserts an entry was removed, so the
stub's exit 0 hides the real binary's exit 6.

**CRITICAL-2 -- R-035's "rejected by both parsers" is only half implemented.**

Spec (`overlay-agent-route/spec.md` R-012, traces R-035):

> GIVEN a manifest row under `longterm-mem/**` with a missing or
> unrecognized route
> WHEN it is parsed by **either the bash dispatch or the Go route
> handling**
> THEN it is rejected with an explicit error, and it does not resolve to a
> skills-destination path

`bin/labdrian-overlay > route_reject_unrouted_longterm_mem` (lines 414-428)
implements this correctly for bash. `engine/skills/ondisk.go >
DeployableManifestPaths` -- the only Go route handler in the repository
(`rg -l 'route' engine --glob '*.go' --glob '!*_test.go'` returns that one
file) -- has no such guard; its doc comment even states "Any other
third-column value falls through to the skill route."

Executed against the shipped package at verify time:

```text
unrouted-2col        err=<nil> deployableSkillsPaths=[longterm-mem/internal/foo.go]
unrecognized-route   err=<nil> deployableSkillsPaths=[longterm-mem/internal/bar.go]
mcp-routed           err=<nil> deployableSkillsPaths=[]
agent-routed         err=<nil> deployableSkillsPaths=[]
```

The Go parser returns no error AND resolves both malformed rows to a
skills-destination path -- exactly the two things the scenario forbids. The
`mcp` and `agent` control rows confirm the probe is exercising the real
exclusion logic.

`tasks.md` task 9.5 is ticked and cites this scenario by name, but it only
adds two bash-driven tests (`TestRouteResolve_UnroutedLongtermMemRowRejected`,
`TestRouteResolve_UnrecognizedRouteLongtermMemRowRejected`, both invoking
`bin/labdrian-overlay` through `callRouteResolve`). Task 9.6 implements only
the bash guard, and task 9.9 explicitly closes the slice with "no further
production code beyond 9.2/9.6/9.7". No task in the change implements or
tests the Go half. This is a ticked task whose stated scenario coverage
exceeds what shipped.

#### WARNING

**WARNING-1 -- every install invokes a subcommand that does not exist.**

`bin/labdrian-overlay:2996` runs `"$LONGTERM_MEM_BINARY" vaults seed`,
tolerated by `|| warn "... (subcommand lands in a later slice); continuing"`.
`cmd/longterm-mem/main.go > run` has no `vaults` case, and none was ever
planned as an implementable task -- `vaultreg.Resolve` seeds lazily instead.
Verified at verify time:

```text
$ longterm-mem vaults seed
longterm-mem: unknown subcommand "vaults"
usage: longterm-mem <subcommand> [flags]                            exit 2
```

R-022 is unaffected (lazy seeding covers it), but every `longterm-mem
install` prints a misleading warning about a slice that already landed, and
the surrounding comment at lines 2940-2945 is stale for the same reason.
`tasks.md:108` also still lists `vaults` in the CLI surface.

**WARNING-2 -- `status` cannot report a degraded Engram connection.**

`engram.Open` falls back to an `immutable=1` DSN that "may miss
un-checkpointed WAL frames -- stale but never unsafe" and records why in
`Store.degraded`/`degradedCause`. `Store.Degraded()` is never called outside
tests, and `cmd_status.go:45-52`'s `EngramReachable` seam returns
`true, ""` as soon as `Open` succeeds. R-010 asks only for reachability, so
this is not a spec violation, but `StatusReport.EngramDetail` exists and is
exactly where this belongs.

**WARNING-3 -- the `os/exec` allowlist has a real blind spot.**

`exec_allowlist_test.go` skips any directory named `testdata`. But
`internal/ops/testdata/fixture.go` is a genuine, compiled Go package
(`package testdata`, imported by `status_test.go` and `doctor_test.go`), not
inert fixture data. Nothing in it imports `os/exec` today, so R-021 holds,
but the guard would not notice if it did. The guard also skips `_test.go`
files by design; `cmd/longterm-mem/main_test.go` and
`internal/mcpserver/server_test.go` both import `os/exec` legitimately, to
build and run the real binary.

**WARNING-4 -- documented state-file paths have drifted.**

`tasks.md:111` documents the engine registration record at
`~/.labdrian-overlay/longterm-mem/registration.json`; the shipped constant
(`engine/runtime/longtermmem.go:24`) is `longterm-mem-registration.json`
directly under `StateDir`. Install, status and uninstall all use the same
constant, so behaviour is consistent -- only the recorded contract is
stale. `apply-progress.md:41` likewise documents a `_pragma=query_only(1)`
DSN that the shipped `readOnlyDSN` does not use.

#### SUGGESTION

- `register.Action.String()` is dead code carrying a doc comment that
  describes a use which does not exist. Either wire it into the
  `ErrConflict` message (which currently re-derives the wording) or delete
  it.
- `cmdRegister`'s `--target all` skip for a runtime with no config file
  (added in 12a, pinned by
  `TestCmdRegister_AllSkipsRuntimesThatAreNotInstalled`) is real, correct,
  deliberate behaviour that no spec scenario describes. R-016/R-017/R-018
  are silent on multi-target expansion. Worth folding into the spec at
  archive so it is not re-litigated later as an accident.
- `TestUnregister_OutcomeAndOnDiskEffect` runs its removed / left-alone /
  quiet triple for `claude` and `codex` but not `opencode`; opencode is
  covered by the sibling golden-harness tests, so this is asymmetry, not a
  gap.
- The strict-TDD `### TDD Cycle Evidence` table format was adopted only from
  slice 6 onward. Slices 1-5 carry equivalent narrative evidence; adopting
  the table retroactively would make the apply record uniformly
  machine-readable.

### Verdict

**FAIL** -- 2 CRITICAL, 4 WARNING, 4 SUGGESTION.

The module-level implementation is strong: 80 of 82 spec scenarios are
proved by a test observed passing at runtime, all four contract gates and
both standing import guards are green, `gofmt` and `go vet` are clean,
coverage averages 78%, and the slice-7 / slice-12b unreachable-behaviour
patterns did not recur.

Two requirements are nevertheless not met as written, and both gaps sit
exactly where the test suite does not look:

- **R-019** is violated by the only user-invocable uninstall path.
  `bin/labdrian-overlay:3020` passes the wrong `--state-dir`, so
  `longterm-mem uninstall` leaves every MCP entry in place while still
  deleting the shared binary and the engine registration record. Proved by
  execution, with a passing control.
- **R-035** is implemented in bash only. The Go route handler resolves an
  unrouted or unrecognized `longterm-mem/**` row to a skills-destination
  path without error -- the exact behaviour the scenario forbids -- and
  ticked task 9.5 claims a scenario it only half covers.

Neither is a design flaw: `internal/register` and
`route_reject_unrouted_longterm_mem` are both correct. CRITICAL-1 is a
one-argument fix at a single call site plus an end-to-end test that uses the
real binary instead of a stub; CRITICAL-2 needs a guard in
`DeployableManifestPaths` plus a Go-side test.

This change is not archive-ready. Return to `sdd-apply`.
