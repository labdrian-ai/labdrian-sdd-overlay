# Archive Report: longterm-mem-promotion-scoping

**Archived**: 2026-09-09  
**Change**: longterm-mem-promotion-scoping  
**Archive Location**: `openspec/changes/archive/2026-09-09-longterm-mem-promotion-scoping/`  
**Project**: labdrian-sdd-overlay  
**Artifact Store**: hybrid (openspec files + Engram)

## Executive Summary

Change `longterm-mem-promotion-scoping` has been successfully archived. The single-slice implementation (440 total lines incl. SDD artifacts, size exception granted) adds curated `topic_key` gating to Engram observation promotion eligibility, replacing type/revision-count-based automatic criteria. All 25 tasks (20 core + 5 remediation) completed; 18/18 spec scenarios verified compliant. Specs merged into main repository state.

## Entry Contract vs Realized Delivery

| Field | Planned | Realized | Notes |
|-------|---------|----------|-------|
| Review slices | 1 | 1 PR | Single unit granted size exception (440 lines) |
| Tasks (core) | 20 | 20 | ✅ All complete |
| Tasks (remediation) | — | 5 | ✅ All complete; verifier findings fixed in-batch |
| Spec scenarios | 11 (initial) | 18 | +7 remediation scenarios (leading-slash, edge cases) |
| Delivery strategy | single-pr | single-pr | No chaining; within 400-line budget |

## Specs Synced

### Domain: longterm-mem-promotion

**Action**: Updated (MODIFIED R-007)

| Item | Count | Details |
|------|-------|---------|
| Requirements modified | 1 | R-007 Promotion Eligibility Predicate (rewrite) |
| Scenarios added | 7 | Leading-slash handling, edge cases (remediation) |
| Scenarios total | 11 | All R-007 scenarios verified compliant |
| Composition | ✅ | `gentle-ai sdd-archive-compose` applied without error |

**R-007 Change**: Replaced type/revision-count-based automatic eligibility with curated-topic_key rule:
- Automatic eligibility now requires: non-empty `topic_key` whose first path segment is non-empty AND not in {`sdd`, `review`, `delivery`}
- Explicit promote and pinned always override
- Leading-slash topic keys (e.g. `/sdd/auth`) are excluded (empty first segment)

### Domain: longterm-mem-memory-access

**Action**: Updated (ADDED R-020)

| Item | Count | Details |
|------|-------|---------|
| Requirements added | 1 | R-020 Observation topic_key Exposure |
| Scenarios | 3 | `topic_key` populated, NULL→"", every query path covered |
| Composition | ✅ | `gentle-ai sdd-archive-compose` applied without error |

**R-020 Addition**: Store adapter now exposes `topic_key` from read-only Engram connection for eligibility evaluation (R-007).

## Archive Contents

✅ All artifacts present and verified:

