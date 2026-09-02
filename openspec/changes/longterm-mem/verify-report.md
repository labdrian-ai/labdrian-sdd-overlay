```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:42686bf7f8354e19607c76412367c2599236a72a9e4bcfe05d9031e7f467c674
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 35/35
scenarios: 82/82
test_command: go test -C longterm-mem -count=1 ./... && go test -C engine -count=1 ./... && go test -C tui -count=1 ./...
test_exit_code: 0
test_output_hash: sha256:4944a7effd60366a65a6ba919724e789291a82aca2ddb0b9c8ff71ed2a121ca6
build_command: bash -n bin/labdrian-overlay && bash -n bin/overlay && go vet -C longterm-mem ./... && go vet -C engine ./... && gofmt -l longterm-mem engine tui
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: longterm-mem
**Version**: 9 delta specs, R-001 through R-035
**Mode**: Strict TDD
**Artifact store**: openspec (`openspec/changes/longterm-mem/`)
**Worktree**: `/home/labdrian/labdrian-sdd-overlay-worktrees/longterm-mem`
**Branch**: `feat/lm/longterm-mem-verifyfix-13i-verify-report3` @ `5d87290`
(tree `4b4fdf866c15f4b9b0728688b830b9504ae5ddf7`, working tree clean)
**Run**: fourth verification. Run 1 FAIL (2 CRITICAL). Run 2 PASS WITH
WARNINGS. Run 3 PASS WITH WARNINGS, and found that run 2's WARNING-2 fix had
only partially landed. Commit `e7db2ae` then finished that fix, so the run 3
report on disk predates the artefact correction it demanded. This run
re-derives everything against the tree as it will be archived.

`evidence_revision` is `sha256` of the full 40-character HEAD tree object id,
computed before this report was written into the tree — the same derivation
run 3 used.

Nothing was carried forward as fact. Requirement and scenario counts were
re-derived from the nine delta spec files, every gate was re-executed, every
cited test was matched against a `--- PASS` line produced in this run, both
former CRITICAL findings were re-proved by execution against the shipped
artefacts, and every carried warning and suggestion was re-checked against
the current tree rather than restated.

**Command form.** This session's shell refuses compound `cd A && ... && cd B`
chains, so `entry.json`'s `strict_tdd.test_commands.broad` were run as
`go test -C <dir>`, which is the same invocation with the same working
directory. Nothing else about the gates changed.

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
`verify: ready`, `archive: blocked`. Counted independently from `tasks.md`:
182 checkbox rows, 182 ticked, zero unticked.

#### Count derivation (re-derived independently)

Requirement blocks per delta spec file, from `### Requirement:` headings, and
scenarios from `#### Scenario:` headings:

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

37 requirement blocks resolve to **35** distinct change-level requirement IDs
through the `Traces to: longterm-mem R-nnn` line each block carries. Every one
of the 37 blocks has exactly one such line. Two targets are traced twice:

- `longterm-mem-install` "Install Builds, Copies, and Registers" and
  `runtime-lifecycle` "longterm-mem Component Registration" both trace R-014.
- `overlay-agent-route` "Route Resolution by Manifest Route Column" and
  `skills-ondisk-validation` "On-Disk Gate Excludes mcp-Routed Rows" both
  trace R-013.

37 minus those 2 duplicates is 35, and the union of trace targets is exactly
R-001 through R-035 with no gaps.

**Why the capability-local `ID:` line cannot be used.** Run 3 asserted this;
this run measured it. Only 36 of the 37 blocks carry an `ID:` line at all
(`runtime-lifecycle` has none), those 36 lines yield only **33** distinct
values, `R-006` appears twice and `R-012` three times across different files,
and `R-013` and `R-035` do not appear as an `ID:` value anywhere. Counting by
`ID:` would report 33 requirements and would silently drop the two the
CRITICAL findings were raised against. The `Traces to:` line is the only
correct source, and it yields 35.

### Build and Tests Execution

**Tests**: all green, every module re-run with `-count=1` so no result came
from the build cache.

```text
go test -C longterm-mem -count=1 ./...                               exit 0
  longterm-mem                     ok  0.003s
  longterm-mem/cmd/longterm-mem    ok  1.164s
  longterm-mem/internal/engram     ok  0.383s
  longterm-mem/internal/mcpserver  ok  0.660s
  longterm-mem/internal/ops        ok  0.017s
  longterm-mem/internal/promote    ok  0.592s
  longterm-mem/internal/query      ok  0.175s
  longterm-mem/internal/register   ok  0.085s
  longterm-mem/internal/vault      ok  0.454s
  longterm-mem/internal/vaultreg   ok  0.011s

go test -C engine -count=1 ./...                                     exit 0
  assets, cmd, gadu, gate, installer (15.382s), prespec,
  propagator, runtime, settings, skills  -- all ok

go test -C tui -count=1 ./...                                        exit 0
  tui  ok  5.186s

bash -n bin/labdrian-overlay                                         exit 0
bash -n bin/overlay                                                  exit 0
```

21 packages, all `ok`. `test_output_hash` is the sha256 of those 21 `ok`
lines with the elapsed-time column stripped, since timings vary per run and
would otherwise make the digest meaningless as evidence.

