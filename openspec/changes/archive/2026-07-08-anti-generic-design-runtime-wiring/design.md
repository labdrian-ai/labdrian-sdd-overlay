# Design: Runtime-wire the anti-generic-design contract

## Technical Approach

Register `anti-generic-design` as a THIRD engine-owned embedded contract, cloning the
`skill-discovery-safety` mechanism end-to-end: distinct marker pair in `propagator`, a new
`embeddedContract()` case sourcing an embedded asset, a Merger-installed hook pair, and status
recognition of the new registry block. Content ships in the binary (canonical) with a deployed
standalone copy the registry row points to. Substantive guidance is copied verbatim from the
existing `skills/anti-generic-design/SKILL.md`; only the delivery form changes.

## Architecture Decisions

### Decision: Hooks via `settings.Merger`, not a hand edit
The proposal lists only "two hook lines in `settings.json`". VERIFIED against source: the four
existing hook entries are built and deduped by `engine/settings/settings.go` (`mergeHooks`,
`buildSafety*Entry`), and `uninstall-hooks` removes them by identity. A manual edit would (a) be
dropped on any settings reset, (b) never be removed by `uninstall-hooks`, breaking the "same
install path" mitigation. **Choice**: add a third pair to `Merger` mirroring the safety pair.
**Alternative rejected**: hand-editing `settings.json` — breaks install/uninstall symmetry.

### Decision: Distinct marker slug `anti-generic-design-scope`
**Choice**: new constants `AntiGenericDesignBeginMarker/EndMarker` with slug
`anti-generic-design-scope` and row label `anti-generic-design`. **Rationale**: `Propagate` Case 2
matches its own begin marker via `strings.Contains`; a unique full-string marker plus a unique
first-cell row label (`isContractRow` exact match) guarantees no overwrite of the minimalism or
safety blocks. Generic `<!-- BEGIN:/END:` scanning in `hasUnscopedRow`/`appendToSharedContracts`
already skips foreign blocks, so the third block coexists.

### Decision: Extend `checkRegistry` + identity constants for parity
**Choice**: teach `main.go checkRegistry` and `settings.go HasSupportedClaudeLifecycleState` about
the new marker/identity so `overlay status`/`check` validate the block (Success Criterion 3).
**Rationale**: without it, health checks silently ignore the new contract.

## Data Flow

    UserPromptSubmit ─→ propagate --embedded-contract anti-generic-design
                          └─→ embeddedContract() → asset + marker pair → Propagate()
                                └─→ writes anti-generic-design-scope block in .atl/skill-registry.md
    PreToolUse/Agent ─→ gate-task --embedded-contract anti-generic-design
                          └─→ injects contract path into in-scope (sdd-tasks/sdd-apply) prompts

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `engine/propagator/propagator.go` | Modify | Add `AntiGenericDesignBeginMarker`/`EndMarker` const pair |
| `engine/assets/anti-generic-design.md` | Create | Canonical embedded contract text (frontmatter + body) |
| `engine/assets/assets.go` | Modify | Add `//go:embed anti-generic-design.md` → `var AntiGenericDesign string` |
| `engine/cmd/main.go` | Modify | New `case "anti-generic-design"` in `embeddedContract()`; extend `checkRegistry` marker check; update `usage()` embedded-contracts line |
| `engine/settings/settings.go` | Modify | `embeddedDesignName` const + `isDesignEntry` + `buildDesign{UserPromptSubmit,PreToolUse}Entry` + wire into `mergeHooks`/`removeHooks`/`HasSupportedClaudeLifecycleState` |
| `skills/_shared/anti-generic-design.md` | Create | Standalone deployed copy (identical to asset), registry Path target |
| `~/.claude/settings.json` | Generated | Third hook pair written by `merge-settings` (not hand-edited) |
| `.atl/skill-registry.md` | Generated | New `anti-generic-design-scope` block from propagate |

## Interfaces / Contracts

`embeddedContract("anti-generic-design")` returns (signature unchanged):

    embeddedContractSpec{
      content:     assets.AntiGenericDesign,
      beginMarker: propagator.AntiGenericDesignBeginMarker,
      endMarker:   propagator.AntiGenericDesignEndMarker,
      rowLabel:    "anti-generic-design",
      defaultPath: "skills/_shared/anti-generic-design.md",
    }, true

Contract frontmatter (drives the derived scope row): `applies_to_phases: [sdd-tasks, sdd-apply]`,
`excluded_phases: [sdd-propose, sdd-spec, sdd-design, sdd-verify, sdd-archive]`,
`injection_point: "## Skills to load before work"`. Body = the 4 forbidden patterns, steer-toward
list, and manual self-critique checklist distilled from `skills/anti-generic-design/SKILL.md`, plus
an advisory-scope note (frontmatter is documentation; registry row is load-bearing).

Merger command lines (exact shape mirrors safety pair):

    ...gentle-ai-overlay propagate --registry "${CLAUDE_PROJECT_DIR:-.}/.atl/skill-registry.md" --embedded-contract anti-generic-design || true
    ...gentle-ai-overlay gate-task --embedded-contract anti-generic-design --contract-path "$HOME/.claude/skills/_shared/anti-generic-design.md" || true

## Marker-Collision Risk & Mitigation

Each contract owns a unique full-string BEGIN/END marker and a unique first-cell row label.
`Propagate` isolates by `strings.Contains(registry, begin)` on its OWN marker; foreign blocks are
skipped by generic marker tracking. New slug `anti-generic-design-scope` shares no substring prefix
that would false-match `minimalism-contract-scope` or `skill-discovery-safety-scope`. Propagator
isolation tests MUST assert the three blocks coexist and that propagating one never mutates the
other two.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `embeddedContract("anti-generic-design")` resolves; unknown stays `ok=false` | table test in `main_test.go` |
| Unit | Three-block isolation; idempotent re-propagate | `propagator` test with all three markers |
| Unit | `mergeHooks`/`removeHooks` install/remove the design pair by identity | `settings_test.go` |
| Unit | Asset frontmatter parses; `applies_to_phases == [sdd-tasks, sdd-apply]` | `ParseFrontmatter` on `assets.AntiGenericDesign` |
| Integration | `overlay install-hooks` (build+merge) then `overlay status` reports the block healthy | manual/CI run |

## Migration / Rollout

Additive. `overlay install-hooks` rebuilds `~/.claude/bin/gentle-ai-overlay` and re-runs
`merge-settings` (idempotent — installs the third pair). `overlay apply` deploys the standalone
`skills/_shared/anti-generic-design.md`. Verify with `overlay status`/`overlay check`. Rollback:
`uninstall-hooks`, revert the Go diff + asset, rebuild, delete the standalone copy; the invokable
`skills/anti-generic-design/` skill stays intact as fallback.

## Open Questions

- [ ] Keep `engine/assets/anti-generic-design.md` and `skills/_shared/anti-generic-design.md`
      byte-identical (as safety does) — confirm a sync guard/test is desired in `sdd-tasks`.
