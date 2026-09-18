# Archive Report: Procedural Candidate Detection

**Change**: procedural-candidate-detection
**Archived**: 2026-09-18
**Archive Path**: `openspec/changes/archive/2026-09-18-procedural-candidate-detection/`

## Cycle Timestamps

| Phase | Timestamp | Notes |
|-------|-----------|-------|
| t1 (landing commit) | n/a | `landing_commit` is absent: this change is not yet merged. The archive commit ships inside the final stacked PR #351 (after #333, #347, #349), so no landing commit exists when this report is written. The four review lineages (review-1c5f059a2e4f88bf, review-dc6b86d85945737b, review-1cdba131c496cc29, review-c2926c5a9630236c) were approved and acknowledged, but their receipts were not persisted under `review-receipts/` before acknowledgement, so this report records no `approved_tree`. |

## Promotion and Archive Summary

This change introduced the procedural promotion-candidate detection capability: detection and durable storage of repeated-success patterns and failure-recovery patterns as the first slice of the umbrella procedural-memory-skill-promotion effort. The implementation is complete across all four phases (candidate store, repeat/recovery detection, duplicate rejection, full verification).

### Specs Promoted

**Delta Spec Promoted to Main Spec**:
- Source: `openspec/changes/procedural-candidate-detection/specs/procedural-candidate-detection/spec.md`
- Destination: `openspec/specs/procedural-candidate-detection/spec.md`
- Status: Created (no pre-existing main spec; delta was a full specification)
- Verification: Mechanical copy with `cp -R`, verified by `diff -r` (empty diff)

The promoted spec defines four requirements:
- R-001: Durable Promotion-Candidate Store (Engram-backed, sub-step granularity identity)
- R-002: Repeated-Success Detection (emit at threshold N, no duplicates after)
- R-003: Failure-and-Recovery Detection (same failure + same recovery → candidate)
- R-004: Duplicate Candidate Rejection (read-only registry match via ParseRegistry)

No ADDED/MODIFIED/REMOVED/RENAMED delta headings were present in the source; spec was already in main-spec format.

### Archive Move

**Folder Move**:
- Source: `openspec/changes/procedural-candidate-detection/`
- Destination: `openspec/changes/archive/2026-09-18-procedural-candidate-detection/`
- Method: `git mv` (tracked by Git)
- Verification: Snapshot-based `diff -r` (empty diff confirming byte-identity)

All artifacts preserved:
- proposal.md: present
- design.md: present
- specs/procedural-candidate-detection/spec.md: present
- tasks.md: present (26/26 tasks complete)
- verify-report.md: present
- exploration.md: present

## Phase Completion and Verification Results

### Implementation Status

All 26 tasks across four phases are complete and checked ✓:

**Phase 1: Candidate Store (R-001)**
- Tasks 1.1–1.7: ✓ Complete
  - `NormalizeSlug` implementation with full test coverage
  - Contract document sections 1-3 with schema, field definitions, and slug rules
  - Acceptance checklist for cross-session cold-start scenario

**Phase 2: Repeat/Recovery Detection (R-002, R-003)**
- Tasks 2.1–2.6: ✓ Complete
  - Emission decision table (7 rows: all state transitions)
  - Failure-recovery clustering rules and label support
  - T1/T2 trigger contracts (immediate + session-close sweep)
  - Non-Go acknowledgment: acceptance checklist is equivalent gate, not `go test` unit tests

**Phase 3: Duplicate Rejection (R-004)**
- Tasks 3.1–3.7: ✓ Complete
  - `MatchCandidate` implementation with registry matching (no fuzzy/substring matches)
  - Contract document section 6 with rejection record field rules
  - Acceptance checklist with duplicate and uncovered candidate scenarios

