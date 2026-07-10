# Archive Report: skill-external-provenance

**Change:** skill-external-provenance (Issue #29, Slice 5)
**Archived:** 2026-07-10
**Archived to:** `openspec/changes/archive/2026-07-10-skill-external-provenance/`

## Verify verdict carried into archive

**GO — 0 CRITICAL, 1 WARNING, 1 SUGGESTION** (verify-report.md, dated 2026-06-26, PR-2 scope).

- W-01 (non-blocking): stale `unknown_source_type.yaml` fixture comment referencing pre-slice-5 behavior — maintenance debt, does not block archive.
- S-01 (cosmetic): the `external`+`upstream` cross-field error in `validateEntry` lacks a line number, unlike sibling `parseSource` errors — cosmetic, acceptable as follow-up.
- next_recommended in verify-report.md: `sdd-archive`.

Task completion gate: `apply-progress.md` shows all PR-2 tasks (T-05, T-06, T-07)
plus the WARNING-1 and SUGGESTION-1 fixes marked `[x]` complete; verify-report.md's
task table confirms all as DONE with commit hashes. No stale unchecked tasks found.

## Specs merged

`openspec/changes/skill-external-provenance/specs/spec.md` was a single delta
file (R-112–R-134, SC-55–SC-69) covering two capabilities named in
`proposal.md`'s "Modified Capabilities" section: `skill-package-manager` and
`skill-lifecycle`. Neither capability had a prior entry under
`openspec/specs/`, so per the "main spec does not exist" archive rule the
delta content was copied forward as the initial full spec for each
capability, split by subject matter (the source delta was not already split
per capability):

| Target spec file | Created/Updated | Requirements carried | Scenarios carried |
|---|---|---|---|
| `openspec/specs/skill-package-manager/spec.md` | Created (new capability dir) | R-112–R-125, R-131, R-132 | SC-55–SC-64, SC-69 |
| `openspec/specs/skill-lifecycle/spec.md` | Created (new capability dir) | R-126–R-130, R-133, R-134 | SC-65–SC-68 |

Both files were converted from the delta's EARS-style requirement list into
this repo's existing `openspec/specs/*/spec.md` convention (`## Requirements`
→ `### Requirement: <name>` → `#### Scenario: <name>` with Given/When/Then),
matching the format already used by `overlay-coherence`, `runtime-lifecycle`,
and `oo-quality-contract`. Each requirement heading preserves its original
`R-xxx` ID(s) for traceability back to the delta spec now stored in this
archive folder.

Note: the delta spec's "Non-regression contract" states that slices 1–4 of
issue #29 (R-001–R-111, SC-01–SC-54) remain in force, but no prior archive or
`openspec/specs/` entry for `skill-package-manager` / `skill-lifecycle` exists
in this repository — those earlier slices were merged to `main` via git PRs
before this repo's openspec spec-sync convention was in place. Reconstructing
their historical requirements was out of scope for this archive; only the
slice-5 delta content was merged forward.

## Directory move

- `git mv` used for previously-tracked files: `proposal.md`, `design.md`,
  `tasks.md`, `specs/` (and its contents).
- Plain `mv` used for untracked files: `apply-progress.md`, `verify-report.md`.
- Source directory `openspec/changes/skill-external-provenance/` removed
  (now empty) after the moves.
- No `git add` / `git commit` performed — file operations only, per
  instructions.

## Result

The `skill-external-provenance` SDD cycle is complete: proposed, designed,
tasked, implemented (merged to `main` via PR #48, #49, #50 per git history),
verified (GO), and archived. `openspec/specs/skill-package-manager/spec.md`
and `openspec/specs/skill-lifecycle/spec.md` are now the source of truth for
this behavior.
