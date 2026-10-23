---
name: inception-pipeline
description: "THIS IS THE FIRST SKILL TO USE when starting any unit of work — new project, feature, chore, or fix. It selects the entry tier and drives project-inception when needed; do not invoke project-inception directly unless you explicitly want only the manifest+architect+roadmap trilogy without tier routing. Triggers on: start project, new feature, implement requirement, small fix, kick off SDD, bootstrap, build this, add capability, change request, chore, hotfix, arrancá el proyecto, empezar proyecto, nueva feature, arreglo chico. Sequences the producers as delegated sub-agents, enters the labdrian role chain, and fires closure-feedback after delivery. Never executes producer work itself and never runs the chain's hops."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "3.0.0"
---

## Purpose

You are a **thin pre-chain orchestrator**. You stand in front of the labdrian role chain and turn a raw unit of work into the chain's first record, then hand off. You do FOUR things and nothing else:

1. **Select the entry tier** (1 / 2 / 3) with a reasoning-based checklist.
2. **Sequence the producers** as delegated sub-agents (requirements-capture, project-inception, roadmap-maker).
3. **Enter the role chain** (Chain entry below).
4. **Fire closure-feedback** after the change is delivered.

You NEVER execute producer work yourself (no manifest, no architecture, no code). You NEVER run the chain's hops: `skills/role-chain/SKILL.md` owns them. You only sequence the PRODUCERS and the POST-DELIVERY closure loop.

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
- **Read `references/tier-selection.md` when the tier is ambiguous** (rule 4 fired) — it carries the worked examples. Skip it otherwise.

## Tier Flows (each ends the same way)

Every flow finishes with the identical tail: **Chain entry → closure-feedback after delivery.**

`change-name` is derived **once** by the requirements-capture step (`requirements-from-transcripts`) as a kebab-case slug and inherited verbatim downstream. Never re-derive it.

### Tier 1 — New project
1. Delegate **requirements-from-transcripts** (if a transcript/brief exists) → writes `project/{project}/requirements/{change}`, derives `change-name`.
2. Delegate **project-inception** with `inception_mode: full` and `requirements_brief_ref: {change}` → manifest + architect + roadmap.
3. Chain entry → closure-feedback.

### Tier 2 — Feature/chore on existing project
1. Delegate **requirements-from-transcripts** (per feature) → writes the requirements brief, derives `change-name`.
2. Delegate **project-inception** with `inception_mode: reuse` (loads manifest + architect READ-ONLY; falls back to `full` if either is missing) — this internally drives **roadmap-maker** in `incremental-insert` mode to splice the new item.
3. Chain entry → closure-feedback.

### Tier 3 — Small fix
1. Delegate **requirements-from-transcripts** (lightweight) → derives `change-name`, captures the fix scope.
2. Delegate **roadmap-maker** directly with `mode: incremental-insert`, `tier: 3` → splice the fix item; preserve all actuals/history.
3. Chain entry → closure-feedback.

## Chain entry

After the producers, load `skills/role-chain/SKILL.md`: it owns every hop. This skill writes role-chain records only; it opens no workflow.

- Goal id = chain id = `change-name`: every record goes to `labdrian roles append --project {project} --goal {change} --chain {change} --stdin`.
- The first record is `prototyper → shaper` when a premise is open (the prototyper trigger of the role chain), otherwise `shaper → estimator`.
- Every builder prompt states that strict TDD is active and names the runner, taken from the project's own instructions (`CLAUDE.md`, `AGENTS.md`) or its build files; when neither names one, ask the user once.
- A role agent writes only where its prompt names, so closure-feedback finds an artifact only where you named it. The delivery prompt names the archive report as a file `archive-report.md` inside the project repository: closure-feedback appends the t1 anchor to it, and an anchor outside version control anchors nothing. Name the shaper's spec and the builder's apply-progress the same way when closure-feedback should read them.

## Gate Compliance (non-negotiable orchestrator gates)

When sequencing producers and entering the chain, honor every standing gate:

- **Model gate** — every Agent / sub-agent call MUST include `model`. No model → no call.
- **Launch dedup** — track `(phase, fingerprint)` pairs; emit exactly one launch per distinct task.
- **Skill-resolver injection** — resolve matching `SKILL.md` paths from the registry and inject them as `## Skills to load before work` in each sub-agent prompt (pass paths, not summaries).

## Closure-Feedback (plan → validate → execute; single writer)

