# Role agents — v3 slice 3: estimation skill rewrite (Phase 9, PR 3 of `docs/architecture/role-design/per-role-design-v3.md` §6)

Objective: rewrite `skills/sdd-time-estimation/SKILL.md` in place per E §4 and retire the actuals requirements and pins bound to the old body, per the owner's OD2 decision. Branch `feat/phase9-estimator` from `main` (`64e121db`). Delivery: `ask-on-risk`, one PR (forecast ~400-600 authored changed lines, most of them deletions of retired text and pins). Runners: `cd engine && go test ./...`, `cd tools/entry-contract-validator && go test ./...`, CI-form `skills validate`, skills lint, structural readback for prose.

## Specs

E = `docs/architecture/role-design/per-role-estimation-skill.md`. Read v3 (D6, §6), E in full, and `odd/tasks/role-agents-slice2.md` (approval pattern) before any edit.

- **S1** — v3 §6 row 3, verbatim: "`sdd-time-estimation` rewritten in place (E §4); actuals spec lines and the three pinned tests (`actuals_instrumentation_contract_test.go`, `pipeline_vocabulary_pins_test.go`) retargeted per E OD2". Risk named there: "item 3 (spec wording others cite), item 6 if the pins are deleted instead of retargeted". v3 D6, verbatim: "Estimator = skill `sdd-time-estimation`, rewritten in place within the lint budget (E §4 skeleton), loaded by shaper/builder/prototyper".
- **S2** — The body is E §4's skeleton (E:43-77): frontmatter, Activation Contract, Hard Rules, Decision Gates (prototyper/shaper/builder rows), Execution Steps, Output Contract, References. Keep the directory and the name `sdd-time-estimation` (E:18). The description must be ≤160 runes (`lint.go:313-331`). The lint must pass; E says "measure with the lint before approval". `references/local-docs.md` points at docs that do not exist here (E:7), and the skeleton's References drop it. Assumption: remove the file and its `overlay.manifest` row together.
- **S3** — OD2, owner decision 2026-10-22: "Retirarlos (Recomendado)". The chosen option reads, verbatim: "Se borran de la spec R-009 a R-012 y sus subtests, dejando una nota de retiro. R-001/R-002 (tres unidades nunca mezcladas) se mantiene y su test pasa a fijar el texto nuevo. E §1 ya los había marcado DROP: R-019 congeló los compute-hours y no hay datos que calibrar. Hay que aclarar en inception-pipeline y pre-sdd-contracts que el estimate ya no se guarda en sdd/{change}/estimate. Es riesgo item 6, porque se pierden pins." The question named the slices example among the retired text, so `TestReviewSlicesWorkedExampleReproduces` is retired with them.
- **S4** — Dangling prose. Every statement that the estimation skill writes `sdd/{change}/estimate`, carries the compute-hour model, or feeds calibration is corrected to the new truth, minimally. This covers:
  - `skills/_shared/pre-sdd-contracts.md:5,41,72,80`
  - `skills/inception-pipeline/SKILL.md:15,50-62,73,172,188,209` and `references/tier-selection.md:20,32,54`
  - `skills/_shared/entry-contract.schema.json:119`
  - `skills/_shared/actuals-record.schema.json:5,54`
  - `skills/roadmap-maker/assets/roadmap-template.md:44-45`

  The closure-feedback actuals writer keeps working when no estimate record exists.
- **S5** — Approvals. Any byte change to a global skill needs `labdrian skills approve`; `sdd-time-estimation` is grandfathered today (`engine/skills/approval_baseline.go:53`). The writer never creates `.approval.json` and never runs `skills approve`. It reports each changed skill's id and sha256 for the owner.
- **S6** — Scope fence:
  - No engine production code change; E §4 says "zero engine change".
  - No generator (PR 4), no agent deletions (PR 5), no CI ratchets (PR 6).
  - No new handoff field.
  - The skill name and its installer pins stay.
  - The actuals schema's data shape stays; only descriptions change.

