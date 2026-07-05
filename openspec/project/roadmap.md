# Roadmap SDD — labdrian-sdd-overlay

**Date**: 2026-07-03
**Based on**:
- OpenSpec: `openspec/project/manifest/context.md`, `openspec/project/manifest/rules.md`, `openspec/project/manifest/mission.md`, `openspec/project/architect/final.md`
- SDD history: active + archived changes under `openspec/changes`, PR #66, PR #68, PR #69

## Sequence

### opencode-runtime-plugin-lifecycle — OpenCode runtime plugin lifecycle foundation
- **Status**: completed
- **Goal**: provide the shared runtime command surface and real OpenCode plugin lifecycle behavior.
- **Derived from**: PR #66 (`feat(runtime): add OpenCode plugin lifecycle`) and runtime lifecycle architecture under `engine/runtime`.
- **Dependencies**: none
- **Acceptance evidence**: PR #66 merged; OpenCode plugin install/update/status/uninstall tests pass; `restart_required` status is reported honestly.
- **Risk if done early**: without this foundation, Claude/Codex lifecycle work would duplicate runtime dispatch and target semantics.
- **Command**: completed via merged PR #66
- **Tracking**: estimate [PENDIENTE] · impl completed · verify completed · review completed · fixes completed · closure PR #66 merged · deviation salvage from obsolete PR #13 was sliced into a clean OpenCode foundation · impact enabled Claude and Codex native lifecycle slices.

### claude-runtime-lifecycle — Claude Code runtime lifecycle support
- **Status**: completed
- **Goal**: implement real Claude `status/install/update/uninstall` lifecycle behavior using the shared runtime foundations.
- **Derived from**: archived OpenSpec change `openspec/changes/archive/2026-07-03-claude-runtime-lifecycle/`, PR #68, PR #69.
- **Dependencies**: opencode-runtime-plugin-lifecycle
- **Acceptance evidence**: PR #68 and PR #69 merged; `cd engine && go test ./...`, `cd engine && go vet ./...`, and `cd tui && go test ./...` passed; Judgment Day Round 2 approved.
- **Risk if done early**: before OpenCode/runtime foundations, Claude support would have required one-off command plumbing and unsafe settings mutation semantics.
- **Command**: `/sdd-archive claude-runtime-lifecycle` completed
- **Tracking**: estimate ~520–640 changed lines for code slice · impl completed · verify PASS · review Judgment Day approved after ownership fix · fixes post-review removed broad uninstall matching · closure PR #68/#69 merged · deviation split code and OpenSpec artifacts to keep reviews under the 800-line budget · impact Codex is now the remaining runtime lifecycle target.

### codex-runtime-lifecycle — Codex runtime lifecycle support
- **Status**: completed
- **Goal**: implement native Codex runtime lifecycle installation and honest status semantics without reusing Claude/OpenCode assumptions blindly.
- **Derived from**: user direction that all three CLIs must support installation with native adaptability; `openspec/specs/runtime-lifecycle/spec.md`; Claude archive report notes Codex as next SDD.
- **Dependencies**: claude-runtime-lifecycle
- **Acceptance evidence**: `engine/runtime/codex.go`/`engine/runtime/runtime_test.go`/`engine/runtime/codex_test.go` and `engine/cmd/runtime_test.go` cover native `status/install/update/uninstall`, mutation safety, and `--target all` aggregation; `cd engine && go test ./...`, `cd engine && go vet ./...`, `cd tui && go test ./...`, and `cd tui && go vet ./...` pass.
- **Risk if done early**: implementing Codex before Claude/OpenCode settled would risk false-green support and non-native configuration mutation.
- **Command**: `/sdd-archive codex-runtime-lifecycle`
- **Tracking**: estimate ~620 lines · impl completed · verify PASS · review completed · fixes completed · closure PASS · impact should close the three-runtime parity loop.

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
