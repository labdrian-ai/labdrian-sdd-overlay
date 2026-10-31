# Overlay Update Check Specification

## Purpose

A read-only `update` command that answers "is a newer release available, and
is each target current" without mutating any git ref, file, or persisted
state — closing the gap between the existing mutating `self-update`/`apply`
and a genuinely side-effect-free check. Traces to overlay-versioned-releases
R-006.

## Requirements

### Requirement: Read-only update-availability check

ID: R-001
Traces to: overlay-versioned-releases R-006

The system SHALL provide an `update` command that reports the latest
available release version (via overlay-release-identity's tag-resolution)
and each target's version/digest match status (via overlay-release-state),
and SHALL NOT modify any git ref, file, or persisted state while doing so.

#### Scenario: Reports latest version and per-target status

- GIVEN a newer release tag exists on `origin`
- WHEN `overlay update` runs
- THEN it prints the latest version and marks each target `up-to-date` or `behind (vX.Y.Z available)` without changing local `main` or any target file

#### Scenario: Zero mutation across repeated runs

- GIVEN `overlay update` runs twice in a row with no intervening change
- WHEN the repository's and each target's state is hashed before and after both runs
- THEN no git ref, target file, or state-file byte differs

#### Scenario: Never-deployed target reported without error

- GIVEN a target has no recorded state entry
- WHEN `overlay update` runs
- THEN that target is reported as "never deployed" rather than erroring or fabricating a version

## Open Questions for Design

- None beyond overlay-release-identity's pre-first-tag bootstrap open
  question, which this command also depends on (resolving "the latest
  available release version" when zero tags exist).
