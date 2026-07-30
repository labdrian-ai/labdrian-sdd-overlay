# Tasks: Actuals Calendar-Time / Checkpoint-Count Instrumentation Fix

## Post-Archive Amendment (2026-07-30)

**Added after archive.** The task text below is preserved **verbatim** as the historical record of what was instructed at the time — it was **not** edited, reworded, or renumbered. Five of its lines prescribe a contract that was superseded before delivery; they are corrected in this note rather than in place.

Line numbers below refer to this file **as archived**, before this note was inserted. This note added 21 lines, so each cited line now appears 21 lines lower.

| Archived line | What it prescribes | Final shipped contract |
|---|---|---|
| 37 | Group 3 pins the CALIBRATION proxy formula as `total − sum(phase hours)`. | The shipped residual is per-checkpoint: `(total_wall_clock_hours − sum(phase hours)) / checkpoint_count`. |
| 40 | Group 6 asserts `checkpoint_count == 11` and a `"= 11"` itemization marker. | `checkpoint_count` is **12**; the shipped markers are `"= 12"`, `"Durable floor: 2 of 12"`, and `"AMB-001"`. |
| 46 | The fixture task prescribes `checkpoint_count: 11`, an itemization summing to 11, and a `1 durable, 10 reconstructed` split. | **12** total — **2 durable** (tiering go-ahead + AMB-001 ambiguity clarifying question, both durably observed via `pipeline-state`) + **10 reconstructed** from the closure narrative. |
| 56 | Task 3.2 prescribes the same combined proxy formula `total − sum(phase hours)`. | Same correction as line 37: the shipped formula divides the residual by `checkpoint_count`. |
| 57 | Task 3.3 gates the fixed, uncalibrated interruption buffer on being "below n=3". | No three-sample gate exists anywhere in the estimator skill. The allowance stays separate, fixed, and explicitly marked uncalibrated **until independent interruption evidence exists**. |

The authoritative contract now lives in:

- `openspec/specs/actuals-instrumentation/spec.md` — requirements and scenarios (R-010/R-011 for the interruption allowance, R-014 for the 12 total / 2 durable / 10 reconstructed arithmetic).
- `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json` — the corrected record's exact bytes.

---

## Review Workload Forecast

| File | Action | Est. changed lines |
|---|---|---|
| `engine/skills/actuals_instrumentation_contract_test.go` | Create | ~210 |
| `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json` | Create | ~40 |
| `skills/_shared/actuals-record.schema.json` | Modify (2 description fields only) | ~8 |
| `skills/sdd-time-estimation/SKILL.md` | Modify (Hard Rules, CALIBRATION, Output 6/14, version) | ~55 |
| `skills/inception-pipeline/SKILL.md` | Modify (closure-feedback section) | ~35 |
| **Total** | | **~348** |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
800-line budget risk: Low

