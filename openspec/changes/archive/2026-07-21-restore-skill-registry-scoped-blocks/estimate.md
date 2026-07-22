# Pre-Start Estimate - Restore Skill Registry Scoped Blocks

## Estimate Identity

| Field | Value |
|---|---|
| Status | `PASS` |
| Change name | `restore-skill-registry-scoped-blocks` |
| Tier | `3` |
| Change type | Fix |
| Execution mode | `auto` |
| Artifact store | `hybrid` (OpenSpec + Engram) |
| Delivery strategy | `force-chained` |
| Chain strategy | `stacked-to-main` |
| Review budget | `800` changed lines per review slice |
| Complexity | Low |
| Confidence | High (approximately 80%) |
| Planned range | `3.0-5.0` hours |
| Suggested delivery window | One half-day of active work; allow one business day end-to-end for human approval availability |

The planned range includes implementation, verification, local review-gate preparation, human review, likely review corrections, and contingency. It excludes remote push/PR delivery, binary rebuilds, hook changes, runtime synchronization, and TUI work.

## Executive Summary

This is a small generated-state repair with a well-bounded implementation surface and an existing, tested propagation mechanism. The implementation should modify only `.atl/skill-registry.md` by adding the three absent marker-delimited blocks and should require approximately 9 generated content lines. Most of the elapsed effort is SDD evidence, strict preservation checks, forced chained-review preparation, and human approval rather than implementation.

## Evidence Base

- `requirements.md` limits the repair to three named blocks, one generated registry file, preservation of unrelated content, and `status-hooks` exit `0`.
- `roadmap.md` confirms that the change is independent of the later skill-lifecycle hardening chain and forbids broad install/synchronization recovery.
- `.atl/skill-registry.md` currently contains none of the three scoped marker blocks and has no `Shared Contracts` section.
- `engine/propagator/propagator.go` builds each scoped block as exactly three lines: BEGIN marker, generated row, and END marker. Its fallback appends a missing block while preserving existing content, and subsequent runs are idempotent.
- `engine/cmd/main.go` exposes the narrow `propagate` command, owns distinct marker pairs for `skill-discovery-safety` and `anti-generic-design`, and reports hook status as healthy only when all three markers exist.
- Existing propagator and command tests cover missing-block append behavior, distinct markers, idempotency, and foreign-content preservation. No implementation-code change or new binary is expected.
- No project estimation-calibration record was available, so the range is based on current repository evidence rather than historical hour variance.

## Scope Interpretation

### Included

- Produce the standard Tier 3 SDD planning artifacts needed before apply.
- Invoke the existing narrow propagator path for the three authoritative contracts against `.atl/skill-registry.md`.
- Verify each BEGIN marker, END marker, and generated row occurs exactly once.
- Verify the registry diff contains no unrelated changes.
- Verify a second propagation pass is idempotent or otherwise prove the existing idempotent behavior against the resulting file.
- Run `bin/labdrian-overlay status-hooks` and require exit status `0` with no missing scoped-block report.
- Prepare the forced chained, stacked-to-main review gate and account for human review through approval.

### Excluded

- Binary rebuild or restart.
- Hook installation, rewrite, or redesign.
- Propagator, status-check, contract, or TUI code changes.
- Claude, OpenCode, or Codex runtime synchronization.
- Investigation of expected `upstream..main` diff noise.
- Remote push, PR creation, merge, or deployment delivery in this stage.

## Estimation

| Activity | Estimated hours | Notes |
|---|---:|---|
| Functional and technical confirmation | 0.30-0.45 | Confirm exact command inputs, baseline marker absence, and preservation boundary. |
| SDD proposal, spec, design, and tasks | 0.60-1.00 | Lightweight artifacts for one requirement and four acceptance criteria; no discovery spike expected. |
| Registry implementation | 0.20-0.35 | Run the existing propagator three times with distinct contract inputs; no hand-authored rows or code changes. |
| Verification and evidence capture | 0.40-0.65 | Unique marker/row counts, scoped diff review, idempotency evidence, and `status-hooks` exit `0`. |
| Review-gate and chain preparation | 0.35-0.60 | Keep generated repair and SDD evidence reviewable under forced stacked-to-main delivery; remote delivery remains excluded. |
| Human review and approval | 0.35-0.75 | Review the generated provenance, exact diff boundary, and command evidence. |
| Likely review corrections and closure evidence | 0.20-0.40 | Allows one narrow correction/evidence pass before approval. |
| Base subtotal | **2.40-4.20** | Includes gate and human-review time. |
| Contingency | **0.60-0.80** | Approximately 19-25%; protects against propagator path/registry-shape mismatch, regenerated-row drift, or one review retry. |
| Planned total | **3.00-5.00** | Approval-inclusive estimate; complexity Low, confidence High. |

