# Delta for Runtime Lifecycle

## MODIFIED Requirements

### Requirement: Target Aggregation

The system MUST include Codex and Pi in `--target all` aggregation as real
runtime targets, and MUST NOT mask Claude, OpenCode, or Codex failures
behind Codex or Pi results.
(Previously: covered only Codex inclusion and masking against Claude/OpenCode.)

#### Scenario: Target all includes Codex support

- GIVEN Claude, OpenCode, and Codex lifecycle actions succeed
- WHEN the user runs a runtime action with `--target all`
- THEN the command succeeds with all three target results represented

#### Scenario: Target all preserves non-Codex failures

- GIVEN Codex succeeds or reports an honest partial state
- AND Claude or OpenCode fails
- WHEN the user runs a runtime action with `--target all`
- THEN the overall command fails and reports the failing target

#### Scenario: Target all includes Pi support

- GIVEN Claude, OpenCode, Codex, and Pi lifecycle actions succeed or report honest partial state
- WHEN the user runs a runtime action with `--target all`
- THEN the command succeeds with all four target results represented

#### Scenario: Target all preserves non-Pi failures

- GIVEN Pi succeeds or reports an honest partial state
- AND Claude, OpenCode, or Codex fails
- WHEN the user runs a runtime action with `--target all`
- THEN the overall command fails and reports the failing non-Pi target

### Requirement: Existing Runtime Behavior Non-Regression

The system MUST preserve legacy Claude hook/settings commands, Claude
runtime lifecycle behavior, current OpenCode runtime lifecycle behavior, and
current Codex runtime lifecycle behavior while adding Pi lifecycle support.
(Previously: covered non-regression while adding Codex support only.)

#### Scenario: Legacy Claude commands still work

- GIVEN an existing legacy Claude hook/settings command is invoked
- WHEN the command executes
- THEN its documented behavior is preserved

#### Scenario: OpenCode lifecycle remains unchanged

- GIVEN OpenCode runtime lifecycle is installed or managed
- WHEN OpenCode status, install, update, or uninstall runs
- THEN behavior remains compatible with the existing OpenCode contract

#### Scenario: Codex lifecycle remains unchanged

- GIVEN Codex runtime lifecycle is installed or managed
- WHEN Codex status, install, update, or uninstall runs
- THEN behavior remains compatible with the existing Codex contract, unaffected by Pi target support

## ADDED Requirements

### Requirement: Pi Target Validation and Dispatch Without a Per-File Copy Path

Pi has no per-file copy path: adding the Pi target MUST NOT add entries to
`TARGET_PATHS` or `AGENT_TARGET_PATHS`. Target validation and `--target all`
aggregation MUST accept `pi`, dispatching `apply`, `status`, and
`sync-check` to the Pi package-build/install path instead of a file copy.
Every call site keyed on `TARGET_PATHS` membership MUST handle a non-copy
target explicitly rather than treating a missing entry as an empty path to
`mkdir`. The `Target` enum (`engine/runtime/runtime.go`), `AllTargets`, and
`NewFoundationAdapter` MUST include Pi.

#### Scenario: Pi passes target validation without a copy-path entry

- GIVEN `pi` is not present in `TARGET_PATHS` or `AGENT_TARGET_PATHS`
- WHEN target validation runs for `--target pi`
- THEN validation accepts `pi` and dispatches to the Pi package path
- AND no call site keyed on `TARGET_PATHS` membership attempts to `mkdir` an empty path for `pi`

#### Scenario: Pi is present in the enum, aggregation, and adapter construction

- GIVEN the Pi target has been added
- WHEN the `Target` enum, `AllTargets`, and `NewFoundationAdapter` are each inspected
- THEN all three recognize `pi` and construct or report a `PiAdapter`
