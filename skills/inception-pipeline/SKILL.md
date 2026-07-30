---
name: inception-pipeline
description: "THIS IS THE FIRST SKILL TO USE when starting any unit of work — new project, feature, chore, or fix. It selects the entry tier and drives project-inception when needed; do not invoke project-inception directly unless you explicitly want only the manifest+architect+roadmap trilogy without tier routing. Triggers on: start project, new feature, implement requirement, small fix, kick off SDD, bootstrap, build this, add capability, change request, chore, hotfix, arrancá el proyecto, empezar proyecto, nueva feature, arreglo chico. Sequences the pre-SDD producers as delegated sub-agents, assembles and validates the entry contract, hands off to the SDD engine, and fires closure-feedback after archive. Never executes producer work itself and never routes SDD phases."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "2.0.0"
---

## Purpose

You are a **thin pre-SDD orchestrator**. You stand in front of the SDD engine and turn a raw unit of work into a validated entry contract, then hand off. You do FIVE things and nothing else:

1. **Select the entry tier** (1 / 2 / 3) with a reasoning-based checklist.
2. **Sequence the pre-SDD producers** as delegated sub-agents (requirements-capture, project-inception, roadmap-maker, sdd-time-estimation).
3. **Assemble + deterministically validate** the versioned `sdd/{change}/entry` contract.
4. **Confirm readiness** via the native dispatcher and hand off to the SDD engine (gentleman-agents-team-lite).
5. **Fire closure-feedback** after the engine archives the change.

You NEVER execute producer work yourself (no manifest, no architecture, no estimate, no code). You NEVER route SDD phases — the native `gentle-ai` dispatcher owns proposal → spec → design → tasks → apply → verify → archive. You only sequence the PRE-SDD producers and the POST-SDD closure loop.

All persisted artifacts are written in **English** (see `../_shared/pre-sdd-contracts.md`). The user-facing conversation may stay in the user's language.

## Tier Selection (run first, returns ONE tier + the rule that fired)

Evaluate in order. Stop at the first rule that fires. Record the firing rule in `sdd/{change}/pipeline-state`.

| # | Condition | Tier | Meaning |
|---|---|---|---|
| 1 | No `project/{project}/manifest/context` in engram | **Tier 1** | New project — full inception |
| 2 | manifest + architect exist AND change is self-contained, single surface, **no new architectural surface**, no schema / integration / production risk | **Tier 3** | Small fix — lightweight path |
| 3 | manifest + architect exist AND change adds a feature/capability needing roadmap sequencing or cross-cutting work | **Tier 2** | Feature/chore — reuse foundation |
| 4 | Ambiguous (2 vs 3, or contradictory signals) | **default UP one tier** | Record rationale; ask ONE clarifying question only if genuinely ambiguous |

Rules:
- Tier is decided by **evidence**, not user phrasing. Assert manifest + architect existence in engram before choosing Tier 2 or 3. Absence of the manifest forces Tier 1 regardless of stated intent.
- On ambiguity, **default up** (3→2, 2→1), record the rationale in `sdd/{change}/pipeline-state`, and ask at most ONE clarifying question.
- Persist the chosen tier into the `tier` field of the entry contract.
- **Read `references/tier-selection.md` when the tier is ambiguous** (rule 4 fired) — it carries the worked examples. Skip it otherwise.

## Tier Flows (each ends the same way)

Every flow finishes with the identical tail: **assemble a temporary entry candidate → validate it with the version-matched schema + deterministic validator → persist the exact validated bytes to `sdd/{change}/entry` → `gentle-ai sdd-status {change} --cwd {project-root} --json --instructions` gate → hand to team-lite (route ONLY by `nextRecommended`) → closure-feedback after archive.**

`change-name` is derived **once** by the requirements-capture step (`requirements-from-transcripts`) as a kebab-case slug and inherited verbatim downstream. Never re-derive it.

### Tier 1 — New project
1. Delegate **requirements-from-transcripts** (if a transcript/brief exists) → writes `project/{project}/requirements/{change}`, derives `change-name`.
2. Delegate **project-inception** with `inception_mode: full` and `requirements_brief_ref: {change}` → manifest + architect + roadmap.
3. Delegate **sdd-time-estimation** → writes `sdd/{change}/estimate`.
4. Assemble + validate entry → dispatcher gate → engine → closure-feedback.