## Tasks

| ID | Specs | Route | Status | Commit / evidence |
|---|---|---|---|---|
| T1 | S1-S6 | delegated (owner: "sonnet max para implementaciones"), workflow implement → verify (opus high); B1 fixed inline (L5) | done | wf_adb52e0d-1f4, commit L6 |

## Log

- **L1** (2026-10-21, owner): "bueno cotinua en ese orden ve en ultracode con opus high para las verify y sonnet max para implementaciones".
- **L2** (2026-10-22, owner): after #666 merged, asked to remove the worktree and start PR 3. Owner: "si dale".
- **L3** (2026-10-22): read-only map. OD2 was open: v3 D14 resolved v3 §8, not E §8.
  - Today `TestActualsInstrumentationContract` (5 subtests: R009, R019, R010_R011, R001_R002, R012) and `TestReviewSlicesWorkedExampleReproduces` pin text that exists only in the old body.
  - `inception-pipeline` closure-feedback writes the actuals and holds none of that text, and v3 never deletes it.
  - `sdd/{change}/estimate` is written only by the old body.
  - Owner chose "Retirarlos (Recomendado)" (S3).
- **L4** (2026-10-22, workflow wf_adb52e0d-1f4): the Sonnet writer delivered S1-S6 (13 files, +64/-319).
  - RED: the R001_R002 pin against the old body. GREEN after the rewrite.
  - Engine binaries are byte-identical to base. `manifest routes` lost only the `local-docs.md` row. Schema data shapes are unchanged.
  - Two Opus verifiers passed S1, S2, S3, S5 and S6 and gave S4 `partial`, with the same owner-only blocker B1: the entry contract requires `estimate.planned_range_hours` and `complexity`, and after the rewrite nothing produces them.
  - Owner, asked how to close B1: "Fila inception (Recomendado)".
- **L5** (2026-10-22, inline fix of B1):
  - `sdd-time-estimation` Decision Gates gains an `inception` row: `planned_range_hours` = shaper minutes/60, `no-actuals n=0`, low; `complexity` by pieces (1 low, 2 medium, split high).
  - `inception-pipeline` delegates to that row and never estimates the cache itself. `pre-sdd-contracts` names the row as the source.
  - New pin `inception_row_fills_the_entry_estimate_cache`: RED, then GREEN.
  - Verifier advisory M5 closed: the R001_R002 pin now also requires "never a calendar ETA".
  - Wording trimmed to keep the lint at exit 0 with no advisory.
  - `go test ./skills/` fails only the two approval-gated tests. The entry-contract-validator tests pass.
  - Owner approvals due: `sdd-time-estimation` `35a02131656bbe8fa037cd5990c01bf044a793953d2a86fc6427b199d0a1d1c3`, `inception-pipeline` `6e244eff560b86988b01f7114ac396c1bc676c8110e160880377fab3a69244dd`.
  - `openspec/project/roadmap.md` "next producer: `sdd-time-estimation`" stays true with the row, so it is left unchanged.
- **L6** (2026-10-22): owner approved both skills (`approved: sdd-time-estimation`, `approved: inception-pipeline`, digests as in L5). CI-form `skills validate` rc=0 (40 skills, 9 approved). `go test ./skills/ ./installer/ ./sddregistry/` ok. Committed as the feature's single work unit.
- **Carried advisories:**
  - `estimation-calibration` and `interruption_hours` are still written with no reader. Dropping them is the owner's call.
  - The R-016/R-017/R-018 traces to retired ids are annotated.
  - `inception-pipeline` remains over the lint budget, as it already was at base.
  - The skeleton's "(2026-10-21)" as-of date was dropped to fit the budget; the References line says to update the figures when a ledger closes.
  - Already-deployed `local-docs.md` copies stay as orphans until the installer prunes them.
