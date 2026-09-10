```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:95529b225596666b639fe699bbc5c9c0c61532230c8b54a4ac3514c72e63ee86
verdict: fail
blockers: 1
critical_findings: 1
requirements: 3/4
scenarios: 21/23
test_command: cd engine && go test -count=1 -race ./... && cd ../longterm-mem && go test ./...
test_exit_code: 0
test_output_hash: sha256:48ad120cc4dff90016ae0f9451f6b30c1129ce246d72eb10e736017bebba6edd
build_command: cd engine && go vet ./... && cd ../longterm-mem && go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: longterm-mem-sync-triggers
**Version**: N/A (delta specs `longterm-mem-sync-triggers` + `runtime-lifecycle`)
**Mode**: Strict TDD
**Worktree**: `/home/labdrian/labdrian-sdd-overlay-worktrees/lmst-3` (branch `feat/longterm-mem-sync-triggers-3-archive`, HEAD `76d127a`, base `main`)

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 24 |
| Tasks complete | 24 |
| Tasks incomplete | 0 |
| Tasks marked complete without a delivered artifact | 1 (3.4) |

Planned `review_slices` 3; realized 3 stacked PRs (#299, #300, slice-3 branch pending its PR). `size:exception` granted for slices 1 and 2.

### Build & Tests Execution

**Build (vet)**: PASSED

```text
cd engine && go vet ./... && cd ../longterm-mem && go vet ./...
exit 0 — empty output (no diagnostics)
```

**Tests**: PASSED

```text
cd engine && go test ./synctrigger/... ./settings/... ./cmd/... ./runtime/... ./shelltest/...
exit 0 — synctrigger ok 0.208s, settings ok 0.011s, cmd ok 0.032s, runtime ok 0.156s, shelltest ok 9.709s

cd engine && go test -count=1 -race ./...
exit 0 — 12/12 packages ok (assets, cmd, gadu, gate, installer, prespec, propagator, runtime, settings, shelltest, skills, synctrigger)

cd longterm-mem && go test ./...
exit 0 — 17/17 packages ok

bash engine/shelltest/overlay_longterm_mem_test.sh
exit 0 — all shell test cases passed, including the four sync-trigger cases

shellcheck -S warning bin/labdrian-overlay
exit 1 — only the two known pre-existing SC2064 warnings at lines 1314 and 1475
(`trap "git checkout '${current_branch}' ..."`), both declared known-environmental
by the orchestrator and untouched by this change.
```

**Configured `rules.verify.test_command` (repo-wide)**: exit 1 — see WARNING 1. The single
failure is `tools/archive-reconcile > TestShippedLedgerMatchesThisRepository`, which fails
*because* this change is 24/24 complete and not yet archived. `tools/` is untouched by this
change (`git diff --stat main...HEAD -- tools/` is empty), and the guard's own stderr
prescribes promotion + archive as the fix. It is a pre-archive workflow-state signal, not a
code defect, and it clears when `sdd-archive` runs.

**Coverage**: Not available — no coverage tooling configured (`coverage_threshold: 0`).

### Additional Runtime Exercises (foreground, observed)

| Exercise | Observed result |
|---|---|
| Emitted SessionEnd hook string under `dash -c`, `CLAUDE_PROJECT_DIR` unset, scratch `$HOME` with a fake `longterm-mem` | exit 0; log line `event=session-end cwd=/…/hookexec/proj outcome=ok exit=0 duration=1ms` with an **absolute** cwd; the fake binary's stderr is teed into the log above the summary line. Confirms the `${CLAUDE_PROJECT_DIR:-$PWD}` fallback and the POSIX `>/dev/null 2>&1` guard both work under dash. |
| `bin/labdrian-overlay longterm-mem sync-trigger --event archive --cwd <repo>` against a wedged fake engine (`sleep 30`) | exit 0 in **13 ms** (bound: 2 s); the wedged engine PID is still alive (`kill -0`) after the wrapper returns, proving the wrapper detaches rather than merely bounding its wait. |

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
| Archive-Time Sync Trigger | Archive completion fires a whole-project sync | `overlay_longterm_mem_test.sh > sync-trigger forwards --state-dir and the caller's --event/--cwd to the engine verbatim` (wrapper half only; the closure-feedback call site is static evidence) | PARTIAL |
| Archive-Time Sync Trigger | Trigger lives outside the managed skill | (none found) | UNTESTED |

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

