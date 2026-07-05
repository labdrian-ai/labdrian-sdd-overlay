# Apply Progress: Codex Runtime Lifecycle

## Summary
- Implemented Codex runtime lifecycle as a real Labdrian-owned target with status/install/update/uninstall behavior and config-root precedence semantics.
- Updated CLI/runtime wiring so `--target codex` is no longer advisory-only and participates in standard `all`-target run semantics.
- Synchronized spec/design/task context with current code and applied a targeted status-contract repair so Codex reports managed manifest presence as `partial` when activation/reload proof is unavailable.

## Completed Tasks
- **Phase 1 — Foundation**
  - [x] 1.1 Add Codex-owned manifest model in `engine/runtime/codex.go`: `codexManifestFile`, `type codexManifest`, `managed_by`/`installed_version`/`config_root` fields, plus helper constructors.
  - [x] 1.2 Add `DefaultCodexConfigRoot()` in `engine/runtime/codex.go` with `$CODEX_HOME` precedence and `~/.codex` fallback.
  - [x] 1.3 Add root-resolution unit tests in `engine/runtime/codex_test.go` (precedence, invalid empty roots, non-absolute root handling).

- **Phase 2 — Core Implementation**
  - [x] 2.1 Implement filesystem-backed `CodexAdapter` in `engine/runtime/codex.go` and remove unsupported-only placeholder behavior.
  - [x] 2.2 Implement `Install`/`Update` writing only Labdrian-owned manifest `labdrian-runtime-lifecycle.json` with deterministic payload.
  - [x] 2.3 Implement `Uninstall` deleting only managed manifest and return `restart_required` when removal occurs.
- [x] 2.4 Implement explicit `Status` with `partial` for managed state without activation/reload proof, including invalid/missing/incomplete managed state, and uncertainty reasons where activation/reload status cannot be guaranteed.

- **Phase 3 — Integration / Wiring**
  - [x] 3.1 Update `engine/runtime/runtime.go` to use `NewCodexAdapter(DefaultCodexConfigRoot())` with explicit default-root resolution.
  - [x] 3.2 Preserve injectable-root behavior consistent with Claude/OpenCode while keeping test constructors stable.
  - [x] 3.3 Wire `--target codex` to use real adapter in `engine/cmd/main.go`.
  - [x] 3.4 Ensure `--target all` no longer masks codex failures behind advisory-only behavior.

- **Phase 4 — Testing / Verification**
  - [x] 4.1 Update `engine/runtime/runtime_test.go` to validate real Codex foundation behavior.
  - [x] 4.2 Expand `engine/runtime/codex_test.go` with install/update/uninstall behavior, mutation-boundary and status scenarios.
  - [x] 4.3 Extend `engine/cmd/runtime_test.go` for root precedence, override, mutation behavior, and `--target all` semantics.
  - [x] 4.4 Add explicit non-regression checks for non-Codex flows in runtime command tests.
  - [x] 4.5 Re-run focused validation commands.

- **Phase 5 — Closure Checklist**
  - [x] 5.1 Re-sync and confirm docs/contracts alignment.
  - [x] 5.2 Confirm no regressions to adjacent runtime/advisory messaging.

## Files Changed
- `engine/runtime/codex.go`
- `engine/runtime/runtime.go`
- `engine/runtime/runtime_test.go`
- `engine/runtime/codex_test.go`
- `engine/cmd/main.go`
- `engine/cmd/runtime_test.go`
- `openspec/specs/runtime-lifecycle/spec.md`
- `openspec/project/roadmap.md`

## Evidence
- `engine/runtime/codex_test.go`
  - Root resolution unit tests and Codex lifecycle behavior tests (install/update/uninstall/status, boundaries, mutation safety).
- `engine/runtime/runtime_test.go`
  - Foundation and status contract test updates for Codex adapter.
- `engine/cmd/runtime_test.go`
  - Root precedence / override, root validation, mutation outcomes, and `--target all` inclusion assertions.
- `openspec/changes/codex-runtime-lifecycle/design.md` and `specs/runtime-lifecycle/spec.md`
  - Rechecked for consistency during apply; no stale advisory-only Codex requirements remain.

## Verification
- `cd engine && go test ./runtime` ✅ passed
- `cd engine && go test ./cmd` ✅ passed

## TDD Evidence (strict mode)

| Task | Test File | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|
| 1.3 | `engine/runtime/codex_test.go` | ✅ | ✅ | ✅ 3 cases | ➖ None needed |
| 2.2 | `engine/runtime/codex_test.go` | ✅ | ✅ | ✅ 2 cases | ➖ None needed |
| 2.3 | `engine/runtime/codex_test.go` | ✅ | ✅ | ✅ 2 cases | ➖ None needed |
| 2.4 | `engine/runtime/codex_test.go` | ✅ | ✅ | ✅ 2 cases | ➖ None needed |
| 3.2 | `engine/runtime/runtime_test.go` | ✅ | ✅ | ➖ Single | ➖ None needed |
| 3.3 | `engine/cmd/runtime_test.go` | ✅ | ✅ | ✅ 3 cases | ➖ None needed |
| 4.1–4.4 | `engine/runtime/runtime_test.go` + `engine/runtime/codex_test.go` + `engine/cmd/runtime_test.go` | ✅ | ✅ | ✅ Multiple cross-surface paths | ➖ None needed |

Legend:
- **RED**: behavior-first tests added/extended before implementation for each task surface.
- **GREEN**: suite pass after implementation.
- **TRIANGULATE**: additional tests added to exercise alternate branches/edge conditions.
- **REFACTOR**: no substantial refactor required after implementation stabilization.

## Deviations
- None identified. Implementation remains aligned with design constraints in `design.md` and scenario expectations in `spec.md`.

## Fresh apply review remediation
- Revalidated `engine/cmd/main.go` mixed-target status aggregation after final `runRuntimeCore` change: Codex `partial` during `status --target all` now exits 0 when Claude/OpenCode are healthy.
- Added regression tests in `engine/cmd/runtime_test.go` for:
  - `runtime status --target all` with healthy Claude/OpenCode and codex `partial` (expected exit 0)
  - `runtime status --target all` where Claude/OpenCode fail (expected exit 1)
- Updated `openspec/changes/codex-runtime-lifecycle/specs/runtime-lifecycle/spec.md` to require explicit `partial` with uncertainty text when activation/reload proof is unavailable, avoiding any ambiguity around a misleading `supported` state.
- Re-ran focused runtime validation commands after the final patch:
  - `cd engine && go test ./runtime` (passed)
  - `cd engine && go test ./cmd` (passed)

## Remaining Work
- None. All tasks in `openspec/changes/codex-runtime-lifecycle/tasks.md` are marked complete.
