# Proposal: Runtime-wire the anti-generic-design contract

## Intent

`anti-generic-design` shipped advisory-only: a `SKILL.md` that only applies when its
trigger matches or it is invoked by name. The problem it addresses is the model's
DEFAULT bias (Inter, violet-blue gradient, generic shadow cards, flat 3-col grid),
which surfaces even from prompts that never mention "design" or "UI" (e.g. "add the
checkout button"). Trigger-based discovery cannot match a need the model does not
recognize BEFORE generating — so advisory-only leaves that blind spot uncovered.
This change makes the system AUTO-INJECT the guidance into UI-generating phases,
without anyone asking, exactly like `minimalism-contract` and `skill-discovery-safety`.

## Scope

### In Scope
- Author `skills/_shared/anti-generic-design.md` as a contract-file (same shape as the
  two existing contracts: `applies_to_phases` frontmatter + advisory-scope note),
  distilled from the current `skills/anti-generic-design/SKILL.md` content.
- Register it as an engine-owned EMBEDDED contract (Go engine change — see Approach):
  distinct marker-pair constants in `propagator`, a new `case` in `embeddedContract()`,
  and the canonical text as an embedded asset; rebuild + redistribute the binary.
- Wire `settings.json` hooks: `gate-task --embedded-contract anti-generic-design`
  (PreToolUse/Agent) and `propagate --embedded-contract anti-generic-design`
  (UserPromptSubmit).

### Out of Scope
- REAL per-phase/per-path hook gating (making the hook itself refuse to fire outside
  UI phases). The hook fires every turn; phase scope is soft, honored by the
  orchestrator reading the registry row. Hard gating is its own future SDD.
- Any change to the substantive design guidance (4 forbidden patterns, palette,
  heuristic). Only the delivery form changes: invokable skill → runtime-wired contract.

## Capabilities

### New Capabilities
- None (no new spec-level capability; extends existing runtime-wiring mechanism).

### Modified Capabilities
- `anti-generic-design`: delivery changes from trigger-invoked skill to auto-injected
  contract. Behavioral guidance is unchanged; the injection lifecycle is new.

## Approach

VERIFIED against source (not the original brief's assumption): the propagator DOES
parse and REQUIRE `applies_to_phases` (`engine/propagator/propagator.go`), and each
contract MUST own a DISTINCT marker pair or it overwrites another's registry block.
The only marker pairs are the minimalism default and the hardcoded
`skill-discovery-safety` embedded case; there is NO CLI flag for custom markers. So a
clean, non-colliding third contract MUST use the engine's documented extension point:
add a `case "anti-generic-design"` to `embeddedContract()` in `engine/cmd/main.go`
with new `propagator` marker constants and an embedded asset. Then the two hooks in
`settings.json` invoke it by `--embedded-contract anti-generic-design`.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `engine/propagator/propagator.go` | Modified | Add `AntiGenericDesign` BEGIN/END marker constants |
| `engine/cmd/main.go` | Modified | New `embeddedContract` case + `checkResult` block recognition |
| `engine/assets/` | New | Embedded canonical contract text |
| `skills/_shared/anti-generic-design.md` | New | Deployed standalone copy of the contract |
| `~/.claude/settings.json` | Modified | Two hook lines (gate-task + propagate) |
| `.atl/skill-registry.md` | Generated | New auto-generated scope block |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Marker collision overwrites minimalism/discovery blocks | Med | Use a distinct marker pair + row label; propagator tests cover isolation |
| Binary rebuild/redistribute drift (source vs `~/.claude/bin`) | Med | Ship via the same install path as the other embedded contract; verify with `overlay check` |
| Injection noise in non-UI apply turns | Med | Soft phase scope only; see question round on target phases |
| Brief mis-scoped Go work as "out of scope" | Confirmed | Corrected here with source evidence; see question round |

## Rollback

Additive and reversible: revert the two `settings.json` hook lines, revert the Go
diff (marker constants + `embeddedContract` case + asset), rebuild, and delete
`skills/_shared/anti-generic-design.md`. The original invokable `skills/anti-generic-design/`
skill is untouched and keeps working as a fallback.

## Dependencies

- Go toolchain to rebuild `gentle-ai-overlay`; redistribute to `~/.claude/bin/`.

## Success Criteria

- [ ] `propagate --embedded-contract anti-generic-design` writes a distinct scope block
      to `.atl/skill-registry.md` without touching the other two blocks.
- [ ] `gate-task --embedded-contract anti-generic-design` injects the contract path into
      UI-generating sub-agent prompts.
- [ ] `overlay check` reports the new block present and healthy.
- [ ] A prompt that never says "design" still receives the anti-generic guidance in the
      targeted phase.

## Decisions

1. **Scope**: separate contract, Go work accepted. Merging into `minimalism-contract.md`
   was rejected — it would couple two unrelated concerns (commit/task hygiene vs.
   visual design patterns) into one block, a cohesion violation that risks one
   contract's edits breaking the other. `anti-generic-design` gets its own marker
   pair and its own `embeddedContract()` case.
2. **Target phases**: `applies_to_phases: [sdd-tasks, sdd-apply]` — matches the
   methodological SDD flow (task breakdown, then implementation), not `sdd-apply` alone.
3. **Standalone copy**: keep `skills/anti-generic-design/` as an invokable skill
   alongside the new runtime-wired contract. It remains useful for ad-hoc invocation
   outside the SDD phase flow (e.g. reviewing a design produced elsewhere).
