# GADU Pi Subagent Specification

## Purpose

Make GADU dispatchable as a real Pi subagent (not a relayed persona): the
overlay installs/verifies the Pi Subagents extension, links the generated
`agents/GADU.md` into `~/.pi/agent/agents/` as an overlay-owned asset,
verifies frontmatter compatibility, and reports honest, distinct status and
selective uninstall — never touching gentle-pi- or pi-engram-owned files.

## Requirements

### Requirement: Subagents Extension Installed When Missing

Traces to: R-012

WHEN the overlay installs or verifies GADU's Pi integration, IF neither
`pi-subagents-j0k3r` nor `pi-subagents` is already installed as a Pi
package, THEN the overlay SHALL install `pi-subagents-j0k3r` via `pi
install npm:pi-subagents-j0k3r`.

#### Scenario: Neither package present triggers install

- GIVEN neither package is installed
- WHEN the overlay runs its GADU-install step
- THEN `pi-subagents-j0k3r` SHALL be installed afterward

#### Scenario: Alternate package already installed is treated as satisfied

- GIVEN `pi-subagents` (the alternate package) is already installed
- WHEN the overlay runs its GADU-install step
- THEN the overlay SHALL treat the extension as already satisfied and
  SHALL NOT install a second, redundant package

### Requirement: GADU.md Linked as an Overlay-Owned Asset

Traces to: R-013

WHEN the overlay installs GADU's Pi integration, the overlay SHALL place
the generated `agents/GADU.md` (source: `engine/gadu/persona/body.md`,
produced by `gadu-generate`) at `~/.pi/agent/agents/GADU.md`, marked as
overlay-owned and distinct from gentle-pi's or pi-engram's package-owned
manifest.

#### Scenario: Linked content matches the current generation

- GIVEN the overlay's GADU-install step has run
- WHEN `~/.pi/agent/agents/GADU.md` is inspected
- THEN its content SHALL match the current generated `agents/GADU.md`
- AND it SHALL be recorded as overlay-owned, not in gentle-pi's own asset
  manifest

#### Scenario: gentle-pi's own asset management does not touch it

- GIVEN gentle-pi's own asset-management step (`installSddAssets`/
  `removeRetiredManagedAssets`) runs afterward
- WHEN `~/.pi/agent/agents/GADU.md` is inspected
- THEN it SHALL be unchanged

### Requirement: Frontmatter Verified Compatible With the Extension Parser

Traces to: R-014

WHEN the overlay generates `agents/GADU.md`, the generator SHALL emit
`tools` in a form verified to parse correctly under the installed Pi
Subagents extension. The verified form for `pi-subagents-j0k3r@1.5.15` is
the inline `tools: '*'` wildcard string, which `parseInlineTools` expands
at runtime against the parent session's active tools; the generator SHALL
switch to an explicit tool list only IF a differently-behaving extension
version is detected.

#### Scenario: GADU dispatches with working tool access

- GIVEN the Pi Subagents extension is installed
- WHEN GADU is dispatched via `subagent_*` tools
- THEN GADU SHALL have access to the tools the frontmatter declares, with
  no parse error or silently-empty tool set

### Requirement: Honest, Distinct Extension and Link Status

Traces to: R-015

WHEN the overlay reports Pi package status, the overlay SHALL report the
Pi Subagents extension's installation state (not-installed, installed) and
the `~/.pi/agent/agents/GADU.md` link state (missing, current, stale,
conflict) as two separate, individually observable status values.

#### Scenario: Stale link is reported independent of extension state

- GIVEN the extension is installed but `GADU.md` predates the current
  `engine/gadu/persona/body.md` content
- WHEN status is reported
- THEN the GADU link state SHALL read `stale`, independent of the
  extension state reading `installed`

#### Scenario: Missing extension does not collapse link status

- GIVEN the extension is missing
- WHEN status is reported
- THEN both fields SHALL be independently visible: extension
  `not-installed`, link state reported regardless of extension state

### Requirement: Uninstall Removes Only the Overlay-Owned Link

Traces to: R-016

WHEN the overlay uninstalls GADU's Pi integration, the overlay SHALL
remove only the overlay-owned `~/.pi/agent/agents/GADU.md` link and SHALL
NOT remove, modify, or otherwise touch any gentle-pi-owned or
pi-engram-owned file. The overlay SHALL NOT uninstall the Subagents
extension package itself.

#### Scenario: Only GADU.md is removed

- GIVEN both the overlay-owned `GADU.md` and gentle-pi's own managed agent
  files coexist in `~/.pi/agent/agents/`
- WHEN overlay uninstall runs
- THEN only `GADU.md` SHALL be removed
- AND gentle-pi's managed files SHALL be byte-identical before and after
