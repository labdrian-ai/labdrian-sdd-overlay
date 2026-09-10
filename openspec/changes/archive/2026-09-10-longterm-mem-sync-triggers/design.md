# Design: longterm-mem sync triggers at session close and archive

## Technical Approach

One zero-dependency engine verb, `gentle-ai-overlay sync-trigger --event session-end|archive --cwd DIR [--state-dir DIR]`, is the only runner. It detaches a child of itself, and the child runs `$STATE_DIR/bin/longterm-mem sync` (no `--project`; longterm-mem resolves identity from cwd) under a timeout, classifies the exit code, appends one log line, and the parent exits 0 on every path, including its own pre-spawn failures. `sync-trigger` is the verb name AND the hook identity token. The proposal's "fifth family" wording is superseded post-#291: `SessionEnd` is the **third** family beside the minimalism-contract and anti-generic-design pairs. Three stacked slices: `sync-runner`, `session-end-hook`, `archive-trigger`.

## Architecture Decisions

| Decision | Choice | Rejected | Rationale |
|---|---|---|---|
| Runner location | New package `engine/synctrigger` + `sync-trigger` verb in `engine/cmd/main.go` | Bash in `bin/labdrian-overlay` | The hook command already targets `$ENGINE_BINARY`; Go gets table tests with a fake `longterm-mem` script in `t.TempDir()`; bash cannot classify exits or bound time portably |
| Always-zero parent | `Run` order: (1) validate own argv (`--event` in set, `--cwd` absolute) → (2) `MkdirAll($STATE_DIR/logs)` + `OpenFile(append)` → (3) `os.Executable()` → (4) `cmd.Start()`. Any error in (1)-(4) writes one line to stderr, and to the log when already open, as `error:usage` / `error:log` / `error:self` / `error:spawn`, then `return 0`. Parent never calls `os.Exit` with non-zero; `runSyncTrigger` in `main.go` ignores `Run`'s value beyond logging | Fail loud like `merge-settings` | R-003: the host must never see non-zero. ETXTBSY on `Start` (engine rebuilt mid-session) is exactly case (4) |
| Detach | `exec.Command(os.Executable(), "sync-trigger", ..., "--child")` with `SysProcAttr{Setsid: true}`, stdin `/dev/null`, stdout/stderr = log handle, `Start()` then return | `nohup`/`&` in the hook string; `async: true` hook flag | Setsid survives Claude Code's process-group teardown at SessionEnd; `async` has no archive-path equivalent. `--child` is internal; tests call `RunChild` directly |
| Timeout | Child: `context.WithTimeout(60s)`, `exec.CommandContext`, `Setpgid: true`, `cmd.Cancel` kills the process group, `WaitDelay: 5s` (Go 1.21) | Plain `CommandContext` | `sync` spawns vault Python; killing only the direct child leaves orphans |
| Project resolution | Hook passes `--cwd "${CLAUDE_PROJECT_DIR:-$PWD}"`; child sets `Dir=cwd`, omits `--project` | Engine re-implements `projectid` | `internal/projectid` is unimportable from the engine module; longterm-mem's `adoptFromWorkingDirectory` is the single identity authority |
| Exit mapping | Child captures stderr to a buffer. 0 `ok`; 2 → `skip:no-project` only when stderr contains `could not be resolved from the working directory` (`project_resolve.go:77`), else `error:usage`; 3 `skip:no-vault`; 4 `failure:engram-unavailable`; 5 `failure:vault-subprocess`; timeout `timeout`; other `failure:exit-N`; binary absent `skip:no-binary` | Map every 2 to `skip:no-project` | Exit 2 is generic usage (`exit_codes.go:36`, any `flag.Parse` error); the runner pre-validates its own argv so a residual 2 is a real contract skew and must be visible |
| Log | `$STATE_DIR/logs/sync-trigger.log`, append, `0644`; child stderr lines then one summary line `ts event=<e> cwd=<dir> outcome=<o> exit=<n> duration=<d>` | Syslog, stdout | `~/.labdrian-overlay` is the operator's known state root |
| Hook identity | `LabdrianSyncTriggerIdentity = "sync-trigger"`; `isSyncTriggerEntry = binary && token`; one `SessionEnd` entry `{"hooks":[{"type":"command","command":...}]}`, no matcher | Reusing an `--embedded-contract` token | Same name-based predicate shape as `isMinimalismEntry`/`isDesignEntry` |
| Removal | `removeHooks` iterates `{"UserPromptSubmit","PreToolUse","SessionEnd"}`; chain `isMinimalismEntry \|\| isDesignEntry \|\| isSyncTriggerEntry \|\| isLegacyEntry`. **`legacyIdentities` needs no change**: it holds retired tokens only; `sync-trigger` is live and would move there only if retired later | Adding `sync-trigger` to `legacyIdentities` | Per its doc comment that list is the backward-compat exception for tokens `mergeHooks` no longer writes |
| Status, two layers, one verdict | (a) `runtime status --target claude`: `HasSupportedClaudeLifecycleState` also requires `HasLabdrianSyncTriggerHook(root,"SessionEnd")` → `partial`, message `"...not fully owned/installed in <path>; run 'labdrian uninstall-hooks' then 'labdrian install-hooks'"`. (b) engine `status` (what `status-hooks` execs): new `checkSessionEndHook` appended after `checkPreToolUseHook`; a missing family is **WARN/degraded** (`ok:true, degraded:true`, note names the same two commands) → exit 2, not FAIL/exit 1; unreadable settings stays FAIL like the other hook checks | FAIL (exit 1) pre-upgrade | `partial` is not `unsupported`; the WARN tier (README:116 "exits non-zero" is satisfied by 2) already means "present-but-needs-attention", which is exactly a two-pair install |
| Archive call site | Closure-feedback step 4: `labdrian longterm-mem sync-trigger --event archive --cwd "$root" \|\| true` after the three writes succeed | Calling the engine path from the skill | Wrapper resolves `$ENGINE_BINARY`/`$STATE_DIR` and the stale-engine warning; runs after the STOP-on-partial-write rule |
| Wrapper | `cmd_longterm_mem`: add `sync-trigger` to the `install\|status\|uninstall` whitelist (`bin/labdrian-overlay:3686`); branch guards `-x $ENGINE_BINARY` (else `warn` + `return 0`), launches `sync-trigger --state-dir "$STATE_DIR" "$@"` in the background via `setsid` (falling back to a bare `&` when `setsid` is unavailable) with stdin/stdout/stderr redirected, `disown`s it, and returns immediately without waiting | Exec, or a bounded `timeout` wait | Exit 0 in every branch (R-003); the wrapper never blocks its caller, even against a wedged engine — corrected post-review from the original exec/bounded-wait design (see verify-report WARNING 4) |

