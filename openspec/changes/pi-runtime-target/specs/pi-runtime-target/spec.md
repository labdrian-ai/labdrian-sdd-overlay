# Pi Runtime Target Specification

## Purpose

Define CLI target acceptance, package delivery, contract-gate injection,
honest status, disclosure, and lifecycle actions that let Pi (via gentle-pi)
receive the overlay's skills, agents, and per-phase SDD contract, without
touching gentle-pi's own managed state.

## Requirements

### Requirement: Pi Accepted as a Valid CLI Target

The `labdrian-overlay` CLI MUST accept `pi` as a valid value for `--target`
on `apply`, `status`, `sync-check`, and `uninstall`.

#### Scenario: Pi target is recognized
- GIVEN `--target pi` is passed to any of `apply`, `status`, `sync-check`, `uninstall`
- WHEN the command runs
- THEN it does not report an unknown-target error and returns a Pi-specific result

#### Scenario: Target all includes Pi without masking other failures
- GIVEN `--target all` runs and Pi succeeds or reports honest `partial`
- AND claude, opencode, or codex fails
- WHEN the aggregate command completes
- THEN the overall command fails and names the failing non-Pi target

### Requirement: Package-Delivered Skills and Agents

The system MUST build a `labdrian-pi` package (`package.json` with a `pi`
key, `skills/`, `agents/`, `extensions/`) from the repo and install it via
`pi install <local-path|git-url>`, registering it in
`~/.pi/agent/settings.json` packages. Custom agents MUST ship only under the
package's `agents/`, never written into `~/.pi/agent/agents/`.

#### Scenario: Local-path install registers the package idempotently
- GIVEN a built `labdrian-pi` package directory
- WHEN `apply --target pi` runs `pi install <local-path>`
- THEN the package is listed in `~/.pi/agent/settings.json` packages
- AND re-running install leaves a single package entry, not a duplicate

#### Scenario: Skills and agents are visible in a Pi session
- GIVEN the package is installed
- WHEN a Pi agent session starts
- THEN overlay skills are discoverable and custom agents are present
- AND no file under `~/.pi/agent/agents/` was written or overwritten

### Requirement: Deterministic Contract Gate via `before_agent_start`

WHEN a Pi session starts the `sdd-tasks` or `sdd-apply` agent, a
`before_agent_start` package extension MUST inject the bare contract PATH
LINE for each applicable contract under that contract's `injection_point`
header into that agent's system prompt, selecting applicability from each
contract's frontmatter (`applies_to_phases`/`excluded_phases`) via a strict
parse mirroring `engine/gate/gate.go`. It MUST exclude every other agent,
and MUST compose with (not overwrite) gentle-pi's own `before_agent_start`
handler output. Injected contract paths MUST be contained within the
package root; frontmatter that fails strict parsing MUST yield no injection
for that contract. The extension file MUST be discoverable by Pi's
extension discovery under the package's `extensions/` directory, authored
as `.ts` so it is loaded by Pi's jiti-based loader — not `.js`.

#### Scenario: sdd-tasks and sdd-apply receive both contract path lines
- GIVEN a Pi session starts the `sdd-tasks` or `sdd-apply` agent
- WHEN `before_agent_start` fires
- THEN the extension returns `systemPrompt` containing the bare path line
  for the minimalism-contract and the anti-generic-design contract, each
  under its own `injection_point` header, deterministically for the same
  agent/phase input

#### Scenario: Every other agent is excluded
- GIVEN a Pi session starts any agent other than `sdd-tasks` or `sdd-apply`
- WHEN `before_agent_start` fires
- THEN this extension returns `systemPrompt` unchanged

#### Scenario: Composition with gentle-pi's own handler
- GIVEN gentle-pi's own `before_agent_start` handler also injects into `sdd-*` agents
- WHEN both handlers fire for `sdd-tasks` or `sdd-apply`
- THEN both injections are present in the final `systemPrompt`, neither clobbering the other

#### Scenario: Contract paths stay contained and malformed frontmatter yields no injection
- GIVEN a contract's frontmatter is malformed, or would resolve a path outside the package root
- WHEN `before_agent_start` fires for `sdd-tasks` or `sdd-apply`
- THEN that contract is not injected
- AND no injected path line ever resolves outside the package root

#### Scenario: Extension is discovered from the package extensions directory
- GIVEN the `labdrian-pi` package ships `extensions/gate.ts`
- WHEN Pi's extension discovery scans the installed package
- THEN it loads `gate.ts` via jiti
- AND no `.js` extension file is required or expected

### Requirement: Honest Status for Unproven Activation

`status --target pi` MUST report `partial` rather than `supported` when
package presence, extension load, or longterm-mem MCP visibility cannot be
proven for any owned entry, naming the unproven entry.

#### Scenario: Unproven entry forces partial
- GIVEN any owned Pi entry (package listing, extension wiring, MCP visibility) cannot be proven
- WHEN `status --target pi` runs
- THEN it reports `partial` and names the unproven entry

#### Scenario: All entries proven report supported
- GIVEN every owned Pi entry is proven present
- WHEN `status --target pi` runs
- THEN it reports `supported`

### Requirement: `--no-extensions` Limitation Disclosure

`status --target pi` output and docs MUST state that `pi --no-extensions`
bypasses the `before_agent_start` gate extension for that session, and that
`--no-skills` bypasses skill discovery, without claiming to detect that
either flag was used. There is no `-ns` alias.

#### Scenario: Disclosure text is present
- GIVEN `status --target pi` is run
- WHEN output is inspected
- THEN it includes a static note that `pi --no-extensions` disables the gate
  extension and `pi --no-skills` disables skill discovery
- AND the note does not assert runtime detection of either flag

### Requirement: Pi-Scoped Uninstall

`uninstall --target pi` MUST remove only the `labdrian-pi` package entry via
`pi remove <source>`, passing the same local package path used at install
as `<source>`, and MUST deregister the longterm-mem MCP entry, without
touching gentle-pi- or pi-engram-owned state.

#### Scenario: Only the owned package entry is removed
- GIVEN gentle-pi/pi-engram-owned entries coexist with the `labdrian-pi` package entry
- WHEN `uninstall --target pi` runs
- THEN it runs `pi remove <local-package-path>`
- AND the `labdrian-pi` entry disappears from `~/.pi/agent/settings.json`
  packages, its MCP registration is removed, and gentle-pi/pi-engram-owned
  entries remain byte-identical

### Requirement: Pi Drift Detection via Sync-Check

`sync-check --target pi` MUST report drift between the installed package's
built content plus longterm-mem MCP availability and the current overlay
manifest, using the same idiom as the GADU generator's drift check.

#### Scenario: No drift is reported when unchanged
- GIVEN the installed package matches the current manifest
- WHEN `sync-check --target pi` runs
- THEN it reports no drift

#### Scenario: A source edit is detected as drift
- GIVEN a source edit changes what the package build would produce
- WHEN `sync-check --target pi` runs
- THEN it reports drift, naming the drifted entries (package content and/or MCP availability)
