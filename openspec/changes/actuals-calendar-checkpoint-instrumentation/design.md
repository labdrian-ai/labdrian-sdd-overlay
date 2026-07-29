# Design: Actuals Calendar-Time / Checkpoint-Count Instrumentation Fix

Revision 2 — D2 narrowed per user decision (via orchestrator): no new schema fields; durable-vs-reconstructed split moves to `variance_vs_plan` prose (R-007 amended accordingly).

## Technical Approach

Correct the actuals measurement instrument in place across schema, writer, and estimator, plus one Engram data correction — no new architecture, **no new schema properties**. The central problem is verification: strict TDD is active project-wide but this change is markdown/JSON-schema content, so the declared Go test commands pass trivially. The design therefore ships the RED/GREEN gate **as a committed Go content test** inside the module those commands already run, making `cd engine && go test ./...` genuinely RED before the edits and GREEN after.

## Architecture Decisions

| # | Decision | Alternatives rejected | Rationale |
|---|---|---|---|
| D1 | (Locked, OQ-1b) Correct `total_wall_clock_hours` in place; description boundary rewritten to tiering go-ahead → archive, interruption gaps included (R-003/4/5) | New field / rename | Closed schema; no Go parser coupling; field never carried independent data |
| D2 | **(Revised — user decision)** `checkpoint_count` stays a single field, redefined in place as the R-015 round-trip **total** (tiering go-ahead → archive), losing the "structural LOWER BOUND" framing. The schema gains **zero new properties**. The durable-vs-reconstructed split and the per-category itemization are disclosed in `variance_vs_plan` prose only | Structured sub-count fields (`checkpoint_count_durable`/`_supplemental` + `dependentRequired`) — machine-checkable and calibration-usable, but overruled: user chose free text and amended R-007 to require prose disclosure, keeping this change field-neutral | Migration-free redefinition: no existing record populates `checkpoint_count` (obs #2096 omits it). R-011's future rate divides calendar time by the **total**, which stays structured — the split was provenance quality, not rate arithmetic |
| D3 | Content gate = new Go test `engine/skills/actuals_instrumentation_contract_test.go` asserting canonical marker strings and schema structure | Ad-hoc rg/jq checklist (not durable, not CI); generic JSON-Schema validator (scope creep; entry validator is struct-bespoke) | Precedent: `oo_quality_contract_artifact_test.go` reads `skills/_shared/*` content; `tools/entry-contract-validator/main_test.go:387` already asserts this schema's version string |
| D4 | R-014 correction is fixture-driven: exact upsert bytes committed at `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json`; apply upserts fixture verbatim; verify compares live obs to fixture | Compose upsert ad hoc in-session | CI-assertable content; byte-for-byte verify check |
| D5 | Bundle stays `2.0.0` (schema `$id` unchanged) | Bundle bump 2.1.0 | Now unambiguous: zero new properties, descriptions only; no record invalidated; existing test pins `/actuals-record/2.0.0` |
| D6 | `roadmap-maker/SKILL.md`: **no edit** (R-013) | Symmetry edit | Zero field-name matches; LLM-prose reader inherits corrected source; sole "total wall-clock" mention (line 85) is a correct ownership list |
| D7 | Ship formula **shape** only: window = expected checkpoints × round-trip latency + fixed interruption allowance; allowance fixed, disclosed, uncalibrated until n≥3, never scaling with checkpoint count; rate cites calibration n + provenance or stated bootstrap. No numeric rate (n=1, reconstructed) | Numeric rate now | Locked; honesty constraint |
| D8 | Estimator line-27 fix lands **before** the Engram upsert | Any other order | Today 1.4 ≈ phase sum, so the misread is redundant; after the record says ~36h, an unfixed line 27 corrupts the compute baseline ~25× |

## Content-Verification Gate (RED/GREEN)

Test: `TestActualsInstrumentationContract` (package `skills`, reuse `readRepoFile`). Focused run: `cd engine && go test ./skills/ -run TestActualsInstrumentationContract`. The test is written FIRST and run before the fixture exists — so groups 1-5 are RED on marker absence/old text, and group 6 is RED on the missing fixture file.

| # | Target | Must contain (GREEN) | Must NOT contain (old text) | RED-today trigger | Req |
|---|---|---|---|---|---|
| 1 | schema: both temporal descriptions | "from the tiering go-ahead checkpoint to archive"; "interruption" | "from first apply to archive" | Phrase absent / old phrase present now | R-004/5 |
| 2 | schema: `checkpoint_count` description | "round-trip" | "LOWER BOUND" | "round-trip" absent, "LOWER BOUND" present now | R-015/6/7/8 |
| 3 | estimator CALIBRATION | "NEVER an input to the agent-compute-time baseline"; proxy formula `total − sum(phase hours)` | "and `total_wall_clock_hours`, build a per-phase agent-compute-time baseline" | Marker absent / blend string present now | R-009 |
| 4 | estimator Output 6 | "expected checkpoints × round-trip latency + interruption allowance"; "does not scale with checkpoint count" | — | Both absent now | R-010/11 |
| 5 | inception closure-feedback | "one unit per distinct human round-trip reply"; "explicitly zero" (prose-explicit zero when no non-durable checkpoints) | "is a structural lower bound, not a complete count" | Markers absent / old sentence present now | R-006/7/15 |
| 6 | fixture content pins | parses; `checkpoint_count == 11`; `total_wall_clock_hours != 1.4 && >= 24`; `variance_vs_plan` contains "RECONSTRUCTED FROM THE CLOSURE NARRATIVE, NOT MEASURED", "durably observed", "reconstructed from narrative", and "= 11" itemization | — | Fixture file does not exist → ReadFile fails; thereafter these are drift pins | R-014 (+ amended R-007) |
| 7 | invariants (GREEN today, guards going forward) | schema `properties` key set == the exact current 13 keys (no additions — enforces the user's no-new-field decision); every fixture key ∈ `properties`; schema `required` ⊆ fixture keys | — | Deliberately not RED — labeled invariant, not TDD evidence | D2/D5 |

Group 7's closed-field check remains meaningful despite being GREEN-by-construction: with `additionalProperties: false` and no generic validator in-repo, it is the mechanical proxy proving the fixture would validate against the closed schema, and it now also fails loudly if anyone re-adds sub-count fields.

Not mechanically automatable — stated plainly: (a) **the durable-vs-reconstructed split is no longer machine-verifiable** (prose only, per user decision); compensating checks: group 6 pins the prose markers, and verify re-derives the itemization from obs #2096's narrative, checking the prose arithmetic (1+3+4+1+1+1 = 11) and that the durable line matches pipeline-state (1, tiering go-ahead); (b) R-015 double-application reproducibility — the prose-sum check is the proxy; independent re-derivation is a verify-phase procedural step; (c) the live Engram upsert — CI asserts the fixture; verify compares `mem_get_observation` output to fixture bytes; (d) R-002/R-012 label phrasing — verify-phase checklist review; (e) future-report disclosure (R-011) is a prose contract on future runs.

## Data Flow

    pipeline-state t0 (tiering go-ahead) ─┐
    session narrative (round-trips) ──────┼→ closure-feedback → sdd/{change}/actuals
    archive t1 ───────────────────────────┘   calendar = t1−t0 incl. gaps; checkpoint_count = total round-trips
                                              (durable-vs-reconstructed split: variance_vs_plan prose)
        estimator: compute baseline ← {implementation, review_gate, post_review_fix} ONLY
                   calendar model ← {total_wall_clock_hours, checkpoint_count}

## File Changes

| File | Action | Description |
|---|---|---|
| `engine/skills/actuals_instrumentation_contract_test.go` | Create | The gate (D3) — written first, run RED before the fixture exists |
| `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json` | Create | Exact R-014 upsert bytes: the 9 required keys + `checkpoint_count: 11` only; `total_wall_clock_hours: 36`; itemization + provenance + durable-vs-reconstructed split in `variance_vs_plan` prose. **No sub-count keys** |
| `skills/_shared/actuals-record.schema.json` | Modify | D1 + D2 — **description text only, zero property additions**; `$id` unchanged |
| `skills/sdd-time-estimation/SKILL.md` | Modify | Hard Rules name 3 units (R-001/2); CALIBRATION rewrite (R-009/11); Output 6 + 14 (R-010/12); version 1.1 → 1.2 |
| `skills/inception-pipeline/SKILL.md` | Modify | Closure-feedback: independent calendar measurement; single total per round-trip unit; durable-vs-reconstructed itemization written to `variance_vs_plan` prose, explicit zero when none. Metadata version stays 2.0.0 (bundle-pinned) |
| Engram `sdd/sync-check-repo-behind-origin/actuals` | Correct | Upsert fixture verbatim — final apply step |

## Edit Ordering

1. Write test only → run → **RED** (groups 1-5 markers; group 6 missing fixture; capture output). 2. Write fixture. 3. Schema. 4. Estimator SKILL. 5. Inception closure-feedback. 6. Rerun → **GREEN**. 7. Engram upsert (last; D8 — no estimation run between upsert and merge; CI re-runs the gate on any review-driven edit). 8. Verify: obs equals fixture; re-derive count = 11 from narrative.

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary.

## Migration / Rollout

Files: single `git revert`. Engram: re-upsert original JSON (preserved in obs #2096 revision history and quoted in requirements brief).

## Open Questions

None blocking.
