# OO Quality Contract Specification

## Purpose

Define a local, advisory OO quality contract for domain-heavy TypeScript/NestJS work without making SOLID guidance global SDD policy.

## Requirements

### Requirement: Local Shared Contract

ID: R-001

When the OO quality contract is introduced, the system SHALL provide a concise English-only shared contract at `skills/_shared/oo-quality-contract.md` that does not vendor or copy external `solid-skills` text wholesale.

#### Scenario: Contract exists locally

- GIVEN the change artifacts are inspected
- WHEN `skills/_shared/oo-quality-contract.md` is read
- THEN it contains concise English guidance
- AND it is not a wholesale copy of external `solid-skills` content

### Requirement: Manifest Tracking

ID: R-002

When the shared contract is added, the system SHALL track it in `overlay.manifest` as `_shared/oo-quality-contract.md custom`.

#### Scenario: Manifest contains contract row

- GIVEN overlay-managed artifacts are inspected
- WHEN `overlay.manifest` is read
- THEN it includes `_shared/oo-quality-contract.md custom`

### Requirement: Phase Scope

ID: R-003

When contract frontmatter is declared, the system SHALL include `sdd-design`, `sdd-tasks`, and `sdd-apply` while excluding `sdd-propose`, `sdd-spec`, `sdd-archive`, and first-slice `sdd-verify`.

#### Scenario: Included and excluded phases are explicit

- GIVEN the contract frontmatter is parsed
- WHEN phase scope is evaluated
- THEN included phases are design, tasks, and apply
- AND propose, spec, archive, and verify are excluded

### Requirement: Precedence

ID: R-004

When OO guidance conflicts with governing artifacts, the system SHALL treat specs, design, project conventions, the minimalism contract, and review budget as higher precedence.

#### Scenario: Higher-precedence artifact wins

- GIVEN OO guidance suggests an abstraction
- WHEN specs, design, conventions, minimalism, or review budget disagree
- THEN the higher-precedence artifact determines the accepted behavior

### Requirement: Context Gate

ID: R-005

When work is outside OO/domain-heavy TypeScript, NestJS, or application-code scope, the system SHALL pass through without applying OO guidance.

#### Scenario: Non-domain work passes through

- GIVEN work targets Go scripts, docs, config, generated artifacts, or non-domain changes
- WHEN the contract scope is evaluated
- THEN OO guidance is not applied

### Requirement: Advisory OO Guidance

ID: R-006

When OO guidance applies, the system SHALL present SOLID as diagnostic vocabulary and allow value objects, patterns, and abstractions only for invariants, active variation, or tested seams.

#### Scenario: Abstraction requires justification

- GIVEN domain-heavy TypeScript work is being designed or implemented
- WHEN an abstraction, pattern, or value object is proposed
- THEN it is justified by an invariant, active variation, or tested seam

### Requirement: TDD Configuration Respect

ID: R-007

When testing guidance is referenced, the system SHALL require TDD only when the project or task configuration requires it.

#### Scenario: TDD is not imposed globally

- GIVEN a task has no TDD requirement in its project or task config
- WHEN the OO contract is applied
- THEN the contract does not add an unconditional TDD mandate

### Requirement: First-Slice Boundaries and Validation

ID: R-008

When this first slice is delivered, the system SHALL avoid engine propagation or gate wiring and verify behavior with artifact-level tests or validation.

#### Scenario: No engine wiring in first slice

- GIVEN the first slice implementation is reviewed
- WHEN changed files are inspected
- THEN no engine propagation or gate wiring is added for this contract
- AND artifact-level validation covers path, manifest, scope, precedence, and non-vendoring
