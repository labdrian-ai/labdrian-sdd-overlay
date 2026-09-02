```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:2f7861f9108fbb3c91524ceb4668afe75c53af64f901e377be7f4e440fd6962e
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 35/35
scenarios: 82/82
test_command: cd longterm-mem && go test -count=1 ./... && cd ../engine && go test -count=1 ./... && cd ../tui && go test -count=1 ./...
test_exit_code: 0
test_output_hash: sha256:5857bf6cdea0a8ae9d0dc93b0830638966e1177de8cec2955a66420eb646d7e4
build_command: bash -n bin/labdrian-overlay && bash -n bin/overlay && cd longterm-mem && go vet ./... && gofmt -l . && cd ../engine && go vet ./... && gofmt -l .
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: longterm-mem
**Version**: 9 delta specs, R-001 through R-035
**Mode**: Strict TDD
**Artifact store**: openspec (`openspec/changes/longterm-mem/`)
**Worktree**: `/home/labdrian/labdrian-sdd-overlay-worktrees/longterm-mem`
**Branch**: `feat/lm/longterm-mem-verifyfix-13e-verify-report` @ `e94e42d` (tree `3268e63`, working tree clean)
**Run**: second verification, after the first returned FAIL (2 CRITICAL) and slice 13 remediated it.

`evidence_revision` is `sha256(HEAD tree object id)` for the tree verified,
computed before this report was written into it.

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 179 |
| Tasks complete | 179 |
| Tasks incomplete | 0 |
| Requirements (delta specs) | 35 |
| Scenarios (delta specs) | 82 |
| Requirements fully satisfied | 35 |
| Scenarios compliant | 82 |

Native `gentle-ai sdd-status --json longterm-mem` reports `taskProgress
179/179`, `allComplete: true`, `applyState: all_done`, `verify: ready`.
Slice 13 added 11 tasks (13.1-13.11) to the 168 the first run counted.

Counts were re-derived from the nine delta spec files, not carried over:
memory-access 9 requirements/12 scenarios, query 4/7, promotion 10/25,
ops 4/11, install 2/4, runtime-lifecycle 1/3, mcp-registration 4/11,
overlay-agent-route 2/7, skills-ondisk-validation 1/2. That is 37 spec
requirement blocks, which dedupe to 35 distinct longterm-mem requirement
IDs because `runtime-lifecycle` and `longterm-mem-install` both trace
R-014, and `overlay-agent-route` R-006 and `skills-ondisk-validation`
R-012 both trace R-013.

### Build & Tests Execution

**Tests**: all green, all re-run with `-count=1` so no result came from the
build cache.

```text
cd longterm-mem && go test -count=1 ./...                            exit 0
  longterm-mem                    ok  0.003s
  longterm-mem/cmd/longterm-mem   ok  1.191s
  longterm-mem/internal/engram    ok  0.402s
  longterm-mem/internal/mcpserver ok  0.695s
  longterm-mem/internal/ops       ok  0.022s
  longterm-mem/internal/promote   ok  0.632s
  longterm-mem/internal/query     ok  0.184s
  longterm-mem/internal/register  ok  0.086s
  longterm-mem/internal/vault     ok  0.483s
  longterm-mem/internal/vaultreg  ok  0.008s

cd engine && go test -count=1 ./...                                  exit 0
  assets, cmd, gadu, gate, installer (12.728s), prespec,
  propagator, runtime, settings, skills  -- all ok

cd tui && go test -count=1 ./...                                     exit 0
  tui  ok  5.146s

bash -n bin/labdrian-overlay                                         exit 0
bash -n bin/overlay                                                  exit 0
```

**Static analysis**:

```text
gofmt -l longterm-mem engine tui   exit 0, no output (all files formatted)
cd longterm-mem && go vet ./...    exit 0, no diagnostics
cd engine && go vet ./...          exit 0, no diagnostics
```

**shellcheck** (`shellcheck bin/labdrian-overlay`): exit 1, 8 findings --
4 `SC2064` warnings and 4 informational (`SC2016`, `SC2094`). Every one is
pre-existing. Compared byte-for-byte against the same file on `main`
(`git show main:bin/labdrian-overlay`): the two runs produce the identical
set of eight findings with identical codes and columns, differing only in
line numbers shifted by the change's additions.

| Code | main | this branch | Verdict |
|---|---|---|---|
| SC2094 | 264:11, 265:53 | 296:11, 297:53 | unchanged |
| SC2016 | 297:12, 301:8 | 329:12, 333:8 | unchanged |
| SC2064 | 957, 1118, 1310, 1341 | 1039, 1200, 1433, 1464 | unchanged |

**Zero new shellcheck findings.** The `cmd_longterm_mem` block this change
owns (lines ~2875-3137) produces none.

**Standing import guards** (both executed individually, both PASS):

| Guard | Location | Result |
|---|---|---|
| `TestOSExecImportAllowlist` | `longterm-mem/exec_allowlist_test.go` | PASS -- only `internal/vault/runner.go` imports `os/exec` |
| `TestZeroFetchImportAllowlist` | `engine/skills/zero_fetch_test.go` | PASS |

`engine/go.mod` is still two lines: a `module` directive and `go 1.21`,
with no `require` block at all. Slice 13 touched `engine/skills/ondisk.go`
and added only stdlib `fmt` usage there, so the zero-dependency boundary is
intact.

**Coverage** (per package, `go test -count=1 -cover`):

| Package | Coverage | Delta vs first run | Rating |
|---|---|---|---|
| `internal/ops` | 89.0% | = | Excellent |
| `internal/query` | 85.1% | = | Excellent |
| `internal/promote` | 84.6% | = | Excellent |
| `internal/vault` | 84.1% | = | Excellent |
| `internal/engram` | 82.9% | -0.1 | Excellent |
| `internal/register` | 78.2% | +0.9 | Acceptable |
| `internal/mcpserver` | 77.8% | = | Acceptable |
| `internal/vaultreg` | 67.2% | = | Low |
| `cmd/longterm-mem` | 55.6% | +0.4 | Low |
| root `guard` package | no statements | = | n/a |

Average across measurable packages: 78.3%. Coverage is informational and
does not gate this verdict.

### CRITICAL Closure -- proved by execution, not by reading the fix

Both findings were re-tested from scratch with independent probes, on the
same principle that exposed them: the first run caught them because it ran
the shipped artefact instead of trusting the suite.

#### CRITICAL-1 (R-019) -- CLOSED

Full install/uninstall round trip through the real `bin/labdrian-overlay`,
sandbox `HOME`, all three runtimes seeded with an unrelated MCP entry each.
The real `~/.claude.json`, `opencode.json` and codex `config.toml` were
never touched.

```text
$ HOME=SB bash bin/labdrian-overlay longterm-mem install --target all
longterm-mem: register: claude: ok
longterm-mem: register: opencode: ok
longterm-mem: register: codex: ok
[longterm-mem] install: supported -- claude: supported; opencode: supported; codex: supported
  install-state.json  SB/.labdrian-overlay/longterm-mem/install-state.json  (371 bytes)
  tracking            claude codex opencode
  binary              SB/.labdrian-overlay/bin/longterm-mem  (16 MB)
  all three configs   unrelated entry + ownership-tagged longterm-mem entry

