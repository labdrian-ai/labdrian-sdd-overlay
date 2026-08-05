# Delta for skill-package-manager

## ADDED Requirements

### Requirement: Exit-Code Consumer Enumeration Precedes The Behavior Change

ID: R-001

WHERE a change adds a divergence class to `skills validate`'s exit contract,
the change MUST persist a named enumeration of every consumer of that exit
code, together with the assessed impact on each consumer, before any code
path emits the new class.

#### Scenario: Enumeration exists before the new class ships

- GIVEN a change that adds `UNREGISTERED_ON_DISK` and `MISSING_ON_DISK` to the exit contract
- WHEN the implementing code path is merged
- THEN a persisted consumer enumeration (path/line and per-consumer disposition) already exists in the change history

### Requirement: Existing Registry/Manifest Divergence Classes Remain Unchanged

ID: R-008

WHILE the on-disk cross-check is active, `skills validate` MUST keep the
`MISSING_IN_MANIFEST`, `MISSING_IN_REGISTRY`, `TAG_MISMATCH`, and `MIXED_TAG`
outcomes, their diagnostic format, and their exit codes exactly as they were
before this change.

#### Scenario: Existing behavior and green paths are unaffected

- GIVEN the current `skills validate` test suite for the four registry/manifest classes
- WHEN the on-disk cross-check is added
- THEN every existing test passes with no assertion edits
- AND a registry/manifest pair with no divergence of any kind still exits 0

### Requirement: On-Disk Diagnostics Name A Working Remediation

ID: R-009

WHEN `skills validate` exits non-zero because of an on-disk divergence, the
diagnostic MUST name the manifest-row edit that resolves it and MUST NOT cite
a command that cannot resolve it (`sync-manifest` cannot clear
`UNREGISTERED_ON_DISK` for reference files or `_shared/*` files, because
`isSkillRow` only regenerates `*/SKILL.md` rows).

#### Scenario: Diagnostic names the manifest-row edit, not sync-manifest

- GIVEN an `UNREGISTERED_ON_DISK` or `MISSING_ON_DISK` diagnostic
- WHEN its text is inspected
- THEN it names editing the specific `overlay.manifest` row/path as the remediation
- AND it does not claim `sync-manifest` resolves the divergence

### Requirement: Documented Exit Contract Covers The New Classes

ID: R-010

WHERE `skills validate` can exit non-zero because of an on-disk divergence,
the overlay's documentation (CLI help text, README, and the command's doc
comment) MUST state that the command also cross-checks the skills directory
against `overlay.manifest`.

#### Scenario: Documentation matches the real contract

- GIVEN the documentation sites for `skills validate` (help text, README, `engine/cmd/main.go` doc comment)
- WHEN their content is compared against the actual exit contract
- THEN each site states that an on-disk divergence also causes a non-zero exit