## Changed-Line Forecast

| Review surface | Forecast | Rationale |
|---|---:|---|
| Generated registry implementation | 9-12 added lines, 0-3 removed lines | Three blocks contain three generated lines each; a small newline/placement adjustment is possible because the registry currently lacks a `Shared Contracts` section. |
| Pre-archive SDD planning and verification artifacts | 220-360 changed lines | One lightweight requirement, narrow design/tasks, and concise verification evidence. |
| Archive/closure artifacts if reviewed in the same delivery chain | 80-140 changed lines | Delta synchronization and closure reports, with no runtime or code documentation expansion. |
| Total anticipated review workload | 309-512 changed lines | Expected to remain below the 800-line budget in every forced review slice. |

The implementation delta alone is far below any size-based chaining threshold. Chaining is retained solely because the entry contract forces it.

## Review Workload Forecast

The forced stack should remain two small, focused review units:

1. Generated registry repair and direct acceptance evidence: approximately 9-12 implementation lines plus concise command output; expected human review time 15-25 minutes.
2. SDD verification and closure artifacts: approximately 300-500 documentation lines across the lifecycle; expected human review time 10-20 minutes when traceability remains one-to-one with R-001 and AC-001 through AC-004.

No review slice is forecast to approach the 800-line budget. If SDD artifacts exceed the upper forecast, the correct response is to tighten repetitive evidence, not broaden or further fragment the implementation.

## Assumptions

- The current deployed `bin/labdrian-overlay` already contains the propagator and all three marker definitions reflected in current source.
- The three contract files and embedded contract assets parse successfully with their current frontmatter.
- The registry remains in its observed non-empty state and no concurrent refresh rewrites it during apply.
- The narrow propagator command can target the repository registry without invoking installation or synchronization workflows.
- No proposal, spec, design, or tasks artifact exists yet for this change.
- Human review begins without a long scheduling delay; calendar waiting beyond reviewer availability is not counted as active effort.
- Remote delivery is deferred, even though local review preparation follows the forced chained and stacked-to-main contract.

## Dependencies

- Existing `engine propagate` command and marker-based idempotent replacement behavior.
- Current `.atl/skill-registry.md` and the three authoritative contract definitions.
- Existing executable `bin/labdrian-overlay` for the final read-only health check.
- Reviewer availability for final approval.

## Risks

| Risk | Impact | Mitigation represented in estimate |
|---|---|---|
| A broad install or synchronization command mutates healthy surfaces | High | Require the narrow propagator path and inspect the complete registry diff before approval. |
| Generated output differs from the expected three-line block shape | Medium | Count markers and rows, inspect source-derived content, and use contingency for one correction pass. |
| A concurrent registry refresh overwrites or broadens the repair | Medium | Before rollback, compare current bytes/hash with the transaction's last written snapshot; restore only on equality, otherwise preserve current bytes and report manual recovery. |
| `status-hooks` remains degraded after markers are restored | Medium | Stop rather than rebuilding or rewriting hooks; use contingency to diagnose invocation/content mismatch within scope. |
| Forced chaining creates disproportionate process overhead | Low | Keep two focused review units and avoid artificial implementation slicing. |

## Recommendation

Proceed through the normal lightweight Tier 3 SDD path, then apply the existing propagator without modifying source code. Treat any need for a binary rebuild, hook rewrite, runtime sync, TUI edit, or registry cleanup as a scope break requiring re-estimation. The expected active effort is `3.0-5.0` hours through human approval, with a one-business-day delivery window when review availability is included.