**Zero skipped tests, and every cited test observed passing.** Both modules
were re-run with `-v` and every `--- PASS`, `--- FAIL` and `--- SKIP` line
captured: **981 PASS lines, 0 FAIL, 0 SKIP**. Every environment-gated test
(`-short`, `pgrep`, `python3`, root) executed. The 81 distinct test names
cited in the compliance matrix below were then matched mechanically against
that captured list: **81 cited, 0 not observed passing.** No row in the
matrix rests on a test that was merely found on disk.

**Static analysis**:

```text
gofmt -l longterm-mem engine tui   exit 0, no output (all files formatted)
go vet -C longterm-mem ./...       exit 0, no diagnostics
go vet -C engine ./...             exit 0, no diagnostics
```

**shellcheck** (`shellcheck -f gcc bin/labdrian-overlay`): exit 1, 8 findings,
every one pre-existing. The `main` baseline was extracted with
`git show main:bin/labdrian-overlay` and linted with the same shellcheck
0.11.0 binary in this session; both runs produce an identical set of eight
findings with identical codes and identical columns, differing only in line
numbers shifted by this change's additions (2786 lines on `main`, 3181 here).

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

Checked independently of the guards themselves: a string scan for `os/exec`
across the whole `longterm-mem` module returns five files. Three are
`_test.go`. Of the two non-test files, `internal/vault/runner.go:13` is the
real importer and `internal/ops/doctor.go:62` mentions `os/exec` only in a
doc comment explaining why it does NOT import it. R-021 holds.
`engine/go.mod` is still two lines, a `module` directive and `go 1.21`, with
no `require` block at all.

**Coverage** (per package, `go test -C longterm-mem -count=1 -cover ./...`):

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

Average across measurable packages: 78.3%, unchanged from runs 2 and 3 — no
source file changed since run 3. Coverage is informational and does not gate
this verdict.

### Did the record correction land this time?

Run 3 found the run 2 remediation half-applied: of three adjacent count lines
in slice 13's `### Test Summary`, only the first had been corrected, and the
slice 13 `### TDD Cycle Evidence` table had no row for tasks 13.12–13.14.
Commit `e7db2ae` (the only commit since run 3, `apply-progress.md` only,
+22/-2) claims to finish it. Each of the four items run 3 named was re-checked
against the file as it stands, and then against the tree.

| Item run 3 named | State now | Evidence |
|---|---|---|
| `Total tests written` | **CLOSED** | `apply-progress.md:4512` reads `8 new` and lists eight names |
| `Total tests passing` | **CLOSED** | `:4513` reads `all 8 new` (was `all 6 new`) |
| `Layers used` | **CLOSED** | `:4514` reads `Unit (3), Integration (5 ...)` (was `Unit (3), Integration (3 ...)`) |
| TDD Cycle Evidence row for 13.12–13.14 | **CLOSED** | a `13.12–13.14` row now exists at `:4508`, naming `engine/installer/route_test.go`, its layer, safety net, RED, GREEN and triangulation |

All three count lines were read as adjacent lines, not one at a time — the
mistake that let this survive run 2.

**Verified against the tree, not only against the record.** The eight test
names the corrected line lists were located on disk and matched against this
run's captured `--- PASS` output:

| Test | File | Layer | Observed |
|---|---|---|---|
| `TestDeployableManifestPaths_RejectsUnroutedLongtermMemRow` | `engine/skills/ondisk_test.go:94` | Unit | PASS |
| `TestDeployableManifestPaths_RejectsUnrecognizedRouteLongtermMemRow` | `ondisk_test.go:113` | Unit | PASS |
| `TestDeployableManifestPaths_LongtermMemRouteGuardAcceptsEveryValidRoute` | `ondisk_test.go:134` | Unit | PASS |
| `TestInstall_UninstallRoundTripRemovesTheMcpEntry` | `engine/installer/route_test.go:1634` | Integration | PASS |
| `TestUninstall_HardFailureKeepsTrackingAndSharedBinary` | `route_test.go:1702` | Integration | PASS |
| `TestUninstall_MissingBinaryStillConverges` | `route_test.go:1896` | Integration | PASS |
| `TestUninstall_VersionSkewStillConverges` | `route_test.go:1958` | Integration | PASS |
| `TestCmdStatus_ReportsDegradedEngramConnection` | `longterm-mem/cmd/longterm-mem/main_test.go:304` | Integration | PASS |

Three unit and five integration — the layer split the corrected line now
claims, derived from where the tests actually live rather than from the
record. The parenthetical is also true: four of the five integration tests
drive the real compiled `longterm-mem` binary through the real
`bin/labdrian-overlay` (both convergence tests perform a real install first
and only then delete or replace the binary), and the fifth forces the real
SQLite `immutable=1` fallback.

There is no evidence row for task 13.11 and none is expected: 13.11 is the
slice-verification task, not a test-writing one. The other thirteen slice 13
tasks are covered by the six rows in the table.

**Verdict on the closure: the four items run 3 named are genuinely closed.**
The correction is not partial this time. But re-reading the whole slice 13
section rather than only the lines run 3 pointed at turned up a *different*
part of the same section that still describes the pre-correction tree — see
WARNING-1. That is a new finding, not the old one relabelled.

### CRITICAL closure, re-proved by execution on this tree

