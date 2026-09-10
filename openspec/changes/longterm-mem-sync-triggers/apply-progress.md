# Apply Progress: longterm-mem-sync-triggers

## Slice 1 — sync-runner (Phase 1, R-003)

**Status**: implemented, tests green, committed under a granted `size:exception`.

**Validator findings fixed** (commit `199d8e1`, +72/-7 lines, `engine/synctrigger/{synctrigger.go,synctrigger_test.go}` only): F2 — `Run` now writes a `sync-trigger: <outcome>: <err>` stderr notice for `error:self` and `error:spawn`, matching `error:usage`/`error:log`. F3 — `RunChild` tees the child's stderr into the log via `io.MultiWriter` so the raw `longterm-mem` stderr lines land before the summary `outcome=` line, per design.md's Log contract. F4 — a present-but-non-executable `longterm-mem` binary now maps to its own `error:binary-not-executable` outcome instead of colliding with the parent's `error:spawn`. F5 — corrected the fake-self test script/comment: the re-exec argv puts the event at `$3` and the cwd at `$5`, not `$2`/`$4`. The optional table-fold of the single-case `TestRunChild_*` functions was skipped (not attempted) to keep this fix scoped to the four confirmed findings.

### Granted Exception

- **Scope**: slice 1 `sync-runner` only
- **Authored lines**: 624 (`git diff --cached --shortstat -- engine`)
- **Reason**: strict-TDD coverage of the R-003 non-blocking sync runner contract — `Run`/`RunChild`/verb wiring are one cohesive `design.md` contract, and the overage is almost entirely the 13 mandated test scenarios, not padding.
- **Authorized by**: owner, 2026-09-09

### Plan vs Realized Slice Count

- Planned: 3 slices (`sync-runner` → `session-end-hook` → `archive-trigger`)
- Realized so far: 1 (`sync-runner`, this commit)

### Completed Tasks

