# Archive Report: pi-runtime-target

**Status**: ARCHIVED  
**Date**: 2026-09-11  
**Change**: pi-runtime-target  
**Artifact Store**: Hybrid (OpenSpec + Engram)

## Cycle Timestamps

| Phase | Timestamp | Notes |
|-------|-----------|-------|
| t0 (proposal launch) | 2026-09-10 13:31:43 | Engram observation #3330 (`sdd/pi-runtime-target/pipeline-state`) created_at |
| t1 (landing commit) | (not yet merged) | This archive is committed inside the delivery chain (PRs #308 → #310 → #311 → #313 → #314) before the merge exists. `landing_commit` is intentionally absent and will be recorded by closure-feedback after the chain merges. |

## Executive Summary

All 5 slices (25 implementation tasks) completed and verified. 12 requirements / 33 scenarios all COMPLIANT (requirements 12/12, scenarios 33/33, no critical findings). Delta specs for `pi-runtime-target` (new), `runtime-lifecycle` (MODIFIED), and `longterm-mem-mcp-registration` (ADDED sections) synced to main specs. Change folder moved to archive.

## Review Lifecycle

| Slice | Work Unit | Review Lineage | Status | Notes |
|-------|-----------|-----------------|--------|-------|
| 1 | pi-target-plumbing (R-001, R-008) | Earlier in chain | Approved | Size exception granted (407 lines, 7 over 400-line budget). Post-review fix lineage `review-ea4451a282f311c9` addressed R4-silent-package-skip (CRITICAL) and regression fallout. |
| 2 | pi-package-build (R-002, R-003, R-010) | `review-8a450066550edf93` → `review-a78a0853d02b76a0` | Approved after correction | One correction cycle; CRITICAL findings remediated and re-verified. |
| 3 | pi-contract-gate (R-004, R-007) | `review-00736b03c4985597` | Approved | Full compliance. |
| 4 | pi-longterm-mem-mcp (R-005) | `review-99842f74d6e8627a` | Approved | Full compliance. |
| 5 | pi-lifecycle (R-006, R-009) | `review-1c85b8d1be41654c` | Approved | Full compliance. Remediation 2 (W-01, W-02, W-03) and Remediation 3 (N-01, S-01, S-02) included. |

## Artifacts Archived

- [x] proposal.md — change framing, scope, rollback plan, affected areas
- [x] specs/pi-runtime-target/spec.md — new spec created and synced
- [x] specs/runtime-lifecycle/spec.md — main spec MODIFIED and delta requirements merged
- [x] specs/longterm-mem-mcp-registration/spec.md — main spec extended with Pi requirements
- [x] design.md — implementation design across five phases
- [x] tasks.md — 25/25 implementation tasks marked complete
- [x] apply-progress.md — batch-wise and remediation implementation updates
- [x] verify-report.md — full verification with 33/33 scenarios COMPLIANT

**Archive location**: `openspec/changes/archive/2026-09-11-pi-runtime-target/`

## Specs Synced

### New Spec: pi-runtime-target

**Source**: `openspec/changes/pi-runtime-target/specs/pi-runtime-target/spec.md`  
**Destination**: `openspec/specs/pi-runtime-target/spec.md`  
**Action**: Created (full spec, not a delta)  
**Requirements**: 4 (all COMPLIANT)
- Pi Accepted as a Valid CLI Target
- Package-Delivered Skills and Agents
- Deterministic Contract Gate via `before_agent_start`
- Honest Status for Unproven Activation
- Pi-Scoped Uninstall

**Scenarios**: 14 (all COMPLIANT)

### Modified Spec: runtime-lifecycle

**Source**: `openspec/changes/pi-runtime-target/specs/runtime-lifecycle/spec.md`  
**Destination**: `openspec/specs/runtime-lifecycle/spec.md`  
**Action**: Merged MODIFIED and ADDED sections  
**Changes**:
- **MODIFIED**: "Existing Runtime Behavior Non-Regression" — extended to include Codex and Pi lifecycle preservation
- **MODIFIED**: "Target Aggregation" — extended to include Pi in `--target all` aggregation and failure masking rules
- **ADDED**: "Pi Target Validation and Dispatch Without a Per-File Copy Path" — new requirement governing Pi's non-copy-path dispatch model

### Extended Spec: longterm-mem-mcp-registration

**Source**: `openspec/changes/pi-runtime-target/specs/longterm-mem-mcp-registration/spec.md`  
**Destination**: `openspec/specs/longterm-mem-mcp-registration/spec.md`  
**Action**: Extended with ADDED sections  
**Changes**:
- **ADDED**: "MCP Registration — Pi" — Pi package-declared `mcp.json` registration, not direct `~/.pi/agent/mcp.json` write
- **ADDED**: "Multi-Target Expansion Treats Pi Like the Other Runtimes" — Pi follows same skip/fail rules as claude/opencode/codex

## Requirements & Scenarios

### pi-runtime-target spec
- **Requirements**: 4 (all COMPLIANT)
  - Pi Accepted as a Valid CLI Target (2 scenarios)
  - Package-Delivered Skills and Agents (2 scenarios)
  - Deterministic Contract Gate via `before_agent_start` (5 scenarios)
  - Honest Status for Unproven Activation (3 scenarios)
  - Pi-Scoped Uninstall (2 scenarios)

- **Scenarios**: 14 (all COMPLIANT)
  - Pi target is recognized
  - Target all includes Pi without masking other failures
  - Local-path install registers the package idempotently
  - Skills are visible in a Pi session and agents ship as package content
  - sdd-tasks and sdd-apply receive both contract path lines
  - Every other agent is excluded
  - Composition with gentle-pi's own handler
  - Contract paths stay contained and malformed frontmatter yields no injection
  - Extension is discovered from the package extensions directory
  - Unproven entry forces partial
  - Honest status per-entry (incomplete registration, missing extension, missing MCP)
  - Pi remove uninstall removes owned state
  - Unrelated Pi state remains intact

### runtime-lifecycle spec (modified)
- **Requirements**: 3 modified/added (all COMPLIANT)
  - Existing Runtime Behavior Non-Regression (extended with Codex/Pi scenarios)
  - Target Aggregation (extended with Pi scenarios)
  - Pi Target Validation and Dispatch Without a Per-File Copy Path

- **Scenarios**: 6 added (all COMPLIANT)
  - Codex lifecycle remains unchanged
  - Target all includes Pi support
  - Target all preserves non-Pi failures
  - Pi passes target validation without a copy-path entry
  - Pi is present in the enum, aggregation, and adapter construction

### longterm-mem-mcp-registration spec (extended)
- **Requirements**: 2 added (all COMPLIANT)
  - MCP Registration — Pi (8 scenarios)
  - Multi-Target Expansion Treats Pi Like the Other Runtimes (2 scenarios)

- **Scenarios**: 10 added (all COMPLIANT)
  - pi-engram's mcp.json stays untouched
  - The registration target is a package-relative mcp.json file
  - The server name is package-prefixed
  - User/project config takes precedence over the package declaration
  - pi-engram init does not drop the registration
  - Registration is skipped on expansion when the package is absent
  - Registration fails when Pi is named explicitly and the package is absent
  - An expansion skips Pi when its package is not installed
  - Pi named explicitly still fails without the package installed

## Implementation Summary

**Total Slices**: 5 (all committed)
**Total Tasks**: 25 (all marked complete: 25/25)
**Implementation Batches**: 4 main batches + 3 remediation batches (C-01/C-02 fixes, W-01/W-02/W-03 fixes, N-01/S-01/S-02 corrections)

### Slice 1: pi-target-plumbing (R-001, R-008)
**Status**: COMMITTED with size exception  
**Budget**: 407 lines (7 over 400-line budget; `size:exception` granted by owner)  
**Key Changes**: Target enum, adapter skeleton, bin script validation, non-regression verified  
**Post-review Fix**: `review-ea4451a282f311c9` addressed R4-silent-package-skip (CRITICAL) and regression fallout

### Slice 2: pi-package-build (R-002, R-003, R-010)
**Status**: COMMITTED  
**Review Path**: `review-8a450066550edf93` → `review-a78a0853d02b76a0` (one correction cycle)  
**Key Changes**: pipkg build, symlink safety, atomic swap, skills registry target

### Slice 3: pi-contract-gate (R-004, R-007)
**Status**: COMMITTED  
**Review**: `review-00736b03c4985597`  
**Key Changes**: before_agent_start extension, path-line injection, composition with gentle-pi

### Slice 4: pi-longterm-mem-mcp (R-005)
**Status**: COMMITTED  
**Review**: `review-99842f74d6e8627a`  
**Key Changes**: Package-declared mcp.json, register --target pi, skip/fail probe semantics

### Slice 5: pi-lifecycle (R-006, R-009)
**Status**: COMMITTED  
**Review**: `review-1c85b8d1be41654c`  
**Key Changes**: Honest status reporting, pi remove uninstall, lifecycle spec updates, README  
**Remediation 2**: W-01/W-02/W-03 fixes (stale exemption removed, status delegation, spec text)  
**Remediation 3**: N-01/S-01/S-02 corrections (aliases, slice reference, proof naming)

## Test Coverage & Verification

**Verdict**: PASS WITH WARNINGS (12/12 requirements, 33/33 scenarios COMPLIANT)
- **Blockers**: 0 (all CRITICAL findings remediated in Remediation 1)
- **Warnings**: 7 total (W-01 through W-07; W-01, W-02, W-03 remediated; W-04 through W-07 treated as follow-ups per explicit out-of-scope decision)
- **Suggestions**: 4 (tracked separately, non-blocking)

**Evidence**:
- All phases: RED tests written first, GREEN implementation passing, TRIANGULATE with multiple scenarios
- TDD cycle: Strict enforcement with test-first discipline per task completion matrix
- Runtime verification: Live Pi 0.85.1 testing (2026-09-11) confirmed package install, extension load, agent discovery, mcp.json registration, uninstall cleanup
- Regression testing: Non-regression verified for claude/opencode/codex targets via shelltest and cmd suites
- Test counts: 25+ new tests across engine/runtime, engine/cmd, engine/shelltest, longterm-mem/register packages

## Size Exceptions Granted

| Slice | Change | Budget | Requested | Granted | Reason | Status |
|-------|--------|--------|-----------|---------|--------|--------|
| 1 | pi-target-plumbing | 400 lines | 407 lines | YES | Cohesive target plumbing including cmd_longterm_mem regression fix, not separable | COMMITTED |

## Follow-Up Work

**Issues Created** (per phase findings):
- #312 — W-04 (visibility follow-up)
- #315 — W-05 (disclosure follow-up)
- #316 — W-06 (discovery follow-up)

**Warnings Deferred to Follow-Up** (per explicit phase instruction):
- W-04 — (scope of issue #312)
- W-05 — (scope of issue #315)
- W-06 — (scope of issue #316)
- W-07 — (scope review deferred)

These are explicitly tracked as out-of-scope for Remediation 1 and Remediation 2 per the verify-report. A fourth remediation batch may be warranted if any W-04 through W-07 are found to be load-bearing.

## Observation IDs Read

This is a hybrid-mode archive. The following artifacts were read from the filesystem (OpenSpec):

- `openspec/changes/pi-runtime-target/proposal.md` (read for scope/approach)
- `openspec/changes/pi-runtime-target/design.md` (read for design details)
- `openspec/changes/pi-runtime-target/tasks.md` (verified: all 25/25 tasks marked complete)
- `openspec/changes/pi-runtime-target/apply-progress.md` (read for batch-wise detail and review lineages)
- `openspec/changes/pi-runtime-target/verify-report.md` (read for verdict and scenario compliance)
- `openspec/changes/pi-runtime-target/entry.json` (read for review_slices and contract validation)
- `openspec/changes/pi-runtime-target/specs/pi-runtime-target/spec.md` (copied to main spec)
- `openspec/changes/pi-runtime-target/specs/runtime-lifecycle/spec.md` (merged to main spec)
- `openspec/changes/pi-runtime-target/specs/longterm-mem-mcp-registration/spec.md` (merged to main spec)

**No Engram artifacts were read in archive phase** (orchestrator is responsible for entry validation; apply-progress and verify-report provided all necessary context for final-state authority per the hierarchy).

## Archive Validation

- [x] All delta specs merged into main specs per OpenSpec convention
- [x] Change folder moved to archive with date prefix (2026-09-11)
- [x] Archive contains all artifacts (proposal, specs, design, tasks, apply-progress, verify-report)
- [x] Archived tasks.md has no unchecked implementation tasks (all 25/25 marked [x])
- [x] Active changes directory no longer has pi-runtime-target
- [x] `diff -r` verification showed no differences between snapshot and archive (mechanical copy success)
- [x] Main spec files updated correctly:
  - `openspec/specs/pi-runtime-target/spec.md` created (copy verified)
  - `openspec/specs/runtime-lifecycle/spec.md` merged (MODIFIED + ADDED)
  - `openspec/specs/longterm-mem-mcp-registration/spec.md` extended (ADDED)

## Metadata

| Field | Value |
|-------|-------|
| Change Name | pi-runtime-target |
| Proposal ID | (see Engram #3330 for t0) |
| Slices Planned | 5 |
| Slices Realized | 5 |
| Total Task Count | 25 |
| Tasks Completed | 25/25 (100%) |
| Requirements Met | 12/12 (100%) |
| Scenarios Passed | 33/33 (100%) |
| Blocker Issues | 0 |
| Critical Findings | 0 (post-remediation) |
| Warnings | 7 (3 remediated, 4 deferred to follow-up) |
| Size Exceptions | 1 (Slice 1, granted) |
| Verification Verdict | PASS WITH WARNINGS |
| Archive Date | 2026-09-11 |
| Archive Path | `openspec/changes/archive/2026-09-11-pi-runtime-target/` |

---

**Prepared by**: sdd-archive executor  
**Mode**: hybrid (OpenSpec + Engram persistence)  
**Skill Version**: sdd-archive/SKILL.md v2.0
