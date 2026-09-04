# Longterm-Mem Ops Specification

## Purpose

Defines the operability surface of `longterm-mem`: a health-check `status`,
a diagnostic `doctor` with five named checks, the MCP stdio server exposing
`query` and `promote`, and the no-persistent-process constraint that governs
both the server and every CLI subcommand.

## Requirements

### Requirement: Status Reporting

ID: R-010
Traces to: longterm-mem R-010

WHEN `status` is requested for project P, the longterm-mem component SHALL
report Engram reachability, P's vault provisioning state, and the last
successful sync completion time.

#### Scenario: Healthy status reports all three fields

- GIVEN Engram is reachable, the vault is provisioned, and a prior sync has
  been recorded
- WHEN status is requested
- THEN all three are reported healthy plus the recorded timestamp

#### Scenario: Never-provisioned vault is reported, not an error

- GIVEN the vault has never been provisioned
- WHEN status is requested
- THEN the vault is reported as not provisioned instead of erroring

#### Scenario: Never-synced project reports never, not a fabricated timestamp

- GIVEN no sync has ever completed for P
- WHEN status is requested
- THEN the last-sync field reports never rather than a stale or fabricated
  timestamp

### Requirement: Diagnostic Checks

ID: R-011
Traces to: longterm-mem R-011

WHEN doctor is requested for project P, the longterm-mem component SHALL run
five read-only diagnostic checks — vault-config resolvability, address-map
integrity, wiki-registration consistency, precedence-sidecar consistency, and
runtime-prerequisite presence — and report each check's pass/fail state
individually.

A failed check is a verdict about the VAULT: the run completed, the report
names the check, and the operator repairs the vault. A failure of doctor's
own — an unresolvable vault registry, an unreportable result — is a verdict
about longterm-mem: no check ran, and there is nothing for the operator to
repair by changing configuration. The two SHALL therefore carry distinct
exit codes: exit 9 (doctor_checks_failed) WHEN every check ran and at least
one reported failed, and the component's generic internal code WHEN doctor
could not run or report its checks at all. Sharing one code made the two
indistinguishable to a caller acting on the code alone, which is the only
reason the code exists.

#### Scenario: Unresolvable vault config is named

- GIVEN P's vault-registry entry points at a non-existent path
- WHEN doctor runs
- THEN the vault-config-resolvable check reports failed with that path

#### Scenario: Corrupted address-map entry is named

- GIVEN a promoted page missing its address-map entry
- WHEN doctor runs
- THEN the address-map-integrity check reports that specific inconsistency

#### Scenario: Unregistered promoted page is named

- GIVEN a promoted page absent from the vault's catalog and log
- WHEN doctor runs
- THEN the wiki-registration-consistency check reports that specific
  inconsistency

#### Scenario: Missing runtime prerequisite is named

- GIVEN a required external runtime prerequisite is not present
- WHEN doctor runs
- THEN the runtime-prerequisites check reports it missing rather than
  letting a later call fail with a generic error

#### Scenario: A failed check exits with doctor's own code

- GIVEN doctor ran every check to completion and at least one reported
  failed
- WHEN the process exits
- THEN it exits 9, so a caller acting on the exit code alone knows the
  vault has a named, repairable problem rather than that doctor broke

#### Scenario: Doctor's own failure is not reported as a diagnostic verdict

- GIVEN doctor could not resolve P's vault registry, so no check ran
- WHEN the process exits
- THEN it exits with the generic internal code rather than 9, so a failure
  of longterm-mem's own is never mistaken for a finding about the vault

### Requirement: MCP Stdio Server Exposes Query and Promote

ID: R-012
Traces to: longterm-mem R-012

WHEN the longterm-mem MCP server is launched, the longterm-mem component
SHALL serve the `query` and `promote` tools over MCP stdio.

#### Scenario: Tool-listing handshake lists both tools

- GIVEN the longterm-mem MCP server is running
- WHEN an MCP client performs its tool-listing handshake
- THEN the response lists both `query` and `promote`

#### Scenario: Query round-trips over stdio

- GIVEN a connected client
- WHEN it calls `query` with a valid project and query string
- THEN it receives the grouped result list over the same stdio connection

#### Scenario: A frozen Engram snapshot is declared, not passed off as live

- GIVEN the server's Engram connection was established through the
  read-only immutable fallback, so it answers from a point-in-time snapshot
  taken when the session opened rather than from the live database
- WHEN a client calls `query`
- THEN the result carries a diagnostic naming the degraded snapshot and its
  cause, so the client can tell a live corpus from a session-long frozen
  one rather than reading an incomplete result as a complete one

### Requirement: No Persistent Daemon

ID: R-034
Traces to: longterm-mem R-034

The longterm-mem component SHALL NOT spawn any process that outlives its
invoking session.

#### Scenario: MCP server exits with its session

- GIVEN a runtime spawns the longterm-mem MCP server
- WHEN the session ends
- THEN the server process exits with the session and leaves no residual
  background process

#### Scenario: No CLI subcommand leaves a residual process

- GIVEN any longterm-mem CLI subcommand completes
- WHEN the process list is checked afterward
- THEN no longterm-mem background process remains running
