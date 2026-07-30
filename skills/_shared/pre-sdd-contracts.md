# Pre-SDD Shared Contracts

## Purpose

This file defines the persistence contracts, language rule, identifier rule, and registry-block format shared by requirements-from-transcripts, project-manifest, project-architect, roadmap-maker, sdd-time-estimation, and inception-pipeline. Each skill references this file instead of re-deciding these conventions independently.

Contract bundle version: `2.0.0`. The inception skill, this authority, both schemas, and the deterministic validator MUST remain version-matched. Missing or mismatched bundle assets are a hard stop.

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
| Closure actuals | `sdd/{change}/actuals` | inception-pipeline closure-feedback (ONLY writer) |
| Estimation calibration | `project/{project}/estimation-calibration` | inception-pipeline closure-feedback |
| Pipeline state | `sdd/{change}/pipeline-state` | inception-pipeline |
| SDD initialization | `sdd-init/{project}` | sdd-init |
| Delivery plan | `delivery/{change}` | inception-pipeline entry assembly |

> **Note:** Engine keys (`sdd/{change}/{proposal,spec,design,tasks,apply-progress,verify-report,archive-report,state}`) are owned by the SDD engine and are NOT listed here — do not reuse them.

> All automated saves (SDD phase artifacts, schemas, state) use `capture_prompt: false`.

---

## Schemas

Two closed JSON Schema files (Draft 2020-12) live alongside this document:

- **`./entry-contract.schema.json`** — validates contract version `2.0.0` before handoff. Contains artifact references (not embedded artifact content), resolved delivery parameters, strict-TDD commands, and ordered review slices.
- **`./actuals-record.schema.json`** — validated at closure before writing. Contains implementation, review, and wall-clock hours plus scope and variance notes.

Schema validation alone is insufficient for ordering, dependency, path, range, and delivery invariants. The entry candidate MUST also pass the version-matched `entry-contract-validator` through `labdrian validate-entry-contract`. A missing validator, non-zero exit, or version mismatch fails closed; prose review is not a substitute.

### Requirements Brief Shape

EARS-formatted requirement items (R-NNN), each carrying a `scope` enum of `new-capability | feature | fix`, plus a project-inception handoff table summarizing the change's entry contract inputs.

### Estimate Record Shape

Two effort ranges — `planned_range_hours` (low/high, agent-compute-time basis) and `human_equivalent_hours` (low/high, secondary complexity/tiering signal only, never the headline) — plus `expected_checkpoints` (count of human-confirmation checkpoints), a human-readable `complexity` assessment, a `confidence` level, and the calibration sample size (`n`) the estimate was built from. See `sdd-time-estimation/SKILL.md` for the full agent-compute-time model.

### Entry Contract v2 Shape

The entry contract is one closed object with these groups:

- Identity and execution: `contract_version`, `change_name`, `project`, `tier`, `interaction_mode`, and `artifact_store_mode`.
- Artifact references: `entry`, `requirements`, manifest `context` / `mission` / `rules`, `architecture`, `roadmap`, `estimate`, `sdd_init`, `pipeline_state`, and `delivery`.
- Estimate cache: planned range, complexity, confidence, and confirmation that human review is included. The richer estimate artifact remains authoritative for human-equivalent hours, expected checkpoints, and calibration sample size.
- Delivery normalization: requested PR strategy, resolved delivery strategy, chaining flag/topology, per-slice review budget, and any approved size exception.
- Verification inputs: strict-TDD focused/broad commands, ordered requirement-linked review slices, and the expected native `nextRecommended` value.

Every artifact reference has its stable `topic_key`. An `openspec_path` is optional and MUST be included only when that exact normalized repository-relative path already exists or will be written atomically with the validated candidate. Never invent an OpenSpec path to make `hybrid` or `openspec` look complete. `engram` and `none` modes prohibit OpenSpec paths.

The deterministic validator additionally enforces:

- Exact topic-key derivation from `project` and `change_name`, with no duplicate topic keys or OpenSpec paths.
- Normalized OpenSpec paths below `openspec/` and no OpenSpec paths in `engram` / `none` modes.
- Contiguous slice order, unique slice IDs, and dependencies that reference prior slices only.
- Low/high range ordering and review-budget exception consistency.
- `force-chained` → `auto-chain` + chaining required; `force-single` → `single-pr` or approved `exception-ok`.

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
