# Estimation skill for the role team — exploration and minimal design (DRAFT for owner approval, 2026-10-21)

Method: audit the existing skill and every repo artifact that forecasts or records size/effort; keep only elements with a cited need; validate every tooling claim against this checkout (`memory/research-prose-vs-installed-source.md`). Contracts applied: `skills/_shared/minimalism-contract.md:12-26` (ladder; seams never collapsed). Fixed: estimator is a skill, not an agent; the 8-role vocabulary and `RoleHandoff` (`engine/roles/roles.go:17-26`, `handoff.go:57-78`) stay. Paths are repository-relative; CLAUDE.md/AGENTS.md are cited by section.

## 1. Audit of `skills/sdd-time-estimation/SKILL.md` (Q1)

Size: 24,801 bytes, ≈6,100 estimated body tokens by the lint proxy (bytes/4, `engine/skills/lint.go:42-47,70-72`) against a hard 1,000 and recommended 700: six times the hard limit. It is grandfathered on the approval baseline (`engine/skills/approval_baseline.go:11-13,53`); the owner decided on 2026-09-30 that the SDD/ODD skills are rewritten within the lint budget for the new team, each leaving the baseline with clean lint and an approval record (`odd/tasks/skill-lifecycle.md:95`). Origin: upstream import (`author: gentleman-programming`, `version: "1.3"`, `:6-7`); `references/local-docs.md` (282 bytes) points at `docs/bitbucket-cli.md` and `apps/backend/docs/tecnico/tablas/detalle/rpa.md`, neither of which exists here (`rg --files`: no match). No row of `.atl/skill-registry.md` names it (`rg estimation`: 0 matches).

| Element (lines) | Verdict | Evidence |
|---|---|---|
| Three units never blended (`:18`); ranges never a point (`:23`); `n=0` never blocks (`:24`); confidence scales with `n` (`:37`); widen after a miss, never anchor low (`:40`); above one slice: estimate the slices, not the change (`:66`) | KEEP, ≈6 lines | each is either an actuals-spec rule (`openspec/specs/actuals-instrumentation/spec.md:5`) or matches an observed miss (§3) |
| Compute-hour calibration, round-trip latency, interruption allowance, the `n=2` ceiling and R-011 selection-bias essays (`:29-39`, ≈1,100 words) | DROP | R-019 froze the three compute fields (`spec.md:215-217`); the two records on disk are the whole sample (§3) |
| Bootstrap hour defaults (`:41`); human-equivalent hours (`:21`, output 3b) | DROP | no source for the hours; the handoff excludes human labor by owner decision (`odd/tasks/shaper-handoff-contract.md:38,220`) |
| PERSISTENCE to `sdd/{change}/estimate` and plan-vs-actual pairs (`:42-46`); checkpoint count and delivery window (`:68`, `:85`) | DROP from this skill | writer is `inception-pipeline` closure-feedback (`skills/_shared/pre-sdd-contracts.md:72,80`), not a role; the handoff estimate "is not a calendar ETA" (`engine/shaper/handoff_v3.go:33`); the entry contract keeps `expected_checkpoints` on its own (`skills/_shared/entry-contract.schema.json:116`) |
| Five changed-line buckets (`:66`) | CHANGE | replaced by the two thresholds the owner already runs on: ≤~100 lines inline, ~400 per slice (CLAUDE.md "Writer trigger", ODD step 6; `skills/sdd-tasks/SKILL.md:146`) |
| 13 execution steps, 14 output items (`:63-93`) | CHANGE | → 5 steps, 1 output line per package (§4) |

