---
name: project-manifest
description: "Turn a project idea into a strategic context, agent mission, and mandatory working rules through discovery, draft, and final phases."
license: MIT
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Receive project idea, name, resolved `artifact_store.mode` (`engram | openspec | hybrid | none`), optional `conversation_history` and `phase_override` (`discovery | draft | final`), and parent-resolved memorycard capabilities and project-scoped locators. Produce distinct context, mission and rules documents; no code or technical architecture.

## Hard Rules

Use supplied context; retrieve optional `sdd-init/{project}` and `sdd/{project}/testing-capabilities`. Search project-scoped records, get full records, merge continuation updates and readback writes. Never reopen resolved storage. Historical `engram` means memory-only, not Engram writes or export; `openspec` uses files, `hybrid` both, `none` inline. Return concrete missing-input or capability blockers.

Keep `project/{project}/manifest/{context,mission,rules}` and `openspec/project/manifest/{context,mission,rules}.md`. Discovery never persists; draft and final follow placement. Apply all eight `references/universal-base-rules.md` rules and confirmed domain rules only. Distinguish facts, decisions, hypotheses and gaps; never invent facts or rules. New artifacts and envelopes are English except user quotes; preserve Spanish records.

In final, merge and readback `.atl/skill-registry.md` and the project-scoped `skill-registry` record when applicable. Replace the manifest block in place, preserving unrelated bytes; create only if absent and never duplicate. Exact markers:

<!-- BEGIN: project-manifest-rules (auto-generated) -->
<!-- END: project-manifest-rules -->

Preserve the architect block in `../_shared/pre-sdd-contracts.md`. Compact rules: imperative, one per line, about 15 lines maximum.

## Decision Gates

- `discovery`: gaps prevent a draft; report missing evidence and ask at most one blocking question per parent interaction.
- `draft`: enough evidence for three drafts, with visible uncertainty; confirmation stays with the parent/owner.
- `final`: critical information confirmed; otherwise block in draft. Honor `phase_override` without inventing evidence.

## Execution Steps

1. Read `references/phase-templates.md` for the phase and response; read `references/universal-base-rules.md` for discovery topics and all eight rules before draft or final.
2. Read context, registry and full manifest records or files; merge prior content.
3. Produce the phase; report failed persistence or readback.
4. Return decisions to the parent; point to `../project-inception/SKILL.md` and `../role-chain/SKILL.md` for handoff, without executing roles.

## Output Contract

Return `status`, `executive_summary`, `detailed_report`, `artifacts`, `next_recommended`, `risks`, and `skill_resolution`, with phase, placement, locators, blockers and warnings. Templates are outlines.