## Data Flow

    Claude SessionEnd ─┐                       ┌─ parent: argv → log → self → Setsid child; return 0
    closure-feedback ──┴─▶ gentle-ai-overlay sync-trigger ─┤
                                                           └─ child: longterm-mem sync (Dir=cwd, 60s, stderr buffered)
                                                                 └─▶ vault write ─▶ logs/sync-trigger.log

## File Changes

| Slice | File | Action |
|---|---|---|
| 1 `sync-runner` | `engine/synctrigger/synctrigger.go` + `_test.go` | Create: `Options{Event,Cwd,StateDir,Binary,Timeout,Self string; Stderr io.Writer}`, `Run` (parent, always 0), `RunChild`, `classify(exit int, stderr string) string`, `openLog`, `appendLog` |
| 1 | `engine/cmd/main.go` + `main_test.go` | `case "sync-trigger"`, usage line, `runSyncTrigger` (flag parse delegated to `Run`; always `os.Exit(0)`) |
| 2 `session-end-hook` | `engine/settings/settings.go` + `_test.go` | Identity const, `HasLabdrianSyncTriggerHook`, `buildSyncTriggerSessionEndEntry`, `isSyncTriggerEntry`, `mergeHooks`/`removeHooks`/`HasSupportedClaudeLifecycleState` |
| 2 | `engine/cmd/main.go` + `main_test.go` | `checkSessionEndHook` in `statusCore` |
| 2 | `engine/runtime/claude.go` + `claude_test.go` | Partial message names the upgrade step |
| 2 | `bin/labdrian-overlay` (2620-2621, 2703), `README.md` (115, 323) | Wording: "three hook families (two pairs + SessionEnd sync-trigger), five entries" |
| 3 `archive-trigger` | `bin/labdrian-overlay` (3686 whitelist, `cmd_longterm_mem` branch, usage) + `engine/shelltest` | Wrapper verb |
| 3 | `skills/inception-pipeline/SKILL.md` | Execute step 4 + gotcha line |
| 2 | `openspec/specs/runtime-lifecycle/spec.md` | Delta via sdd-spec |

## Interfaces / Contracts

```go
type Options struct { Event, Cwd, StateDir, Binary, Self string; Timeout time.Duration; Stderr io.Writer }
func Run(o Options) int                 // parent: 0 on every path
func RunChild(o Options) Outcome         // Outcome{Kind string; Exit int; Dur time.Duration}
func classify(exit int, stderr string) string
```