Decision: **replace the body in place, keep the directory and name `sdd-time-estimation`.** The name is pinned where a rename would cost delivery work for no function: `engine/sddregistry/sddregistry.go:52` (installer deploy list; `engine/installer/testdata/registry_pins.golden:126`), the approval baseline, and design v2 keeps skill names while renaming agents. New description (≤160 runes, `lint.go:313-331`) names the three roles and "work package". Owner-gated cost: the actuals spec binds requirements to this body's wording (`spec.md:74` cites "line 27", `:100` cites "Output item 14") and three tests pin phrases and worked examples of the current text (`engine/skills/actuals_instrumentation_contract_test.go:16,127-681`, `tools/entry-contract-validator/pipeline_vocabulary_pins_test.go:31,855-869`); a rewrite fails them, so those requirements are retargeted to whatever actuals writer survives the role team, or retired with it (OD2).

## 2. Work package (Q2): reuse, no new vocabulary

A work package is one shaper handoff **stage**: exactly one `{stage, low_minutes, high_minutes}` per stage, positive, `low <= high` (`handoff_v3.go:31-39,244-280`; `odd/tasks/shaper-phase3-residual.md:18`). The builder executes a stage as one ODD **unit** (one behavior with its code, tests, docs; CLAUDE.md "Writer trigger") delivered in **slices** of about 400 authored changed lines, additions plus deletions, generated files excluded (ODD step 6; `sdd-tasks/SKILL.md:146`), where a unit of ≤~100 lines always runs inline. The entry contract already types the size estimate as a range (`review_slices[].estimated_changed_lines`, `entry-contract.schema.json:225,257`; `max_changed_lines_per_slice` `:150`). Two axes, both already recorded in the ledgers: **authored changed lines** (size) and **independently reasoned pieces** (the cause of the observed overruns, `openspec/changes/archive/2026-08-05-deterministic-verification-evidence/tasks.md:219`). Task Size small/large decides tracking (resume test), not the estimate. A prototyper spike is a package with a box instead of an estimate (§6). Rule carried over from `:66`: a package whose high bound exceeds one slice is split, not estimated.

## 3. Calibration data (Q3): what is real

| Measure | Source | n | Status |
|---|---|---|---|
| Authored changed lines, forecast vs `git diff --stat` | `odd/tasks/*.md` forecasts and actuals; SDD ledgers; actuals records | 7 features, 6 units | **real**, reproducible |
| Slices planned vs delivered | longterm-mem 18 → 82 PRs (`SKILL.md:45`, `.../2026-09-02-longterm-mem/entry.json`); skill-lifecycle 6-8 → 11 (`odd/tasks/skill-lifecycle.md:106`) | 2 | real |
| Ledger-counted lines vs authored | 292 authored settled as 440 (`memory/attempt-ledger-counts-sdd-artifacts.md`); +4-18 lines per objective of bookkeeping (`tasks.md:206`) | 14 | real; counts `tasks.md`/progress files |
| Tokens, cache share, recorded/estimated cost per model, agent type, project, day | `telemetry report` (`docs/architecture/adr-telemetry-cost-baseline.md`; `telemetry/readers.go:19-23`) | whole corpus (84,977 Claude calls) | real, reproducible; **not per work package** (D1 open, `odd/tasks/telemetry-cost-baseline.md:16,37`); no durations: `Call` keeps `Day` only (`readers.go:20,71-72`) |
| Checkpoints planned vs actual | 4 → 7; 12 with 10 of 12 reconstructed (`engine/skills/testdata/corrected-actuals-*.json`) | 2 | weak |
| Wall-clock hours | 6.64 measured; 36 reconstructed from narrative | 2 | weak |
| Agent compute hours | 1.39; ≈1.4 | 2, frozen by R-019 | dead |
| Agent minutes per stage (`low_minutes`/`high_minutes`) | no record anywhere in the repo or Engram pins | 0 | **invented unit**: a seam with no source |

