# Exploration: oo-quality-contract

### Current State

The overlay already uses compact shared contracts for cross-phase behavior. `skills/_shared/minimalism-contract.md` is the closest precedent: it is a Markdown contract with advisory frontmatter, tracked as `_shared/minimalism-contract.md custom` in `overlay.manifest`, and scoped by runtime prompt injection rather than by becoming a standalone `SKILL.md`. The engine can parse contract frontmatter (`applies_to_phases`, `excluded_phases`, `injection_point`) and inject or strip a bare contract path under `## Skills to load before work` for matching `subagent_type` values.

The project rules explicitly require scoped governance and manifest-tracked skill/agent changes. The current generated `.atl/skill-registry.md` is an index of skills only and does not currently list shared contracts, while archived minimalism artifacts describe an older Shared Contracts row approach. Current engine code is the stronger source for deployment: `_shared/*` entries are infra rows excluded from `ManifestView`, and contract scoping is enforced through frontmatter plus propagator/gate behavior, not through a normal skill registry entry.

The prior discovery memory already rejected importing `ramziddin/solid-skills` as-is because it is TypeScript/NestJS/OOP-oriented and dogmatic. The useful part is a narrow advisory quality contract for OO/domain-heavy TypeScript or NestJS work, not a global SOLID/TDD mandate.

### Affected Areas

- `skills/_shared/oo-quality-contract.md` — recommended canonical contract location, following the minimalism and skill-discovery-safety shared-contract pattern.
- `overlay.manifest` — must include `_shared/oo-quality-contract.md custom` so the overlay tracks and deploys the contract.
- `skills.registry.yaml` — likely not updated for the contract itself, because `_shared/*` files are infra artifacts rather than skill directories; only update if later design intentionally models shared contracts in the registry.
- `.atl/skill-registry.md` — should not be hand-edited as a durable source of truth; if used, it must be regenerated or propagated and treated as an index, not the canonical contract.
- `engine/propagator/propagator.go` — may need a distinct marker pair and row label if this contract is propagated like `skill-discovery-safety`.
- `engine/cmd/main.go` — may need an embedded-contract entry or install-hook propagation wiring if runtime injection should be deterministic across targets.
- `engine/gate/gate.go` — existing frontmatter-driven injection/strip behavior can support another contract if invoked with the right contract content/path.
- `engine/*_test.go` — tests should prove the new contract has distinct markers, correct phase scope, coexistence with other contracts, and pass-through outside scope.
- `openspec/specs/minimalism-contract/spec.md` — precedent for phase-scoped contract requirements and artifact-level verification patterns.

### Approaches

1. **Standalone skill** — create `skills/oo-quality-contract/SKILL.md` and register it as a normal custom skill.
   - Pros: Fits the existing skill registry shape; easy for agents to discover as a skill.
   - Cons: Too broad by default; risks matching all TypeScript or review work; conflicts with the decision not to inject into propose/spec; heavier than necessary for an advisory contract.
   - Effort: Medium

2. **Shared contract with deterministic scoped injection** — create `skills/_shared/oo-quality-contract.md`, track it in `overlay.manifest`, and extend the contract propagation/gate mechanism with a distinct `oo-quality-contract-scope` block and phase frontmatter.
   - Pros: Matches the minimalism-derived pattern; keeps the artifact compact; supports phase gating; avoids treating advisory OO heuristics as universal skill law.
   - Cons: Requires small engine/test changes if deterministic propagation is required; current status checks are still minimalism-centric and may need careful extension.
   - Effort: Medium

3. **Documentation only** — document the guidance in OpenSpec or project docs without runtime injection.
   - Pros: Lowest implementation cost; no runtime risk.
   - Cons: Agents will not reliably see it during design/apply/review work; does not answer the requested overlay deployment question; likely repeats the original problem as passive documentation.
   - Effort: Low

### Recommendation

Use Approach 2: a compact shared contract at `skills/_shared/oo-quality-contract.md`, tracked in `overlay.manifest`, with phase-scoped injection controlled by frontmatter and engine wiring. Do not create a normal `SKILL.md` and do not inject it into `sdd-propose` or `sdd-spec`.

Recommended frontmatter shape:

```yaml
---
applies_to_phases: [sdd-design, sdd-apply, sdd-tasks]
excluded_phases: [sdd-propose, sdd-spec, sdd-archive]
injection_point: "## Skills to load before work"
language_context: [typescript, nestjs]
activation_context: [oo-domain-design, domain-heavy-application-code, review]
---
```

Treat `sdd-verify` as out of scope for the first slice unless a later spec adds an explicit advisory-warning mode. If included later, it should only warn when the contract was explicitly configured or loaded; it must not become a global SOLID gate.

High-level contract content should say:

- Apply only when the work is OO/domain-heavy TypeScript or NestJS; otherwise pass through silently.
- Specs, design artifacts, project conventions, minimalism, and review budget outrank this contract.
- Prefer simple procedural/data-oriented code when the domain does not need OO seams.
- Use SOLID as diagnostic vocabulary, not as mandatory ceremony.
- Add abstractions only for active variation, explicit domain invariants, or tested seams.
- Value objects/entities are optional tools, not defaults; introduce them only when they protect invariants or reduce duplication.
- TDD is preferred when configured by the project or task, but the contract must not impose unconditional TDD beyond existing SDD/testing rules.
- Design patterns must be named only when they clarify a current design force; no pattern shopping.
- During review, report issues as advisory warnings tied to concrete risk, not as style violations.

### Risks

- Medium: Over-injection could make OO/SOLID guidance leak into non-TypeScript or non-domain work, conflicting with minimalism.
- Medium: A normal skill registry entry could over-match and behave like global dogma.
- Medium: Adding another embedded contract can duplicate minimalism/skill-discovery-safety mechanics unless the implementation uses a generic multi-contract path.
- Low: `.atl/skill-registry.md` may drift because it is generated/index-like; later phases should avoid treating hand edits as durable truth.

### Acceptance Criteria and Tests for Later Phases

- The contract file exists at `skills/_shared/oo-quality-contract.md`, is concise, English-only, and does not copy external `solid-skills` text wholesale.
- `overlay.manifest` contains `_shared/oo-quality-contract.md custom`; the file deploys with overlay-managed shared artifacts.
- The contract frontmatter declares explicit included and excluded SDD phases and a `## Skills to load before work` injection point.
- `sdd-propose` and `sdd-spec` prompts never receive `oo-quality-contract.md`.
- `sdd-design`, `sdd-apply`, and optionally `sdd-tasks` receive the contract path only when the configured scope applies.
- `sdd-verify` is excluded unless a later spec explicitly defines advisory-only warning behavior.
- Tests prove the new contract uses a distinct marker pair and row label, coexists with minimalism and skill-discovery-safety, is idempotent, and strips/passes through on excluded phases.
- Tests prove broken frontmatter fails safe and does not block sub-agent launch.
- Validation includes `cd engine && go test ./...`, `cd tui && go test ./...`, and relevant manifest/registry validation commands if touched.

### Ready for Proposal

Yes. The proposal should frame this as a local shared contract, not as an imported external skill. The next phase should decide whether to implement deterministic engine propagation in the first slice or begin with manifest-tracked contract plus artifact-level prompt observation.
