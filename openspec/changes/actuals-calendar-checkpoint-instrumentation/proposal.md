# Proposal: Actuals Calendar-Time / Checkpoint-Count Instrumentation Fix

## Intent

`sdd-time-estimation` v1.1 fixed the headline unit (agent-compute-time). The measuring instrument feeding it is still corrupted:

- `skills/sdd-time-estimation/SKILL.md:27` folds `total_wall_clock_hours` into the per-phase **agent-compute-time** baseline, while `skills/_shared/actuals-record.schema.json:38` declares that field as **elapsed wall-clock** time — a direct cross-file unit contradiction.
- The sole actuals record (Engram obs #2096) stores compute time there: 1.4 ≈ 0.6+0.61+0.15 phase sum, so the field carries no independent information — while real delivery spanned more than one calendar day (provider session reset) and 11 human round-trips, narrated only in `variance_vs_plan` free text.
- The schema's two temporal fields declare different boundaries: line 38 "first apply to archive" vs `checkpoint_count` anchored at tiering go-ahead (lines 68-71) — arithmetically invalid for any latency-rate division.
- Output Contract item 6 (`SKILL.md:67`) demands a delivery window with no declared formula — the door human-effort intuition re-enters through.

Fix the instrument so calendar forecasts become evidence-driven. Requirements: R-001..R-015 (obs #2526).

## Scope

### In Scope

| Deliverable | Reqs | Surface |
|---|---|---|
| Hard Rules name 3 units (compute / calendar / checkpoint), never blended | R-001, R-002 | `sdd-time-estimation/SKILL.md` |
| Calendar time captured independently of phase sum, interruption gaps included, shared boundary tiering-go-ahead → archive on both fields | R-003, R-004, R-005 | schema + `inception-pipeline` closure-feedback |
| Checkpoint unit = one human round-trip reply; durable sub-count kept; non-durable checkpoints as explicit supplemental count (zero, not absent); truthful description | R-015, R-006, R-007, R-008 | schema + closure-feedback |
| CALIBRATION excludes calendar field from compute baseline; delivery window from declared formula (checkpoints × round-trip latency + interruption allowance); rate cites calibration sample or stated bootstrap; item 14 reports units distinctly | R-009, R-010, R-011, R-012 | `sdd-time-estimation/SKILL.md` |
| roadmap-maker tracking sources corrected field semantics | R-013 | `roadmap-maker/SKILL.md` |
| Correct obs #2096: calendar-time ≠ 1.4h; `checkpoint_count` = 11, itemized 1+3+4+1+1+1, method self-documented | R-014 | Engram data |

### Out of Scope

- Overlay staleness detection (separate queued change); roadmap item 21's stale `planned` status; `sdd-time-estimation` absent from `.atl/skill-registry.md`; upstream gentle-ai#1976.
- New durable checkpoint-observation instrumentation (OQ-3, future roadmap item); `entry-contract.schema.json`; reworking the v1.1 headline-unit contract.

## Capabilities

### New Capabilities
- `actuals-instrumentation`: the three-unit actuals measurement contract — capture boundaries, checkpoint counting unit, calibration-input rules, and delivery-window formula disclosure across schema, writer, estimator, and roadmap consumer.

### Modified Capabilities
None — no existing spec in `openspec/specs/` covers estimation or actuals.

## Approach

- **Correct `total_wall_clock_hours` in place** (OQ-1b resolved, exploration-verified): schema is closed (`additionalProperties: false`, schema line 7); the description is already accurate, so the text is not the bug; no Go code parses actuals field content (grep across `engine/**/*.go`: filename presence + one version string in `main_test.go` only); roadmap-maker reads actuals via LLM prose with no field-name coupling. Once corrected, `total_wall_clock_hours − Σ(phase hours)` becomes a zero-cost checkpoint-wait proxy for R-010/R-011.
- **Honesty constraint**: with n=1 and obs #2096 missing all four complexity-unit fields, this change CANNOT produce a real numeric latency rate. R-010/R-011 are scoped as "define the formula shape and start populating its inputs" — a stated bootstrap default with disclosed n, never an invented figure.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `skills/_shared/actuals-record.schema.json` | Modified | Boundary alignment, counting-unit rule, supplemental count, description truthfulness |
| `skills/sdd-time-estimation/SKILL.md` | Modified | Hard Rules, CALIBRATION line 27, Output items 6 + 14 |
| `skills/inception-pipeline/SKILL.md` | Modified | Closure-feedback measures calendar time independently, counts per round-trip |
| `skills/roadmap-maker/SKILL.md` | Modified | Tracking line sources corrected fields |
| Engram obs #2096 | Corrected | R-014 data fix via topic-key upsert |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Bootstrap latency default read as calibrated precision | Med | R-011: disclose n and bootstrap-vs-calibrated in every report |
| R-014's 11 is narrative reconstruction, not live observation | Med | Corrected record self-documents method and provenance |
| Writer still cannot durably observe mid-SDD checkpoints | High | R-007 supplemental count is explicit (zero, never absent); durable fix deferred to OQ-3 |
| Green `go test` mistaken for verification of content-only change | Med | sdd-design must define a content-verification RED/GREEN gate (obs #2530) |

## Rollback Plan

All file edits are prose/schema text — single `git revert`. The obs #2096 correction is a topic-key upsert; restore by re-upserting the original JSON (preserved verbatim in obs #2096 revision history and quoted in obs #2526).

## Dependencies

- None external. `inception-pipeline` remains the single writer of `sdd/{change}/actuals`.

## Success Criteria

- [ ] `SKILL.md:27` baseline inputs list only `implementation_hours`, `review_gate_hours`, `post_review_fix_hours` (R-009)
- [ ] Both schema fields declare tiering-go-ahead → archive (R-004)
- [ ] Delivery-window output states formula, inputs, and calibration n (R-010/R-011)
- [ ] obs #2096 corrected: calendar-time ≠ 1.4h, checkpoint total 11 itemized (R-014)
- [ ] All 15 requirement IDs traced into the spec
