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
