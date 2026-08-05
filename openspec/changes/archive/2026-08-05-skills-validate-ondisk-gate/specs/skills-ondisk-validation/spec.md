# Skills On-Disk Validation Specification

## Purpose

Define the observable contract for cross-checking the overlay's `skills/`
directory against deployable `overlay.manifest` rows inside `skills
validate`, so a file with no deploying row, or a deploying row with no file,
fails the gate.

## Requirements

### Requirement: Explicit Skills-Directory Input

ID: R-002

The on-disk cross-check MUST resolve the directory it scans from an explicit
caller-supplied input and MUST NOT derive it from the process's current
working directory. The exact input and its absent-input behavior are a named
design output (R-002); this requirement pins only the observable guarantee.

#### Scenario: Directory resolution is cwd-independent and has no implicit fallback

- GIVEN the same explicit skills-directory input
- WHEN `skills validate` is invoked from at least three different working directories, including one where the working directory is `engine/` (a real 28-file Go package)
- THEN the on-disk cross-check scans the same resolved directory each time
- AND it never treats `engine/` or any other cwd as an implicit skills directory

### Requirement: On-Disk Core Is Unit-Testable Without Walking The Real Tree

ID: R-003

The on-disk cross-check logic inside `RenderValidateCore` MUST be exercisable
in unit tests through an injected or substitutable file listing, without
performing a filesystem walk of the real repository tree. The exact seam is a
named design output (R-003) that documents the rejected alternatives.

#### Scenario: Core test runs without touching the repository tree

- GIVEN a test-supplied file listing and manifest view
- WHEN the on-disk cross-check logic runs
- THEN it produces divergences from the supplied listing alone, with no `os.Stat` or `filepath.WalkDir` call against the real `skills/` tree

### Requirement: Infra-Exclusion Rule Independence Is Pinned By Test

ID: R-004

The deployable-path rule used by the on-disk cross-check MUST stay
independent from the infra-exclusion rule used by manifest loading. Paths
under `_shared/` MUST be treated as deployable by the on-disk check even
though manifest loading excludes them as infra.

#### Scenario: Unifying either rule set fails the pin test

- GIVEN a bidirectional test asserting `_shared/*` paths are deployable for the on-disk check and excluded for manifest loading
- WHEN either rule set is changed to match the other
- THEN the pin test fails

### Requirement: Unregistered File Fails Validation

ID: R-005

IF a regular file exists under the skills directory and no `overlay.manifest`
row deploys it, THEN `skills validate` MUST write an `UNREGISTERED_ON_DISK`
diagnostic naming that path to stderr and MUST exit 1.

#### Scenario: Unregistered file blocks validation, registering it clears it

- GIVEN a file under the skills directory with no deploying manifest row
- WHEN `skills validate` runs
- THEN stderr names the path in an `UNREGISTERED_ON_DISK` diagnostic and the process exits 1
- AND once a deploying row is added for that path, a rerun produces no such diagnostic and exits 0

### Requirement: Orphan Manifest Row Fails Validation

ID: R-006

IF an `overlay.manifest` row deploys a path under the skills directory and no
file exists at that path, THEN `skills validate` MUST write a
`MISSING_ON_DISK` diagnostic naming that row path to stderr and MUST exit 1.

#### Scenario: Orphan row blocks validation

- GIVEN a manifest row deploying a path with no file on disk
- WHEN `skills validate` runs
- THEN stderr names the row path in a `MISSING_ON_DISK` diagnostic
- AND the process exits 1

### Requirement: Full-Scan Divergence Reporting

ID: R-007

WHEN more than one on-disk divergence exists, `skills validate` MUST report
every divergence found in the same run rather than stopping at the first.

#### Scenario: Three divergences reported in one run

- GIVEN two unregistered files and one orphan manifest row
- WHEN `skills validate` runs
- THEN stderr contains three distinct diagnostics, one per divergence, and the process exits 1

### Requirement: The Overlay's Own Tree Passes The Gate

ID: R-011

WHEN `skills validate` runs against this repository's real
`overlay.manifest` and real skills directory, the command MUST exit 0.

#### Scenario: Real tree is clean

- GIVEN the repository's current `overlay.manifest` and skills directory, unmodified
- WHEN `skills validate` runs with the on-disk cross-check enabled
- THEN the process exits 0 with no `UNREGISTERED_ON_DISK` or `MISSING_ON_DISK` diagnostic