Fire this once the chain is terminal: the delivery hop has returned and `labdrian roles resume --project {project} --goal {change} --chain {change}` reports `"terminal": true` (its last record is the completed hand-off to delivery, so no hop follows; `terminal` marks that hand-off, not the merge: `landing_commit` below is read from the merge itself). While it reports `"terminal": false`, or fails because the chain has no records, the chain is still running: do not fire. A terminal chain whose delivery has not merged yet is not closed either: wait for the merge, because t1 and `landing_commit` come from it. The chain's records are files at `$XDG_STATE_HOME/labdrian/role-chains/<project>/<goal>/<chain>/<seq>.json` (`$HOME/.local/state` when `XDG_STATE_HOME` is unset); read the closing `reviewer → delivery` record there. inception-pipeline is the **single writer** of `sdd/{change}/actuals` — this avoids forking the managed `sdd-archive` skill while keeping one authoritative actuals source.

**Plan** — read what the chain left, at the locators the parent named in its prompts (Chain entry), and do NOT modify engine outputs (nothing a hop wrote; the one carve-out is Execute item 3). An optional field whose artifact was never named, or cannot be read, is omitted, never guessed:
- the archive report, the file the delivery prompt named (the delivery hop's envelope lists it under `artifacts`). It is the carve-out target of Execute item 3 and cannot be omitted, so confirm it exists BEFORE the first write: when the delivery prompt named no path, or the file is not there, report it and write nothing. It is also the source of `changed_lines`, if it records a realized diff-stat total; otherwise compute a diff-stat against the base commit
- the verifier's reports, stored beside the review results as `<head12>-<n>.verifier.md` when the verifier ran
- the builder's apply-progress
- the shaper's spec (for `requirement_count`)
- `sdd/{change}/pipeline-state` (for the tier-selection outcome, and the ambiguity clarifying question if fired — the only checkpoints inception-pipeline itself durably records)
- the chain's stored review results, `odd/review-results/<feature>/<head12>-<n>.json` (`<feature>` is the name of the feature document, `odd/tasks/<feature>.md`; `odd/review-results/README.md` spells out the layout) — this is the ONLY place `approved_tree` is read from. The reviewed head is the `candidate.head` of the result that the closing `reviewer → delivery` record binds as its `review-result` evidence, and `approved_tree` is `git show -s --format=%T <reviewed head>`; it becomes the recorded `approved_tree` anchor when a result was stored for this candidate, and is never recorded when none exists — see below. A stored result lists no lenses, so `review_lens_count` is recorded only when the closing `review-report` record's `context` states the rows the reviewer was sent, and is omitted otherwise.
- **t0**: run `engram export <tmp>.json` and select the observation whose `topic_key` equals `sdd/{change}/pipeline-state`; t0 is that observation's typed `created_at` field — never the rendered `Created:` footer of `mem_get_observation`, which can be reflowed or relabeled without notice. WHEN no `pipeline-state` observation exists — a change entered directly at the role chain never ran inception-pipeline — t0 falls back to the earliest `created_at` among the change's own `sdd/{change}/*` observations in the same export, and the fallback MUST be named by source in `variance_vs_plan` so it is never mistaken for a tiering-go-ahead anchor. A `pipeline-state` observation carrying no `topic_key` is invisible to this selector and MUST be treated as absent, falling to the same rule.

Do NOT source `changed_lines` from the shaper's tasks artifact — its Review Workload Forecast is a pre-implementation planning guard, not an exact diff count (see `sdd-tasks/SKILL.md`).

**`total_wall_clock_hours` is measured independently of the compute-hour sum.** It shares the same boundary as `checkpoint_count` — from the tiering go-ahead checkpoint (`sdd/{change}/pipeline-state`) through merge, not archive: post-merge archive-authorization replies are bookkeeping and MUST NOT count as development time. It MUST include any interruption gaps (e.g. provider rate/session-limit resets), not only active agent-compute time. Never derive it by summing `implementation_hours` + `review_gate_hours` + `post_review_fix_hours`; that sum is the agent-compute-time proxy, a distinct and usually smaller unit that must stay separately labeled.

**t1 is anchored in a versioned artifact recorded at delivery, never re-derived from unversioned state.** The chain's review results are stored in the repository, not under `.git/`, and each is bound to its feature by its path (`odd/review-results/<feature>/`): once committed with the feature, a fresh clone has them, and no lineage has to be matched to a change after the fact. closure-feedback takes the reviewed head from the stored result and derives `approved_tree` from it with `git show -s --format=%T <reviewed head>`; it never searches history for the commit that landed the change. Scanning commit history for whichever commit touched the change's own files (`odd/tasks/<feature>.md`, `odd/review-results/<feature>/`) is not a safe substitute: a commit belonging to an unrelated change can touch the same files and silently become the wrong t1 — not hypothetical, this repository's own history contains a case where the same scan, over `openspec/changes/{change}/`, lands on an unrelated PR's merge commit instead of the change's true landing commit. There is no folder-scan fallback anywhere in this rule. WHEN no result is stored for the candidate, there is no independent tree to check the landing commit against and the anchor is self-asserted, never verified — see below.

**closure-feedback WRITES the anchor; it never merely reads one.** The ordering makes this unavoidable: this skill fires AFTER the delivery hop, and the delivery hop writes the archive report BEFORE the merge exists — so no archive report the chain wrote can name a landing commit that had not happened yet. Describing the anchor as something "the archive-report records" therefore assigned the write to a step that had already finished. At delivery — when the merge has just happened and the values are trivially known — derive `landing_commit` from the actual merge commit (the commit that merged the change onto the default branch; the last slice to land when delivered as chained PRs) and record it, in versioned prose, into the `## Cycle Timestamps` section this skill appends (Execute item 3) — always. Record into that same section `approved_tree` (that delivery's approved candidate tree — `git show -s --format=%T <reviewed head>`, the reviewed head being the one the stored review result names) ONLY WHEN review ran for this candidate.

`tools/archive-anchor-gate` is history-only (owner decision 5, ADR C13-S0): it scans the archive reports under `openspec/changes/archive/`, not chain deliveries, and a chain adapter for it is a separate goal; until then the section a chain delivery appends has no machine check, and the anchor rules below are its only guard.

WHEN no review ran, `approved_tree` MUST NOT be recorded at all — never synthesized from `landing_commit`'s own tree, because a check comparing a value against itself can never fail, and reporting an unfailable check as verification is a fabricated assurance, the same defect class this instrument exists to eliminate from the numbers it measures. No review ran for this candidate when no result is stored for it, and also when the stored result's reviewed head does not resolve in this repository (a squashed branch that was deleted): there is no tree to take. WHEN `approved_tree` IS recorded, the anchor is **verifiable rather than merely asserted**: `git show -s --format=%T <landing_commit>` MUST equal the recorded `approved_tree`; WHEN it does not, the anchor is REJECTED, t1 is omitted, and the mismatch is disclosed in `variance_vs_plan` — never trust an unverified SHA. WHEN `approved_tree` is absent because no review ran, t1 STILL resolves from `landing_commit`'s committer timestamp alone — the measurement is not withheld — but the anchor is recorded as **self-asserted**: used, but with no independent authority to check it against. A tree hash MUST be used to **verify** a commit, never to **discover** one. Trees are not unique across commits — a content-preserving merge reproduces its parent's tree — and this repository contains a real three-way collision where `be2c3ca` (the merge that landed `deterministic-verification-evidence`), `aa81361` (an unrelated PR merge) and `459b48d` (an unrelated feature commit) all carry the same tree. Searching that tree for "the commit that landed the change" therefore returns commits belonging to other changes, whichever end of the range is picked; "earliest carrier" is a guess wearing the costume of a rule. WHEN reconstructing a historical record whose archive-report names a landing SHA in versioned prose (e.g. "merged as `be2c3ca`"), use that SHA directly — it is the recorded identity, and the tree is only a cross-check on it. WHEN a tree hash is all that is available and more than one commit on the default branch carries it, the anchor is ambiguous: t1 is omitted and the ambiguity is disclosed, never resolved by position. WHEN no `landing_commit` was ever recorded — a change predating this convention — t1 is omitted (R-021), `variance_vs_plan` states that the change predates the anchor convention, and no folder-scan re-derivation is attempted.

t1 is `landing_commit`'s committer timestamp, once resolved. The resolution outcome — verified, self-asserted, rejected, or absent — MUST be named in `variance_vs_plan` so no reader mistakes a self-asserted anchor (used, but never independently checked) for a verified one (independently checked against a stored review result).

**`checkpoint_count` counts one unit per distinct human round-trip reply (R-015), uniformly regardless of how many decisions a single reply resolved. It shares total_wall_clock_hours' tiering-go-ahead-through-merge boundary.** inception-pipeline hands routing of every hop from the shaper to delivery to the role chain and does not durably observe every live confirmation exchanged during those hops (e.g. mid-design product decisions, judgment-day rounds, chained-PR split confirmation, merge authorization) — do not claim "your own record" of checkpoints you never routed, and do not invent a field to proxy them. Always include the durable floor `sdd/{change}/pipeline-state` actually carries: tiering go-ahead (always 1) + ambiguity clarifying question if fired (0-1) — any blocking ambiguity resolved by asking the user, whatever phase surfaced it and whichever tier-selection rule fired, not only a rule-4 tier ambiguity — this floor is guaranteed, never dropped. If a fuller count is reconstructable from the session narrative, fold it into the single `checkpoint_count` total (never a separate field) and itemize in `variance_vs_plan` free text which checkpoints were durably observed (via `pipeline-state`) versus reconstructed from the closure narrative. If no non-durable checkpoints occurred, `variance_vs_plan` MUST state this explicitly zero, and MUST still name the other half with it — which checkpoints were durably observed via `pipeline-state`, AND that zero were reconstructed from the closure narrative — never silently omitting either. The validator's half-disclosure rule rejects prose naming only one half, so "all durably observed" alone and "zero reconstructed" alone are both hard stops. Post-merge archive-authorization replies are excluded from this count as post-boundary bookkeeping, same as `total_wall_clock_hours`.

**Two more figures are yours to WRITE, and both live in `variance_vs_plan` prose because the schema is closed (R-003/R-004/R-005 forbid adding a property, and `additionalProperties: false` is unchanged).** Each was MANDATORY on the reading side through `sdd-time-estimation/SKILL.md`, which no longer reads either figure since its rewrite; `roadmap-maker/assets/roadmap-template.md` still gives `realized_slices` a tracking row, so an unwritten one leaves that mechanism inert, reading a field nobody fills. They ride the same prose-disclosure precedent as the durable-vs-reconstructed checkpoint split, and they inherit its stated cost: free text is not machine-verifiable.

- **`interruption_hours: <n>`** — the observed interruption duration for this change, naming its cause (session reset, provider rate limit, human unavailability). Record it whenever an interruption occurred; record `interruption_hours: 0` explicitly when none did, because "no line" and "no interruption" are different facts and a reader cannot tell them apart. R-011, which excluded interruption-contaminated samples from the round-trip-latency calibration, was retired together with that calibration, so no skill reads this figure now; it stays a record of what interrupted the change.
- **`realized_slices: <n>`** — how many review candidates actually landed, counted from what exists (the PRs delivered under the chosen `chain_strategy`, or the stored review results that carried their own review boundary — one per slice: a second blind launch of one head is a second file, not a second slice), never from the plan. The planned side is the slice count the chain's shaper/estimator record states; there is no structured field for either side in the actuals record. When the planned count is not stated there, write `planned slice count not recorded`; when the realized count genuinely cannot be established, write `realized slice count not recorded` — never reuse the planned number as though it were the outcome, which is precisely how an overrun disappears into a plan that looks like it held.

**Validate** — compute the actuals object **once** (implementation_hours, review_gate_hours, total_wall_clock_hours when resolved, post_review_fix_hours, approval_decision, scope_drift_notes, variance_vs_plan, plus the optional complexity-unit fields `requirement_count`, `changed_lines`, `review_lens_count`, `checkpoint_count` when their source artifact is readable) and validate it with the **deterministic validator**, never by reading it:

```bash
# -C changes the process working directory, so both paths are absolute. Each
# tool under tools/ is its own Go module; there is no repository-root module.
root="$(git rev-parse --show-toplevel)"
go run -C "$root/tools/actuals-record-validator" . \
  --schema "$root/skills/_shared/actuals-record.schema.json" \
  --instance /absolute/path/to/actuals-candidate.json
```

**Fail-closed rule.** Exit `0` is the only result that authorises the write. Any non-zero exit — and a missing or unbuildable validator, which is not the same thing as a pass — is a hard stop: report it and write nothing. Reading the JSON, checking that the fields look present, or attesting that you were careful is NOT validation; the record's author cannot be its validator. Beyond schema shape the validator enforces what shape cannot express: `approval_decision` inside the closed outcome vocabulary, non-negative hours and counts, the durable-vs-reconstructed disclosure a recorded `checkpoint_count` owes, the measured-vs-reconstructed disclosure a recorded `total_wall_clock_hours` owes, and — the rule that exists because it was actually broken — that `scope_drift_notes` and `variance_vs_plan` carry their own content instead of deferring it to some other section ("see the narrative below — this field is summarised there rather than duplicated" satisfies every required-field check JSON Schema can make while recording nothing). Cross-reference in ADDITION to the content, never INSTEAD of it. The complexity-unit fields are recorded for reference only (`sdd-time-estimation` no longer derives a per-unit calibration rate from them) — omit any field whose source artifact could not be read rather than guessing; `total_wall_clock_hours` is validly absent when its anchors do not resolve (R-021), and `implementation_hours`, `review_gate_hours`, and `post_review_fix_hours` are validly absent while no durable source exists for them (R-019) — every other resolved field is still required. If validation fails for any other reason, report and STOP — do not partial-write.

**`implementation_hours`, `review_gate_hours`, and `post_review_fix_hours` stay unpopulated (R-019) — state why, do not leave it silent.** No durable source exists for any of the three yet: orchestrator telemetry is session-transient, the `sdd-attempt` ledger has no timestamp fields, and transcripts carry no structured subagent durations. None of the three MAY be filled with `total_wall_clock_hours` or any other elapsed-time proxy while this remains true.

**Execute** — write, in order:
1. `sdd/{change}/actuals` (the authoritative single actuals notebook).
2. `project/{project}/estimation-calibration` (append/update — variance patterns; no skill reads it since `sdd-time-estimation` was rewritten).
3. Append a `## Cycle Timestamps` section to the archived `archive-report.md` the delivery prompt named (confirmed in Plan) — a named, delimited, append-only carve-out to closure-feedback's own "do NOT modify engine outputs" rule (the report's other bytes untouched): a 2-row table with t0 (topic key, observation id, `created_at`, primary-or-fallback source) and t1 (`landing_commit`, `approved_tree` when recorded, committer timestamp, and the verified/self-asserted/rejected outcome).

On any partial-write failure (any of the three written, another not), **report the inconsistency and STOP**. Do not silently leave the stores out of sync.

4. AFTER the three writes above succeed, fire the archive-time sync trigger from the project root:

```bash
labdrian longterm-mem sync-trigger --event archive --cwd "$root" || true
```

This is a **best-effort, non-blocking, informational** call — it runs strictly after the STOP-on-partial-write rule above, never before, and its outcome (including the `|| true` swallowing a non-zero exit) never blocks or reverses closure. `labdrian longterm-mem sync-trigger` itself always exits 0 regardless of whether the sync it schedules actually ran (the `sync-trigger` branch of `cmd_longterm_mem` in `bin/lib/longterm_mem.sh`), so the trailing `|| true` here is a second, redundant safety net — kept because closure-feedback must stay non-blocking even if that guarantee ever regresses.

Downstream consumers read on their next run — you do NOT push to them:
- **roadmap-maker** reads `sdd/{change}/actuals` on its next render.
- **sdd-time-estimation** no longer reads `project/{project}/estimation-calibration`; its reference class comes from the `odd/tasks/*.md` and archived `tasks.md` ledgers.

## Gotchas (real traps)

- **Single actuals writer.** Only closure-feedback writes `sdd/{change}/actuals`. Never let `sdd-archive` or any other skill write it — do NOT modify the managed `sdd-archive`.
- **Assert foundation before Tier 2/3.** manifest + architect MUST exist in engram before choosing Tier 2 or 3. Missing manifest → Tier 1.
- **Default up on ambiguity.** When the tier is unclear, choose the higher tier, record rationale in `sdd/{change}/pipeline-state`, ask at most one question.
- **change-name is derived once.** requirements-capture owns it; inherit verbatim, never re-derive downstream.
- **Stop on partial closure writes.** If actuals, calibration, or the archive-report append fails, report and stop — do not leave the stores inconsistent.
- **The archive-report append is a named carve-out, not a precedent.** The `## Cycle Timestamps` append-only carve-out applies to that one delimited section only — it does not license editing any other part of the delivery hop's archive report.
- **The archive-time sync trigger never gates closure.** Execute step 4's `labdrian longterm-mem sync-trigger --event archive --cwd "$root" || true` runs after all three writes succeed and its result is purely informational — a failed, skipped, or missing sync is never a reason to report closure as incomplete, and it lives here rather than in the managed `sdd-archive/SKILL.md`.

## References

- `../_shared/pre-sdd-contracts.md` — contract authority: topic keys, change-name rule, language rule, schemas, registry-block format.
- `skills/role-chain/SKILL.md` — the chain this skill enters; it owns every hop.
- `odd/review-results/README.md` — the layout of the stored review results closure-feedback reads.
- `../_shared/actuals-record.schema.json` — actuals record schema (validated at closure).
- `references/tier-selection.md` — worked tier-selection examples. **Load conditionally**: read only when the tier is ambiguous (rule 4).
