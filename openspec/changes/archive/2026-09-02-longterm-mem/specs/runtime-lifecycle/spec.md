# Delta for Runtime Lifecycle

## ADDED Requirements

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