$ HOME=SB bash bin/labdrian-overlay longterm-mem uninstall --target all
longterm-mem: unregister: claude: removed
longterm-mem: unregister: opencode: removed
longterm-mem: unregister: codex: removed
==> longterm-mem binary removed
  install-state.json  {"schema":1,"targets":{}}
  tracking            absent
  binary              absent
  engine record       absent
  .claude.json        {"mcpServers":{"unrelated":{"command":"other","args":["x"]}}}
  opencode.json       {"mcp":{"unrelated":{"type":"local","command":["other","x"]}}}
  config.toml         [mcp_servers.unrelated] / command = "other"
```

All three entries were REMOVED (exit 0), not reported `unmanaged` (exit 6),
and every configuration file is byte-identical to its pre-install content.
That is the exact inverse of the first run's reproduction.

**Both branch directions were then driven deliberately.** The review
correction split non-zero statuses into "the operator can fix this" and
"no retry can change this", so both halves were tested:

| # | Induced condition | `unregister` | Tracking | Binary | Engine record | Entries | Process exit |
|---|---|---|---|---|---|---|---|
| B1 | `.claude.json` unreadable (`chmod 000`) | claude exit 1; opencode/codex exit 0 | claude KEPT | KEPT | KEPT | claude PRESENT; other two removed | **1** |
| B1b | same sandbox, permissions restored, re-run `--target claude` | exit 0 removed | cleared | removed | removed | all absent | 0 |
| B2 | binary deleted before uninstall | not runnable | cleared | (already gone) | removed | all three PRESENT | 0 |
| B2b | documented recovery: reinstall, re-run uninstall | exit 0 removed x3 | cleared | removed | removed | all absent | 0 |
| B3 | binary replaced by a stub exiting 2 (version skew) | exit 2 x3 | cleared | removed | removed | all three PRESENT | 0 |
| B4 | ownership record deleted (entries now untagged) | exit 6 unmanaged x3 | cleared | removed | removed | all three PRESENT | 0 |
| B5 | partial uninstall, `--target claude` only | exit 0 removed | codex+opencode KEPT | **KEPT** | KEPT | claude absent; other two PRESENT | 0 |
| B6 | partial uninstall with `--purge` | exit 0 removed | cleared | removed | removed | claude absent; other two PRESENT | 0 |

Neither unusable-binary direction produces a wrong outcome:

- **B1 is the load-bearing direction.** A real, operator-fixable failure
  keeps the target tracked, leaves the shared binary AND the engine record
  in place, and returns exit 1. The first run's blast radius -- tracking
  cleared, binary and record deleted while an active entry survives -- is
  now unreachable through this path. B1b proves the state is recoverable
  rather than merely blocked.
- **B2 and B3 converge instead of wedging.** Both print an explicit warning
  naming the target, saying the entry stays in its runtime config, and
  giving two exits ("remove it by hand, or reinstall and re-run
  uninstall"). B3 adds "Not retried; every re-run reproduces this."
  The messages are truthful, not a dead end: B2b executes the documented
  recovery and it fully converges. Crucially, `install-state.json` keeps
  its ownership records in both cases, which is what makes that recovery
  possible. Treating these as blocking would leave `--purge` -- the one
  action that orphans the entries -- as the only escape.
- **B5 is R-019 scenario 3 end to end**: uninstalling one runtime removes
  only that runtime's entry and keeps the shared binary, the engine record
  and the other two entries. **B4 is R-019 scenario 2 end to end**: an
  entry longterm-mem does not own is left in place and reported.

The shipped code matches: `bin/labdrian-overlay:3007` calls `register
--target "$t"` with no `--state-dir`, and `:3068` calls `unregister
--target "$t"` with no `--state-dir`, so both resolve
`defaultRegisterStateDir()` = `~/.labdrian-overlay/longterm-mem`
(`register_paths.go:41-47`). The exit status is captured into
`unregister_exit` and branched on (`:3067-3093`), not swallowed.

Regression tests confirmed executing, not skipping:

```text
--- PASS: TestInstall_UninstallRoundTripRemovesTheMcpEntry (0.58s)
--- PASS: TestUninstall_HardFailureKeepsTrackingAndSharedBinary (0.54s)
--- PASS: TestUninstall_MissingBinaryStillConverges (0.54s)
--- PASS: TestStatusUninstall_SkipBuildStep (4 sub-tests)
```

`TestInstall_UninstallRoundTripRemovesTheMcpEntry` builds and runs the real
binary through the real entrypoint and deliberately points `STATE_DIR`
outside `$HOME/.labdrian-overlay` -- the exact condition that hid the
defect. The two-line `echo dummy` stub that made the first run's suite
green is gone from the uninstall assertion path.

#### CRITICAL-2 (R-035) -- CLOSED

Re-probed with an independent program linked against the shipped
`engine/skills` package (a scratch module with a `replace` onto this
worktree), not by reading the test:

```text
row                            deployableSkillsPaths  err
unrouted-2col                  []                     ondisk: manifest row "longterm-mem/internal/foo.go" under
                                                      longterm-mem/** declares no route (missing third column)
unrecognized-route             []                     ondisk: manifest row "longterm-mem/internal/bar.go" under
                                                      longterm-mem/** declares an unrecognized route "bogus"
empty-3rd-col-trailing-ws      []                     (same "no route" error)
mcp-routed        CONTROL      []                     <nil>
agent-routed      CONTROL      []                     <nil>
opencode-agent    CONTROL      []                     <nil>
skill-routed      CONTROL      [longterm-mem/z.md]    <nil>
non-ltm unrouted  CONTROL      [skills/foo/SKILL.md]  <nil>
non-ltm bogus     CONTROL      [skills/bar/SKILL.md]  <nil>
ltm prefix-lookalike CONTROL   [longterm-memories/thing.go]  <nil>

real overlay.manifest:  err=<nil>  83 deployable rows
longterm-mem rows resolving to a skills destination: []
```

Both rows the first run proved defective now return an explicit error
naming the row AND resolve to nothing. The controls prove the guard is
scoped, not blanket: every one of the four valid routes still parses, a
non-`longterm-mem` row with a bogus route still falls through to the skill
default (unchanged, out of R-012's scope), and `longterm-memories/` does
not match the `longterm-mem/` prefix. The real repository manifest parses
clean.

The implementation is the shape the remediation claims: `ondisk.go:48-53`
declares `validLongtermMemRoutes` as an independent four-value set, and
`ondisk.go:93-98` applies it only to rows under `longterm-mem/`, before the
`nonSkillRoutes` exclusion at `:100`. It is genuinely not `nonSkillRoutes`
plus an entry -- `nonSkillRoutes` still answers "does this route leave
skills/?" and correctly omits `skill`.

Tests confirmed passing:

```text
--- PASS: TestDeployableManifestPaths_RejectsUnroutedLongtermMemRow
--- PASS: TestDeployableManifestPaths_RejectsUnrecognizedRouteLongtermMemRow
--- PASS: TestDeployableManifestPaths_LongtermMemRouteGuardAcceptsEveryValidRoute
--- PASS: TestRouteDomain_MatchesBashAndGo
--- PASS: TestRepositorySkillsAreFullyRegistered
```

The triangulation test hardcodes `{skill, agent, opencode-agent, mcp}`
rather than iterating the map it guards, so it is a real pin and not a
tautology.

### Spec Compliance Matrix

Every row was checked against production code AND a test observed passing
at runtime in this run. Test names are exact; sub-test names are the
literal spec scenario titles wherever the implementation used table-driven
tests. The 33 rows the first run passed were re-verified, not carried
over: the whole suite was re-executed with `-count=1`, every cited test
function was confirmed still present on disk, and the scenario-named
sub-tests were re-run with `-v` to read their names back.

#### longterm-mem-memory-access (9 requirements, 12 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-001 | Component builds as an independent module | `longterm-mem/go.mod`, `cmd/longterm-mem/main.go` | `main_test.go > TestMain_BuildsIndependentModule` | COMPLIANT |
| R-001 | engine/'s zero-dependency gate stays green | `engine/go.mod` (no require block) | `engine/skills/zero_fetch_test.go > TestZeroFetchImportAllowlist` | COMPLIANT |
| R-002 | Default connection is read-only | `internal/engram/store.go > Open`, `readOnlyDSN` | `store_test.go > TestOpen_DefaultIsReadOnly` | COMPLIANT |
| R-002 | Overridden connection stays read-only | same | `store_test.go > TestOpen_OverridePathStaysReadOnly` | COMPLIANT |
| R-020 | Soft-deleted and other-project observations excluded | `store.go > ListObservations` | `store_test.go > TestListObservations_ScopesProjectAndExcludesSoftDeleted` | COMPLIANT |
| R-021 | No subprocess call to Engram's CLI | `internal/vault/runner.go` (sole `os/exec` importer) | `exec_allowlist_test.go > TestOSExecImportAllowlist` | COMPLIANT |
| R-003 | Configured override is used | `internal/vaultreg/registry.go > Resolve` | `registry_test.go > TestResolve_ConfiguredOverrideWins` | COMPLIANT |
| R-022 | Default resolves without a code-level constant | `registry.go > Seed`, `DefaultProject` | `registry_test.go > TestResolve_DefaultSeedEntryForOverlayProject`, `TestSeed_OnlyWhenFileAbsent` | COMPLIANT |
| R-023 | Unconfigured, non-default project rejected | `registry.go > ErrVaultNotConfigured` | `registry_test.go > TestResolve_UnconfiguredNonDefaultProjectRejected` | COMPLIANT |
| R-004 | Default top-N and full field parse | `internal/vault/retrieve.go` | `retrieve_test.go > TestRetrieve_DefaultTopNAndFullFieldParse` | COMPLIANT |
| R-004 | Explicit top-N override | same | `retrieve_test.go > TestRetrieve_ExplicitTopNOverride` | COMPLIANT |
| R-024 | Never-indexed vault maps to not_provisioned | `internal/vault/status.go > statusForExitCode` | `retrieve_test.go > TestRetrieve_NotProvisionedExitTenMapsToStatus` | COMPLIANT |

#### longterm-mem-query (4 requirements, 7 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-005 | Already-provisioned vault refresh | `internal/vault/index.go` | `index_test.go > TestIndex_AlreadyProvisionedRefresh` | COMPLIANT |
| R-005 | Never-indexed vault is provisioned first | same | `index_test.go > TestIndex_NeverIndexedIsProvisionedFirst` | COMPLIANT |
| R-025 | Failing rebuild step reports failure | same | `index_test.go > TestIndex_RebuildStepFailureReportsFailure` | COMPLIANT |
| R-006 | Results grouped by source in native rank order | `internal/query/query.go > Query` | `query_test.go > TestQuery_GroupedBySourceInNativeRankOrder` | COMPLIANT |
| R-006 | Linked pair emitted once | same | `query_test.go > TestQuery_LinkedPairEmittedOnce` | COMPLIANT |
| R-006 | Missing project argument rejected | same | `query_test.go > TestQuery_MissingProjectRejected` | COMPLIANT |
| R-026 | Unprovisioned vault degrades, no error | same | `query_test.go > TestQuery_NotProvisionedDegradesToEngramOnly` | COMPLIANT |

#### longterm-mem-promotion (10 requirements, 25 scenarios)

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
| R-033 | Supersession updates status and related | `propagate.go`, `frontmatter.go > PatchStatusFields` | `TestPropagate/Supersession_updates_status_and_related,_body_untouched` | COMPLIANT |
| R-033 | Soft-delete with no successor archives the page | same | `TestPropagate/Soft-delete_with_no_successor_archives_the_page` | COMPLIANT |
| R-033 | Untouched observation keeps its status | same | `TestPropagate/Untouched_observation_keeps_its_status` | COMPLIANT |
| R-033 | Status patch on a locally edited page still lands | same | `TestPropagate/Status_patch_on_a_locally_edited_page_still_lands_(canon_wins)` | COMPLIANT |
| R-032 | Below-threshold observation promoted via explicit call | `explicit.go`, `cmd_promote.go` | `eligible_test.go > TestPromote_ExplicitCallOverridesAutomaticEligibility` | COMPLIANT |
| R-032 | Invalid observation id is rejected | same | `eligible_test.go > TestPromote_InvalidObservationIdRejected`, `main_test.go > TestCmdPromote_InvalidIdExits7` | COMPLIANT |

#### longterm-mem-ops (4 requirements, 11 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-010 | Healthy status reports all three fields | `internal/ops/status.go > Status` | `TestStatus/Healthy_status_reports_all_three_fields` | COMPLIANT |
| R-010 | Never-provisioned vault reported, not an error | same | `TestStatus/Never-provisioned_vault_is_reported,_not_an_error` | COMPLIANT |
| R-010 | Never-synced reports never, not a fabricated timestamp | `status.go > readLastSyncCompletedAt` | `TestStatus/Never-synced_project_reports_never,_not_a_fabricated_timestamp` | COMPLIANT |
| R-011 | Unresolvable vault config is named | `internal/ops/doctor.go` | `TestDoctor/Unresolvable_vault_config_is_named` | COMPLIANT |
| R-011 | Corrupted address-map entry is named | same | `TestDoctor/Corrupted_address-map_entry_is_named` | COMPLIANT |
| R-011 | Unregistered promoted page is named | same | `TestDoctor/Unregistered_promoted_page_is_named` | COMPLIANT |
| R-011 | Missing runtime prerequisite is named | `doctor.go`, `runner.go > PrerequisitePresent` | `TestDoctor/Missing_runtime_prerequisite_is_named` | COMPLIANT |
| R-012 | Tool-listing handshake lists both tools | `internal/mcpserver/server.go` | `server_test.go > TestServer_ToolListingListsQueryAndPromote` | COMPLIANT |
| R-012 | Query round-trips over stdio | same | `server_test.go > TestServer_QueryRoundTripsOverStdio` | COMPLIANT |
| R-034 | MCP server exits with its session | `server.go`, `cmd_mcp.go` | `server_test.go > TestServer_ExitsWhenStdinCloses` (executed, not skipped) | COMPLIANT |
| R-034 | No CLI subcommand leaves a residual process | `cmd/longterm-mem/*` | `main_test.go > TestCLI_NoResidualProcessAfterAnySubcommand` (executed, not skipped) | COMPLIANT |

#### longterm-mem-install (2 requirements, 4 scenarios) and runtime-lifecycle (1 requirement, 3 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-014 | Install builds, copies, then reports per-runtime status | `cmd_longterm_mem`, `engine/runtime/longtermmem.go > Install` | `route_test.go > TestInstall_BuildsCopiesThenReportsPerRuntimeStatus` | COMPLIANT |
| R-014 | Status and uninstall skip the build step | `cmd_longterm_mem`, `LongtermMemAdapter.Status/Uninstall` | `route_test.go > TestStatusUninstall_SkipBuildStep` (4 sub-tests) | COMPLIANT |
| R-014 (rt) | Install records registration and reports per-runtime status | `longtermmem.go > writeRegistration`, `evaluateAll` | `longtermmem_test.go > TestLongtermMemAdapter_InstallRecordsRegistrationAndReportsStatus` | COMPLIANT |
| R-014 (rt) | Status and uninstall report without requiring a build | same | `longtermmem_test.go > TestLongtermMemAdapter_StatusAndUninstallRequireNoBuild` | COMPLIANT |
| R-014 (rt) | No update or rollback surface is offered | `longtermmem.go > Update`, `Rollback` (both refuse) | `longtermmem_test.go > TestLongtermMemAdapter_UpdateAndRollbackRefused` | COMPLIANT |
| R-015 | Binary persists after the installing process exits | `LONGTERM_MEM_BINARY` | `route_test.go > TestInstall_BinaryPersistsAfterProcessExits` | COMPLIANT |
| R-015 | Binary path is stable absent install/uninstall | same | `route_test.go > TestInstall_BinaryPathStableAcrossInspections` | COMPLIANT |

#### longterm-mem-mcp-registration (4 requirements, 11 scenarios)

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
| **R-019** | **Selective removal across all three runtimes** | `internal/register/unregister.go`; `bin/labdrian-overlay:3007` and `:3068` (both now resolve the module default state dir) | `unregister_test.go > TestUnregister_SelectiveRemovalAcrossAllThreeRuntimes` (package) plus `route_test.go > TestInstall_UninstallRoundTripRemovesTheMcpEntry` (real binary through the real entrypoint) plus the verify-time round trip across all three runtimes | **COMPLIANT** |
| R-019 | Untagged entry preserved and reported, not removed | `unregister.go > UnregisterUnmanaged`, `cmd_unregister.go` (exit 6) | `unregister_test.go > TestUnregister_UntaggedEntryPreservedAndReported`; verify-time proof B4 | COMPLIANT |
| R-019 | Partial uninstall does not remove the shared binary | `installstate.go > Delete`, `longtermmem_maybe_remove_binary` | `unregister_test.go > TestUnregister_PartialUninstallKeepsSharedBinary`; `TestStatusUninstall_SkipBuildStep/UninstallSingleTargetSkipsBuildAndLeavesBinaryInPlace`; verify-time proofs B5 and B6 | COMPLIANT |

#### overlay-agent-route (2 requirements, 7 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-013 | Agent-routed file deployed to claude agents only | `route_resolve` | `route_test.go > TestRouteResolve_GADUAgentRow` | COMPLIANT |
| R-013 | Skill-routed file deployed to three skills destinations | same | `route_test.go > TestRouteResolve_GADUSkillRow` | COMPLIANT |
| R-013 | Existing two-column rows default to skill route | same | `route_test.go > TestRouteResolve_LegacySkillRow` | COMPLIANT |
| R-013 | Bash dispatch recognizes the mcp route | `route_resolve` mcp branch | `route_test.go > TestRouteResolve_McpRow`, `TestApply_InvokesLongtermMemInstallOnceForMcpRow` | COMPLIANT |
| R-013 | Go route handling recognizes the mcp route | `ondisk.go > nonSkillRoutes` | `ondisk_test.go > TestDeployableManifestPaths_ExcludesMcpRoute`, `TestRouteDomain_MatchesBashAndGo` | COMPLIANT |
| R-013 | opencode-agent route is unaffected | `route_resolve` | `route_test.go > TestRouteResolve_OpencodeAgentUnaffected` | COMPLIANT |
| **R-035** | **Unrouted longterm-mem row is rejected by both parsers** | bash `route_reject_unrouted_longterm_mem` and Go `ondisk.go > validLongtermMemRoutes` at `:93-98` | `route_test.go > TestRouteResolve_UnroutedLongtermMemRowRejected` / `..._UnrecognizedRouteLongtermMemRowRejected` (bash) plus `ondisk_test.go > TestDeployableManifestPaths_RejectsUnroutedLongtermMemRow` / `..._RejectsUnrecognizedRouteLongtermMemRow` / `..._LongtermMemRouteGuardAcceptsEveryValidRoute` (Go) | **COMPLIANT** -- see WARNING-1 for a residual one-column parity gap outside this scenario's row grammar |

#### skills-ondisk-validation (1 requirement, 2 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-013 (ondisk) | mcp-routed row not required under skills dir | `ondisk.go > DeployableManifestPaths`, `nonSkillRoutes["mcp"]` | `ondisk_test.go > TestDeployableManifestPaths_ExcludesMcpRoute`; `TestRepositorySkillsAreFullyRegistered` | COMPLIANT |
| R-013 (ondisk) | mcp-routed row not a false UNREGISTERED_ON_DISK | `ondisk.go > ScanSkillFiles`/`DiffOnDisk` | `ondisk_test.go > TestRepositorySkillsAreFullyRegistered` (real `overlay.manifest`, zero divergences) | COMPLIANT |

**Compliance summary**: 82/82 scenarios compliant, 0 FAILING, 0 UNTESTED.
35/35 requirements fully satisfied.

### Reachability Audit (dead behaviour behind ticked tasks)

`golang.org/x/tools/cmd/deadcode` RTA from `cmd/longterm-mem` over the
whole module now reports **zero** unreachable functions -- the first run
reported three.

```text
cd longterm-mem && deadcode -test=false ./cmd/longterm-mem   (no output)
cd longterm-mem && deadcode ./cmd/longterm-mem               (no output)
cd longterm-mem && deadcode -test=false ./...                (no output)
```

Positive control, so an empty result is a finding and not a broken tool:
the same binary run against `engine/cmd` in this worktree reports 16
unreachable functions (`gadu.PersonaBody`, `prespec.New`,
`runtime.MutatePrompt`, ...). None of them is in a file this change
touched -- `engine/runtime/longtermmem.go` and `engine/skills/ondisk.go`
are both fully reachable.

Disposition of the three the first run named:

| Function | Disposition | Evidence |
|---|---|---|
| `internal/engram/store.go > Store.Degraded` | WIRED into production | `cmd_status.go:57-58` reads it and returns a degraded detail; `main_test.go > TestCmdStatus_ReportsDegradedEngramConnection` PASS |
| `internal/engram/store.go > Store.Path` | DELETED, with its `Store.path` field | absent from `store.go`; `TestOpen_DefaultIsReadOnly` and `TestOpen_OverridePathStaysReadOnly` both survive |
| `internal/register/decide.go > Action.String` | DELETED | no `Action) String` anywhere in `internal/register`; `UnregisterOutcome.String()` (live, called through `%s` in `cmd_unregister.go`) had its doc comment corrected |

The shell-side dead call site the first run reported as WARNING-1 is also
gone: `bin/labdrian-overlay` no longer invokes `longterm-mem vaults seed`,
and `cmd/longterm-mem/main.go > run` has no `vaults` case (R-022 is
satisfied by lazy seeding in `vaultreg.Resolve`). Only a historical
comment at `:2948` still mentions the removed call, correctly, in the past
tense.

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| D1 read-only Engram DSN | Partially | `readOnlyDSN` is `mode=ro` + `_query_only=true` + `_txlock=deferred` + `_pragma=busy_timeout(2000)`, exactly read-only. But D1 says "no `immutable=1`", and `store.go:124` ships `readOnlyImmutableDSN` as a fallback. Not a spec break (still read-only) and now honestly surfaced by `status` -- but design.md was never amended. See WARNING-6. |
| D4 one writer per file | Yes | `LongtermMemAdapter` writes only its registration record; runtime configs are written only by `internal/register`. |
| D5 vault registry, lazy seed, flag>env>row | Yes | `vaultreg.Resolve`; the removal of the `vaults seed` call did not regress it (`TestSeed_OnlyWhenFileAbsent`, `TestResolve_DefaultSeedEntryForOverlayProject`). |
| D6 sidecar precedence store, tmp+fsync+rename | Yes | `promote/store.go`, `vaultreg.writeJSONAtomic`, `installstate.Save`. |
| D9 one `Decide` semantics table for install and uninstall | Yes | `jsonInstall`/`tomlInstall`/`jsonUninstall`/`tomlUninstall` all call `Decide`. |
| D9 install and uninstall share one install-state location | **Yes (was the CRITICAL-1 break)** | Both call sites now omit `--state-dir` and resolve `defaultRegisterStateDir()`. Proved by round trip. |
| D13 four-value route domain shared by bash and Go | Yes, with one narrow gap | Both parsers now reject a missing or unrecognized route on `longterm-mem/**`. Bash additionally rejects a one-column row; Go skips it silently. See WARNING-1. |
| State-file paths from `tasks.md:111` | Partially | Behaviour is internally consistent, the recorded contract is stale. See WARNING-5. |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD evidence reported | Partial | 11 of 19 slices carry a `### TDD Cycle Evidence` table (slices 6 through 13). Slices 1-5 record equivalent per-requirement RED/GREEN narrative plus a `### Verification` block with exact commands. Substantive evidence exists for every slice; only the format differs. |
| All tasks have tests | Yes | Every test file named in `apply-progress.md` and `tasks.md` exists on disk. |
| RED confirmed (test files exist) | Yes | 170 `Test*` functions across 36 test files in `longterm-mem`, plus 36 in `engine`. Slice 13's RED evidence quotes verbatim pre-fix failures for both CRITICALs and for the degraded-status wiring. |
| GREEN confirmed (tests pass now) | Yes | Full suite re-executed with `-count=1` at verify time, exit 0 in all three modules. |
| Triangulation adequate | Yes | Scenario-shaped table-driven sub-tests name each spec scenario literally; `Decide`'s table is pinned exhaustively across all 8 input combinations; slice 13 added `..._LongtermMemRouteGuardAcceptsEveryValidRoute` as an explicit third case that hardcodes the domain rather than iterating the map it guards. |
| Safety net for modified files | Yes | Each slice's `### Verification` block records re-running prior slices plus both standing import guards; slice 13's table records "full suite green before edit" for every row. |
| Apply record matches shipped code | **No** | Slice 13's evidence table and test summary describe 6 new tests and 10 tasks; 7 new tests and a further production branch shipped. See WARNING-2. |

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit | ~152 | 31 | `go test` |
| Integration (subprocess: real binary, real `bin/labdrian-overlay`, real vault fixtures, real SQLite fallback) | ~23 | 5 (`cmd/longterm-mem/main_test.go`, `internal/mcpserver/server_test.go`, `engine/installer/route_test.go`, `engine/skills/ondisk_test.go`, `engine/runtime/longtermmem_test.go`) | `go test` + `bash` + `python3` + `pgrep` |
| E2E (browser) | 0 | 0 | not applicable |
| **Total** | **170 (longterm-mem) + engine/tui suites** | **36 + 36** | |

Environment-gated skips (`-short`, missing `pgrep`, missing `python3`,
running as root) were all inactive in this run. Confirmed EXECUTED, not
skipped, by reading `-v` output: `TestServer_ExitsWhenStdinCloses`,
`TestCLI_NoResidualProcessAfterAnySubcommand`,
`TestCmdStatus_ReportsDegradedEngramConnection`,
`TestInstall_UninstallRoundTripRemovesTheMcpEntry`,
`TestUninstall_HardFailureKeepsTrackingAndSharedBinary`,
`TestUninstall_MissingBinaryStillConverges`.

### Assertion Quality

946 `t.Error`/`t.Fatal` assertions across the `longterm-mem` module. A scan
for tautologies (`if true`, `== true)`, `assert(true`, `expect(true`)
across `longterm-mem`, `engine/skills` and `engine/installer` returns
nothing. No assertion-free tests, no ghost loops over possibly-empty
collections, and no orphan empty-collection assertions were found.
Negative cases are paired with positive ones throughout
(`TestDecide_RefuseIsDistinctFromNoop`,
`TestUpdate_UnmodifiedPageUpdatesNormally` alongside
`TestUpdate_LocallyEditedPageSkippedWithDiagnostic`,
`TestUnregister_OutcomeAndOnDiskEffect`'s removed/left-alone/quiet triple).

Slice 13's three new Go route tests each assert BOTH halves of R-012's THEN
clause -- an error naming the row AND the row's absence from the returned
path set -- rather than only the error. That is the pairing the first run's
CRITICAL-2 showed was missing.

**Assertion quality**: all assertions verify real behaviour. One structural
caveat, not an assertion defect -- see WARNING-4.

### Quality Metrics

**Formatter**: `gofmt -l longterm-mem engine tui` -- no output, clean.
**Vet**: `go vet ./...` in both `longterm-mem` and `engine` -- no diagnostics.
**Shell syntax**: `bash -n bin/labdrian-overlay`, `bash -n bin/overlay` -- both clean.
**Shell lint**: `shellcheck bin/labdrian-overlay` -- 8 findings, all pre-existing and byte-identical to the `main` baseline; zero new.
**Dead code**: `deadcode` RTA from `cmd/longterm-mem` -- zero findings (was three).

### Disposition of the first run's findings

| First run | Status now | Evidence |
|---|---|---|
| CRITICAL-1 -- uninstall never removes any MCP entry | **CLOSED** | Round trip removes all three entries and restores every config byte-for-byte; both branch directions driven deliberately (B1-B6) |
| CRITICAL-2 -- R-035 implemented in bash only | **CLOSED** | Independent probe against the shipped `engine/skills`: both malformed shapes now error and resolve to nothing; four controls unaffected |
| WARNING-1 -- install invokes a `vaults` subcommand that does not exist | **CLOSED** in code, residue in docs | Call removed from `bin/labdrian-overlay`; `main.go > run` has no `vaults` case. `tasks.md:108` still lists `vaults` in the CLI surface -- folded into WARNING-5 |
| WARNING-2 -- `status` cannot report a degraded Engram connection | **CLOSED** | `cmd_status.go:57-58` reads `Store.Degraded()`; `TestCmdStatus_ReportsDegradedEngramConnection` PASS |
| WARNING-3 -- `os/exec` allowlist skips a real `testdata` package | **OPEN, judgement holds** | now WARNING-4 below |
| WARNING-4 -- documented state-file paths have drifted | **OPEN, judgement holds** | now WARNING-5 below |
| SUGGESTION -- `Action.String()` is dead code with a false doc comment | **CLOSED** | deleted; `UnregisterOutcome.String()`'s comment corrected |
| SUGGESTION -- `--target all` skip is undocumented in the spec | **OPEN** | now SUGGESTION-1 below |
| SUGGESTION -- `TestUnregister_OutcomeAndOnDiskEffect` omits opencode | **OPEN** | now SUGGESTION-2 below |
| SUGGESTION -- TDD table format only from slice 6 | **OPEN** | now SUGGESTION-3 below |

### Issues Found

#### CRITICAL

None. Both of the first run's CRITICAL findings are closed and proved
closed by execution against the shipped artefacts, not by reading the fix
or trusting the suite.

#### WARNING

**WARNING-1 (new) -- the two route parsers still disagree on a one-column
`longterm-mem/**` row.**

`ondisk.go:83-85` skips any line with fewer than two fields before the
`longterm-mem/**` guard at `:93` can see it, so a row that is only a path
is silently ignored. The bash parser rejects the same row:

```text
Go   DeployableManifestPaths("longterm-mem/only-one-field")
       -> paths=[]  err=<nil>                 (silently skipped)

bash route_resolve "longterm-mem/only-one-field"
       -> exit=1
          route_resolve: longterm-mem row 'longterm-mem/only-one-field'
          declares no route (missing third column); must be one of:
          skill, agent, opencode-agent, mcp
```

This does NOT fail R-035's scenario. The scenario's row grammar is
`path tag [route]`, and both parsers reject both shapes it describes
(2-column, and 3-column with an unrecognized value). A one-field line has
no tag either, so it is not a manifest row at all, and Go's `len(fields) <
2` skip is the pre-existing whole-manifest rule applied identically to
every path prefix, not a `longterm-mem` carve-out. The harm the requirement
names -- "silently resolving it to a skills-destination path" -- does not
occur: Go returns nothing for it, and `all_tracked_files` would still send
it through bash's guard on any `apply`, which hard-fails. It is
nevertheless a real divergence in a domain D13 claims both parsers share,
and it is the only such divergence left.

**WARNING-2 (new) -- production behaviour shipped that no task and no apply
record describes.**

`bin/labdrian-overlay:3027-3037` (the `unregister_usable` pre-check for a
missing or non-executable binary) and `:3077-3084` (the exit-2 version-skew
convergence branch), plus the test
`engine/installer/route_test.go > TestUninstall_MissingBinaryStillConverges`,
are all shipped and all correct -- but:

- `tasks.md` slice 13 stops at 13.11; no task covers this branching.
- `apply-progress.md` slice 13's TDD Cycle Evidence table has no row for it,
  and its Test Summary says "**Total tests written**: 6 new" and names six.
  Seven shipped.
- `apply-progress.md` was committed (`052836b`) AFTER the correction landed
  (`d3a540e`), so this is an omission, not a sequencing artefact.

The behaviour itself is documented -- thoroughly -- but only in the commit
message of `d3a540e` ("native review (resilience) caught the first version
trading one failure mode for another ... 127 and 2 converge with an
explicit message, 1 keeps tracking and blocks cleanup because it is
fixable"). A commit message is not the change's apply record. This is the
inverse of the first run's CRITICAL-2 shape: there a ticked task claimed
more than shipped; here shipped code exceeds what the record claims. It
should be reconciled before archive folds the change's history into the
spec baseline.

**WARNING-3 (new) -- the exit-2 convergence branch has no automated test.**

`TestUninstall_MissingBinaryStillConverges` covers the non-executable
binary. Nothing drives a binary that runs and exits 2. The branch at
`:3077-3084` is therefore live production code whose behaviour no test
pins; a later edit could invert it into the blocking arm without any test
failing. Verified manually in this run (proof B3: a stub exiting 2 for all
three targets produces three explicit version-skew warnings, clears
tracking, converges at exit 0, and leaves `install-state.json`'s ownership
records intact so the documented reinstall recovery works).

**WARNING-4 (carried, deliberately left open -- the judgement still holds)
-- the `os/exec` allowlist has a real blind spot.**

`exec_allowlist_test.go:31` still skips any directory named `testdata`, and
`internal/ops/testdata/fixture.go` is still a genuine compiled Go package
(`package testdata`, imported by `status_test.go` and `doctor_test.go`),
not inert fixture data. Re-checked at verify time: nothing under
`internal/ops/testdata/` imports `os/exec`, so R-021 holds today and
`TestOSExecImportAllowlist` PASSES on its own terms. The guard simply would
not notice if that changed. Leaving it open remains defensible -- the
package is test-only, small, and reviewed -- but it is a guard with a hole
in it, not a guard without one.

**WARNING-5 (carried, deliberately left open -- the judgement still holds,
with one new item) -- documented paths and surfaces have drifted.**

| Recorded | Shipped | Where |
|---|---|---|
| `~/.labdrian-overlay/longterm-mem/registration.json` | `longterm-mem-registration.json` directly under `StateDir` | `tasks.md:111` vs `engine/runtime/longtermmem.go:24` |
| DSN `_pragma=query_only(1)` | `_query_only=true` | `apply-progress.md:41` vs `store.go:114` |
| CLI surface includes `vaults` | no `vaults` subcommand exists or is planned | `tasks.md:108` vs `main.go:23-46` |

All three are documentation-only. Install, status and uninstall all use the
same registration constant, so behaviour is internally consistent; the DSN
that shipped is the correct one for the driver; and the `vaults` entry is
new residue from WARNING-1's otherwise-clean fix. None affects a spec
scenario.

**WARNING-6 (new) -- design D1 forbids `immutable=1`; the code ships it.**

`design.md:13` records D1 as: "`modernc.org/sqlite` v1.57.0; DSN
`mode=ro&_query_only=true&_busy_timeout=2000` (`_query_only` key,
`sqlite.go:351`); **no `immutable=1`**". `internal/engram/store.go:117-126`
defines `readOnlyImmutableDSN` = `readOnlyDSN(path) + "&immutable=1"`, and
`Open` (`:72`) retries with it whenever the primary connection fails.

This is a design deviation, not a spec break: the fallback is exactly as
read-only as the primary (`mode=ro` and `_query_only=true` are both still
set), so R-002 holds, and R-020's scoping is unaffected. Slice 13 also
turned it from silent into observable -- `longterm-mem status` now reports
`reachable=true` plus a degraded detail naming the cause. The remaining
defect is that `design.md` still says the opposite of what ships. Either
amend D1 to record the fallback and why, or remove the fallback.

#### SUGGESTION

- **SUGGESTION-1 (carried)** -- `cmdRegister`'s `--target all` skip for a
  runtime with no config file (pinned by
  `TestCmdRegister_AllSkipsRuntimesThatAreNotInstalled`, and paired with
  `TestCmdRegister_NamedTargetWithoutAConfigStillFails`) is real, correct,
  deliberate behaviour that no spec scenario describes. R-016/R-017/R-018
  are silent on multi-target expansion. Worth folding into the spec at
  archive so it is not re-litigated later as an accident.
- **SUGGESTION-2 (carried)** -- `TestUnregister_OutcomeAndOnDiskEffect`
  still runs its removed / left-alone / quiet triple for `claude` and
  `codex` only (`uninstallCases()`, `unregister_test.go:27-41`). opencode
  is covered by the sibling golden-harness tests, so this is asymmetry,
  not a gap.
- **SUGGESTION-3 (carried)** -- the `### TDD Cycle Evidence` table format
  was adopted from slice 6 onward; slices 1-5 carry equivalent narrative
  evidence. Adopting the table retroactively would make the apply record
  uniformly machine-readable.
- **SUGGESTION-4 (new)** -- `--purge`'s forcing behaviour has no automated
  test. `route_test.go:1528` only asserts that the refusal MESSAGE mentions
  `--purge`. Verified manually here (proof B6: `uninstall --target claude
  --purge` with opencode and codex still registered removes the shared
  binary, the engine record and the tracking file while leaving the other
  two entries in place) -- which is the documented escape hatch, but it is
  the one path that can orphan entries and nothing pins it.
- **SUGGESTION-5 (new)** -- cosmetic: `longtermmem_maybe_remove_binary`
  prints `==> longterm-mem binary removed: <path>` unconditionally after
  `rm -f`, so the missing-binary convergence path (proof B2) reports
  removing a binary that was already gone, one line after warning that the
  binary is missing. Harmless, but it reads as contradictory in exactly the
  situation an operator is trying to understand.

### Verdict

**PASS WITH WARNINGS** -- 0 CRITICAL, 6 WARNING, 5 SUGGESTION.

35 of 35 requirements and 82 of 82 scenarios are satisfied, each by a test
observed passing at runtime in this run. All four contract gates are green,
`gofmt`/`go vet` are clean across both Go modules, `shellcheck` adds no new
finding against the `main` baseline, both standing import guards pass
individually, `engine/go.mod` still declares no dependency, and the
`deadcode` RTA that reported three unreachable functions now reports zero
against a working positive control.

Both blocking findings are closed, and closed the way they were found:

- **R-019.** A full install/uninstall round trip through the real
  `bin/labdrian-overlay` removes the ownership-tagged entry from all three
  runtime configs and restores each file byte-for-byte, where the first run
  reproduced `unmanaged` and three orphaned entries. The remediation's
  branching was then driven deliberately in both directions: a real,
  operator-fixable failure keeps the target tracked and preserves the
  binary and the engine record while returning exit 1 (and recovers on
  re-run once fixed), whereas a missing binary and a version-skewed binary
  converge with explicit, truthful messages whose documented recovery was
  executed and confirmed to work. Neither direction produces a wrong
  outcome.
- **R-035.** An independent probe against the shipped `engine/skills`
  package -- not the test -- shows both malformed row shapes now returning
  an explicit error naming the row and resolving to no skills-destination
  path, with four control rows proving the guard is scoped rather than
  blanket, and the real repository manifest parsing clean.

The six warnings are all genuine and none blocks archive. Two are the
first run's, re-examined rather than re-copied, and both judgements to
leave them open still hold on today's evidence. Four are new: one narrow
parser divergence on a row shape outside R-035's grammar, one untested
production branch, one design decision that says the opposite of what
ships, and -- the one worth acting on first -- an apply record that
describes six new tests and ten tasks where seven tests and a further
production branch shipped.

That last one is a bookkeeping gap, not a behavioural one, but it is the
same class of defect the first run's CRITICAL-2 was: a mismatch between
what the change's record claims and what the change actually contains.
Archive folds that record into the baseline. Reconciling `tasks.md` and
`apply-progress.md` slice 13 with the shipped `unregister_usable` and
exit-2 branches, and adding a test for the exit-2 branch, would close
WARNING-2 and WARNING-3 together.

This change is archive-ready. Recommended next phase: `sdd-archive`.
