# Lightweight Requirements Brief - Restore Skill Registry Scoped Blocks

## Artifact Identity

| Field | Value |
|---|---|
| Project | `labdrian-sdd-overlay` |
| Tier | `3` |
| Execution mode | `auto` |
| Artifact store | `hybrid` (OpenSpec + Engram) |
| Change name | `restore-skill-registry-scoped-blocks` |
| Authoritative OpenSpec path | `openspec/changes/restore-skill-registry-scoped-blocks/requirements.md` |
| Authoritative Engram topic | `project/labdrian-sdd-overlay/requirements/restore-skill-registry-scoped-blocks` |
| Delivery strategy | `force-chained` |
| Chain strategy | `stacked-to-main` |
| Review budget | `800` changed lines |

## Executive Summary

Restore the TUI hook-status health signal by regenerating the three missing scoped contract blocks in `.atl/skill-registry.md` through the repository's existing propagation mechanism. The repair is limited to those generated blocks, must preserve all unrelated registry content, and is complete when `bin/labdrian-overlay status-hooks` exits `0`.

## Lightweight Requirement Object

```yaml
R-NNN: R-001
scope: fix
one_line_impact: Restore a healthy TUI hook status without changing healthy binaries, hooks, contracts, target synchronization, or unrelated registry content.
acceptance_evidence: The three generated scoped blocks each occur once, the registry diff contains no unrelated content changes, and bin/labdrian-overlay status-hooks exits 0.
change_name: restore-skill-registry-scoped-blocks
```

## R-001 - Restore Generated Registry Scope Blocks

**Scope:** fix
**Type:** fix
**Size:** small
**Priority:** high
**Keywords:** TUI degraded status, status-hooks, skill registry, scoped blocks, propagation, minimalism-contract-scope, skill-discovery-safety-scope, anti-generic-design-scope
**Source anchor:** "Repair must restore only the three generated scoped blocks through the repository's authoritative propagation mechanism, preserve unrelated registry content, and verify `status-hooks` returns 0."
**Intent:** Remove the false degraded TUI state by restoring generated registry state rather than changing healthy runtime components or status behavior.
**Requirement:** WHEN the authoritative registry propagation mechanism targets `.atl/skill-registry.md`, the overlay SHALL restore exactly the `minimalism-contract-scope`, `skill-discovery-safety-scope`, and `anti-generic-design-scope` generated blocks while preserving all unrelated registry content.

## Acceptance Criteria

- **AC-001 - Generated blocks:** GIVEN the current registry lacks all three required scoped blocks, WHEN the existing propagation mechanism runs for the three contracts, THEN each corresponding BEGIN/END marker pair and generated row is present exactly once.
- **AC-002 - Content preservation:** GIVEN the pre-repair `.atl/skill-registry.md`, WHEN the repaired file is compared with it, THEN the only content additions or replacements are inside the three generated scoped blocks.
- **AC-003 - Health signal:** GIVEN all three generated blocks are present, WHEN `bin/labdrian-overlay status-hooks` runs, THEN it exits `0` and does not report any scoped block as missing.
- **AC-004 - Healthy surfaces remain healthy:** GIVEN the engine binary, installed hooks, contracts, and target synchronization are already healthy, WHEN the repair is complete, THEN no rebuild, restart, hook rewrite, contract rewrite, or synchronization repair has occurred.

## Scope Boundaries

### In Scope

- Regenerate only the `minimalism-contract-scope`, `skill-discovery-safety-scope`, and `anti-generic-design-scope` blocks in `.atl/skill-registry.md`.
- Use the existing engine propagation path and its marker-based, idempotent replacement behavior; do not hand-author generated rows.
- Preserve all registry content outside those generated marker ranges.
- Verify the repair with `bin/labdrian-overlay status-hooks` and an exit status of `0`.

### Non-Goals

- Do not rebuild or restart the engine binary.
- Do not reinstall, rewrite, or redesign hooks or contract files.
- Do not modify propagation, status-check, or TUI implementation code.
- Do not investigate, reduce, or suppress the expected `upstream..main` UX diff noise.
- Do not change Claude, OpenCode, or Codex synchronized files.
- Do not perform unrelated cleanup in `.atl/skill-registry.md`.

## Traceability

| Source or Evidence | Observation | Requirement / Criterion |
|---|---|---|
| User request | Restore only the three named generated blocks via authoritative propagation and preserve unrelated registry content. | `R-001`, `AC-001`, `AC-002` |
| Reproduced `bin/labdrian-overlay status-hooks` output | Binary, UserPromptSubmit hook, PreToolUse hook, and contract report `OK`; registry reports the three scoped blocks missing. | `R-001`, `AC-003`, `AC-004` |
| Verified diagnosis: `bin/labdrian-overlay status --target all` | Exits `0`; Claude, OpenCode, and Codex files are synchronized. | `AC-004`, non-goals |
| `.atl/skill-registry.md` marker search | No BEGIN marker exists for any of the three required block IDs. | `AC-001` |
| `engine/cmd/main.go:1435-1457` | `status-hooks` is healthy only when all three marker blocks exist and otherwise returns the degraded missing-block warning. | `R-001`, `AC-003` |
| `engine/propagator/propagator.go:206-233` | `Propagate` replaces its own marker block or appends the generated block while leaving foreign blocks untouched. | `R-001`, `AC-001`, `AC-002` |
| Engram observation `#4539` | Prior reproduction records `status-hooks` exit `2`, healthy runtime components, and the expected out-of-scope `upstream..main` noise. | `AC-003`, `AC-004`, non-goals |

## Project-Inception Handoff

| Order | Requirement ID | Requirement Size | Change Type | Keywords | Change Name | Dependency | Minimum PASS Evidence |
|---:|---|---|---|---|---|---|---|
| 1 | `R-001` | Small | Fix | status-hooks, skill registry, scoped blocks, propagation, minimalism-contract-scope, skill-discovery-safety-scope, anti-generic-design-scope | `restore-skill-registry-scoped-blocks` | Existing engine propagation path and current `.atl/skill-registry.md` | Three unique generated blocks; unrelated registry content unchanged; `bin/labdrian-overlay status-hooks` exits `0` |

## Risks

- Invoking a broader install or synchronization workflow could mutate healthy surfaces. The downstream repair must use the narrow existing propagation path against the project registry.
- Manual registry edits could drift from contract metadata or duplicate marker blocks. Generated content must come from the authoritative propagator.
- Treating the large `upstream..main` diff as part of this repair would expand scope without addressing the degraded health signal.