**Compliance summary**: 21/23 scenarios COMPLIANT, 1 PARTIAL, 1 UNTESTED.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| Non-Blocking Sync Runner Contract | Implemented | `engine/synctrigger/synctrigger.go`: `Run` returns 0 on every path (argv, log-open, `os.Executable`, `Start`); `RunChild` classifies and logs; `filepath.Abs` resolves a relative `--cwd` before the absolute check. |
| SessionEnd Sync Trigger | Implemented | `engine/settings/settings.go`: `LabdrianSyncTriggerIdentity`, `buildSyncTriggerSessionEndEntry`, `isSyncTriggerEntry` wired into `mergeHooks` (SessionEnd only), `removeHooks`, `HasSupportedClaudeLifecycleState`. |
| Archive-Time Sync Trigger | Implemented, unguarded | Call site present at `skills/inception-pipeline/SKILL.md:201` with `|| true`, and absent from `skills/sdd-archive/SKILL.md` (verified by inspection). No automated regression guard exists — see CRITICAL 1. |
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
| Wrapper invocation shape | Deviated (improved) | design.md said "execs `sync-trigger`"; the review correction replaced both the exec and the later bounded `timeout` with a fully detached background launch. This strengthens R-003 (non-blocking, not merely bounded) and is proven by the 13 ms / still-alive-PID exercise. `design.md` was not updated to match. |
| Archive call site in closure-feedback step 4 | Yes | |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | Partial | Cycle-evidence tables present for slices 1 and 2; slice 3 records RED-first only in prose, with no table. |
| All tasks have tests | No | 23/24 — task 3.4's scripted check was never written. |
| RED confirmed (tests exist) | Partial | All named test files exist and were executed, except task 3.4's check, which does not exist. |
| GREEN confirmed (tests pass) | Yes | Every test file named in the evidence tables passes on re-execution here. |
| Triangulation adequate | Yes | `RunChild` 9 scenarios, `Run` 5, settings 4 new + 3 extended counts, statusCore 3. |
| Safety Net for modified files | Yes | Slices 1 and 2 record pre-edit suite-green for each modified package. |

**TDD Compliance**: 4/6 checks fully passed, 2 partial.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit | 14 (synctrigger) + 11 (settings, sync-trigger-related) + 5 (cmd) + 8 (runtime) | 4 | `go test` |
| Integration (shell) | 4 sync-trigger cases within 25 total | 1 | `bash` shelltest harness |
| E2E | 0 | 0 | not installed |
| **Total** | **~42 change-relevant** | **5** | |

### Changed File Coverage

Coverage analysis skipped — no coverage tool configured (`coverage_threshold: 0`).

### Assertion Quality

All assertions verify real behavior. Assertion density is healthy
(`synctrigger_test.go` 49 assertions / 14 test funcs; `settings_test.go` 174 / 28;
`claude_test.go` 37 / 8). No tautologies, no orphan empty-collection checks, no
type-only assertions standing alone, and no ghost loops — the two `range` loops in
`synctrigger_test.go` iterate static table literals that can never be empty. The single
`t.Skip` at `synctrigger_test.go:357` is a legitimate root-permission guard for the
unwritable-directory case, not a suppressed assertion.

**Assertion quality**: 0 CRITICAL, 0 WARNING

### Quality Metrics

**Linter (`go vet`)**: No errors across both modules.
**Shell linter (`shellcheck -S warning`)**: 2 pre-existing SC2064 warnings at lines 1314/1475, both known-environmental and untouched by this change.
**Type Checker**: Covered by `go vet` / `go build` — no errors.
**Formatting**: `gofmt -l .` reported clean during apply; no drift observed.

### Admission Gate