~348 lines is well under the session's 800-line review budget (also under the skill-default 400-line guard). No file crosses a risk boundary alone; the largest single file is the new Go contract test, which is additive scaffolding, not logic branching. Single PR, work-unit commits inside it (see Suggested Work Units).

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | RED gate: contract test asserting canonical markers/fixture, run before fixture exists | PR 1 | `cd engine && go test ./skills/ -run TestActualsInstrumentationContract` (expect FAIL) | N/A — content-assertion test, no runtime scenario | `git rm engine/skills/actuals_instrumentation_contract_test.go` |
| 2 | R-014 fixture: exact corrected-record bytes | PR 1 | Same as Unit 1 (still RED — schema/SKILL edits not yet made) | N/A — fixture file only | `git rm engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json` |
| 3 | Schema description edits (D1/D2, zero new properties) | PR 1 | Same command, progresses toward GREEN | N/A — JSON description text only | `git checkout -- skills/_shared/actuals-record.schema.json` |
| 4 | Estimator fix: CALIBRATION baseline exclusion + formula shape (MUST precede Unit 6) | PR 1 | Same command | N/A — SKILL.md prose | `git checkout -- skills/sdd-time-estimation/SKILL.md` |
| 5 | Inception closure-feedback: round-trip counting unit + prose provenance | PR 1 | Same command → GREEN, then `cd engine && go test ./...` | N/A — SKILL.md prose | `git checkout -- skills/inception-pipeline/SKILL.md` |
| 6 | Engram upsert of corrected `sdd/sync-check-repo-behind-origin/actuals` (LAST, after Unit 4) | PR 1 | N/A — data operation; verify-phase compares obs to fixture bytes | N/A — Engram write, no runtime scenario | Re-upsert prior JSON (preserved in obs #2096 revision history) |

## Phase 0: RED Gate (write test first, run before fixture exists)

- [x] 0.1 Create `engine/skills/actuals_instrumentation_contract_test.go`, package `skills`, reusing the `readRepoFile` helper (see `oo_quality_contract_artifact_test.go`). Implement `TestActualsInstrumentationContract` with 7 `t.Run` groups per the design's RED/GREEN table:
  - Group 1: schema `total_wall_clock_hours` + boundary description contains "from the tiering go-ahead checkpoint to archive" and "interruption"; must NOT contain "from first apply to archive". (R-004/R-005)
  - Group 2: schema `checkpoint_count` description contains "round-trip"; must NOT contain "LOWER BOUND". (R-015/R-006/R-007/R-008)
  - Group 3: `sdd-time-estimation/SKILL.md` CALIBRATION text contains "NEVER an input to the agent-compute-time baseline" and the proxy formula `total − sum(phase hours)`; must NOT contain "and `total_wall_clock_hours`, build a per-phase agent-compute-time baseline". (R-009)
  - Group 4: `sdd-time-estimation/SKILL.md` Output item 6 contains "expected checkpoints × round-trip latency + interruption allowance" and "does not scale with checkpoint count". (R-010/R-011)
  - Group 5: `inception-pipeline/SKILL.md` closure-feedback contains "one unit per distinct human round-trip reply" and "explicitly zero"; must NOT contain "is a structural lower bound, not a complete count". (R-006/R-007/R-015)
  - Group 6: read+parse `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json`; assert `checkpoint_count == 11`; `total_wall_clock_hours != 1.4 && >= 24`; `variance_vs_plan` contains "RECONSTRUCTED FROM THE CLOSURE NARRATIVE, NOT MEASURED", "durably observed", "reconstructed from narrative", and "= 11". (R-014, amended R-007)
  - Group 7 (invariant, GREEN-by-construction): schema `properties` key set equals the exact current 13 keys (no additions); every fixture key is in `properties`; schema `required` is a subset of fixture keys. (D2/D5 — regression guard against re-adding sub-count fields)
- [x] 0.2 Run `cd engine && go test ./skills/ -run TestActualsInstrumentationContract` and capture output. Confirm RED: groups 1-5 fail on marker absence/old-text presence, group 6 fails on missing fixture file (`ReadFile` error), group 7 passes (schema unchanged today).

## Phase 1: Fixture (R-014)

- [x] 1.1 Create `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json` — the 9 required schema keys plus `checkpoint_count: 11` only (no sub-count keys). `total_wall_clock_hours: 36`. `variance_vs_plan` includes: the mandatory disclaimer "RECONSTRUCTED FROM THE CLOSURE NARRATIVE, NOT MEASURED"; the itemization 1 (tiering go-ahead, durably observed) + 3 (proposal-decision confirmations) + 4 (judgment-day rounds) + 1 (chained-PR split confirmation) + 1 (merge authorization) + 1 (ACTION-hint fix scope) = 11; and an explicit durable-vs-reconstructed split (1 durable, 10 reconstructed).

## Phase 2: Schema Description Edits (D1/D2 — zero new properties)

- [x] 2.1 In `skills/_shared/actuals-record.schema.json`, rewrite `total_wall_clock_hours.description` to state the boundary "from the tiering go-ahead checkpoint to archive" and that it includes interruption gaps. (R-004/R-005)
- [x] 2.2 In `skills/_shared/actuals-record.schema.json`, rewrite `checkpoint_count.description` in place: redefine as the R-015 round-trip total (drop "structural LOWER BOUND" framing), state the durable floor (R-006), and note that non-durable provenance is disclosed in `variance_vs_plan` prose (R-007/R-008). Do NOT add any property; `additionalProperties: false` and `$id` (bundle `2.0.0`) stay unchanged. (R-006/R-007/R-008/R-015, D2/D5)

## Phase 3: Estimator Fix — MUST precede Phase 5 (D8 binding order)

- [x] 3.1 In `skills/sdd-time-estimation/SKILL.md` Hard Rules, name agent-compute-time, elapsed-calendar-time, and human-confirmation-checkpoint-count as three independently measured units; state they are never blended into one figure. (R-001/R-002)
- [x] 3.2 Rewrite the CALIBRATION rule (line 27): baseline built ONLY from `implementation_hours`, `review_gate_hours`, `post_review_fix_hours`; add explicit marker "NEVER an input to the agent-compute-time baseline" for `total_wall_clock_hours`; add the proxy formula `total − sum(phase hours)` for calendar-time-only signal. (R-009)
- [x] 3.3 Rewrite Output Contract item 6 (delivery window): declared formula `expected checkpoints × round-trip latency + interruption allowance`; fixed, disclosed, uncalibrated interruption buffer below n=3 that does not scale with checkpoint count ("does not scale with checkpoint count"); latency rate cites calibration n or a stated bootstrap default (Low confidence) when n=0. (R-010/R-011)
- [x] 3.4 Rewrite Output Contract item 14 (Actuals and Calibration section): elapsed-calendar-time and checkpoint count reported under labels separate from agent-compute-time. (R-012)
- [x] 3.5 Bump `skills/sdd-time-estimation/SKILL.md` metadata version 1.1 → 1.2.

## Phase 4: Inception Closure-Feedback Edits

- [x] 4.1 In `skills/inception-pipeline/SKILL.md` closure-feedback (Plan/Validate section), state elapsed-calendar-time is measured independently of the compute-time sum, sharing the tiering-go-ahead-to-archive boundary with `checkpoint_count`, including interruption gaps. (R-003/R-004/R-005)
- [x] 4.2 Rewrite the `checkpoint_count` guidance: drop "structural lower bound, not a complete count"; state the counting unit is "one unit per distinct human round-trip reply" (R-015) applied uniformly; state the durable floor (tiering go-ahead + ambiguity question) is always included (R-006); require writing the durable-vs-reconstructed itemization to `variance_vs_plan` prose, including an "explicitly zero" statement when no non-durable checkpoints occurred (R-007). Metadata version stays 2.0.0 (bundle-pinned per D5).

## Phase 5: GREEN Gate

- [x] 5.1 Rerun `cd engine && go test ./skills/ -run TestActualsInstrumentationContract`. Confirm all 7 groups pass (GREEN).
- [x] 5.2 Run the broad suite `cd engine && go test ./...`. Confirm no regressions.

## Phase 6: Engram Upsert (LAST — after Phase 3, no estimation run between this and merge)

- [x] 6.1 Upsert Engram `sdd/sync-check-repo-behind-origin/actuals` with content byte-identical to `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json`. (R-014) **Ordering constraint**: this task MUST NOT run before Phase 3 (estimator fix) is committed — an unfixed CALIBRATION rule reading a corrected ~36h record would corrupt the compute-time baseline ~25×.
- [x] 6.2 Guard note (no separate file change): no `sdd-time-estimation` estimation run may occur between this upsert and merge (design D8 guard) — verify-phase confirms no intervening `sdd/*/estimate` write references this record's pre-correction value. No estimation skill was run during this apply batch after the Phase 6.1 upsert.

## Not Tasked (explicit decisions)

- `skills/roadmap-maker/SKILL.md`: **no edit** (R-013, design D6) — verified zero field-name coupling; the sole "total wall-clock" mention is a correct ownership list. If this is wrong, it is a risk to flag at verify, not a task to add here.
