# Design: Pi Runtime Target

## Technical Approach

Pi only accepts content via `pi install <source>` / removes via `pi remove <source>` (package model), unlike claude/opencode/codex's per-file copy (`TARGET_PATHS`/`AGENT_TARGET_PATHS`). `pi` becomes a valid `--target` dispatched through a `TARGET_KINDS[pi]=package` lookup (new) rather than through the copy loop, to the engine binary, which owns package build (`engine/pipkg`, mirrors `engine/gadu`), the `before_agent_start` gate extension, and package-declared MCP.

## Corrections from verified research (supersede prior revision)

- **A1 — MCP shape**: `pi.mcp` in `package.json` is a package-relative **path** (or array of paths) to a file holding a normal top-level `mcpServers` object (pi-mcp-adapter README:131-143), not an inline object. Decision: ship `mcp.json` in the built package; `package.json` declares `"pi": {"mcp": "./mcp.json"}`. `longterm-mem register --target pi` writes `mcpServers.longterm-mem` into that `mcp.json` using the **existing, unmodified** `jsonInstall(containerKey="mcpServers", ...)` — no writer change. The previously proposed `containerPath []string` widening is dropped: `locate()`/`WriteMember` are root-key-only by design and `mcp.json`'s `mcpServers` is already root-level, so no nesting problem exists.
- **A2 — Uninstall verb**: `pi remove <source>` (docs/packages.md:29,43), where `<source>` is the same install string used for `pi install` (our local package path) — not `pi uninstall <name>`.
- **A3 — Bypass flags**: `--no-extensions` disables extension discovery; `--no-skills` disables skill discovery; there is no `-ns` alias for either (README:595-597, docs/usage.md:224-226). R-007 disclosure is rewritten: `status --target pi` states both flags disable their respective discovery, with no detection API for either.
- **A4 — Extension shape and chaining**: `export default function (pi) { pi.on("before_agent_start", async (event, ctx) => { ... return { systemPrompt }; }); }` (docs/extensions.md:64,161; gentle-pi `gentle-ai.ts:6477/6065`). Handlers **chain**: each receives the already-accumulated `event.systemPrompt` from prior handlers (docs/extensions.md:538-539,565), so `labdrian-gate.js` only needs `return { systemPrompt: event.systemPrompt + entry }` — no manual merge with gentle-pi's output. The composition test is kept regardless, to pin execution-order behavior across the two packages.
- **A5 — File type**: package `extensions/` discovery of plain `.js` is undocumented; all examples are `.ts` loaded via jiti. Decision: ship `extensions/labdrian-gate.ts` written in type-annotation-free syntax (valid TS with zero TS-specific constructs), keeping the "no TS toolchain" trade-off honest — the file never needs `tsc`. The Go test copies it to a `.mjs` fixture (same bytes) before running it under `node`, and this copy step is asserted/stated explicitly in the test's own comment so the divergence from `opencode_test.go`'s direct `.mjs` import is documented, not silent.
- **C4/C5 — Injection format and Go mirror**: the gate reads each contract's `applies_to_phases`/`excluded_phases`/`injection_point` frontmatter with a strict inline-list parse (mirrors `engine/gate/gate.go:216-263`) and injects a **bare path line** under the `injection_point` header — exactly `gate.go:319-382`'s format and the OpenCode plugin's `injectPrompt` (`opencode_test.go:521-522`), never prose. The Go-side unit test reuses the **already-exported** mirror in `engine/runtime/runtime.go:85-180` (`LoadContractPhases`, `AppliesToPhase`, `ExcludesPhase`, `MutatePrompt`, `InjectPrompt`, `CanonicalEntry`) as the assertion oracle instead of re-deriving gate.go's logic a third time.

