# Archive Report: Procedural Memory Lifecycle

- **Change**: `procedural-memory-lifecycle`
- **Archived**: 2026-09-21
- **Archive status**: PASS — canonical composition completed and the change folder was archived.
- **Verification status**: PARTIAL — the historical verification limitations below remain explicit and unchanged.

## Cycle Timestamps

| Phase | Timestamp | Notes |
|-------|-----------|-------|
| t1 (landing commit) | n/a | landing_commit is absent, so there is no landing commit to anchor on: the implementation slices merged to `main` through separate pull requests over several days, and this archive reaches `main` only through PR #434, which does not exist as a merge when this record is written. None is inferred from tree matching or position. |

## Executive Summary

Native `gentle-ai.sdd-status` v2 authorized archive: `artifactStore: openspec`,
`taskProgress: 118/118`, `applyState: all_done`, `nextRecommended: archive`, and
`blockedReasons: []`. The archive-anchor gate passed with approved review receipt
`review-receipts/review-db49979047ecf94b.review-state.json` and
`candidate_tree` / `approved_tree` `80358570049cec3cd8d4a478868dc023b510b8ff`.

All applicable file-backed specs were composed before the move. Existing
canonical specs received only their pending additive requirements; four absent
domains were copied as full canonical specs. No destructive `REMOVED` or large
`MODIFIED` operation was present, so no destructive merge approval was required.

## Artifacts Read

- `proposal.md`
- `exploration.md`
- `specs/procedural-candidate-detection/spec.md`
- `specs/procedural-skill-drafting/spec.md`
- `specs/procedural-skill-maintenance/spec.md`
- `specs/procedural-skill-registration/spec.md`
- `specs/skill-lifecycle/spec.md`
- `specs/skill-lint/spec.md`
- `design.md`
- `tasks.md`
- `apply-progress.md`
- `verify-report.md`
- `codex-smoke.md`
- `openspec/config.yaml`
- The approved archive-anchor review receipt noted above

No `sync-report.md` was present. The persisted `tasks.md` was re-read immediately
before composition and contains no unchecked implementation task markers.

## Native Status and Action Context

| Field | Value |
|---|---|
| Schema | `gentle-ai.sdd-status` v2 |
| Artifact store | `openspec` |
| Change root | `/home/labdrian/labdrian-sdd-overlay/openspec/changes/procedural-memory-lifecycle` |
| State | `ready` |
| Task progress | `118/118` complete, `0` pending |
| Apply state | `all_done` |
| Next recommended | `archive` |
| Blocked reasons | none |
| Action context mode | `repo-local` |
| Workspace root | `/home/labdrian/labdrian-sdd-overlay` |
| Allowed edit roots | `/home/labdrian/labdrian-sdd-overlay` |

No archive path, canonical path, or move target was outside the authoritative
workspace or allowed edit root. Native status reported no active same-domain
change; the active-change scan also found no collision for the six domains below.

## Canonical Spec Composition

All operations were classified as pending because the current canonical content
did not already contain this change's requirement blocks and no prior composition
history established an applied operation. Composition completed successfully.

### Existing canonical specs updated

- `openspec/specs/procedural-candidate-detection/spec.md`
  - **ADDED**: `Do-Not-Capture List Gates Candidate Quality`
  - **ADDED**: `Extended Status Vocabulary`
  - **ADDED**: `Disposition Field on the Candidate Record`
  - **ADDED**: `Append-Only History on Candidate and Draft Records`
  - **ADDED**: `Registered/Promoted Hash Line Recorded on the Candidate Record`
  - **ADDED**: `OccurrencesSincePromotion Is Derived, Not Stored, and Applies at Either Tier`
  - **ADDED**: `AbsorbedInto Field on the Candidate Record`
- `openspec/specs/skill-lifecycle/spec.md`
  - **ADDED**: `AddCore Enforces a LintSkill Hard Gate`

### New canonical domain specs created from full change specs

- `openspec/specs/procedural-skill-drafting/spec.md`
  - `Do-Not-Capture List Refuses Drafting Disqualified Candidates`
  - `Lesson Shape for Summary`
  - `Disposition Decides Extend-Before-Create`
  - `Draft Records Live Only in Engram`
  - `Extended Status Vocabulary Reflects Drafting Progress`
  - `History Is Append-Only`
- `openspec/specs/procedural-skill-maintenance/spec.md`
  - `Ownership-by-Hash Refuses Revision of a Human-Owned Skill`
  - `OccurrencesSincePromotion Is Derived and Triggers Revision at Either Tier`
  - `Revision Follows the Drafting and Registration Paths Per Tier`
  - `Retirement Detection Is Report-Only`
  - `Retirement Removal Is a Decision, Scoped by Tier`
  - `Consolidation Records an Existence-Verified AbsorbedInto Reference`
- `openspec/specs/procedural-skill-registration/spec.md`
  - `Codex Discovery Is Confirmed Before Codex Support Is Claimed`
  - `Multi-Target Project-Tier Registration`
  - `LintSkill Is a Hard Gate Run on the Stamped Bytes, Before Any Hash or Write`
  - `Provenance Frontmatter Is Stamped on Every Registered Skill`
  - `Project Lock File Records Ownership Evidence`
  - `Registration Status Is registered, Distinct From promoted`
  - `Registration Produces Exactly One Scoped Commit`
  - `No File Is Written Under skills/ Except Through the Human add Path`
  - `Global Promotion Reuses AddCore Unchanged Apart From the Lint Gate`
  - `Tests Use Temporary Directories Only`
