# Delta for Runtime Lifecycle

## ADDED Requirements

### Requirement: Multi-Contract Runtime Model

The runtime MUST support evaluating multiple managed guidance contracts without assuming that every contract is phase-only. Each contract decision SHALL consider the contract's phase scope and, when declared, explicit context requirements before injecting guidance.

#### Scenario: Multiple contracts are evaluated independently

- GIVEN minimalism, skill-discovery-safety, and OO quality contracts are configured
- WHEN runtime guidance is evaluated for a supported phase
- THEN each contract receives an independent decision
- AND one contract's inclusion or exclusion does not force another contract's decision

#### Scenario: Unknown or malformed contract data passes through

- GIVEN a contract has malformed metadata or an unsupported context rule
- WHEN runtime guidance is evaluated
- THEN the runtime MUST leave the target prompt unchanged for that contract
- AND other valid contracts remain eligible for evaluation

### Requirement: Explicit OO Context Gate

OO quality guidance MUST NOT be injected from phase scope alone. The runtime SHALL inject OO quality guidance only when the phase matches and trusted work-context metadata explicitly proves OO/domain-heavy TypeScript or NestJS application-code scope.

#### Scenario: Scoped TypeScript domain work receives OO guidance

- GIVEN trusted work-context metadata identifies OO/domain-heavy TypeScript or NestJS application-code work
- AND the current phase is included by the OO quality contract
- WHEN runtime guidance is evaluated
- THEN OO quality guidance is injected exactly once

#### Scenario: Non-domain work passes through in included phases

- GIVEN trusted work-context metadata identifies Go, docs, config, generated artifacts, or non-domain work
- AND the current phase is included by the OO quality contract
- WHEN runtime guidance is evaluated
- THEN OO quality guidance MUST NOT be injected

#### Scenario: Missing context metadata is not enough

- GIVEN no trusted work-context metadata is available
- AND the current phase is included by the OO quality contract
- WHEN runtime guidance is evaluated
- THEN OO quality guidance MUST NOT be injected

#### Scenario: Prompt text is not used as proof

- GIVEN prompt text mentions TypeScript, NestJS, SOLID, or domain modeling
- AND trusted work-context metadata is absent or insufficient
- WHEN runtime guidance is evaluated
- THEN OO quality guidance MUST NOT be injected

### Requirement: Runtime Contract Non-Regression

Adding context-aware OO contract support MUST preserve existing minimalism, skill-discovery-safety, Claude lifecycle, and OpenCode lifecycle behavior unless a later spec explicitly changes those contracts.

#### Scenario: Existing phase-only contracts remain stable

- GIVEN a runtime prompt that currently receives minimalism or skill-discovery-safety guidance
- WHEN multi-contract evaluation is introduced
- THEN the existing guidance behavior remains unchanged

#### Scenario: Runtime lifecycle commands remain compatible

- GIVEN Claude or OpenCode runtime lifecycle commands are invoked
- WHEN context-aware OO contract support is present
- THEN existing install, update, uninstall, and status behavior remains compatible
