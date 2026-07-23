# Delta for Skill Registry Propagate

This delta defines restoration acceptance only; it does not change propagation behavior.

## ADDED Requirements

### Requirement: Authoritative Scoped Block Restoration

The repair MUST use the existing authoritative propagator and shared contracts to generate exactly one marker-delimited block and exactly one generated row for each of `minimalism-contract-scope`, `skill-discovery-safety-scope`, and `anti-generic-design-scope`. Generated rows MUST NOT be hand-authored. (Traceability: AC-001)

#### Scenario: Restore exactly three unique scoped blocks

- GIVEN `.atl/skill-registry.md` lacks the three required scope blocks
- WHEN the authoritative propagation mechanism runs for the three corresponding contracts
- THEN each named BEGIN/END marker pair and its generated row occurs exactly once

### Requirement: Registry Preservation and Idempotence

The repair MUST preserve all registry content unrelated to the three generated marker ranges, and a second identical propagation pass MUST produce no registry change. (Traceability: AC-002)

#### Scenario: Preserve unrelated registry content

- GIVEN the registry content captured before restoration
- WHEN the restored registry is compared with that capture
- THEN content outside the three generated marker ranges is unchanged

#### Scenario: Repeat restoration without changes

- GIVEN all three required blocks and rows occur exactly once
- WHEN the same three authoritative propagations run again
- THEN `.atl/skill-registry.md` has no resulting diff and no duplicate marker or row

### Requirement: Healthy Hook Status

After restoration, `bin/labdrian-overlay status-hooks` MUST exit `0` and MUST NOT emit a missing-scope warning for any restored block. (Traceability: AC-003)

#### Scenario: Report healthy status

- GIVEN the three required generated blocks are present exactly once
- WHEN `bin/labdrian-overlay status-hooks` runs
- THEN it exits `0` without a missing-scope warning

### Requirement: Healthy Surfaces Remain Untouched

The repair MUST modify only `.atl/skill-registry.md`; the existing binary, hooks, shared contracts, propagation and status-check code, TUI, and Claude, OpenCode, and Codex targets MUST remain unchanged. No rebuild, restart, rewrite, installation, or synchronization repair SHALL occur. (Traceability: AC-004)

#### Scenario: Prove excluded surfaces are unchanged

- GIVEN the binary, hooks, contracts, and synchronized targets are healthy before restoration
- WHEN the repair diff and executed operations are reviewed
- THEN only the registry restoration is present and no excluded surface or workflow was changed

## Out of Scope

- TUI code or UX changes, including `upstream..main` investigation or warning suppression.
- Broad install or synchronization workflows, target changes, unrelated registry cleanup, and remote delivery.
