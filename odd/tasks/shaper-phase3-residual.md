# ODD Task — Shaper Phase 3 residual (handoff v3)

> **Roadmap authority:** [Standalone Platform Master Roadmap](../../openspec/decisions/standalone-platform-roadmap.md), canonical **Phase 3 — Shaper handoff**. Phase 3 is recorded as *verified for the approved core*; this feature closes its residual outcome so Phase 4 can rely on the full bounded plan. Prior ledger: `odd/tasks/shaper-handoff-contract.md`.

## Objective

Complete the Phase 3 outcome: a bounded plan that carries architecture, stages, **roles, tests**, acceptance, **risks, estimates, memory scope, and delivery limits**, and rejects incomplete, **contradictory**, or out-of-scope handoffs.

## Approved design (user, 2026-09-26)

Handoff **version 3** is version 2 plus six fields. Versions 1 and 2 are unchanged.

| Field | Shape | Rule |
|---|---|---|
| `roles` | `[{role, responsibility}]` | At least 1. Free-form, non-blank, no duplicate `role`. Phase 4 types the vocabulary later. |
| `tests` | `[string]` | At least 1 non-blank planned test or check beyond acceptance. |
| `risks` | `[{risk, mitigation}]` | Present, may be `[]`; each entry non-blank. |
| `estimates` | `[{stage, low_minutes, high_minutes}]` | Exactly one per stage, referencing stages by exact text; positive integer agent-minutes with `low <= high`. Agent effort to execute Goal stages only, excluding plan preparation and human labor; no calendar ETA. |
| `memory_scope` | string | Non-blank. |
| `delivery_limit` | string | Non-blank. |

**Deterministic contradiction and scope rejection (v3 only):** estimates missing, duplicated, or referencing an unknown stage; `low > high`; non-positive minutes; duplicate roles; and **OD6**: any stage, criterion, test, or role responsibility byte-identical to a Goal `non_goals` item is rejected (v1/v2 keep the human-review flag). Semantic contradictions remain human-review flags. `memory_scope` and `delivery_limit` are shown next to the Goal's `memory_scope` and `delivery_boundary` in the presented view for human judgment.

## Scope and constraints

- Branch `feat/shaper-handoff-v3` from `main` (`c852f0e`), worktree `~/labdrian-sdd-overlay-shaper`.
- In scope: `engine/shaper` parsing, validation, readiness, presented view and view safety, disclosures; `engine/cmd/shaper.go` only if output must change; README CLI reference.
- Out of scope: Phase 4 role typing, executing planned checks, calendar ETA, persistent catalogs, new runtime adapters.
- Trust model unchanged: digests plus deny guards, not a signature.
- TDD: strict (always-on project instruction). Runner: `go test` from `engine/`. Checks: `gofmt -l`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, and staticcheck (CI runs `staticcheck@v0.7.0`).
- RDD: on. Each work-unit commit is assessed; due reviews run natively with user consent.
- Delivery: `ask-on-risk`, `stacked-to-main`; issue per slice via `feature.yml`.

## Tasks

- [x] V1 — Handoff v3 parsing and validation: the six fields, strict nested keys, and structural contradiction rejection. *Route: delegated writer.* **Evidence:** commit `b3434cd` after rebasing onto `main` `afc718c` (originally `aeafd64`; 602 lines). RED was a compile failure; staticcheck is clean. Native review `review-8072c21fb3d2f9ae` **approved** with no correction and was acknowledged. Informational: `R3-estimate-duplicate-stage-assumption` (estimates keyed by stage text are ambiguous if two stages share identical text), `R3-acceptance-strictness-untested-on-v3`, `R3-tests-null-item-implicit-handling`.
- [x] V2 — Readiness and presented view for v3, with OD6 as a refused-kind rejection `goal_non_goal_overlap`, the six fields rendered beside the Goal's memory scope and delivery boundary, view safety extended to them, and a v3 ready disclosure stating the plan fields are present. *Route: delegated writer.* **Evidence:** commit `b13af49` plus the README commit `20cb2e5`. Native review `review-e1cfb996d9e0a1de` (four lenses): **CRITICAL `R4-doc-enforcement-gap`**. The README claimed an out-of-order estimate rejection that does not exist, and the contradiction guarantee was unproven within the candidate. The bounded correction is `968f692` (27 of 40 lines): exact README wording plus `TestEvaluateV3ContradictoryPlanNeverReachesReady`, which was shown load-bearing by disabling the duplicate-role check. The targeted validator **approved** it; acknowledged.
- [x] V3 — README and CLI reference (`20cb2e5`, `968f692`), native reviews, and delivery: issues #421–#422 and stacked pull requests #423 (v3-parse, 602 lines) and #424 (v3-readiness), both awaiting protected labels and merge after live confirmation.
- [x] V4 — Live confirmation (2026-09-26, real human input). The user authorized installing the v3 engine; the previous binary is backed up in `~/.labdrian-shaper-v4-backup-20260926T040354`. In `~/labdrian-shaper-s8` (fixture commit `844dd24`, `handoff-v3.json`), assessment before clearance was exit 3, `draft`, with only `clearance_missing`, and the v2 fixture stayed `ready`. The user ran `/shaper-clear --handoff handoff-v3.json --goal goal.json` in the Pi TUI and affirmed after reading the full plan. Assessment after: **exit 0, `ready`, clearance `verified`**. A one-byte edit (`.` to `!` in a role responsibility) gave exit 3, `draft`; restoring gave exit 0, `ready`.
  - **Defect found in the live output and fixed:** the shared forgery disclosure said the Phase 3 plan outcome was unmet, contradicting the v3 ready disclosure. Commit `0179c58` keeps that statement only in the v2 ready disclosure, in Go and in the Pi gate. RED: the v3 disclosure test failed on "not yet met". A superseded assertion that the shared disclosure mention Phase 3 moved to the v2 ready disclosure. Native review (high, four lenses) in progress.

## Acceptance criteria

- A complete v3 handoff with a verified clearance reaches `ready` and discloses no missing plan fields.
- Each missing or contradictory v3 field keeps the handoff invalid or draft with a named reason.
- v1 and v2 behavior and existing tests are unchanged.

## Progress and evidence

(none yet)

## Next step

Phase 3 is closed. PRs #423 and #424 merged to `main` (`5d9cb1d`) after `status:approved` on #421–#422 and `size:exception` on #423–#424 (user-authorized, applied with read-back). The primary checkout was fast-forwarded to `main`; two untracked `engine/workflowprofile` files identical to `main` were moved to the untracked backup first. The live engine and Pi gate were redeployed from `5d9cb1d`, with backups in `~/.labdrian-shaper-final-backup-*`. The roadmap marks Phase 3 **verified** for the full outcome. Next: Phase 4, reusable roles.
