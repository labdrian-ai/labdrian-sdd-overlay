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

### Requirement: Existing Runtime Behavior Non-Regression

The system MUST preserve legacy Claude hook/settings commands and current OpenCode runtime lifecycle behavior.

#### Scenario: Legacy Claude commands still work

- GIVEN an existing legacy Claude hook/settings command is invoked
- WHEN the command executes
- THEN its documented behavior is preserved

#### Scenario: OpenCode lifecycle remains unchanged

- GIVEN OpenCode runtime lifecycle is installed or managed
- WHEN OpenCode status, install, update, or uninstall runs
- THEN behavior remains compatible with the existing OpenCode contract

### Requirement: Transitional Target Aggregation

The system MUST keep `--target all` successful with an advisory when Claude and OpenCode succeed while Codex remains temporarily unsupported.

#### Scenario: Target all warns for Codex transition

- GIVEN Claude and OpenCode lifecycle actions succeed
- AND Codex lifecycle remains unsupported in this SDD
- WHEN the user runs a runtime action with `--target all`
- THEN the command succeeds with a warning or advisory for Codex
