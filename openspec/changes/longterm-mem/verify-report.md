```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:e677fe3f09d1da6ea30ba3b85bb9b005c28a1de9d3c5a5a135f16e1efd0b1529
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 35/35
scenarios: 82/82
test_command: cd longterm-mem && go test -count=1 ./... && cd ../engine && go test -count=1 ./... && cd ../tui && go test -count=1 ./...
test_exit_code: 0
test_output_hash: sha256:61ae27d328f6231060df4deccfa313f4c9f602a8f02edfe9f48efb6e5dd5d5d0
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
**Branch**: `feat/lm/longterm-mem-verifyfix-13g-verify-report2` @ `29ec9a4`
(tree `23e008a6946982bde7d18f2eb581c3ed5fa7d63b`, working tree clean)
**Run**: third verification. Run 1 returned FAIL (2 CRITICAL). Run 2 returned
PASS WITH WARNINGS after remediation. Commit `496b50f` then closed three of
run 2 findings, so the report on disk described an earlier tree; this run
re-derives everything against the current one.

`evidence_revision` is `sha256` of the full 40-character HEAD tree object id,
computed before this report was written into the tree.

Nothing was carried forward from run 2. Requirement and scenario counts were
re-derived from the nine delta spec files, every gate was re-executed, every
cited test was confirmed present on disk, and both former CRITICAL findings
were re-proved by execution against the shipped artefacts rather than by
re-running the tests written to close them.

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 182 |
| Tasks complete | 182 |
| Tasks incomplete | 0 |
| Requirements (distinct, delta specs) | 35 |
| Scenarios (delta specs) | 82 |
| Requirements fully satisfied | 35 |
| Scenarios compliant | 82 |

Native `gentle-ai sdd-status longterm-mem --json --instructions` reports
`taskProgress 182/182`, `allComplete: true`, `applyState: all_done`,
`verify: ready`. Counted independently from `tasks.md`: 182 checkbox rows,
182 ticked, zero unticked. Slice 13 grew from 11 tasks to 14 (13.12 through
13.14 added by `496b50f`), which is the whole of the 179 to 182 delta.

#### Count derivation (re-derived, not copied)

Requirement blocks per delta spec file, counted from `### Requirement:`
headings, and scenarios from `#### Scenario:` headings:

| Spec | Requirement blocks | Scenarios |
|---|---|---|
| `longterm-mem-memory-access` | 9 | 12 |
| `longterm-mem-promotion` | 10 | 25 |
| `longterm-mem-ops` | 4 | 11 |
| `longterm-mem-query` | 4 | 7 |
| `longterm-mem-mcp-registration` | 4 | 11 |
| `longterm-mem-install` | 2 | 4 |
| `overlay-agent-route` | 2 | 7 |
| `runtime-lifecycle` | 1 | 3 |
| `skills-ondisk-validation` | 1 | 2 |
| **Total** | **37** | **82** |

37 requirement blocks resolve to **35** distinct change-level requirement IDs.
The mapping is the `Traces to: longterm-mem R-nnn` line each block carries,
read block by block rather than by counting ID mentions in prose:

- `longterm-mem-install` "Install Builds, Copies, and Registers" and
  `runtime-lifecycle` "longterm-mem Component Registration" both trace R-014.
- `overlay-agent-route` "Route Resolution by Manifest Route Column" (local
  ID R-006) and `skills-ondisk-validation` "On-Disk Gate Excludes mcp-Routed
  Rows" (local ID R-012) both trace R-013.

37 minus those 2 duplicates is 35, and the union of trace targets is exactly
R-001 through R-035 with no gaps. Note that delta specs carry their own
capability-local `ID:` values, which are NOT the change-level IDs: reading
`ID:` instead of `Traces to:` yields a different and wrong mapping.

### Build and Tests Execution

**Tests**: all green. Every module was re-run with `-count=1`, so no result
came from the build cache. These are the contract broad gates from
`entry.json` `strict_tdd.test_commands.broad`.

```text
cd longterm-mem && go test -count=1 ./...                            exit 0
  longterm-mem                     ok  0.003s
  longterm-mem/cmd/longterm-mem    ok  1.200s
  longterm-mem/internal/engram     ok  0.413s
  longterm-mem/internal/mcpserver  ok  0.693s
  longterm-mem/internal/ops        ok  0.023s
  longterm-mem/internal/promote    ok  0.618s
  longterm-mem/internal/query      ok  0.185s
  longterm-mem/internal/register   ok  0.093s
  longterm-mem/internal/vault      ok  0.469s
  longterm-mem/internal/vaultreg   ok  0.014s

cd engine && go test -count=1 ./...                                  exit 0
  assets, cmd, gadu, gate, installer (15.355s), prespec,
  propagator, runtime, settings, skills  -- all ok

cd tui && go test -count=1 ./...                                     exit 0
  tui  ok  5.127s

bash -n bin/labdrian-overlay                                         exit 0
bash -n bin/overlay                                                  exit 0
```

21 packages, all `ok`. `engine/installer` rose from 12.7s in run 1 and 15.0s
here, consistent with the new integration test added by `496b50f`.

**Zero skipped tests.** Both modules were additionally re-run with `-v` and
scanned for `--- SKIP`: no matches in `longterm-mem`, no matches in `engine`.
Every environment-gated test (`-short`, `pgrep`, `python3`, root) executed.

**Static analysis**:

```text
gofmt -l longterm-mem engine tui   exit 0, no output (all files formatted)
cd longterm-mem && go vet ./...    exit 0, no diagnostics
cd engine && go vet ./...          exit 0, no diagnostics
```

