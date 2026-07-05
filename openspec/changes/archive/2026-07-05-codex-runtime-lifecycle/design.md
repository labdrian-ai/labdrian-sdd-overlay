# Design: Codex Runtime Lifecycle

## Technical Approach

Replace the placeholder `CodexAdapter` with a conservative filesystem-backed adapter that manages only Labdrian-owned Codex lifecycle artifacts. The default Codex root resolves from `$CODEX_HOME` when set, otherwise from `~/.codex`; `--config-root` remains an explicit test/operator override passed into `NewCodexAdapter(root)` like Claude/OpenCode adapters. Codex status is conservative: the status reflects whether managed manifest/config state is verifiably correct and includes explicit activation/reload uncertainty when runtime proof is missing.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Codex artifact model | Store a Labdrian-owned JSON manifest at `<codex-root>/labdrian-runtime-lifecycle.json`; preserve the existing `<codex-root>/skills` deployment model used by installer tests, but lifecycle install/update owns only the manifest. | Mutate Codex global config, copy OpenCode plugin files, or infer an unknown Codex schema. | A single owned manifest is native enough for Codex root semantics, reviewable, reversible, and avoids corrupting unrelated user state. |
| Root resolution | Add `DefaultCodexConfigRoot()` using `CODEX_HOME`, then `$HOME/.codex`/`os.UserHomeDir()`. | Reuse OpenCode XDG rules or require `--config-root`. | Codex already has a home-like convention in installer coverage and the spec requires `$CODEX_HOME` precedence. |
| Status honesty | `partial` when Codex managed state is absent, unreadable, stale, malformed, missing owned fields, or present/current but activation/reload proof is unavailable; include a reason such as `activation/reload proof unavailable` when runtime reload cannot be verified. `unsupported` is for unresolved roots or impossible safe operation. `supported` is reserved for cases where activation/reload proof is verified, which is not yet implemented. | Reserve `supported` until active reload is proven. | Reporting `supported` while activation/reload is unverified violates the status contract and would overstate runtime readiness. |
| Target aggregation | Replace the Codex advisory bypass with action-severity aggregation: Claude/OpenCode hard failures always fail; Codex hard failures fail; Codex read-only `status` partials may warn when other targets succeed, but must not hide any Claude/OpenCode failure. | Fail `--target all` for every Codex `partial`, or keep the old Codex unsupported bypass. | The spec allows honest Codex partial state while requiring non-Codex failures to remain visible and failing. |

## Data Flow

```text
engine runtime <action> --target codex
  └─ parseRuntimeArgs
     └─ runtimeAdapterForTarget(codex, root)
        └─ CodexAdapter
           ├─ resolve root: --config-root | CODEX_HOME | ~/.codex
           ├─ install/update: write owned manifest atomically
           ├─ status: validate manifest; partial means managed state is current
           │          but reasons disclose activation/reload proof limits
           └─ uninstall: remove only owned manifest
```

For `--target all`, the loop remains Claude → OpenCode → Codex and prints every result. Aggregation is testable by action:

- `install`, `update`, `uninstall`: `unsupported` or `partial` is a hard failure for every target; `restart_required` is a successful mutation result.
- `status`: Claude/OpenCode `unsupported`, `partial`, or `restart_required` remains a hard failure. Codex `unsupported` is a hard failure. Codex `partial` is a warning only when Claude/OpenCode are successful, because it represents honest degraded managed state rather than a masked non-Codex failure.
- If Claude or OpenCode fails, the overall command fails even when Codex is `supported` or `partial`.

## File Changes

| File | Action | Description |
|---|---|---|
| `engine/runtime/codex.go` | Modify | Implement root resolution, manifest read/write/remove, status semantics, and helper paths. |
| `engine/runtime/runtime.go` | Modify | Add `DefaultCodexConfigRoot()` and return `NewCodexAdapter(DefaultCodexConfigRoot())` for Codex. |
| `engine/cmd/main.go` | Modify | Route explicit roots to Codex and remove advisory-only Codex failure bypass. |
| `engine/runtime/codex_test.go` | Create | Unit tests for root resolution, install/update/status/uninstall, invalid manifests, and unrelated file preservation. |
| `engine/runtime/runtime_test.go` | Modify | Update Codex foundation expectations from placeholder unsupported to conservative adapter semantics. |
| `engine/cmd/runtime_test.go` | Modify | Replace advisory `all` test with real Codex aggregation success/failure tests. |

## Interfaces / Contracts

```go
const codexManifestFile = "labdrian-runtime-lifecycle.json"

type codexManifest struct {
    ManagedBy        string `json:"managed_by"` // "labdrian-sdd-overlay"
    InstalledVersion string `json:"installed_version"`
    ConfigRoot       string `json:"config_root"`
}
```

The adapter owns only `codexManifestFile`. Uninstall removes that file only; it MUST NOT delete `<root>/skills`, Codex config files, or arbitrary user entries. `Status()` returns `partial` for missing/incomplete managed state and for current managed manifest state when activation/reload proof is unavailable; it returns `unsupported` only when the root is unresolved or safe operation is impossible.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | Root resolution and manifest validation | Table-driven Go tests with `t.Setenv`, `t.TempDir()`, and no real home directory. |
| Unit | Mutation boundary | Create unrelated Codex files under `t.TempDir()` and assert byte-identical preservation after install/update/uninstall. |
| CLI core | `--target codex` and `--target all` exit semantics | Use `runRuntimeCore` buffers and sandbox roots; verify Codex install success semantics, Codex status partial warning behavior, Codex mutation outcomes, and Claude/OpenCode failure dominance. |
| Regression | Claude/OpenCode unchanged | Keep existing lifecycle tests green; update only expectations affected by aggregation. |

## Migration / Rollout

No data migration required. Existing Codex user state is preserved because only the Labdrian manifest is written or removed.

## Open Questions

- [ ] Codex activation/reload proof remains unknown; this slice currently reports `partial` for managed state while keeping activation/reload uncertainty visible in message/reasons until a future mechanism is proven.
