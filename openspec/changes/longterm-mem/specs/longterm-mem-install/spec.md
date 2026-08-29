# Longterm-Mem Install Specification

## Purpose

Defines the overlay-side install/status/uninstall dispatch for longterm-mem:
building and placing the binary, delegating registration reporting to the
runtime lifecycle surface, and the persistent-binary-path property those
dispatches rely on.

## Requirements

### Requirement: Install Builds, Copies, and Registers

ID: R-014
Traces to: longterm-mem R-014

WHEN longterm-mem install is invoked for a target, the overlay entrypoint
SHALL build the longterm-mem binary and copy it to the fixed install path,
then invoke the runtime lifecycle install action for longterm-mem to record
the registration.

#### Scenario: Install builds, copies, then reports per-runtime status

- GIVEN longterm-mem install is invoked for all targets
- WHEN it executes
- THEN a binary exists at the fixed install path afterward, and a
  per-runtime status (supported, partial, unsupported, or restart-required)
  is reported for Claude Code, opencode, and codex

#### Scenario: Status and uninstall skip the build step

- GIVEN longterm-mem status or longterm-mem uninstall is invoked
- WHEN it executes
- THEN the overlay entrypoint calls the corresponding runtime lifecycle
  status or uninstall action directly, without a build step

### Requirement: Persistent Binary Install Path

ID: R-015
Traces to: longterm-mem R-015

The longterm-mem binary SHALL reside at one fixed, documented, persistent
path after install, remaining present and invocable until an uninstall
removes it.

#### Scenario: Binary persists after the installing process exits

- GIVEN a fresh install
- WHEN the installing process exits
- THEN the binary still exists at the documented fixed path and is
  invocable

#### Scenario: Binary path is stable absent install/uninstall activity

- GIVEN no install or uninstall is in progress
- WHEN the system is inspected at any later time
- THEN the binary's path is unchanged from the one install placed it at