**shellcheck** (`shellcheck -f gcc bin/labdrian-overlay`): exit 1, 8 findings.
Every one is pre-existing. The `main` baseline was extracted with
`git show main:bin/labdrian-overlay` and linted with the same shellcheck
binary; both runs produce an identical set of eight findings with identical
codes and identical columns, differing only in line numbers shifted by this
change additions.

| Code | main | this branch | Verdict |
|---|---|---|---|
| SC2094 (note) | 264:11, 265:53 | 296:11, 297:53 | unchanged |
| SC2016 (note) | 297:12, 301:8 | 329:12, 333:8 | unchanged |
| SC2064 (warning) | 957:23, 1118:23, 1310:25, 1341:23 | 1039:23, 1200:23, 1433:25, 1464:23 | unchanged |

**Zero new shellcheck findings.** The `cmd_longterm_mem` block this change
owns produces none, including the `unregister_usable` pre-check and the
exit-status `case` added by slice 13.

**Standing import guards**, both executed individually, both PASS:

| Guard | Location | Result |
|---|---|---|
| `TestOSExecImportAllowlist` | `longterm-mem/exec_allowlist_test.go` | PASS |
| `TestZeroFetchImportAllowlist` | `engine/skills/zero_fetch_test.go` | PASS |

Checked independently of the guards themselves: an AST-free scan for the
string `os/exec` across the whole `longterm-mem` module returns five files.
Three are `_test.go`. Of the two non-test files, `internal/vault/runner.go`
is the real importer and `internal/ops/doctor.go` mentions `os/exec` only in
a doc comment explaining why it does NOT import it. R-021 holds.
`engine/go.mod` is still two lines, a `module` directive and `go 1.21`, with
no `require` block at all.

**Coverage** (per package, `go test -count=1 -cover`):

| Package | Coverage | Rating |
|---|---|---|
| `internal/ops` | 89.0% | Excellent |
| `internal/query` | 85.1% | Excellent |
| `internal/promote` | 84.6% | Excellent |
| `internal/vault` | 84.1% | Excellent |
| `internal/engram` | 82.9% | Excellent |
| `internal/register` | 78.2% | Acceptable |
| `internal/mcpserver` | 77.8% | Acceptable |
| `internal/vaultreg` | 67.2% | Low |
| `cmd/longterm-mem` | 55.6% | Low |
| root `guard` package | no statements | n/a |

Average across measurable packages: 78.3%, unchanged from run 2. Coverage is
informational and does not gate this verdict.

### The three closures claimed by commit 496b50f

Each was checked against the tree, not against the commit message.

#### Closure 1 -- run 2 WARNING-3 (exit-2 convergence branch untested): HOLDS

`engine/installer/route_test.go` gained
`TestUninstall_VersionSkewStillConverges` (70 added lines). It executes and
passes in this run, alongside the three sibling uninstall tests:

```text
--- PASS: TestInstall_UninstallRoundTripRemovesTheMcpEntry (0.57s)
--- PASS: TestUninstall_HardFailureKeepsTrackingAndSharedBinary (0.55s)
--- PASS: TestUninstall_MissingBinaryStillConverges (0.58s)
--- PASS: TestUninstall_VersionSkewStillConverges (0.56s)
```

It is not merely present, it provably reaches the branch it claims to pin.
The test installs for real, unlinks the installed binary (required: the
just-executed file is still mapped, so an in-place rewrite fails with
`ETXTBSY`), writes a `0755` stub that exits 2, and then asserts the run
converges, names the skew, and clears tracking.

The load-bearing detail is that the stub is executable, so the
`unregister_usable` pre-check at `bin/labdrian-overlay:3033-3037` passes and
control reaches the `case` at `:3069`. The assertion string `version skew`
occurs exactly once in `bin/labdrian-overlay`, at `:3082`, inside the exit-2
arm. The missing-binary arm at `:3036` emits a different message and cannot
satisfy that assertion, so the two convergence tests cannot pass through the
same branch. The remaining arm (`*`, exit 1) is the deliberately blocking one
and is pinned separately by
`TestUninstall_HardFailureKeepsTrackingAndSharedBinary`. All three arms of
the exit-status branch now have an automated test.

#### Closure 2 -- run 2 WARNING-2 (record understates what shipped): PARTIAL

Run 2 named three distinct defects. Only two are closed.

| Defect named by WARNING-2 | Status | Evidence |
|---|---|---|
| No task covers the `unregister_usable` pre-check or the exit-2 branch | **CLOSED** | `tasks.md` 13.12 describes both, ticked |
| No task covers `TestUninstall_MissingBinaryStillConverges` | **CLOSED** | `tasks.md` 13.13, ticked; 13.14 adds the new version-skew test |
| The slice 13 TDD Cycle Evidence table has no row for that branching | **OPEN** | the table at `apply-progress.md:4499` still covers only 13.1 through 13.10 |

The task-list half is fully and accurately closed. The apply-record half is
not, and the edit that was made introduced a fresh internal contradiction:
of the three lines in the same `### Test Summary` block, only the first was
updated.

```text
apply-progress.md:4511  Total tests written: 8 new (... eight named ...)   <- corrected
apply-progress.md:4512  Total tests passing: all 6 new, plus every ...     <- still 6
apply-progress.md:4513  Layers used: Unit (3), Integration (3 -- ...)      <- still totals 6
```

