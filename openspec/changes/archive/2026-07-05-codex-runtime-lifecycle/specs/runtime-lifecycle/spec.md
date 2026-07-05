# Delta for Runtime Lifecycle

## ADDED Requirements

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

## MODIFIED Requirements

### Requirement: Existing Runtime Behavior Non-Regression

The system MUST preserve legacy Claude hook/settings commands, Claude runtime lifecycle behavior, and current OpenCode runtime lifecycle behavior while adding Codex lifecycle support.
(Previously: preserved legacy Claude commands and OpenCode lifecycle behavior only.)

#### Scenario: Legacy Claude commands still work

- GIVEN an existing legacy Claude hook/settings command is invoked
- WHEN the command executes
- THEN its documented behavior is preserved

#### Scenario: OpenCode lifecycle remains unchanged

- GIVEN OpenCode runtime lifecycle is installed or managed
- WHEN OpenCode status, install, update, or uninstall runs
- THEN behavior remains compatible with the existing OpenCode contract

#### Scenario: Claude lifecycle remains unchanged

- GIVEN Claude runtime lifecycle is installed or managed
- WHEN Claude status, install, update, or uninstall runs
- THEN behavior remains compatible with the existing Claude contract

### Requirement: Transitional Target Aggregation

The system MUST include Codex in `--target all` aggregation as a real runtime target after Codex support, and MUST NOT mask Claude or OpenCode failures behind Codex results.
(Previously: `--target all` succeeded with an advisory when Codex remained temporarily unsupported.)

#### Scenario: Target all includes Codex support

- GIVEN Claude, OpenCode, and Codex lifecycle actions succeed
- WHEN the user runs a runtime action with `--target all`
- THEN the command succeeds with all three target results represented

#### Scenario: Target all preserves non-Codex failures

- GIVEN Codex succeeds or reports an honest partial state
- AND Claude or OpenCode fails
- WHEN the user runs a runtime action with `--target all`
- THEN the overall command fails and reports the failing target
