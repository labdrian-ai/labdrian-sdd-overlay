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

#### Scenario: The binary path follows the state directory it was given

- GIVEN the overlay entrypoint is run with an overridden state directory
- AND it has deployed the longterm-mem binary under that state directory,
  and registered MCP entries naming that same deployed path
- WHEN the runtime lifecycle status or install action runs for the
  `longterm-mem` component with that state directory
- THEN it resolves the binary under the state directory it was given, not
  under the default one
- AND the deployed binary is reported as present, and an entry naming it is
  recognised as one this overlay owns

#### Scenario: A state directory that is not already normalized still agrees

- GIVEN the overlay entrypoint is run with an overridden state directory whose
  path is not lexically normalized — a trailing separator, a duplicated
  separator, a `.` element, or a `..` segment
- AND it has deployed the longterm-mem binary under that state directory,
  and registered MCP entries naming that same deployed path
- WHEN the runtime lifecycle status or install action runs for the
  `longterm-mem` component with that state directory
- THEN the path written into each MCP entry and the path the component
  derives from the state directory are the SAME string, because ownership is
  decided by comparing entries byte for byte
- AND the entry the overlay itself just wrote is recognised as owned, not
  reported as an entry without a record

### Requirement: longterm-mem Status Reports Observed State, Not Assumed Intent

Traces to: longterm-mem R-014

No change-level `ID:` is claimed here on purpose. This rule was discovered
during delivery — the component's own "honest per-runtime status" clause above
turned out to be violated by the status matrix it was implemented with — so it
has no R-NNN of its own. `Traces to:` is the key that resolves it, as it is for
`Multi-Target Expansion Skips Runtimes That Are Not Installed` in
`longterm-mem-mcp-registration`.

A runtime that is not installed on the machine, and a runtime that is installed
but was never registered with `longterm-mem`, are both OBSERVED states with no
defect in them. The component MUST NOT report either as `partial`, and MUST NOT
let either drag the aggregate status below `supported`. It MUST still name the
two apart in its per-runtime reporting, since "the runtime is not here" and
"the runtime is here and unregistered" are different facts about the machine.

Which runtimes the operator actually asked for is the caller's knowledge, not
the engine's; the component MUST NOT infer intent from the absence of a
registration. This is the engine-side counterpart of
`longterm-mem-mcp-registration`'s rule that a `--target all` expansion skips an
absent runtime without failing the run on its account.

Whether a runtime is installed MUST be decided by that runtime's own
configuration file existing on disk. It MUST NOT be inferred from whether the
component could resolve a configuration root, which is a machine-wide fact
shared by all three runtimes, and a configuration file that exists but cannot
be read or parsed MUST NOT be reported as an absent runtime.

#### Scenario: An absent runtime does not fail the component's status

- GIVEN the `longterm-mem` component is installed and registered for the only
  runtime present on the machine
- AND the other two runtimes have no configuration file on disk
- WHEN the runtime lifecycle status action runs for the `longterm-mem`
  component
- THEN the aggregate status is healthy `supported`
- AND each absent runtime is reported as not installed rather than as a defect

#### Scenario: An installed but unregistered runtime is reported apart from an absent one

- GIVEN a runtime's configuration file exists on disk and carries no
  `longterm-mem` entry
- WHEN the runtime lifecycle status action runs for the `longterm-mem`
  component
- THEN that runtime is reported as not registered
- AND its reported reason differs from the one used for a runtime that is not
  installed at all

#### Scenario: An unreadable configuration file is not reported as an absent runtime

- GIVEN a runtime's configuration file exists on disk but cannot be parsed
- WHEN the runtime lifecycle status action runs for the `longterm-mem`
  component
- THEN that runtime MUST NOT be reported as not installed

### Requirement: longterm-mem Records Only MCP Entries It Owns

Traces to: longterm-mem R-014, R-016, R-017, R-018

No change-level `ID:` is claimed here either, for the reason given above.

The `longterm-mem` component's registration record MUST only ever record a
runtime whose MCP entry was actually observed AND is one this overlay wrote. It
MUST NOT record a runtime whose entry is absent, and it MUST NOT record an
entry it cannot prove it wrote.

An entry the component does not own MUST still be observed as PRESENT and
reported as an unmanaged entry. Reporting it as though nothing were registered
would hide a third party's configuration behind a healthy status.

This keeps the engine record from contradicting the module-owned `register`
step, which refuses to write over a same-named entry it does not own. It also
preserves drift detection in the other direction: a record written from a
genuinely observed entry MUST still report a defect once that entry later
disappears.

#### Scenario: An MCP entry the component does not own is never recorded

- GIVEN a runtime's configuration file carries a `longterm-mem` MCP entry this
  overlay did not write
- WHEN the runtime lifecycle install action runs for the `longterm-mem`
  component
- THEN no registration record is written for that runtime
- AND that runtime is reported as carrying an unmanaged entry, not as
  unregistered and not as healthy `supported`

#### Scenario: A recorded entry that later disappears is still reported as a defect

- GIVEN the `longterm-mem` component recorded a runtime from an entry it owned
- AND that entry is later removed from the runtime's own configuration
- WHEN the runtime lifecycle status action runs for the `longterm-mem`
  component
- THEN that runtime is reported as `partial` with a record but no entry

### Requirement: longterm-mem Uninstall Reports Its Own Verdict

Traces to: longterm-mem R-014

No change-level `ID:` is claimed here either, for the reason given above.

The `longterm-mem` component's uninstall action MUST report success when the
registration record it owns was removed, or was already absent — both mean the
requested end state holds. It MUST report failure only when the state directory
cannot be resolved or the removal itself failed.

Uninstall MUST NOT derive its result from the install-health status matrix. That
matrix answers "is this component installed and healthy", whose healthy outcome
describes a live installation, so deriving uninstall's result from it makes a
flawless uninstall structurally incapable of reporting success.

Uninstall MAY report per-runtime observations of what each runtime's own
configuration still holds. Those observations are information about the machine,
not part of the verdict: removing an MCP entry from a runtime's own
configuration belongs to the module-owned `unregister` step, never to this
component.

This constrains only uninstall's own result. What a SUBSEQUENT status reports
remains governed by the status requirements above.

#### Scenario: Uninstall reports success when it removed what it owns

- GIVEN the `longterm-mem` component has a registration record
- WHEN the runtime lifecycle uninstall action runs for it
- THEN the registration record is removed
- AND the action reports healthy `supported`

#### Scenario: Uninstall reports success when there was nothing to remove

- GIVEN the `longterm-mem` component has no registration record
- WHEN the runtime lifecycle uninstall action runs for it
- THEN the action reports healthy `supported`

#### Scenario: Uninstall reports failure when the removal could not be performed

- GIVEN the `longterm-mem` component's state directory cannot be resolved
- WHEN the runtime lifecycle uninstall action runs for it
- THEN the action reports `unsupported`
