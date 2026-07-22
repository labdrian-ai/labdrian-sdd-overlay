# Roadmap SDD — labdrian-sdd-overlay

**Date**: 2026-07-21
**Based on**:
- OpenSpec: `openspec/project/manifest/context.md`, `openspec/project/manifest/rules.md`, `openspec/project/manifest/mission.md`, `openspec/project/architect/final.md`
- SDD history: active + archived changes under `openspec/changes`, PR #66, PR #68, PR #69, PR #78

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

### oo-quality-contract — Local advisory OO quality contract foundation
- **Status**: completed
- **Goal**: provide a concise, manifest-tracked OO quality contract for domain-heavy TypeScript/NestJS work without making SOLID guidance global SDD policy.
- **Derived from**: PR #78; archived OpenSpec change `openspec/changes/archive/2026-07-08-oo-quality-contract/`; promoted spec `openspec/specs/oo-quality-contract/spec.md`.
- **Dependencies**: existing shared-contract pattern and manifest tracking in `overlay.manifest`; no runtime wiring dependency in the first slice.
- **Acceptance evidence**: PR #78 merged; `skills/_shared/oo-quality-contract.md` exists; `overlay.manifest` tracks `_shared/oo-quality-contract.md custom`; artifact tests verify contract path, manifest row, phase scope, precedence, context gate, advisory posture, TDD non-mandate, non-vendoring, and no first-slice runtime wiring.
- **Risk if done early**: adding OO guidance before proving a scoped, non-runtime contract could have leaked SOLID advice into non-OO, Go, shell, docs, config, or generated-artifact work.
- **Command**: completed via merged PR #78
- **Tracking**: estimate 180-260 changed lines originally, later exceeded review budget due to archived SDD documentation · impl completed · verify PASS · review completed · fixes completed · closure PR #78 merged · deviation proceeded under maintainer-approved 2026-07-08 single-PR size exception · impact creates the source contract for future deterministic runtime loading.

### oo-quality-contract-runtime-wiring — Deterministic OO contract runtime loading
- **Status**: completed
- **Goal**: wire deterministic runtime loading/injection for the OO quality contract only when the target phase and work context require it.
- **Derived from**: `openspec/specs/oo-quality-contract/spec.md` R-003 through R-008; PR #78 archive notes that runtime injection, gate wiring, propagation, and `.atl/skill-registry.md` edits were intentionally deferred; architecture identifies `engine/gate` as the scoped marker injection contract and `engine/propagator` as the registry row/marker-block generator.
- **Dependencies**: oo-quality-contract; current `engine/gate` scoped marker injection behavior; current `engine/propagator` registry row and marker-block generation behavior; current `.atl/skill-registry.md` generated registry semantics. This slice does not depend on the later `skill-package-manager`, `skill-lifecycle`, or `skill-project-scope` hardening chain; if exploration discovers a hard dependency there, re-sequence before apply.
- **Acceptance evidence**: tests prove the contract is loaded only for intended SDD phases and OO/domain-heavy TypeScript/NestJS application contexts; tests prove it is not loaded for Go, shell, docs, config, generated artifacts, non-domain, or non-OO work; existing engine and TUI tests remain green.
- **Risk if done early**: over-injecting OO/SOLID guidance or bypassing higher-precedence specs, design, project conventions, the minimalism contract, and review-budget boundaries.
- **Command**: `/sdd-archive oo-quality-contract-runtime-wiring`
- **Tracking**: estimate 500-750 changed lines originally · impl completed · verify PASS · review completed with blocker fixes applied · closure archived on 2026-07-08 · deviation actual review size exceeded the original 800-line budget forecast and requires reviewer-burden/size-exception handling before PR or equivalent review submission · impact converted the advisory artifact into scoped runtime behavior without broadening governance scope.

### restore-skill-registry-scoped-blocks — Restore generated registry scope blocks
- **Status**: completed
- **Goal**: restore a healthy hook-status signal by regenerating exactly the `minimalism-contract-scope`, `skill-discovery-safety-scope`, and `anti-generic-design-scope` blocks through the authoritative propagation path while preserving unrelated registry content.
- **Derived from**: archived change `openspec/changes/archive/2026-07-21-restore-skill-registry-scoped-blocks/` R-001 and AC-001 through AC-004; manifest rules requiring the trusted engine control-plane and minimal surface change; architecture contracts for `engine/propagator`, marker-based generated registry sections, and semantic status exit codes.
- **Dependencies**: current `engine/propagator` marker-based replacement behavior and current `.atl/skill-registry.md`; independent of the planned `skill-package-manager` → `skill-lifecycle` → `skill-project-scope` hardening chain.
- **Acceptance evidence**: all three required BEGIN/END marker pairs and generated rows occur exactly once; unrelated registry content is preserved; `bin/labdrian-overlay status-hooks` exits `0`; runtime/TUI hook status is healthy and healthy binaries, hooks, contracts, and synchronized targets remain untouched.
- **Risk if done early**: using a broad install or synchronization path, or hand-authoring generated rows, could mutate healthy surfaces or duplicate blocks; expected `upstream..main` TUI diff noise remains a separate non-goal.
- **Command**: `/sdd-archive restore-skill-registry-scoped-blocks` completed 2026-07-21
- **Tracking**: estimate 3-5h · actual 1.30h · impl completed (11/11 tasks) · verify PASS WITH WARNINGS (4/4 requirements, 5/5 scenarios) · review approved recovered lineage `review-bc87e2c7d46b98f7` · fixes completed with historical absent-lock cleanup and rollback evidence marked non-reusable · closure archive completed 2026-07-21 · deviation actual 1.70-3.70h below estimate · impact restored healthy runtime/TUI hook status without source changes or later roadmap dependency changes.

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