Reference class, authored lines, actual ÷ forecast (measured 2026-10-21): features: standalone-goal-contract 641/250 = 2.6 (`odd/tasks/standalone-goal-contract.md:33`); runtime-adapters 16,400/4,200 = 3.9, 11,900 of them tests (`odd/tasks/runtime-adapters.md:41,131`); skill-lifecycle 2.6-3.9 by the ledger's own figure, "Phase 7 four times" (`skill-lifecycle.md:36,106`); obs #2687→#2767 forecast 1,050-1,750, landed +5,767/−113 ≈ 4.2 (`corrected-actuals-skills-validate-ondisk-gate.json:14`); sync-check 950 vs ~430-470 ≈ 2.1 (`corrected-actuals-sync-check-repo-behind-origin.json:11`); skills-validate-ondisk-gate 511 vs 520-975 ≈ 0.7 after an upward bias correction (`...ondisk-gate.json:14`); telemetry T1 520/400 = 1.3 (`telemetry-cost-baseline.md:45`). Seven features: **0.7× to 4.2×, five above 2×**; the excess is tests (≈70% of lines in two ledgers) and review follow-ups. Units under the 200-line attempt cap: 66/70, 146/120, 134/130, 93/110, 193/160, 163/135 of the high bound = **0.85-1.22×**, over only when test/fixture-heavy or carrying five pieces (`tasks.md:166,219,263`). Verdict: lines can be calibrated coarsely at both levels (small n, no percentiles); minutes cannot be calibrated at all. Honest minimum for minutes: a wide range, `basis: no-actuals n=0`, confidence low, stated beside the number. The only candidate durable source is telemetry: Claude records carry per-line timestamps (`readers.go:44,71`), so first→last per subagent file would give per-call durations; not built, not this skill (OD3). Caution from `research-prose-vs-installed-source.md` applies: no number above is a percentile or a rate; they are the whole population.

## 4. One body, three entry points (Q4)

Mechanism, zero engine change: one `SKILL.md`; a `Decision Gates` table with one row per role; the parent passes the path under `## Skills to load before work` (orchestrator "Skill Registry Protocol") from a registry row whose Trigger names `shaper`, `builder`, `prototyper`. Gate injection would honour `applies_to_phases` (`engine/gate/gate.go:127-142`, `engine/contract/contract.go:38-47`) but `gate_task.go:131` carries exactly one `--contract-file` today; adding a list is engine work with no observed parent omission behind it: deferred. Body skeleton (≈2.6 KB ≈ 650 estimated tokens; measure with the lint before approval; H2 order per `lint.go:280-287`):

```markdown
---
name: sdd-time-estimation
description: "Size a work package before committing: prototyper time-box, shaper per-stage ranges, builder slice sizing. Ranges with a stated basis, never a point."
license: Apache-2.0
metadata:
  author: labdrian
  version: "2.0"
---
## Activation Contract
Load before a prototyper spike, a shaper handoff (`estimates[]`), or a builder unit. One body; read your row in Decision Gates.

## Hard Rules
- Ranges only. Every range carries its basis (`lines-ref n=<k>` or `no-actuals n=0`) and a confidence (low|medium|high). `n=0` never blocks: estimate and flag it low.
- Size in authored changed lines first (additions + deletions, tests included, generated excluded). Minutes derive from lines last; minutes have no actuals (`n=0`): keep them wide and say so.
- Reference class, this repository (2026-10-21): features landed 0.7-4.2x their line forecast, five of seven above 2x (tests and review follow-ups); units under 200 lines landed 0.85-1.22x their high bound, over only when test- or fixture-heavy. Start from the class, then adjust for pieces; never from how fast the work feels.
- A package whose high bound exceeds one slice (~400 lines) or carries more than two independently reasoned pieces is split, not estimated.
- A miss above the high bound widens the next range; never anchor low. The attempt ledger counts bookkeeping files: add headroom.

## Decision Gates
| Role | When | Output |
|---|---|---|
| prototyper | before a spike | a box per unknown (lookups or spikes, e.g. 5); at the box stop and report `resolved` or `open` plus what would resolve it; no estimate |
| shaper | per handoff stage | pieces, lines range, `low_minutes`/`high_minutes`; an open unknown becomes a `risks[]` item with its mitigation and widens that stage's high |
| builder | per unit, before writing | lines range and pieces; <= ~100 lines: inline; > ~400 or > 2 pieces: slice; re-forecast at each slice boundary against `git diff --stat` |

## Execution Steps
1. List the pieces (behaviors, files, tests, fixtures, docs). 2. Place the package in the reference class; give a lines range. 3. Split what exceeds a slice. 4. Shaper only: derive minutes, wide, `n=0`. 5. Write basis and confidence beside every range.

## Output Contract
Per package: `pieces`, `lines low-high`, `basis`, `confidence`. Shaper adds `low_minutes`/`high_minutes` per stage; prototyper adds `box` and `unknowns[]` as `resolved|open`.

## References
- `odd/tasks/*.md` forecasts and `openspec/changes/archive/*/tasks.md`: the reference class; update the figures when a ledger closes.
```

