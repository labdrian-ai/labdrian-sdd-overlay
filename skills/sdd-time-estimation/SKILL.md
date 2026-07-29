---
name: sdd-time-estimation
description: "Use this skill when the user or orchestrator asks how long a change will take, wants an effort or complexity estimate before starting SDD execution, or says 'cuánto tarda', 'estimá el esfuerzo', 'time estimate', 'effort estimate', 'how long will this take'. Produces a pre-start planning report with agent-compute-time ranges, confidence level, contingency buffer, and expected human-checkpoint count included. Also use to calibrate future estimates from actuals written by inception-pipeline closure-feedback."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.2"
---

## Activation Contract

Use this skill when the user or orchestrator asks for time, effort, complexity, delivery window, or implementation scope for a requirement before starting SDD execution. Treat the estimate as a pre-start planning report tied to likely SDD phases.

**Default execution model is agent-orchestrated.** Unless the user states the work will be implemented by a human directly, assume delivery goes through `inception-pipeline`-orchestrated AI sub-agents (`sdd-apply`, `sdd-verify`, review lenses, judgment-day judges). Agent-compute-time and human-implementation-hours are different units running at different rates — never blend them into a single number.

## Hard Rules

- **Three units, independently measured, never blended (R-001/R-002):** agent-compute-time, elapsed-calendar-time, and human-confirmation-checkpoint-count are three separately tracked units. Any report showing two or more of them MUST show them under separate labels — never summed or averaged into one figure.
- Inspect only the relevant workspace and requirement context before estimating.
- If SDD artifacts already exist, use them in this order: proposal, spec, design, tasks. If they do not exist, estimate from the requirement and state assumptions.
- **The headline range is agent-compute-time, not human-implementation-hours.** Build it from per-phase throughput (see Execution Steps) calibrated against this project's own `sdd/*/actuals` records. A human-equivalent-hour figure may still be reported, but only as a secondary complexity/tiering signal — it must never be the number presented as "how long this takes."
- Be conservative within each unit separately: don't return optimistic agent-compute-time, and don't inflate it using human-effort intuition either — that reintroduces the exact bias this skill exists to avoid.
- Give ranges, never a single exact duration.
- Ask questions only when missing information blocks estimation; otherwise continue with explicit assumptions.
- Include a contingency buffer whenever there is cross-system work, unclear rules, sensitive data, migration risk, or external dependency risk.
- Track estimation accuracy after work closes: compare planned estimate, agent-compute-time actually spent, human review effort, post-review fixes, and final approval/cierre time.
- Treat "actual time" as the full process time until human review approval, not only the first SDD implementation/verification pass.
- **CALIBRATION (pre-start, read-only):** At pre-start, READ `project/{project}/estimation-calibration` if present, and `mem_search` for `sdd/*/actuals` records for this project (writer: `inception-pipeline` closure-feedback — see `../_shared/pre-sdd-contracts.md`). From each actuals record's `implementation_hours`, `review_gate_hours`, and `post_review_fix_hours` ONLY, build a per-phase agent-compute-time baseline. `total_wall_clock_hours` is NEVER an input to the agent-compute-time baseline — it measures elapsed calendar time (tiering-go-ahead to archive, including interruption gaps), a distinct unit that must stay separately labeled, never folded into the compute-time sum. As a read-only diagnostic only, `total − sum(phase hours)` (i.e. `total_wall_clock_hours` minus the sum of the three phase-hour fields above) approximates round-trip/interruption overhead per record and feeds the delivery-window latency rate below — this proxy figure is never itself an input to the compute-time baseline. State confidence by sample size: `n=0` → no project baseline, use the stated bootstrap defaults below and flag Low confidence; `n=1-2` → Low/Medium confidence, scale qualitatively by relative complexity (this change vs. the calibration change's known scope); `n>=3` **and** the actuals records carry the optional complexity-unit fields (`requirement_count`, `changed_lines`, `review_lens_count`, `checkpoint_count`) → compute an explicit rate per unit (`implementation_hours / changed_lines`, `implementation_hours / requirement_count`, `review_gate_hours / review_lens_count`) and raise confidence accordingly; if `n>=3` but the complexity-unit fields are missing on older records, fall back to qualitative scaling and note that the rate will sharpen once enough records carry the new fields. `review_lens_count: 0` is a legitimate Low-risk "no lens" outcome, not a missing value — exclude those records from the `review_gate_hours / review_lens_count` division (it would be undefined) and fold their `review_gate_hours` into the verify-only baseline instead. Never hide misses — if a phase consistently overruns its baseline, widen that phase's range instead of anchoring low.
- **Bootstrap defaults (used only when zero project calibration data exists):** these are agent-compute-time, not human hours — proposal/spec/design/tasks (planning phases, mostly single-agent drafting): 0.1-0.4h combined for a Low/Medium complexity change; apply (implementation): 0.2-1.5h depending on changed-line bucket; verify + review gate (including judgment-day rounds): 0.2-1.0h; post-review fixes: 0.05-0.3h. Treat these as placeholders to be replaced by this project's real calibration after the first change closes — never present them with the same confidence as a calibrated rate.
- **PERSISTENCE (mandatory, pre-start only):** After producing the estimation report and before implementation starts, call `mem_save` with topic key `sdd/{change}/estimate` and `capture_prompt: false`. The saved record must carry: `planned_range_hours` (low/high, agent-compute-time basis), `human_equivalent_hours` (low/high, secondary signal), `expected_checkpoints` (count), `complexity`, `confidence`, and the calibration `n` used. Use the `change` slug inherited from the requirements/entry chain — never re-derive it. See `../_shared/pre-sdd-contracts.md` for topic-key authority.

