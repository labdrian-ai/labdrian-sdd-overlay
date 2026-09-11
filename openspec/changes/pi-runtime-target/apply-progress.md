# Apply Progress: pi-runtime-target

## Batch 1 — Slice 1: pi-target-plumbing (R-001, R-008)

**Mode**: Strict TDD
**Status**: Implementation and verification complete, `size:exception`
granted, and committed as work units (see "Granted exception" and
"Commits" below).

### Granted exception

- **Scope**: `pi-target-plumbing` (slice 1)
- **Lines**: 407 authored (386 insertions + 21 deletions), 7 over the
  400-line default guard
- **Reason**: cohesive target plumbing, including the `cmd_longterm_mem`
  regression fix (see Deviations below) — not separable without splitting
  a single behavioral unit
- **Authorized by**: owner, 2026-09-10
- **Recorded in**: `openspec/changes/pi-runtime-target/entry.json` by the
  orchestrator (this apply batch did not edit `entry.json`)

### Plan vs Realized Slice Count

slices planned=5 realized=1 (this batch: `pi-target-plumbing` committed).

### Completed Tasks

- [x] 1.1 RED tests written first (`TestExpandTarget_Pi` in
  `engine/runtime/runtime_test.go`; `TestResolveTargets_Pi` and
  `TestIsCopyTarget_ClaudeTrue_PiFalse` in the new
  `engine/shelltest/overlay_pi_target_test.go` — see Deviations)
- [x] 1.2 GREEN: `TargetPi` added to the `Target` enum, `ExpandTarget`
  (`all` now expands to claude/opencode/codex/pi), `ParseTarget`,
  `NewFoundationAdapter`; `engine/runtime/pi.go` created with `PiAdapter`
  (`Target()` wired to `TargetPi`; every other lifecycle method returns an
  honest `CapabilityUnsupported` stub, mirroring `foundationAdapter`'s
  shape but as its own named type so later slices have a home to extend)
- [x] 1.3 `TARGET_KINDS`, `is_valid_target`, `is_copy_target`,
  `package_target_stub_message` added to `bin/labdrian-overlay`.
  `resolve_targets` updated (`all` → `claude opencode codex pi`; unknown
  names still `die`). All 8 `TARGET_PATHS`-keyed call sites updated:
  - `cmd_apply`, `cmd_status`, `cmd_sync_check` loops: dispatch `pi` to
    `package_target_stub_message` and `continue`, never reaching the
    `TARGET_PATHS[$t]` lookup or `mkdir`
  - `cmd_capture`, `cmd_restore`, `cmd_repair_sdd_registry`,
    `cmd_skill_registry`: `pi` is recognized as a valid target name
    (`is_valid_target`) but rejected with a clear package-target message
    (`is_copy_target` fails) — these commands are inherently per-file/
    copy-target concepts (backups, skill-registry.md rows) with no
    package-target equivalent yet, so an explicit die is the honest
    behavior, not silent inclusion
  - `engine/cmd/main.go` usage/help text and `runRuntimeCore`'s
    `--target all` aggregation updated: Pi's honest `CapabilityUnsupported`
    is exempted from failing the aggregate **only** when riding along
    inside `--target all` (mirrors the pre-existing Codex partial
    exemption); an explicit `--target pi` still fails loudly
  - **Regression found and fixed** (not in the original 8-site list):
    `cmd_longterm_mem` also calls `resolve_targets`, so its `--target all`
    silently picked up `pi` and called `longterm-mem register --target
    pi`, which longterm-mem's own binary rejects (exit 2, version skew) —
    this is `engine/installer` test territory and is explicitly out of
    scope for this slice. Fixed by excluding `pi` from the targets
    `cmd_longterm_mem` acts on for `--target all`, and rejecting an
    explicit `--target pi` there with a clear message. No file under
    `longterm-mem/` was touched.
- [x] 1.4 Non-regression verified: `shellcheck -S warning bin/labdrian-overlay`
  reports only the 2 pre-existing SC2064 warnings (lines ~1352/1513, shifted
  by the additions, same known issue named in the prompt). `status`/
  `sync-check --target claude|opencode|codex` output diffed byte-for-byte
  against the pre-change script (via `git show HEAD:bin/labdrian-overlay`)
  under matched `HOME`/`OVERLAY_DIR` — identical in every case.

### Files Changed

