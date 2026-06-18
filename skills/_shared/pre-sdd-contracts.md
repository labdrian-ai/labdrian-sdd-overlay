# Pre-SDD Shared Contracts

## Purpose

This file defines the persistence contracts, language rule, identifier rule, and registry-block format shared by requirements-from-transcripts, project-manifest, project-architect, roadmap-maker, sdd-time-estimation, and inception-pipeline. Each skill references this file instead of re-deciding these conventions independently.

---

## Language Rule (authoritative)

ALL persisted project/SDD artifacts — manifest, mission, rules, architecture, roadmap, requirements brief, estimate, entry contract, actuals, and registry blocks — are written in **ENGLISH**. The user-facing conversation and persona are unaffected and may remain in Spanish or any other language. Spanish artifacts are acceptable ONLY if the user explicitly requests it for a specific artifact.

This rule applies to **new writes only**. Legacy Spanish artifacts are tolerated until their next full regeneration. Do NOT retro-translate existing artifacts.

---

## Change-Name Authority

`change-name` is derived **exactly once**, by the requirements-capture step (`requirements-from-transcripts`), as a **kebab-case slug**, and is **inherited verbatim** by every downstream stage and topic key. It is **never re-derived downstream**.

Slug rules:
- Lowercase only
- Words separated by hyphens
- No spaces, underscores, or special characters
- Stable — never renamed once set

---

## Engram Topic Keys

| Artifact | Topic Key | Writer |
|---|---|---|
| Requirements brief | `project/{project}/requirements/{change}` | requirements-from-transcripts |
| Manifest | `project/{project}/manifest/{context,mission,rules}` | project-manifest |
| Architecture | `project/{project}/architect/final` | project-architect |
| Roadmap | `project/{project}/roadmap` | roadmap-maker |
| Estimate (pre-start) | `sdd/{change}/estimate` | sdd-time-estimation |
| Entry contract | `sdd/{change}/entry` | inception-pipeline |
| Closure actuals | `sdd/{change}/actuals` | sdd-archive (ONLY writer) |
| Pipeline state | `sdd/{change}/pipeline-state` | inception-pipeline |

> **Note:** Engine keys (`sdd/{change}/{proposal,spec,design,tasks,apply-progress,verify-report,archive-report,state}`) are owned by the SDD engine and are NOT listed here — do not reuse them.

> All automated saves (SDD phase artifacts, schemas, state) use `capture_prompt: false`.

---

## Schemas

Two JSON Schema files (draft 2020-12) live alongside this document:

- **`./entry-contract.schema.json`** — validated by the orchestrator gate before handing off to the SDD engine. Contains topic-key references (not content) and delivery parameters for a change.
- **`./actuals-record.schema.json`** — validated at closure before writing. Contains implementation, review, and wall-clock hours plus scope and variance notes.

### Requirements Brief Shape

EARS-formatted requirement items (R-NNN), each carrying a `scope` enum of `new-capability | feature | fix`, plus a project-inception handoff table summarizing the change's entry contract inputs.

### Estimate Record Shape

Planned effort range in hours (`low` / `high`), a human-readable `complexity` assessment, a `confidence` level, and a note that human-review and gate time are included in the planned range.

---

## Registry-Block Format

Skills that inject compact rules into `.atl/skill-registry.md` use a **delimited replace-by-marker** block. Updates replace the block idempotently and never duplicate content. Keep blocks compact and imperative. Writers must locate the existing block by its markers and replace the entire content between them.

### project-manifest-rules block

```
<!-- BEGIN: project-manifest-rules (auto-generated) -->
...content...
<!-- END: project-manifest-rules -->
```

### project-architect-constraints block

```
<!-- BEGIN: project-architect-constraints (auto-generated) -->
...content...
<!-- END: project-architect-constraints -->
```

Writers must:
1. Search `.atl/skill-registry.md` for the BEGIN marker.
2. Replace everything between the BEGIN and END markers with the new content.
3. Never append a second block — always replace in place.