## Decision Gates

| Situation | Action |
|---|---|
| Clear, simple requirement | Estimate directly with low buffer and high-confidence assumptions. |
| Unknown systems, rules, or data | Estimate with stated assumptions and medium buffer. |
| Cross-system or production-impacting change | Use high buffer, highlight dependencies, raise risks early. |
| Ambiguous requirement | Ask the minimum blocking questions and also provide a provisional estimate. |
| Missing requirement or SDD input | Request the requirement or available SDD artifact summary first. |
| No project calibration data yet (`n=0`) | Use the bootstrap defaults, mark confidence Low, and state explicitly that the range will tighten after this change closes. |
| Completed SDD with human review | Record actual agent-compute-time, review/fix time, approval time, variance, and lessons for future estimates. |

## Execution Steps

1. Classify the request: new feature, change request, bug fix, validation rule, integration, reporting change, data change, documentation update, or process improvement.
2. Read the smallest relevant project surface needed to size impact: target workspace, touched modules, likely integrations, existing tests, and deployment/runtime constraints.
3. Define the probable SDD path and which phases will run as delegated sub-agents (proposal, spec, design, tasks, apply, verify, review, archive — the default under `inception-pipeline`) versus any phase a human will do directly.
4. Estimate this change's own complexity units: expected requirement/scenario count, expected changed-line bucket (small <150 / medium 150-450 / large >450), and expected review-lens count (per the Review Lens Selection tiering: 0 / 1 / up to 4).
5. For each agent-executed phase, derive an agent-compute-time range by applying the calibration baseline (from the CALIBRATION rule) scaled to this change's complexity units relative to the calibration source. For any human-executed phase, estimate in human-effort hours instead.
6. Separately estimate the expected number of human-confirmation checkpoints (tiering go-ahead, proposal/product decisions, judgment-day rounds, chained-PR split confirmation, merge authorization). This — not feature complexity — is the actual calendar-time driver for agent-orchestrated work; it does not shrink just because compute-time is small.
7. Add a contingency buffer to the agent-compute-time range for ambiguity, coupling, data sensitivity, production risk, and known session/rate-limit interruption risk (calendar time can exceed compute time when a session resets mid-flight).
8. Rate complexity as Low, Medium, High, or Critical — this still drives tiering, chaining, and review-lens selection, independent of the compute-time-vs-human-hour distinction.
9. Rate confidence as Low, Medium, or High, weighted down when calibration `n` is small.
10. If the work is too large for a safe single delivery, recommend SDD slicing or chained PRs and provide phase-level agent-compute-time guidance.
11. Before implementation starts, return the estimation report and **persist the pre-start estimate** per the PERSISTENCE rule.
12. After each SDD completes, update the estimate record with actual agent-compute-time per phase, review findings, fix effort, approval/cierre time, checkpoint count actually hit, variance from plan, and calibration notes — this is what sharpens the next project baseline.
13. Use prior actuals and variance patterns to sharpen future pre-start estimates; never hide misses.

## Output Contract

Return:
1. Summary of the requirement
2. Scope interpretation
3. Estimation table with two sections: (a) **Agent-compute-time per phase** (primary/headline, with the calibration `n` behind each figure), (b) Human-effort-hour equivalent (secondary, complexity/tiering signal only)
4. Total estimated agent-compute-time range (headline)
5. Expected human-confirmation checkpoint count
6. Suggested delivery window — derived from a declared formula: expected checkpoints × round-trip latency + interruption allowance, with every input disclosed. Until calibration reaches n≥3 actuals records, the interruption allowance MUST be a fixed, explicitly-disclosed buffer marked "uncalibrated" and does not scale with checkpoint count. The round-trip-latency rate cites its calibration n: WHEN n=0, use a stated bootstrap default marked Low confidence; WHEN n≥1, cite the calibration sample and disclosed n — never present it as a calibrated figure.
7. Complexity
8. Confidence level, with the calibration sample size disclosed
9. Assumptions
10. Dependencies
11. Risks
12. Open questions, only if needed
13. Final recommendation, including whether to start with SDD discovery/planning and the suggested SDD slices with estimated timing
14. For completed work: Actuals and Calibration section reporting agent-compute-time, elapsed-calendar-time, and checkpoint count under separate labels, never blended — planned vs actual agent-compute-time per phase, review time, post-review fix time, approval/cierre time, elapsed-calendar-time, checkpoints actually hit, variance, and lessons learned

## References

- `references/local-docs.md`