| File | Action | What Was Done |
|---|---|---|
| `engine/runtime/runtime.go` | Modified | `TargetPi` const; `ParseTarget`, `ExpandTarget`, `NewFoundationAdapter` cases |
| `engine/runtime/pi.go` | Created | `PiAdapter` skeleton — `Target()` wired, all other methods an honest `CapabilityUnsupported` stub |
| `engine/runtime/runtime_test.go` | Modified | `pi` case added to the `ParseTarget` table; new `TestExpandTarget_Pi` |
| `engine/cmd/main.go` | Modified | Usage text; `runRuntimeCore`'s `--target all` aggregation exemption extended to Pi's stub state |
| `engine/cmd/runtime_test.go` | Modified | `pi` assertion added to the existing `TestRunRuntimeCore_AllTargetsRunsCodexLifecycleTogether` (untouched otherwise, still green); two new tests for explicit `--target pi` and no-masking under `--target all` |
| `engine/shelltest/overlay_pi_target_test.go` | Created | `TestResolveTargets_Pi`, `TestIsCopyTarget_ClaudeTrue_PiFalse`, `TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir` |
| `bin/labdrian-overlay` | Modified | `TARGET_KINDS`/helpers; `resolve_targets`; 8 `TARGET_PATHS`-keyed call sites; `cmd_longterm_mem` pi exclusion (regression fix); usage text for apply/status/sync-check |

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.2 (Go enum/adapter) | `engine/runtime/runtime_test.go`, `engine/runtime/pi.go` | Unit | ✅ full `runtime` suite green before change | ✅ Written — confirmed by reverting `runtime.go`/`pi.go` to HEAD and re-running `go test ./runtime/...`: build failed with `undefined: engineRuntime.TargetPi`/`PiAdapter` | ✅ Passed after restoring the GREEN implementation | ✅ 4 targets (claude/opencode/codex/pi) covered in the `ParseTarget` table + adapter-type assertion | ➖ None needed — matches existing `foundationAdapter` shape |
| 1.3 (bash target plumbing) | `engine/shelltest/overlay_pi_target_test.go` | Integration (sources the real script) | ✅ `overlay_longterm_mem_test.sh`/other shelltest suites green before and after | ✅ Written — tests reference `is_copy_target`/pi-inclusive `resolve_targets` behavior that did not exist beforehand | ✅ Passed — `go test ./shelltest/...` green | ✅ 4 targets in `TestIsCopyTarget_ClaudeTrue_PiFalse`; explicit vs `all` in `TestResolveTargets_Pi`; status+sync-check × pi+all in the no-crash test | ➖ None needed |
| 1.3 (aggregate no-masking, Go) | `engine/cmd/runtime_test.go` | Unit/integration (`runRuntimeCore`) | ✅ Full `cmd` suite green before change (7 pre-existing aggregate/target tests) | ✅ Written — new tests assert behavior the pre-change exemption logic didn't have | ✅ Passed | ✅ explicit `--target pi` (fails) vs `--target all` (Pi exempted, claude/opencode/codex failure still surfaces) | ➖ None needed |

### Test Summary

- **Total tests written/extended**: 8 (1 Go unit in `runtime`, 1 table-row
  extension, 3 Go unit/integration in `cmd`, 3 shelltest in `shelltest`)
- **Total tests passing**: all of the above, plus the full pre-existing
  suite (`go test -count=1 ./...` across the `engine` module — all packages
  green, including `engine/installer` after the longterm-mem regression fix)
- **Layers used**: Unit (Go runtime/cmd), Integration (shelltest sourcing
  the real bash script, plus one real subprocess invocation of the built
  binary)
- **Approval tests**: None — no refactoring of pre-existing behavior, only
  additive plumbing plus the one regression fix (a genuine bug caused by
  this slice, fixed with a matching test-equivalent manual verification —
  see Deviations)

### Deviations from Design

1. **Test naming/location split (task 1.1)**: `tasks.md` listed
   `TestExpandTarget_Pi`, `TestResolveTargets_Pi`, and
   `TestIsCopyTarget_ClaudeTrue_PiFalse` all "in `engine/runtime/runtime_test.go`".
   `resolve_targets` and `is_copy_target` are bash functions added to
   `bin/labdrian-overlay` in this same slice — there is no Go equivalent to
   test them against. They are implemented instead in
   `engine/shelltest/overlay_pi_target_test.go`, following the exact
   `source "$overlay"; <call function>` pattern already established by
   `overlay_binary_path_test.go` in the same package. Only
   `TestExpandTarget_Pi` (a genuine Go-package concern) landed in
   `runtime_test.go` as literally named.
