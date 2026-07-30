# Archive Report: Actuals Calendar-Time / Checkpoint-Count Instrumentation Fix

**Change**: actuals-calendar-checkpoint-instrumentation | **Project**: labdrian-sdd-overlay | **Archived**: 2026-07-30 | **Artifact store**: hybrid (OpenSpec + Engram)

## Executive Summary

Change completed and verified. The actuals-record measurement instrument has been corrected across schema, estimator, writer, and one historical Engram observation. All 9 requirements and 13 scenarios passed verification (PASS WITH WARNINGS, 0 blockers, 0 critical findings). All 16 implementation tasks are checked and substantively complete. Spec merged to main specs. Change folder archived with date prefix.

## Artifacts Migrated to Archive

### OpenSpec Artifacts
- Archive location: `openspec/changes/archive/2026-07-30-actuals-calendar-checkpoint-instrumentation/`
- **proposal.md**: Intent, scope, approach, risks, rollback plan
- **design.md**: Technical approach, architecture decisions D1-D8, content gate specification, data flow
- **tasks.md**: 6 work units across 6 phases (RED gate, fixture, schema, estimator fix, closure-feedback, Engram upsert), all checked
- **verify-report.md**: Verification outcome PASS WITH WARNINGS; all 4 prior CRITICAL blockers resolved; 13/13 scenarios compliant; 9/9 requirements met
- **specs/actuals-instrumentation/spec.md**: Full spec with 9 requirements and 13 scenarios; 15 requirement IDs R-001..R-015 traced

### Engram Artifacts (Observation IDs)
- **Proposal**: obs #2538 (2026-07-29 18:18:43)
- **Spec**: obs #2539 (2026-07-29 18:24:17)
- **Design**: obs #2540 (2026-07-29 18:28:12)
- **Tasks**: obs #2541 (2026-07-29 19:37:24)
- **Verify-report**: obs #2557 (2026-07-29 20:53:01)

### Main Specs Updated
- **Created**: `openspec/specs/actuals-instrumentation/spec.md` — full specification for three-unit actuals measurement contract

## Verification Summary