Counted from the eight names the corrected line itself lists, the true layer
split is Unit 3 (`ondisk_test.go`) and Integration 5
(`TestInstall_UninstallRoundTripRemovesTheMcpEntry`,
`TestUninstall_HardFailureKeepsTrackingAndSharedBinary`,
`TestUninstall_MissingBinaryStillConverges`,
`TestUninstall_VersionSkewStillConverges`,
`TestCmdStatus_ReportsDegradedEngramConnection`). The record now says eight
were written and six passed, which is not true of any tree.

This is carried forward as WARNING-1 below. It is a record-accuracy defect
of exactly the class WARNING-2 identified, and archive folds this record into
the spec baseline, so it should be finished rather than declared done.

#### Closure 3 -- run 2 WARNING-6 (design D1 contradicted the code): HOLDS

`design.md` D1 previously ended `no immutable=1`. It now reads, in relevant
part, that `immutable=1` is not the primary DSN but that `Open` retries with
it when the primary read-only connection cannot be established, that the
fallback is stale but never unsafe and always still `mode=ro&_query_only=true`,
and that `Store.Degraded()` reports the state so `status` can surface it.

Every clause of that amendment was checked against
`longterm-mem/internal/engram/store.go` and holds:

| D1 clause | Code | Holds |
|---|---|---|
| `immutable=1` is not the primary DSN | `readOnlyDSN` returns `mode=ro&_txlock=deferred&_pragma=busy_timeout(2000)&_query_only=true` | Yes |
| `Open` retries with it on primary failure | `Open` calls `openAndPing(readOnlyImmutableDSN(path))` only after the primary `openAndPing` fails | Yes |
| the fallback is still `mode=ro` and `_query_only=true` | `readOnlyImmutableDSN` is literally `readOnlyDSN(path) + "&immutable=1"` | Yes |
| `Store.Degraded()` reports it | `Open` returns `degraded: true` with the primary error as cause; `Degraded()` returns both | Yes |
| `status` surfaces it | `cmd_status.go` `EngramReachable` reads `store.Degraded()` and returns a detail naming the stale fallback | Yes |

The design record and the shipped code now agree. R-002 is unaffected either
way, because both DSNs are read-only.

### CRITICAL closure, re-proved by execution on this tree

A remediation that passed once is not the same claim as a remediation that
still holds today, so both former CRITICAL findings were re-driven through
the shipped artefacts rather than by re-reading the fix or re-running the
tests written to close them.

#### CRITICAL-1 (R-019) -- CLOSED

**Execution proof.** The `longterm-mem` binary was built fresh from this tree
(`go build ./cmd/longterm-mem`) into a scratch directory, and driven directly
through a full register/unregister round trip across all three runtimes, with
a sandbox config root per runtime and one shared `--state-dir`. Each runtime
was seeded with an unrelated pre-existing MCP entry first.

```text
register   --target claude    -> longterm-mem: register: claude: ok
register   --target opencode  -> longterm-mem: register: opencode: ok
register   --target codex     -> longterm-mem: register: codex: ok

install-state.json after install
  targets: claude / codex / opencode, each with a fingerprint

.claude.json      unrelated + ownership-tagged longterm-mem entry
opencode.json     unrelated + ownership-tagged longterm-mem entry
config.toml       [mcp_servers.unrelated] + [mcp_servers.longterm-mem]
```

Selective removal was then driven one target at a time.

```text
unregister --target claude    -> longterm-mem: unregister: claude: removed   (exit 0)

.claude.json      longterm-mem entry GONE, unrelated entry intact
opencode.json     longterm-mem entry STILL PRESENT
config.toml       [mcp_servers.longterm-mem] STILL PRESENT
install-state.json  claude record removed; codex and opencode records kept

unregister --target opencode  -> removed  (exit 0)
unregister --target codex     -> removed  (exit 0)
install-state.json  {"schema": 1, "targets": {}}
```

`diff -r` against a pristine snapshot taken before any registration reports
**no difference in any of the three configuration files**. The only additions
anywhere in the sandbox are the `.bak` backups the writers create by design
and the state directory. All three entries were REMOVED, not reported
`unmanaged`, which is the exact inverse of the run 1 reproduction.

R-019 scenario 2 was driven separately: claude was re-registered, its
ownership record deleted so the entry became untagged, and `unregister` run
again.

```text
unregister --target claude  ->  exit 6
  longterm-mem: unregister: claude: unmanaged (an entry exists that
  longterm-mem does not own; left untouched)

.claude.json  longterm-mem entry STILL PRESENT, byte-for-byte
```

**Entrypoint proof.** The defect itself was `bin/labdrian-overlay` passing
`--state-dir "$STATE_DIR"` to `unregister` while `register` used the module
default, so the two could never agree on where `install-state.json` lives.
Every `--state-dir` occurrence in the file was re-read:

| Line | Call | `--state-dir` |
|---|---|---|
| 3007 | `longterm-mem register --target "$t"` | absent |
| 3068 | `longterm-mem unregister --target "$t"` | absent |
| 3009, 3014, 3121 | `engine runtime install/status/uninstall` | present, and correct -- that is the engine registration record, a different file |

Both module-owned calls now resolve `defaultRegisterStateDir()`, which is
`~/.labdrian-overlay/longterm-mem` (`register_paths.go:41-47`). The exit
status is captured into `unregister_exit` and branched on at `:3067-3093`,
not swallowed.

**Suite corroboration.**
`TestInstall_UninstallRoundTripRemovesTheMcpEntry` builds and runs the real
binary through the real entrypoint and deliberately points `STATE_DIR`
outside the home directory, which is the exact condition that hid the
defect. It passed here.

