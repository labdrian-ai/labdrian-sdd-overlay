# Delta for Runtime Lifecycle

## ADDED Requirements

### Requirement: Adapter Coverage Over The Active Roster Is Total And Explicit

ID: R-141
Traces to: labdrian-sdd-overlay R-141

Lifecycle adapter construction for an active roster runtime MUST resolve
through an explicit registry keyed by that runtime, and that registry's key
set MUST equal the active roster exactly. The inert fallback adapter MUST
remain reachable only for a target that is not an active roster name.

#### Scenario: Every active runtime resolves to its own adapter

- GIVEN the active roster
- WHEN an adapter is constructed for each active runtime
- THEN each returns that runtime's own adapter type
- AND none returns the inert fallback

#### Scenario: A roster entry without an adapter fails at test time, not at run time

- GIVEN a runtime is marked `active` in the roster and no adapter constructor
  is registered for it
- WHEN the test suite runs
- THEN it fails naming that runtime and the missing registration
- AND the failure occurs before any lifecycle action is executed

#### Scenario: The fallback still answers for a genuinely unknown target

- GIVEN a target that is not an active roster name
- WHEN an adapter is constructed for it
- THEN the inert fallback is returned and reports `unsupported` for every
  action, as it does today