```text
gentle-ai sdd-verify-validate --input <report> --requirements 4 --scenarios 23
exit 0 — {"valid":true,"verdict":"fail","evidence_revision":"sha256:95529b2255…"}
```

Totals recounted from the delta specs at HEAD `76d127a`:
`rg -c '^### (Requirement|REQ-[0-9]+):'` → `longterm-mem-sync-triggers/spec.md` 3,
`runtime-lifecycle/spec.md` 1 (**4 requirements**);
`rg -c '^#### Scenario:'` → 16 and 7 (**23 scenarios**).

### Issues Found

**CRITICAL**:

1. **Scenario "Trigger lives outside the managed skill" is UNTESTED, and task 3.4 is marked
   complete without a deliverable.** `tasks.md:54` claims a RED scripted check asserting that
   the closure-feedback command string with `|| true` appears verbatim in
   `skills/inception-pipeline/SKILL.md`. No such check exists anywhere in the tree: commit
   `69a5d37` added only `skills/inception-pipeline/SKILL.md` (+9) and `tasks.md`, with no test
   file, and no Go test or shelltest case references the archive call site
   (`rg 'sync-trigger' --glob '*_test.go' --glob '*.sh'` returns only the slice-1/2 files and
   the wrapper shelltest). The behavior is currently correct by inspection — the line is at
   `skills/inception-pipeline/SKILL.md:201` and is absent from `skills/sdd-archive/SKILL.md` —
   but it is unguarded. The spec made this its own scenario precisely because
   `sdd-archive/SKILL.md` is managed and overwritten on update: if the call site ever migrates
   there, the archive trigger is silently lost on the next update with nothing to catch it.
   **Fix**: add the planned scripted check (assert the verbatim `|| true` command string in
   `skills/inception-pipeline/SKILL.md` and its absence from `skills/sdd-archive/SKILL.md`),
   then re-verify. This also upgrades the PARTIAL scenario above.

**WARNING**:

1. **Configured repo-wide `rules.verify.test_command` exits 1** on
   `tools/archive-reconcile > TestShippedLedgerMatchesThisRepository`. The guard fails because
   `longterm-mem-sync-triggers` is 24/24 complete and unarchived, and because
   `openspec/specs/longterm-mem-sync-triggers/spec.md` is not yet promoted. `tools/` is
   untouched by this change. This is the expected pre-archive state and self-clears when
   `sdd-archive` promotes both delta specs and moves the change folder; it is not a defect in
   this change and must not be treated as one.
2. **Slice 3 has no TDD Cycle Evidence table.** Slices 1 and 2 each carry one; slice 3's
   apply-progress section records the RED-first rewrite of
   `case_sync_trigger_does_not_block_on_a_wedged_engine` in prose only. The underlying work is
   verifiable (the tightened 2 s bound and the `kill -0` liveness assertion are both present and
   passing), but the reported evidence shape is inconsistent across slices.
3. **Self-reported strict-TDD ordering deviation for tasks 1.3/1.4.** `Run`'s RED test was
   written in the same edit pass as its GREEN implementation, so the RED gate was satisfied
   independently only for the `RunChild` half. Disclosed by apply; recorded here, not
   re-litigated.
4. **`design.md` not updated after the wrapper review correction.** The design still describes
   the wrapper as exec'ing the engine; the shipped wrapper launches it fully detached. The code
   is the stronger of the two, but the design now under-describes the contract it guarantees.

**SUGGESTION**:

1. The `sync-trigger` log at `$STATE_DIR/logs/sync-trigger.log` is append-only with no
   rotation (design.md open question). Worth revisiting once `doctor` starts reading it.
2. Consider folding the single-case `TestRunChild_*` functions into the existing table, an
   optional cleanup explicitly skipped during the slice-1 findings fix.

### Verdict

FAIL

One CRITICAL: spec scenario "Trigger lives outside the managed skill" has no covering test, and
task 3.4 is marked complete without its deliverable. All 23 scenarios are behaviorally correct
and every executed test, vet, race, and shell suite passes; the sole blocker is the missing
regression guard the change's own plan committed to. Add that check and re-verify to reach
archive readiness.