**Limitation, stated rather than papered over.** Run 2 additionally drove
`bin/labdrian-overlay longterm-mem install|uninstall` end to end under a
sandbox home directory. This session refuses any command that overrides
`HOME`, so that particular shape could not be repeated, and the induced-failure
matrix run 2 labelled B1 through B6 was not re-driven by hand. What replaces
it here is stronger than re-reading the fix and weaker than run 2 sandbox:
the module-owned selective-removal semantics were proved by direct execution
of a freshly built binary, the entrypoint state-dir agreement was proved by
reading both call sites, and the entrypoint-level round trip and both
convergence branches were proved by integration tests that drive the real
script and the real binary. No part of the CRITICAL-1 claim rests on
unexecuted code.

#### CRITICAL-2 (R-035) -- CLOSED

Re-probed with an independent program in a scratch module with a `replace`
directive onto this worktree, linked against the shipped `engine/skills`
package and calling `DeployableManifestPaths` directly. No test file in the
repository was involved.

```text
=== DEFECTIVE SHAPES (R-035 must reject) ===
unrouted-2col          paths=[]  err=ondisk: manifest row
                       "longterm-mem/internal/foo.go" under longterm-mem/**
                       declares no route (missing third column); must be one
                       of: skill, agent, opencode-agent, mcp
unrecognized-route     paths=[]  err=ondisk: manifest row
                       "longterm-mem/internal/bar.go" under longterm-mem/**
                       declares an unrecognized route "bogus"; ...
empty-3rd-col-ws       paths=[]  err=(same "no route" error)

=== CONTROLS (must be unaffected) ===
ltm mcp-routed         paths=[]                            err=<nil>
ltm agent-routed       paths=[]                            err=<nil>
ltm opencode-agent     paths=[]                            err=<nil>
ltm skill-routed       paths=[longterm-mem/z.md]           err=<nil>
non-ltm unrouted       paths=[skills/foo/SKILL.md]         err=<nil>
non-ltm bogus route    paths=[skills/bar/SKILL.md]         err=<nil>
ltm prefix-lookalike   paths=[longterm-memories/thing.go]  err=<nil>
ltm one-column-only    paths=[]                            err=<nil>

=== REAL REPOSITORY MANIFEST ===
err=<nil>  deployable rows=83
longterm-mem rows resolving to a skills destination: 0
```

Both shapes run 1 proved defective now return an explicit error naming the
row AND resolve to nothing. The controls prove the guard is scoped rather
than blanket: all four valid routes still parse, a non-`longterm-mem` row
with a bogus route still falls through to the skill default, and
`longterm-memories/` does not match the `longterm-mem/` prefix. The real
`overlay.manifest` parses clean with 83 deployable rows and zero
`longterm-mem` rows resolving to a skills destination.

The implementation matches what the remediation claims:
`ondisk.go:48-53` declares `validLongtermMemRoutes` as an independent
four-value set, and `:93-98` applies it only to rows under `longterm-mem/`,
before the `nonSkillRoutes` exclusion at `:100`. It is genuinely not
`nonSkillRoutes` plus an entry, since `nonSkillRoutes` answers a different
question and correctly omits `skill`.

The `ltm one-column-only` row is the residual parity gap carried as
WARNING-2 below; it is outside this scenario row grammar.

### Spec Compliance Matrix

Every row was checked against production code AND a covering test observed
passing at runtime in this run. Verification method for this run: the whole
suite was executed with `-count=1` and zero skips, every test function name
cited below was confirmed present on disk by extracting all 170 `func Test`
names from the 36 `longterm-mem` test files and all names from the 36
`engine` test files and matching each citation against that list, and the
scenario-named sub-tests were re-run with `-v` so their names could be read
back literally.

#### longterm-mem-memory-access (9 requirements, 12 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-001 | Component builds as an independent module | `longterm-mem/go.mod`, `cmd/longterm-mem/main.go` | `main_test.go > TestMain_BuildsIndependentModule` | COMPLIANT |
| R-001 | engine zero-dependency gate stays green | `engine/go.mod` (no require block) | `engine/skills/zero_fetch_test.go > TestZeroFetchImportAllowlist` | COMPLIANT |
| R-002 | Default connection is read-only | `internal/engram/store.go > Open`, `readOnlyDSN` | `store_test.go > TestOpen_DefaultIsReadOnly` | COMPLIANT |
| R-002 | Overridden connection stays read-only | same | `store_test.go > TestOpen_OverridePathStaysReadOnly` | COMPLIANT |
| R-020 | Soft-deleted and other-project observations excluded | `store.go > ListObservations` | `store_test.go > TestListObservations_ScopesProjectAndExcludesSoftDeleted` | COMPLIANT |
| R-021 | No subprocess call to Engram CLI | `internal/vault/runner.go` (sole `os/exec` importer) | `exec_allowlist_test.go > TestOSExecImportAllowlist` | COMPLIANT |
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
| R-027 | Type mapped onto the vault contract enum | `internal/promote/page.go`, `frontmatter.go` | `page_test.go > TestEmitPage_TypeMappedOntoVaultEnum` | COMPLIANT |
| R-027 | Related links resolve | `page.go`, `internal/engram/relations.go` | `page_test.go > TestEmitPage_RelatedLinksResolve` | COMPLIANT |
| R-027 | Filename survives a retitle | `internal/promote/address.go` | `page_test.go > TestEmitPage_FilenameSurvivesRetitle` | COMPLIANT |
| R-027 | Freshly promoted page passes the vault lint | `internal/promote/lint.go > LintPage` | `lint_test.go > TestLintPage_FreshlyPromotedPagePasses` | COMPLIANT |
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
| R-019 | Selective removal across all three runtimes | `internal/register/unregister.go`; `bin/labdrian-overlay:3007` and `:3068`, both resolving the module default state dir | `unregister_test.go > TestUnregister_SelectiveRemovalAcrossAllThreeRuntimes`, `route_test.go > TestInstall_UninstallRoundTripRemovesTheMcpEntry`, plus this run direct-binary round trip across all three runtimes | COMPLIANT |
| R-019 | Untagged entry preserved and reported, not removed | `unregister.go > UnregisterUnmanaged`, `cmd_unregister.go` (exit 6) | `unregister_test.go > TestUnregister_UntaggedEntryPreservedAndReported`, plus this run exit-6 execution proof | COMPLIANT |
| R-019 | Partial uninstall does not remove the shared binary | `installstate.go > Delete`, `longtermmem_maybe_remove_binary` | `unregister_test.go > TestUnregister_PartialUninstallKeepsSharedBinary`, `TestStatusUninstall_SkipBuildStep/UninstallSingleTargetSkipsBuildAndLeavesBinaryInPlace` | COMPLIANT |