A remediation that passed once is not the same claim as a remediation that
still holds today, so both former CRITICAL findings were re-driven through the
shipped artefacts rather than by re-reading the fix or re-running the tests
written to close them.

#### CRITICAL-1 (R-019) — CLOSED

**Execution proof.** The `longterm-mem` binary was built fresh from this tree
(`go build -C longterm-mem ./cmd/longterm-mem`) into a scratch directory and
driven directly through a full register/unregister round trip across all
three runtimes, with a sandbox config root per runtime and one shared
`--state-dir`. Each runtime was seeded with an unrelated pre-existing MCP
entry first.

```text
register --target claude   -> longterm-mem: register: claude: ok      exit 0
register --target opencode -> longterm-mem: register: opencode: ok    exit 0
register --target codex    -> longterm-mem: register: codex: ok       exit 0

install-state.json after install
  targets: claude / codex / opencode, each with a fingerprint

.claude.json    unrelated + ownership-tagged longterm-mem entry
opencode.json   unrelated + ownership-tagged longterm-mem entry
config.toml     mcp_servers.unrelated + mcp_servers.longterm-mem
```

Selective removal was then driven one target at a time.

```text
unregister --target claude   -> longterm-mem: unregister: claude: removed   exit 0

.claude.json      longterm-mem entry GONE, unrelated entry intact
opencode.json     longterm-mem entry STILL PRESENT
config.toml       longterm-mem table STILL PRESENT
install-state.json  claude record removed; codex and opencode records kept

unregister --target opencode -> removed  exit 0
unregister --target codex    -> removed  exit 0
install-state.json  schema 1, targets empty
```

`diff -r` against a pristine snapshot taken before any registration reports
**no difference in any of the three configuration files**. The only additions
anywhere in the sandbox are the `.bak` backups the writers create by design.
All three entries were REMOVED, not reported `unmanaged`, which is the exact
inverse of the run 1 reproduction.

R-019 scenario 2 was driven separately in a second sandbox: claude was
registered, its ownership record deleted so the shipped entry became untagged,
and `unregister` run again.

```text
unregister --target claude  ->  exit 6
  longterm-mem: unregister: claude: unmanaged (an entry exists that
  longterm-mem does not own; left untouched)

.claude.json sha256 before: 2811fc4843e580645156cc74ad2da7b769ab49e12fa7e5a5dced186edd41723c
.claude.json sha256 after:  2811fc4843e580645156cc74ad2da7b769ab49e12fa7e5a5dced186edd41723c
  byte-for-byte identical; the longterm-mem entry is still there
```

**Entrypoint proof.** The defect itself was `bin/labdrian-overlay` passing
`--state-dir "$STATE_DIR"` to `unregister` while `register` used the module
default, so the two could never agree on where `install-state.json` lives.
Every `--state-dir` occurrence in the file was re-read:

| Line | Call | `--state-dir` |
|---|---|---|
| 3007 | `longterm-mem register --target "$t"` | absent |
| 3068 | `longterm-mem unregister --target "$t"` | absent |
| 3009, 3014, 3121 | `engine runtime install/status/uninstall` | present, and correct — that is the engine registration record, a different file |

Both module-owned calls resolve `defaultRegisterStateDir()`, which is
`~/.labdrian-overlay/longterm-mem` (`register_paths.go:41-47`). The exit
status is captured into `unregister_exit` at `:3067-3068` and branched on at
`:3069-3093`, not swallowed.

**Suite corroboration.** `TestInstall_UninstallRoundTripRemovesTheMcpEntry`
builds and runs the real binary through the real entrypoint and deliberately
points `STATE_DIR` outside the home directory, which is the exact condition
that hid the defect. It passed here.

**Limitation, stated rather than papered over.** Run 2 additionally drove
`bin/labdrian-overlay longterm-mem install|uninstall` end to end under a
sandbox home directory. This session refuses any command that overrides
`HOME`, so that shape could not be repeated by hand, and the induced-failure
matrix run 2 labelled B1 through B6 was not re-driven manually. Runs 3 and 4
are identical in this respect. What replaces it is stronger than re-reading
the fix and weaker than run 2's sandbox: module-owned selective-removal
semantics proved by direct execution of a freshly built binary, entrypoint
state-dir agreement proved by reading both call sites, and the
entrypoint-level round trip plus all three exit-status arms proved by
integration tests that set `HOME` in their own child process environment and
drive the real script and the real binary. No part of the CRITICAL-1 claim
rests on unexecuted code.

#### CRITICAL-2 (R-035) — CLOSED

Re-probed with an independent program in a scratch module carrying a
`replace` directive onto this worktree, linked against the shipped
`engine/skills` package and calling `DeployableManifestPaths` directly. No
test file in the repository was involved.