- [x] 1.1 RED `engine/synctrigger/synctrigger_test.go`: `RunChild` table — no binary→`skip:no-binary`; exit2+not-a-repo stderr→`skip:no-project`; exit2+other→`error:usage`; exit3/4/5/1→mapped; sleeping fake vs short timeout→`timeout`, no orphan; exit0→`ok`
- [x] 1.2 GREEN `engine/synctrigger/synctrigger.go`: `Options`, `RunChild`, `classify`, `openLog`, `appendLog`
- [x] 1.3 RED same file: `Run` table — bad `--event`→0+`error:usage`; unwritable logs dir (0500)→0+`error:log`; non-exec `Self`→0+`error:spawn`; happy path→0, log line within 2s
- [x] 1.4 GREEN `synctrigger.go` `Run`: validate argv → open log → `os.Executable()` → detached `Start()` (`Setsid`) → always `return 0`
- [x] 1.5 RED `engine/cmd/main_test.go`: `runSyncTrigger` with no args exits 0 (injected `exit`)
- [x] 1.6 GREEN `engine/cmd/main.go`: `case "sync-trigger"`, usage line, `runSyncTrigger`

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1/1.2 | `engine/synctrigger/synctrigger_test.go` | Unit | N/A (new pkg) | ✅ compile-fail (undefined Options/RunChild) | ✅ `go test ./synctrigger/...` all pass | ✅ 9 RunChild scenarios (missing binary, exit2×2 branches, exit3/4/5/1, timeout+no-orphan, exit0) | ➖ None needed — already clean, no duplication |
| 1.3/1.4 | same file | Unit | N/A (new) | ✅ written alongside GREEN in same file (Run/RunChild designed together per design.md's single-file contract) | ✅ `go test -run 'TestRun_'` all pass | ✅ 4 Run scenarios (bad event, unwritable log dir 0500, non-exec self, happy-path within 2s) | ➖ None needed |
| 1.5/1.6 | `engine/cmd/main_test.go` | Unit | ✅ full `cmd` suite green before edit | ✅ `go vet` failed with `undefined: runSyncTriggerCore` before wiring | ✅ `go test ./cmd/...` passes after `case "sync-trigger"` + `runSyncTrigger`/`runSyncTriggerCore`/`parseSyncTriggerArgs` added | ➖ Single scenario (no-args → exit 0); spec requires only R-003's "never non-zero" | ➖ None needed |

**Deviation from strict RED-first ordering**: tasks 1.3/1.4's RED test was written into `synctrigger_test.go` in the same edit pass as `Run`'s GREEN implementation, because `Run` and `RunChild` are one cohesive contract per `design.md`'s "Interfaces / Contracts" section (both live in `synctrigger.go`, share `Options`). The test still executed against the already-compiled `Run` and passed on first run — functionally verified, but the RED gate (test failing before the code existed) was satisfied for the RunChild half only, not independently for `Run`. Noted per skill rule "if the design is wrong or incomplete, NOTE IT" — this is a process note, not a design deviation.

### Test Summary
- Total tests written: 14 (9 RunChild scenarios across 6 test funcs incl. 1 subtest table of 4, 4 Run scenarios, 1 main.go verb test)
- Total tests passing: 14/14
- Layers used: Unit (14)
- Pure functions created: `classify` (exit code + stderr → outcome string)

### Files Changed

| File | Action | What Was Done |
|------|--------|----------------|
| `engine/synctrigger/synctrigger.go` | Created | `Options`, `Outcome`, `Run` (parent, always 0), `RunChild` (child, classify+log), `classify`, `openLog`, `appendLog` |
| `engine/synctrigger/synctrigger_test.go` | Created | RunChild table (9 cases) + Run table (4 cases) |
| `engine/cmd/main.go` | Modified | `case "sync-trigger"`, usage line, `runSyncTrigger`/`runSyncTriggerCore`/`parseSyncTriggerArgs` |
| `engine/cmd/main_test.go` | Modified | `TestRunSyncTriggerCore_NoArgs_ExitsZero` |

### Verification (foreground, all observed)

- `cd engine && go test ./synctrigger/... ./cmd/...` → both packages `ok`
- `cd engine && go vet ./... && go test -count=1 ./...` → all 12 packages `ok`, vet clean
- `cd engine && gofmt -l .` → empty (no unformatted files)
- `git diff --cached --shortstat -- engine` → **624 insertions**, 4 files — over the 400-line budget
- Manual smoke (built to scratchpad `engine-bin`):
  - `sync-trigger --event session-end --cwd <repo> --state-dir <scratch-state-with-fake-longterm-mem>` → exit 0, log line `outcome=ok exit=0 duration=1ms`
  - Same with `--state-dir` pointing at a dir with no `bin/longterm-mem` → exit 0, log line `outcome=skip:no-binary`
  - Same with `logs/` dir `chmod 0500` (unwritable) → exit 0, stderr `sync-trigger: error:log: ... permission denied`

### Workload / PR Boundary

- Mode: chained PR slice (stacked-to-main), PR 1 of 3
- Current work unit: `1 sync-runner`
- Boundary: starts from clean `4b4fa19`; ends with `engine/synctrigger` package + `sync-trigger` verb wiring, fully tested — **not yet committed** pending budget decision
- Estimated review budget impact: 624 authored lines vs 400 budget (56% over). Rollback boundary if approved as-is: `git rm -r engine/synctrigger && git checkout 4b4fa19 -- engine/cmd/main.go engine/cmd/main_test.go`

### Resolution

**400-line budget exceeded, exception granted**: 624 authored lines (`git diff --cached --shortstat -- engine`) vs the 400-line cap given for this slice. The owner granted a `size:exception` on 2026-09-09 (see "Granted Exception" above) instead of the alternative two-commit split. Committed as two work units per `work-unit-commits`: the `engine/synctrigger` package + `main.go`/`main_test.go` wiring in one commit, and this apply-progress/tasks.md record in a second `docs(sdd)` commit.

### Remaining Tasks (this change, not this slice)

- [x] Phase 2: session-end-hook (PR 2, R-002/R-003) — done, see below
- [ ] Phase 3: archive-trigger (PR 3, R-001/R-003) — `bin/labdrian-overlay` wrapper, `engine/shelltest`, `skills/inception-pipeline/SKILL.md` — **out of scope for slice 2, not touched**
- [ ] Phase 4: Full verification across `engine`, `longterm-mem`, `shellcheck`

### Status

6/6 slice-1 tasks complete, verified, and committed under a granted `size:exception`. Ready for `sdd-verify` on slice 1.

---

## Slice 2 — session-end-hook (Phase 2, R-002, R-003)

**Status**: implemented, tests green, committed as two work units, within budget (no exception needed).

### Plan vs Realized Slice Count

- Planned: 3 slices (`sync-runner` → `session-end-hook` → `archive-trigger`)
- Realized so far: 2 (`sync-runner`, `session-end-hook`)

### Completed Tasks

- [x] 2.1 RED `engine/settings/settings_test.go`: `TestMerge_AddsSessionEndSyncTrigger_CoexistsWithForeign`, `TestUninstall_RemovesSessionEndSyncTrigger_LeavesForeign`
- [x] 2.2 GREEN `engine/settings/settings.go`: `LabdrianSyncTriggerIdentity`, `buildSyncTriggerSessionEndEntry`, `isSyncTriggerEntry`; wired into `mergeHooks`/`removeHooks`
- [x] 2.3 Extended counts for `SessionEnd`: `TestUninstall_CountIsZeroAfterInstall`, `TestMerge_Idempotent`, `TestSchema_InstallTwice_Idempotent`; `legacyIdentities`/`TestRemoveHooksCleansUpLegacySafetyAndProjectionEntries` left unchanged
- [x] 2.4 RED `TestHasSupportedClaudeLifecycleState_RequiresSyncTriggerFamily` (via new `withSyncTriggerFamily` helper), `TestInstall_UpgradesTwoFamiliesToThree_PreservesExisting`; flipped the "both" case of `TestHasSupportedClaudeLifecycleState_RequiresDesignPair` to expect `false` (two families is no longer sufficient)
- [x] 2.5 GREEN `HasSupportedClaudeLifecycleState` requires `HasLabdrianSyncTriggerHook(root,"SessionEnd")`
- [x] 2.6 RED `engine/cmd/main_test.go`: `buildSettingsWithHooks` now adds a `SessionEnd` sync-trigger entry, flipping `TestStatusCore_AllOK` to the fully-installed (not-degraded) case; added `TestStatusCore_SessionEndMissing_Degraded` (WARN, exit tier 2, note names both remediation commands) and `TestStatusCore_SessionEndPresent_OK`
- [x] 2.7 GREEN `engine/cmd/main.go`: `checkSessionEndHook` in `statusCore`, after `checkPreToolUseHook`; WARN/degraded on missing family, FAIL only on unreadable settings
- [x] 2.8 RED `engine/runtime/claude_test.go`: `TestClaudeStatusPartialMessageNamesRemediationCommands` — seeds a two-family (no SessionEnd) fixture and asserts the partial message names both commands
- [x] 2.9 GREEN `engine/runtime/claude.go`: partial message now appends `"; run 'labdrian uninstall-hooks' then 'labdrian install-hooks'"`
- [x] 2.10 Docs `bin/labdrian-overlay` (~2620-2621, 2703), `README.md` (~115, 323): updated to "three hook families (two pairs + SessionEnd sync-trigger), five entries"

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 2.1-2.5 | `engine/settings/settings_test.go` | Unit | ✅ full `settings` suite green before edit (14 pre-existing tests) | ✅ `go vet` failed `undefined: settings.LabdrianSyncTriggerIdentity` before any prod code | ✅ `go test ./settings/...` all pass after `LabdrianSyncTriggerIdentity`, `HasLabdrianSyncTriggerHook`, `isSyncTriggerEntry`, `buildSyncTriggerSessionEndEntry` added and wired | ✅ 4 new scenarios (coexist-with-foreign install, coexist-with-foreign uninstall, family-required-false, family-required-true) + 3 extended count assertions + 1 upgrade-preserves-existing test | ➖ None needed — mirrors the existing minimalism/design pair shape exactly |
| 2.6-2.7 | `engine/cmd/main_test.go` | Unit | ✅ full `cmd` suite green before edit | ✅ `TestStatusCore_SessionEndMissing_Degraded` failed (no WARN emitted, no remediation text) before `checkSessionEndHook` existed | ✅ `go test ./cmd/...` all pass after `checkSessionEndHook` wired into `statusCore` | ✅ 3 scenarios: all-OK (not degraded), SessionEnd missing (WARN, tier 2, note text), SessionEnd present (OK) | ➖ Extracted `remediationNote` const shared by the WARN note and reused for consistency |
| 2.8-2.9 | `engine/runtime/claude_test.go` | Unit | ✅ full `runtime` suite green before edit | ✅ new test failed: message lacked `labdrian uninstall-hooks`/`labdrian install-hooks` substrings | ✅ `go test ./runtime/...` all pass after message updated | ➖ Single scenario — the message text has one shape, no branching | ➖ None needed |
| 2.10 | `bin/labdrian-overlay`, `README.md` | N/A | N/A (docs) | N/A | N/A | N/A (structural doc edit, no branching) | N/A |

### Test Summary
- Total tests written: 9 new (`TestHasSupportedClaudeLifecycleState_RequiresSyncTriggerFamily`, `TestInstall_UpgradesTwoFamiliesToThree_PreservesExisting`, `TestMerge_AddsSessionEndSyncTrigger_CoexistsWithForeign`, `TestUninstall_RemovesSessionEndSyncTrigger_LeavesForeign`, `TestStatusCore_SessionEndMissing_Degraded`, `TestStatusCore_SessionEndPresent_OK`, `TestClaudeStatusPartialMessageNamesRemediationCommands`) + 1 flipped assertion (`TestHasSupportedClaudeLifecycleState_RequiresDesignPair` "both" case) + 4 extended existing tests (count assertions in `TestUninstall_CountIsZeroAfterInstall`, `TestMerge_Idempotent`, `TestSchema_InstallTwice_Idempotent`, `TestStatusCore_AllOK`)
- Total tests passing: all (`go test -count=1 ./...` — 12 packages `ok`, `go vet` clean)
- Layers used: Unit (all)
- Pure functions / helpers created: `isSyncTriggerEntry`, `buildSyncTriggerSessionEndEntry`, `HasLabdrianSyncTriggerHook`, `checkSessionEndHook`, test helpers `withSyncTriggerFamily` and `buildForeignSessionEndStopFixture`

### Files Changed

| File | Action | What Was Done |
|------|--------|----------------|
| `engine/settings/settings.go` | Modified | `LabdrianSyncTriggerIdentity` const, `HasLabdrianSyncTriggerHook`, `isSyncTriggerEntry`, `buildSyncTriggerSessionEndEntry`; wired into `mergeHooks` (SessionEnd only, never Stop), `removeHooks` (now also scans `SessionEnd`), `HasSupportedClaudeLifecycleState` |
| `engine/settings/settings_test.go` | Modified | 2 new coexistence tests, 1 new family-required test + helper `withSyncTriggerFamily`, 1 new upgrade test, extended count assertions in 3 existing tests, flipped the "both" case in the design-pair test |
| `engine/cmd/main.go` | Modified | `checkSessionEndHook`, `remediationNote` const, wired into `statusCore` after `checkPreToolUseHook` |
| `engine/cmd/main_test.go` | Modified | `buildSettingsWithHooks` gains a `SessionEnd` entry; `TestStatusCore_AllOK` now asserts not-degraded/no-WARN; 2 new tests |
| `engine/runtime/claude.go` | Modified | Partial status message appends the two-command remediation text |
| `engine/runtime/claude_test.go` | Modified | New `TestClaudeStatusPartialMessageNamesRemediationCommands` |
| `bin/labdrian-overlay` | Modified | Wording at install-hooks/uninstall-hooks completion banners: "three hook families ... five entries" |
| `README.md` | Modified | `uninstall-hooks` table row and `install-hooks`/`uninstall-hooks` workflow description updated to the three-family/five-entry shape |

### Verification (foreground, all observed)

- `cd engine && go test ./settings/... ./cmd/... ./runtime/...` → all three packages `ok`
- `cd engine && go vet ./... && go test -count=1 ./...` → all 12 packages `ok`, vet clean
- `cd engine && gofmt -l .` → empty (no unformatted files)
- `shellcheck -S warning bin/labdrian-overlay` → only the 2 pre-existing known SC2064 warnings at lines 1305/1466 (`trap "git checkout '${current_branch}' ..."`), unrelated to this change
- `git diff --shortstat -- engine bin README.md` (working tree at the time of measurement) → **487 insertions(+), 25 deletions(-)** across 8 files — well within the native attempt's 600-line objective budget for this slice, no exception needed
- Live-shaped fixture round-trip (scratchpad-built `engine-bin`, scratch copy of `~/.claude/settings.json` — real file never touched):
  - Before: `SessionEnd` = `[{"hooks":[{"async":true,"command":"'/home/labdrian/.local/bin/moshi-hook' claude-hook","type":"command"}]}]`
  - After `merge-settings`: moshi-hook entry preserved byte-for-byte, new `sync-trigger` entry appended: `command -v <bin> &>/dev/null && <bin> sync-trigger --event session-end --cwd "${CLAUDE_PROJECT_DIR:-.}" || true`
  - `status` (against a fake `$HOME` mirroring the scratch settings): `[OK  ] hook: SessionEnd (sync-trigger)`, exit 0
  - After `uninstall-hooks`: `SessionEnd` reduced back to only the moshi-hook entry — diffed byte-identical against the original pre-merge snapshot

### Workload / PR Boundary

- Mode: chained PR slice (stacked-to-main), PR 2 of 3, on branch `feat/longterm-mem-sync-triggers-2-session-end` stacked on slice 1's `feat/longterm-mem-sync-triggers`
- Current work unit: `2 session-end-hook`
- Boundary: starts from slice 1's `d49bc81`; ends with the `engine/settings`/`engine/cmd`/`engine/runtime` SessionEnd wiring plus docs wording, fully tested and committed
- Estimated review budget impact: 487+25=512 authored lines vs the 600-line objective budget — under budget, no exception needed
- Rollback boundary: `git revert c18fda2 260a265` (two commits: prod+test code, then docs/tasks), or `git reset --hard d49bc81` to fully unwind slice 2

### Deviations from Design

The original slice-2 commit used `${CLAUDE_PROJECT_DIR:-.}` for the `--cwd` fallback, following the existing minimalism/design entry convention in this file rather than design.md's `${CLAUDE_PROJECT_DIR:-$PWD}` prose. This was **not** functionally equivalent to `$PWD`: `.` is a relative path, and `engine/synctrigger/synctrigger.go`'s `Run` rejected any non-absolute `--cwd` as `error:usage` *before opening its log*, so whenever `CLAUDE_PROJECT_DIR` was unset the SessionEnd hook silently disabled the sync with no trace in the log. The phase validator caught this (finding F1) and it was fixed by:

- emitting `${CLAUDE_PROJECT_DIR:-$PWD}` as design.md specifies (`engine/settings/settings.go`, `buildSyncTriggerSessionEndEntry`), and
- making `Run` itself resolve a relative `--cwd` via `filepath.Abs` before the absolute-path validation, so a relative path never becomes a usage error regardless of how it was produced (`engine/synctrigger/synctrigger.go`).

**Validator findings fixed**: F1 (relative `--cwd` fallback disabling the sync silently — fixed at both the emitter and `Run`'s validation, above), F2 (the missing-binary guard used the bash-only `&>/dev/null`, which is inert under the `sh -c`/dash runtime Claude Code actually uses to invoke hooks — changed to POSIX `>/dev/null 2>&1` for the new SessionEnd entry only; the four pre-existing entries are unchanged, filed as an observation, not a change), F3 (corrected the `matcher-less like UserPromptSubmit` comment — SessionEnd does support a `matcher` field, omitting one just matches every event — and added a note on SessionEnd's short default timeout, which is why the runner detaches immediately).

### Size Exception

- **Scope**: slice 2 `session-end-hook`, cumulative across its original commits and this follow-on fix
- **Authored lines**: 487 code/doc lines + 380 test lines (git diff --shortstat across the slice's commits)
- **Reason**: the SessionEnd hook family, its lifecycle-state/status wiring, and the three validator-findings fix are one cohesive contract; the overage is test coverage and doc updates, not padding
- **Authorized by**: owner, 2026-09-09/2026-09-10

### Plan vs Realized Slice Count (fix pass)

- Planned: 3 slices (`sync-runner` → `session-end-hook` → `archive-trigger`)
- Realized so far: 2 (`sync-runner`, `session-end-hook`)

### Issues Found

None beyond the validator findings above, now fixed.

### Remaining Tasks (this change, not this slice)

- [ ] Phase 3: archive-trigger (PR 3, R-001/R-003) — `bin/labdrian-overlay` wrapper, `engine/shelltest`, `skills/inception-pipeline/SKILL.md` — **out of scope for slice 2, not touched**
- [ ] Phase 4: Full verification across `engine`, `longterm-mem`, `shellcheck`

### Status

10/10 slice-2 tasks complete, verified, and committed within budget. 16/24 total tasks across the change complete (Phase 1 + Phase 2). Ready for `sdd-verify` on slice 2, or for `sdd-apply` to resume with slice 3 (`archive-trigger`).

## Slice 3 — archive-trigger (Phase 3, R-001/R-003)

**Status**: implemented, tests green.

**Review correction**: bounded-wait rejected as not non-blocking; wrapper now returns immediately, engine detached.

The native review's targeted validator rejected the first `sync-trigger` fix (commit `0468a80`): when `timeout` was available, `bin/labdrian-overlay`'s `sync-trigger` branch still invoked the engine synchronously and could block the SessionEnd or archive caller for up to 10 seconds, and the shelltest accepted elapsed times up to 12 seconds — it verified *bounded waiting*, not the documented *non-blocking* guarantee.

Fixed by removing the wait entirely: the `sync-trigger` branch in `cmd_longterm_mem` (`bin/labdrian-overlay`) now always launches the engine detached (`setsid` when available, falling back to a plain background job) with stdin from `/dev/null` and stdout/stderr discarded, backgrounds it, and returns 0 immediately without waiting on it at all. The `timeout` path is gone — the engine already carries its own internal bounded child timeout, so the wrapper adds no wait on top of it. A missing/non-executable engine binary still degrades to a warning and an immediate `return 0`.

`engine/shelltest/overlay_longterm_mem_test.sh`'s `case_sync_trigger_does_not_block_on_a_wedged_engine` was rewritten RED-first: the fake wedged engine (30s sleep) now also records its own PID, the wrapper's elapsed-time bound was tightened from 12s to 2s, and the test additionally asserts the wedged engine's PID is still alive (`kill -0`) right after the wrapper returns — proving the wrapper never awaited it as its own child, not merely that it returned within some bound. `case_sync_trigger_forwards_event_and_cwd` was adjusted to poll (up to 3s) for the fake engine's record file instead of expecting it synchronously, since the launch is now asynchronous; the forwarded-args assertions themselves are unchanged.

### Verification (foreground, all observed)

- `bash engine/shelltest/overlay_longterm_mem_test.sh` → all cases pass, including the rewritten wedged-engine and forwarding cases
- `shellcheck -S warning bin/labdrian-overlay` → only the 2 pre-existing known SC2064 warnings (`trap "git checkout '${current_branch}' ..."`), unrelated to this change
- `git diff --shortstat 69a5d37..HEAD` → measured at commit time, see commit message

### Plan vs Realized Slice Count

- Planned: 3 slices (`sync-runner` → `session-end-hook` → `archive-trigger`)
- Realized: 3 (`sync-runner`, `session-end-hook`, `archive-trigger`)
