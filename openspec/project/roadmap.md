# Roadmap SDD — labdrian-sdd-overlay

**Date**: 2026-07-02
**Based on**:
- OpenSpec: `openspec/project/manifest/context.md`, `openspec/project/manifest/rules.md`, `openspec/project/manifest/mission.md`, `openspec/project/architect/final.md`
- SDD history: active + archived changes under `openspec/changes`

## Sequence

### skill-package-manager — Package manager and install lifecycle hardening
- **Status**: planned
- **Goal**: strengthen package manifest install lifecycle for local skills and provenance.
- **Derived from**: manifest/rules (`preserve reproducible merge semantics`) and architecture (`engine/skills` is the control-plane)
- **Dependencies**: none
- **Acceptance evidence**: updated install/sync lifecycle tests and successful `overlay` rollout.
- **Risk if done early**: incomplete test coverage can hide edge cases in manifest lifecycle and introduce hidden drift.
- **Command**: `/sdd-new skill-package-manager` (if re-opened)
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE] · deviation [PENDIENTE] · impact [PENDIENTE]

### skill-lifecycle — Lifecycle hooks for skill tracking and validation
- **Status**: planned
- **Goal**: formalize lifecycle checks and deterministic validation for skill entries.
- **Derived from**: manifest/rules (`explicit errors`, `manifest tracking`) and architecture (`engine/skills`)
- **Dependencies**: skill-package-manager
- **Acceptance evidence**: stable lifecycle validations and no silent state corruption under invalid manifests.
- **Risk if done early**: conflicting lifecycle transitions can mask merge intent and fail automated sync.
- **Command**: `/sdd-new skill-lifecycle`
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE] · deviation [PENDIENTE] · impact [PENDIENTE]

### skill-project-scope — Scoped registry and project manifest integration
- **Status**: planned
- **Goal**: tighten scope resolution for project-level registries in engine and skill loading.
- **Derived from**: architecture boundaries in manifest and requirements for reproducible sync.
- **Dependencies**: skill-lifecycle
- **Acceptance evidence**: deterministic project-scope behavior in registry operations and documented user-facing behavior.
- **Risk if done early**: partial scope handling can route manifests incorrectly and corrupt per-project rules.
- **Command**: `/sdd-new skill-project-scope`
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE] · deviation [PENDIENTE] · impact [PENDIENTE]

### gadu-portable-operator — Portable GADU generation pipeline closure
- **Status**: in-progress
- **Goal**: complete and stabilize dual output (agent + skill) generation contract with strict generation checks.
- **Derived from**: manifest mission and existing `gadu-operator` outputs.
- **Dependencies**: skill-project-scope
- **Acceptance evidence**: generated artifacts match canonical persona source and pass check mode.
- **Risk if done early**: mismatch between generated agent and portable persona causing inconsistent orchestration behavior.
- **Command**: `/sdd-continue gadu-portable-operator`
- **Tracking**: estimate [PENDIENTE] · impl [PENDIENTE] · verify [PENDIENTE] · review [PENDIENTE] · fixes [PENDIENTE] · closure [PENDIENTE] · deviation [PENDIENTE] · impact [PENDIENTE]
