# Tasks: longterm-mem Sync Triggers

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 550-900 total across 3 slices |
| 400-line budget risk | Medium (each slice under 400; total exceeds) |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 sync-runner → PR 2 session-end-hook → PR 3 archive-trigger |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 sync-runner | verb: detach, timeout, classify, log | PR 1 | `cd engine && go test ./cmd/... ./synctrigger/...` | fake `longterm-mem` script in `t.TempDir()` | delete `engine/synctrigger/`, revert `main.go` case |
| 2 session-end-hook | hook family, status check, wording | PR 2 | `cd engine && go test ./settings/... ./cmd/...` | N/A — pure Go table tests | revert `settings.go`/`main.go`/`claude.go` hunks |
| 3 archive-trigger | wrapper + closure-feedback call site | PR 3 | `shellcheck -S warning bin/labdrian-overlay` | `engine/shelltest` fake-engine script | revert wrapper whitelist + `SKILL.md` step |

## Phase 1: sync-runner (PR 1, R-003)

- [x] 1.1 RED `engine/synctrigger/synctrigger_test.go`: `RunChild` table — no binary→`skip:no-binary`; exit2+not-a-repo stderr→`skip:no-project`; exit2+other→`error:usage`; exit3/4/5/1→mapped; sleeping fake vs short timeout→`timeout`, no orphan; exit0→`ok`
- [x] 1.2 GREEN `engine/synctrigger/synctrigger.go`: `Options`, `RunChild`, `classify`, `openLog`, `appendLog`
- [x] 1.3 RED same file: `Run` table — bad `--event`→0+`error:usage`; unwritable logs dir (0500)→0+`error:log`; non-exec `Self`→0+`error:spawn`; happy path→0, log line within 2s
- [x] 1.4 GREEN `synctrigger.go` `Run`: validate argv → open log → `os.Executable()` → detached `Start()` (`Setsid`) → always `return 0`
- [x] 1.5 RED `engine/cmd/main_test.go`: `runSyncTrigger` with no args exits 0 (injected `exit`)
- [x] 1.6 GREEN `engine/cmd/main.go`: `case "sync-trigger"`, usage line, `runSyncTrigger`

## Phase 2: session-end-hook (PR 2, R-002, R-003)

- [x] 2.1 RED `engine/settings/settings_test.go`: `TestMerge_AddsSessionEndSyncTrigger_CoexistsWithForeign`, `TestUninstall_RemovesSessionEndSyncTrigger_LeavesForeign`
- [x] 2.2 GREEN `engine/settings/settings.go`: `LabdrianSyncTriggerIdentity`, `buildSyncTriggerSessionEndEntry`, `isSyncTriggerEntry`; wire into `mergeHooks`/`removeHooks`
- [x] 2.3 Extend counts for `SessionEnd`: `TestUninstall_CountIsZeroAfterInstall`, `TestMerge_Idempotent`, `TestSchema_InstallTwice_Idempotent`; leave `legacyIdentities`/`TestRemoveHooksCleansUpLegacySafetyAndProjectionEntries` unchanged
- [x] 2.4 RED `TestHasSupportedClaudeLifecycleState_RequiresSyncTriggerFamily` (via `withSyncTriggerFamily`), `TestInstall_UpgradesTwoFamiliesToThree_PreservesExisting`
- [x] 2.5 GREEN `HasSupportedClaudeLifecycleState` requires `HasLabdrianSyncTriggerHook(root,"SessionEnd")`
- [x] 2.6 RED `engine/cmd/main_test.go`: update `buildSettingsWithHooks` (line 929) to add a `SessionEnd` sync-trigger entry, flipping `TestStatusCore_AllOK` (line 985) to the fully-installed case; add `TestStatusCore_SessionEndMissing_Degraded` (WARN, exit tier 2, note names both remediation commands) and `TestStatusCore_SessionEndPresent_OK` — 3 `TestStatusCore_*` fixtures affected (`rg 'func TestStatusCore_'` → 14 total in file)
- [x] 2.7 GREEN `engine/cmd/main.go`: `checkSessionEndHook` in `statusCore`, after `checkPreToolUseHook`, WARN/degraded not FAIL
- [x] 2.8 RED `engine/runtime/claude_test.go`: partial-message test names the upgrade step
- [x] 2.9 GREEN `engine/runtime/claude.go`: partial message text
- [x] 2.10 Docs `bin/labdrian-overlay` (2620-2621, 2703), `README.md` (115, 323): "three hook families (two pairs + SessionEnd sync-trigger), five entries"

## Phase 3: archive-trigger (PR 3, R-001, R-003)

- [x] 3.1 RED `engine/shelltest`: absent engine returns 0; fake engine forwards `--event archive --cwd`; unknown subcommand still dies
- [x] 3.2 GREEN `bin/labdrian-overlay`: add `sync-trigger` to whitelist (line 3686); `cmd_longterm_mem` branch guards `-x $ENGINE_BINARY`, execs verb, updates usage
- [x] 3.3 Verify: `shellcheck -S warning bin/labdrian-overlay`
- [x] 3.4 RED scripted check: closure-feedback command string with `|| true` appears verbatim in `skills/inception-pipeline/SKILL.md`
- [x] 3.5 GREEN `skills/inception-pipeline/SKILL.md`: step 4 call site + gotcha line (`sdd-archive/SKILL.md` stays untouched)

## Phase 4: Full Verification

- [x] 4.1 `cd engine && go vet ./... && go test ./...`
- [x] 4.2 `cd longterm-mem && go vet ./... && go test ./...`
- [x] 4.3 `shellcheck -S warning bin/labdrian-overlay`
