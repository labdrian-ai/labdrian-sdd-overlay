# GADU Pi Subagent Specification

## Purpose

Make GADU dispatchable as a real Pi subagent (not a relayed persona): the
overlay ensures a working `subagent_*` dispatch runner is present —
preferring gentle-pi's own native subagents (>= 2.6.0) and falling back to
the third-party Pi Subagents extension only when native support is
unavailable — links the generated `agents/GADU.md` into
`~/.pi/agent/agents/` as an overlay-owned asset, verifies frontmatter
compatibility, and reports honest, distinct status and selective
uninstall — never touching gentle-pi- or pi-engram-owned files.

## Requirements

### Requirement: Subagent Runner Selected, Preferring Native Over the Legacy Extension

Traces to: R-012

WHEN the overlay installs or verifies GADU's Pi integration, IF
`~/.pi/agent/settings.json`'s `packages` array lists `npm:gentle-pi` at a
version >= 2.6.0 (gentle-pi's own native `subagent_*` tools), THEN the
overlay SHALL NOT install the third-party Subagents extension — doing so
while native support is present leaves gentle-pi's native tools
unregistered. IF that obsolete extension (`pi-subagents-j0k3r` or
`pi-subagents`) is already installed alongside native support, THEN the
overlay SHALL disclose the conflict and the exact removal command
(`pi remove npm:pi-subagents-j0k3r`) without removing it itself (it is not
overlay-owned). Otherwise — gentle-pi native subagents unavailable — IF
neither `pi-subagents-j0k3r` nor `pi-subagents` is already installed as a
Pi package, THEN the overlay SHALL install `pi-subagents-j0k3r` via `pi
install npm:pi-subagents-j0k3r`.

#### Scenario: Native gentle-pi subagents present skips the legacy extension

- GIVEN `npm:gentle-pi` at version >= 2.6.0 is listed in
  `~/.pi/agent/settings.json`
- WHEN the overlay runs its GADU-install step
- THEN `pi-subagents-j0k3r` SHALL NOT be installed

#### Scenario: Both native and the legacy extension present discloses the conflict

- GIVEN `npm:gentle-pi` at version >= 2.6.0 AND `pi-subagents-j0k3r` are
  both listed
- WHEN the overlay runs its GADU-install step
- THEN the overlay SHALL disclose the conflict and name
  `pi remove npm:pi-subagents-j0k3r` as the remediation
- AND the overlay SHALL NOT remove the extension package itself

#### Scenario: Neither package present triggers install when native is unavailable

- GIVEN gentle-pi native subagents are unavailable and neither extension
  package is installed
- WHEN the overlay runs its GADU-install step
- THEN `pi-subagents-j0k3r` SHALL be installed afterward

#### Scenario: Alternate package already installed is treated as satisfied

- GIVEN gentle-pi native subagents are unavailable and `pi-subagents` (the
  alternate package) is already installed
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

### Requirement: Honest, Distinct Subagent Runner and Link Status

Traces to: R-015

WHEN the overlay reports Pi package status, the overlay SHALL report the
subagent runner's state (`native` — gentle-pi >= 2.6.0, `legacy` — the Pi
Subagents extension only, `conflict` — both installed, `absent` —
neither installed) and the `~/.pi/agent/agents/GADU.md` link state
(missing, current, stale, conflict) as two separate, individually
observable status values. `native` and `legacy` are both proven/supported
states; `conflict` and `absent` are partial, and a `conflict` names the
exact removal command (`pi remove npm:pi-subagents-j0k3r`) for the
obsolete extension.

Ownership of the link is proven only by `os.Readlink` equality with the
expected `<destDir>/agents/GADU.md` target -- never by comparing file
contents or a side-record fingerprint. Under the stable-path symlink
design (D10), the link's target path does not change across a rebuild,
which is what makes `stale` reachable only when the symlink itself is
broken (its target file no longer exists, e.g. `destDir` was rebuilt from
scratch), never when the linked content merely predates the current
`engine/gadu/persona/body.md` -- that drift is instead caught independently
by `pipkg check` reporting a mode/content mismatch on `agents/GADU.md`.

#### Scenario: Stale link is reported independent of subagent runner state

- GIVEN the subagent runner state reads `native` or `legacy` and `GADU.md`
  is the overlay-owned symlink, but its target file no longer exists
  (`destDir` was rebuilt from scratch without recreating the link)
- WHEN status is reported
- THEN the GADU link state SHALL read `stale`, independent of the
  subagent runner state

#### Scenario: Missing subagent runner does not collapse link status

- GIVEN the subagent runner state reads `absent`
- WHEN status is reported
- THEN both fields SHALL be independently visible: subagent runner
  `absent`, link state reported regardless of runner state

#### Scenario: Both runners present is reported as conflict, not supported

- GIVEN gentle-pi native subagents (>= 2.6.0) AND the Pi Subagents
  extension are both installed
- WHEN status is reported
- THEN the subagent runner state SHALL read `conflict`
- AND status SHALL NOT report supported while this conflict is unproven
  clear

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
