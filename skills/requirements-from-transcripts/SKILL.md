---
name: requirements-from-transcripts
description: "Trigger: meeting transcripts, customer stories, client conversations, user interviews, requerimientos desde meets, historias de cliente. Turn stakeholder conversations into traceable EARS requirements that seed inception."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "2.0.0"
---

## Activation Contract

Use for transcripts, chats, customer stories, stakeholder or support notes, a clarify-request brief or a prespec transcript that must become requirements. The brief seeds project-inception, or roadmap-maker for a small fix. Load `../_shared/pre-sdd-contracts.md`.

## Hard Rules

- Do not invent requirements: derive each from evidence or mark it as an assumption. Artifacts are English; stakeholder quotes stay verbatim.
- Every `**Requirement:**` line is ONE EARS sentence with exactly one SHALL; banned soft verbs are blocking.
- One fix or feature per requirement; split anything not independently testable.
- IDs use `R-001` format, assigned once, never renumbered. Each item carries a `scope` of `new-capability | feature | fix`, acceptance evidence and keywords in English and stakeholder language.
- Derive `change-name` exactly once as a kebab-case slug; emit it in the brief header and the Project-Inception Handoff table. Downstream inherits it verbatim and never re-derives it.
- Persist the brief through parent-resolved memorycard capabilities to `project/{project}/requirements/{change}`, then read it back. Never write or export Engram.
- The brief feeds project-inception or roadmap-maker, then the role chain (`../inception-pipeline/SKILL.md`). Never create `sdd/*` or `openspec/` artifacts.

## Decision Gates

1. Single unambiguous fix, no transcript (Tier 3): Lightweight Mode below. On any ambiguity, switch to the full pipeline.
2. Otherwise run the contradiction and ambiguity pre-analysis before any requirement. An issue blocks when it affects permissions, data, workflow, money, compliance or user-visible behavior: ask about one issue at a time, with options and their tradeoffs, then stop and wait. Continue only once each issue is resolved or the user approves named assumptions.

## Execution Steps

1. Identify the source type; run gate 2, returning the decision prompt of `references/output-template.md` for the first blocking issue.
2. Read `references/patterns-catalog.md`. Extract anchors, cluster them into concerns, and split each into atomic requirements with ID, scope, type, priority, dependency, keywords, EARS sentence, acceptance scenarios and evidence.
3. Fill the brief from `references/output-template.md`: traceability matrix, order (dependencies first, user-visible fixes next, polish last), Manifest, Architecture and SDD Roadmap Inputs, and its Quality Bar.
4. Derive `change-name`, add the Project-Inception Handoff table, persist and read back.

Lightweight Mode emits the EARS sentence plus:

```yaml
R-NNN: <zero-padded ID>
scope: fix
one_line_impact: <user-visible or system impact>
acceptance_evidence: <minimum observable proof>
change_name: <kebab-case slug>
```

It skips the pre-analysis, input layers and traceability matrix.

## Output Contract

Return one of: the decision prompt for the first blocking issue; the brief with its `change-name`, R-NNN items and handoff table plus the memorycard record and readback result; or the Tier 3 object.