Role differences without copies: the prototyper's box reuses the owner's own budget vocabulary, counted in lookups because a sub-agent can count its tool calls but not its wall time (CLAUDE.md "Evidence budget rule": 3 calls ≈10k tokens; "Mapping trigger": ≈5 sequential lookups); the evidence that exploration needs a box is that `general-purpose` sub-agents hold 38.8% of all context tokens (`telemetry-cost-baseline.md:48`; ADR context) and `skills/sdd-explore/SKILL.md` has no stop rule (`rg -i "time|budget|bound"`: only the orchestrator-branch line 18). The shaper's row fills the seam fields exactly (`handoff_v3.go:244-280`) and puts uncertainty in the existing `risks[]`. The builder's row is the existing Review Workload Forecast (`sdd-tasks/SKILL.md:144-171`, enforced by `sdd-apply/SKILL.md:82-106`) reduced to the two ODD thresholds plus the piece count and the ledger headroom. Confidence and basis for the chain ride in the estimator record's `context.summary`/`open_questions` (`handoff.go:47-51`), which the parent appends anyway (§5); no handoff field is added.

## 5. Chain tension (Q5): recommend (a), zero engine change

Dependents of `estimator`: 34 files, all engine data or its tests and docs: `roles.go:20,32,64-69` with `roles_test.go:78-90`; the chain store tests and the on-disk golden chain `engine/roles/filechain/testdata/golden-v1/000002.json:7` (`from_role: estimator`); `engine/cmd/roles*_test.go`, `engine/workflow/lifecycle_test.go:893`; `README.md:516`; `openspec/decisions/standalone-platform-roadmap.md:34` (Phase 4 marked verified with this sequence); `odd/tasks/reusable-roles.md:7,32` (live smoke chain). No agent file, skill, or dispatcher names it (design v2 §2 row "estimator"). (a) The shaper uses the skill; the parent appends `shaper→estimator` and `estimator→builder`, both `completed`, `payload_kind` the handoff artifact name and `payload_sha256` the handoff bytes whose `estimates[]` are the estimator's output, `context.summary` the confidence and basis. The chain only requires passing through `estimator` (`roles.go:113-117,131-151`); `roles append` exists (`engine/cmd/roles.go:3-5`); nothing forbids two consecutive records with one payload digest (`handoff_validate.go:72-80` checks digest shape only; `chain.go` has no payload rule: inferred from absence). Cost: two appends per goal instead of one. (b) Making `estimator` optional (`roles.go:114-117` + tests + README + roadmap row) or removing it changes a verified seam (high-risk item 3), invalidates stored chains and the golden fixture (item 1) and reopens the Phase 4 status; no failure demands it. If the extra append is ever observed as noise, optional-estimator is the smallest seam change and stays owner-gated (OD1).

## 6. External practice (Q6): only what changed the design

