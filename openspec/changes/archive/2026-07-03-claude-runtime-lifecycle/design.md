# Design: Claude Runtime Lifecycle

## Technical Approach

Replace the placeholder `ClaudeAdapter` with a narrow adapter that drives the existing Claude Code settings merger. The runtime CLI will resolve a Claude root separately from OpenCode: default to the real Claude Code root (`$HOME/.claude`) and use `--config-root` only as an explicit sandbox/non-default override. OpenCode remains on its current adapter path; Codex remains unsupported but is treated as transitional advisory for `--target all` when Claude and OpenCode succeed.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Claude implementation boundary | Implement directly in `engine/runtime/claude.go` using `engine/settings.Merger`. | Shared runtime lifecycle refactor. | Keeps the slice reviewable and avoids changing proven OpenCode behavior. |
| Claude root resolution | Add `DefaultClaudeConfigRoot() -> $HOME/.claude`; `--config-root` overrides only when provided. | Reuse OpenCode XDG defaults or cwd-relative roots. | Claude Code settings live under `.claude`; cwd-relative mutation is unsafe for real users and tests. |
| Settings mutation | Reuse/extend safe merge, uninstall, atomic write, and `.bak` backup semantics. | Hand-edit JSON in the adapter. | Existing merger already preserves unrelated settings and tests the Claude hook shape. |
| Status truthfulness | `supported` only when settings are readable and both Labdrian-owned hook families are present for the adapter identity. Missing/invalid settings return `unsupported` or `partial`; no fake “healthy” state. | Treat install success as supported immediately. | The spec requires provable installed/healthy state, not optimistic success. |
| `--target all` transition | All-target execution succeeds when Claude and OpenCode are non-failing and only Codex is unsupported; emit Codex advisory. | Fail on any unsupported target. | Preserves short transition while Codex is deliberately out of scope. |

## Data Flow

```text
runtime CLI ──parse action/target/root──> runtimeAdapterForTarget
     │                                      │
     │ --target claude                      ├─> ClaudeAdapter(root)
     │ --target opencode                    ├─> OpenCodeAdapter(root/default)
     └ --target all                         └─> CodexAdapter(unsupported advisory)

ClaudeAdapter install/update/uninstall ──> settings.Merger ──> <root>/settings.json + .bak
ClaudeAdapter status ──> settings read-only inspection ──> LifecycleResult
```

## File Changes

| File | Action | Description |
|---|---|---|
| `engine/runtime/claude.go` | Modify | Replace placeholder with install/update/status/uninstall backed by Claude settings. |
| `engine/runtime/runtime.go` | Modify | Let `NewFoundationAdapter` construct Claude with default root; keep Codex unchanged. |
| `engine/cmd/main.go` | Modify | Resolve Claude default root, pass explicit `--config-root` to Claude, and implement all-target Codex advisory semantics. |
| `engine/settings/settings.go` | Modify | Add minimal read-only helper(s) for owned-entry status if needed; preserve existing merge/uninstall behavior. |
| `engine/runtime/runtime_test.go` | Modify | Update Claude adapter contract from placeholder unsupported to real lifecycle behavior. |
| `engine/runtime/claude_test.go` | Create | Fixture-based Claude lifecycle tests with `t.TempDir()`. |
| `engine/cmd/runtime_test.go` | Modify | CLI tests for Claude root selection and all-target transition. |
| `engine/settings/settings_test.go` | Modify | Cover any new read-only status helper and owned-entry detection. |

## Interfaces / Contracts

```go
func DefaultClaudeConfigRoot() string // $HOME/.claude or "" when unresolved
func NewClaudeAdapter(root string) ClaudeAdapter
```

Claude settings path is `filepath.Join(root, "settings.json")`; hook command identity is `filepath.Join(root, "bin", "gentle-ai-overlay")`. Install and update call `Merger.Install`; uninstall calls `Merger.Uninstall`; status inspects only Labdrian-owned entries for that identity. Existing legacy `merge-settings`, `uninstall-hooks`, and OpenCode APIs remain compatible.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | Claude root validation, install/update/uninstall/status, backup behavior, owned-entry boundary. | Table-driven Go tests with `t.TempDir()` fixture roots; never touch real `$HOME`. |
| Integration | Runtime CLI dispatch, default vs explicit root, all-target aggregation. | `runRuntimeCore` tests with temp HOME/config roots and captured stdout/exit codes. |
| Regression | OpenCode and legacy settings behavior. | Existing `engine/runtime` and `engine/settings` suites remain green. |

## Migration / Rollout

No data migration required. Existing users can run `runtime install --target claude`; uninstall removes only Labdrian-owned entries. Codex implementation is explicitly deferred to the next SDD.

## Open Questions

None.
