# Delta for Overlay Agent Route

## MODIFIED Requirements

### Requirement: Route Resolution by Manifest Route Column

ID: R-006
Traces to: longterm-mem R-013

(Resolves F3, F4) `bin/labdrian-overlay` SHALL route each tracked file based
on an OPTIONAL third column `route` ∈ {`skill`, `agent`, `opencode-agent`,
`mcp`} in `overlay.manifest`; when the third column is absent the route
defaults to `skill`; `agent`-routed entries SHALL be deployed exclusively to
`~/.claude/agents`; `opencode-agent`-routed entries SHALL keep their existing
deployment behavior unchanged; `skill`-routed entries SHALL be deployed to
the three skills destinations; `mcp`-routed entries SHALL be dispatched to
the longterm-mem install path (build, copy, and register) rather than to any
skills or agents destination; existing two-column manifest rows SHALL
resolve correctly as route=skill without modification (backward-compatible
default); both the bash dispatch in `bin/labdrian-overlay` and the Go route
handling SHALL recognize the same four-value route domain.
(Previously: domain was {`skill`, `agent`, `opencode-agent`}; this delta adds
`mcp` only.)

#### Scenario: Agent-routed file deployed to claude agents only

- GIVEN the manifest contains row `agents/GADU.md  custom  agent`
- WHEN `cmd_apply` runs
- THEN the file is deployed to `~/.claude/agents/GADU.md` and NOT to any
  skills destination (`~/.claude/skills`, `~/.config/opencode/skills`,
  `~/.codex/skills`)

#### Scenario: Skill-routed file deployed to three skills destinations

- GIVEN the manifest contains row `skills/gadu-operator/SKILL.md  custom  skill`
- WHEN `cmd_apply` runs
- THEN the file is deployed to all three skills destinations identically to
  pre-change behavior, with no regression

#### Scenario: Existing two-column rows default to skill route

- GIVEN the manifest contains existing two-column `path  managed|custom` rows
- WHEN the updated installer reads the manifest
- THEN all existing entries resolve to route=skill and deploy correctly
  without modification to those rows

#### Scenario: Bash dispatch recognizes the mcp route

- GIVEN a manifest row `longterm-mem/...  custom  mcp`
- WHEN `bin/labdrian-overlay`'s bash dispatch parses it
- THEN it invokes the longterm-mem install path (build, copy, register)

#### Scenario: Go route handling recognizes the mcp route

- GIVEN the same row
- WHEN the Go route handling in `engine/skills`/`engine/installer` parses it
- THEN it recognizes the `mcp` route rather than erroring as unrecognized

#### Scenario: opencode-agent route is unaffected

- GIVEN an existing manifest row routed `opencode-agent`
- WHEN the updated bash dispatch and Go route handling parse it
- THEN it resolves and deploys exactly as before this change, with no
  regression

## ADDED Requirements

### Requirement: Unrouted or Unrecognized longterm-mem Row Is Rejected

ID: R-012
Traces to: longterm-mem R-035

IF a row under `longterm-mem/**` declares no route or an unrecognized
route, THEN the overlay manifest parser SHALL reject the row rather than
silently resolving it to a skills-destination path.

#### Scenario: Unrouted longterm-mem row is rejected by both parsers

- GIVEN a manifest row under `longterm-mem/**` with a missing or
  unrecognized route
- WHEN it is parsed by either the bash dispatch or the Go route handling
- THEN it is rejected with an explicit error, and it does not resolve to a
  skills-destination path
