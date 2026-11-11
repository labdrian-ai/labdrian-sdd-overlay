---
name: roadmap-maker
description: "Build an SDD roadmap or insert one requirement: roadmap, SDD roadmap, sequence SDDs, roadmap maker, ordenar SDDs, secuenciar cambios, planificá los sprints. Requires manifest and architecture."
license: MIT
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Produce the third inception artifact after manifest and architecture, or insert one requirement into an existing roadmap. Receive project, placement, `mode` (`build` by default or `incremental-insert`), and optional sequencing constraints from the parent.

## Hard Rules

- Require final-enough `project/{project}/manifest/context`, `project/{project}/manifest/rules` and `project/{project}/architect/final`. Cite manifest, architecture or history for every item; do not invent items or dependencies. An unresolved source is `[PENDING DECISION]`.
- Preserve inherited requirement and change identities exactly. Order by dependencies and risk, with no cycles; completed work is foundation, never work to redo. Do not re-plan completed work to fill a template.
- Insert mode splices ONE item into the existing roadmap, updating only affected dependency chains. Preserve unrelated rows, IDs, completed work, prior actuals and history; never regenerate the whole roadmap.
- Read `assets/roadmap-template.md` when rendering or tracking, including inserts. Its tracking sources and sentinels govern output. Read `sdd/{change}/actuals`; only inception closure writes actuals. Never write or infer them from plans or estimates. Preserve legacy sentinels and measured versus reconstructed disclosures; zero is a measurement.
- The parent resolves memorycard capabilities, project-scoped locators and placement. Search and get full records, merge existing content and readback writes. Preserve `project/{project}/roadmap` and optional `openspec/project/roadmap.md`; legacy `engram | openspec | hybrid | none` placement values remain readable, not migration targets or authorization to write Engram. Do not ask the storage question again.
- New artifacts and envelopes are English except verbatim quotes. Existing records and history are not translated or migrated. The parent executes routes; this skill does not dispatch roles.

## Decision Gates

Missing or critically incomplete manifest or architecture: return the exact blocker and do not persist. Missing existing roadmap or sourced requirement in insert mode: return the blocker. Report source gaps without inventing data; ask the parent at most one blocking question when needed.

## Execution Steps

1. Load required inputs, existing history and prior roadmap through the parent's supplied locators. Read the template and relevant chain shaper/estimator records and actuals for tracking.
2. In `build`, produce or replace the entire roadmap from cited items and dependency order, retaining completed foundations. In `incremental-insert`, splice one item without regenerating the roadmap.
3. Render tracking from available sources, preserving `[PENDING]` versus `[NOT MEASURED — reason]` and source attribution. Preserve existing legacy sentinels. Do not replace absent measurements with estimates.
4. Reopen before each change and after closure; update when reality diverges. Do not write spec, design or apply detail. Merge and persist the roadmap at the resolved supported placement, then readback. Name `../inception-pipeline/SKILL.md` and `../role-chain/SKILL.md` as current entry paths; the parent owns route execution.

## Output Contract

Return `completed`, `blocked`, or `partial`, actual artifact locators, source gaps and at most one parent-blocking question. The template is an outline for new output, not an instruction to rewrite historical records.
