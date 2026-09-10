```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:24d4b548cf4b3bf7d0c8bfc56ed9a832082a742eed8528793ec19585fbb98bee
verdict: pass
blockers: 0
critical_findings: 0
requirements: 4/4
scenarios: 23/23
test_command: cd engine && go vet ./... && go test -count=1 -race ./... && cd ../longterm-mem && go vet ./... && go test ./...
test_exit_code: 0
test_output_hash: sha256:8ae0de15743d87619f180064fe4e1c9714fe0bfa355536256bcd0e20c0f8986f
build_command: cd engine && go vet ./... && cd ../longterm-mem && go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: longterm-mem-sync-triggers
**Version**: N/A (delta specs `longterm-mem-sync-triggers` + `runtime-lifecycle`)
**Mode**: Strict TDD
**Worktree**: `/home/labdrian/labdrian-sdd-overlay-worktrees/lmst-3` (branch `feat/longterm-mem-sync-triggers-3-archive`, HEAD `8d65686`, base `main`)
**Re-run trigger**: remediation of the prior FAIL (obs #3323, HEAD `76d127a`) — commits `e554908` (new shelltest case `case_archive_sync_trigger_call_site_is_only_in_inception_pipeline`) and `8d65686` (design.md wrapper row and tasks.md 3.2/3.4 aligned with the detached wrapper).

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 24 |
| Tasks complete | 24 |
| Tasks incomplete | 0 |
| Tasks marked complete without a delivered artifact | 0 |

Planned `review_slices` 3; realized 3 stacked PRs (#299, #300, #302). `size:exception` granted for slices 1 and 2.

### Build & Tests Execution

**Build (vet)**: PASSED

```text
cd engine && go vet ./...
cd longterm-mem && go vet ./...
exit 0 — empty output (no diagnostics)
```

**Tests**: PASSED

```text
cd engine && go test -count=1 -race ./...
exit 0 — 12/12 packages ok (assets, cmd, gadu, gate, installer, prespec, propagator, runtime, settings, shelltest, skills, synctrigger)

cd longterm-mem && go test ./...
exit 0 — 17/17 packages ok

bash engine/shelltest/overlay_longterm_mem_test.sh
exit 0 — all shell test cases passed (58 cases), including the four sync-trigger cases and the new
`case_archive_sync_trigger_call_site_is_only_in_inception_pipeline`

