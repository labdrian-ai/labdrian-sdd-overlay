# GADU Generator Specification

## Purpose

Generator that reads the canonical GADU source and emits both delivery shapes, keeping
them synchronized. Ships with an automated test. Generated artifacts are not
upstream-capturable — they are produced by the generator, not imported from an upstream.

## Requirements

### Requirement: Generator Emits Both Delivery Shapes

ID: R-002

WHEN the generator executes, the system SHALL read the canonical GADU source and emit
both `agents/GADU.md` and `skills/gadu-operator/SKILL.md` carrying the same persona body
with no hand-maintained divergence between the two outputs.

#### Scenario: Both artifacts produced with consistent content

- GIVEN the canonical source exists and the generator is invoked
- WHEN the persona body sections of both emitted files are compared
- THEN both contain identical persona body content derived from the canonical source

#### Scenario: One emitted file missing forces regeneration

- GIVEN `skills/gadu-operator/SKILL.md` was deleted after the last generator run
- WHEN the generator runs again
- THEN both `agents/GADU.md` and `skills/gadu-operator/SKILL.md` are present afterward

### Requirement: Generator Test Enforces Freshness

ID: R-003

The generator SHALL ship with an automated test that fails if either generated artifact
is missing, stale, or diverges from the canonical source.

#### Scenario: Stale artifact detected

- GIVEN the canonical source was modified after `agents/GADU.md` was last emitted
- WHEN the generator test runs
- THEN the test fails, identifying which artifact is stale or divergent

#### Scenario: Missing artifact detected

- GIVEN `skills/gadu-operator/SKILL.md` does not exist
- WHEN the generator test runs
- THEN the test fails, reporting the missing artifact by name

#### Scenario: Both artifacts in sync pass the test

- GIVEN the generator ran without interruption and neither file was modified afterward
- WHEN the generator test runs
- THEN the test passes with no failures

#### Scenario: Generator test is discoverable from canonical Go command

- GIVEN the generator test is implemented under `engine/`
- WHEN `cd engine && go test ./...` is executed
- THEN the generator test is discovered and run without additional configuration or flags

### Requirement: Capture Iterates Only Managed-Tagged Files

ID: R-010

(Resolves F5) WHEN `cmd_capture` runs, the system SHALL iterate only manifest entries
tagged `managed` via `managed_files()`; entries tagged `custom` (including all GADU rows)
SHALL be excluded from the capture iteration set and SHALL NOT be overwritten or
re-hashed by capture.

#### Scenario: Custom-tagged GADU entries skipped by capture

- GIVEN `agents/GADU.md` and `skills/gadu-operator/SKILL.md` are registered as
  `custom` in the manifest
- WHEN `cmd_capture` runs
- THEN neither file appears in `managed_files()` iteration; neither is modified or
  re-hashed

#### Scenario: Managed-tagged entries are captured normally alongside custom entries

- GIVEN the manifest contains entries tagged `managed` alongside the `custom` GADU rows
- WHEN `cmd_capture` runs
- THEN only `managed`-tagged entries are iterated and captured; `custom` rows are
  untouched
