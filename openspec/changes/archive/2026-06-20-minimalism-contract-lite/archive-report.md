# Archive Report — minimalism-contract-lite

**Archived**: 2026-06-20
**Change**: minimalism-contract-lite
**Artifact store**: openspec (repo-local)
**Archive location**: `openspec/changes/archive/2026-06-20-minimalism-contract-lite/`
**Verification status**: 0 confirmed CRITICAL; archive_ready=true
**Archive mode**: intentional-with-warnings (see Residual Warnings below)

---

## Change Archived

**Change**: minimalism-contract-lite
**Archived to**: `openspec/changes/archive/2026-06-20-minimalism-contract-lite/`

---

## Specs Synced

| Domain | Action | Details |
|--------|--------|---------|
| minimalism-contract | Created (first archive — no prior main spec) | Delta spec copied directly to `openspec/specs/minimalism-contract/spec.md` (6 requirements: R-001 through R-006, all with scenarios) |

Source: `openspec/changes/minimalism-contract-lite/specs/minimalism-contract/spec.md`
Target: `openspec/specs/minimalism-contract/spec.md`

This is the first archived change in this overlay repo. The `openspec/specs/` directory
did not exist prior to this archive operation; it was created as part of this step.

---

## Archive Contents

- proposal.md — present (includes Part A fix: "Scoping fragility" bullet updated 2026-06-20)
- specs/minimalism-contract/spec.md — present
- design.md — present
- tasks.md — present (12/12 tasks [x] complete: T-01 through T-07 + T-FIX-1 through T-FIX-5)
- apply-progress.md — present
- baseline.md — present

**Task completion gate**: PASSED. All implementation tasks are marked [x] in tasks.md.
No stale unchecked tasks.

---

## Source of Truth Updated

The following spec now reflects the new behavior:
- `openspec/specs/minimalism-contract/spec.md` (created — first archived change)

This spec is the authoritative source for the minimalism contract requirements (R-001–R-006).
All future SDD changes that touch `skills/_shared/minimalism-contract.md` or the scoping
mechanism MUST reference and update this spec.

---

## Part A Fix Applied (pre-archive)

The "Scoping fragility" bullet in `proposal.md` (Risks section, ~line 171) was updated
immediately before archiving to reflect the actual implemented mechanism:

**Before**: "because scoping relies on direct orchestrator injection (not the registry)..."
**After**: "scoping is bound by the scoped Trigger row in `.atl/skill-registry.md`
(advisory/probabilistic; the resolver matches trigger prose, no deterministic hook)..."

The old text described the REJECTED mechanism from the proposal draft. The new text matches
the actual design Decision 1 part B implementation. This closes the last doc-accuracy gap
before archiving.

---

## Residual Warnings (carried forward — not blocking)

### WARNING: AC-1 scoping is advisory/observed, not enforced

Scoping of `minimalism-contract.md` to `sdd-tasks` and `sdd-apply` is achieved via a
prose Trigger row in `.atl/skill-registry.md`. The orchestrator resolver matches this row
probabilistically — there is no deterministic phase filter. AC-1 was verified by static
artifact inspection (positive: registry row exists with scoped trigger; negative: no other
artifact adds the path to excluded phases). Runtime confirmation requires inspecting actual
orchestrator launch prompts. This is a recurring observational guard, not a guaranteed
property. Reported as WARNING per design Decision 1 and verify-phase findings.

### WARNING: Manual baseline pending-first-archives

`baseline.md` was created with the template and instructions, but no data entries exist.
This is the first change archived in this overlay, so there are no prior archived changes
to use as baseline. AC-4 measurement (directional LOC/dep trend) is blocked until 3-5
more changes complete the full SDD cycle. The baseline file is the recovery-safe store
(file-based, not Engram, due to project-ambiguity in this repo's cwd).

---

## SLICE-2 CANDIDATES (deferred — do NOT act on in this slice)

These items were explicitly deferred from this slice and MUST NOT be implemented until
a separate SDD change is opened:

1. **Cross-project scoped propagation to consumer registries**: `.atl/skill-registry.md`
   is per-project and is NOT deployed by `overlay apply`. Consumer projects (genesis, kadia)
   regenerate their own registries and receive `minimalism-contract.md` WITHOUT the scoped
   Trigger row. Making the scope carry to consumers requires either (a) the parsing hook
   below or (b) a convention for registry regeneration to carry overlay-scoped entries.

2. **Pre-launch injection hook parsing `applies_to_phases` frontmatter** (Design option b):
   This would make scoping deterministic. The current resolver does not parse the advisory
   frontmatter. A future slice may introduce this hook if AC-1 drift recurs in practice.
   Building it now violates rung 1 (YAGNI) of the very ladder shipped in this change.

3. **Optional `sdd-verify` minimalism gate per AC-5**: A slice-2 verification gate (e.g.,
   grep check for `// minimal:` comment presence in rung-6 selections) is proposed ONLY if
   AC-4 demonstrates a measurable behavior change worth enforcing harder. AC-4 cannot be
   evaluated until the baseline has 3-5 entries (see WARNING above). This gate MUST NOT be
   added before AC-4 evidence is available.

---

## Absolute Constraint Compliance

- `~/.claude/CLAUDE.md`: NOT touched throughout all phases. Confirmed.
- `sdd-phase-common.md`: NOT touched throughout all phases. Confirmed.
- All git operations (commit, push, add): left to orchestrator. Archive executor did NOT
  run any git commands.

---

## Folder Move Note

The source change folder `openspec/changes/minimalism-contract-lite/` has been fully
reproduced in the archive location. Deletion of the source folder requires filesystem
operations (directory removal) that are beyond the write-file scope of this executor.
The orchestrator must complete the move by removing `openspec/changes/minimalism-contract-lite/`
as part of the git commit that records this archive. Until the source folder is removed,
both paths exist — the archive folder is the authoritative copy.

---

## SDD Cycle Complete

The change `minimalism-contract-lite` has been fully planned, implemented, verified, and
archived. The minimalism contract is deployed to all three targets (claude, opencode, codex)
per apply-progress.md. The spec is now the source of truth at
`openspec/specs/minimalism-contract/spec.md`. Ready for the next change.
