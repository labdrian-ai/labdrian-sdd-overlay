# Proposal: OO Quality Contract Runtime Wiring

## Intent

Enable deterministic runtime support for `skills/_shared/oo-quality-contract.md` without applying OO/SOLID guidance globally. The current runtime can gate by SDD phase only; it cannot reliably infer OO/domain-heavy TypeScript or NestJS work context. User value is safer guidance injection: pass through by default, inject only when explicit metadata proves scope.

## Scope

### In Scope
- Define a first implementation slice for multi-contract, context-aware runtime support.
- Treat `language_context` and `activation_context` as load-bearing only when explicit context metadata is available.
- Preserve existing minimalism and skill-discovery-safety behavior.
- Specify pass-through behavior for absent, malformed, or insufficient context metadata.
- Add tests/validation proving non-domain Go, docs, config, and generated-artifact work does not receive OO guidance.

### Out of Scope
- Global or phase-only OO auto-injection for `sdd-design`, `sdd-tasks`, or `sdd-apply`.
- Prompt-text heuristics for detecting TypeScript, NestJS, or domain-heavy work.
- Manual edits to generated registry content unless marker-owned regeneration is designed.
- Broad production rewiring before specs and design define deterministic context metadata.

## Approach

Use a staged path: design the runtime model for multiple contracts and explicit context predicates first. The default decision is pass-through unless phase scope and trusted work-context metadata both match. OO-specific hook, registry, or OpenCode plugin wiring must remain disabled or guarded until deterministic context is present.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `engine/gate/gate.go` | Future Modified | Context-aware contract decisions beyond phase-only gating. |
| `engine/runtime/opencode.go` | Future Modified | Multi-contract prompt configuration. |
| `engine/runtime/labdrian-runtime-parity-plugin.mjs` | Future Modified | Context-aware plugin injection. |
| `engine/settings/settings.go` | Future Modified | Distinct managed hook identity if OO hooks are enabled. |
| `skills/_shared/oo-quality-contract.md` | Reference | Source metadata and precedence constraints. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Phase-only wiring over-applies OO guidance | High | Keep explicitly out of scope. |
| Context metadata is underspecified | Medium | Require spec/design before implementation. |
| Multi-contract runtime exceeds review budget | Medium | Slice work by contract model, gate behavior, then runtime adapters. |

## Rollback Plan

Revert the runtime wiring independently from the archive documents. Rollback means removing the multi-contract gate/runtime changes in `engine/gate`, `engine/runtime`, `engine/cmd`, and `engine/settings`, then restoring the previous phase-only behavior while keeping unrelated archived specs intact.

## Dependencies

- Completed `oo-quality-contract` spec, especially R-003 phase scope, R-004 precedence, R-005 context gate, R-007 TDD respect, and R-008 first-slice no-wiring boundary.
- Existing fail-safe behavior: malformed or unknown inputs pass through unchanged.

## Review Workload Expectations

- Review budget: 800 changed lines.
- Expected implementation should be sliced if multi-contract model plus adapters approaches the budget.
- Final delivery included runtime code, tests, and archive updates; it was not documentation-only.

## Success Criteria

- [x] Specs define deterministic context metadata and pass-through defaults.
- [x] Design rejects phase-only OO injection without trusted context.
- [x] Tests prove scoped matching TypeScript/NestJS domain work injects exactly once, while absent or non-matching trusted invocation context passes through.
