# Proposal: Codex Runtime Lifecycle

## Intent

Close the three-runtime lifecycle loop by replacing Codex advisory-only behavior with native, honest lifecycle semantics for `engine runtime --target codex`, while preserving Claude and OpenCode behavior.

## Proposal Question Round

Proceeding with confirmed assumptions: Codex root resolves from `$CODEX_HOME`, else `~/.codex`; only Labdrian-owned artifacts may be mutated; `supported` is reported only when installed/configured state is provable; activation/reload uncertainty must be surfaced rather than hidden.

## Scope

### In Scope
- Implement Codex `status`, `install`, `update`, and `uninstall` semantics where Codex-native state can be proven.
- Define conservative Codex root resolution and Labdrian-owned lifecycle artifacts.
- Update `--target all` aggregation so Codex participates as supported/partial/unsupported without masking Claude/OpenCode failures.
- Add Go tests for root resolution, mutation boundaries, status honesty, lifecycle actions, and target aggregation.

### Out of Scope
- TUI dashboard work.
- Blindly transplanting old PR #13 code.
- Reworking Claude/OpenCode lifecycle beyond safe aggregation updates.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `runtime-lifecycle`: add Codex lifecycle support and replace transitional Codex advisory aggregation with honest runtime states.

## Approach

Build on `engine/runtime` adapter foundations. Replace `engine/runtime/codex.go` placeholders with a conservative Codex adapter that resolves roots, writes/removes only Labdrian-owned artifacts, and reports partial/unsupported when Codex installation, configuration, or reload state cannot be proven. Update CLI aggregation only as needed for `all` target semantics.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `engine/runtime/codex.go` | Modified | Codex lifecycle implementation replaces advisory placeholder. |
| `engine/runtime/runtime.go` | Modified | Shared target/status behavior if Codex states require aggregation support. |
| `engine/cmd/main.go` | Modified | `--target all` handling transitions from Codex advisory exception to real state aggregation. |
| `engine/runtime/*_test.go` | Modified | Add/adjust lifecycle, ownership, root, and status tests. |
| `engine/cmd/runtime_test.go` | Modified | Add/adjust all-target success and failure masking tests. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Codex artifact shape is wrong | Med | Keep artifacts minimal, Labdrian-owned, and native; design must document shape. |
| False-green status | Med | Status reports supported only with provable installed/configured state. |
| Aggregation regression | Med | Preserve Claude/OpenCode failure propagation with explicit tests. |

## Rollback Plan

Revert Codex adapter and aggregation changes; Codex returns to `CapabilityUnsupported` advisory behavior while Claude/OpenCode remain unchanged.

## Dependencies

- Existing OpenCode and Claude runtime lifecycle foundations.
- Go table-driven filesystem tests using `t.TempDir()`.

## Success Criteria

- [x] `engine runtime --target codex` reports honest status and performs supported lifecycle actions.
- [x] Codex install/update/uninstall mutate only Labdrian-owned artifacts under the resolved Codex root.
- [x] `--target all` includes Codex without masking Claude/OpenCode failures.
- [x] `cd engine && go test ./...` remains green.
