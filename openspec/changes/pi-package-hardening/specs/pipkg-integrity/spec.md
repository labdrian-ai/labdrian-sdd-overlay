# Pipkg Integrity Specification

## Purpose

Close pipkg's real integrity gaps so a Pi package's build/verify pipeline
catches file-mode drift, build-root permission drift, registry path
traversal, and SKILL.md name/directory mismatches — the same pipeline that
builds and verifies GADU's own asset.

## Requirements

### Requirement: Mode Drift Detection

Traces to: R-001

WHEN pipkg compares an installed package's files against the registry, the
`Check`/`listFiles` comparison SHALL report a file as drifted IF its mode
differs from the registry-recorded mode, in addition to any byte-content
difference.

#### Scenario: Mode-only drift is reported

- GIVEN an installed file with byte-identical content but a changed mode
  (e.g. 0644 vs 0755)
- WHEN `pipkg Check` runs
- THEN the file SHALL be reported as drifted

#### Scenario: Identical files report no drift

- GIVEN an installed file identical in both bytes and mode
- WHEN `pipkg Check` runs
- THEN the file SHALL NOT be reported as drifted

### Requirement: Build Root Permissions

Traces to: R-002

WHEN pipkg creates its build root directory, pipkg SHALL set that
directory's permissions to 0755.

#### Scenario: Fresh build root is 0755

- GIVEN a fresh `pipkg Build` invocation
- WHEN the build root is created
- THEN its mode SHALL be 0755

### Requirement: Registry Path Containment

Traces to: R-003

IF a registry entry's `path`, once joined to the package root, resolves
outside that package root, THEN pipkg build SHALL reject that entry and
SHALL NOT write any file for it.

#### Scenario: Traversal path is rejected

- GIVEN a registry entry with `path: "../../outside"`
- WHEN `pipkg Build` runs
- THEN the build SHALL fail with an explicit rejection referencing that
  entry
- AND no file SHALL be written outside the package root

#### Scenario: In-root path builds normally

- GIVEN a registry entry with a normal in-root relative path
- WHEN `pipkg Build` runs
- THEN the entry SHALL build normally

### Requirement: SKILL.md Name Matches Directory

Traces to: R-004

WHEN pipkg validates a skill entry, IF the entry's `SKILL.md` frontmatter
`name` field does not match the entry's directory name, THEN pipkg
validation SHALL reject that entry.

#### Scenario: Mismatch is rejected

- GIVEN a skill directory `foo/` whose `SKILL.md` declares `name: bar`
- WHEN pipkg validates the registry
- THEN validation SHALL fail, naming the mismatch

#### Scenario: Match passes

- GIVEN a skill directory `foo/` whose `SKILL.md` declares `name: foo`
- WHEN pipkg validates the registry
- THEN validation SHALL pass for that entry