```text
=== DEFECTIVE SHAPES (R-035 must reject) ===
unrouted-2col        paths=[]  err=ondisk: manifest row
                     "longterm-mem/internal/foo.go" under longterm-mem/**
                     declares no route (missing third column); must be one
                     of: skill, agent, opencode-agent, mcp
unrecognized-route   paths=[]  err=ondisk: manifest row
                     "longterm-mem/internal/bar.go" under longterm-mem/**
                     declares an unrecognized route "bogus"; ...
empty-3rd-col-ws     paths=[]  err=(same "no route" error)

=== CONTROLS (must be unaffected) ===
ltm mcp-routed       paths=[]                            err=<nil>
ltm agent-routed     paths=[]                            err=<nil>
ltm opencode-agent   paths=[]                            err=<nil>
ltm skill-routed     paths=[longterm-mem/z.md]           err=<nil>
non-ltm unrouted     paths=[skills/foo/SKILL.md]         err=<nil>
non-ltm bogus route  paths=[skills/bar/SKILL.md]         err=<nil>
ltm prefix-lookalike paths=[longterm-memories/thing.go]  err=<nil>
ltm one-column-only  paths=[]                            err=<nil>

=== REAL REPOSITORY MANIFEST ===
err=<nil>  deployable rows=83
longterm-mem rows resolving to a skills destination: 0
```

Both shapes run 1 proved defective now return an explicit error naming the row
AND resolve to nothing. The controls prove the guard is scoped rather than
blanket: all four valid routes still parse, a non-`longterm-mem` row with a
bogus route still falls through to the skill default, and `longterm-memories/`
does not match the `longterm-mem/` prefix. The real `overlay.manifest` parses
clean with 83 deployable rows and zero `longterm-mem` rows resolving to a
skills destination.

The implementation matches what the remediation claims: `ondisk.go:48-53`
declares `validLongtermMemRoutes` as an independent four-value set, and
`:93-98` applies it only to rows under `longterm-mem/`, before the
`nonSkillRoutes` exclusion at `:100`. It is genuinely not `nonSkillRoutes`
plus an entry, since `nonSkillRoutes` answers a different question and
correctly omits `skill`.

The `ltm one-column-only` row is the residual parity gap carried as WARNING-2
below; it is outside this scenario's row grammar.

### Spec Compliance Matrix

Every row was checked against production code AND a covering test matched to a
`--- PASS` line produced in this run. Verification method: the whole suite was
executed with `-count=1` and `-v`, all 981 result lines captured, and each of
the 81 distinct test names below matched mechanically against that capture —
81 cited, 0 not observed passing, 0 FAIL, 0 SKIP.

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
| R-034 | No CLI subcommand leaves a residual process | `cmd/longterm-mem` subcommand table | `main_test.go > TestCLI_NoResidualProcessAfterAnySubcommand` (executed, not skipped) | COMPLIANT |

#### longterm-mem-install (2 requirements, 4 scenarios) and runtime-lifecycle (1 requirement, 3 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-014 | Install builds, copies, then reports per-runtime status | `cmd_longterm_mem`, `engine/runtime/longtermmem.go > Install` | `route_test.go > TestInstall_BuildsCopiesThenReportsPerRuntimeStatus` | COMPLIANT |
| R-014 | Status and uninstall skip the build step | `cmd_longterm_mem`, `LongtermMemAdapter.Status/Uninstall` | `route_test.go > TestStatusUninstall_SkipBuildStep` (4 sub-tests, all PASS) | COMPLIANT |
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
| R-019 | Selective removal across all three runtimes | `internal/register/unregister.go`; `bin/labdrian-overlay:3007` and `:3068`, both resolving the module default state dir | `unregister_test.go > TestUnregister_SelectiveRemovalAcrossAllThreeRuntimes`, `route_test.go > TestInstall_UninstallRoundTripRemovesTheMcpEntry`, plus this run's direct-binary round trip across all three runtimes | COMPLIANT |
| R-019 | Untagged entry preserved and reported, not removed | `unregister.go > UnregisterUnmanaged`, `cmd_unregister.go` (exit 6) | `unregister_test.go > TestUnregister_UntaggedEntryPreservedAndReported`, plus this run's exit-6 execution proof with identical sha256 before and after | COMPLIANT |
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
| R-035 | Unrouted longterm-mem row is rejected by both parsers | bash `route_reject_unrouted_longterm_mem:414-430`, Go `ondisk.go > validLongtermMemRoutes` at `:93-98` | `route_test.go > TestRouteResolve_UnroutedLongtermMemRowRejected` and `..._UnrecognizedRouteLongtermMemRowRejected` (bash); `ondisk_test.go > TestDeployableManifestPaths_RejectsUnroutedLongtermMemRow`, `..._RejectsUnrecognizedRouteLongtermMemRow`, `..._LongtermMemRouteGuardAcceptsEveryValidRoute` (Go); plus this run's independent probe | COMPLIANT, with a residual one-column parity gap outside this scenario's row grammar (WARNING-2) |

#### skills-ondisk-validation (1 requirement, 2 scenarios)

