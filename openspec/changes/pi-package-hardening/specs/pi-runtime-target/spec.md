# Delta for Pi Runtime Target

## MODIFIED Requirements

### Requirement: Package-Delivered Skills and Agents

Traces to: R-013

The system MUST build a `labdrian-pi` package (`package.json` with a `pi`
key, `skills/`, `agents/`, `extensions/`) from the repo and install it via
`pi install <local-path|git-url>`, registering it in
`~/.pi/agent/settings.json` packages. Custom agents MUST ship only under
the package's `agents/`, never written into `~/.pi/agent/agents/`, EXCEPT
for the single overlay-owned `~/.pi/agent/agents/GADU.md` link, which the
GADU-subagent capability installs and manages separately from the package
build.
(Previously: custom agents were forbidden from ever being written into
`~/.pi/agent/agents/`, with no exception.)

#### Scenario: Local-path install registers the package idempotently
- GIVEN a built `labdrian-pi` package directory
- WHEN `apply --target pi` runs `pi install <local-path>`
- THEN the package is listed in `~/.pi/agent/settings.json` packages
- AND re-running install leaves a single package entry, not a duplicate

#### Scenario: Skills are visible in a Pi session and agents ship as package content
- GIVEN the package is installed
- WHEN a Pi agent session starts
- THEN overlay skills are discoverable
- AND the custom agent files are present under the package's `agents/` (Pi 0.85.1 packages declare no agent resource and gentle-pi discovers agents only from `~/.pi/agent/agents/`, so a package cannot make them session-visible)
- AND no file under `~/.pi/agent/agents/` was written or overwritten by the package build, other than the overlay-owned `GADU.md` link installed by the GADU-subagent capability

### Requirement: Honest Status for Unproven Activation

Traces to: R-015

`status --target pi` MUST report `partial` rather than `supported` when
package presence, extension load, longterm-mem MCP visibility, the Pi
Subagents extension's installation state, or the `GADU.md` link state
cannot be proven for any owned entry, naming the unproven entry.
(Previously: covered package presence, extension load, and longterm-mem
MCP visibility only.)

#### Scenario: Unproven entry forces partial
- GIVEN any owned Pi entry (package listing, extension wiring, MCP visibility, Subagents extension state, GADU link state) cannot be proven
- WHEN `status --target pi` runs
- THEN it reports `partial` and names the unproven entry

#### Scenario: All entries proven report supported
- GIVEN every owned Pi entry is proven present
- WHEN `status --target pi` runs
- THEN it reports `supported`

### Requirement: Pi-Scoped Uninstall

Traces to: R-016

`engine runtime uninstall --target pi` (the adapter path; there is no
top-level `labdrian-overlay uninstall` verb) MUST remove only the
`labdrian-pi` package entry via `pi remove <source>`, passing the same
local package path used at install as `<source>`, MUST deregister the
longterm-mem MCP entry, and MUST remove the overlay-owned
`~/.pi/agent/agents/GADU.md` link, without touching gentle-pi- or
pi-engram-owned state and without uninstalling the Pi Subagents extension
package itself.
(Previously: removed only the package entry and the longterm-mem MCP
entry; did not address the GADU link.)

#### Scenario: Only the owned package entry is removed
- GIVEN gentle-pi/pi-engram-owned entries coexist with the `labdrian-pi` package entry
- WHEN `engine runtime uninstall --target pi` runs
- THEN it runs `pi remove <local-package-path>`
- AND the `labdrian-pi` entry disappears from `~/.pi/agent/settings.json`
  packages, its MCP registration is removed, and gentle-pi/pi-engram-owned
  entries remain byte-identical

#### Scenario: GADU link is removed without touching the extension package
- GIVEN the overlay-owned `~/.pi/agent/agents/GADU.md` link exists
- WHEN `engine runtime uninstall --target pi` runs
- THEN `GADU.md` SHALL be removed
- AND the Pi Subagents extension package itself SHALL remain installed

### Requirement: Pi Drift Detection via Sync-Check

Traces to: R-006, R-007

`sync-check --target pi` MUST report drift between the installed package's
built content plus longterm-mem MCP availability and the current overlay
manifest, using the same idiom as the GADU generator's drift check. WHEN
the deployed package's `package.json` carries a resolvable
`labdrian.builtFrom` ref, the comparison SHALL use that ref's git tree
rather than the current working-tree checkout; WHEN the ref is
unresolvable, `sync-check --target pi` SHALL compare against `main` and
SHALL state in its output that the comparison target was `main`.
(Previously: compared against the current overlay manifest with no
built-from-ref awareness, which produced false drift against the current
checkout on feature branches — #315.)

#### Scenario: No drift is reported when unchanged
- GIVEN the installed package matches the current manifest
- WHEN `sync-check --target pi` runs
- THEN it reports no drift

#### Scenario: A source edit is detected as drift
- GIVEN a source edit changes what the package build would produce
- WHEN `sync-check --target pi` runs
- THEN it reports drift, naming the drifted entries (package content and/or MCP availability)

#### Scenario: Feature-branch checkout does not cause false drift
- GIVEN a deployed package built from `main` at a recorded `labdrian.builtFrom` commit, and a feature branch checked out with unrelated changes
- WHEN `sync-check --target pi` runs
- THEN it reports no drift caused by the branch's unrelated changes
