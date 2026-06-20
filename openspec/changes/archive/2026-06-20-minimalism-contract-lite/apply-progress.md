# Apply Progress — minimalism-contract-lite

Date: 2026-06-19
Mode: Standard (strict_tdd: false — Markdown authoring, no test runner)
Delivery: single-PR scope (400-line budget risk: Low)

---

## Completed Tasks

- [x] T-01 — Created `skills/_shared/minimalism-contract.md` with advisory YAML frontmatter,
  6-rung preference ladder, architectural tiebreaker, and 3-state `// minimal:` comment
  convention. File is ~35 lines, self-minimal.

- [x] T-02 — Refactored `skills/project-architect/SKILL.md` lines 164-166. Replaced the
  three ad-hoc "Do NOT inflate" bullets with a single citational reference to
  `../_shared/minimalism-contract.md` as canonical source. The reference explicitly states
  it is a documentation reference (not a load instruction) and that the ladder applies only
  during `sdd-tasks`/`sdd-apply`, NOT during architecture/design.

- [x] T-03 — Registered `_shared/minimalism-contract.md custom` in `overlay.manifest`
  immediately after the `_shared/pre-sdd-contracts.md custom` line. Classification: custom
  (overlay-original, no vendor counterpart).

- [x] T-04 — Added the `minimalism-contract` row to the Shared Contracts table in
  `.atl/skill-registry.md` with phase-scoped Trigger description: "Inject ONLY into
  sdd-tasks and sdd-apply sub-agent prompts under '## Skills to load before work'. Do NOT
  inject into propose/spec/design/verify/archive." This is the load-bearing scoping binding
  the resolver consumes.

- [x] T-05 — AC-1 observational verification performed (artifact-level, static):
  **Positive observation (PASS):** Registry row for `minimalism-contract` exists with
  path `skills/_shared/minimalism-contract.md` and Trigger scoped to sdd-tasks/sdd-apply.
  **Negative observation (PASS):** No other artifact in the overlay adds
  `minimalism-contract.md` to any other phase's load set. The `project-architect/SKILL.md`
  citational reference is plane-1 (documentation) only, confirmed by T-02.
  **WARNING:** Scoping is probabilistic (registry Trigger prose). Runtime confirmation
  requires inspecting actual orchestrator launch prompts. AC-1 is a recurring observational
  guard, not a hard guarantee. Deterministic enforcement is deferred (option b).

- [x] T-06 — Created `openspec/changes/minimalism-contract-lite/baseline.md`. No archived
  changes exist in this overlay yet; baseline marked as pending-first-archives. File
  includes instructions and table template for future entries.

## Remaining Tasks

- [x] T-07 — Deploy via `overlay apply` — COMPLETED (2026-06-19, post-verify critical fixes batch).
  - Ran `overlay apply --target opencode` and `overlay apply --target codex`.
  - Confirmed both targets IN_SYNC for `skills/_shared/minimalism-contract.md` and
    `skills/project-architect/SKILL.md` via `overlay sync-check`.
  - Note: the previous apply-progress incorrectly showed `[ ]` while tasks.md showed `[x]`.
    Root cause: T-07 had been deployed to claude only (the previous apply batch ran for the
    `claude` target as part of the commit `1beda34`), not for opencode/codex. Both are now
    deployed and verified. Status reconciled.

---

## Files Changed