## Architecture Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Pi content delivery | Build `labdrian-pi` package tree into `$HOME/.labdrian-overlay/pi/labdrian-pi` at apply time (`engine/pipkg.Build`), `pi install <path>` | One source of truth (skills.registry.yaml + agents/); no committed duplicate tree |
| Target dispatch | New `TARGET_KINDS[claude]=copy [opencode]=copy [codex]=copy [pi]=package` array in `bin/labdrian-overlay`; `resolve_targets`/`--target all` includes `pi`; copy-loop call sites keyed off `TARGET_PATHS` membership stay unchanged in shape, gaining an `is_copy_target`/`is_valid_target` helper wrapping both arrays | Explicit membership test replaces implicit `-v TARGET_PATHS[...]` guards at the 8 sites below without duplicating "is this a real target" logic |
| Gate mechanism | `before_agent_start` extension (`extensions/labdrian-gate.ts`), chains additively with gentle-pi's own handler (A4) | Research-confirmed mechanism; `tool_call` cannot mutate input |
| Gate implementation | Type-annotation-free `.ts`, go:embed'd, tested by copying to `.mjs` under `node` (A5) | Matches undocumented-but-safest package convention; keeps zero-TS-toolchain trade-off honest |
| MCP delivery | `package.json` → `"pi":{"mcp":"./mcp.json"}`; `mcp.json` holds `mcpServers.longterm-mem`, written by unmodified `jsonInstall` (A1) | Matches pi-mcp-adapter's actual schema; removed automatically on `pi remove` |
| Uninstall verb | `PiAdapter.Uninstall()` runs `pi remove <package-path>` (A2) | Matches the real CLI; `pi uninstall` does not exist |
| Contract injection format | Bare path line under `injection_point` header, via `engine/runtime/runtime.go`'s exported `InjectPrompt`/`MutatePrompt` (C4/C5) | Byte-identical semantics to Claude's gate and the OpenCode plugin; no new prose format to maintain |
| Pi CLI invocation | `exec.Command("pi", "install"/"remove", path)`, fixed argv, `exec.LookPath("pi")` first | No shell interpolation |
| Distribution scope | Local-path install only | Covers actual daily-use case; git/npm deferred |
| Skill selection | `pipkg.Build(overlayRoot, registryPath, destDir)` selects entries whose `install.targets` contains `pi` in `skills.registry.yaml` (C3) | Explicit opt-in per skill, not an implicit claude-set copy |
| `register --target pi` config root | `--config-root <built package dir>` (points at the package, not a user config root like the other three targets) (C3) | The package IS the config surface for Pi's MCP; documented as an intentional asymmetry, not an open question |
| `register --target all` semantics for pi | Probe: package dir present **and** listed in `~/.pi/agent/settings.json` `packages` → attempt; absent → skip (same as codex's missing-config skip); explicit `--target pi` with package absent → fail (C6) | Matches existing `cmd_register.go` all-vs-explicit skip/fail asymmetry |
| Security (F1-F3) | Extension resolves contract paths only inside the package root (reject `..`/absolute); strict frontmatter parse (malformed → pass-through, never throw); `pipkg.Build` refuses symlinks under `skills/`/`agents/`, builds into a temp dir and atomically swaps into `destDir`, sets 0644 files / 0755 dirs | Contains the extension's file-read surface; atomic swap avoids partial/stale package state |

## `TARGET_PATHS`-keyed call sites requiring the `is_copy_target`/`TARGET_KINDS` change (bin/labdrian-overlay)

`resolve_targets` (:355-364, hard-coded `"claude opencode codex"`); validation guards at :1298, :1900, :2987, :3039; per-target loops at :1506-1507, :2075-2076, :2323-2324. Each guard/loop is updated to use the new helper so `pi` is recognized as valid and routed to the package path (`pipkg`/`PiAdapter`) instead of falling through to `mkdir -p ""` against an empty `TARGET_PATHS[pi]`.

## Data Flow

    bin/labdrian-overlay --target pi apply
         │ is_copy_target(pi) == false → package dispatch
         ▼
    engine binary: pipkg.Build(overlayRoot, registryPath, destDir)
         │  selects skills.registry.yaml entries with install.targets ∋ "pi"
         │  copies agents/*.md → package agents/ (reject symlinks)
         │  embeds extensions/labdrian-gate.ts
         │  writes package.json {"pi":{"mcp":"./mcp.json"}}, mcp.json {"mcpServers":{}}
         │  builds in temp dir, atomic swap into destDir
         ▼
    longterm-mem register --target pi --config-root <destDir>
         │  RegisterPi → jsonInstall(containerKey="mcpServers", memberKey="longterm-mem", ...)
         ▼
    PiAdapter.Install(): exec pi install <destDir>
         ▼
    Pi session start → gentle-pi before_agent_start (chain, first)
         → labdrian-gate.ts before_agent_start (chain, reads event.systemPrompt, appends path line)

## File Changes (restructured into 5 slices, each < 400 authored lines)

### Slice 1 — `pi-target-plumbing`
| File | Action |
|---|---|
| `engine/runtime/runtime.go` | Modify — `TargetPi`, `ExpandTarget`/`ParseTarget`/`AllTargets`, `NewFoundationAdapter` case |
| `engine/runtime/pi.go` | Create — `PiAdapter` skeleton (`Target()` only wired; other methods `CapabilityUnsupported` stub for this slice) |
| `bin/labdrian-overlay` | Modify — `TARGET_KINDS`, `is_copy_target` helper, `resolve_targets`/`Valid:` strings accept `pi`, the 8 call sites listed above |

### Slice 2 — `pi-package-build`
| File | Action |
|---|---|
| `engine/pipkg/pipkg.go` | Create — `Build`, `Check` (temp-dir + atomic swap, symlink rejection) |
| `skills.registry.yaml` | Modify — add `pi` as a valid `install.targets` value; opt in the entries that should ship to Pi |
| `engine/runtime/pi.go` | Modify — `Install`/`Apply`/`SyncCheck` call `pipkg.Build`/`Check`, `exec.Command("pi","install",path)` |
| `bin/labdrian-overlay` | Modify — apply/sync-check dispatch for `pi` routed to the engine binary |

### Slice 3 — `pi-contract-gate`
| File | Action |
|---|---|
| `engine/pipkg/labdrian-gate.ts` | Create — go:embed'd extension, chains via `event.systemPrompt` |
| `engine/pipkg/pipkg.go` | Modify — embeds and copies the extension into the built package |
| `engine/runtime/runtime.go` | None (reused as-is, C5) |

### Slice 4 — `pi-longterm-mem-mcp`
| File | Action |
|---|---|
| `longterm-mem/internal/register/pi.go` | Create — `RegisterPi(packageDir, stateDir, binary string) error`, targets `<packageDir>/mcp.json` |
| `longterm-mem/cmd/longterm-mem/cmd_register.go` | Modify — `--target pi` case; `all`-expansion probe (package dir + settings.json listing) per C6 |
| `engine/pipkg/pipkg.go` | Modify — writes `package.json`'s `"pi":{"mcp":"./mcp.json"}` and an empty `mcp.json` skeleton |

### Slice 5 — `pi-lifecycle`
| File | Action |
|---|---|
| `engine/runtime/pi.go` | Modify — `Status` (honest per-entry proof + `--no-extensions`/`--no-skills` disclosure), `Uninstall` (`pi remove <path>`, never touches gentle-pi/pi-engram files), `Update`/`Rollback` |
| `openspec/specs/runtime-lifecycle/spec.md` | Modify — Pi requirements/scenarios |
| `README.md` | Modify — Pi deploy target section |

## Interfaces / Contracts

```go
// engine/pipkg/pipkg.go
func Build(overlayRoot, registryPath, destDir string) error
func Check(overlayRoot, registryPath, destDir string) error
```
```ts
// engine/pipkg/labdrian-gate.ts (type-annotation-free; valid plain JS too)
export default function (pi) {
  pi.on("before_agent_start", async (event, ctx) => {
    const name = readAgentName(event); // mirrors gentle-pi's readAgentStartNames shape
    if (name !== "sdd-tasks" && name !== "sdd-apply") return {};
    return { systemPrompt: injectContracts(event.systemPrompt, name) };
  });
}
```

## Testing Strategy — mapped to 5 slices, strict TDD

| Slice | RED (fails first) | GREEN | Command |
|---|---|---|---|
| pi-target-plumbing | `TestExpandTarget_Pi`, `TestResolveTargets_Pi`, `TestIsCopyTarget_ClaudeTrue_PiFalse` (shelltest) | Target/kind plumbing wired | `cd engine && go test ./runtime/... ./cmd/... ./shelltest/...` |
| pi-package-build | `TestPipkgBuild_SelectsPiTargetedSkills`, `TestPipkgBuild_RejectsSymlinks`, `TestPipkgBuild_AtomicSwap`, `TestPipkgCheck_DetectsDrift` | `pipkg.Build`/`Check` implemented | `cd engine && go test ./runtime/...` (new `engine/pipkg/...` added to focused set) |
| pi-contract-gate | `TestLabdrianGateInjectsPathLine_SddTasksSddApply` (Go, via `engine/runtime` exported mirror), `TestLabdrianGateChainsAfterGentlePi` (node, skip if absent — copies `.ts`→`.mjs` fixture, mirrors `opencode_test.go:472`), `TestLabdrianGatePathContainment_RejectsTraversal` | Extension implemented, path-contained, chains correctly | `cd engine && go test ./runtime/...` |
| pi-longterm-mem-mcp | `TestRegisterPi_WritesMcpServersLongtermMem`, `TestCmdRegister_TargetAll_SkipsAbsentPi`, `TestCmdRegister_TargetPiExplicit_FailsWhenPackageAbsent` | `RegisterPi` + `cmd_register.go` case implemented | `cd longterm-mem && go test ./internal/register/... ./cmd/...` |
| pi-lifecycle | `TestPiAdapter_StatusPartialOnUnprovenEntry`, `TestPiAdapter_StatusDisclosesNoExtensionsNoSkills`, `TestPiAdapter_UninstallUsesRemoveNotUninstall`, `TestPiAdapter_UninstallNeverTouchesGentlePiFiles` | Status/Uninstall honest and correct-verb | `cd engine && go test ./runtime/...` |
| All | — | — | `go vet ./... && go test ./...` (both modules) + `shellcheck -S warning bin/labdrian-overlay` |

**Manual-only (no headless Pi runner)**: live Pi session skill/agent discovery post-install; gate injection observed in an actual `sdd-tasks`/`sdd-apply` system prompt; `pi remove` observed to leave `~/.pi/agent/settings.json`'s gentle-pi/pi-engram-owned entries byte-identical. These are checkpoint verification steps, not automated RED tests, and are called out as such in `pi-package-build` and `pi-contract-gate` and `pi-lifecycle` task lists.

## Threat Matrix

| Boundary | Applicability | Design response |
|---|---|---|
| Documentation-like paths | N/A — skills/agents copied verbatim, never executed | — |
| Git repository selection | N/A — local-path install only | — |
| Commit state / Push state / PR commands | N/A — no git operations | — |
| Process integration (non-template row) | Applicable — `pi install`/`pi remove` subprocess | Fixed argv, `exec.LookPath("pi")` first, no shell string; `TestPiAdapter_InstallNoShellInjection` |
| Path containment (non-template row, F1) | Applicable — gate reads contract files at load | Reject `..`/absolute; resolve only under package root; `TestLabdrianGatePathContainment_RejectsTraversal` |

## Migration / Rollout

Additive only. No `jsonInstall`/writer signature change (A1 correction). Five stacked PRs per the re-sliced `entry.json` order; each independently `git revert`-able.

## Where I doubt the 400-line budget

- **pi-package-build**: symlink rejection + atomic temp-dir swap + registry-target selection + drift `Check` is real logic, not scaffolding; combined with its RED tests this slice is the most likely to brush 400 lines. If it does, split `Check`/drift-detection into `pi-package-build` (Build only) and fold `Check` into `pi-target-plumbing`'s `sync-check` wiring instead — flagging this now rather than discovering it mid-implementation.
- **pi-contract-gate**: the `.ts`→`.mjs` test-fixture copy step plus a strict frontmatter parser mirror adds Go-side surface beyond the extension file itself; still expected under budget but tighter than the other slices given the two-language test.

## Open Questions

- [ ] Confirm `skills.registry.yaml`'s exact opt-in set for `install.targets: [pi]` during `pi-package-build` implementation — this design assumes an explicit per-skill decision, not an automatic claude-set mirror.
