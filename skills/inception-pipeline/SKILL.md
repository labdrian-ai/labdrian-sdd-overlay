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

1. Name the chain: goal id = chain id = `change-name`. Every record goes to `labdrian roles append --project {project} --goal {change} --chain {change} --stdin`.
2. Make the first record `prototyper → shaper` when a premise is open (the prototyper trigger of the role chain), otherwise `shaper → estimator`:

   ```json
   {
     "version": 1,
     "project_id": "acme-shop",
     "goal_id": "add-wishlist",
     "chain_id": "add-wishlist",
     "seq": 1,
     "from_role": "shaper",
     "to_role": "estimator",
     "prev_sha256": "0000000000000000000000000000000000000000000000000000000000000000",
     "payload_kind": "tasks",
     "payload_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
     "evidence": [],
     "context": {"summary": "Tasks shaped: 4 tasks, 1 slice.", "decisions": [], "open_questions": []},
     "status": "completed"
   }
   ```

   Replace the ids with the project and the change-name, and `payload_sha256` with the sha256 of the payload (here the shaper's tasks artifact). A first record's `prev_sha256` is 64 zeros. The engine refuses a record with a member missing, misspelled or unknown.
3. In every builder prompt, state that strict TDD is active and name the runner, taken from the project's own instructions (`CLAUDE.md`, `AGENTS.md`) or its build files. When neither names one, ask the user once.
4. A role agent writes only where its prompt names, so closure-feedback finds an artifact only where you named it. Have the delivery prompt name the archive report as a file `archive-report.md` inside the project repository: closure-feedback appends the t1 anchor to it, and an anchor outside version control anchors nothing.
5. Name the shaper's spec and the builder's apply-progress the same way when closure-feedback should read them.

## Gate Compliance (non-negotiable orchestrator gates)

When sequencing producers and entering the chain, honor every standing gate:

- **Model gate** — every Agent / sub-agent call MUST include `model`. No model → no call.
- **Launch dedup** — track `(phase, fingerprint)` pairs; emit exactly one launch per distinct task.
- **Skill-resolver injection** — resolve matching `SKILL.md` paths from the registry and inject them as `## Skills to load before work` in each sub-agent prompt (pass paths, not summaries).

## Closure-Feedback (plan → validate → execute; single writer)

inception-pipeline is the **single writer** of `sdd/{change}/actuals`. That avoids forking the managed `sdd-archive` skill and keeps one authoritative actuals source.

### When to fire

1. Run `labdrian roles resume --project {project} --goal {change} --chain {change}`.
2. `"terminal": true` means the delivery hop has returned: the last record is the completed hand-off to delivery and no hop follows. It marks that hand-off, not the merge.
3. `"terminal": false`, or an error because the chain has no records, means the chain is still running: do not fire.
4. A terminal chain whose delivery has not merged yet is not closed either. Wait for the merge: t1 and `landing_commit` come from it.

The chain's records are files at `$XDG_STATE_HOME/labdrian/role-chains/<project>/<goal>/<chain>/<seq>.json` (`$HOME/.local/state` when `XDG_STATE_HOME` is unset). Read the closing `reviewer → delivery` record there.

### Plan

Read what the chain left, at the locators the parent named in its prompts (Chain entry). Do NOT modify hop outputs (anything a hop wrote); the one carve-out is Execute item 3. Omit an optional field whose artifact was never named or cannot be read; never guess it.

- **Archive report**: the file the delivery prompt named (the delivery hop's envelope lists it under `artifacts`). Execute item 3 writes into it and cannot be omitted, so confirm it exists BEFORE the first write. If the delivery prompt named no path, or the file is not there, report it and write nothing. It is also the source of `changed_lines` if it records a realized diff-stat total; otherwise compute a diff-stat against the base commit.
- **Verifier reports**: `<head12>-<n>.verifier.md`, stored beside the review results, when the verifier ran.
- **Builder's apply-progress** and **shaper's spec** (the spec gives `requirement_count`).
- **`sdd/{change}/pipeline-state`**: the tier-selection outcome and the ambiguity clarifying question if it fired — the only checkpoints inception-pipeline itself durably records.
- **Stored review results**: `odd/review-results/<feature>/<head12>-<n>.json`, where `<feature>` is the name of the feature document `odd/tasks/<feature>.md` (`odd/review-results/README.md` spells out the layout). This is the ONLY place `approved_tree` is read from (see t1). A stored result lists no lenses, so record `review_lens_count` only when the closing `review-report` record's `context` states the rows the reviewer was sent.
- Do NOT source `changed_lines` from the shaper's tasks artifact: its Review Workload Forecast is a pre-implementation planning guard, not an exact diff count (see `sdd-tasks/SKILL.md`).

#### t0

1. Run `engram export <tmp>.json` and select the observation whose `topic_key` equals `sdd/{change}/pipeline-state`.
2. t0 is that observation's typed `created_at` field — never the rendered `Created:` footer of `mem_get_observation`, which can be reflowed or relabeled without notice.
3. If no `pipeline-state` observation exists (a change entered directly at the role chain never ran inception-pipeline), t0 falls back to the earliest `created_at` among the change's own `sdd/{change}/*` observations in the same export. The fallback MUST be named by source in `variance_vs_plan`, so it is never mistaken for a tiering-go-ahead anchor.
4. A `pipeline-state` observation with no `topic_key` is invisible to this selector: treat it as absent and apply step 3.

#### t1 and the landing anchor

t1 is `landing_commit`'s committer timestamp, once resolved. The anchor is recorded in a versioned artifact at delivery and is never re-derived from unversioned state.

1. The chain's review results live in the repository, not under `.git/`, and each is bound to its feature by its path (`odd/review-results/<feature>/`). Committed with the feature, they reach a fresh clone.
2. Closure-feedback WRITES the anchor; it never merely reads one. This skill fires after the delivery hop, and that hop wrote the archive report before the merge existed, so nothing the chain wrote can name the landing commit. Once the merge has happened, record in versioned prose, in the `## Cycle Timestamps` section this skill appends (Execute item 3):
   - `landing_commit`, always: the actual merge commit that merged the change onto the default branch (the last slice to land when delivered as chained PRs);
   - `approved_tree`, ONLY WHEN review ran for this candidate: `git show -s --format=%T <reviewed head>`, the reviewed head being the `candidate.head` of the stored result that the closing `reviewer → delivery` record binds as its `review-result` evidence.
3. No review ran when no result is stored for this candidate, and also when the stored result's reviewed head does not resolve in this repository (a squashed branch that was deleted): there is no tree to take. Then `approved_tree` MUST NOT be recorded at all. Never synthesize it from `landing_commit`'s own tree: a check comparing a value against itself can never fail, and reporting an unfailable check as verification is a fabricated assurance.
4. When `approved_tree` IS recorded, the anchor is **verifiable rather than merely asserted**: `git show -s --format=%T <landing_commit>` MUST equal the recorded `approved_tree`. When it does not, the anchor is REJECTED, t1 is omitted, and the mismatch is disclosed in `variance_vs_plan` — never trust an unverified SHA.
5. When `approved_tree` is absent because no review ran, t1 STILL resolves from `landing_commit`'s committer timestamp alone — the measurement is not withheld — but the anchor is recorded as **self-asserted**: used, but with no independent authority to check it against.
6. Never search history for the commit that landed the change.
   - A tree hash MUST be used to **verify** a commit, never to **discover** one. Trees are not unique across commits (a content-preserving merge reproduces its parent's tree; `be2c3ca`, `aa81361` and `459b48d` all carry one tree in this repository), so a tree search returns commits of other changes whichever end of the range is picked, and the "earliest carrier" is a guess, not a rule.
   - Scanning history for whichever commit touched the change's own files (`odd/tasks/<feature>.md`, `odd/review-results/<feature>/`) is no safe substitute: a commit of an unrelated change can touch the same files and silently become the wrong t1. There is no folder-scan fallback anywhere in this rule.
   - A historical record whose archive report names a landing SHA in versioned prose (e.g. "merged as `be2c3ca`"): use that SHA directly. It is the recorded identity; the tree is only a cross-check on it.
   - If a tree hash is all that is available and more than one commit on the default branch carries it, the anchor is ambiguous: t1 is omitted and the ambiguity is disclosed, never resolved by position.
   - If no `landing_commit` was ever recorded (a change predating this convention), t1 is omitted (R-021), `variance_vs_plan` states that the change predates the anchor convention, and no folder-scan re-derivation is attempted.
7. The resolution outcome — verified, self-asserted, rejected, or absent — MUST be named in `variance_vs_plan`, so no reader mistakes a self-asserted anchor (used, but never independently checked) for a verified one (checked against a stored review result).
8. `tools/archive-anchor-gate` is history-only (owner decision 5, ADR C13-S0): it scans only the archive reports under `openspec/changes/archive/`. Its chain adapter is a separate goal (the ADR's Follow-ups); until then a report outside that tree has no machine check and the rules above are its only guard.

#### Figures

- **`total_wall_clock_hours`** is measured independently of the compute-hour sum. It runs from the tiering go-ahead checkpoint (`sdd/{change}/pipeline-state`) through merge, not archive: post-merge archive-authorization replies are bookkeeping and MUST NOT count as development time. It MUST include interruption gaps (such as provider rate or session-limit resets), not only active agent-compute time. Never derive it by summing `implementation_hours` + `review_gate_hours` + `post_review_fix_hours`: that sum is the agent-compute-time proxy, a distinct and usually smaller unit that must stay separately labeled.
- **`checkpoint_count`** counts one unit per distinct human round-trip reply (R-015), uniformly regardless of how many decisions a single reply resolved. It shares the boundary of `total_wall_clock_hours`, from the tiering go-ahead through merge; post-merge archive-authorization replies are excluded from this count as post-boundary bookkeeping.
  - The role chain routes every hop from the shaper to delivery, so inception-pipeline does not durably observe every live confirmation exchanged during those hops (mid-design product decisions, judgment-day rounds, chained-PR split confirmation, merge authorization). Do not claim "your own record" of checkpoints you never routed, and do not invent a field to proxy them.
  - Always include the durable floor that `sdd/{change}/pipeline-state` carries: the tiering go-ahead (always 1) plus the ambiguity clarifying question if it fired (0-1). Any blocking ambiguity resolved by asking the user counts, whatever phase surfaced it and whichever tier-selection rule fired, not only a rule-4 tier ambiguity. The floor is guaranteed and never dropped.
  - If a fuller count is reconstructable from the session narrative, fold it into the single `checkpoint_count` total (never a separate field) and itemize in `variance_vs_plan` free text which checkpoints were durably observed (via `pipeline-state`) versus reconstructed from the closure narrative.
  - If no non-durable checkpoints occurred, `variance_vs_plan` MUST state this explicitly zero and MUST still name the other half with it: which checkpoints were durably observed via `pipeline-state`, AND that zero were reconstructed from the closure narrative. The validator's half-disclosure rule rejects prose naming only one half, so "all durably observed" alone and "zero reconstructed" alone are both hard stops.
- **Two more figures** go in `variance_vs_plan` prose, because the schema is closed (R-003/R-004/R-005 forbid adding a property, and `additionalProperties: false` is unchanged). Free text is not machine-verifiable.
  - `interruption_hours: <n>` — the observed interruption duration for this change, naming its cause (session reset, provider rate limit, human unavailability). Record it whenever an interruption occurred. Record `interruption_hours: 0` explicitly when none did, because "no line" and "no interruption" are different facts and a reader cannot tell them apart. No skill reads this figure now (R-011, which excluded interruption-contaminated samples from the round-trip-latency calibration, was retired with that calibration); it stays a record of what interrupted the change.
  - `realized_slices: <n>` — how many review candidates actually landed, counted from what exists and never from the plan: the PRs delivered under the chosen `chain_strategy`, or the stored review results that carried their own review boundary, one per slice (a second blind launch of one head is a second file, not a second slice). `roadmap-maker/assets/roadmap-template.md` has a tracking row for it, so an unwritten figure leaves that row reading a field nobody fills.
  - The planned side of `realized_slices` is the slice count the chain's shaper/estimator record states; there is no structured field for either side in the actuals record. When the planned count is not stated there, write `planned slice count not recorded`. When the realized count cannot be established, write `realized slice count not recorded`. Never reuse the planned number as though it were the outcome: that is how an overrun disappears into a plan that looks like it held.

### Validate

1. Compute the actuals object **once**: `implementation_hours`, `review_gate_hours`, `total_wall_clock_hours` when resolved, `post_review_fix_hours`, `approval_decision`, `scope_drift_notes`, `variance_vs_plan`, plus the optional complexity-unit fields `requirement_count`, `changed_lines`, `review_lens_count` and `checkpoint_count` when their source artifact is readable. Those four are recorded for reference only (`sdd-time-estimation` no longer derives a per-unit calibration rate from them): omit any whose source artifact could not be read rather than guessing.
2. Validate it with the **deterministic validator**, never by reading it:

   ```bash
   # -C changes the process working directory, so both paths are absolute. Each
   # tool under tools/ is its own Go module; there is no repository-root module.
   root="$(git rev-parse --show-toplevel)"
   go run -C "$root/tools/actuals-record-validator" . \
     --schema "$root/skills/_shared/actuals-record.schema.json" \
     --instance /absolute/path/to/actuals-candidate.json
   ```

3. Exit `0` is the only result that authorises the write. Any non-zero exit is a hard stop, and so is a missing or unbuildable validator, which is not a pass: report it and write nothing. Reading the JSON, checking that the fields look present, or attesting that you were careful is NOT validation; the record's author cannot be its validator.
4. Beyond schema shape the validator enforces: `approval_decision` inside the closed outcome vocabulary; non-negative hours and counts; the durable-vs-reconstructed disclosure a recorded `checkpoint_count` owes; the measured-vs-reconstructed disclosure a recorded `total_wall_clock_hours` owes; and that `scope_drift_notes` and `variance_vs_plan` carry their own content instead of deferring it to some other section. That last rule exists because it was actually broken: "see the narrative below — this field is summarised there rather than duplicated" satisfies every required-field check JSON Schema can make while recording nothing. Cross-reference in ADDITION to the content, never INSTEAD of it.
5. `total_wall_clock_hours` is validly absent when its anchors do not resolve (R-021), and `implementation_hours`, `review_gate_hours`, and `post_review_fix_hours` are validly absent while no durable source exists for them (R-019) — every other resolved field is still required. If validation fails for any other reason, report and STOP — do not partial-write.
6. The three compute-time fields stay unpopulated (R-019); state why, do not leave it silent. No durable source exists for any of them yet: orchestrator telemetry is session-transient, the `sdd-attempt` ledger has no timestamp fields, and transcripts carry no structured subagent durations. None of the three MAY be filled with `total_wall_clock_hours` or any other elapsed-time proxy while this remains true.

### Execute

Write items 1-3 in order. If one of them fails after another succeeded, report the inconsistency and STOP; never leave the stores silently out of sync.

1. `sdd/{change}/actuals` (the authoritative single actuals notebook).
2. `project/{project}/estimation-calibration` (append/update — variance patterns; no skill reads it since `sdd-time-estimation` was rewritten).
3. Append a `## Cycle Timestamps` section to the archived `archive-report.md` the delivery prompt named (confirmed in Plan) — a named, delimited, append-only carve-out to closure-feedback's own "do NOT modify hop outputs" rule (the report's other bytes untouched): a 2-row table with t0 (topic key, observation id, `created_at`, primary-or-fallback source) and t1 (`landing_commit`, `approved_tree` when recorded, committer timestamp, and the verified/self-asserted/rejected outcome).
4. After items 1-3 succeed, fire the archive-time sync trigger from the project root:

   ```bash
   labdrian longterm-mem sync-trigger --event archive --cwd "$root" || true
   ```

   It is best-effort and informational: it runs only after items 1-3 succeed, and a failed, skipped or missing sync never blocks or reverses closure. The command already exits 0 whatever the sync does (the `sync-trigger` branch of `cmd_longterm_mem` in `bin/lib/longterm_mem.sh`); the trailing `|| true` is a redundant safety net, kept so closure stays non-blocking even if that guarantee regresses.

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
