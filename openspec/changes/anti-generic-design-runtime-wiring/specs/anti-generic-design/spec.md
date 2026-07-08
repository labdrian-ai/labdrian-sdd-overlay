# Delta for anti-generic-design

## ADDED Requirements

### Requirement: Distinct marker pair for anti-generic-design registry block

ID: R-101

WHEN the propagator scopes a registry block for the `anti-generic-design` embedded
contract, `engine/propagator/propagator.go` SHALL expose dedicated
`AntiGenericDesignBeginMarker`/`AntiGenericDesignEndMarker` constants distinct from
both `BeginMarker`/`EndMarker` (minimalism-contract) and
`DiscoverySafetyBeginMarker`/`DiscoverySafetyEndMarker` (skill-discovery-safety).

#### Scenario: Three independent blocks coexist

- GIVEN a registry already containing minimalism-contract and
  skill-discovery-safety scoped blocks
- WHEN `propagate --embedded-contract anti-generic-design` runs
- THEN the registry gains an `anti-generic-design-scope` block using its own
  BEGIN/END markers
- AND the two pre-existing blocks are left byte-identical

#### Scenario: Marker uniqueness

- GIVEN the three marker-pair constants defined in `propagator.go`
- WHEN their string values are compared pairwise
- THEN no two marker pairs share a BEGIN or END string

### Requirement: embeddedContract() resolves "anti-generic-design"

ID: R-102

WHEN `embeddedContract("anti-generic-design")` is called, `engine/cmd/main.go`
SHALL return `ok=true` with an `embeddedContractSpec` whose `content` is the
embedded canonical text, `beginMarker`/`endMarker` are the R-101 constants,
`rowLabel` is `"anti-generic-design"`, and `defaultPath` is
`"skills/_shared/anti-generic-design.md"`.

#### Scenario: propagate resolves the new case

- GIVEN the engine binary built with the new case
- WHEN `propagate --embedded-contract anti-generic-design --registry <path>` runs
- THEN it does not hit the `"unknown embedded contract"` error branch
- AND the registry row's path cell reads `skills/_shared/anti-generic-design.md`

#### Scenario: gate-task resolves the new case

- GIVEN the same engine binary
- WHEN `gate-task --embedded-contract anti-generic-design` runs with a well-formed
  PreToolUse Agent payload on stdin
- THEN it emits an injection response referencing the contract path, not the
  fail-safe `{}` pass-through

### Requirement: Embedded canonical asset

ID: R-103

The engine SHALL ship the canonical `anti-generic-design` contract text as a
compiled-in Go asset (mirroring `assets.SkillDiscoverySafety`) so `gate-task` and
`propagate` can source its content without reading an external file at runtime.

#### Scenario: Works without the deployed file present

- GIVEN `skills/_shared/anti-generic-design.md` is absent from disk
- WHEN `gate-task --embedded-contract anti-generic-design` runs
- THEN it still emits a valid injection response sourced from the embedded asset

### Requirement: Contract frontmatter declares target phases

ID: R-104

The deployed `skills/_shared/anti-generic-design.md` file SHALL carry
`applies_to_phases: [sdd-tasks, sdd-apply]` in its frontmatter so
`propagator.ParseFrontmatter` derives scope without error.

#### Scenario: Frontmatter parses to the two target phases

- GIVEN `skills/_shared/anti-generic-design.md`'s frontmatter
- WHEN `propagator.ParseFrontmatter` parses its content
- THEN `phases.AppliesTo` equals `["sdd-tasks", "sdd-apply"]`
- AND no error is returned

#### Scenario: Missing applies_to_phases fails loud

- GIVEN a copy of the contract with `applies_to_phases` removed
- WHEN `propagator.ParseFrontmatter` parses it
- THEN it returns an error and callers surface it (propagate exits 1; gate-task
  logs a stderr warning and passes through)

### Requirement: settings.json wires both hooks for the new contract

ID: R-105

WHEN `~/.claude/settings.json` is configured for `anti-generic-design`, it SHALL
contain one `PreToolUse` entry (matcher `"Agent"`) whose command invokes
`gate-task --embedded-contract anti-generic-design` and one `UserPromptSubmit`
entry whose command invokes
`propagate --registry "${CLAUDE_PROJECT_DIR:-.}/.atl/skill-registry.md" --embedded-contract anti-generic-design`,
each following the same `command -v ... && ... || true` fail-safe shape as the
existing `skill-discovery-safety` lines.

#### Scenario: Both hook lines present and shaped like the precedent

- GIVEN the updated `settings.json`
- WHEN the `hooks.PreToolUse` and `hooks.UserPromptSubmit` arrays are inspected
- THEN exactly one new entry exists in each array referencing
  `--embedded-contract anti-generic-design`
- AND both entries use the `command -v /home/labdrian/.claude/bin/gentle-ai-overlay ... || true` guard

#### Scenario: Existing hook entries untouched

- GIVEN the same updated `settings.json`
- WHEN the pre-existing minimalism-contract and skill-discovery-safety hook
  entries are compared before/after
- THEN they are unchanged

### Requirement: Original invokable skill remains unchanged

ID: R-106

IF the `anti-generic-design-runtime-wiring` change is applied, THEN
`skills/anti-generic-design/SKILL.md` SHALL remain present and invocable by
trigger/name, with its content unmodified by this change.

#### Scenario: Content is untouched

- GIVEN `skills/anti-generic-design/SKILL.md` before the change
- WHEN the change is applied
- THEN the file's content is byte-identical afterward

#### Scenario: Still invocable outside the SDD flow

- GIVEN a user reviewing a design produced outside the SDD phase flow
- WHEN they invoke `anti-generic-design` by name/trigger
- THEN they receive the same guidance as before this change, independent of the
  new embedded contract
