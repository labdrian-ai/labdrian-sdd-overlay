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

### Requirement: A Configuration With No MCP Container Is Installed Into, Not Refused

Traces to: longterm-mem R-016, R-017, R-018

No change-level `ID:` is claimed here, for the same reason the multi-target rule
below claims none: this rule was discovered during delivery rather than
specified up front, so it has no R-NNN of its own, and `Traces to:` is the key
that resolves a requirement.

WHEN a runtime's configuration file exists but declares no MCP container at all
— no `mcpServers` key for Claude Code, no `mcp` key for opencode, no
`mcp_servers` table for codex — the overlay SHALL create that container as part
of the same byte-preserving edit that writes the longterm-mem entry, rather than
failing the registration. Every pre-existing byte of the document SHALL survive
unchanged, and the pre-edit backup SHALL be written before the edit lands, as it
is for any other registration write.

This is the config a brand-new machine has, not an exotic one: a fresh opencode
install writes an `opencode.json` with no `mcp` key, and Claude Code's
`~/.claude.json` carries no `mcpServers` key until something adds one. The three
writers SHALL agree about it — one writer treating a missing container as fatal
while another appends its own is a defect, not a scope boundary.

#### Scenario: A configuration with no MCP container is installed into

- GIVEN a runtime's configuration file exists, is valid, and declares no MCP
  container at all, with unrelated settings of its own
- WHEN longterm-mem installs
- THEN the container is created holding only the ownership-tagged longterm-mem
  entry, every unrelated setting remains byte-identical, and the registration
  succeeds

#### Scenario: An empty configuration document is installed into

- GIVEN a runtime's configuration file exists and is an empty document
- WHEN longterm-mem installs
- THEN the container and the ownership-tagged longterm-mem entry are created and
  the registration succeeds, rather than failing for want of a container to
  write into

### Requirement: A Lost Ownership Record Is Recovered, Not Fatal

Traces to: longterm-mem R-016, R-017, R-018

No change-level `ID:` is claimed here, for the same reason as the two rules
around it.

WHEN the sidecar ownership record for a target is missing but the runtime's own
configuration holds an entry byte-identical to the one longterm-mem would write,
registration SHALL re-derive ownership from that entry, restore the ownership
record, and leave the configuration file untouched. Registration SHALL NOT
report a conflict in that case.

An entry that differs from the one longterm-mem would write SHALL still be
refused and left untouched, whatever its name: ownership is re-derived from the
entry's exact content, never from the fact that something with that name exists.

The ownership record is one small file in the overlay's own state directory.
Without this rule, losing it — a restored backup predating the install, a wiped
state directory, a mistyped state-directory override — makes every runtime's
entry read as a third party's at once, and the only recovery is hand-editing
every runtime configuration the overlay itself wrote.

#### Scenario: Registration recovers from a lost ownership record

- GIVEN a runtime was registered and the ownership record was then lost, while
  the entry longterm-mem wrote is still in the runtime's configuration
- WHEN longterm-mem registers again for the same binary
- THEN it succeeds, the ownership record is restored, and the runtime's
  configuration file is byte-identical to what it was before the run

#### Scenario: An entry that is not ours is still refused after a lost record

- GIVEN the ownership record is missing and the same-named entry in the
  runtime's configuration differs from the one longterm-mem would write
- WHEN longterm-mem registers
- THEN it refuses, reports the conflict, and leaves that entry byte-identical

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