2. **Regression fix outside the original file list**: `cmd_longterm_mem`
   in `bin/labdrian-overlay` was not in the design's 8-site list, but it
   calls the same `resolve_targets` helper and broke
   (`engine/installer`'s `TestInstall_BinaryPathStableAcrossInspections`
   and two sibling tests) once `pi` joined `all`'s expansion. Fixed with a
   4-line scope exclusion inside `cmd_longterm_mem`, explicitly not
   touching anything under `longterm-mem/` — reported here rather than
   silently folded into the "8 sites" count, since design did not
   anticipate it.
3. **`--target all` aggregate exemption (cmd/main.go)**: not explicitly
   named in design's Slice 1 file list (`engine/runtime/runtime.go` was
   listed as the only `runtime`-side file; `cmd/main.go` was listed
   separately as "CLI wiring"). The exemption logic was necessary to keep
   `TestRunRuntimeCore_AllTargetsRunsCodexLifecycleTogether` and
   `TestRunRuntimeCore_AllTargetsStatusAllowsCodexPartialWithoutFailing`
   green without modifying their asserted exit codes — Pi's honest
   `CapabilityUnsupported` stub would otherwise fail every `--target all`
   run until slice 5 (`pi-lifecycle`) gives Pi real status proof. This
   mirrors the pre-existing Codex-partial exemption in the same switch and
   is intended to be revisited once Pi has real lifecycle logic.

### Issues Found

None beyond the `cmd_longterm_mem` regression above, which is fixed.

### Budget

`git diff --shortstat fc74d56 -- engine bin README.md` → **386 insertions(+),
21 deletions(-) = 407 changed lines**, 7 lines (1.75%) over the 400-line
guard. Per the minimalism contract this was NOT reduced by trimming
comments, tests, or restyling code — every comment documents a design
rationale (the `TARGET_KINDS` seam, the aggregate exemption, the
`cmd_longterm_mem` scope boundary) and every test is load-bearing (see TDD
Cycle Evidence above; none are redundant across the Go/shelltest layers).
**The owner granted a `size:exception`** for this scope on 2026-09-10 (see
"Granted exception" above), so this work unit is committed as-is.

### Commits

- `9faf011` — `feat(runtime): accept pi as a package-model target`
  (engine/runtime, engine/cmd, engine/shelltest, bin/labdrian-overlay)
- `eb8c14c` — `docs(sdd): record pi-runtime-target slice 1 apply progress`
  (tasks.md, apply-progress.md)

### Slice tracking

slices planned=5 realized=1 (entry contract `review_slices` read directly
from `openspec/changes/pi-runtime-target/entry.json`; not confirmed as
validated by the orchestrator per Step 2c, so treated as informational
only. R=1: `pi-target-plumbing` committed this batch — the first realized
slice of the 5 planned.)

### Remaining Tasks

- [ ] Phase 2: pi-package-build (R-002, R-003, R-010)
- [ ] Phase 3: pi-contract-gate (R-004, R-007)
- [ ] Phase 4: pi-longterm-mem-mcp (R-005)
- [ ] Phase 5: pi-lifecycle (R-006, R-009)
- [ ] Phase 6: Manual-only verification (live Pi)

### Workload / PR Boundary

- Mode: chained PR slice (stacked-to-main, auto-chain)
- Current work unit: `pi-target-plumbing` (PR1) — committed, not pushed
- Boundary: starts at `fc74d56` (planning only); ends at the two commits
  above
- Estimated review budget impact: 407 changed lines (7 over the 400
  default; `size:exception` granted by the owner)

### Status

4/4 Phase 1 tasks functionally complete, verified, and committed as two
work units. `size:exception` granted and recorded. Ready for the next
slice (`pi-package-build`) or `sdd-verify` at the orchestrator's
discretion.

### Post-review fix (native review lineage `review-ea4451a282f311c9`)

A native review of Slice 1 escalated one CRITICAL finding
(R4-silent-package-skip) and several WARNING/MEDIUM findings against the
commits above. This scoped follow-up, still within Slice 1
(`pi-target-plumbing`), fixes them:

1. **R4-silent-package-skip (CRITICAL)**: `apply|status|sync-check
   --target pi` now `die`s (exit 1) with a clear message instead of
   silently printing the stub and returning 0. Under `--target all`, the
   pi stub now forces the same commands' own aggregate exit non-zero
   (`pi_stub_included` tracked per command) — unattended automation can no
   longer read a run that silently skipped pi as a clean success.
   `engine/cmd/main.go`'s `runRuntimeCore` aggregate exemption for Pi's
   honest `CapabilityUnsupported` is narrowed to the `status` action only
   (mirroring the pre-existing Codex-partial exemption, which was already
   status-only), so `engine runtime install|update|uninstall --target all`
   also now fails while pi is unsupported instead of exiting 0.
