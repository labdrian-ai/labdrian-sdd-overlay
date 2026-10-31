```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:5d97dba7a4e97146eecb5b40388f5e8e3fab3b734f7455ef76e33456dc96f43a
verdict: fail
blockers: 1
critical_findings: 1
requirements: 14/16
scenarios: 31/35
test_command: cd tui && go test ./... -cover -count=1
test_exit_code: 0
test_output_hash: sha256:67b83a66c27fceab5d9755db9a4dfc36a01dcaa6e93b083f356ba5520987f158
build_command: cd tui && go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: overlay-versioned-releases
**Version**: 7 delta/new capability specs (16 requirements, 35 scenarios)
**Mode**: Strict TDD
**Candidate**: `feat/overlay-versioned-releases-tui-surfacing` @ `c47faca` (clean tree; all 4 slices)
**Scope**: full change (Slices 1, 2, 3a, 3b)

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 17 |
| Tasks complete | 17 |
| Tasks incomplete | 0 |

`gentle-ai sdd-status overlay-versioned-releases` reports `tasks: 17/17 complete`, `apply: all_done`,
`verify: ready`, `nextRecommended: verify`, `blockedReasons: []`. Every checked task was traced to
shipped code (see Correctness); no checkbox is decorative.

### Build & Tests Execution

**Build**: PASSED

```text
$ cd tui && go build ./...
(no output)          exit 0

$ cd tui && go vet ./...
(no output)          exit 0

$ cd tui && gofmt -l .
(no output)          exit 0

$ bash -n bin/labdrian-overlay
(no output)          exit 0

