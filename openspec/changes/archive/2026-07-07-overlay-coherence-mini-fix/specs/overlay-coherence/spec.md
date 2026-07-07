# Overlay Coherence Specification

## Purpose

Define coherence requirements for wrapper command forwarding, read-only hook status behavior, portable GADU output wording, directly related documentation, and focused verification.

## Requirements

### Requirement: Wrapper GADU generation command

The wrapper command `bin/labdrian-overlay gadu-generate [--check]` SHALL forward to the engine `gadu-generate` command with `OVERLAY_DIR` set to the repository root, and SHALL preserve all user-supplied arguments after `gadu-generate`.

#### Scenario: Forward check mode to engine

- GIVEN a repository checkout with an available engine binary
- WHEN a user runs `bin/labdrian-overlay gadu-generate --check`
- THEN the engine SHALL receive `gadu-generate --check`
- AND the engine process SHALL observe `OVERLAY_DIR` as the repository root

#### Scenario: Preserve future user arguments

- GIVEN a user supplies arguments after `gadu-generate`
- WHEN the wrapper dispatches the command
- THEN the wrapper SHALL pass those arguments unchanged to the engine

### Requirement: Read-only hook status failure

The `status-hooks` command SHALL NOT build, deploy, install, or otherwise mutate filesystem state when the engine binary is missing. It SHALL fail loudly with actionable guidance that tells the user to run `install-hooks`.

#### Scenario: Missing binary fails without mutation

- GIVEN the engine binary required by `status-hooks` is absent
- WHEN a user runs `bin/labdrian-overlay status-hooks`
- THEN the command SHALL exit unsuccessfully
- AND it SHALL NOT build or deploy the engine binary
- AND the output SHALL direct the user to run `install-hooks`

### Requirement: Runtime-neutral GADU generated artifacts

The canonical GADU body SHALL avoid runtime-specific claims, including wording equivalent to `You run on Claude`. Regeneration SHALL keep all generated GADU artifacts in sync and SHALL include `agents/GADU.md`, `opencode/agents/GADU.md`, and `skills/gadu-operator/SKILL.md`.

#### Scenario: Canonical body is portable

- GIVEN the canonical GADU body is regenerated into runtime artifacts
- WHEN generated artifacts are checked
- THEN none SHALL contain runtime-specific claims from the canonical body
- AND all three generated artifacts SHALL be present and in sync with the canonical source

### Requirement: Direct documentation coherence

README and directly related OpenSpec documentation SHALL describe the wrapper `gadu-generate` behavior and SHALL identify the three generated GADU artifacts.

#### Scenario: Reader sees accurate command and artifact count

- GIVEN a reader consults README or directly related OpenSpec docs
- WHEN they look for GADU generation behavior
- THEN the docs SHALL describe the wrapper command path
- AND they SHALL list all three generated artifacts

### Requirement: Module-scoped verification

Verification SHALL include module-scoped tests or checks for each touched surface: wrapper command dispatch, read-only hook status behavior, GADU generation portability, and documentation coherence.

#### Scenario: Focused checks cover changed surfaces

- GIVEN implementation changes are complete
- WHEN the module-scoped verification is run
- THEN it SHALL cover the touched shell, Go generator, and documentation surfaces
- AND it SHALL NOT require unrelated full runtime lifecycle behavior changes