- Shape Up, ch. 3 "Set Boundaries" (basecamp.com/shapeup/1.2-chapter-03, fetched 2026-10-21; verified in source): "Estimates start with a design and end with a number. Appetites start with a number and end with a design"; "fixed time, variable scope". → the prototyper gets a box, not an estimate; the shaper's `delivery_limit` and `out_of_scope` are the appetite fields that already exist (`roles.go:60`).
- Flyvbjerg, "From Nobel Prize to Project Management: Getting Risks Right", PMJ 37(3), 2006, arXiv 1302.3642 (abstract verified): inaccuracy comes from optimism bias and strategic misrepresentation; the cure is the outside view, a reference class of comparable projects. → the skill ships this repository's own class (§3) and forbids starting from the inside view.
- METR, 2025-07-10 (metr.org/blog/2025-07-10-early-2025-ai-experienced-os-dev-study/, verified): 16 developers, 246 issues; forecast +24% speedup, post-hoc belief +20%, measured 19% slower. Update 2026-02-24 (metr.org/blog/2026-02-24-uplift-update/, verified): late-2025 data "an unreliable signal" (participation selection); "speedup of −18% (−38% to +9%)" for returning developers. → the 43-point gap between self-forecast and measurement is why every range prints basis and confidence; no absolute speed figure is used.
- arXiv 2603.06276 (2026-03-06, v2 2026-04-08; abstract verified): zero-shot LLMs predict story points, "unitless, project-specific", better than supervised models trained on 80% of the data (4 LLMs, 16 projects). → relative sizing by an LLM is credible; absolute minutes are evidenced nowhere: size in lines and pieces, derive minutes last. Inferred: story points are not agent minutes.
- Not adopted: story points, t-shirt sizes, hour bootstrap tables (no consumer here; the seam wants minutes).

## 7. Considered and cut (rung)

Estimator agent file (1: owner decision; nothing launches one). Compute-hour, latency and interruption calibration (`SKILL.md:29-39`; source frozen by R-019). Bootstrap hours (`:41`; invented). Human-equivalent hours (`:21`; handoff excludes human labor). Checkpoints and delivery window (`:68,85`; not a calendar ETA by seam; entry contract keeps its field). Five line buckets (`:66`; two ODD thresholds exist). `sdd/{change}/estimate` persistence and pairs (`:42-46`; the handoff record already binds the estimate bytes by digest; the writer's survival is OD2). Gate injection (engine change at `gate_task.go:131`; no omission observed; rung 3 registry row instead). A `confidence` field in handoff v5 (item 3; `context.summary` is free). A minutes-from-lines formula (`n=0`; stated instead). Renaming the skill (item 5, installer pins; no gain). Telemetry per-subagent durations (a real need, R-019's missing source, but a telemetry task). `applies_to_phases` frontmatter on the skill (only useful once the gate takes a list).

## 8. Open decisions for the owner

1. **OD1** Accept (a): two parent appends per goal, vocabulary untouched. Alternative: optional `estimator` (seam change, item 3).
2. **OD2** The actuals spec and the three tests pinned to the old body (`spec.md:5,74,100`; `actuals_instrumentation_contract_test.go`; `pipeline_vocabulary_pins_test.go:855`): retarget to the surviving actuals writer, or retire with `inception-pipeline` if the role team replaces it.
3. **OD3** Give minutes a source: telemetry per-subagent durations plus D1 "delivered change"; until then `n=0` stays printed on every minutes range.
4. **OD4** Prototyper box unit: lookups/spikes (recommended; the agent can count them) versus minutes (a sub-agent cannot observe its wall time).

## 9. Not verified

Whether `propagate` scopes a registry row from a non-`_shared` skill's frontmatter. Whether a chain accepts one payload digest in two consecutive records (no rule found; no test proves it). The skeleton's byte count (estimated; the lint decides). Pi session timestamps for durations (Claude's checked, `readers.go:44`; Pi not). Platform honouring of per-role skill loading beyond the prompt path (same gap as design v2 §6). The METR late-2025 figures, which METR itself discounts.
