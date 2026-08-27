# Overlay Release Surfacing Specification

## Purpose

The user-facing surface of the versioned-release model: a `version` command,
a `doctor` consistency row, and TUI rendering of per-target version/digest/
backup state with a restore action. Traces to overlay-versioned-releases
R-009, R-010, R-011.

## Requirements

### Requirement: doctor reports a per-target version/digest consistency row

ID: R-001
Traces to: overlay-versioned-releases R-009 (WARN, not FAIL — settled by proposal.md "doctor WARN row")

WHERE a target's persisted state records a version, the `doctor` command
SHALL print a `WARN` row naming that target and recommending `apply` WHEN its
live aggregate digest does not match its recorded version's expected digest,
and SHALL print an `[ok]`-equivalent row WHEN it matches. A digest mismatch
SHALL NOT cause `doctor` to exit non-zero, preserving its existing "exits
non-zero only on a hard FAIL" contract.

#### Scenario: In-sync target passes

- GIVEN target `claude` is in sync with its recorded version
- WHEN `overlay doctor` runs
- THEN it prints a passing row for `claude`'s asset-version check

#### Scenario: Drifted target warns, does not fail the exit code

- GIVEN target `claude`'s live digest does not match its recorded version
- WHEN `overlay doctor` runs
- THEN it prints a `WARN` row naming `claude` and recommending `apply`, and `doctor`'s exit code stays 0 (not a hard FAIL) if no other hard-FAIL condition exists

#### Scenario: Existing host-toolchain checks unaffected

- GIVEN `doctor`'s existing checks (go, gentle-ai, discovery tools, engine binary, skill-registry)
- WHEN the new per-target row is added
- THEN those existing checks still run and report exactly as before (regression-free)

### Requirement: version command reports installed vs. latest per target

ID: R-002
Traces to: overlay-versioned-releases R-010

The system SHALL provide a `version` command (and `--version` flag) that
prints the repository's current release version and, per target, its
recorded deployed version. A target with no recorded state SHALL be printed
as "never deployed" rather than a stale or fabricated version.

#### Scenario: Behind-target reported by name

- GIVEN local `main` is at `v1.4.0` and target `claude` is deployed at `v1.3.0`
- WHEN `overlay version` runs
- THEN it prints the repo version `v1.4.0` and `claude: v1.3.0 (behind)`

#### Scenario: Never-deployed target

- GIVEN a target has no recorded state
- WHEN `overlay version` runs
- THEN it prints that target as "never deployed"

### Requirement: TUI surfaces version, digest-match, and a confirmed restore action

ID: R-003
Traces to: overlay-versioned-releases R-011

WHILE the TUI's status view is displayed, it SHALL show each target's
recorded release version and digest-match status, matching `overlay
version`'s output. WHEN a target has an available backup, the TUI SHALL make
a `restore` action selectable for it, routed through the existing
confirm→run→result screens with `Mutating: true` and a `ConfirmMessage`
stating that restore overwrites the target's currently deployed files,
following the same confirmation pattern already established for
`apply`/`self-update`.

#### Scenario: Version and behind-indicator shown

- GIVEN target `claude` is behind its latest release
- WHEN the TUI status view renders
- THEN it displays `claude`'s recorded version and a "behind" indicator matching `overlay version`'s output

#### Scenario: Restore action selectable and confirmed before running

- GIVEN target `claude` has an available backup
- WHEN the TUI status view renders and the user selects `claude`'s restore action
- THEN the action is selectable, `screenConfirm` shows `Mutating: true` and a `ConfirmMessage` naming that restore overwrites `claude`'s deployed files, and the restore only runs after explicit confirmation

## Open Questions for Design

- **Exact TUI layout/keybinding** for the new restore action is flagged as a
  design-phase detail in the requirements brief; not fixed here.