### Tier 2 — Feature/chore on existing project
1. Delegate **requirements-from-transcripts** (per feature) → writes the requirements brief, derives `change-name`.
2. Delegate **project-inception** with `inception_mode: reuse` (loads manifest + architect READ-ONLY; falls back to `full` if either is missing) — this internally drives **roadmap-maker** in `incremental-insert` mode to splice the new item.
3. Delegate **sdd-time-estimation** → writes `sdd/{change}/estimate`.
4. Assemble + validate entry → dispatcher gate → engine → closure-feedback.

### Tier 3 — Small fix
1. Delegate **requirements-from-transcripts** (lightweight) → derives `change-name`, captures the fix scope.
2. Delegate **roadmap-maker** directly with `mode: incremental-insert`, `tier: 3` → splice the fix item; preserve all actuals/history.
3. Delegate **sdd-time-estimation** → writes `sdd/{change}/estimate`.
4. Assemble + validate entry → dispatcher gate → engine (recommend a **slim slice**) → closure-feedback.

## Entry Contract Assembly & Validation (v2; fail closed)

Read `../_shared/pre-sdd-contracts.md` and `../_shared/entry-contract.schema.json` before assembly. The skill metadata, shared authority, entry schema, actuals schema, and validator MUST all declare contract bundle `2.0.0`. If an asset is missing or mismatched, **STOP**. Do not reconstruct a schema from memory or fall back to prose validation.

Assemble one candidate JSON object in a temporary file. It MUST contain every required v2 group:

- Identity: `contract_version`, inherited `change_name`, `project`, selected `tier`, `interaction_mode`, and `artifact_store_mode`.
- `artifact_refs`: entry, requirements, manifest context/mission/rules, architecture, roadmap, estimate, SDD init, pipeline state, and delivery plan.
- Estimate cache: `planned_range_hours`, lowercase-normalized `complexity` (`Critical` → `critical`), `confidence`, and `human_review_included: true`. Keep richer estimate-only calibration fields in `sdd/{change}/estimate`; do not duplicate them here.
- Delivery decision: `requested_pr_strategy`, normalized `delivery_strategy`, `chaining_required`, `chain_strategy`, and `review_budget` including size-exception state.
- Test/review plan: `strict_tdd.enabled`, non-empty focused and broad test command lists, ordered requirement-linked `review_slices`, and `expected_native_next_recommendation`.

Artifact references carry the stable topic keys defined by the shared authority. Before validation, assert every producer artifact exists. Include `openspec_path` only for a real, normalized repository-relative path below `openspec/`; omit it when no such artifact exists. Never synthesize a path from a topic key. In `engram` or `none` mode, omit every `openspec_path`.

Normalize delivery before validation:

- `force-chained` resolves to `auto-chain`, `chaining_required: true`, and a concrete non-`none` chain strategy.
- `force-single` resolves to `single-pr` when every slice is within budget, or `exception-ok` only with an explicitly approved size exception.
- `auto-chain` always means chaining is required. `single-pr` and `exception-ok` always mean it is not.
- Slice order starts at 1 and is contiguous. IDs are unique; dependencies name prior slices only. A slice over budget requires an approved exception.

Run the repository wrapper so caller-relative paths work and validator exit codes remain visible:

```bash
labdrian validate-entry-contract \
  --schema skills/_shared/entry-contract.schema.json \
  --instance /absolute/path/to/entry-candidate.json
```

Any non-zero result is a hard stop. Do not persist a partially validated object. On success, persist the exact candidate bytes to `sdd/{change}/entry` with `capture_prompt: false` and, only when a real OpenSpec entry path was declared, atomically write the same object there. Then delete the temporary candidate.

## Dispatcher Gate & Handoff (route by native JSON only)

Route SDD **only** through the native dispatcher status JSON:

```
gentle-ai sdd-status {change} --cwd {project-root} --json --instructions
```

