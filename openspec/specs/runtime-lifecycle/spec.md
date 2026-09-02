# Runtime Lifecycle Specification

## Purpose

Define CLI runtime lifecycle behavior for Claude, OpenCode, and Codex targets, including safe configuration mutation, target aggregation, and compatibility boundaries.

## Requirements

### Requirement: Claude Lifecycle Support

The system MUST support Claude runtime `status`, `install`, `update`, and `uninstall` through the runtime lifecycle command surface.

#### Scenario: Claude install succeeds

- GIVEN Claude settings are writable
- WHEN the user runs runtime install for target `claude`
- THEN the command succeeds
- AND Claude status becomes provably healthy as `supported`

#### Scenario: Claude status is honest

- GIVEN Claude lifecycle state cannot be proven installed and healthy
- WHEN the user runs runtime status for target `claude`
- THEN the command MUST NOT report healthy `supported`

#### Scenario: Claude update refreshes lifecycle state

- GIVEN Claude is already installed by Labdrian
- WHEN the user runs runtime update for target `claude`
- THEN the command succeeds
- AND Claude remains provably healthy as `supported`

#### Scenario: Claude uninstall removes owned lifecycle state

- GIVEN Claude has Labdrian-owned lifecycle entries installed
- WHEN the user runs runtime uninstall for target `claude`
- THEN the command succeeds
- AND subsequent Claude status is not healthy `supported`

### Requirement: Claude Config Root Selection

Claude runtime commands MUST use the real Claude Code settings location by default and SHALL use `--config-root` only when explicitly provided.

#### Scenario: Default root uses real settings

- GIVEN no `--config-root` is provided
- WHEN Claude lifecycle action runs
- THEN the action targets the default Claude Code settings location

#### Scenario: Explicit config root isolates operation

- GIVEN `--config-root` points to a sandbox root
- WHEN Claude lifecycle action runs
- THEN the action mutates only that sandbox root

### Requirement: Labdrian-Owned Mutation Boundary

Claude lifecycle commands MUST mutate only Labdrian-owned hooks and settings entries and MUST NOT rewrite unrelated user configuration.

#### Scenario: User settings are preserved

- GIVEN settings contain Labdrian entries and unrelated user entries
- WHEN install, update, or uninstall runs for `claude`
- THEN unrelated user entries remain semantically unchanged

### Requirement: Safe Backup and Rollback

Configuration writes MUST use safe merge and backup semantics, and failures SHALL leave a recoverable previous settings state.

#### Scenario: Backup supports rollback

- GIVEN a writable Claude settings file exists
- WHEN Claude lifecycle mutation succeeds
- THEN a recoverable backup of the prior settings state is available

#### Scenario: Failed mutation preserves prior state

- GIVEN Claude settings cannot be safely written
- WHEN Claude lifecycle mutation is attempted
- THEN the command fails
- AND the previous settings state remains usable

### Requirement: Codex Root Resolution

Codex lifecycle commands MUST resolve the Codex root from `$CODEX_HOME` when set, and MUST otherwise use `~/.codex`.

#### Scenario: CODEX_HOME selects root

- GIVEN `$CODEX_HOME` is set to a sandbox path
- WHEN a Codex lifecycle action runs
- THEN the action targets that sandbox path

#### Scenario: Default root is used

- GIVEN `$CODEX_HOME` is unset
- WHEN a Codex lifecycle action runs
- THEN the action targets `~/.codex`

### Requirement: Codex Lifecycle Support

The system MUST support Codex runtime `status`, `install`, `update`, and `uninstall` where Codex-native state can be proven.

#### Scenario: Codex install succeeds

- GIVEN the resolved Codex root is writable
- WHEN the user runs runtime install for target `codex`
- THEN the command succeeds
- AND Codex-owned lifecycle state exists under the resolved root

#### Scenario: Codex status avoids false green

- GIVEN installed, configured, or activation state cannot be proven
- WHEN the user runs runtime status for target `codex`
- THEN the command MUST NOT report healthy `supported`

#### Scenario: Codex update refreshes owned state

- GIVEN Codex has Labdrian-owned lifecycle state installed
- WHEN the user runs runtime update for target `codex`
- THEN owned lifecycle state is refreshed without requiring unrelated config changes

#### Scenario: Codex uninstall removes owned state

