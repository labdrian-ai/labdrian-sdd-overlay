# Manifest — Final: labdrian-sdd-overlay

## Context Document

### Nature of the project
`labdrian-sdd-overlay` is an infrastructure repository that extends `gentle-ai` into a long-running overlay so local customizations survive upstream updates. It is not a runtime end-user application; it is an internal developer tooling platform for skill, agent, and workflow governance.

### Universe and scope
The project covers:
- A Go control-plane engine (`engine/`) for deterministic scoping, hook mediation, skill and manifest operations.
- A BubbleTea terminal interface (`tui/`) that exposes the overlay status/apply/sync workflow.
- Managed and custom skill artifacts under `skills/`, tracked through `overlay.manifest`.
- Deployment and synchronization tooling (`bin/overlay`, `bin/labdrian-overlay`) to apply overlays to `~/.claude/skills`, `~/.config/opencode/skills`, and `~/.codex/skills`.

### Conceptual core
The architecture is built around a two-branch model and deterministic projection:
1. `upstream`: pristine vendor baseline.
2. `main`: operator overlay + local customizations.

All updates must compose these layers without overwriting the user’s intentional custom rules.

### Current state of knowledge
- Tech stack: multi-module Go repository with modules `engine` and `tui`.
- Detection from bootstrap config shows command semantics are module-scoped (`cd engine`, `cd tui`) for test and vet.
- OpenSpec config already defines strict testing/linting commands and change-tracking policies.
- Existing active/archived changes include `skill-package-manager`, `skill-lifecycle`, `skill-manifest-gen`, `skill-external-provenance`, `gadu-portable-operator`.

### Operational assumptions
The project assumes that deterministic control surfaces are preferable to free-form manual editing of generated or vendor files, because drift and silent override are high-risk in multi-runtime installations.

### Central problem
Without explicit boundaries and sequencing, overlay work tends to regress into ad-hoc skill edits, brittle manual merges, and hidden drift against upstream updates.

### Design constraints
- Must preserve the overlay workflow in a way that is reproducible across Claude Code, opencode, and codex runtimes.
- Must keep skill deployment and manifest synchronization deterministic.
- Must keep governance rules for SDD phases (`sdd-tasks`/`sdd-apply`) scoped, not global.

### Strategic modules
- `engine`: deterministic CLI core (commands: propagate, gate-task, merge-settings, status, prespec, skills, etc.).
- `tui`: command front-end and dashboard.
- `skills` registry and manifest: source of truth for installed behaviors.
- CI/verification: repository gates (`go test`, `go vet`, `shellcheck`) to maintain platform stability.

### Strategic risks
- Upstream `gentle-ai` changes can conflict with local behavior if merge/apply is not enforced.
- Scope leakage of governance contracts can break non-SDD tool flows.
- Branch divergence or merge conflicts in tracked skill artifacts can break reproducibility.

### Evolution path
Immediate evolution remains `project-manifest ->
architect -> roadmap`, then incremental SDD-backed changes and archive-driven maintenance.

### Guiding principle
Treat overlay behavior as policy infrastructure: small, explicit, versioned, and recoverable.
