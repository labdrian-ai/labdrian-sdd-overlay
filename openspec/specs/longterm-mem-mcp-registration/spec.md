# Longterm-Mem MCP Registration Specification

## Purpose

Defines the ownership-tagged MCP registration writers for Claude Code,
opencode, and codex, and the safe, selective uninstall that reverses them
without disturbing any unrelated entry.

## Requirements

### Requirement: MCP Registration — Claude Code

ID: R-016
Traces to: longterm-mem R-016

WHEN longterm-mem is installed for Claude Code, the overlay SHALL write an
ownership-tagged longterm-mem MCP server entry into Claude Code's
configuration using the existing atomic, backed-up config-merge pattern.

#### Scenario: Unrelated entries are preserved

- GIVEN Claude Code's configuration already has unrelated MCP server entries
- WHEN longterm-mem installs
- THEN only a new ownership-tagged longterm-mem entry is added, and all
  pre-existing entries remain byte-identical

#### Scenario: Reinstall is idempotent

- GIVEN an ownership-tagged longterm-mem entry already exists
- WHEN longterm-mem re-installs
- THEN it replaces that entry in place

#### Scenario: Untagged same-named entry is refused, not overwritten

- GIVEN an untagged longterm-mem-named entry exists that longterm-mem did
  not write
- WHEN longterm-mem installs
- THEN it refuses to overwrite it and reports the conflict

### Requirement: MCP Registration — opencode

ID: R-017
Traces to: longterm-mem R-017

WHEN longterm-mem is installed for opencode, the overlay SHALL write an
ownership-tagged longterm-mem MCP entry into opencode's configuration, using
opencode's own single-argument-list command shape and the same atomic,
backed-up merge pattern.

#### Scenario: Unrelated entries are preserved

- GIVEN opencode's configuration already has unrelated MCP entries
- WHEN longterm-mem installs
- THEN only a new ownership-tagged longterm-mem entry is added, and all
  pre-existing entries remain byte-identical

#### Scenario: Reinstall is idempotent

- GIVEN an ownership-tagged longterm-mem entry already exists
- WHEN longterm-mem re-installs
- THEN it replaces that entry in place

#### Scenario: Untagged same-named entry is refused, not overwritten

- GIVEN an untagged longterm-mem-named entry exists that longterm-mem did
  not write
- WHEN longterm-mem installs
- THEN it refuses to overwrite it and reports the conflict

### Requirement: MCP Registration — codex

ID: R-018
Traces to: longterm-mem R-018

WHEN longterm-mem is installed for codex, the overlay SHALL write an
ownership-tagged longterm-mem MCP server section into codex's configuration
via a section-preserving edit that leaves unrelated sections untouched.

#### Scenario: Unrelated sections and ordering are preserved

- GIVEN codex's configuration has an existing unrelated MCP server section
  and unrelated top-level keys
- WHEN longterm-mem installs
- THEN only a new longterm-mem section is added, all other sections and key
  ordering remain unchanged, and the file remains valid

#### Scenario: Reinstall is idempotent

- GIVEN an ownership-tagged longterm-mem section already exists
- WHEN longterm-mem re-installs
- THEN it replaces that section in place

### Requirement: Multi-Target Expansion Skips Runtimes That Are Not Installed

Traces to: longterm-mem R-016, R-017, R-018

No change-level `ID:` is claimed here on purpose. This rule was discovered
during delivery rather than specified up front, so it has no R-NNN of its own,
and every free number in that space belongs to an unrelated requirement —
`R-020` is the mid-term query scoping rule. Capability-local IDs already
collide across delta spec files, which is why `Traces to:` is the key that
resolves a requirement, not `ID:`.

The `--target all` expansion SHALL skip a runtime whose configuration file is
absent, reporting the skip, and SHALL NOT fail the run on its account. A
runtime named explicitly SHALL NOT be skipped: naming it asserts it should be
there, so an absent configuration file is a failure.

This rule belongs to the command layer that drives every per-runtime writer,
not to any one runtime.

#### Scenario: An expansion skips a runtime that is not installed

- GIVEN `--target all` is used and one of the three runtimes has no
  configuration file on disk, while the others do
- WHEN registration runs
- THEN that runtime is skipped and the skip is reported, and the run still
  succeeds for the runtimes that are installed

#### Scenario: A runtime named explicitly still fails without a configuration file

- GIVEN a single runtime is named explicitly and it has no configuration file
  on disk
- WHEN registration runs
- THEN it fails, rather than being skipped the way an expansion would skip it

### Requirement: Ownership-Tagged Safe Uninstall

ID: R-019
Traces to: longterm-mem R-019

WHEN longterm-mem is uninstalled from a runtime, the overlay SHALL remove
only the ownership-tagged longterm-mem MCP entry from that runtime's
configuration.

#### Scenario: Selective removal across all three runtimes

- GIVEN Claude Code, opencode, and codex configurations each have a
  longterm-mem entry plus other unrelated MCP entries
- WHEN uninstall runs for one runtime
- THEN only that runtime's longterm-mem entry is removed and all other
  entries in all three files remain untouched

#### Scenario: Untagged entry is preserved and reported, not removed

- GIVEN an untagged longterm-mem-named entry exists that longterm-mem did
  not write
- WHEN uninstall runs
- THEN it leaves that entry in place and reports it as unmanaged rather than
  removing it

#### Scenario: Partial uninstall does not remove the shared binary

- GIVEN longterm-mem is uninstalled from only one of the three runtimes
- WHEN the command completes
- THEN the shared persistent binary is not removed — only a full uninstall
  across all three runtimes, or an explicit purge, removes it