2. **R4-sync-check-success-without-check (WARNING)**: `sync-check` for a
   non-copy target now emits `SYNC_CHECK:$t: unsupported -- ...` (was
   `partial`) plus an explicit `VERDICT:$t:UNSUPPORTED` line, so a health
   check parsing `SYNC_CHECK:`/`VERDICT:` output detects pi's
   incompleteness the same way it detects every other target's state.
3. **R2-misleading-stub-schedule (WARNING)**: `engine/runtime/pi.go`'s
   `PiAdapter.stub()` used one shared message ("scheduled for the
   pi-package-build PR slice") for every action, including Uninstall and
   Rollback. It now names the slice that actually owns each action:
   Apply/Install/Status/SyncCheck/Update → pi-package-build (package
   delivery); Uninstall/Rollback → pi-lifecycle (there is no
   install/undo logic to schedule into pi-package-build). The bash-side
   `package_target_stub_message` (apply/status/sync-check only) was
   already accurate and is unchanged.
4. **R3-shell-stub-exit-unproved / R3-copy-target-preservation-unproved /
   R3-longterm-all-regression-unproved (WARNING)**:
   `engine/shelltest/overlay_pi_target_test.go` now asserts exit codes
   explicitly for `status --target pi` (1), `sync-check --target pi`
   (non-zero), and `--target all` for both commands (non-zero, pi
   undeployed); a new byte-for-byte comparison test proves the
   claude/opencode/codex sections of `status`/`sync-check --target all`
   output are unchanged from a solo per-target run; and two new tests
   cover `cmd_longterm_mem`'s pre-existing pi exclusion — an explicit
   `longterm-mem ... --target pi` still dies, and `longterm-mem uninstall
   --target all` never acts on a `pi` entry even if one is present in the
   install-tracking file (regression guard).
5. **Validator MEDIUM**: `overlay version`/`overlay update` listing
   `pi: never deployed` (via `resolve_targets all`) is INTENTIONAL — the
   same honest "never deployed" reporting every other never-installed
   target gets, not a special case. Pinned by
   `TestOverlayVersionAndUpdate_ListPiNeverDeployed`.
6. Fixed a stale commit hash in this file's own Commits section
   (`386e2d8` → `eb8c14c` — the same-message commit that actually landed
   on this branch).

**Regression fallout fixed in the same pass** (not part of the review
findings, but caused by fix #1's exit-code change): 8 call sites across
`engine/installer/route_test.go` and `engine/installer/sync_check_test.go`
ran `apply --target all` (some followed by `status --target all`) purely
as setup and asserted a 0 exit. `sync_check_test.go`'s 8 occurrences were
switched to `apply --target claude` (all of them only exercise claude
afterward). `route_test.go`'s 8 occurrences need claude+opencode+codex
deployed together in one call (agent-file/multi-target assertions, or the
`--target all` deploy-loop shape itself), so two new tolerant helpers
(`runApplyAllTolerantOfPi`, `runStatusAllTolerantOfPi`) were added there:
they fail the test on any error EXCEPT one whose output shows pi's own
stub line, keeping every other regression surfaced normally.

**Verification** (foreground, all green): `cd engine && gofmt -l .`
(empty); `go vet ./... && go test -count=1 ./...` (every package,
including `installer` and `shelltest`); `shellcheck -S warning
bin/labdrian-overlay` (only the 2 pre-existing SC2064 warnings); a
built-engine smoke test confirmed `status --target pi` exits 1,
`sync-check --target pi` exits non-zero with a clear message,
`sync-check --target all` emits `SYNC_CHECK:pi: unsupported` and
`VERDICT:pi:UNSUPPORTED`, and `status --target all`'s claude/opencode/codex
section is byte-identical to `git show 2279248:bin/labdrian-overlay`'s
output for the same command (diffed under matched HOME) except for the
trailing pi section.

Diff: `git diff --shortstat` → 9 files changed, ~630 insertions(+), ~74
deletions(-) (bin/labdrian-overlay, engine/cmd/main.go,
engine/cmd/runtime_test.go, engine/installer/route_test.go,
engine/installer/sync_check_test.go, engine/runtime/pi.go,
engine/runtime/runtime_test.go, engine/shelltest/overlay_pi_target_test.go,
and this file).
