---
applies_to_phases: [sdd-design, sdd-tasks, sdd-apply]
excluded_phases: [sdd-propose, sdd-spec, sdd-archive, sdd-verify]
injection_point: "## Skills to load before work"
language_context: [typescript, nestjs]
activation_context: [oo-domain-design, domain-heavy-application-code, review]
---
# OO Quality Contract

> Advisory scope: apply this contract only to OO/domain-heavy TypeScript or NestJS
> application work. It is local guidance, not a global architecture rule.

## Precedence

Specs and specs-derived requirements, design, project conventions, the minimalism contract, and review budget win over this contract.
If this contract suggests an abstraction but a higher-precedence artifact disagrees, follow the higher-precedence artifact.

## Context gate

For Go, shell, docs, config, generated artifacts, non-domain work, and non-OO changes, pass through without OO guidance.
Do not retrofit classes, patterns, or value objects into simple data or procedural work.

## Advisory guidance

Use SOLID as diagnostic vocabulary, not as a checklist. Prefer simple code until domain invariants, active variation, or tested seams justify an abstraction.

Allow entities, value objects, policies, ports, adapters, and patterns only when they protect invariants, isolate active variation, or make a tested seam clearer.

## Testing boundary

Do not impose TDD from this contract. Require TDD only when the project or task config already requires it.

## Review posture

Report advisory warnings tied to concrete risk: unclear ownership, unstable dependency direction, duplicated domain rules, hidden side effects, or abstractions without current justification.