#### overlay-agent-route (2 requirements, 7 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-013 | Agent-routed file deployed to claude agents only | `route_resolve` | `route_test.go > TestRouteResolve_GADUAgentRow` | COMPLIANT |
| R-013 | Skill-routed file deployed to three skills destinations | same | `route_test.go > TestRouteResolve_GADUSkillRow` | COMPLIANT |
| R-013 | Existing two-column rows default to skill route | same | `route_test.go > TestRouteResolve_LegacySkillRow` | COMPLIANT |
| R-013 | Bash dispatch recognizes the mcp route | `route_resolve` mcp branch | `route_test.go > TestRouteResolve_McpRow`, `TestApply_InvokesLongtermMemInstallOnceForMcpRow` | COMPLIANT |
| R-013 | Go route handling recognizes the mcp route | `ondisk.go > nonSkillRoutes` | `ondisk_test.go > TestDeployableManifestPaths_ExcludesMcpRoute`, `TestRouteDomain_MatchesBashAndGo` | COMPLIANT |
| R-013 | opencode-agent route is unaffected | `route_resolve` | `route_test.go > TestRouteResolve_OpencodeAgentUnaffected` | COMPLIANT |
| R-035 | Unrouted longterm-mem row is rejected by both parsers | bash `route_reject_unrouted_longterm_mem`, Go `ondisk.go > validLongtermMemRoutes` at `:93-98` | `route_test.go > TestRouteResolve_UnroutedLongtermMemRowRejected` and `..._UnrecognizedRouteLongtermMemRowRejected` (bash); `ondisk_test.go > TestDeployableManifestPaths_RejectsUnroutedLongtermMemRow`, `..._RejectsUnrecognizedRouteLongtermMemRow`, `..._LongtermMemRouteGuardAcceptsEveryValidRoute` (Go); plus this run independent probe | COMPLIANT, with a residual one-column parity gap outside this scenario row grammar (WARNING-2) |

#### skills-ondisk-validation (1 requirement, 2 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-013 (ondisk) | mcp-routed row not required under skills dir | `ondisk.go > DeployableManifestPaths`, `nonSkillRoutes["mcp"]` | `ondisk_test.go > TestDeployableManifestPaths_ExcludesMcpRoute`, `TestRepositorySkillsAreFullyRegistered` | COMPLIANT |
| R-013 (ondisk) | mcp-routed row not a false UNREGISTERED_ON_DISK | `ondisk.go > ScanSkillFiles`/`DiffOnDisk` | `ondisk_test.go > TestRepositorySkillsAreFullyRegistered` (real `overlay.manifest`, zero divergences; this run probe reads 83 deployable rows with no error) | COMPLIANT |

**Compliance summary**: 82/82 scenarios compliant, 0 FAILING, 0 UNTESTED.
35/35 requirements fully satisfied.

### Reachability Audit

`golang.org/x/tools/cmd/deadcode` RTA from `cmd/longterm-mem` over the whole
module reports **zero** unreachable functions.

```text
cd longterm-mem && deadcode -test=false ./cmd/longterm-mem   (no output)
cd longterm-mem && deadcode ./cmd/longterm-mem               (no output)
cd longterm-mem && deadcode -test=false ./...                (no output)
```

**Positive control**, so an empty result is evidence rather than a broken
tool: the same binary, run against `engine/cmd` in this same worktree in the
same session, reports 16 unreachable functions.

```text
cd engine && deadcode -test=false ./cmd
  gadu/gadu.go:179          unreachable func: PersonaBody
  prespec/grid.go:43,64,87,105,134
  propagator/propagator.go:181
  runtime/runtime.go:97,101,105,118,143,159,163,173,251
```

None of the 16 is in a file this change touched.
`engine/runtime/longtermmem.go` and `engine/skills/ondisk.go` are both fully
reachable.

Disposition of the three functions run 1 named, re-checked on this tree:

| Function | Disposition | Evidence |
|---|---|---|
| `internal/engram/store.go > Store.Degraded` | WIRED into production | `cmd_status.go` `EngramReachable` reads it and returns a degraded detail; `TestCmdStatus_ReportsDegradedEngramConnection` PASS in this run |
| `internal/engram/store.go > Store.Path` | DELETED | no `func (s *Store) Path` anywhere in the module |
| `internal/register/decide.go > Action.String` | DELETED | no `Action) String` anywhere in the module |