- `openspec/specs/skill-lint/spec.md`
  - `LintSkill Is a Pure, Stdlib-Only Function`
  - `Hard Rules Block on Structural and Required-Field Defects`
  - `LintSkillFile Composes SplitSkillFile and LintSkill`
  - `Advisory Warnings Never Block`
  - `One Source of Truth for Lint Rules`
  - `engine skills lint CLI Exit Contract`

There were no `MODIFIED`, `REMOVED`, or `RENAMED` operations. The configured
`rules.archive` warning for destructive deltas was therefore not triggered.

## Verification Findings Preserved

- Functional engine and longterm-mem suites and vet checks passed, including the
  configured focused and full commands recorded in `verify-report.md`.
- The retirement rollback-of-rollback final acceptance branch was not
  independently exercised. Existing implementation and unit/CLI evidence exists,
  but this report does not claim that final acceptance scenario passed.
- Codex project-skill discovery passed for `.agents/skills/<id>/`; skill-body
  loading remains unverified/inconclusive because the host filesystem sandbox
  failed. The sandbox was not weakened.
- Strict-TDD canonical evidence was reconciled with task-level rows. Historical
  safety-net test counts were not captured and are intentionally not invented.
- The verification report remains `PARTIAL`, not a clean `PASS`, while the native
  archive readiness projection remains authoritative and clear of blockers.

## Archive Move

The active change was moved without deleting or rewriting its historical
artifacts:

`openspec/changes/procedural-memory-lifecycle/`
→ `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/`

The destination did not exist before the move. The move preserves the proposal,
exploration, design, six delta specs, tasks, cumulative apply progress, verify
report, Codex smoke record, and all 15 review-receipt JSON files. The archive
report itself is included at the destination.

## Exact Files Changed

### Canonical composition writes

- `openspec/specs/procedural-candidate-detection/spec.md`
- `openspec/specs/skill-lifecycle/spec.md`
- `openspec/specs/procedural-skill-drafting/spec.md`
- `openspec/specs/procedural-skill-maintenance/spec.md`
- `openspec/specs/procedural-skill-registration/spec.md`
- `openspec/specs/skill-lint/spec.md`

### Archived paths

- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/archive-report.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/proposal.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/exploration.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/design.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/codex-smoke.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/tasks.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/apply-progress.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/verify-report.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/specs/procedural-candidate-detection/spec.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/specs/procedural-skill-drafting/spec.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/specs/procedural-skill-maintenance/spec.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/specs/procedural-skill-registration/spec.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/specs/skill-lifecycle/spec.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/specs/skill-lint/spec.md`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-24fc80ac3513305c.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-42be59cf3243a7ff.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-55ca2b96b20aef2d.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-6353816f626756f1.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-6d89070e3ba2bbbd.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-70263a4dee1c98b7.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-77e824f1dcccfc4f.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-9ea42239d8e2080b.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-b375153aa152604e.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-b75e4a27b9494ff8.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-c4c6f452b3ecc485.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-cafb047b78880102.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-d01f0b8ab38550f8.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-d89971d41a526146.review-state.json`
- `openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/review-receipts/review-db49979047ecf94b.review-state.json`

No source code, PR, push, or unrelated untracked `.agents/`, `.claude/skills/`,
`.pi/`, `skills-lock.json`, or `docs/architecture/` content was changed by this
archive.

## Next Recommendation

`next_recommended: archive` was satisfied. The change is now closed in the
OpenSpec archive; any future work should be a new change or an explicit follow-up
for the recorded verification limitations.

## Phase Envelope

```yaml
status: pass
executive_summary: >-
  Canonical specs were composed and procedural-memory-lifecycle was moved to
  openspec/changes/archive/2026-09-21-procedural-memory-lifecycle. Native status
  authorized archive at 118/118 tasks with applyState all_done. Verification
  remains partial only because its historical limitations were preserved.
artifacts:
  archive_report: openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/archive-report.md
  canonical_specs:
    - openspec/specs/procedural-candidate-detection/spec.md
    - openspec/specs/procedural-skill-drafting/spec.md
    - openspec/specs/procedural-skill-maintenance/spec.md
    - openspec/specs/procedural-skill-registration/spec.md
    - openspec/specs/skill-lifecycle/spec.md
    - openspec/specs/skill-lint/spec.md
  archived_change: openspec/changes/archive/2026-09-21-procedural-memory-lifecycle/
next_recommended: archive
risks:
  - WARNING: retirement rollback-of-rollback final acceptance was not independently exercised
  - WARNING: Codex skill-body loading remains unverified/inconclusive due host sandbox failure
  - INFO: historical strict-TDD safety-net counts were not captured
skill_resolution: paths-injected
```

## Key Learnings

1. Native status is the authoritative admission signal for OpenSpec archive work.
2. Verification limitations must remain explicit rather than being converted into passes.
3. Canonical composition distinguishes full new domains from additive delta requirements.
