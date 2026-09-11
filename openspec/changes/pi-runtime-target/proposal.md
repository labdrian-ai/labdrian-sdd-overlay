# Proposal: Pi Runtime Target

## Intent

The overlay currently ships parity adapters for `claude`, `opencode`, and `codex` only. Pi (via gentle-pi) is a fourth agent runtime the user actively runs, but it has no `--target pi`, no package of skills/agents, no per-phase contract gate, and no longterm-mem MCP registration path. Without this, Pi sessions get none of the overlay's SDD discipline (minimalism/anti-generic-design contracts, GADU, longterm-mem) that claude/opencode/codex already receive. This closes that gap using research-confirmed Pi mechanisms (`before_agent_start`, `pi.mcp`, local-path packages).

## Scope

### In Scope
- `bin/labdrian-overlay --target pi` builds and installs a `labdrian-pi` npm-shaped package (skills + custom agents) via `pi install` from a local path (dev) / git (others); updates are rebuild + reinstall.
- Custom agents shipped only under the package's `agents/`, never written into gentle-pi's `~/.pi/agent/agents/`.
- A `before_agent_start` extension in the package that injects the per-phase contract gate (mirroring `engine/gate/gate.go` frontmatter matching) into `sdd-tasks`/`sdd-apply` only, composed with (not replacing) gentle-pi's own `before_agent_start` handler.
- longterm-mem MCP reachable from Pi, coexisting with pi-engram's `mcp.json`, via `pi.mcp` package declaration (per `pi-mcp-adapter` precedence rules).
- Honest `partial`/`unsupported` status reporting, `pi -ns` disclosure in Status/Install output, Pi-scoped uninstall, and `sync-check --target pi` drift detection.
- Non-regression of claude/opencode/codex targets.

### Out of Scope
- Modifying gentle-pi itself or its `~/.pi/agent/` global state.
- OpenCode changes of any kind.
- `session_shutdown` sync wiring for Pi (deferred).
- A CI-run Pi integration test (no headless Pi runner available); verification stays local/manual per slice.

## Capabilities

### New Capabilities
- `pi-runtime-target`: CLI target, package build/install, extension gate, MCP registration, lifecycle actions for Pi.

### Modified Capabilities
- `runtime-lifecycle`: extend the `Target` enum (`engine/runtime/runtime.go`) and `NewFoundationAdapter`/`AllTargets` with a `PiAdapter`; extend `bin/labdrian-overlay`'s `TARGET_PATHS`/`AGENT_TARGET_PATHS` and target-validation strings.
- `longterm-mem-mcp-registration`: extend `longterm-mem register --target` (`longterm-mem/cmd/longterm-mem/cmd_register.go`) and `internal/register/` with a `pi` case that writes `pi.mcp`-shaped config, not `~/.pi/agent/mcp.json` directly.

## Approach

Four stacked slices (chained PRs, `pi-target-core` → `pi-contract-gate` → `pi-longterm-mem-mcp` → `pi-lifecycle`).

1. **Package build** (mirrors `engine/gadu` generator precedent): a build step emits a `labdrian-pi/` package directory (package.json + `skills/` copied from the overlay + `agents/`), installed with `pi install <path|git-url>`. `PiAdapter` (new `engine/runtime/pi.go`) implements `Adapter`, reusing `ClaudeAdapter`/`OpenCodeAdapter`'s file-fingerprint pattern.
2. **Contract gate**: plain JavaScript extension (not TypeScript — no TS toolchain wired into this repo's CI; documented trade-off: no compile-time type check, offset by focused Go-side unit tests on the frontmatter-parsing logic it mirrors) under the package's `extensions/`. It reads `applies_to_phases`/`excluded_phases`/`injection_point` frontmatter the same way `engine/gate/gate.go` does, restricted to `sdd-tasks`/`sdd-apply`, and merges its `systemPrompt` return with gentle-pi's own handler output rather than overwriting it.
3. **MCP**: package.json `pi` key ships `mcp: { "longterm-mem": {...} }`; `internal/register` gets a `pi` target writing to the package manifest (not a live user config file), since `pi.mcp` is package-declared, not injected into a user JSON/TOML like claude/opencode/codex.
4. **Lifecycle**: Status/Install messages state Pi-specific caveats (`pi -ns` disables extensions — no detection API exists, so this is disclosed as a limitation, not runtime-checked); `sync-check --target pi` diffs installed package hash against the generator's current output (same idiom as `engine/gadu.Check`); Uninstall calls `pi uninstall labdrian-pi` and removes only overlay-owned state.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `bin/labdrian-overlay` | Modified | `TARGET_PATHS`, `AGENT_TARGET_PATHS`, target validation strings, `--target pi` dispatch |
| `engine/runtime/pi.go` (new) | New | `PiAdapter` implementing `Adapter` (Target/Apply/Install/Status/SyncCheck/Update/Rollback/Uninstall) |
| `engine/runtime/runtime.go` | Modified | `TargetPi` constant, `AllTargets`, `NewFoundationAdapter` |
| `pi/` (new dir, package source) | New | `package.json`, `skills/`, `agents/`, `extensions/gate.js` — build output installed via `pi install` |
| `longterm-mem/internal/register/pi.go` (new) | New | Pi MCP registration writing `pi.mcp` package manifest shape |
| `longterm-mem/cmd/longterm-mem/cmd_register.go` | Modified | `--target pi` case |
| `openspec/specs/runtime-lifecycle/spec.md` | Modified | Pi target requirements |
| `README.md` | Modified | Pi deploy target documentation |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| gentle-pi upstream changes `before_agent_start` signature/precedence | Med | Compose (never replace) its handler; pin research to installed version; sync-check surfaces drift |
| `pi -ns` silently disables the gate with no detection API | High | Documented limitation (R-007); status output discloses it explicitly rather than claiming false coverage |
| Extension in plain JS has no CI type-check | Med | Focused unit tests on the mirrored Go-side frontmatter logic; small, single-purpose file |
| Four hard-coded target lists (`TARGET_PATHS`, `AGENT_TARGET_PATHS`, `Target` enum, register `--target`) drift out of sync when adding `pi` | Med | Single PR slice (`pi-target-core`) touches all four together; `sync-check --target pi` catches later drift |
| No headless Pi CI runner | High | Manual/local verification per slice checkpoint; honest `partial` status if unverifiable (R-006) |

## Rollback Plan

Each slice is independently revertable (`git revert`) since Pi is additive: no existing claude/opencode/codex code path is modified, only extended (new enum case, new adapter file, new register target). Users can also run `bin/labdrian-overlay uninstall --target pi` to remove the package and Pi-owned state without touching other targets.

## Dependencies

- Pi CLI and gentle-pi package installed locally for manual verification (already present per research: `~/.pi/agent/npm/node_modules/gentle-pi`).
- `pi-mcp-adapter` 2.32.1+ semantics for `pi.mcp` precedence (research C5).

## Success Criteria

- [ ] `bin/labdrian-overlay --target pi status` reports honest supported/partial/unsupported per capability, including `pi -ns` disclosure.
- [ ] A live Pi session lists the GADU agent from the installed package and the contract gate injects minimalism/anti-generic-design guidance only into `sdd-tasks`/`sdd-apply` system prompts (verified manually, not via CI).
- [ ] `longterm-mem register --target pi` registers the MCP server without modifying pi-engram's existing `mcp.json`.
- [ ] `sync-check --target pi` detects package drift after a source edit.
- [ ] claude/opencode/codex `status`/`sync-check`/`register` outputs are byte-identical to pre-change behavior (non-regression, R-008).
