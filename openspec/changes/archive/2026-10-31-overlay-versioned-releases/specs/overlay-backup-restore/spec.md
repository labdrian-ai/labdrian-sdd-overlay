# Overlay Backup and Restore Specification

## Purpose

Gives every mutating deploy a rollback point: `apply` snapshots a target's
pre-change managed files into a timestamped backup before overwriting them,
capped at the 3 most recent per target (auto-pruned), and `restore` rolls a
target back from its most recent backup. Traces to overlay-versioned-releases
R-007, R-008.

## Requirements

### Requirement: Automatic timestamped backup before a mutating apply

ID: R-001
Traces to: overlay-versioned-releases R-007

WHEN `apply` is about to change one or more deployed files for a target, the
system SHALL create a timestamped backup of that target's current deployed
managed files before making any change. WHEN a target is already fully in
sync with the version being applied (a true no-op), the system SHALL create
no new backup.

#### Scenario: Backup created before files are overwritten

- GIVEN target `claude` has files that differ from the version about to be deployed
- WHEN `apply --target claude` runs
- THEN a new timestamped backup directory containing `claude`'s pre-change managed files exists before any target file is overwritten

#### Scenario: No-op apply creates no backup

- GIVEN target `claude` is already fully in sync with the version being applied
- WHEN `apply --target claude` runs
- THEN no new backup is created for `claude`

### Requirement: Backup retention capped at 3 per target, auto-pruned

ID: R-002
Traces to: overlay-versioned-releases R-007 (retention settled by proposal.md: "retain 3 per target, auto-prune")

WHEN a new backup is created for a target, the system SHALL retain only the 3
most recent backups for that target, automatically pruning older ones.

#### Scenario: Fourth backup prunes the oldest

- GIVEN target `claude` already has 3 retained backups
- WHEN a 4th backup is created for `claude`
- THEN the oldest of the previous 3 is pruned and exactly 3 backups remain

### Requirement: Restore rolls a target back from its most recent backup

ID: R-003
Traces to: overlay-versioned-releases R-008

WHEN a user runs `restore` for a target with at least one available backup,
the system SHALL replace that target's currently deployed managed files with
its most recent backup's files. IF a target has no backups, THEN `restore`
SHALL exit non-zero with a clear error instead of silently no-op'ing or
deleting files.

#### Scenario: Restore matches the most recent backup

- GIVEN target `claude` has at least one prior backup
- WHEN `overlay restore --target claude` runs with no further arguments
- THEN `claude`'s deployed managed files match the most recent backup's files exactly

#### Scenario: No-backup-available error path

- GIVEN target `claude` has no backups
- WHEN `overlay restore --target claude` runs
- THEN the command exits non-zero with a clear error, and no file is deleted or modified

## Open Questions for Design

- **Restore selection UX beyond "most recent":** whether `restore` accepts a
  specific backup timestamp argument, or only ever restores the latest, is
  flagged as open in the requirements brief and is not settled by
  proposal.md (proposal's success criteria state only "restore restores the
  latest backup"). Deferred to design.
