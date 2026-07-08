## Exploration: OO Quality Contract Runtime Wiring

### Current State
Runtime contract injection is deterministic for phase-only contracts, but it does not currently understand OO work context.

- `engine/gate/gate.go` reads one contract file, parses `applies_to_phases`, `excluded_phases`, and `injection_point`, then injects or strips one bare contract path line based only on `subagent_type`. It is fail-safe: malformed input, unknown phases, missing prompts, or broken frontmatter pass through unchanged.
- `engine/propagator/propagator.go` can generate scoped registry rows for any single contract when given distinct `Config` markers and row labels. It derives included/excluded phase text from frontmatter, preserves foreign marker blocks, and repairs stale or unscoped rows.
- `engine/cmd/main.go` exposes the generic `propagate` and `gate-task` commands. Minimalism uses external `--contract-file`; `skill-discovery-safety` uses the embedded-contract extension point with distinct markers. `oo-quality-contract` is not wired here yet.
- `engine/settings/settings.go` installs exactly two Claude hook pairs today: minimalism and skill-discovery-safety. A third contract would need its own identity, propagate hook, and PreToolUse gate hook to avoid dedup/removal collisions.
- `engine/runtime/opencode.go` and `engine/runtime/labdrian-runtime-parity-plugin.mjs` currently support a single `prompt_config` derived from `skills/_shared/minimalism-contract.md`. OpenCode runtime injection has no multi-contract list and no context gate beyond phase sets.
- `.atl/skill-registry.md` is generated as an index of skills and paths, not the current source of scoped shared-contract marker blocks. It has no `Shared Contracts` marker block for `oo-quality-contract` yet.
- `skills/_shared/oo-quality-contract.md` declares `language_context` and `activation_context`, but the current parser ignores those fields. Existing spec `openspec/specs/oo-quality-contract/spec.md` requires non-domain work to pass through and explicitly avoided runtime wiring in the first slice.

### Affected Areas
- `engine/propagator/propagator.go` — reusable row generation exists, but needs an OO-specific marker pair/label if registry propagation is added.
- `engine/gate/gate.go` — phase-only injection exists; deterministic context gating would need new metadata and decision logic, or the OO contract must remain manually loaded.
- `engine/cmd/main.go` — embedded/external contract dispatch is the CLI integration point for additional contracts.
- `engine/settings/settings.go` — Claude hook installation/removal must add a third independent hook pair if OO injection runs through hooks.
- `engine/runtime/opencode.go` — OpenCode config currently stores one contract, so OO support would require a multi-contract prompt configuration or a separate adapter path.
- `engine/runtime/labdrian-runtime-parity-plugin.mjs` — plugin currently mutates prompts from one prompt config and only by phase.
- `.atl/skill-registry.md` — generated registry semantics are path-index based; any OO scoped row must be marker-owned or regenerated safely.
- `skills/_shared/oo-quality-contract.md` — source contract for phase, language, activation, and precedence metadata.
- `openspec/specs/oo-quality-contract/spec.md` — promoted constraints require avoiding global OO injection and preserving minimalism/review-budget/spec/design precedence.
- `engine/skills/oo_quality_contract_artifact_test.go` — current first-slice test forbids runtime wiring and must be changed or superseded by this second-slice spec/tests.

### Approaches
1. **Phase-only OO hook** — Add OO as a third managed contract using the existing gate/propagator path and inject into `sdd-design`, `sdd-tasks`, and `sdd-apply` solely by frontmatter phase scope.
   - Pros: Smallest change; reuses proven fail-safe hook behavior; straightforward Go tests.
   - Cons: Violates the context gate because every design/tasks/apply subagent would receive OO guidance, including Go, docs, config, and non-domain work.
   - Effort: Low

2. **Prompt-text heuristic gate** — Extend gate/plugin config to inspect prompt text for TypeScript/NestJS/domain hints before injecting OO.
   - Pros: Can reduce over-injection without changing orchestration contracts.
   - Cons: Not deterministic enough; prompts may omit file paths or mention TypeScript only as context; false positives can still globalize OO advice and false negatives silently skip guidance.
   - Effort: Medium

3. **Explicit context-scoped contract loading** — Keep automatic phase strip behavior safe, but require the orchestrator/runtime to pass explicit work-context metadata before injecting OO; until that metadata exists, expose OO as a registry/skill path for manual or orchestrator-selected loading only.
   - Pros: Preserves the spec's context gate; avoids global SOLID/OO injection; keeps precedence and review-budget constraints intact; creates a clear extension point for future deterministic metadata.
   - Cons: Requires staged runtime design before fully automatic loading; not a one-slice automatic solution unless the orchestrator already supplies reliable context.
   - Effort: Medium

### Recommendation
Use Approach 3 as the safe staged path. The current runtime can reliably know `subagent_type`, but it cannot reliably know whether the target work is OO/domain-heavy TypeScript or NestJS. Therefore, do not add phase-only OO auto-injection. For this change, design the runtime contract model so multiple contracts can coexist, add OO-specific marker/identity plumbing only behind an explicit deterministic context predicate, and keep pass-through as the default when context is absent.

Practical next slice: write proposal/spec/design for a multi-contract prompt model with `language_context` and `activation_context` treated as load-bearing only when explicit context metadata is present. Tests should prove Go/docs/config/non-domain prompts pass through even in `sdd-design`, `sdd-tasks`, and `sdd-apply`; TypeScript/NestJS domain-heavy contexts inject exactly one OO contract path; minimalism and skill-discovery-safety behavior remains unchanged.

### Risks
- Current parser ignores `language_context` and `activation_context`; treating the existing frontmatter as enforced would be incorrect.
- Phase-only injection would over-apply OO guidance and conflict with the promoted spec.
- OpenCode runtime currently has a single-contract config shape, so multi-contract support may exceed one small implementation slice.
- Hook installation/removal identities must remain distinct or a third contract could collide with minimalism/safety lifecycle behavior.
- `.atl/skill-registry.md` is generated; manual edits are fragile unless marker preservation/regeneration is part of the design.

### Ready for Proposal
Yes — propose a staged runtime change that first establishes deterministic multi-contract/context metadata support and only enables OO injection when explicit work context proves OO/domain-heavy TypeScript or NestJS scope. The orchestrator should tell the user that fully automatic OO loading is unsafe with the current phase-only runtime signal.