- Read `nextRecommended` and `blockedReasons` from the JSON. Treat them as authoritative over any free text.
- Preserve the native token exactly: phase recommendations are unprefixed (`propose`, `spec`, `design`, `tasks`, `apply`, `verify`, `remediate`, `archive`); control/review recommendations include `sdd-new`, `select-change`, `review`, and `resolve-review`.
- Assert `nextRecommended` matches `expected_native_next_recommendation` in the validated entry. A mismatch means the handoff is stale or incomplete: **STOP**, refresh evidence, and rebuild the candidate rather than editing one field in place.
- **Cold-start exemption (first gate call for a new change).** A change the engine has not created yet is not a blocked change. The gate PASSES — hand off — when ALL of these hold: `nextRecommended` is `sdd-new` or `select-change`, `changeRoot` is `null`, every `artifacts` value is `missing`, and every entry in `blockedReasons` names only the not-yet-created change. This is the normal first-run state under `openspec` and `hybrid`: inception-pipeline never creates the change root (it does not route SDD phases), so the engine must create it. Treating this state as a blocker would deadlock every new change at its own gate. A cold-start handoff still requires `expected_native_next_recommendation` to have predicted the same control token, per the assert above.
- If `blockedReasons` is non-empty for any other reason AND `nextRecommended` is not `verify`, **STOP** and report the blockers. Do not hand off, do not archive.
- If `nextRecommended` is `verify`, verification/remediation may run to refresh evidence.
- Hand off to the SDD engine (gentleman-agents-team-lite) and let it route every SDD phase by `nextRecommended`. inception-pipeline does NOT route phases itself.

## Gate Compliance (non-negotiable orchestrator gates)

When sequencing producers and handing off, the orchestrator MUST honor every standing gate:

- **SDD Init Guard** — before any SDD command, confirm `sdd-init` ran for the project; if not, run it first.
- **Version-matched entry gate** — all v2 bundle assets must exist and the deterministic validator must return 0 before the entry is persisted or the dispatcher runs.
- **Artifact-reference truthfulness** — every topic key resolves; every declared OpenSpec path is real and normalized. Missing evidence stops the flow.
- **Model gate** — every Agent / sub-agent call MUST include `model`. No model → no call.
- **Launch dedup** — track `(phase, fingerprint)` pairs; emit exactly one launch per distinct task.
- **Skill-resolver injection** — resolve matching `SKILL.md` paths from the registry and inject them as `## Skills to load before work` in each sub-agent prompt (pass paths, not summaries).
- **Strict-TDD forwarding** — if `sdd-init/{project}` has `strict_tdd: true`, forward strict-TDD mode + test runner to `sdd-apply` / `sdd-verify`.
- **Apply-progress continuity** — for continuation batches, tell `sdd-apply` that prior `sdd/{change}/apply-progress` exists and must be MERGED, not overwritten.
- **Review Workload Guard** — after `sdd-tasks`, inspect the Review Workload Forecast and apply the cached `delivery_strategy` (and `chain_strategy` if chaining) before `sdd-apply`.

These gates belong to the orchestrator running the SDD engine. inception-pipeline forwards the inputs (strict_tdd, delivery_strategy, chain_strategy, artifact_store_mode) via the entry contract; it never routes the phases itself.

## Closure-Feedback (plan → validate → execute; single writer)

Fire this AFTER the SDD engine archives the change. inception-pipeline is the **single writer** of `sdd/{change}/actuals` — this avoids forking the managed `sdd-archive` skill while keeping one authoritative actuals source.

**Plan** — read the engine's native outputs (do NOT modify them):
- `sdd/{change}/archive-report` (for `changed_lines`, if it records a realized diff-stat total; otherwise compute a diff-stat against the base commit)
- `sdd/{change}/verify-report`
- `sdd/{change}/apply-progress`
- `sdd/{change}/spec` (for `requirement_count`)
- `sdd/{change}/pipeline-state` (for the tier-selection outcome, and the ambiguity clarifying question if fired — the only checkpoints inception-pipeline itself durably records)
- the native `gentle-ai review` receipt (`gentle-ai.review-receipt/v2`), if one exists (its `selected_lenses` array length is `review_lens_count`)

Do NOT source `changed_lines` from `sdd/{change}/tasks` — its Review Workload Forecast is a pre-implementation planning guard, not an exact diff count (see `sdd-tasks/SKILL.md`).

**`total_wall_clock_hours` is measured independently of the compute-hour sum.** It shares the same boundary as `checkpoint_count` — from the tiering go-ahead checkpoint (`sdd/{change}/pipeline-state`) through archive — and MUST include any interruption gaps (e.g. provider rate/session-limit resets), not only active agent-compute time. Never derive it by summing `implementation_hours` + `review_gate_hours` + `post_review_fix_hours`; that sum is the agent-compute-time proxy, a distinct and usually smaller unit that must stay separately labeled.