The dead shell-side call site is also still gone: `bin/labdrian-overlay` no
longer invokes `longterm-mem vaults seed`, and `cmd/longterm-mem/main.go`
has cases only for `index`, `query`, `sync`, `status`, `doctor`, `promote`,
`mcp`, `register`, `unregister` -- no `vaults`. Only a historical comment at
`:2948` mentions the removed call, correctly in the past tense.

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| D1 read-only Engram DSN | **Yes, now that D1 is amended** | `readOnlyDSN` is `mode=ro` + `_query_only=true` + `_txlock=deferred` + `_pragma=busy_timeout(2000)`. `Open` retries with `readOnlyImmutableDSN`, which is that string plus `&immutable=1`, and marks the store degraded. `496b50f` amended D1 to record exactly this. Run 2 WARNING-6 is closed. |
| D4 one writer per file | Yes | `LongtermMemAdapter` writes only its registration record; runtime configs are written only by `internal/register`. |
| D5 vault registry, lazy seed, flag>env>row | Yes | `vaultreg.Resolve`; removing the `vaults seed` call did not regress it (`TestSeed_OnlyWhenFileAbsent`, `TestResolve_DefaultSeedEntryForOverlayProject`). |
| D6 sidecar precedence store, tmp+fsync+rename | Yes | `promote/store.go`, `vaultreg.writeJSONAtomic`, `installstate.Save`. |
| D9 one `Decide` semantics table for install and uninstall | Yes | `jsonInstall`/`tomlInstall`/`jsonUninstall`/`tomlUninstall` all call `Decide`. |
| D9 install and uninstall share one install-state location | Yes | Both call sites omit `--state-dir`; every remaining `--state-dir` in the entrypoint belongs to an `engine runtime` call. |
| D13 four-value route domain shared by bash and Go | Yes, with one narrow gap | Both parsers reject a missing or unrecognized route on `longterm-mem/**`. Bash additionally rejects a one-column row; Go skips it silently. See WARNING-2. |
| State-file paths recorded at `tasks.md:111` | Partially | Behaviour is internally consistent, the recorded contract is stale. See WARNING-4. |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD evidence reported | Partial | 10 of 19 slices carry a `### TDD Cycle Evidence` table (slices 6, 7, 8a, 8b, 9, 10a, 10b, 11a, 11b, 13). Slices 1 through 5, 12a and 12b instead record per-task RED/GREEN narrative subsections plus a `### Verification` block with exact commands. Substantive evidence exists for every slice; only the format differs. Run 2 recorded this as 11 slices and as "slices 6 through 13"; the accurate figure is 10, because 12a and 12b use the narrative form too. |
| All tasks have tests | Yes | Every test file named in `apply-progress.md` and `tasks.md` exists on disk; every test function cited in the compliance matrix was matched against the extracted list of real `func Test` names. |
| RED confirmed (test files exist) | Yes | 170 `Test*` functions across 36 test files in `longterm-mem`; 36 test files in `engine`. Slice 13 RED evidence quotes verbatim pre-fix failures for both CRITICALs, for the degraded-status wiring, and for both convergence branches. |
| GREEN confirmed (tests pass now) | Yes | Full suite re-executed with `-count=1`, exit 0 in all three modules, zero skips. |
| Triangulation adequate | Yes | Scenario-shaped table-driven sub-tests name each spec scenario literally; `Decide` table is pinned exhaustively; `..._LongtermMemRouteGuardAcceptsEveryValidRoute` hardcodes the four-value domain rather than iterating the map it guards; all three arms of the uninstall exit-status branch now have a test. |
| Safety net for modified files | Yes | Each slice `### Verification` block records re-running prior slices plus both standing import guards; slice 13 table records "full suite green before edit" for every row. |
| Apply record matches shipped code | **Partially** | `tasks.md` now matches (13.12 through 13.14). `apply-progress.md` slice 13 still has no TDD Cycle Evidence row for that branching, and its Test Summary now contradicts itself. See WARNING-1. |

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit (in-process, no subprocess) | 141 | 34 | `go test` |
| Integration (drives a real compiled binary, the real `bin/labdrian-overlay`, real vault fixtures, or the real SQLite fallback) | 29 in `longterm-mem`, plus 49 across the three `engine` files listed | 5 (`cmd/longterm-mem/main_test.go`, `internal/mcpserver/server_test.go`, `engine/installer/route_test.go`, `engine/skills/ondisk_test.go`, `engine/runtime/longtermmem_test.go`) | `go test` + `bash` + `python3` + `pgrep` |
| E2E (browser) | 0 | 0 | not applicable |
| **Total** | **170 in `longterm-mem`, plus the `engine` and `tui` suites** | **36 + 36** | |

Measurement basis: 170 `func Test` declarations across 36 test files in
`longterm-mem`, of which `cmd/longterm-mem/main_test.go` holds 25 and
`internal/mcpserver/server_test.go` holds 4; the three `engine` files listed
hold 32, 14 and 3 respectively and are counted separately because they are
not part of the 170. The unit row is the remainder. The split is approximate
at the boundary, since a few files mix in-process and subprocess tests.

Environment-gated skips (`-short`, missing `pgrep`, missing `python3`,
running as root) were all inactive: a `-v` run of both modules produced zero
`--- SKIP` lines.

### Assertion Quality

946 `t.Error`/`t.Errorf`/`t.Fatal`/`t.Fatalf` assertions across the
`longterm-mem` module. A scan for tautologies (`expect(true`, `assert(true`,
`if true {`, `== true)`, bare `t.Error()`) across `longterm-mem`,
`engine/skills` and `engine/installer` returns nothing. No assertion-free
tests, no ghost loops over possibly-empty collections, and no orphan
empty-collection assertions were found. Negative cases are paired with
positive ones throughout.

