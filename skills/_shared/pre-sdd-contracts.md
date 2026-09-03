# Pre-SDD Shared Contracts

## Purpose

This file defines the persistence contracts, language rule, identifier rule, and registry-block format shared by requirements-from-transcripts, project-manifest, project-architect, roadmap-maker, sdd-time-estimation, and inception-pipeline. Each skill references this file instead of re-deciding these conventions independently.

Contract bundle version: `2.1.0`. The inception skill, this authority, both schemas, and the deterministic validator MUST remain version-matched. Missing or mismatched bundle assets are a hard stop.

The bundle version is a compatibility set, not an exact-match lock. The entry schema validates the union of every supported version's vocabulary and still accepts contracts written by a previous supported bundle (currently `2.0.0`); `contract_version` records which bundle produced a contract rather than gating which vocabulary applies. Per-version feature gating is deliberately not enforced: the entry schema carries no conditional keywords by design, and every version-sensitive and cross-field invariant lives in the deterministic validator instead. Emit the current version in new contracts; never rewrite an archived contract to a newer version.

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

- **`./entry-contract.schema.json`** — validates a contract declaring any supported version (`2.0.0` or `2.1.0`) before handoff; new contracts declare `2.1.0`. Contains artifact references (not embedded artifact content), resolved delivery parameters, strict-TDD commands, and ordered review slices.
- **`./actuals-record.schema.json`** — validated at closure before writing. Contains implementation, review, and wall-clock hours plus scope and variance notes.

Schema validation alone is insufficient for ordering, dependency, path, range, and delivery invariants. The entry candidate MUST also pass the version-matched `entry-contract-validator` through `labdrian validate-entry-contract`. A missing validator, non-zero exit, or version mismatch fails closed; prose review is not a substitute.

The actuals record has its own validator and the same fail-closed rule. Run `go run -C "$root/tools/actuals-record-validator" . --schema "$root/skills/_shared/actuals-record.schema.json" --instance <absolute-candidate-path>` (with `root="$(git rev-parse --show-toplevel)"`; every tool under `tools/` is its own Go module, so there is no repository-root module to `go run ./tools/...`) before writing the record; exit `0` is the only result that authorises the write, and a missing or unbuildable validator is a stop, not a pass. It is a separate binary from `entry-contract-validator` because that tool's version handshake reads `properties.contract_version.enum`, which this schema does not have (its version lives in `$id`), and its semantic pass checks entry-contract invariants that do not exist in an actuals record. Beyond schema shape it enforces the closed `approval_decision` vocabulary, non-negative hours and counts, the provenance disclosures a recorded `checkpoint_count` and `total_wall_clock_hours` owe, and that the required narratives carry their own content rather than pointing at where the content supposedly lives — a pointer satisfies every required-field check JSON Schema can make while recording nothing.

### Requirements Brief Shape

EARS-formatted requirement items (R-NNN), each carrying a `scope` enum of `new-capability | feature | fix`, plus a project-inception handoff table summarizing the change's entry contract inputs.

### Estimate Record Shape

Two effort ranges — `planned_range_hours` (low/high, agent-compute-time basis) and `human_equivalent_hours` (low/high, secondary complexity/tiering signal only, never the headline) — plus `expected_checkpoints` (count of human-confirmation checkpoints), a human-readable `complexity` assessment, a `confidence` level, and the calibration sample size (`n`) the estimate was built from. See `sdd-time-estimation/SKILL.md` for the full agent-compute-time model.

### Entry Contract v2 Shape

The entry contract is one closed object with these groups:

- Identity and execution: `contract_version`, `change_name`, `project`, `tier`, `interaction_mode`, and `artifact_store_mode`.
- Artifact references: `entry`, `requirements`, manifest `context` / `mission` / `rules`, `architecture`, `roadmap`, `estimate`, `sdd_init`, `pipeline_state`, and `delivery`.
- Estimate cache: planned range, complexity, confidence, confirmation that human review is included, and the optional `expected_checkpoints` count. The richer estimate artifact remains authoritative for human-equivalent hours and calibration sample size. `expected_checkpoints` is the one exception to "the cache stays thin", and it earns it: it is the PLAN side of the only planning quantity with a recorded actual counterpart (`checkpoint_count` in `sdd/{change}/actuals`), measured over the same tiering-go-ahead-to-merge boundary, so a checkpoint variance is computable from the two records alone. Because the `estimate` object is `additionalProperties: false`, its absence was not a thin cache but an active rejection — a producer carrying the field failed the whole contract with exit 5. Optional, so contracts written before it stay valid; the validator additionally enforces a floor of 1, since the tiering go-ahead checkpoint fires on every change.
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

## Delivery Vocabulary (producer side)

`requested_pr_strategy` is caller intent. `delivery_strategy` is what that intent resolved to once the review budget was applied. The entry contract stores **resolved values only**.

That is why the schema's `delivery_strategy` domain is `single-pr | auto-chain | exception-ok` and does not carry the orchestrator's fourth token, `ask-on-risk`. `ask-on-risk` is an unresolved policy — "ask the user if the tasks forecast flags review-budget risk" — not a delivery outcome. By the time a candidate validates, that question has been answered. A contract carrying `ask-on-risk` would assert that a decision it claims to have made is still open. Never normalize `ask-on-risk` into a contract, and never invent a token for "undecided": an undecided delivery strategy means the candidate is not ready to validate.

`chain_strategy` stores the topology — `none | feature-branch-chain | stacked-to-main` — with `none` required whenever `chaining_required` is `false`.

The full map across every delivery and chaining vocabulary, including the `sdd-tasks` forecast literal (which admits `size-exception` and `pending`, neither of which is a topology and neither of which may reach a contract), lives in `skills/_shared/sdd-orchestrator-workflow.md`, section **Delivery and Chain Vocabulary Map**. Keep the two in sync. If they disagree, the schema and the deterministic validator win for stored values, and the workflow map wins for routing.

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
