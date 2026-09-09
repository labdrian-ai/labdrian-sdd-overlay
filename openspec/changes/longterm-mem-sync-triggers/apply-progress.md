# Apply Progress: longterm-mem-sync-triggers

## Slice 1 — sync-runner (Phase 1, R-003)

**Status**: implemented, tests green, committed under a granted `size:exception`.

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

- [ ] Phase 2: session-end-hook (PR 2, R-002/R-003) — `engine/settings`, `engine/runtime/claude.go`, `bin/labdrian-overlay`/`README.md` wording — **out of scope for slice 1, not touched**
- [ ] Phase 3: archive-trigger (PR 3, R-001/R-003) — `bin/labdrian-overlay` wrapper, `engine/shelltest`, `skills/inception-pipeline/SKILL.md` — **out of scope for slice 1, not touched**
- [ ] Phase 4: Full verification across `engine`, `longterm-mem`, `shellcheck`

### Status

6/6 slice-1 tasks complete, verified, and committed under a granted `size:exception`. Ready for `sdd-verify` on slice 1, or for `sdd-apply` to resume with slice 2 (`session-end-hook`).
