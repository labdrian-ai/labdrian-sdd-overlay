# Technical Architecture — labdrian-sdd-overlay

## 1) Executive Summary
This project is an overlay orchestration system around `gentle-ai` that composes a Go-backed control plane (`engine`) with a text UI (`tui`) and declarative manifest-based deployment to multiple AI runtimes.

## 2) Architectural Style
Hybrid control-plane + thin UI. The `engine` owns enforcement and mutations; the `tui` is a presentation and workflow driver. Runtime integrations are declarative through `overlay.manifest` and deterministic registry checks.

## 3) System Context and boundaries
- **Inputs**: user CLI commands (`labdrian`, `labdrian-overlay`, `bin/overlay`) and repository state.
- **Boundary**: command parsing and execution in `engine/cmd` and modules under `engine/*`.
- **UI boundary**: BubbleTea frontend (`tui/*`) calling the CLI.
- **State boundary**: git-tracked files in repo, `.claude/settings.json`, generated skill registry caches.
- **External dependencies**: shell filesystem, go toolchain, shellcheck, external shell env.

## 4) Core Modules and Contracts
- **engine/cmd**: command entry and dispatch for `propagate`, `gate-task`, `merge-settings`, `uninstall-hooks`, `status`, `prespec`, `skills`, `gadu-generate`.
- **engine/gate**: task/agent hook guard and scoped marker injection contract.
- **engine/propagator**: registry row insertion and marker block generation for scoped safety rules.
- **engine/settings**: merge/uninstall settings JSON with backup and atomic write guarantees.
- **engine/skills**: load, validate, install, sync, manifest reconciliation, and lifecycle metadata handling.
- **engine/prespec**: readiness/lint/rank/brief primitives for pre-spec workflow.
- **engine/gadu**: canonical persona-to-skill/agent generation workflow.
- **tui**: dashboard/navigation layer that invokes CLI commands and renders statuses.

## 5) Data Model and Contracts
- `overlay.manifest`: ordered list of tracked files with management type/source scope.
- Skill registry rows and `skills.registry.yaml` include provenance metadata and deployment scope.
- `.atl/skill-registry.md` stores resolved skill index and optional auto-generated rules sections.
- `openspec/config.yaml` stores SDD test and policy rules.

## 6) Invariants
- `overlays` are non-destructive: overlay changes cannot silently mutate unrelated files.
- Settings operations must be atomic, preserve unrelated keys, and be idempotent where possible.
- Hook contracts must not corrupt user JSON configuration.
- All command output should remain actionable and deterministic.

## 7) Deployment Model
- `overlay.manifest` defines managed/custom entries.
- `overlay apply` regenerates deployments to runtime targets.
- `overlay status/sync-check` verify drift and update suggestions.
- Hooks (`~/.claude/settings.json`) are optional but deterministic when enabled.

## 8) Security and Safety
- No remote fetch/execution in `engine` skill management; external repos are inert metadata only.
- Settings mutation uses backup-first semantics and explicit validation.
- Hook operations are reversible via uninstall flows.

## 9) Performance and Scalability
- No large data-plane load; performance critical path is command latency and text output scanning.
- Complexity remains linear in manifest size and filesystem scan scope.
- Module tests provide early failure for regressions.

## 10) Observability
- Command logs/errors and status exit codes communicate health states.
- Exit codes are semantic and used by CI/automation to detect drift and failure.
- Test and CI output is ground truth for verification loops.

## 11) Risks and Mitigations
- Upstream behavior drift: mitigate with periodic sync/capture/apply and archiveable change history.
- Scope leakage: mitigate with strict marker-based hook boundaries.
- Merge conflicts: mitigate with managed/custom separation in manifest and deterministic apply workflows.

## 12) Trade-offs
- The dual-module Go layout introduces duplicated tool setup overhead but avoids mixed-module command confusion and keeps compilation boundaries clean.
- A deterministic shell/Go hybrid model is more verbose than a pure Go or pure shell approach, but safer for cross-platform developer tooling.

## 13) Discarded alternatives
- **Single-root Go module** was rejected because tooling commands and vendored assets are naturally separated and command ownership becomes ambiguous.
- **GUI-only controls** was rejected because deterministic CLI is needed for automation and CI hooks.

## 14) Evolution roadmap
- Consolidate the manifest, architecture, and roadmap workflow in OpenSpec as the versioned project foundation.
- Expand acceptance tests for hook failure modes before merging runtime behavior changes.
- Preserve minimalism of contracts while improving incremental merge ergonomics.

## 15) Compliance matrix
- **Manifest context/rules used**: Multi-runtime overlay scope, deterministic merge model, hook scoping constraints, module-aware validation requirements.
- **Technical rules observed**: No direct upstream edits, module boundary preservation, registry-driven deployment, fail-loud error handling.
- **Evidence**: `README.md`, `overlay.manifest`, `engine` command modules, `tui` front-end, and tests in `engine/*`, `tui/*`.
- **Gaps**: none at the architectural level for this project scope; operational scope changes require manifest/SDD updates before execution.