Hook command: `command -v <bin> &>/dev/null && <bin> sync-trigger --event session-end --cwd "${CLAUDE_PROJECT_DIR:-$PWD}" || true`.

## Testing Strategy (strict TDD; RED then GREEN per row; each slice < 400 authored lines)

| Slice | RED test | GREEN |
|---|---|---|
| 1 | `synctrigger_test.go` table on `RunChild` with a fake `longterm-mem` script: missing binary → `skip:no-binary`; exit 2 + not-a-repo stderr → `skip:no-project`; exit 2 + other stderr → `error:usage`; exit 3/4/5/1 → mapped kinds; sleeping script vs 200ms timeout → `timeout` and no orphan (pgid probe); exit 0 → `ok`; each asserts one summary line | `RunChild`, `classify`, `appendLog` |
| 1 | `Run` table: bad `--event` → 0 + stderr `error:usage`; unwritable `StateDir/logs` (0500 dir) → 0 + stderr `error:log`; `Self` non-executable path → 0 + log `error:spawn`; happy path → 0 and child log line appears within 2s | `Run` |
| 1 | `main_test.go`: `runSyncTrigger` with no args exits 0 (`exit` injected like `runRuntimeCore`) | verb wiring |
| 2 | `settings_test.go`: `TestMerge_AddsSessionEndSyncTrigger_CoexistsWithForeign` (moshi-shaped `SessionEnd`+`Stop` seeds; 1 ours in `SessionEnd`, 0 in `Stop`, foreign intact); `TestUninstall_RemovesSessionEndSyncTrigger_LeavesForeign`; extend `TestUninstall_CountIsZeroAfterInstall` (750-765), `TestMerge_Idempotent` (271-281), `TestSchema_InstallTwice_Idempotent` (992) with `SessionEnd` counts (per-key "2" assertions stay 2); `TestRemoveHooksCleansUpLegacySafetyAndProjectionEntries` (478) unchanged as the `legacyIdentities` guard | builder, `mergeHooks`, `removeHooks` |
| 2 | `TestHasSupportedClaudeLifecycleState_RequiresDesignPair` (1093) `both` flips to `false`; new `..._RequiresSyncTriggerFamily` via helper `withSyncTriggerFamily(root, hookCommand)` over unchanged `buildRootWithPairs(hookCommand, includeMinimalism, includeDesign)`; `TestInstall_UpgradesTwoFamiliesToThree_PreservesExisting` | predicate |
| 2 | `main_test.go`: `TestStatusCore_AllOK` (985) flips degraded until its fixture gains `SessionEnd`; new `TestStatusCore_SessionEndMissing_Degraded` (exit tier 2, note names both commands) and `TestStatusCore_SessionEndPresent_OK`; `claude_test.go` partial-message test | `checkSessionEndHook`, message |
| 3 | `engine/shelltest`: `longterm-mem sync-trigger` with absent engine returns 0; with fake engine forwards `--event archive --cwd`; unknown subcommand still dies; `shellcheck -S warning` | wrapper |
| 3 | Scripted check: closure-feedback command string appears verbatim in `SKILL.md` with `\|\| true` | skill edit |

## Threat Matrix

| Boundary | Applicability | Response | RED |
|---|---|---|---|
| Documentation-like paths | N/A: no file classification | — | — |
| Git repository selection | Applicable: `--cwd` from `CLAUDE_PROJECT_DIR`/`$PWD`; relative, missing, non-repo dirs | Relative → `error:usage`; non-repo → exit 2 + stderr match → `skip:no-project` | slice 1 rows |
| Commit / Push / PR | N/A: no VCS mutation | — | — |
| Subprocess spawn/timeout/orphans | Applicable | Pre-`Start` errors → 0; `Setpgid` + group kill + `WaitDelay` | slice 1 rows |

## Migration / Rollout

The post-#291 upgrade path is `labdrian uninstall-hooks` (strips retired entries via `legacyIdentities`) then `labdrian install-hooks`; the `SessionEnd` family rides that same step. Until it runs: `runtime status` → `partial`, `status-hooks` → WARN/exit 2, both naming the two commands. A plain re-`install-hooks` on a two-pair machine also adds only the new entry (byte-stable pairs proven by the upgrade test). `uninstall-hooks` removes all five. No data migration.

## Open Questions

- [ ] Log rotation: none (append-only); revisit if `doctor` starts reading the file.
- [ ] `sync` records derived project names in the identity ledger on every firing; flag if ledger noise appears.
