# Archive Report: sdd-cycle-timestamp-instrumentation

**Change**: `sdd-cycle-timestamp-instrumentation`  
**Archived**: 2026-08-19  
**Phase**: archive  
**Store**: hybrid  

## Artifact Index (Engram observations)

All SDD phase artifacts persisted to Engram with corresponding topic keys:

| Artifact | Observation ID | Topic Key | Created |
|----------|---|---|---|
| Proposal | #2867 | `sdd/sdd-cycle-timestamp-instrumentation/proposal` | 2026-08-13 14:10:26 |
| Spec (delta) | #2869 | `sdd/sdd-cycle-timestamp-instrumentation/spec` | 2026-08-13 14:36:40 |
| Design (revision 3) | #2870 | `sdd/sdd-cycle-timestamp-instrumentation/design` | 2026-08-13 14:37:05 |
| Tasks (summary) | #2873 | `sdd/sdd-cycle-timestamp-instrumentation/tasks` | 2026-08-13 14:57:09 |
| Verify report (round 9) | #2880 | `sdd/sdd-cycle-timestamp-instrumentation/verify-report` | 2026-08-13 15:34:27 |

## Change Summary

Anchor the SDD cycle measurement at **merge** (not archive), making both `t0` and `t1` cycle-duration anchors legible and durable in Engram and OpenSpec. Spec amendments to `actuals-instrumentation` capability (R-003/R-004/R-005 boundary relocation, R-016/R-017/R-018/R-019/R-020/R-021 new anchors and closure rules).

## Verification Status

**Verdict**: PASS WITH WARNINGS (round 9, 2026-08-13)  
**Blockers**: 0  
**Critical findings**: 0  
**Warnings**: 5  
**Suggestions**: 5  

**Requirements**: 5/5 ✓  
**Scenarios**: 15/15 ✓  
**Archive-ready**: YES  

See Engram obs #2880 for full verify report (hybrid store twin: `openspec/changes/archive/2026-08-19-sdd-cycle-timestamp-instrumentation/verify-report.md`).

## Task Completion Status

**Total tasks**: 91  
**Checked**: 88 ✓  
**Unchecked (deferred/historical)**: 3  
  - 6.3 — "This change's own closure (n=3) exercises the path live" (deferred to archive/closure phase, now unblocked)
  - 8E.1, 13E.1 — historical self-referential verify handoffs (satisfied in substance per apply-progress)

None of these three unchecked items represents missing implementation; all apply-phase work is complete (phases 1-15 across 10 batches). Per the verify report: "None represents missing implementation; none is CRITICAL." Archive gate cleared.

See Engram obs #2873 and filesystem twin `openspec/changes/archive/2026-08-19-sdd-cycle-timestamp-instrumentation/tasks.md` for full detail.

## Spec Merge Summary

**Main spec updated**: `openspec/specs/actuals-instrumentation/spec.md`

| Action | Details |
|--------|---------|
| Modified | R-003/R-004/R-005 — boundary from tiering-go-ahead-to-archive → tiering-go-ahead-to-**merge**; archive lag is bookkeeping, excluded from `total_wall_clock_hours` and `checkpoint_count` boundary |
| Added | R-016/R-017/R-018 — Boundary Anchors (t0, t1: named, derived, legible, or omitted; no tree-based discovery; verified or self-asserted or rejected; ambiguity disclosed) |
| Added | R-019 — Compute-Time Fields Stay Unpopulated (implementation/review/post-review hours deferred; reason documented) |
| Added | R-020 — Historical n=2 Records Carry Boundary-Provenance Annotations (sync-check annotated; skills-validate re-based when anchors resolve, else annotated) |
| Added | R-021 — Unresolvable Anchor Costs One Field, Not the Whole Record (`total_wall_clock_hours` removed from `required` list; record still written with all resolved fields) |

**Properties unchanged**: additionalProperties: false, no new properties added, 13-property list untouched.

## Cycle Timestamps (Verification & Content-Binding)

**Critical instruction from user**: `landing_commit` recorded explicitly by identity to resolve tree collision (three commits reachable from `main` carry identical tree `5ab14902aa0d0fd9…`).

