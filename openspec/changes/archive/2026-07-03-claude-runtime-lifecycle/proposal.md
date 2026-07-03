# Proposal: Claude Runtime Lifecycle

## Intent

Make `engine runtime --target claude` a real lifecycle command for Claude Code instead of a placeholder, using the existing hooks/settings infrastructure safely while preserving legacy commands and current OpenCode behavior.

## Scope

### In Scope
- Implement Claude `status`, `install`, `update`, and `uninstall` lifecycle behavior.
- Default Claude operations to the real Claude Code settings location; keep `--config-root` for tests, sandboxes, and explicit non-default roots.
- Mutate only Labdrian-owned hooks/settings entries with existing safe write/backup semantics.
- Report honest Claude status with `supported` as the healthy target state.
- Keep `--target all` successful when Claude and OpenCode are supported while Codex remains unsupported, with a short-lived warning/advisory.

### Out of Scope
- Codex runtime lifecycle implementation.
- TUI dashboard/runtime screen.
- Blind transplant of legacy PR #13 code.
- Rewriting, normalizing, or reordering unrelated user hooks/settings entries.

## Capabilities

### New Capabilities
- `runtime-lifecycle`: lifecycle behavior for CLI runtime targets, including install/update/uninstall/status, target aggregation, and safe user configuration mutation.

### Modified Capabilities
- None.

## Approach

Replace the Claude placeholder adapter in `engine/runtime/claude.go` with a focused implementation that calls the current Claude hooks/settings system. Reuse existing settings merge/uninstall helpers and backup-safe writes. Update runtime command expectations so Claude is supported, Codex remains no-scope/unsupported, and `--target all` treats temporary Codex unsupported status as a warning when supported targets succeed.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `engine/runtime/claude.go` | Modified | Real Claude adapter lifecycle. |
| `engine/runtime/runtime.go` | Modified | Target aggregation/status semantics if needed. |
| `engine/settings/settings.go` | Modified | Reuse or lightly extend safe Claude settings mutation. |
| `engine/runtime/runtime_test.go` | Modified | Claude supported contract and target-all behavior. |
| `engine/cmd/runtime_test.go` | Modified | CLI expectations for Claude and partial target-all warning. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| User settings corruption | Med | Use existing backup/safe merge utilities; mutate only Labdrian-owned entries. |
| False healthy status | Med | Report `supported` only when install state can be proven. |
| Codex warning becomes permanent | Low | Keep advisory explicit and bounded to this transition. |

## Rollback Plan

Revert Claude adapter and runtime CLI expectation changes. Users can run `engine runtime uninstall --target claude` first, or restore the generated settings backup if manual rollback is required.

## Dependencies

- Existing Claude hooks/settings infrastructure and backup-safe writes.
- Existing `engine runtime` command dispatch and OpenCode lifecycle behavior.

## Success Criteria

- [ ] Claude runtime status reports `supported` only for a proven installed/healthy state.
- [ ] Install/update/uninstall affect only Labdrian-owned Claude settings entries.
- [ ] `--config-root` keeps tests isolated from the real home/settings location.
- [ ] OpenCode behavior remains green; Codex stays unsupported with short-lived warning under `--target all`.