`TestUninstall_VersionSkewStillConverges`, the only new test since run 2,
asserts four distinct things: that the run converges, that the output names
the version skew, that it tells the operator the entry was left behind, and
that tracking no longer lists the target. It is not a smoke test, and the
`version skew` string it matches is unique to the branch it claims to pin.

**Assertion quality**: all assertions verify real behaviour. One structural
caveat, not an assertion defect -- see WARNING-3.

### Quality Metrics

**Formatter**: `gofmt -l longterm-mem engine tui` -- no output, clean.
**Vet**: `go vet ./...` in both `longterm-mem` and `engine` -- no diagnostics.
**Shell syntax**: `bash -n bin/labdrian-overlay`, `bash -n bin/overlay` -- both clean.
**Shell lint**: `shellcheck bin/labdrian-overlay` -- 8 findings, all pre-existing and identical to the `main` baseline; zero new.
**Dead code**: `deadcode` RTA from `cmd/longterm-mem` -- zero findings, with a 16-finding positive control.

### Disposition of run 2 findings

| Run 2 finding | Status now | Evidence |
|---|---|---|
| CRITICAL: none | still none | both run 1 CRITICALs re-proved closed by execution on this tree |
| WARNING-1 -- one-column `longterm-mem/**` row: bash rejects, Go skips | **OPEN, judgement holds** | re-proved by the independent probe. Now WARNING-2 below |
| WARNING-2 -- production behaviour shipped that no task and no apply record describes | **PARTIALLY CLOSED** | `tasks.md` 13.12 through 13.14 close the task half; the apply record half is unfinished and now self-contradictory. Now WARNING-1 below |
| WARNING-3 -- exit-2 convergence branch has no automated test | **CLOSED** | `TestUninstall_VersionSkewStillConverges` executes, passes, and provably drives the exit-2 arm |
| WARNING-4 -- `os/exec` allowlist skips a real `testdata` package | **OPEN, judgement holds** | now WARNING-3 below |
| WARNING-5 -- documented state-file paths and CLI surface have drifted | **OPEN, judgement holds** | all three items re-confirmed; now WARNING-4 below |
| WARNING-6 -- design D1 forbids `immutable=1`, the code ships it | **CLOSED** | D1 amended; every clause of the amendment verified against `store.go` and `cmd_status.go` |
| SUGGESTION-1 -- `--target all` skip undocumented in the spec | OPEN | now SUGGESTION-1 |
| SUGGESTION-2 -- `TestUnregister_OutcomeAndOnDiskEffect` omits opencode | OPEN | now SUGGESTION-2 |
| SUGGESTION-3 -- TDD table format only from slice 6 | OPEN, restated more accurately | now SUGGESTION-3 |
| SUGGESTION-4 -- `--purge` forcing behaviour has no automated test | OPEN | now SUGGESTION-4 |
| SUGGESTION-5 -- cosmetic contradictory binary-removed message | OPEN | now SUGGESTION-5 |

### Issues Found

#### CRITICAL

None. Both run 1 CRITICAL findings are closed, and both were re-proved
closed in this run by execution against the shipped artefacts rather than by
re-running the tests written to close them.

#### WARNING

**WARNING-1 (carried, closure incomplete) -- the slice 13 apply record still
does not match what shipped, and now contradicts itself.**

Run 2 WARNING-2 named three defects. Commit `496b50f` closed the two in
`tasks.md` and left the third, while editing one of three adjacent lines in
`apply-progress.md`:

| Location | Content | State |
|---|---|---|
| `apply-progress.md:4511` | Total tests written: 8 new, with eight names listed | corrected |
| `apply-progress.md:4512` | Total tests passing: all 6 new, plus every pre-existing test | **stale** |
| `apply-progress.md:4513` | Layers used: Unit 3, Integration 3 | **stale**, totals 6 |
| `apply-progress.md:4499` TDD Cycle Evidence table | rows for 13.1 through 13.10 only | **no row for 13.12 through 13.14** |

Derived from the eight names the corrected line itself lists, the true split
is Unit 3 and Integration 5. The record as it stands says eight tests were
written and six passed, which describes no tree that has ever existed.

Severity is WARNING, not CRITICAL: no spec scenario is affected, no task is
unchecked, no test fails, and the shipped behaviour is correct and now fully
tested. But this is a record-accuracy defect of exactly the class the finding
was raised about, in exactly the artefact archive folds into the spec
baseline, and the commit that claimed to close it stopped one line short. It
should be finished before archive rather than recorded as closed.

**WARNING-2 (carried, judgement holds) -- the two route parsers still
disagree on a one-column `longterm-mem/**` row.**

`ondisk.go:83-85` skips any line with fewer than two fields before the
`longterm-mem/**` guard at `:93` can see it, so a row that is only a path is
silently ignored. Re-proved by the independent probe in this run: the Go
parser returns an empty path set and a nil error for
`longterm-mem/only-one-field`. The bash parser rejects the same row, because
`route_reject_unrouted_longterm_mem` at `:414-430` fires for every path
matching the `longterm-mem/` prefix, including a one-column row whose third
field resolves to the empty string, and exits 1 naming the row.

