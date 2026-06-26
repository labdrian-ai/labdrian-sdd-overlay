# Overlay Agent Route Specification

## Purpose

New distribution route in `bin/labdrian-overlay` for agent-typed files. Covers routing
logic, manifest schema compatibility, command coverage, durability, and bootstrap scope.
Applies to both `bin/labdrian-overlay` (modified) and `overlay.manifest` (modified).

## Requirements

### Requirement: Route Resolution by Manifest Route Column

ID: R-006

(Resolves F3, F4) `bin/labdrian-overlay` SHALL route each tracked file based on an
OPTIONAL third column `route` ∈ {`skill`, `agent`} in `overlay.manifest`; when the
third column is absent the route defaults to `skill`; `agent`-routed entries SHALL be
deployed exclusively to `~/.claude/agents`; `skill`-routed entries SHALL be deployed to
the three skills destinations; existing two-column manifest rows SHALL resolve correctly
as route=skill without modification (backward-compatible default).

#### Scenario: Agent-routed file deployed to claude agents only

- GIVEN the manifest contains row `agents/GADU.md  custom  agent`
- WHEN `cmd_apply` runs
- THEN the file is deployed to `~/.claude/agents/GADU.md` and NOT to any skills
  destination (`~/.claude/skills`, `~/.config/opencode/skills`, `~/.codex/skills`)

#### Scenario: Skill-routed file deployed to three skills destinations

- GIVEN the manifest contains row `skills/gadu-operator/SKILL.md  custom  skill`
- WHEN `cmd_apply` runs
- THEN the file is deployed to all three skills destinations identically to pre-change
  behavior, with no regression

#### Scenario: Existing two-column rows default to skill route

- GIVEN the manifest contains existing two-column `path  managed|custom` rows
- WHEN the updated installer reads the manifest
- THEN all existing entries resolve to route=skill and deploy correctly without
  modification to those rows

### Requirement: All Five Commands Honor Agent Route

ID: R-007

(Resolves F3 live-file lines) WHEN any of `cmd_apply`, `cmd_capture`, `cmd_status`,
`cmd_sync_check`, or `cmd_bootstrap` processes manifest entries — including file lookups
at every iteration point within those commands — the system SHALL apply route resolution
to determine correct destination(s) for each entry, covering all iteration paths, not
only the main manifest loop.

#### Scenario: Status reports agent file install state

- GIVEN `agents/GADU.md` is overlay-managed
- WHEN `cmd_status` runs
- THEN it reports the install state of the file under `~/.claude/agents`, not under a
  skills directory

#### Scenario: Sync-check detects clobbered agent file

- GIVEN `agents/GADU.md` was removed from `~/.claude/agents` by a gentle-ai upgrade
- WHEN `cmd_sync_check` runs
- THEN it reports the agent file as out-of-sync

#### Scenario: overlay.manifest records routing for both entry types

- GIVEN the manifest contains both `agents/GADU.md  custom  agent` and `skills/gadu-operator/SKILL.md  custom  skill`
- WHEN any of the five commands reads the manifest
- THEN it correctly identifies the agent entry as agent-routed and the skill entry as
  skill-routed without ambiguity

#### Scenario: Bootstrap applies route resolution to agent entries

- GIVEN the manifest contains `agents/GADU.md  custom  agent`
- WHEN `cmd_bootstrap` runs
- THEN `~/.claude/agents/GADU.md` is installed using the same route resolution as
  `cmd_apply`

### Requirement: Evidence-Based Durability Guard

ID: R-008

(Resolves F1, Locked Decision 3) `agents/GADU.md` and `skills/gadu-operator/SKILL.md`
SHALL be registered as overlay-managed and SHALL survive a `gentle-ai sync` / `gentle-ai
upgrade` without being clobbered, re-deployable by the overlay after upgrade; the guard
implementation SHALL be the minimum necessary to achieve re-deployability, sized after
experiment E1 (see R-013) is run and its result is documented.

#### Scenario: E1 confirms clobbering — overlay re-apply guard is exercised

- GIVEN experiment E1 is documented and confirms `gentle-ai upgrade` removes files from
  `~/.claude/agents`
- WHEN `cmd_apply` runs after such an upgrade
- THEN `~/.claude/agents/GADU.md` is reinstalled from the overlay source

#### Scenario: E1 confirms no clobbering — standard overlay registration is sufficient

- GIVEN experiment E1 is documented and confirms `gentle-ai upgrade` does NOT remove
  files from `~/.claude/agents`
- WHEN `cmd_sync_check` runs after a `gentle-ai upgrade`
- THEN both GADU artifacts are reported as in-sync and no additional guard code beyond
  standard overlay-managed registration is required

### Requirement: No Auto-Spawn or Runtime Agent Definitions

ID: R-009

This change SHALL NOT wire GADU into the SDD orchestrator's auto-spawn flow and SHALL NOT
emit native opencode/Codex agent-definition files (`.config/opencode/agents/*.md` or
`~/.codex/agents/*.toml`).

#### Scenario: No GADU files in opencode or Codex agent directories

- GIVEN the generator has run and `cmd_apply` has executed
- WHEN `.config/opencode/agents/` and `~/.codex/agents/` are inspected
- THEN no GADU-related agent-definition files are present in those directories

#### Scenario: SDD orchestrator has no GADU auto-spawn entry

- GIVEN the change is fully applied
- WHEN the SDD orchestrator state and phase routing are inspected
- THEN no reference to GADU exists in any auto-spawn or phase-routing configuration

### Requirement: Bootstrap Is Route-Aware and Installs All Routes

ID: R-011

(Resolves F6) WHEN `cmd_bootstrap` runs on a fresh machine, the system SHALL apply
route resolution to all manifest entries and SHALL install both `skill`-routed files to
the three skills destinations AND `agent`-routed files to `~/.claude/agents`;
`cmd_bootstrap` is the fifth function (alongside `cmd_apply`, `cmd_capture`,
`cmd_status`, `cmd_sync_check`) that honors route awareness.

#### Scenario: Fresh machine bootstrap installs both skills and agent files

- GIVEN a fresh machine and a manifest containing both `skill`-routed and `agent`-routed
  entries
- WHEN `cmd_bootstrap` runs
- THEN overlay skills are installed to all three skills destinations AND
  `~/.claude/agents/GADU.md` is present

#### Scenario: Bootstrap route logic is consistent with cmd_apply

- GIVEN the manifest contains `agents/GADU.md  custom  agent`
- WHEN `cmd_bootstrap` runs
- THEN `~/.claude/agents/GADU.md` is installed using the same route resolution logic as
  `cmd_apply`

### Requirement: E1 Experiment Documented Before Guard Is Sized

ID: R-013

BEFORE any durability guard code is implemented for `agents/GADU.md`, the implementor
SHALL run experiment E1 (execute `gentle-ai upgrade` on the target machine and inspect
`~/.claude/agents` for clobbering) and SHALL document the result in the overlay
implementation notes, so that the guard is sized to the minimum necessary for the
observed behavior.

#### Scenario: E1 result documented before guard code is committed

- GIVEN no guard code has been committed
- WHEN E1 is executed by running `gentle-ai upgrade` and inspecting `~/.claude/agents`
- THEN the result (clobbers / does not clobber) is written to the overlay implementation
  notes before any guard code is sized, written, or merged
