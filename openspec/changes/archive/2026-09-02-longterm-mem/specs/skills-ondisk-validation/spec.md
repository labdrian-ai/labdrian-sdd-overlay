# Delta for Skills On-Disk Validation

## ADDED Requirements

### Requirement: On-Disk Gate Excludes mcp-Routed Rows

ID: R-012
Traces to: longterm-mem R-013

The on-disk cross-check's deployable-path rule MUST recognize `mcp` as a
non-skill route so that `DeployableManifestPaths` excludes `mcp`-routed
manifest rows from the skills-directory presence check, the same way it
already excludes `agent`-routed rows.

#### Scenario: mcp-routed row is not required to exist under the skills directory

- GIVEN an `overlay.manifest` row routed `mcp`
- WHEN `skills validate`'s on-disk cross-check runs
- THEN that row is excluded from the on-disk presence check and produces no
  `MISSING_ON_DISK` diagnostic for not existing under the skills directory

#### Scenario: mcp-routed row does not falsely register as an unregistered skill file

- GIVEN a file deployed via an `mcp`-routed row that does not live under the
  skills directory
- WHEN `skills validate`'s on-disk cross-check runs
- THEN it produces no `UNREGISTERED_ON_DISK` diagnostic for that file