This does NOT fail R-035. That scenario row grammar is path, tag, optional
route, and both parsers reject both shapes it describes. A one-field line has
no tag either, so it is not a manifest row at all, and the Go two-field skip
is the pre-existing whole-manifest rule applied identically to every path
prefix, not a `longterm-mem` carve-out. The harm the requirement names,
silently resolving such a row to a skills-destination path, does not occur:
Go returns nothing for it, and bash hard-fails it on any apply. It remains a
real divergence in a domain D13 claims both parsers share, and it is the only
one left.

**WARNING-3 (carried, judgement holds) -- the `os/exec` allowlist has a real
blind spot.**

`exec_allowlist_test.go` skips any directory named `testdata` during its
walk, and `internal/ops/testdata/fixture.go` is still a genuine compiled Go
package declaring `package testdata`, not inert fixture data. Re-checked at
verify time: it imports `os` but not `os/exec`, so R-021 holds today and
`TestOSExecImportAllowlist` passes on its own terms. It is the only Go file
under any `testdata` directory in the module. The guard simply would not
notice if that changed. Leaving it open remains defensible, since the package
is test-only, small and reviewed, but it is a guard with a hole in it.

**WARNING-4 (carried, judgement holds) -- documented paths and surfaces have
drifted from the code.**

| Recorded | Shipped | Where |
|---|---|---|
| `~/.labdrian-overlay/longterm-mem/registration.json` | `longterm-mem-registration.json` directly under the state dir | `tasks.md:111` vs `engine/runtime/longtermmem.go:24` |
| DSN `_pragma=query_only(1)` | `_query_only=true` | `apply-progress.md:41` vs `store.go:114` |
| CLI surface includes `vaults` | no `vaults` subcommand exists or is planned | `tasks.md:108` vs `main.go:24-40` |

All three are documentation-only and all three were re-confirmed present on
this tree. Install, status and uninstall all use the same registration
constant, so behaviour is internally consistent; the DSN that shipped is the
correct one for the driver; and the `vaults` entry is residue from the
otherwise-clean removal of the seed call. None affects a spec scenario. The
`apply-progress.md` DSN drift is now internally inconsistent as well: line 41
records `_pragma=query_only(1)` while the reconciliation section added at
line 4743 correctly writes `mode=ro&_query_only=true`.

#### SUGGESTION

- **SUGGESTION-1 (carried)** -- the `cmdRegister` `--target all` skip for a
  runtime with no config file, pinned by
  `TestCmdRegister_AllSkipsRuntimesThatAreNotInstalled` and paired with
  `TestCmdRegister_NamedTargetWithoutAConfigStillFails`, is real, correct,
  deliberate behaviour that no spec scenario describes. R-016, R-017 and
  R-018 are silent on multi-target expansion. Worth folding into the spec at
  archive so it is not re-litigated later as an accident.
- **SUGGESTION-2 (carried)** -- `uninstallCases()` in
  `unregister_test.go:27-41` still returns only claude and codex, so
  `TestUnregister_OutcomeAndOnDiskEffect` runs its removed, left-alone and
  quiet triple for two runtimes rather than three. opencode is covered by the
  sibling golden-harness tests, so this is asymmetry, not a gap.
- **SUGGESTION-3 (carried, restated)** -- the `### TDD Cycle Evidence` table
  appears in 10 of 19 slices. Run 2 described the gap as slices 1 through 5;
  in fact slices 12a and 12b also use the narrative form, including 12b, the
  R-019 slice whose defect became run 1 CRITICAL-1. Adopting the table across
  all 19 would make the apply record uniformly machine-readable.
- **SUGGESTION-4 (carried)** -- the `--purge` forcing path still has no
  automated test. `route_test.go:1528-1529` only asserts that the refusal
  message mentions `--purge`. It is the documented escape hatch and the one
  path that can orphan entries, and nothing pins it.
- **SUGGESTION-5 (carried)** -- cosmetic:
  `longtermmem_maybe_remove_binary` removes the binary and then
  unconditionally prints a line saying the binary was removed, so the
  missing-binary convergence path reports removing a binary that was already
  gone, one line after warning that the binary is missing. Harmless, but it
  reads as contradictory in exactly the situation an operator is trying to
  understand.

### Verdict

**PASS WITH WARNINGS** -- 0 CRITICAL, 4 WARNING, 5 SUGGESTION.

35/35 requirements and 82/82 scenarios are compliant, each with a covering
test observed passing at runtime in this run. All 182 tasks are complete.
Every contract gate passes: three module test suites green with `-count=1`
and zero skips, both shell syntax checks clean, `gofmt` and `go vet` clean
across `longterm-mem` and `engine`, zero new shellcheck findings against the
`main` baseline, both standing import guards green when run individually, and
zero `deadcode` findings under a working 16-finding positive control. Both
former CRITICAL findings were re-proved closed by execution against the
shipped artefacts.

Two of the three closures commit `496b50f` claimed hold in full. The third,
run 2 WARNING-2, is only partially closed: the task list was fixed, the apply
record was not, and the partial edit left `apply-progress.md` stating that
eight tests were written and six passed. That is carried above as WARNING-1
rather than declared closed, because it is a record defect in the artefact
archive folds into the spec baseline, and because a third run that launders a
finding is worse than a third run that reports it.

No WARNING blocks archive on its own terms: none breaks a spec scenario, none
leaves a task unchecked, and none corresponds to a failing or missing test.
WARNING-2, WARNING-3 and WARNING-4 were each judged non-blocking by run 2 for
reasons this run re-verified rather than assumed. WARNING-1 is the one the
maintainer should decide on deliberately: it is cheap to finish, it lives in
the record archive makes permanent, and it is the second consecutive run to
report that same block as inaccurate.
