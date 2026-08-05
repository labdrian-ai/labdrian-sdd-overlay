# Archive Report: skills-validate-ondisk-gate

**Date:** 2026-08-05
**Change:** skills-validate-ondisk-gate
**Project:** labdrian-sdd-overlay
**Archive path:** openspec/changes/archive/2026-08-05-skills-validate-ondisk-gate/
**Status:** COMPLETE

## Executive Summary

The change `skills-validate-ondisk-gate` has been fully implemented, verified, and merged to main. All 12 tasks are complete. The on-disk validation gate is now operational in `skills validate`, enforcing that files under `skills/` must have deploying rows in `overlay.manifest`. Two new capabilities have been formally specified and synced to the canonical specs. The change is archived.

## Final State (Per Authority Hierarchy)

### Native Review Authority
- **Receipt:** `sha256:787b2d753fdd57c19c66bdc8161331621a5f0911e95704d6fba7c8a2389c1edf`
- **Lineage:** `review-578b97aed1cb4065`, generation 1, terminal state: **APPROVED**
- **Gate result:** `allow` (reviewGate.result: allow)
- **Candidate tree:** `b654d2d006d4d151145fd6452817f97d1ddc9ebb` (byte-identical to merged main @ 79995ea)
- **Base relationship:** `base_relationship_valid: true`
- **Validation gates:** `gentle-ai review validate --gate=pre-commit` and `--gate=pre-pr` both returned `allow`
- **Risk level:** HIGH (canonical 4-lens review)
- **Frozen paths:** 17 (including delta specs and implementation files)

