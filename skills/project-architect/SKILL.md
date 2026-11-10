---
name: project-architect
description: "Design a defensible technical architecture from project constraints, with explicit trade-offs and manifest compliance."
license: MIT
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Receive the project name, resolved `artifact_store.mode` (`engram | openspec | hybrid | none`), optional focus, and parent-resolved memorycard capabilities and project-scoped locators. Design architecture from manifest context and rules; mission, existing stack and previous architecture inform it when available. Write architecture, not code; do not install dependencies or packages.

## Hard Rules

Search project-scoped records and get full records; merge updates and readback writes. Keep `project/{project}/manifest/{context,mission,rules}`, `sdd-init/{project}`, `project/{project}/architect/final` and `openspec/project/architect/final.md`. Historical `engram` means memory-only, not Engram writes or export; `openspec` uses files, `hybrid` both, `none` inline. Never reopen resolved storage. Report missing capabilities or permission.

Cite the actual constraints behind decisions. Preserve all technical sections and mandatory Section 15 compliance table: cover every applicable mandatory rule, redesign or disclose tension and mitigation, never silently omit a rule. State real uncertainties, trade-offs, rejected alternatives and unsupported scope; do not invent choices or a quota. New artifacts and envelopes are English except user quotes; preserve existing Spanish records.

After persisting, merge and readback `.atl/skill-registry.md` and the project-scoped `skill-registry` record when applicable. Replace only the architecture-owned block idempotently, preserving the manifest block and unrelated bytes; create a block only if absent, never duplicate it. Keep compact imperative constraints, about 15 lines maximum. Exact markers:

<!-- BEGIN: project-architect-constraints (auto-generated) -->
<!-- END: project-architect-constraints -->

## Decision Gates

- Missing manifest or critical sections: read `references/blocked-response.md`, return the concrete blocker and its existing choices to the parent; do not produce or persist architecture while blocked.
- Proceed without a persisted manifest only when the owner explicitly selects the existing inline-context choice and supplies its required context. Record its provenance and assumptions; do not claim compliance with an absent persisted manifest. Apply Section 15 to the supplied mandatory rules.
- Missing inputs or permission: report the blocker and at most one blocking question per parent interaction. A new consequential decision stays with the parent and owner.

## Execution Steps

1. Retrieve the actual manifest, optional mission, stack and prior architecture through supplied locators; check critical sections using `references/blocked-response.md` when inputs are missing or incomplete.
2. With sufficient inputs, read `references/architecture-template.md` and design its technical sections. Ground decisions in constraints and evidence; mark unresolved information instead of inventing it.
3. Persist according to the resolved placement, then update the registry when applicable. Read back each write; report partial failures.
4. Return the template's structured response and handoff to the parent. For subsequent work point to `../project-inception/SKILL.md` and `../role-chain/SKILL.md`; do not execute them.

## Output Contract

Return status, project, placement, actual artifact locators, decisions, real trade-offs, risks, uncertainties, blockers and next step. Templates are outlines; distinguish inline context from persisted manifest provenance.