**Phase 4: Full Verification**
- Tasks 4.1–4.6: ✓ Complete
  - Full test suite pass: `cd engine && go vet ./... && go test ./...`
  - File/manifest integrity confirmed (no `skills/` mutations except `skills/_shared/procedural-candidate-detection.md`)
  - No Go-Engram write-path introduction (grepped for DSN construction; none found)
  - All R-001/R-002/R-003/R-004 acceptance scenarios executed (10/10 scenarios pass; see verify-report.md)
  - Two follow-up corrections from review-1cdba131c496cc29 applied (R3-contract-test-unasserted-new-rule, R3-sweep-memo-key-truncated)

**Per verify-report.md** (Engram #3445):
- Full verification run: 10/10 acceptance scenarios PASS
- R-003 scenarios 5–6 re-run with resolvable anchors after initial fix
- Test framework confirms untruncated normalized form used in MatchCandidate comparison

### Test Results

**Engine Tests** (Phase 4 verification):
```
$ cd engine && go test ./...
[all packages] ok ... (cached)
[complete output available; all pass]
```

**Archive-Reconcile Tests**:
- Before archive move: FAIL (expected; procedural-candidate-detection stranded)
- After archive move: FAIL (unrelated blocker; see Downstream Blockers section below)

The archive-reconcile test failure after archiving is NOT caused by procedural-candidate-detection: that change is now correctly archived and no longer stranded. The failure is due to a pre-existing syntax error in an untracked downstream folder (procedural-memory-lifecycle) with an HTML comment delimiter on the same line as a checkbox in its tasks.md file, line 46. This is a separate repository issue unrelated to this archive operation.

## Final State

### Source of Truth Updated
- **New Main Spec**: `openspec/specs/procedural-candidate-detection/spec.md` — sole source of truth for this capability; delta spec removed from active changes
- **No Spec Deltas**: No `openspec/changes/{change-name}/specs/` deltas remain

### Artifacts Archived
All artifacts copied and verified:
- [x] proposal.md present
- [x] specs/ (now empty in active; full spec promoted to main)
- [x] design.md present
- [x] tasks.md present (26/26 complete)
- [x] verify-report.md present
- [x] exploration.md present

### Downstream Blockers

**Known Issue**: `openspec/changes/procedural-memory-lifecycle/` (untracked folder, not part of this PR):
- This downstream change carries a delta against this capability that must be rebased on the newly promoted spec at `openspec/specs/procedural-candidate-detection/spec.md`
- **Current blocker**: Syntax error in `procedural-memory-lifecycle/tasks.md`, line 46 — HTML comment delimiter shares a line with a task checkbox, which the archive-reconcile tool cannot parse. This must be resolved before the downstream change can be processed.
- Action: Mentioned here for traceability; do NOT touch this folder as part of this archive (per handoff). The downstream change owner must rebase and fix the syntax error.

## Review and Delivery Status

All four native reviews approved and acknowledged:
- review-1c5f059a2e4f88bf (Phase 1)
- review-dc6b86d85945737b (Phase 2)
- review-1cdba131c496cc29 (Phase 3 + follow-ups)
- review-c2926c5a9630236c (Phase 4)

The change is complete and archived. Delivery follows ordinary repository policy (PR #351 is open for merge under user discretion).

## Artifact Observation IDs

- **apply-progress**: Engram #3408
- **verify-report**: Engram #3445 (full untruncated acceptance results recorded)

## Verification Contract

- Archive performed mechanically with shell `cp -R` and `git mv`
- All `diff -r` readbacks confirmed byte-identity (empty diffs)
- Snapshot-based verification ensures no truncation or alteration during copy/move
- Tasks document preserved at byte-identity; completion status unchanged from source
- No model-driven Read/Write copy used; all artifacts preserved with complete fidelity

**Diff outputs (conforming to Mechanical Copy Contract)**:

### Spec Promotion Diff
```
<empty diff — source and destination identical>
```

### Archive Move Diff
```
<empty diff — source snapshot and destination identical>
```

## Closure

This change is archived and closed. No further work is required for procedural-candidate-detection itself. The capability is ready for downstream consumers to depend on the promoted spec at `openspec/specs/procedural-candidate-detection/spec.md`.