$ bash -n bin/overlay
(no output)          exit 0
```

**Tests**: 143 top-level tests PASSED (198 including subtests), 0 failed, 0 skipped

```text
$ cd tui && go test ./... -cover -count=1
ok  	github.com/labdrian-ai/labdrian-sdd-overlay/tui	6.638s	coverage: 87.9% of statements
```

Per-file test-function counts: `release_backend_test.go` 29, `restore_backend_test.go` 21,
`main_test.go` 69, `view_test.go` 11, `selfupdate_backend_test.go` 10, `capture_backend_test.go` 2,
`logo_test.go` 1 = 143 (matches the 143 `--- PASS` top-level entries exactly).

**Coverage**: 87.9% of statements (Go package `tui`) — no configured threshold. The bash backend
`bin/labdrian-overlay` carries no coverage instrumentation; it is exercised through Go-driven
scratch-repo integration tests (`runBackendFunc` / `runBackendSubcommandWithHome`).

### Spec Compliance Matrix

Requirement IDs below are spec-file-local (`R-00N`); the change-level `R-001..R-011` trace targets
are shown in parentheses.

#### overlay-backup-restore (change R-007, R-008)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| R-001 (R-007) | Backup created before files are overwritten | `restore_backend_test.go > TestRestoreBackend_BackupTarget_CreatesBackupBeforeOverwrite`, `TestRestoreBackend_Apply_CreatesBackupBeforeOverwrite` | COMPLIANT |
| R-001 (R-007) | No-op apply creates no backup | `restore_backend_test.go > TestRestoreBackend_BackupTarget_NoOpCreatesNoBackup`, `TestRestoreBackend_BackupTarget_NeverDeployedCreatesNoBackup` | COMPLIANT |
| R-002 (R-007) | Fourth backup prunes the oldest | `restore_backend_test.go > TestRestoreBackend_PruneBackups_FourthPrunesOldest`, `TestRestoreBackend_PruneBackups_SameSecondCollisionPrunesTrueOldest` | COMPLIANT |
| R-003 (R-008) | Restore matches the most recent backup | `restore_backend_test.go > TestRestoreBackend_Restore_MatchesLatestBackup`, `TestRestoreBackend_Restore_SameSecondCollisionPicksTrueMostRecent` | COMPLIANT |
| R-003 (R-008) | No-backup-available error path | `restore_backend_test.go > TestRestoreBackend_Restore_NoBackupExitsNonZeroNoFileChanges` | COMPLIANT |

#### overlay-release-identity (change R-001)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| R-001 | Tag exists on the release commit | (none found — produced by `.github/workflows/release.yml`, not runnable in the repo suite) | UNTESTED |
| R-001 | Monotonic ordering across releases | `release_backend_test.go > TestReleaseBackend_ResolveLatestTag_SemverOrderNotLexicalAndFetchesExplicitly` (consumer-side ordering only) | PARTIAL |
| R-002 | New commits trigger a release tag | (none found — CI workflow) | UNTESTED |
| R-002 | Already-tagged HEAD is skipped | (none found — CI workflow) | UNTESTED |
| R-003 | Tags fetched explicitly before resolution | `release_backend_test.go > TestReleaseBackend_ResolveLatestTag_SemverOrderNotLexicalAndFetchesExplicitly` (clone never fetched tags itself) | COMPLIANT |
| R-003 | Semver-aware ordering, not lexical | same test (`v1.10.0` beats `v1.9.0`) + `TestReleaseBackend_ResolveLatestTag_SkipsLightweightTag`, `..._OnlyReachableFromGivenRef`, `..._NoTagsReturnsNone` | COMPLIANT |

#### overlay-release-state (change R-002, R-003)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| R-001 (R-002) | Successful apply records version and digest | `release_backend_test.go > TestReleaseBackend_ApplyRecordsVersionAndDigestForTarget` | COMPLIANT |
| R-001 (R-002) | First-ever apply creates the state file | `release_backend_test.go > TestReleaseBackend_StateWriteTarget_FirstApplyCreatesStateFile` | COMPLIANT |
| R-001 (R-002) | Never-deployed target reported honestly | `release_backend_test.go > TestReleaseBackend_StateReadTarget_NeverDeployedWhenMissing`, `..._CorruptFileReportsNeverDeployedWithWarn` | COMPLIANT |
| R-002 (R-003) | Matching digest for a fully in-sync target | `release_backend_test.go > TestReleaseBackend_ComputeTargetDigest_MatchesManualAlgorithm`, `TestReleaseBackend_Status_InSyncTargetNamesVersion` | COMPLIANT |
| R-002 (R-003) | Digest changes on a single-file mutation | `release_backend_test.go > TestReleaseBackend_ComputeTargetDigest_MutationChangesDigest` | COMPLIANT |
| R-002 (R-003) | Digest is order-independent of manifest row order | `release_backend_test.go > TestReleaseBackend_ComputeTargetDigest_ManifestReorderIsIdentical` | COMPLIANT |

#### overlay-release-surfacing (change R-009, R-010, R-011)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| R-001 (R-009) | In-sync target passes | `restore_backend_test.go > TestRestoreBackend_Doctor_InSyncTargetPasses` | COMPLIANT |
| R-001 (R-009) | Drifted target warns, does not fail the exit code | `restore_backend_test.go > TestRestoreBackend_Doctor_DriftedTargetWarnsExitZero` | COMPLIANT |
| R-001 (R-009) | Existing host-toolchain checks unaffected | `restore_backend_test.go > TestRestoreBackend_Doctor_ExistingChecksUnaffected` | COMPLIANT |
| R-002 (R-010) | Behind-target reported by name | `restore_backend_test.go > TestRestoreBackend_Version_BehindTargetReportedByName` | COMPLIANT |
| R-002 (R-010) | Never-deployed target | `restore_backend_test.go > TestRestoreBackend_Version_NeverDeployedTarget`, `..._VersionFlag_AliasesVersionCommand` | COMPLIANT |
| R-003 (R-011) | Version and behind-indicator shown | `view_test.go > TestViewDashboard_VersionLineRendering`, `TestViewDashboard_ShowsReleaseBehindIndicator`, `TestViewDashboard_EmptyRecordedVersionRendersNoVersionLine` | COMPLIANT |
| R-003 (R-011) | Restore action selectable and confirmed before running | `main_test.go > TestRestoreActionRegistered`, `TestUpdateActions_Restore_WithBackupShowsConfirmNamingTimestampVersion`, `TestUpdateActions_Restore_NoBackupIsNoOp`, `TestUpdateActions_Restore_PartialBackupAvailabilityAmongSelection` | COMPLIANT |

#### overlay-update-check (change R-006)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| R-001 (R-006) | Reports latest version and per-target status | `release_backend_test.go > TestReleaseBackend_Update_ReportsLatestVersionAndNeverDeployedTargets`, `..._Update_BehindTargetReportedByName`, `..._Update_UpToDateTargetReportedByName`, `..._Update_PreFirstTagReportsNoReleasesPublishedYet` | COMPLIANT |
| R-001 (R-006) | Zero mutation across repeated runs | `release_backend_test.go > TestReleaseBackend_Update_ZeroMutationAcrossRepeatedRuns` | COMPLIANT |
| R-001 (R-006) | Never-deployed target reported without error | `release_backend_test.go > TestReleaseBackend_Update_ReportsLatestVersionAndNeverDeployedTargets` | COMPLIANT |

#### sync-check-verdicts delta — ADDED (change R-004)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| R-007 | Version named when apply is required | `release_backend_test.go > TestReleaseBackend_SyncCheck_DigestMismatchActionNamesReleaseVersion` | COMPLIANT |
| R-007 | Never-deployed target recommends apply without a fabricated version | `release_backend_test.go > TestReleaseBackend_SyncCheck_NeverDeployedTargetNoFabricatedVersion` | COMPLIANT |
| R-008 | In-sync target names its version | `release_backend_test.go > TestReleaseBackend_Status_InSyncTargetNamesVersion` | COMPLIANT |

#### tui-self-update delta — MODIFIED (change R-005)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| R-004 | Clean tree converges main to the latest release tag, original branch restored | `release_backend_test.go > TestReleaseBackend_SelfUpdate_ConvergesToTagNotRawOriginHead` (asserts `feature-x` restored, its HEAD unchanged, `v1.5.0` named) | COMPLIANT |
| R-004 | Only main moves, and it converges to the tag, not raw origin/main HEAD | same test (asserts `main == v1.5.0^{commit}` and `main != origin/main`) | COMPLIANT |
| R-004 | Already at latest release tag — no-op | `release_backend_test.go > TestReleaseBackend_SelfUpdate_AlreadyAtLatestTagNoOp` | COMPLIANT |
| R-007 | Post-update release convergence with untagged origin commits | `release_backend_test.go > TestReleaseBackend_ComputeRepoBehindRelease_ZeroAfterSelfUpdateConvergesWithUntaggedOriginAhead` + `main_test.go > TestBannerVisible_ReleaseTakesPrecedenceOnceKnown` + `view_test.go > TestRepoLine_OriginDriftDemotesToInformationalOnceReleaseKnown` | COMPLIANT |
| R-007 | Pre-first-tag legacy convergence keeps the old claim | `release_backend_test.go > TestReleaseBackend_SelfUpdate_ZeroTagFallbackConvergesLegacyWithNotice` + pre-existing `selfupdate_backend_test.go:433` (asserts `REPO_BEHIND_ORIGIN=0` after a zero-tag self-update, still passing) | COMPLIANT |

**Compliance summary**: 31/35 scenarios compliant · 3 UNTESTED · 1 PARTIAL · 0 FAILING
**Requirement summary**: 14/16 requirements fully compliant (`overlay-release-identity` R-001 and R-002 are CI-workflow-only)

### Correctness (Static Evidence)

Every one of the 17 checked tasks maps to shipped code at `c47faca`:

| Task | Claimed artifact | Located at | Status |
|---|---|---|---|
| 1.1 | `resolve_latest_release_tag()` + `pushUpstreamTag` | `bin/labdrian-overlay:446`; helper in `tui/release_backend_test.go` | Implemented |
| 1.2 | `compute_target_digest()` via `route_resolve`, sorted lines | `bin/labdrian-overlay:492` (`LC_ALL=C sort` at :528) | Implemented |
| 1.3 | `state_read_target` / `state_write_target` (awk, tmp+mv) | `bin/labdrian-overlay:539`, `:578` (`mktemp` + `mv` at :599/:615) | Implemented |
| 1.4 | `cmd_apply` wired to `state_write_target` | `bin/labdrian-overlay:1187-1192` | Implemented |
| 1.5 | `.github/workflows/release.yml` | present; parses as valid YAML; `permissions.contents: write`, `fetch-depth: 0`, skip-if-tagged, D7 bump, annotated tag + push | Implemented |
| 1.6 | `bash -n` + focused Go run | re-executed this phase, both clean | Verified |
| 2.1 | `cmd_self_update` rewritten onto tag resolution | `bin/labdrian-overlay:1239` (tag mode at :1311-1338, D1 fallback at :1281-1309) | Implemented |
| 2.2 | `compute_repo_behind_release()` + VERDICT fields | `bin/labdrian-overlay:1776`; VERDICT emitted at `:1972` | Implemented |
| 2.3 | `cmd_status` in-sync-at-version + `cmd_update()` + dispatch | `bin/labdrian-overlay:1654+`, `:1352`, dispatch `:2747` | Implemented |
| 2.4 | `bash -n` + focused Go run | re-executed this phase, clean | Verified |
| 3a.1 | `backup_target()` / `prune_backups()` in `cmd_apply` | `bin/labdrian-overlay:634`, `:720`; called at `:1146` | Implemented |
| 3a.2 | `cmd_restore()` + dispatch | `bin/labdrian-overlay:1418`; dispatch `:2748` | Implemented |
| 3a.3 | per-target digest row in `cmd_doctor` | `bin/labdrian-overlay:2385-2402` (check `(f)`, WARN never touches `fail`) | Implemented |
| 3a.4 | `cmd_version()` + `--version` alias | `bin/labdrian-overlay:1560`; dispatch `version\|--version` at `:2749` | Implemented |
| 3b.1 | `TargetVerdict` fields, key parsing, classify precedence, `Actions()` | `tui/run.go:196-203`, `:306-318`, `:215-228`, `:98-101` | Implemented |
| 3b.2 | `model.go` behindRelease+probe, `view.go` repoLine/viewDashboard | `tui/model.go:67-72`, `:100-101`, `:391`; `tui/view.go:134`, `:467`, `:483` | Implemented |
| 3b.3 | full Go run + both `bash -n` | re-executed this phase, clean | Verified |

**Independent-review bug fixes — both confirmed present with regression coverage:**

1. **Same-second backup-collision ordering (Slice 3a).** Bash glob expansion sorts paths *with*
   their trailing slash, so `<ts>-1/` sorts before `<ts>/` (ASCII `-` 0x2D < `/` 0x2F), inverting
   chronology. Fixed by an explicit `LC_ALL=C sort` of the bare basenames in **both**
   `prune_backups` (`bin/labdrian-overlay:734-736`) and `cmd_restore` (`:1464-1466`).
   Covered by `TestRestoreBackend_PruneBackups_SameSecondCollisionPrunesTrueOldest` and
   `TestRestoreBackend_Restore_SameSecondCollisionPicksTrueMostRecent` — both passing.
2. **Multi-target restore invoking backup-less targets (Slice 3b).** `model.pendingTargets`
   (`tui/model.go:38-49`) now holds the exact target set computed once by `restoreConfirmInfo`
   (`:391-405`), and `updateConfirm` reuses it verbatim (`:411`) instead of recomputing
   `selectedTargets()`. Covered by `TestUpdateActions_Restore_PartialBackupAvailabilityAmongSelection`,
   which asserts `pendingTargets == [claude]` when only `claude` has a backup — passing.

**Cross-slice integration (verified, not assumed):**

- **VERDICT key contract (Slice 2 producer → Slice 3b consumer).** The backend emits
  `REPO_BEHIND_RELEASE=`, `RECORDED_VERSION=`, `DIGEST_MATCH=` at `bin/labdrian-overlay:1972`; the
  TUI parses those exact three literals at `tui/run.go:306,315,317`. Both sides are pinned to the
  same literals by tests: the producer side by
  `TestReleaseBackend_SyncCheck_DigestMismatchActionNamesReleaseVersion` (asserts
  `RECORDED_VERSION=v1.4.0`, `DIGEST_MATCH=no` in real backend stdout) and
  `TestReleaseBackend_SyncCheck_NeverDeployedTargetNoFabricatedVersion` (asserts
  `REPO_BEHIND_RELEASE=` present); the consumer side by `TestParseSyncCheck_ReleaseFields`.
  I additionally executed the real backend in a scratch repo and observed the exact line:
  `VERDICT:claude:UPSTREAM_CHANGED=0 OVERLAY_NOT_DEPLOYED=1 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=NA RECORDED_VERSION=NA DIGEST_MATCH=NA` — keys match.
- **`cmd_restore` invocation shape (Slice 3b caller → Slice 3a callee).** The restore `Action` is
  `SupportsAll: false` (`tui/run.go:98`), so `buildArgSets` takes its `default` branch
  (`tui/run.go:487-492`) and emits `["restore", "--target", "<name>"]` per target — never
  `--target all`. That is exactly the shape Slice 3a's tests exercise
  (`overlay restore --target claude`), and `cmd_restore` explicitly refuses `all`
  (`bin/labdrian-overlay:1444-1445`, covered by `TestRestoreBackend_Restore_RefusesTargetAll`).

**No regressions in pre-existing behavior:** all 10 `selfupdate_backend_test.go` and 2
`capture_backend_test.go` tests pass unchanged. No test function was deleted. Three pre-existing
tests were legitimately updated for contract changes the design mandates (`TestClassifyPrecedence`
for `classify`'s 3→5 parameters, `TestActionMenuShapeAndOrder` for 8→9 actions,
`TestSelfUpdateActionRegistered` for the new menu neighbour) — reviewed line by line, none weakened.

### Coherence (Design)

| Decision | Followed? | Evidence |
|---|---|---|
| D1 — pre-first-tag bootstrap | Yes | `resolve_latest_release_tag` returns literal `none` (`:473`); `cmd_self_update` falls back verbatim to origin/main convergence with a "no release tags yet -- legacy convergence" notice (`:1281-1309`); `cmd_apply` records `untagged` (`:1129`); `cmd_update`/`cmd_version` print "(no releases published yet)" (`:1376`, `:1569`). `release.yml` cuts `v1.0.0` when `git describe` finds no tag (`:53-58`). Covered by `..._SelfUpdate_ZeroTagFallbackConvergesLegacyWithNotice`, `..._Update_PreFirstTagReportsNoReleasesPublishedYet`, `..._ComputeRepoBehindRelease_NAPreTag`. |
| D2 — `REPO_BEHIND_RELEASE` primacy | Yes (with a caveat, see CRITICAL-1) | `classify` precedence is capture > apply/digest-mismatch > behind-release > behind-origin (`tui/run.go:215-228`); `bannerVisible` keys on release-behind once known, falling back to origin-only while NA (`tui/model.go:188`); `repoLine` demotes raw origin drift to an informational line (`tui/view.go:134`). `compute_repo_behind_release` correctly honours its `check_origin` flag and stays cached-only by default (`:1785-1793`) — the design's own "cached-only, like `REPO_BEHIND_ORIGIN`" wording. Covered by `TestBannerVisible_ReleaseTakesPrecedenceOnceKnown`, `TestRepoLine_ReleaseBehindBannerStates`, `TestRepoLine_OriginDriftDemotesToInformationalOnceReleaseKnown`. |
| D3 — state/backup location | Yes | `~/.labdrian-overlay/state.json` and `~/.labdrian-overlay/backups/<target>/<utc-ts>/`; the Go-side `latestBackup` reads the same documented layout (`tui/run.go:595`). |
| D4 — restore UX | Yes | Most-recent default (`:1501-1505`), `--list` (`:1468-1486`), `--backup <ts>` (`:1493-1500`), refuses `--target all` (`:1444`), retain-3 auto-prune (`prune_backups` `keep=3` at `:739`). TUI uses most-recent only, ConfirmMessage names timestamp + version (`tui/model.go:398`). All six behaviours have dedicated passing tests. |
| D5 — digest determinism | Yes | `compute_target_digest` emits `<repo_rel>:<sha256\|MISSING>` per managed row, then `LC_ALL=C sort \| sha256sum` (`:519-528`). Proven order-independent by `..._ManifestReorderIsIdentical` and content-sensitive by `..._MutationChangesDigest`; the exact algorithm is re-derived independently by `..._MatchesManualAlgorithm`. |
| D6 — `update` read-only meaning | Yes | `cmd_update` performs no `git checkout`, touches no branch head, worktree, target file, or state (`:1352-1400`); proven by `..._Update_ZeroMutationAcrossRepeatedRuns`. |
| D7 — conventional-commit bump | Yes (untested) | Inline in `release.yml:60-84` only, exactly as decided ("rejected: a bump helper in the CLI"). No automated coverage — see WARNING-1. |
| D8 — state I/O | Yes | Canonical JSON, `mktemp` + `mv` atomic write (`:598-615`), tolerant awk reader that degrades to `NEVER_DEPLOYED` + WARN (`:539-567`). Covered by `..._StateWriteTarget_FirstApplyCreatesStateFile`, `..._PreservesOtherTargetsOnUpdate`, `..._StateReadTarget_CorruptFileReportsNeverDeployedWithWarn`. |

**Design deviations** (both self-disclosed in apply-progress, neither breaks a spec):

- Slice 3a's "backup needed" test is a per-file `diff --brief` rather than an aggregate digest
  compare, deliberately excluding the never-deployed case. Pinned by
  `TestRestoreBackend_BackupTarget_NeverDeployedCreatesNoBackup`. Consistent with R-001's
  "current deployed managed files" wording. Accepted.
- Slice 3b gates restore availability in `updateActions` rather than hiding the menu row, because
  the static `Actions()` architecture has no per-selection conditional visibility. R-003's
  "selectable for it" is satisfied behaviourally (pressing enter with no backup is a no-op).
  Accepted.

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD evidence reported | PARTIAL | No tabular "TDD Cycle Evidence" table in apply-progress; per-slice prose sections report RED→GREEN, safety-net baselines, and mutation testing instead. See WARNING-2. |
| All tasks have tests | PASS | 16/17 tasks are code tasks with named covering tests; task 1.6/2.4/3b.3 are verification tasks, re-executed this phase. |
| RED confirmed (tests exist) | PASS | All named test files exist: `release_backend_test.go`, `restore_backend_test.go`, `view_test.go`, `main_test.go`. |
| GREEN confirmed (tests pass) | PASS | 143/143 top-level tests pass on re-execution with `-count=1` (cache-defeating, per apply's own recorded gotcha about `go test` caching not keying off the bash script). |
| Triangulation adequate | PASS | Multi-case triangulation throughout, e.g. digest (4 tests: manual-algorithm, reorder, mutation, ref-mode, missing-file), tag resolution (4 tests), restore (8 tests), update (6 tests). |
| Safety net for modified files | PASS | Each slice records a pre-edit baseline run of the pre-existing suite (12 → 36 → 58 → 121 tests) and a post-edit re-run; the final full suite confirms none regressed. |
| RED observed prospectively | PARTIAL | Slices 1/2/3a claim RED→GREEN with mutation testing as reinforcement. Slice 3b states implementation was written "first-pass together with tests, then mutation-tested for retroactive RED proof" — mutation testing is strong evidence the tests are load-bearing, but it is not literal RED-first. See WARNING-3. |

**TDD compliance**: 5 full passes, 2 partial.

Mutation-testing evidence recorded by apply (each mutation reverted, files re-diffed byte-identical):
removing `LC_ALL=C sort` broke the reorder-invariance test; disabling the annotated-tag `objecttype`
check broke the lightweight-tag test; removing `state_write_target`'s preservation loop broke the
preserve-on-update test; disabling `merge-base --is-ancestor` broke the already-at-tag no-op test;
removing `|| digestMismatch` broke `TestClassifyPrecedence`; removing the `restoreConfirmInfo` guard
broke `TestUpdateActions_Restore_NoBackupIsNoOp`; `latestBackup` picking `names[0]` broke
`TestLatestBackup_PicksLexicallyLastAsMostRecent`; collapsing `bannerVisible`'s dual mode broke four
banner tests. Apply also self-reported and repaired a genuine test-quality gap found this way (the
ACTION-guard mutation initially failed to go RED because the fixture had zero tags; the fixture was
strengthened with a real `v1.5.0` tag). That is credible, self-critical TDD evidence.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit (pure Go, no exec/no FS) | 62 | `main_test.go`, `view_test.go`, `logo_test.go` | `go test` |
| Integration (real backend via scratch git repos / real FS) | 81 | `release_backend_test.go` (29), `restore_backend_test.go` (21), `selfupdate_backend_test.go` (10), `capture_backend_test.go` (2), plus `latestBackup`/`probe` FS+exec tests in `main_test.go` (19) | `go test` + `git` + `bash` |
| E2E (browser/HTTP) | 0 | — | not applicable to a CLI/TUI |
| **Total** | **143** | **7** | |

The backend integration layer is the load-bearing one here and it is genuinely heavy: real `git init`
scratch repos, real annotated tag pushes, real `HOME` overrides, real file trees. No mocked git.

### Changed File Coverage

| File | Statement coverage of new/changed functions | Rating |
|---|---|---|
| `tui/run.go` | `Actions` 100%, `classify` 100%, `ParseSyncCheck` 95.2%, `probeBehindRelease` 100%, `probeBehindOriginCmd` 100%, `buildArgSets` 100%, `latestBackup` 90.9% | Excellent |
| `tui/model.go` | `newModel` 100%, `bannerVisible` 100%, `restoreConfirmInfo` 100%, `runActionCmd` 100%, `Update` 81.1%, `updateActions` 79.2%, `updateConfirm` 60% | Acceptable |
| `tui/view.go` | `versionLine` 100%, `repoLine` 100%, `viewConfirm` 100%, `viewDashboard` 88.1% | Excellent |
| `bin/labdrian-overlay` | no coverage instrumentation (bash); exercised by 50 backend integration tests | Not measurable |
| `.github/workflows/release.yml` | 0% — no automated coverage | See WARNING-1 |

**Package coverage**: 87.9% of statements. The lowest changed-code figures (`updateConfirm` 60%,
`updateActions` 79.2%) are dominated by pre-existing keybinding branches, not new logic; every
new branch introduced by this change is covered.

### Assertion Quality

Scanned `release_backend_test.go`, `restore_backend_test.go`, and the added blocks of
`main_test.go` / `view_test.go`.

**Assertion quality**: All assertions verify real behaviour. No tautologies
(`if true`, `x == x`), no orphan empty-collection checks, no type-only assertions standing alone,
no ghost loops, no smoke-test-only cases. Assertion density is high: 113 `t.Error`/`t.Fatal` calls
across 29 tests in `release_backend_test.go`, 89 across 21 in `restore_backend_test.go`.
Assertions bind to concrete values — exact digests, exact timestamps, exact branch SHAs, exact
`VERDICT`/`ACTION` substrings, exact exit codes, byte-identity snapshots before/after — not to
mock call counts or CSS-equivalents. Zero `vi.mock`-equivalent mocking of git.

### Quality Metrics

**Linter / vet**: PASS — `go vet ./...` exit 0, no output.
**Formatter**: PASS — `gofmt -l .` exit 0, no output.
**Shell syntax**: PASS — `bash -n bin/labdrian-overlay` and `bash -n bin/overlay` both exit 0.
**Workflow YAML**: PASS — `.github/workflows/release.yml` parses; single job `tag-release`,
`permissions: {contents: write}`.

### Issues Found

**CRITICAL**

- **CRITICAL-1 — `sync-check`'s default path now invokes `git fetch`, violating two standing
  requirements that this change declared it would not alter.**

  Root cause is a single line: `cmd_sync_check` calls
  `release_version="$(resolve_latest_release_tag main)"` at `bin/labdrian-overlay:1862`
  **unconditionally**, and `resolve_latest_release_tag` always runs
  `git fetch origin '+refs/tags/v*:refs/tags/v*'` when an `origin` remote exists
  (`bin/labdrian-overlay:449-461`). The neighbouring calls on the same code path —
  `compute_repo_behind_origin "$check_origin"` (`:1854`) and
  `compute_repo_behind_release "$check_origin"` (`:1857`) — both correctly gate their fetch behind
  the flag, which makes the omission at `:1862` a clear oversight rather than an intended change.

  *Proven by execution, not inference.* In a scratch clone cloned with `--no-tags` (zero local
  tags) whose origin carried an annotated `v1.2.3`, running `bin/labdrian-overlay sync-check` with
  **no** `--fetch` and **no** `--check-origin` left `git tag -l` reporting `v1.2.3`. The read-only
  default path performed network I/O and mutated local `refs/tags/*`.

  *Requirements violated* (both are standing specs under `openspec/specs/`, and this change's
  deltas modify neither):
  - `openspec/specs/sync-check-verdicts/spec.md` R-001 — "WHEN sync-check runs without
    `--check-origin` or `--fetch`, the system SHALL compute ... **without invoking `git fetch`**",
    scenario: "AND no `git fetch` is invoked". *(The normative sentence is arguably scoped to the
    origin comparison; the scenario's "AND no `git fetch` is invoked" is not scoped at all.)*
  - `openspec/specs/tui-self-update/spec.md` R-001 — "WHEN the TUI starts, `Init()` SHALL return a
    `tea.Cmd` that reads `REPO_BEHIND_ORIGIN` from the cached ref only, **MUST NOT invoke
    `git fetch`**", scenario: "THEN no `git fetch` is invoked". `model.Init()` →
    `probeBehindOriginCmd` (`tui/run.go:394-406`) runs `sync-check` with no flags, so every TUI
    launch now transitively fetches. *(This one has no scoping ambiguity.)*

  Also contradicts this change's own design D2 ("`REPO_BEHIND_RELEASE=<n|NA>` ... cached-only, like
  `REPO_BEHIND_ORIGIN`"), its delta `tui-self-update` R-007 ("This capability still MUST NOT alter
  `sync-check-verdicts`' detection logic"), and `proposal.md`'s narrow "VERDICT/ACTION lines gain
  release-version fields" scope.

  *Newly introduced by this change.* At the base commit `7c4efe0~1`, `cmd_sync_check`'s body
  contained no fetch and no tag resolution; the line was added by Slice 2 (`fe1c100`).

  *Why no test caught it.* The suite has no backend-level assertion that `sync-check` performs no
  fetch. The closest test, `main_test.go:1612`, only asserts the TUI does not **pass**
  `--fetch`/`--check-origin` — it tests the flag, not the effect. Standing
  `sync-check-verdicts` R-001's "no git fetch is invoked" scenario has never had a covering test.

  *Blast radius:* `sync-check`, `status` (unaffected — it does not resolve tags), and every TUI
  launch. `cmd_apply` (`:1128`), `cmd_self_update` (`:1279`), `cmd_update` (`:1373`) and
  `cmd_version` (`:1566`) also call `resolve_latest_release_tag`, but for those the fetch is either
  required by spec (R-003, D6) or defensible; only `:1862` sits on a contractually
  no-fetch path. `cmd_doctor` and `cmd_status` are clean.

  *Mitigating facts:* the fetch is bounded (`timeout 10`, `http.lowSpeedLimit`) and suffixed
  `|| true`, so the standing "offline default path completes without error" scenario still holds —
  measured exit 0 in 0.099s against an unreachable origin. The impact is a contract violation plus
  unrequested network I/O and local ref mutation, not a crash.

  *Remediation shape (not applied — verify does not fix):* `cmd_sync_check` needs the release
  version only for its ACTION line, so it can resolve from **locally cached** tags without
  fetching. The exact pattern already exists in this file at `compute_repo_behind_release:1807-1815`
  (`git for-each-ref --merged <ref> --sort=-v:refname 'refs/tags/v*'` filtered on
  `objecttype == "tag"`). A `--check-origin`-gated fetch parameter on
  `resolve_latest_release_tag`, or a cached-only sibling helper, would restore the contract. The
  fix must ship with the regression test standing R-001 never had: assert that a default
  `sync-check` in a `--no-tags` clone leaves `git tag -l` empty.

**WARNING**

- **WARNING-1 — `overlay-release-identity` R-001/R-002 have zero automated coverage (3 UNTESTED,
  1 PARTIAL scenarios).** The CI release-cutting behaviour (skip-if-tagged, conventional-commit
  bump, annotated tag push, first-tag `v1.0.0`) lives entirely in `.github/workflows/release.yml`
  and is unreachable from the repo test suite. `design.md`'s threat matrix explicitly accepted this
  ("skip guard is inline in workflow (accepted: not runnable in repo suite)"), and apply reports a
  manual 7-case sanity check of the bump arithmetic. I independently confirmed the workflow parses
  as valid YAML with the required `permissions: contents: write`, `fetch-depth: 0`, skip-if-tagged
  guard, and D7 bump mapping. Under a strict reading of the verify decision gates these scenarios
  would be CRITICAL `UNTESTED`; I am recording them as WARNING because the design accepted the gap
  deliberately and the risk is bounded (a misbump mis-sizes an increment, it does not break
  ordering, and consumer-side resolution — the part that actually matters to users — is thoroughly
  tested). **The first real merge to `main` is the actual proof; watch that run.**
- **WARNING-2 — apply-progress has no tabular "TDD Cycle Evidence" section.** Strict TDD verify
  expects a RED/GREEN/TRIANGULATE/SAFETY-NET/REFACTOR table per task. The artifact instead carries
  richer per-slice prose (baseline counts, mutation-testing logs, self-reported gaps). The evidence
  is present and independently verifiable — this is a format deviation, not missing evidence, so I
  am not treating it as the CRITICAL the strict module's default branch would assign.
- **WARNING-3 — Slice 3b's RED phase was reconstructed retroactively.** apply-progress states
  implementation was written "first-pass together with tests, then mutation-tested for retroactive
  RED proof", while `tasks.md` marks 3b.1 as "RED→GREEN". Four mutations were confirmed RED, which
  is strong evidence the tests are load-bearing, but it is not prospective RED-first. Slices 1, 2
  and 3a claim genuine RED→GREEN.
- **WARNING-4 — apply-progress is stale on both independent-review bug fixes.** The artifact's
  Slice 3a section (`prune_backups` description) still states "bash pathname expansion already
  yields lexically sorted ... results", which is the pre-fix rationale, and its "Issues found and
  fixed during GREEN" lists only the `return 0` errexit bug — the same-second collision fix is not
  mentioned. Slice 3b's section describes `restoreConfirmInfo` but not the `pendingTargets` fix.
  **The code and regression tests for both fixes are present and passing** (verified above); only
  the narrative artifact lags. Worth reconciling before archive so the historical record is accurate.

**SUGGESTION**

- **SUGGESTION-1 — No single test carries real backend output through `ParseSyncCheck`.** The
  producer/consumer contract is held by matching string literals in two separate tests. That is
  sound today, but one end-to-end test (run `sync-check` in a scratch repo, feed its real stdout to
  `ParseSyncCheck`, assert the parsed struct) would make key-name drift impossible to miss.
- **SUGGESTION-2 — `tui-self-update` R-007 scenario 1 is asserted across two tests, not one.**
  `..._ZeroAfterSelfUpdateConvergesWithUntaggedOriginAhead` asserts `REPO_BEHIND_RELEASE=0` via the
  helper function directly and does not assert the companion `REPO_BEHIND_ORIGIN=2` on the same
  VERDICT line. The banner half is covered separately by the view/model tests. Complete, but a
  single VERDICT-line assertion would read more directly against the scenario.
- **SUGGESTION-3 — `invocationSeverity` (`tui/run.go:512`) sits at 0% coverage.** Pre-existing, not
  introduced here, but it is the exact aggregation logic the Slice 3b restore bug fix was
  protecting; worth a direct test.
- **SUGGESTION-4 — both slice 3a/3b diffs overran their tasks.md forecasts** (~910 vs 220–380 and
  ~983 vs 150–290 lines). Apply flagged both, and the overage is dominated by test volume, but the
  forecasting model under-predicts integration-test-heavy bash work and could be recalibrated.

### Verdict

**FAIL**

The implementation is otherwise strong — 143/143 tests green, 87.9% coverage, all 17 tasks traced
to real shipped code, 31/35 scenarios compliant, all 8 design decisions honoured, both
independent-review bug fixes confirmed present with real regression tests, and zero regressions in
the pre-existing suite. But one line (`bin/labdrian-overlay:1862`) makes the read-only default
`sync-check` path invoke `git fetch` and mutate local `refs/tags/*`, contradicting two standing
requirements this change explicitly promised not to alter — including `tui-self-update` R-001's
unambiguous "MUST NOT invoke `git fetch`", which every TUI launch now violates. That is a real
behavioural regression, proven by execution, introduced by this change, and uncovered by any test.
It blocks archive.

**Next recommended**: `sdd-apply` (remediation) — fix `cmd_sync_check`'s tag resolution to be
cached-only unless `--check-origin`/`--fetch` is passed, add the missing regression test, refresh
apply-progress per WARNING-4, then re-run `sdd-verify`.
