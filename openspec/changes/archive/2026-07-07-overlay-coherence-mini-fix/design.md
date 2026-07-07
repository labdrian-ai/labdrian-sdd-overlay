# Design: Overlay Coherence Mini-Fix

## Technical Approach

Keep the overlay architecture intact: `bin/labdrian-overlay` remains the thin user-facing wrapper, while `engine/cmd` and `engine/gadu` remain the control-plane/generator owners. This change adds one wrapper bridge, makes hook status diagnostics truly read-only, updates the canonical GADU persona body, regenerates the three generated outputs, and aligns README/OpenSpec wording with the implementation.

## Architecture Decisions

| Area | Decision | Rationale |
|------|----------|-----------|
| Wrapper command | Add `gadu-generate` to `usage()` and dispatch with `exec env OVERLAY_DIR="$OVERLAY_DIR" "$ENGINE_BINARY" gadu-generate "$@"`. | Mirrors existing `prespec`/`skills` forwarding, preserves all post-command arguments, and gives the installed engine the repo root it already requires. |
| Missing engine binary | `gadu-generate` and `status-hooks` fail loud when `$ENGINE_BINARY` is absent. | `install-hooks` is the explicit mutating setup path; read-only/status paths must not build or deploy. |
| Status hooks | Remove `go` requirement and build fallback from `cmd_status_hooks`; check only executable presence, then `exec "$ENGINE_BINARY" status`. | Satisfies the read-only contract and avoids writing `~/.claude/bin/gentle-ai-overlay` during diagnostics. |
| GADU wording | Replace canonical `You run on Claude...` wording with runtime-neutral phrasing in `engine/gadu/persona/body.md`, then regenerate all outputs. | Generated Claude Code, OpenCode, and skill artifacts share the same body, so portability must be fixed at the source. |
| Docs coherence | Update only direct command/artifact docs. | Minimizes review scope and avoids unrelated runtime lifecycle churn. |

## Data Flow

```text
User
  └─ bin/labdrian-overlay gadu-generate [args...]
       ├─ requires existing ~/.claude/bin/gentle-ai-overlay
       ├─ exports OVERLAY_DIR=<repo root>
       └─ execs engine gadu-generate [args...]
            └─ engine/gadu reads canonical body and writes/checks 3 artifacts
```

`status-hooks` flow is intentionally shorter: wrapper validates the deployed binary is executable, otherwise exits non-zero with guidance to run `labdrian install-hooks` or `bin/labdrian-overlay install-hooks`; no `go build`, `mkdir`, deploy, settings write, or tool install occurs.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `bin/labdrian-overlay` | Modify | Add `gadu-generate` help/dispatch; make `status-hooks` missing-binary failure read-only and actionable. Do not edit `bin/overlay`. |
| `engine/gadu/persona/body.md` | Modify | Remove runtime-specific Claude wording from canonical persona body. |
| `agents/GADU.md` | Regenerate | Claude Code agent output from canonical body. |
| `opencode/agents/GADU.md` | Regenerate | OpenCode native agent output from canonical body. |
| `skills/gadu-operator/SKILL.md` | Regenerate | Portable skill output from canonical body. |
| `README.md` | Modify | Document wrapper `gadu-generate [--check]`, three generated artifacts, and read-only `status-hooks` behavior. |
| `openspec/changes/overlay-coherence-mini-fix/*` | Modify | Keep proposal/spec/design/tasks wording coherent as the change proceeds. |

## Interfaces / Contracts

- `bin/labdrian-overlay gadu-generate [--check] [...future args]` passes every argument after `gadu-generate` unchanged to `gentle-ai-overlay gadu-generate`.
- The wrapper supplies `OVERLAY_DIR` as the resolved repository root, overriding neither the engine contract nor generated paths.
- `status-hooks` missing binary error must mention the binary path and `install-hooks`; it must exit unsuccessfully before any mutating command.
- Generated artifacts remain machine-owned and checked by `gadu-generate --check`.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|--------------|----------|
| Shell syntax | Wrapper remains parseable. | `bash -n bin/labdrian-overlay`; run ShellCheck if available. |
| Wrapper behavior | `gadu-generate --check` dispatch includes `OVERLAY_DIR` and preserves args; `status-hooks` missing binary does not build/deploy. | Prefer focused shell tests under `engine/installer` using `t.TempDir()`, sandbox `HOME`, and a fake executable engine if practical; otherwise perform manual command checks with sandboxed env. |
| Go generator | Three artifacts emitted, body identical, no runtime-specific canonical phrase, stale/missing checks fail. | Extend `engine/gadu/gadu_test.go` with table-driven cases. |
| Docs/OpenSpec | README and change docs list three artifacts and wrapper command. | Targeted content checks/manual review. |
| Module validation | Touched Go module stays green. | `cd engine && go test ./...`; `cd tui && go test ./...` only if TUI docs/contracts are touched. |

## Migration / Rollout

No data migration required. Existing users who relied on implicit `status-hooks` build must run `install-hooks` explicitly once. Rollback is a single work-unit revert of wrapper, persona, regenerated artifacts, docs, and tests.

## Risks

- **Medium**: Scripts may depend on `status-hooks` auto-building; mitigated by explicit error guidance.
- **Low**: Generated output may drift if regeneration is skipped; mitigated by `gadu-generate --check` and Go tests.
- **Low**: Wrapper behavior tests may be brittle if they invoke the real home directory; mitigate with sandboxed `HOME`/`OVERLAY_DIR` only.

## Open Questions

None.