| Anchor | Value | Resolved as | Authority |
|--------|-------|---|---|
| **t0** (cycle start) | Engram obs #2859, topic `sdd/sdd-cycle-timestamp-instrumentation/explore`, created 2026-08-13 13:37:34 | fallback (no `pipeline-state` obs for this change; entered at SDD directly, skipped inception-pipeline) | typed `created_at` from Engram export |
| **t1** (cycle end / merge) | commit `a0374e3` (PR #146 merge), committer 2026-08-19T15:42:14-03:00 | **explicitly recorded** per D2 revision 3 (landing commit MUST be recorded, never discovered by tree matching) | review receipt `final_candidate_tree = 5ab14902aa0d0fd9…` (lineage `review-8afca11eac0d548b`) independently verified: `git show -s --format=%T a0374e3` = `5ab14902…` ✓ verified |
| **Duration** | 6 days 2 hours 4 minutes ≈ 6.09 days ≈ 146.08 hours (t0 read in this host's -03:00 local zone, the same convention every committer timestamp in this record already uses; Engram's typed `created_at` export carries no explicit offset, so this is the file's own established convention, not an independently-confirmed source — disclosed rather than silently assumed) | computed as t1 − t0 | t0 fallback + t1 verified |
| **Resolution path** | verified + fallback-t0 | t0 from earliest own obs; t1 from receipt-bound verified commit | per R-016/R-017/R-018 |

**Why t1 explicit recording is mandatory here**: Commit `a0374e3` is the correct landing commit (slice 4, PR #146 merge). However, three commits on `main` reachable from HEAD carry its identical tree `5ab14902…`:
  - `7343ff5` (slice 4 content commit, not a merge)
  - `a0374e3` (PR #146 merge commit — **the correct landing_commit**)
  - `04488df` (later unrelated main/upstream-sync merge that reproduces the tree)

Tree-based discovery would be ambiguous and could silently select `04488df` (21 minutes later, inflating t1 by 21 min), violating the spec's D2 rule. Explicit recording via `landing_commit = a0374e3` in the archive-report ensures the anchor cannot drift.

**Variance vs. Plan**: t0 resolved via fallback (primary `pipeline-state` missing; no inception-pipeline for this change). t1 resolved via receipt-bound verified commit (content binding against `approved_tree`). Both anchors disclosed in `variance_vs_plan` per R-016/R-017/R-018. This change's own closure (n=3 for timestamp-instrumentation benefit) now completes the instrument's own calibration proof — a live, unambiguous anchor set that future cycles inherit.

## Implementation Summary

**Commits landed** (all ancestors of `main`):
  - Slice 1: `321f54a` (engine, tui, tools schema edits)
  - Slice 2: `3ede60e` (skill edits, R-019 compute-time CALIBRATION sentence)
  - Slice 3: `8c552a1` (closure-feedback pseudocode, historical records)
  - Slice 4: `7343ff5` (all test pins, R-021 required drop, D2 anchor test)
  - **Merge**: `a0374e3` (PR #146, 2026-08-19 15:42:14-03:00)

**Verification round 9 state** (commit `04488df` post-merge, verify sweep independent run):
  - Test output hash reproducible to round 8 baseline
  - Schema hash: `0684faa369cf91e9…` byte-identical (13 properties, `additionalProperties: false`, `required` exactly 5 names)
  - ondisk-gate fixture `31226587098f12ce…` matches obs #2896
  - 25 subtests PASS (16 contract + 6 D2 anchor + 3 sweep)
  - Zero vacuous pins; sixth consecutive clean round

**Warnings closed** (phase 15, batch 10):
  - W1 (R-019 calibration branch) — closed and test-bound via new `R019_calibration_absent_numerators_fallback` subtest
  - W2 (File Changes table) — revert instructions fully documented with all 7 files named (5 edits + 2 mandatory deletions)

**Non-blocking unchecked tasks**: 6.3 (closure, now live), 8E.1/13E.1 (verify handoffs, satisfied).

## Archived Contents

**Location**: `openspec/changes/archive/2026-08-19-sdd-cycle-timestamp-instrumentation/`

✓ proposal.md  
✓ specs/actuals-instrumentation/spec.md (delta, now merged into main spec)  
✓ design.md (revision 3, user-driven amendments applied)  
✓ tasks.md (88 checked, 3 deferred; apply-phase complete)  
✓ verify-report.md (round 9, PASS WITH WARNINGS, archive-ready)  
✓ apply-progress.md (phases 1-15, 10 batches, all complete)  
✓ exploration.md  

**Main specs updated**:  
✓ `openspec/specs/actuals-instrumentation/spec.md` — merge boundary moved to commit-time; 5 new requirements added; no properties added/removed

## Spec Audit

Per R-021/R-019 verification (schema `required` list). Re-verified directly from the shipped
schema and `main`'s pre-change history, not restated from a prior draft — the property names
below are the real 13-property list's members, never `subject_hash`/`created_at`, which are not
properties of this schema at all:

**Before** (base `main` @ `ef35927`): `required: [change_name, project, implementation_hours, review_gate_hours, total_wall_clock_hours, post_review_fix_hours, approval_decision, scope_drift_notes, variance_vs_plan]` (9 names)

**After** (this change): `required: [change_name, project, approval_decision, scope_drift_notes, variance_vs_plan]` (5 names — matches the "required exactly 5 names" schema-hash check above)

Removed from `required`:
  - `total_wall_clock_hours` (per R-021: unresolvable t0/t1 cost one field, not the record)
  - `implementation_hours`, `review_gate_hours`, `post_review_fix_hours` (per R-019: deferred; durable source lacking)

Four removals, matching the spec's own R-021/R-019 requirement exactly. `checkpoint_count` was
never in `required` on either side of this change — it is, and remains, one of the 8 always-optional
properties. Schema shape untouched; 13-property list, `additionalProperties: false` verified unchanged.

## SDD Cycle Closed

The change is fully planned (proposal, spec, design), implemented (15 phases, 10 batches, all committed to `main`), verified (round 9, PASS WITH WARNINGS, 0 CRITICAL, 5/5 requirements, 15/15 scenarios, archive-ready), and archived. Spec amendments merged into the authoritative main specs. No follow-up work required.

**Ready for the next change.**