- **proposal.md** — Change intent, scope, capabilities, approach, risks, rollback
- **specs/**
  - `longterm-mem-promotion/spec.md` — R-007 Promotion Eligibility Predicate (MODIFIED, merged)
  - `longterm-mem-memory-access/spec.md` — R-020 topic_key Exposure (ADDED, merged)
- **design.md** — Technical approach, architecture decisions, data flow, file changes, interfaces, testing strategy, threat matrix
- **tasks.md** — 20 core + 5 remediation tasks; all marked complete [x]
- **apply-progress.md** — Implementation evidence: TDD cycle per phase, work units, acceptance evidence (sync --dry-run transcript), deviations, remediation batch
- **verify-report.md** — PASS verdict: 25/25 tasks complete, 18/18 scenarios compliant, 144/144 focused tests green, 17/17 broad packages green, design decisions followed

**Task Completion Verified**: 25/25 tasks ✅, all marked [x], no stale unchecked implementation tasks.

## Verification Status

**Verdict**: PASS (from `verify-report.md`)

| Check | Result | Notes |
|-------|--------|-------|
| Build | ✅ | `cd longterm-mem && go build` — exit 0 |
| Focused tests | ✅ 144/144 | `go test ./internal/promote/... ./internal/engram/...` — all pass |
| Broad suite | ✅ 17/17 packages | `go vet ./... && go test ./...` — all pass |
| Spec scenarios | ✅ 18/18 | All R-007 and R-020 scenarios compliant |
| Acceptance evidence | ✅ | `sync --dry-run` transcript: 90 promoted / 538 skipped; pre-change 182 sdd/, 58 untopiced, 4 delivery/, 1 review/ rows now correctly excluded unless pinned or explicit |
| Critical findings | 0 | None |
| Blockers | 0 | None |

## Implementation Highlights

### Core Changes
1. **`longterm-mem/internal/engram/store.go`** — Added `TopicKey string` field to `Observation`; scanned from SQLite `topic_key` column via `sql.NullString`
2. **`longterm-mem/internal/promote/eligible.go`** — Rewrote `Eligible(obs, explicit)` as `explicit || obs.Pinned || curatedTopicKey(obs.TopicKey)`; removed `eligibleTypes` and `minEligibleRevisionCount`
3. **Fixture sweep** — Scoped by call path: 30 direct `Promote(obs, false)` sites + ~24 store-backed `fixtureObs` rows; excluded `update_test.go` and `reconcile*_test.go` (they never reach `Eligible`)

### Remediation Batch (post-verify)
- **R.1**: Fixed empty-first-segment bypass (`/sdd/auth` now correctly excluded)
- **R.2**: Added 8 edge-case scenarios to `eligible_test.go` (bare keys, whitespace, leading-slash, case-sensitivity)
- **R.3**: Integration assertion in `plan_test.go` — `sdd/x` fixture skipped by both `Plan` and `Sync`
- **R.4**: Refreshed 3 stale doc comments
- **R.5**: Documented stale-page retention in `README.md` (`sync` does not auto-retract previously-promoted pages)

## Lineage (Engram Observations)

| Artifact | Observation ID | Status |
|----------|---|--------|
| Entry | #3288 | Superseded by this archive report |
| Proposal | #3290 | Topic key `sdd/longterm-mem-promotion-scoping/proposal` |
| Spec | #3291 | Topic key `sdd/longterm-mem-promotion-scoping/spec` |
| Design | #3292 | Topic key `sdd/longterm-mem-promotion-scoping/design` |
| Tasks | #3293 | Topic key `sdd/longterm-mem-promotion-scoping/tasks` |
| Apply-progress | #3294 | Topic key `sdd/longterm-mem-promotion-scoping/apply-progress` |
| Verify-report | #3295 | Topic key `sdd/longterm-mem-promotion-scoping/verify-report` |
| Archive-report | (new) | Topic key `sdd/longterm-mem-promotion-scoping/archive-report` |

**All artifact observation IDs recorded for traceability.**

## File Changes Summary

| File | Action | Lines |
|------|--------|-------|
| `longterm-mem/internal/engram/store.go` | Modified | +15, −3 |
| `longterm-mem/internal/engram/store_test.go` | Modified | +24, −0 |
| `longterm-mem/internal/promote/eligible.go` | Modified | +20, −40 |
| `longterm-mem/internal/promote/eligible_test.go` | Modified | +150, −25 |
| `longterm-mem/internal/promote/sync_test.go` | Modified | +3, −0 |
| `longterm-mem/internal/promote/plan_test.go` | Modified | +24, −0 |
| `longterm-mem/internal/promote/propagate_test.go` | Modified | +11, −0 |
| `longterm-mem/internal/promote/writer_test.go` | Modified | +30, −3 |
| `longterm-mem/internal/promote/projectmove_test.go` | Modified | +21, −0 |
| `longterm-mem/internal/promote/projectmove_recovery_test.go` | Modified | +15, −0 |
| `longterm-mem/cmd/longterm-mem/sync_dryrun_test.go` | Modified | +1, −0 |
| `longterm-mem/README.md` | Modified | +8, −3 |
| **Total (core)** | — | **+322, −74** = 396 lines |

**Remediation batch** (+94, −16 across 7 files, bringing total to 440 incl. SDD artifacts).

## Archive Verification

| Check | Status | Evidence |
|-------|--------|----------|
| Archive directory created | ✅ | `openspec/changes/archive/2026-09-09-longterm-mem-promotion-scoping/` exists |
| All artifacts moved | ✅ | proposal.md, design.md, tasks.md, verify-report.md, specs/, apply-progress.md, entry.json present |
| Source removed from active changes | ✅ | `openspec/changes/longterm-mem-promotion-scoping/` no longer exists |
| Byte-identity verification | ✅ | `diff -r` pre-move snapshot vs. archive (empty output = pass) |
| Main specs updated | ✅ | Both delta specs merged via `sdd-archive-compose` without error |

**Archive Readback**: ✅ Empty diff confirms mechanical move preserved all bytes.

## Closure Checklist

- [x] Task Completion Gate passed (25/25 tasks complete, no unchecked implementation tasks)
- [x] Verify report status: PASS (no CRITICAL findings, no blockers)
- [x] Delta specs merged into main specs via `sdd-archive-compose`
- [x] Change folder moved to archive with date prefix (2026-09-09)
- [x] Active changes directory cleaned (source removed)
- [x] Archive contents verified byte-identical
- [x] All artifact observation IDs recorded
- [x] Lineage traceability established

## SDD Cycle Complete

The change has been fully planned (proposal), specified (delta specs merged), designed, implemented (20 core tasks + 5 remediation tasks), verified (18/18 scenarios PASS), and archived. The SDD cycle is closed.

**Ready for next change.** 
## Cycle Timestamps

Recorded at archive time, before the change reached `main`. The inception-pipeline closure-feedback appends the landing-commit row after the merge.

| Anchor | Value | Source | Outcome |
|--------|-------|--------|---------|
| **t0** (tiering go-ahead) | `2026-09-08T20:31:53Z` | `sdd/longterm-mem-promotion-scoping/pipeline-state`, Engram observation #3280 (`created_at`; provisional key restated as #3282) | primary |
| **t1** (landing commit) | `landing_commit`: `0a461d9c49713ba25ddbd0f41531987087c292a6` (merge of PR #294 onto `main`), committer timestamp `2026-09-09T03:03:42Z` | `gh pr view 294 --json mergeCommit,mergedAt`, cross-checked against `git show -s --format=%ci` | self-asserted, not verified — no native review receipt reached `approved` for this candidate (lineage `review-c7b86c944a5c7171` stopped `unachievable_lens_slot` after 2/4 lenses), so no `approved_tree` is recorded here and none can be checked against `landing_commit`'s own tree; t1 still resolves from `landing_commit`'s own committer timestamp alone |