### Task Completion Gate
- **Persisted tasks artifact:** `openspec/changes/skills-validate-ondisk-gate/tasks.md` (Engram obs #2779)
- **Task completion:** 12/12 complete, 0 unchecked
- **Status:** PASSED — no stale checkbox reconciliation needed

### Verify Report Authority
- **Report observation:** Engram obs #2783
- **Verdict:** PASS (0 critical findings, 0 blockers)
- **Requirements:** 11/11 compliant
- **Scenarios:** 11/11 covered
- **Test suite:** All green (engine, tui, tools/* all passed)
- **Real-tree acceptance:** PASS — `skills validate` exits 0 against actual manifest and `skills/` directory

### Merge Status (Handoff Authority)
- **Merged to main:** YES — commit `79995ea` (PR #130)
- **PR strategy:** Delivered as single PR, not the planned chained PRs (delivery deviation, per handoff)
- **CI status:** All 6 jobs green on PR #130:
  - Engine Tests: PASS
  - TUI Tests: PASS
  - Shell Lint: PASS
  - Deterministic Check Runner: PASS
  - Entry Contract Validator: PASS
  - Review Preflight Guard: PASS

### Key Authoritative Facts (Handoff Overranks Snapshots)
- **One CRITICAL found and corrected:** R4-001 (resilience) — `RenderValidateCore` was printing registry divergences only after three fatal on-disk stages, silently discarding already-computed divergences before exiting. Fixed in commit `c780d5e`, 47 lines against 200-line correction budget. Independently validated.
- **Total authored:** 511 lines (464 pre-review + 47 correction)
- **Delivery deviation recorded:** Originally planned as feature-branch-chain with two PRs; delivered as single PR because review froze all five commits atomically. Recording this for calibration.

## Specs Synced to Main

### NEW Capability: skills-ondisk-validation
- **Location:** `openspec/specs/skills-ondisk-validation/spec.md` (created)
- **Requirements:** 7 (R-002, R-003, R-004, R-005, R-006, R-007, R-011)
- **Status:** All 7 requirements implemented and verified
- **Description:** `skills validate` cross-checks the `skills/` tree against deployable `overlay.manifest` rows, exits 1 on any divergence

### MODIFIED Capability: skill-package-manager
- **Location:** `openspec/specs/skill-package-manager/spec.md` (existing, appended)
- **Added requirements:** 4 (R-001, R-008, R-009, R-010)
- **Pre-existing requirements:** 17 (R-112..R-132, unchanged)
- **Merge verification:** None of the four delta heading names collided with existing headings. All existing requirements preserved with ID lines intact.
- **Status:** All 4 new requirements implemented and verified

## Archive Contents Checklist

| Artifact | Path | Status |
|----------|------|--------|
| proposal.md | archive/.../proposal.md | ✓ Copied |
| design.md (Revision 2) | archive/.../design.md | ✓ Copied |
| tasks.md (12/12 complete) | archive/.../tasks.md | ✓ Copied |
| entry.json | archive/.../entry.json | ✓ Copied |
| apply-progress.md | archive/.../apply-progress.md | ✓ Copied |
| requirements.md | archive/.../requirements.md | ✓ Copied (reference to obs #2769) |
| specs/skills-ondisk-validation/spec.md | archive/.../specs/... | ✓ Created in archive |
| specs/skill-package-manager/spec.md | archive/.../specs/... | ✓ Created in archive |

## SDD Artifact Observation IDs (For Traceability)

- **Proposal:** obs #2776 (Engram topic `sdd/skills-validate-ondisk-gate/proposal`)
- **Spec (Delta):** obs #2777 (Engram topic `sdd/skills-validate-ondisk-gate/spec`)
- **Design (Revision 2):** obs #2778 (Engram topic `sdd/skills-validate-ondisk-gate/design`)
- **Tasks:** obs #2779 (Engram topic `sdd/skills-validate-ondisk-gate/tasks`)
- **Apply Progress:** obs #2780 (Engram topic `sdd/skills-validate-ondisk-gate/apply-progress`)
- **Verify Report:** obs #2783 (Engram topic `sdd/skills-validate-ondisk-gate/verify-report`)
- **Requirements:** obs #2769 (Engram topic `project/labdrian-sdd-overlay/requirements/skills-validate-ondisk-gate`)
- **Entry:** obs #2774 (Engram topic `sdd/skills-validate-ondisk-gate/entry`)
- **Delivery:** obs #2773 (Engram topic `delivery/skills-validate-ondisk-gate`)
- **Estimate:** obs #2770 (Engram topic `sdd/skills-validate-ondisk-gate/estimate`)
- **Pipeline State:** obs #2772 (Engram topic `sdd/skills-validate-ondisk-gate/pipeline-state`)

## Recorded Follow-Ups (For Future Work, Do NOT Fix Now)

Per handoff instructions, these are intermediate findings that do not block archive but should be tracked:

### 1. R-002 Positive Property Coverage Debt (WARNING)
**Source:** verify-report finding W1 (obs #2783)
**Issue:** `engine/skills/skills_test.go:380-417` `cwd_independence_with_absolute_source_root` uses `stubScan(...)` which discards the directory argument. The subtest cannot observe which directory was resolved, so the three `os.Chdir` calls cannot affect the asserted value. The relative `--source-root ../skills` vector shipped at `.github/workflows/ci.yml:158` has no unit equivalent.
**Assessment:** The requirement IS satisfied — no-cwd-fallback is enforced by the complementary `neverScan` stub, and positive resolution is proven at runtime (probes 3, 4). What is missing is a durable regression guard: roughly 3 lines for a recording stub asserting `scanSkills` received the resolved `--source-root` plus one relative-path vector.
**Recommendation:** Low-priority follow-up to add the coverage recording guard. Does not affect shipped behavior.

### 2. Proposal Text Contradicts Design Revision 2 (W2)
**Source:** verify-report finding W2 (obs #2783)
**Issue:** `openspec/changes/skills-validate-ondisk-gate/proposal.md` (now archived) states "Exit-code blast radius is 9 consumers" and lists `tui/run.go` as Modified, while design revision 2 closes the consumer table at 11 rows with TUI as "No change" and the shipped diff touches no `tui/` file.
**Status:** R-001 is COMPLIANT — the enumeration exists and was re-derived. The defect is traceability: two persisted artifacts in the same change folder state different consumer counts. Resolved by archiving: the proposal becomes a historical snapshot, and the design's 11-row enumeration is the authoritative record.
**Recording:** Noted for change-traceability calibration.

### 3. Load Module Dead Code (Out of Scope)
**Source:** Discovered in passing during code review
**Issue:** `engine/skills/load.go:11` `Load` function is unreachable (no callers found in repository)
**Status:** Separate dead-code issue, unrelated to this change. Out of scope for skills-validate-ondisk-gate.
**Recommendation:** File as separate follow-up issue.

### 4. Estimate Calibration Note
**Source:** Handoff final-state facts
**Issue:** Estimate forecast 520-995 changed lines; actual authored was 511. This runs OPPOSITE to the project's historical under-forecast bias (obs #2702, obs #2687).
**Status:** One data point. Worth flagging for estimate model recalibration.

## Delivery Deviation Recorded

**Original plan:** Feature-branch-chain with two chained PRs (slice 1 → slice 2)
**Actual delivery:** Single PR with all five commits atomically frozen and approved
**Reason:** Native review receipt bound all five commits to a single (base_tree, candidate_tree) pair. Merging a first slice would have moved `main` off the frozen `base_tree` and voided the receipt for the second slice. Feature-branch-chain topology is incompatible with atomic receipt binding.
**Impact:** No feature impact. All requirements met, all tasks complete, same tests green. Change scope and shipping timeline unchanged.
**Recorded for:** Delivery strategy calibration in future auto-chain scenarios.

## Archiving Summary

- **Active change folder:** Successfully moved from `openspec/changes/skills-validate-ondisk-gate/` to `openspec/changes/archive/2026-08-05-skills-validate-ondisk-gate/`
- **Canonical specs:** Successfully merged into `openspec/specs/skills-ondisk-validation/` (new) and `openspec/specs/skill-package-manager/` (appended)
- **Main specs NOW reflect:** 11 new requirements across two capabilities, all implemented and verified
- **Backward compatibility:** Existing 17 requirements in skill-package-manager (R-112..R-132) preserved unchanged
- **Archive audit trail:** Complete artifact traceability via observation IDs

## Closure

The SDD cycle for `skills-validate-ondisk-gate` is COMPLETE. The change has been:
✓ Proposed, specified, and designed
✓ Implemented with strict TDD
✓ Verified against all 11 requirements with zero critical findings
✓ Approved by native review (high-risk 4-lens, receipt valid)
✓ Merged to main with all 6 CI gates passing
✓ Archived with full traceability

The on-disk validation gate is now live and enforcing in production. New skills/ files without overlay.manifest rows will fail the `skills validate` gate.

---

**Archive Report Created:** 2026-08-05
**Engram Topic:** sdd/skills-validate-ondisk-gate/archive-report
**Archive Mode:** hybrid (filesystem + Engram)
**SDD Cycle Status:** CLOSED