**`checkpoint_count` counts one unit per distinct human round-trip reply (R-015), uniformly regardless of how many decisions a single reply resolved.** inception-pipeline hands routing of proposal → spec → design → tasks → apply → verify → archive to the SDD engine and does not durably observe every live confirmation exchanged during those phases (e.g. mid-design product decisions, judgment-day rounds, chained-PR split confirmation, merge authorization) — do not claim "your own record" of checkpoints you never routed, and do not invent a field on `sdd/{change}/entry` to proxy them. Always include the durable floor `sdd/{change}/pipeline-state` actually carries: tiering go-ahead (always 1) + ambiguity clarifying question if fired (0-1) — any blocking ambiguity resolved by asking the user, whatever phase surfaced it and whichever tier-selection rule fired, not only a rule-4 tier ambiguity — this floor is guaranteed, never dropped. If a fuller count is reconstructable from the session narrative, fold it into the single `checkpoint_count` total (never a separate field) and itemize in `variance_vs_plan` free text which checkpoints were durably observed (via `pipeline-state`) versus reconstructed from the closure narrative. If no non-durable checkpoints occurred, `variance_vs_plan` MUST state this explicitly zero — never silently omit the disclosure.

**Validate** — compute the actuals object **once** (implementation_hours, review_gate_hours, total_wall_clock_hours, post_review_fix_hours, approval_decision, scope_drift_notes, variance_vs_plan, plus the optional complexity-unit fields `requirement_count`, `changed_lines`, `review_lens_count`, `checkpoint_count` when their source artifact is readable) and validate it against `../_shared/actuals-record.schema.json`. The complexity-unit fields feed `sdd-time-estimation`'s per-unit calibration rate (hours per requirement, hours per changed line) once a project accumulates enough actuals — omit any field whose source artifact could not be read rather than guessing. If validation fails, report and STOP — do not partial-write.

**Execute** — write both, in order:
1. `sdd/{change}/actuals` (the authoritative single actuals notebook).
2. `project/{project}/estimation-calibration` (append/update — variance patterns for future pre-start estimates).

On any partial-write failure (one written, the other not), **report the inconsistency and STOP**. Do not silently leave the two stores out of sync.

Downstream consumers read on their next run — you do NOT push to them:
- **roadmap-maker** reads `sdd/{change}/actuals` on its next render.
- **sdd-time-estimation** reads `project/{project}/estimation-calibration` on its next pre-start estimate.

## Gotchas (real traps)

- **Single actuals writer.** Only closure-feedback writes `sdd/{change}/actuals`. Never let `sdd-archive` or any other skill write it — do NOT modify the managed `sdd-archive`.
- **Never route SDD phases.** The native dispatcher owns proposal → … → archive. inception-pipeline only sequences PRE-SDD producers + POST-SDD closure.
- **Assert foundation before Tier 2/3.** manifest + architect MUST exist in engram before choosing Tier 2 or 3. Missing manifest → Tier 1.
- **Default up on ambiguity.** When the tier is unclear, choose the higher tier, record rationale in `sdd/{change}/pipeline-state`, ask at most one question.
- **Route by native JSON only.** Use `gentle-ai sdd-status {change} --cwd {project-root} --json --instructions` → `nextRecommended` / `blockedReasons`.
- **Never weaken validation.** Missing/mismatched v2 assets, validator failures, stale dispatcher recommendations, and fabricated OpenSpec paths all stop the handoff.
- **change-name is derived once.** requirements-capture owns it; inherit verbatim, never re-derive downstream.
- **Stop on partial closure writes.** If actuals or calibration write fails, report and stop — do not leave the stores inconsistent.

## References

- `../_shared/pre-sdd-contracts.md` — contract authority: topic keys, change-name rule, language rule, schemas, registry-block format.
- `../_shared/entry-contract.schema.json` — entry contract schema (validated at the dispatcher gate).
- `../_shared/actuals-record.schema.json` — actuals record schema (validated at closure).
- `references/tier-selection.md` — worked tier-selection examples. **Load conditionally**: read only when the tier is ambiguous (rule 4).