| File | Action | What Was Done |
|------|--------|---------------|
| `skills/_shared/minimalism-contract.md` | Created | 6-rung ladder + tiebreaker + 3-state comment convention with advisory frontmatter |
| `skills/project-architect/SKILL.md` | Modified | Replaced lines 164-166 with citational reference to minimalism-contract.md |
| `overlay.manifest` | Modified | Added `_shared/minimalism-contract.md custom` line |
| `.atl/skill-registry.md` | Modified | Added `minimalism-contract` row to Shared Contracts table with scoped Trigger; added to git tracking (was untracked — fix #4) |
| `openspec/changes/minimalism-contract-lite/baseline.md` | Created | Manual baseline file (pending-first-archives) |
| `openspec/changes/minimalism-contract-lite/tasks.md` | Modified | Added Apply Progress table with [x] marks |
| `openspec/changes/minimalism-contract-lite/proposal.md` | Modified | Added MECHANISM REVERSAL NOTEs at items 3 and out-of-scope line (fix #3) |
| `~/.config/opencode/skills/_shared/minimalism-contract.md` | Deployed | Via `overlay apply --target opencode` (fix #1) |
| `~/.config/opencode/skills/project-architect/SKILL.md` | Deployed | Via `overlay apply --target opencode` (fix #2) |
| `~/.codex/skills/_shared/minimalism-contract.md` | Deployed | Via `overlay apply --target codex` (fix #1) |
| `~/.codex/skills/project-architect/SKILL.md` | Deployed | Via `overlay apply --target codex` (fix #2) |

---

## Deviations from Design

- **skill-registry/SKILL.md not found**: The tasks referenced this file for the "marker
  format" to regenerate the registry row. The file does not exist in the overlay. Fallback:
  used the existing table format in `.atl/skill-registry.md` (Shared Contracts section)
  consistently. The row follows the same `| name | path | trigger |` column format used for
  all other registry entries. This is idempotent and consistent; no marker format was
  violated. Design intent (scoped Trigger row via the registry table, NOT a BEGIN/END
  constraint block) is preserved.

- **No `<!-- BEGIN/END -->` marker block for T-04**: Task and design explicitly state NOT
  to use constraint blocks. The row was added to the Shared Contracts table directly.

- **`.atl/skill-registry.md` was untracked (post-verify critical fix #4)**: The load-bearing
  scoping binding had no git durability. Fixed by running `git add .atl/skill-registry.md`
  in the follow-up critical fixes batch. The file is repo-local governance (not a deployable
  skill), so it was NOT added to `overlay.manifest` — overlay.manifest tracks `skills/`
  files that deploy to targets. A future clone must restore this file manually or via a
  documented setup step unless committed to the repo. The file is now committed (see
  follow-up commit after critical fixes).

- **T-07 deployed to claude only in initial apply (post-verify finding)**: The first
  `overlay apply` run only deployed the `claude` target. The `opencode` and `codex` targets
  were left OVERLAY_NOT_DEPLOYED. Fixed in the critical fixes batch by running
  `overlay apply --target opencode` and `overlay apply --target codex`. All targets
  now IN_SYNC per `overlay sync-check`.

---

## Close-Out Doc Fixes (GADU-directed, post-verify)

- [x] Fix #6 (design.md — false claim): Corrected option (c) Pros line. Was "travels with the
  overlay (registry is overlay-tracked)". Changed to "git-tracked in the overlay repo (durable
  via repo history, not deployed by `overlay apply`)". `.atl/skill-registry.md` is NOT deployed
  by `overlay apply`; the false "travels" wording implied it was.

- [x] Fix #7 (design.md — propagation caveat): Added "Propagation caveat" paragraph after the
  "Honesty" section's first paragraph (before "Option (b) is explicitly deferred"). Documents
  that `.atl/skill-registry.md` is per-project; consumers regenerate their own registry and
  receive the contract WITHOUT the scoped Trigger row unless they carry it; cross-project
  scoped propagation is out of scope for this lite slice; AC-1 is asserted for the overlay
  repo only.

## ABSOLUTE CONSTRAINT COMPLIANCE

- `~/.claude/CLAUDE.md`: NOT touched. Confirmed.
- `sdd-phase-common.md`: NOT touched. Confirmed.
- All scoping achieved via overlay-tracked registry Trigger row (T-04) + advisory
  frontmatter (T-01). No global files mutated.

---

## Workload / PR Boundary

- Mode: single PR (400-line budget risk: Low — ~50 actual changed lines)
- Current work unit: all authoring tasks (T-01 through T-06)
- Boundary: starts from empty contract file, ends with registered + referenced contract
- Estimated review budget impact: ~50 lines — well under 400-line budget

## Status

7/7 tasks complete (all tasks including T-07 deploy). All post-verify fixes applied:
- [x] Fix #1: minimalism-contract.md deployed to opencode and codex targets
- [x] Fix #2: project-architect/SKILL.md deployed to opencode and codex targets
- [x] Fix #3: MECHANISM REVERSAL NOTEs added to proposal.md at items 3 and out-of-scope line
- [x] Fix #4: .atl/skill-registry.md added to git tracking
- [x] Fix #5: T-07 status reconciled between tasks.md and apply-progress.md
- [x] Fix #6: Corrected false "travels with the overlay" claim in design.md option (c) Pros
- [x] Fix #7: Added Propagation caveat paragraph to design.md Honesty section

All targets IN_SYNC per `overlay sync-check`. Change is fully deployed.