- GIVEN Codex has Labdrian-owned lifecycle state installed
- WHEN the user runs runtime uninstall for target `codex`
- THEN Labdrian-owned lifecycle state is removed
- AND unrelated Codex configuration remains present

### Requirement: Codex Owned-Only Mutation Boundary

Codex lifecycle commands MUST mutate only Labdrian-owned artifacts or entries and MUST NOT rewrite unrelated Codex user configuration.

#### Scenario: User Codex config is preserved

- GIVEN the Codex root contains Labdrian-owned state and unrelated user config
- WHEN install, update, or uninstall runs for `codex`
- THEN unrelated user config remains semantically unchanged

#### Scenario: Failed Codex mutation is safe

- GIVEN Codex lifecycle state cannot be safely written or removed
- WHEN a Codex mutation is attempted
- THEN the command fails without reporting success
- AND previously usable unrelated Codex state remains usable

### Requirement: Codex Activation Honesty

Codex status MUST surface uncertainty when activation or reload cannot be proven, and MUST NOT hide uncertainty behind a healthy state.

#### Scenario: Reload proof is unavailable

- GIVEN Codex owned state is present but activation or reload cannot be verified
- WHEN the user runs runtime status for target `codex`
- THEN status reports `partial` with an explicit reason indicating activation/reload proof is unavailable

### Requirement: Codex status is honest by default

Codex status must never return `supported` when activation/reload state cannot be proven; it must return `partial` with uncertainty surfacing and actionable follow-up text.

#### Scenario: Codex manifest exists but session lifecycle proof is missing

- GIVEN the Codex manifest is present but runtime session lifecycle activation cannot be proven
- WHEN the user runs runtime status for target `codex`
- THEN status reports `partial`, not `supported`
- AND status output includes the uncertainty reason text

### Requirement: Existing Runtime Behavior Non-Regression

The system MUST preserve legacy Claude hook/settings commands, Claude runtime lifecycle behavior, and current OpenCode runtime lifecycle behavior while adding Codex lifecycle support.

#### Scenario: Legacy Claude commands still work

- GIVEN an existing legacy Claude hook/settings command is invoked
- WHEN the command executes
- THEN its documented behavior is preserved

#### Scenario: OpenCode lifecycle remains unchanged

- GIVEN OpenCode runtime lifecycle is installed or managed
- WHEN OpenCode status, install, update, or uninstall runs
- THEN behavior remains compatible with the existing OpenCode contract

### Requirement: Target Aggregation

The system MUST include Codex in `--target all` aggregation as a real runtime target after Codex support, and MUST NOT mask Claude or OpenCode failures behind Codex results.

#### Scenario: Target all includes Codex support

- GIVEN Claude, OpenCode, and Codex lifecycle actions succeed
- WHEN the user runs a runtime action with `--target all`
- THEN the command succeeds with all three target results represented

#### Scenario: Target all preserves non-Codex failures

- GIVEN Codex succeeds or reports an honest partial state
- AND Claude or OpenCode fails
- WHEN the user runs a runtime action with `--target all`
- THEN the overall command fails and reports the failing target

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

### Requirement: longterm-mem Component Registration

Traces to: longterm-mem R-014

The runtime lifecycle command surface MUST support a `longterm-mem`
component for `install`, `status`, and `uninstall` — recording its
registration and reporting an honest per-runtime status (`supported`,
`partial`, `unsupported`, or `restart_required`) for Claude Code, opencode,
and codex — without offering `update` or `rollback` for that component.

#### Scenario: Install records registration and reports per-runtime status

- GIVEN the overlay entrypoint has built and copied the longterm-mem binary
- WHEN it invokes the runtime lifecycle install action for the
  `longterm-mem` component
- THEN the registration is recorded and a per-runtime status is reported for
  Claude Code, opencode, and codex

#### Scenario: Status and uninstall report without requiring a build

- GIVEN the `longterm-mem` component is already registered
- WHEN the runtime lifecycle status or uninstall action runs for it
- THEN it reports or removes the registration directly, without any build
  step

#### Scenario: No update or rollback surface is offered

- GIVEN the `longterm-mem` component is registered
- WHEN the runtime lifecycle command surface is inspected for available
  actions on that component
- THEN no `update` or `rollback` action is offered for it
