---
name: sdd-time-estimation
description: "Use this skill when the user or orchestrator asks how long a change will take, wants an effort or complexity estimate before starting SDD execution, or says 'cuánto tarda', 'estimá el esfuerzo', 'time estimate', 'effort estimate', 'how long will this take'. Produces a pre-start planning report with hour ranges, confidence level, contingency buffer, and human-review time included. Also use to calibrate future estimates from actuals written by inception-pipeline closure-feedback."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when the user or orchestrator asks for time, effort, complexity, delivery window, or implementation scope for a requirement before starting SDD execution. Treat the estimate as a pre-start planning report tied to likely SDD phases.

## Hard Rules

- Inspect only the relevant workspace and requirement context before estimating.
- If SDD artifacts already exist, use them in this order: proposal, spec, design, tasks. If they do not exist, estimate from the requirement and state assumptions.
- Be conservative. Do not return optimistic numbers when scope, integrations, data impact, or production risk are unclear.
- Give ranges, never a single exact duration.
- Ask questions only when missing information blocks estimation; otherwise continue with explicit assumptions.
- Include a contingency buffer whenever there is cross-system work, unclear rules, sensitive data, migration risk, or external dependency risk.
- Track estimation accuracy after work closes: compare planned estimate, SDD execution effort, human review effort, post-review fixes, and final approval/cerrado time.
- Treat "actual time" as the full process time until human review approval, not only the first SDD implementation/verification pass.
- **CALIBRATION (pre-start, read-only):** At pre-start, READ `project/{project}/estimation-calibration` if present (writer: `inception-pipeline` closure-feedback — see `../_shared/pre-sdd-contracts.md`). Use the recorded variance patterns to sharpen the estimate: adjust ranges and confidence to past misses. Never hide misses — if the project consistently overruns, widen the range instead of anchoring low.
- **PERSISTENCE (mandatory, pre-start only):** After producing the estimation report and before implementation starts, call `mem_save` with topic key `sdd/{change}/estimate` and `capture_prompt: false`. The saved record must carry: `planned_range_hours` (low/high), `complexity`, `confidence`, and a note that human-review and gate time is included in the planned range. Use the `change` slug inherited from the requirements/entry chain — never re-derive it. See `../_shared/pre-sdd-contracts.md` for topic-key authority.

## Decision Gates

| Situation | Action |
|---|---|
| Clear, simple requirement | Estimate directly with low buffer and high-confidence assumptions. |
| Unknown systems, rules, or data | Estimate with stated assumptions and medium buffer. |
| Cross-system or production-impacting change | Use high buffer, highlight dependencies, raise risks early. |
| Ambiguous requirement | Ask the minimum blocking questions and also provide a provisional estimate. |
| Missing requirement or SDD input | Request the requirement or available SDD artifact summary first. |
| Completed SDD with human review | Record actual effort, review/fix time, approval time, variance, and lessons for future estimates. |

## Execution Steps

1. Classify the request: new feature, change request, bug fix, validation rule, integration, reporting change, data change, documentation update, or process improvement.
2. Read the smallest relevant project surface needed to size impact: target workspace, touched modules, likely integrations, existing tests, and deployment/runtime constraints.
3. Define the probable SDD path: discovery/proposal, spec/design, tasks, implementation, verification, rollout.
4. Break work into estimate components: functional analysis, technical analysis, development/configuration, unit testing, integrated testing, UAT support, documentation update, deployment/support.
5. Estimate each component in hour ranges; then add a contingency buffer based on ambiguity, coupling, data sensitivity, and production risk.
6. Rate complexity as Low, Medium, High, or Critical. Rate confidence as Low, Medium, or High.
7. If the work is too large for a safe single delivery, recommend SDD slicing or chained PRs and provide phase-level time guidance.
8. Before implementation starts, return the estimation report and **persist the pre-start estimate** via `mem_save` to topic key `sdd/{change}/estimate` with `capture_prompt: false`. Include `planned_range_hours` (low/high), `complexity`, `confidence`, and a note that human-review and gate time is already accounted for in the range. Use the `change` slug inherited from the requirements/entry chain.
9. After each SDD completes, update the estimate record with implementation/verification effort, review findings, fix effort, approval/cierre time, variance from plan, and calibration notes.
10. Use prior actuals and variance patterns to sharpen future pre-start estimates; never hide misses.

## Output Contract

Return:
1. Summary of the requirement
2. Scope interpretation
3. Estimation table with Activity, Estimated hours, Notes
4. Total estimated range
5. Suggested delivery window
6. Complexity
7. Confidence level
8. Assumptions
9. Dependencies
10. Risks
11. Open questions, only if needed
12. Final recommendation, including whether to start with SDD discovery/planning and the suggested SDD slices with estimated timing
13. For completed work: Actuals and Calibration section with planned vs actual range, implementation time, review time, post-review fix time, approval/cierre time, variance, and lessons learned

## References

- `references/local-docs.md`