**Verdict**: PASS WITH WARNINGS (per verify-report obs #2557, evidence_revision: sha256:961d3a50e65c1bf2cadcd5a5455cdf6660ddba28770cbc7c4de415ee5b6c018c)

| Metric | Result |
|--------|--------|
| Blockers | 0 |
| Critical findings | 0 |
| Requirements | 9/9 compliant |
| Scenarios | 13/13 passing |
| Tasks checked | 16/16 |
| TDD: RED-capable groups | 8/10 (2 invariants not RED) |
| TDD: GREEN confirmation | Yes (all 10 subtests passing, full suite exit 0) |
| Build/test status | Pass (engine 81.0% coverage, tui 79.3% coverage) |

## Blocker Resolution

All four previously CRITICAL blockers were resolved between apply and archive:

1. **Live obs #2096 contradiction** (observed 11/1/10 vs fixture 12/2/10): RESOLVED — verified byte-identical (3535 bytes, sha256:cd630b73a1ec3e900d907814df11a8cf7ed626b914ee5270516f937865a6c8a0) to committed fixture
2. **Apply-progress #2542 missing Cycle Evidence table**: RESOLVED — revision 2 carries 10-row table with RED-capable column; every claim independently reproduced from `git show`
3. **R-001/R-002 no covering assertion**: RESOLVED — `R001_R002_three_units_never_blended` is RED-capable (0→1 markers)
4. **R-012/R-013 no covering test**: RESOLVED — `R012_actuals_output_separate_labels` is RED-capable; R-013 invariant honestly labeled as structurally-GREEN (negative obligation, no edit per D6)

## Final State Facts

**Merged commit**: 0094afd ("fix(sdd): remediate actuals verification blockers")
- 51 changed lines in 2 files
- 143-line replaced verify-report (this archive is from revised report obs #2557)

**Review authority**: Two approved receipts
- `review-ca8ba8bf608f4b25` — reliability lens, 0 SEVERE (remediation candidate, 199 lines)
- `review-actuals-remediation-successor` — scope_changed successor, reliability lens, 0 BLOCKER/CRITICAL (full 755-line base-diff)

**Engram correction**: obs #2096 `sdd/sync-check-repo-behind-origin/actuals` upserted with `checkpoint_count: 12` (1 tiering go-ahead + 1 AMB-001 ambiguity + 10 reconstructed from narrative) and `total_wall_clock_hours: 36` (reconstructed from closure narrative), accompanied by mandatory provenance disclaimer

**Checkpoint re-derivation** (independent verification):
- 1 tiering go-ahead (durable, pipeline-state #2073 recorded)
- 1 AMB-001 ambiguity question (durable, pipeline-state #2073 recorded)
- 3 proposal-decision confirmations (reconstructed from narrative)
- 4 judgment-day rounds (reconstructed from narrative)
- 1 chained-PR split confirmation (reconstructed from narrative)
- 1 merge authorization (reconstructed from narrative)
- 1 ACTION-hint fix scope (reconstructed from narrative)
- **Total**: 12 checkpoints, 2 durable + 10 reconstructed

## Specification Coverage

The archived spec defines the three-unit actuals measurement contract:

| Unit | Coverage |
|------|----------|
| **Agent-compute-time** | Baseline built from 3 phase fields only (implementation_hours, review_gate_hours, post_review_fix_hours); `total_wall_clock_hours` explicitly excluded (R-009) |
| **Elapsed-calendar-time** | Captured independently, includes interruption gaps, shared tiering-go-ahead→archive boundary with checkpoint_count, corrected in place in schema (R-003/004/005) |
| **Human-confirmation-checkpoint-count** | One unit per round-trip reply (R-015), durable floor guaranteed (R-006), non-durable checkpoints disclosed in `variance_vs_plan` prose (R-007/008) |

**Delivery window formula**: checkpoint_count × round-trip_latency + fixed_interruption_allowance (R-010); latency calibration uses only interruption-clean samples (R-011); until explicit interruption evidence exists, allowance remains uncalibrated and non-scaling (D7)

**Roadmap coupling**: No tracking-line source field matches (R-013, per D6 no-edit decision); only prose ownership attribution remains

## Design Decisions Confirmed

| D | Decision | Status |
|---|----------|--------|
| D1 | Correct `total_wall_clock_hours` in place; description boundary rewritten | Confirmed — schema closed, zero new properties |
| D2 | `checkpoint_count` stays single field; durable-vs-reconstructed split in prose | Confirmed — zero new properties; prose disclosure in `variance_vs_plan` |
| D3 | Content gate = committed Go test `actuals_instrumentation_contract_test.go` | Confirmed — 232-line test, 10 groups, RED-capable, reuses `readRepoFile` |
| D4 | Fixture-driven R-014 correction with byte-for-byte compare | Confirmed — live obs #2096 byte-identical to fixture |
| D5 | Schema bundle stays 2.0.0, no version bump | Confirmed — `$id` unchanged, no record invalidated |
| D6 | `roadmap-maker/SKILL.md` receives no edit | Confirmed — file byte-identical to `main`; R-013 negative obligation honored |
| D7 | Formula shape only; interruption-clean gating; fixed uncalibrated allowance | Confirmed — all present and pinned; n=0 disclosure with bootstrap Low-confidence |
| D8 | Estimator fix precedes Engram upsert | Confirmed — commits d51a042/66bb084/508736b all predate upsert by 02:45:18 timestamp |

## Outstanding Non-Blocking Items (for Future Workflow)

The archive captures the change as complete per its requirements; the following are recorded as observations but NOT as incomplete work:

- **W-3 in verify-report**: tasks.md lines 37, 40, 46, 56, 57 prescribe superseded arithmetic (11/1/10, three-sample allowance). Historical record preserved; normalize through owning workflow when next touched, not silently here.
- **W-4 in verify-report**: inception-pipeline scopes durable ambiguity to "rule 4 fired", but schema/spec say "if fired" with no restriction. sync-check-repo-behind-origin recorded rule 1 ambiguity durably. Reconcile in future change when broadening ambiguity capture.
- **Schema invariant weakness** (W-1): Closed-schema invariant test does not assert `additionalProperties: false` directly. Verified independently; schema remains closed. Add assertion in future refactor.
- **Marker precision** (W-2, W-5): Content assertions are file-scoped; some markers replicable across sections; fixture pins weaker than designed. Not spec violations; consider tightening drift guards in next cycle.
- **Dead RED-era branch** (W-6): `D2_D5_closed_schema_invariant` silently `return`s when the fixture is unreadable, self-disabling half that group. Absence is still caught loudly by group 6's `readRepoFile`, so no scenario fails; the dead branch should be deleted. (Not resolved in the archived candidate — addressed in a follow-up commit on this branch.)
- **Future roadmap** (R-013 scope note): Extending `roadmap-maker` tracking-line template with dedicated calendar-time and checkpoint-count slots is deliberately OUT OF SCOPE, deferred to future change.

## Artifact Checksum Summary

| Artifact | Type | Final state |
|----------|------|-----------|
| Spec | Full specification, 9 requirements, 13 scenarios | Committed to `openspec/specs/actuals-instrumentation/spec.md` |
| Fixture | Engram obs #2096 & test fixture | 3535 bytes, sha256:cd630b73a1ec3e900d907814df11a8cf7ed626b914ee5270516f937865a6c8a0 |
| Test gate | Go contract test | 232 lines, 10 groups, all subtests GREEN |
| Build/test suite | Real execution | Exit 0; engine 81.0% coverage, tui 79.3% |
| Verify evidence | Revision hash | sha256:961d3a50e65c1bf2cadcd5a5455cdf6660ddba28770cbc7c4de415ee5b6c018c |

## Task Completion Status

All 16 implementation tasks across 6 phases marked complete and substantively verified:

- Phase 0 (RED gate): Test written, baseline confirms RED on 8 groups
- Phase 1 (Fixture): Fixture created with corrected 12-checkpoint count
- Phase 2 (Schema): Description edits in place (zero new properties)
- Phase 3 (Estimator): CALIBRATION baseline rewritten, formula shape defined
- Phase 4 (Inception): Closure-feedback updated with round-trip unit and prose provenance
- Phase 5 (GREEN gate): All subtests GREEN, full suite exit 0
- Phase 6 (Engram upsert): Obs #2096 corrected with fixture bytes

## SDD Cycle Complete

This change has been fully planned (proposal, design, tasks), implemented (files committed, go test suite GREEN, Engram upsert complete), verified (PASS WITH WARNINGS, 0 blockers), and archived (spec merged to main specs, change folder relocated with date prefix).

The three-unit actuals measurement contract is now codified in `openspec/specs/actuals-instrumentation/` and ready for use by downstream SDD phases (proposal, estimation, roadmap reporting).

---

**Archive report persisted**: OpenSpec (this file) + Engram topic `sdd/actuals-calendar-checkpoint-instrumentation/archive-report` (obs #TBD after mem_save)

**Next recommended**: None — change is complete and closed.

**No blocking risks**: All 0 CRITICAL findings resolved; 6 WARNINGs recorded as observations for future workflow, not blockers.