shellcheck -S warning bin/labdrian-overlay
exit 1 — only the two known pre-existing SC2064 warnings at lines 1314 and 1475
(`trap "git checkout '${current_branch}' ..."`), both declared known-environmental
by the orchestrator and untouched by this change.
```

**Configured `rules.verify.test_command` (repo-wide)**: exit 1 (re-confirmed) — the single
failure is `tools/archive-reconcile > TestShippedLedgerMatchesThisRepository`. `tools/` is
untouched by this change (`git diff --stat main...HEAD -- tools/` is empty). It fails
*because* this change is 24/24 complete and not yet archived — stderr from the guard itself
prescribes promotion of `longterm-mem-sync-triggers/spec.md` and the `git mv` archive step as
the fix. This is a pre-archive workflow-state signal, not a code defect in this change, and it
self-clears when `sdd-archive` runs. Disclosed as informational per the orchestrator's framing,
not treated as a blocker.

**Coverage**: Not available — no coverage tooling configured (`coverage_threshold: 0`).

### Spec Compliance Matrix

#### `specs/longterm-mem-sync-triggers/spec.md`

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| Non-Blocking Sync Runner Contract | Missing binary is a silent skip | `synctrigger_test.go > TestRunChild_MissingBinary_SkipsWithoutRunning` | COMPLIANT |
| Non-Blocking Sync Runner Contract | Missing vault is a silent skip | `synctrigger_test.go > TestRunChild_ExitMapping` (exit 3 → `skip:no-vault`) | COMPLIANT |
| Non-Blocking Sync Runner Contract | Sync failure is logged, caller unaffected | `synctrigger_test.go > TestRunChild_ExitMapping` (1/4/5 → `failure:*`) + `TestRun_HappyPath_DetachesAndLogsWithin2s` | COMPLIANT |
| Non-Blocking Sync Runner Contract | Sync timeout is bounded and logged | `synctrigger_test.go > TestRunChild_Timeout_KillsProcessGroup_NoOrphan` | COMPLIANT |
| Non-Blocking Sync Runner Contract | Successful run is logged | `synctrigger_test.go > TestRunChild_ExitZero_Ok`, `TestRunChild_StderrReachesLogBeforeOutcomeLine` | COMPLIANT |
| Non-Blocking Sync Runner Contract | Unwritable log location does not block the host | `synctrigger_test.go > TestRun_UnwritableLogDir_ErrorLog` | COMPLIANT |
| Non-Blocking Sync Runner Contract | Runner cannot re-exec itself | `synctrigger_test.go > TestRun_NonExecutableSelf_ErrorSpawn` | COMPLIANT |
| Non-Blocking Sync Runner Contract | A usage error is logged as an error, not a skip | `synctrigger_test.go > TestRunChild_Exit2_OtherStderr_ErrorUsage` (vs `TestRunChild_Exit2_NotProjectStderr_SkipsNoProject`) | COMPLIANT |
| SessionEnd Sync Trigger | SessionEnd fires the runner | `settings_test.go > TestBuildSyncTriggerSessionEndEntry_UsesPwdFallbackAndPosixGuard` + live `dash -c` exercise | COMPLIANT |
| SessionEnd Sync Trigger | Stop never fires the sync trigger | `settings_test.go > TestMerge_AddsSessionEndSyncTrigger_CoexistsWithForeign` (asserts `countOurHooks(root,"Stop")==0`) | COMPLIANT |
| SessionEnd Sync Trigger | Install coexists with foreign entries | `settings_test.go > TestMerge_AddsSessionEndSyncTrigger_CoexistsWithForeign` | COMPLIANT |
| SessionEnd Sync Trigger | Install is idempotent | `settings_test.go > TestMerge_Idempotent`, `TestSchema_InstallTwice_Idempotent` | COMPLIANT |
| SessionEnd Sync Trigger | Uninstall removes only the owned entry | `settings_test.go > TestUninstall_RemovesSessionEndSyncTrigger_LeavesForeign` | COMPLIANT |
| SessionEnd Sync Trigger | status-hooks reports the SessionEnd family | `main_test.go > TestStatusCore_SessionEndMissing_Degraded`, `TestStatusCore_SessionEndPresent_OK`, `TestStatusCore_AllOK` | COMPLIANT |
| Archive-Time Sync Trigger | Archive completion fires a whole-project sync | `overlay_longterm_mem_test.sh > case_sync_trigger_forwards_event_and_cwd` (wrapper half) + `case_archive_sync_trigger_call_site_is_only_in_inception_pipeline` (call-site half) | COMPLIANT |
| Archive-Time Sync Trigger | Trigger lives outside the managed skill | `overlay_longterm_mem_test.sh > case_archive_sync_trigger_call_site_is_only_in_inception_pipeline` (asserts the verbatim call line is present in `inception-pipeline/SKILL.md` and `sync-trigger` is absent from `sdd-archive/SKILL.md`) | COMPLIANT |

#### `specs/runtime-lifecycle/spec.md`

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| Claude Lifecycle Support | Claude install succeeds | `claude_test.go > TestClaudeInstallWritesLifecycleHooksAndReportsSupportedStatus` | COMPLIANT |
| Claude Lifecycle Support | Claude status is honest | `claude_test.go > TestClaudeStatusRequiresFullLifecycleState`, `TestClaudeStatusFailsWhenSettingsIsMalformed` | COMPLIANT |
| Claude Lifecycle Support | Claude update refreshes lifecycle state | `claude_test.go > TestClaudeUpdateRefreshesLifecycleAndKeepsSupportedStatus` | COMPLIANT |
| Claude Lifecycle Support | Claude uninstall removes owned lifecycle state | `claude_test.go > TestClaudeUninstallRemovesOwnedHooksAndReturnsUnhealthyStatus` | COMPLIANT |
| Claude Lifecycle Support | Claude install includes the SessionEnd sync-trigger family | `settings_test.go > TestInstall_UpgradesTwoFamiliesToThree_PreservesExisting` + `claude_test.go > TestClaudeInstallWritesLifecycleHooksAndReportsSupportedStatus` | COMPLIANT |
| Claude Lifecycle Support | Claude status reports the SessionEnd family honestly | `settings_test.go > TestHasSupportedClaudeLifecycleState_RequiresSyncTriggerFamily`, `TestHasSupportedClaudeLifecycleState_RequiresDesignPair` | COMPLIANT |
| Claude Lifecycle Support | Claude uninstall removes the SessionEnd sync-trigger entry | `settings_test.go > TestUninstall_RemovesSessionEndSyncTrigger_LeavesForeign`, `claude_test.go` partial-state fixtures | COMPLIANT |

**Compliance summary**: 23/23 scenarios COMPLIANT.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| Non-Blocking Sync Runner Contract | Implemented | `engine/synctrigger/synctrigger.go`: `Run` returns 0 on every path (argv, log-open, `os.Executable`, `Start`); `RunChild` classifies and logs; `filepath.Abs` resolves a relative `--cwd` before the absolute check. |
| SessionEnd Sync Trigger | Implemented | `engine/settings/settings.go`: `LabdrianSyncTriggerIdentity`, `buildSyncTriggerSessionEndEntry`, `isSyncTriggerEntry` wired into `mergeHooks` (SessionEnd only), `removeHooks`, `HasSupportedClaudeLifecycleState`. |
| Archive-Time Sync Trigger | Implemented, guarded | Call site present at `skills/inception-pipeline/SKILL.md:201` with `\|\| true`, absent from `skills/sdd-archive/SKILL.md`, and now protected by `engine/shelltest/overlay_longterm_mem_test.sh > case_archive_sync_trigger_call_site_is_only_in_inception_pipeline`, which fails if the call line disappears from `inception-pipeline/SKILL.md` or leaks into `sdd-archive/SKILL.md`. |
| Claude Lifecycle Support (runtime-lifecycle) | Implemented | Three families / five entries across install, update, uninstall, and both status layers. |

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| Runner in `engine/synctrigger` + `sync-trigger` verb | Yes | |
| Always-zero parent, `error:usage`/`log`/`self`/`spawn` | Yes | Plus an added `error:binary-not-executable` outcome (validator finding F4) — a refinement, not a deviation. |
| Detach with `Setsid`, stdin `/dev/null` | Yes | |
| Child timeout 60s, `Setpgid`, group kill, `WaitDelay` | Yes | `TestRunChild_Timeout_KillsProcessGroup_NoOrphan` proves no orphan. |
| Exit mapping (2 split by stderr, 3/4/5 mapped) | Yes | |
| Log `$STATE_DIR/logs/sync-trigger.log`, child stderr then summary | Yes | Verified live in the dash exercise. |
| Hook identity `sync-trigger`, one SessionEnd entry, no matcher | Yes | |
| `legacyIdentities` unchanged | Yes | |
| Status: `runtime status` partial; engine `status` WARN/exit 2 | Yes | |
| Hook command `${CLAUDE_PROJECT_DIR:-$PWD}` | Yes | Corrected from the original `:-.}` after validator finding F1; POSIX guard replaces `&>/dev/null` for the new entry only. |
| Wrapper guards `-x $ENGINE_BINARY`, exit 0 in every branch | Yes | |
| Wrapper invocation shape (design.md Wrapper row) | Yes (now current) | Remediated commit `8d65686`: design.md's Wrapper row now describes the shipped detached background launch (`setsid`, falling back to a bare `&`, `disown`, return immediately) instead of the original "execs the verb" text. No further design/code gap. |
| Archive call site in closure-feedback step 4 | Yes | |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | Yes | Cycle-evidence tables present for slices 1 and 2, and for the remediation pass; slice 3's original tasks 3.1–3.5 remain prose-only (disclosed WARNING, not a gap in coverage). |
| All tasks have tests | Yes | 24/24 — task 3.4's scripted check now exists (`case_archive_sync_trigger_call_site_is_only_in_inception_pipeline`) and is registered in the case list. |
| RED confirmed (tests exist) | Yes | All named test files exist and were executed, including the remediation case. |
| GREEN confirmed (tests pass) | Yes | Every test file named in the evidence tables passes on re-execution here. |
| Triangulation adequate | Yes | `RunChild` 9 scenarios, `Run` 5, settings 4 new + 3 extended counts, statusCore 3. |
| Safety Net for modified files | Yes | Slices 1 and 2 record pre-edit suite-green for each modified package. |

**TDD Compliance**: 6/6 checks passed.

### Remediation Verification

| Prior finding | Fix commit | Re-verified evidence |
|---|---|---|
| CRITICAL 1 — "Trigger lives outside the managed skill" UNTESTED, task 3.4 checked without deliverable | `e554908` | `rg case_archive_sync_trigger_call_site_is_only_in_inception_pipeline engine/shelltest/overlay_longterm_mem_test.sh` finds both the function definition (line 2130) and its registration in the case list (line 2643); `bash engine/shelltest/overlay_longterm_mem_test.sh` passes it as `ok - the archive sync-trigger call site lives only in inception-pipeline/SKILL.md, not the managed sdd-archive/SKILL.md`. |
| WARNING 4 — design.md Wrapper row stale (described exec, not detach) | `8d65686` | `design.md`'s Wrapper row now reads "launches the verb detached in the background (`setsid`, falling back to a bare `&`) and returns immediately without waiting" — matches `bin/labdrian-overlay`'s shipped `cmd_longterm_mem` `sync-trigger` branch. |
| tasks.md 3.2/3.4 wording drift | `8d65686` | 3.2 now reads "launches the verb detached in the background (`setsid`, falling back to a bare `&`), and returns immediately without waiting"; 3.4 now names the actual delivered case `case_archive_sync_trigger_call_site_is_only_in_inception_pipeline`. |

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit | 14 (synctrigger) + 11 (settings, sync-trigger-related) + 5 (cmd) + 8 (runtime) | 4 | `go test` |
| Integration (shell) | 5 sync-trigger cases within 58 total (4 wrapper cases + 1 new call-site case) | 1 | `bash` shelltest harness |
| E2E | 0 | 0 | not installed |
| **Total** | **~43 change-relevant** | **5** | |

### Changed File Coverage

Coverage analysis skipped — no coverage tool configured (`coverage_threshold: 0`).

### Assertion Quality

All assertions verify real behavior, including the new remediation case: it asserts a literal
verbatim command-line match in `inception-pipeline/SKILL.md` (positive presence) and a substring
absence check in `sdd-archive/SKILL.md` (negative presence), both against real file content, not
mocks or trivial tautologies. Assertion density remains healthy across the change's test files
(`synctrigger_test.go` 49 assertions / 14 test funcs; `settings_test.go` 174 / 28; `claude_test.go`
37 / 8). No tautologies, no orphan empty-collection checks, no type-only assertions standing
alone, and no ghost loops.

**Assertion quality**: 0 CRITICAL, 0 WARNING

### Quality Metrics

**Linter (`go vet`)**: No errors across both modules.
**Shell linter (`shellcheck -S warning`)**: 2 pre-existing SC2064 warnings at lines 1314/1475, both known-environmental and untouched by this change.
**Type Checker**: Covered by `go vet` / `go build` — no errors.
**Formatting**: `gofmt -l .` reported clean during apply; no drift observed.

### Admission Gate

```text
gentle-ai sdd-verify-validate --input <report> --requirements 4 --scenarios 23
exit 0 (see command evidence below)
```

Totals recounted from the delta specs at HEAD `8d65686`:
`rg -c '^### (Requirement|REQ-[0-9]+):'` → `longterm-mem-sync-triggers/spec.md` 3,
`runtime-lifecycle/spec.md` 1 (**4 requirements**);
`rg -c '^#### Scenario:'` → 16 and 7 (**23 scenarios**).

### Issues Found

**CRITICAL**: None

**WARNING**:

1. **Configured repo-wide `rules.verify.test_command` exits 1** on
   `tools/archive-reconcile > TestShippedLedgerMatchesThisRepository`. The guard fails because
   `longterm-mem-sync-triggers` is 24/24 complete and unarchived, and because
   `openspec/specs/longterm-mem-sync-triggers/spec.md` is not yet promoted. `tools/` is
   untouched by this change. This is the expected pre-archive state and self-clears when
   `sdd-archive` promotes both delta specs and moves the change folder; it is not a defect in
   this change and must not be treated as one.
2. **Slice 3's original tasks 3.1–3.5 have no TDD Cycle Evidence table.** Slices 1 and 2 each
   carry one; slice 3's apply-progress section records the RED-first rewrite of
   `case_sync_trigger_does_not_block_on_a_wedged_engine` in prose only. The underlying work is
   verifiable and passing; the remediation pass (task 3.4) does carry its own evidence table.
   The reported evidence shape remains inconsistent across slices, but this is a reporting
   consistency issue, not a missing-test issue — unchanged from the prior verify pass and not
   re-litigated as a blocker.
3. **Self-reported strict-TDD ordering deviation for tasks 1.3/1.4.** `Run`'s RED test was
   written in the same edit pass as its GREEN implementation, so the RED gate was satisfied
   independently only for the `RunChild` half. Disclosed by apply; recorded here, not
   re-litigated.

**SUGGESTION**:

1. The `sync-trigger` log at `$STATE_DIR/logs/sync-trigger.log` is append-only with no
   rotation (design.md open question). Worth revisiting once `doctor` starts reading it.
2. Consider folding the single-case `TestRunChild_*` functions into the existing table, an
   optional cleanup explicitly skipped during the slice-1 findings fix.

### Verdict

PASS

All 4 requirements / 23 scenarios are COMPLIANT with a runtime-passing covering test, including
the previously-UNTESTED "Trigger lives outside the managed skill" scenario, now guarded by
`case_archive_sync_trigger_call_site_is_only_in_inception_pipeline`. `go vet`, `go test -race`
(engine, 12/12 packages), `go test` (longterm-mem, 17/17 packages), and the full 58-case shell
harness all pass; `shellcheck -S warning` shows only the two known pre-existing SC2064 warnings.
The repo-wide `tools/archive-reconcile` failure remains, unchanged, the expected pre-archive
stranded-change signal and is not a defect. Ready for `sdd-archive`.
