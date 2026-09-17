# Delta for Longterm-Mem MCP Registration

## ADDED Requirements

### Requirement: MCP Registration — Pi

Traces to: pi-runtime-target R-005

WHEN longterm-mem is registered for Pi, the overlay SHALL declare `pi.mcp`
in the `labdrian-pi` package's `package.json` as a package-relative PATH to
an `mcp.json` file built inside the package, whose top-level `mcpServers`
key holds a single ownership-tagged longterm-mem server entry — a
single-level container, not a nested container and not an inline
`package.json` declaration — and SHALL NOT modify `~/.pi/agent/mcp.json`
directly. The resulting server SHALL be visible to `pi-mcp-adapter` under a
package-name-prefixed server name. Registration SHALL probe whether the
`labdrian-pi` package is installed before writing, applying the same
skip-if-absent-on-expansion / fail-if-named-explicitly rule the command
layer applies to other targets.

#### Scenario: pi-engram's mcp.json stays untouched

- GIVEN a pre-existing pi-engram-owned `~/.pi/agent/mcp.json`
- WHEN `longterm-mem register --target pi` runs
- THEN `~/.pi/agent/mcp.json` remains byte-identical
- AND longterm-mem becomes available to Pi sessions via the package-declared `pi.mcp` entry

#### Scenario: The registration target is a package-relative mcp.json file

- GIVEN the `labdrian-pi` package build step runs
- WHEN the longterm-mem MCP registration is written
- THEN `package.json`'s `pi.mcp` key holds a package-relative path to an
  `mcp.json` file inside the built package
- AND that file's top-level `mcpServers` key holds exactly one
  ownership-tagged longterm-mem entry, with no nested containers

#### Scenario: The server name is package-prefixed

- GIVEN the `labdrian-pi` package declares `pi.mcp` for longterm-mem
- WHEN `pi-mcp-adapter` resolves available MCP servers for a Pi session
- THEN the longterm-mem server is visible under a name prefixed by the sanitized package name

#### Scenario: User/project config takes precedence over the package declaration

- GIVEN the package declares `pi.mcp` for longterm-mem
- AND a user or project MCP config also declares an entry with the same effective name
- WHEN a Pi session resolves MCP servers
- THEN the user/project entry wins, per `pi-mcp-adapter`'s own precedence rule, and the overlay does not attempt to override that outcome

#### Scenario: pi-engram init does not drop the registration

- GIVEN longterm-mem is registered for Pi via the package declaration
- WHEN `pi-engram init` re-runs
- THEN the longterm-mem registration remains intact

#### Scenario: Registration is skipped on expansion when the package is absent

- GIVEN `--target all` is used and the `labdrian-pi` package is not installed, while other targets are
- WHEN `longterm-mem register` runs
- THEN Pi's MCP registration is skipped and the skip is reported, without failing the run

#### Scenario: Registration fails when Pi is named explicitly and the package is absent

- GIVEN `--target pi` is passed explicitly to `longterm-mem register`
- AND the `labdrian-pi` package is not installed
- WHEN registration runs
- THEN it fails, rather than being skipped

### Requirement: Multi-Target Expansion Treats Pi Like the Other Runtimes

Traces to: pi-runtime-target R-008

The `--target all` expansion SHALL apply the same skip-if-absent and
fail-if-named-explicitly rules to Pi as it does to Claude Code, opencode,
and codex.

#### Scenario: An expansion skips Pi when its package is not installed

- GIVEN `--target all` is used and the `labdrian-pi` package is not installed, while other targets are
- WHEN registration runs
- THEN Pi is skipped and the skip is reported, and the run still succeeds for the other installed targets

#### Scenario: Pi named explicitly still fails without the package installed

- GIVEN `pi` is named explicitly and the `labdrian-pi` package is not installed
- WHEN registration runs
- THEN it fails, rather than being skipped the way an expansion would skip it