| Requirement | Scenario | Production code | Test | Result |
|---|---|---|---|---|
| R-013 (ondisk) | mcp-routed row not required under skills dir | `ondisk.go > DeployableManifestPaths`, the `mcp` entry in `nonSkillRoutes` | `ondisk_test.go > TestDeployableManifestPaths_ExcludesMcpRoute`, `TestRepositorySkillsAreFullyRegistered` | COMPLIANT |
| R-013 (ondisk) | mcp-routed row not a false UNREGISTERED_ON_DISK | `ondisk.go > ScanSkillFiles`/`DiffOnDisk` | `ondisk_test.go > TestRepositorySkillsAreFullyRegistered` (real `overlay.manifest`, zero divergences; this run's probe reads 83 deployable rows with no error) | COMPLIANT |

**Compliance summary**: 82/82 scenarios compliant, 0 FAILING, 0 UNTESTED.
35/35 requirements fully satisfied.

### Reachability Audit

`golang.org/x/tools/cmd/deadcode` RTA from `cmd/longterm-mem` over the whole
module reports **zero** unreachable functions.

```text
deadcode -test=false ./cmd/longterm-mem   (no output, exit 0)
deadcode ./cmd/longterm-mem               (no output, exit 0)
deadcode -test=false ./...                (no output, exit 0)
```

**Positive control**, so an empty result is evidence rather than a broken
tool: the same binary, run against `engine/cmd` in this same worktree in the
same session, reports 16 unreachable functions.

```text
deadcode -test=false ./cmd   (from engine/)
  gadu/gadu.go:179          unreachable func: PersonaBody
  prespec/grid.go:43,64,87,105,134
  propagator/propagator.go:181
  runtime/runtime.go:97,101,105,118,143,159,163,173,251
```

None of the 16 is in a file this change touched. `engine/runtime/longtermmem.go`
and `engine/skills/ondisk.go` are both fully reachable.

Disposition of the three functions run 1 named, re-checked on this tree:

| Function | Disposition | Evidence |
|---|---|---|
| `internal/engram/store.go > Store.Degraded` | WIRED into production | `cmd_status.go:54-57` reads it and returns a degraded detail; `TestCmdStatus_ReportsDegradedEngramConnection` PASS in this run |
| `internal/engram/store.go > Store.Path` | DELETED | no `func (s *Store) Path` anywhere in the module |
| `internal/register/decide.go > Action.String` | DELETED | no `Action) String` anywhere in the module |

The dead shell-side call site is also still gone: the only `vaults` string
left in `bin/labdrian-overlay` is a historical comment at `:2948`, correctly
in the past tense, and `cmd/longterm-mem/main.go:33-45` has cases only for
`index`, `query`, `sync`, `status`, `doctor`, `promote`, `mcp`, `register`,
`unregister` — no `vaults`.

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| D1 read-only Engram DSN | **Yes, D1 as amended** | `readOnlyDSN` (`store.go:113-115`) is `mode=ro` + `_txlock=deferred` + `_pragma=busy_timeout(2000)` + `_query_only=true`. `Open` (`:67-77`) retries with `readOnlyImmutableDSN`, literally `readOnlyDSN(path) + "&immutable=1"`, only after the primary `openAndPing` fails, and returns the store marked `degraded` with the primary error as cause. `Degraded()` at `:97` reports both, and `cmd_status.go` surfaces it. Every clause of the amendment was checked against the code; run 2's WARNING-6 stays closed. |
| D4 one writer per file | Yes | `LongtermMemAdapter` writes only its registration record; runtime configs are written only by `internal/register`. |
| D5 vault registry, lazy seed, flag>env>row | Yes | `vaultreg.Resolve`; removing the `vaults seed` call did not regress it (`TestSeed_OnlyWhenFileAbsent`, `TestResolve_DefaultSeedEntryForOverlayProject`, both PASS). |
| D6 sidecar precedence store, tmp+fsync+rename | Yes | `promote/store.go`, `vaultreg.writeJSONAtomic`, `installstate.Save`. |
| D9 one `Decide` semantics table for install and uninstall | Yes | `jsonInstall`/`tomlInstall`/`jsonUninstall`/`tomlUninstall` all call `Decide`. |
| D9 install and uninstall share one install-state location | Yes | Both call sites omit `--state-dir`; every remaining `--state-dir` in the entrypoint belongs to an `engine runtime` call. Proved by execution this run. |
| D13 four-value route domain shared by bash and Go | Yes, with one narrow gap | Both parsers reject a missing or unrecognized route on `longterm-mem/**`. Bash additionally rejects a one-column row; Go skips it silently. See WARNING-2. |
| Rollback path `uninstall --target all --purge` | Recorded, untested | `design.md:60` names it as the rollback procedure; nothing automated exercises the forcing branch. See SUGGESTION-4. |
| State-file paths recorded at `tasks.md:107,108,111` | Partially | Behaviour is internally consistent, the recorded contract is stale. See WARNING-4. |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD evidence reported | Partial | 10 of 19 slices carry a `### TDD Cycle Evidence` table (slices 6, 7, 8a, 8b, 9, 10a, 10b, 11a, 11b, 13). Slices 1 through 5, 12a and 12b instead record per-task RED/GREEN narrative subsections plus a `### Verification` block with exact commands. Substantive evidence exists for every slice; only the format differs. |
| All tasks have tests | Yes | Every test file named in `apply-progress.md` and `tasks.md` exists on disk; all 81 test names cited in the compliance matrix were matched against this run's captured PASS output. |
| RED confirmed (test files exist) | Yes | 170 `Test*` functions across 36 test files in `longterm-mem`; 36 test files in `engine`. Slice 13 RED evidence quotes verbatim pre-fix failures for both CRITICALs, for the degraded-status wiring, and for both convergence branches. |
| GREEN confirmed (tests pass now) | Yes | Full suite re-executed with `-count=1`, exit 0 in all three modules, 981 PASS lines, zero FAIL, zero SKIP. |
| Triangulation adequate | Yes | Scenario-shaped table-driven sub-tests name each spec scenario literally; the `Decide` table is pinned exhaustively; `..._LongtermMemRouteGuardAcceptsEveryValidRoute` hardcodes the four-value domain rather than iterating the map it guards; all three arms of the uninstall exit-status branch have a test. |
| Safety net for modified files | Yes | Each slice `### Verification` block records re-running prior slices plus both standing import guards; every slice 13 evidence row records "full suite green before edit". |
| Slice 13 evidence table covers every test-writing task | Yes | Six rows cover 13.1–13.10 and 13.12–13.14. 13.11 is the slice-verification task and needs no row. |
| Apply record matches shipped code | **Partially** | The four items run 3 named are closed. The slice 13 `### Files Changed` table and its `### Change status` paragraph still describe the pre-correction tree. See WARNING-1. |

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit (in-process, no subprocess) | 141 | 34 | `go test` |
| Integration (drives a real compiled binary, the real `bin/labdrian-overlay`, real vault fixtures, or the real SQLite fallback) | 29 in `longterm-mem`, plus 49 across the three `engine` files listed | 5 (`cmd/longterm-mem/main_test.go`, `internal/mcpserver/server_test.go`, `engine/installer/route_test.go`, `engine/skills/ondisk_test.go`, `engine/runtime/longtermmem_test.go`) | `go test` + `bash` + `python3` + `pgrep` |
| E2E (browser) | 0 | 0 | not applicable |
| **Total** | **170 in `longterm-mem`, plus the `engine` and `tui` suites** | **36 + 36** | |

Measurement basis, re-counted this run: 170 `func Test` declarations across 36
test files in `longterm-mem`, of which `cmd/longterm-mem/main_test.go` holds
25 and `internal/mcpserver/server_test.go` holds 4; the three `engine` files
listed hold 32, 14 and 3 respectively and are counted separately because they
are not part of the 170. The unit row is the remainder. The split is
approximate at the boundary, since a few files mix in-process and subprocess
tests.

Environment-gated skips (`-short`, missing `pgrep`, missing `python3`, running
as root) were all inactive: a `-v` run of both modules produced zero
`--- SKIP` lines.

### Assertion Quality

946 `t.Error`/`t.Errorf`/`t.Fatal`/`t.Fatalf` assertions across the
`longterm-mem` module. A scan for tautologies (`expect(true`, `assert(true`,
`if true`, `== true)`, bare `t.Error()`) across `longterm-mem`,
`engine/skills` and `engine/installer` returns nothing. No assertion-free
tests, no ghost loops over possibly-empty collections, and no orphan
empty-collection assertions were found. Negative cases are paired with
positive ones throughout.

`TestUninstall_VersionSkewStillConverges`, the newest test in the change,
asserts four distinct things: that the run converges, that the output names
the version skew, that it tells the operator the entry was left behind, and
that tracking no longer lists the target. It is not a smoke test. One
correction to run 3's account: the string `version skew` occurs **twice** in
`bin/labdrian-overlay`, at `:3079` and `:3082`, not once — but `:3079` is a
comment, so `:3082` inside the exit-2 arm is the only occurrence that can ever
reach the output this test matches. The branch-discrimination argument stands;
the count run 3 gave did not.

**Assertion quality**: all assertions verify real behaviour. One structural
caveat, not an assertion defect — see WARNING-3.

### Quality Metrics

**Formatter**: `gofmt -l longterm-mem engine tui` — no output, clean.
**Vet**: `go vet ./...` in both `longterm-mem` and `engine` — no diagnostics.
**Shell syntax**: `bash -n bin/labdrian-overlay`, `bash -n bin/overlay` — both clean.
**Shell lint**: `shellcheck bin/labdrian-overlay` — 8 findings, all pre-existing and identical to the `main` baseline; zero new.
**Dead code**: `deadcode` RTA from `cmd/longterm-mem` — zero findings, with a working 16-finding positive control.

### Disposition of run 3 findings

| Run 3 finding | Status now | Evidence |
|---|---|---|
| CRITICAL: none | still none | both run 1 CRITICALs re-proved closed by execution on this tree |
| WARNING-1 — slice 13 record half-corrected and self-contradictory | **CLOSED** | all three count lines now read 8/8/Unit 3 + Integration 5, the 13.12–13.14 evidence row exists, and all eight named tests were located on disk and observed passing |
| WARNING-2 — one-column `longterm-mem/**` row: bash rejects, Go skips | **OPEN, judgement holds** | re-proved by this run's independent probe and by reading both guards. Still WARNING-2 |
| WARNING-3 — `os/exec` allowlist skips a real `testdata` package | **OPEN, judgement holds** | still WARNING-3 |
| WARNING-4 — documented state-file paths and CLI surface have drifted | **OPEN, judgement holds, one item extended** | all three re-confirmed; the DSN drift is in `tasks.md:107` as well as `apply-progress.md:41`. Still WARNING-4 |
| SUGGESTION-1 through SUGGESTION-5 | all OPEN | each re-verified against the tree below |
| — | **NEW: WARNING-1** | the slice 13 `### Files Changed` table and `### Change status` paragraph still describe the pre-correction tree |

### Issues Found

#### CRITICAL

None. Both run 1 CRITICAL findings are closed, and both were re-proved closed
in this run by execution against the shipped artefacts rather than by
re-running the tests written to close them.

#### WARNING

**WARNING-1 (NEW) — the slice 13 `### Files Changed` table and `### Change
status` paragraph still describe the tree before the 13.12–13.14 correction.**

Run 3's finding is closed. Reading the whole slice 13 section rather than only
the lines run 3 named turned up two more places in the same section that were
never updated when the convergence work landed. Eight of the ten Files Changed
rows are exact; the two that are wrong are exactly the two files tasks
13.12–13.14 touched.

| Location | Record says | Shipped slice 13 aggregate | Gap |
|---|---|---|---|
| `apply-progress.md:4569` `engine/installer/route_test.go` | `+146/-0`, "2 new real-binary integration tests" | `+273/-0`, 4 new test functions | 127 lines, 2 tests |
| `apply-progress.md:4568` `bin/labdrian-overlay` | `+68/-15`, exit rule "anything else keeps it and fails the run" | `+98/-15`, and exit 2 converges | 30 lines, plus a rule the script no longer implements |
| `apply-progress.md:4642` `### Change status` | "All 168 tasks in `tasks.md` are complete" | `tasks.md` has 182 rows, all ticked | 14 tasks |

Measured with `git diff --numstat dc8087b^ HEAD` over the slice 13 file set;
`dc8087b` is slice 13-1, the first commit of the slice. The recorded
`+146/-0` and `+68/-15` match no commit on this branch — `d3a540e` alone is
`+203/-0` and `+97/-12` — so they appear to record the pre-correction 13-2
candidate rather than what shipped.

The `bin/labdrian-overlay` row is the sharper of the three, because it is not
a scope question: it states an exit-status rule that the shipped script
contradicts. `bin/labdrian-overlay:3069-3093` clears tracking and converges
for exit 0, exit 6 **and exit 2**, and `:3033-3037` adds an `unregister_usable`
pre-check that converges for a missing or non-executable binary before the
`case` is reached. Only the `*` arm keeps the target tracked and fails the run.
Task 13.12 records the corrected rule accurately; the Files Changed row two
sections below it still records the superseded one.

`:4642` is a separate, smaller instance. The historical statement at `:4476`
is correctly past-tense and explicitly marked "superseded by Slice 13 below";
`:4642` is present-tense inside slice 13's own status and was already wrong
when written (`tasks.md` held 179 rows at commit `052836b`).

Severity is WARNING, not CRITICAL: no spec scenario is affected, no task is
unchecked, no test fails, and the shipped behaviour is correct and fully
tested. It is reported rather than waived because it is the same family the
last two runs reported, in the same artefact archive folds into the spec
baseline, and because the whole point of a fourth run is to look at the parts
the third run did not.

**WARNING-2 (carried, judgement holds) — the two route parsers still disagree
on a one-column `longterm-mem/**` row.**

`ondisk.go:83-85` skips any line with fewer than two fields before the
`longterm-mem/**` guard at `:93` can see it, so a row that is only a path is
silently ignored. Re-proved by this run's independent probe: the Go parser
returns an empty path set and a nil error for `longterm-mem/only-one-field`.
The bash parser rejects the same row, because
`route_reject_unrouted_longterm_mem` at `:414-430` fires for every path
matching the `longterm-mem/` prefix — `route_resolve:437-439` tests the prefix
before defaulting the route — including a one-column row whose third field
resolves to the empty string, and exits 1 naming the row.

This does NOT fail R-035. That scenario's row grammar is path, tag, optional
route, and both parsers reject both shapes it describes. A one-field line has
no tag either, so it is not a manifest row at all, and the Go two-field skip is
the pre-existing whole-manifest rule applied identically to every path prefix,
not a `longterm-mem` carve-out. The harm the requirement names — silently
resolving such a row to a skills-destination path — does not occur: Go returns
nothing for it, and bash hard-fails it on any apply. It remains a real
divergence in a domain D13 claims both parsers share, and it is the only one
left.

**WARNING-3 (carried, judgement holds) — the `os/exec` allowlist has a real
blind spot.**

`exec_allowlist_test.go:31` skips any directory named `testdata` during its
walk, and `internal/ops/testdata/fixture.go` is still a genuine compiled Go
package declaring `package testdata`, not inert fixture data. Re-checked at
verify time: it imports `os` but not `os/exec`, so R-021 holds today and
`TestOSExecImportAllowlist` passes on its own terms. It is the only Go file
under any `testdata` directory in the module. The guard simply would not
notice if that changed. Leaving it open remains defensible, since the package
is test-only, small and reviewed, but it is a guard with a hole in it.

**WARNING-4 (carried, judgement holds, one item extended) — documented paths
and surfaces have drifted from the code.**

| Recorded | Shipped | Where |
|---|---|---|
| `~/.labdrian-overlay/longterm-mem/registration.json` | `longterm-mem-registration.json` directly under the state dir | `tasks.md:111` vs `engine/runtime/longtermmem.go:24,294` |
| DSN `_pragma=query_only(1)` | `_query_only=true` | `tasks.md:107` and `apply-progress.md:41` vs `store.go:114` |
| CLI surface includes `vaults` | no `vaults` subcommand exists or is planned | `tasks.md:108` vs `main.go:33-45` |

All three are documentation-only and all three were re-confirmed present on
this tree. Install, status and uninstall all use the same registration
constant, so behaviour is internally consistent; the DSN that shipped is the
correct one for the driver; and the `vaults` entry is residue from the
otherwise-clean removal of the seed call. None affects a spec scenario.

Extension over run 3: the DSN drift is in two artefacts, not one. `tasks.md:107`
carries the same wrong string as `apply-progress.md:41`. Both `design.md:13`
and `apply-progress.md:4744` record the correct DSN, so the change's own
artefacts now contradict each other on this value in both directions.

#### SUGGESTION

- **SUGGESTION-1 (carried, re-verified)** — the `cmdRegister` `--target all`
  skip for a runtime with no config file (`cmd_register.go:99-108`), pinned by
  `TestCmdRegister_AllSkipsRuntimesThatAreNotInstalled` (`main_test.go:883`)
  and paired with `TestCmdRegister_NamedTargetWithoutAConfigStillFails`
  (`:929`), is real, correct, deliberate behaviour that no spec scenario
  describes. A search across all nine delta specs for any mention of
  multi-target expansion returns nothing; R-016, R-017 and R-018 are silent on
  it. Worth folding into the spec at archive so it is not re-litigated later
  as an accident.
- **SUGGESTION-2 (carried, re-verified)** — `uninstallCases()` in
  `unregister_test.go:27-41` still returns only claude and codex, so
  `TestUnregister_OutcomeAndOnDiskEffect` runs its removed, left-alone and
  quiet triple for two runtimes rather than three. opencode is covered by the
  sibling golden-harness tests and by this run's three-runtime execution
  proof, so this is asymmetry, not a gap.
- **SUGGESTION-3 (carried, re-verified)** — the `### TDD Cycle Evidence` table
  appears in 10 of 19 slices (6, 7, 8a, 8b, 9, 10a, 10b, 11a, 11b, 13). Slices
  1 through 5 and 12a/12b use the narrative form, including 12b, the R-019
  slice whose defect became run 1 CRITICAL-1. Adopting the table across all 19
  would make the apply record uniformly machine-readable.
- **SUGGESTION-4 (carried, re-verified)** — the `--purge` forcing path still
  has no automated test. `route_test.go:1528-1529` only asserts that the
  refusal message mentions `--purge`; the four `TestStatusUninstall_SkipBuildStep`
  sub-tests cover the non-purge branches. It is the documented rollback
  procedure (`design.md:60`) and the one path that can orphan entries, and
  nothing pins it.
- **SUGGESTION-5 (carried, re-verified)** — cosmetic:
  `longtermmem_maybe_remove_binary` (`bin/labdrian-overlay:2923-2933`) calls
  `rm -f "$LONGTERM_MEM_BINARY"` and then unconditionally prints
  `longterm-mem binary removed: ...`, so the missing-binary convergence path
  reports removing a binary that was already gone, one line after the warning
  at `:3036` said the binary was missing. Harmless, but it reads as
  contradictory in exactly the situation an operator is trying to understand.

### Verdict

**PASS WITH WARNINGS** — 0 CRITICAL, 4 WARNING, 5 SUGGESTION.

35/35 requirements and 82/82 scenarios are compliant, each with a covering
test matched to a `--- PASS` line produced in this run. All 182 tasks are
complete. Every contract gate passes: three module test suites green with
`-count=1`, 981 PASS lines with zero FAIL and zero SKIP, both shell syntax
checks clean, `gofmt` and `go vet` clean across `longterm-mem` and `engine`,
zero new shellcheck findings against the `main` baseline, both standing import
guards green when run individually, and zero `deadcode` findings under a
working 16-finding positive control. Both former CRITICAL findings were
re-proved closed by execution against the shipped artefacts.

**The record correction run 3 demanded did land this time.** All three
adjacent count lines read consistently, the TDD evidence row for 13.12–13.14
exists, and the eight tests the record names were each located on disk and
observed passing — the layer split it claims is derived from where they
actually live, not from the record. The specific defect run 3 reported is
closed, not laundered.

**A different part of the same section is still wrong.** Two of the ten
`### Files Changed` rows understate what shipped by 127 and 30 lines, one of
them states an exit-status rule the shipped script contradicts, and the slice's
`### Change status` paragraph still counts 168 tasks against a `tasks.md` of
182. That is carried as WARNING-1. It is not the run 3 finding relabelled: it
is a fourth instance of the same family, found because this run read the whole
section rather than the four lines it was told to check.

No WARNING blocks archive on its own terms: none breaks a spec scenario, none
leaves a task unchecked, and none corresponds to a failing or missing test.
WARNING-2, WARNING-3 and WARNING-4 were each judged non-blocking by earlier
runs for reasons this run re-verified by execution and inspection rather than
assumed. WARNING-1 is the one the maintainer should decide on deliberately:
it is cheap to finish, it lives in the record archive makes permanent, and it
is now the third consecutive run to report that same file as describing a tree
that is not the one being shipped.
