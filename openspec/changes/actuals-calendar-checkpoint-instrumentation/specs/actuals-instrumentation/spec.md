# Actuals Instrumentation Specification

## Purpose

Defines the actuals-record measurement contract for agent-orchestrated SDD delivery: three independently-tracked units (agent-compute-time, elapsed-calendar-time, human-confirmation-checkpoint-count), one shared capture boundary, a uniform checkpoint-counting rule, and a declared, evidence-disclosed delivery-window formula — spanning the actuals schema, `sdd-time-estimation`, `inception-pipeline` closure-feedback, and `roadmap-maker`.

## Requirements

### Requirement: Three Units Tracked and Never Blended (R-001, R-002)

The Hard Rules MUST name agent-compute-time, elapsed-calendar-time, and human-confirmation-checkpoint-count as three independently measured units. No artifact MAY report two or more of them as a single blended figure.

#### Scenario: Units listed and kept separate

- GIVEN the Hard Rules section and any estimation report or actuals record
- WHEN read
- THEN all three units are named, and any report showing two or more appears under separate labels, never summed

### Requirement: Calendar Time Captured Independently, In Place, With a Shared Boundary (R-003, R-004, R-005)

Elapsed-calendar-time MUST be measured independently of the compute-time sum, corrected **in place** in the existing `total_wall_clock_hours` field, share the tiering-go-ahead-to-archive boundary with `checkpoint_count`, and include interruption gaps. **Binding, schema-wide (not scoped to one field)**: this change MUST NOT add any new property to the actuals schema — not for `total_wall_clock_hours`, not for `checkpoint_count`, not anywhere. `additionalProperties: false` and the existing property set are unchanged; only field descriptions and record values may change.

#### Scenario: Divergent calendar time, shared boundary, interruption included

- GIVEN a closed change with a rate-limit interruption spanning multiple calendar days
- WHEN the actuals record is written
- THEN `total_wall_clock_hours` differs from the compute-time sum, spans tiering-go-ahead through archive (matching `checkpoint_count`'s boundary), and includes the interruption gap

#### Scenario: No schema shape change anywhere in the record

- GIVEN the schema
- WHEN `total_wall_clock_hours` is corrected and `checkpoint_count` is populated
- THEN no property is added anywhere in the schema (including no supplemental or sub-count sibling field) and `additionalProperties: false` is unchanged

### Requirement: One Checkpoint Equals One Round-Trip Reply (R-015)

`checkpoint_count` MUST count one unit per distinct human round-trip reply, uniformly across every category, regardless of how many decisions were resolved within that reply.

#### Scenario: Batching vs. repetition counted correctly

- GIVEN one reply resolving 3 decisions, and a separate step requiring 4 distinct replies
- WHEN checkpoints are counted
- THEN the batched reply contributes 1 and the repeated step contributes 4

### Requirement: Durable Checkpoints Guaranteed; Non-Durable Checkpoints Disclosed as Free-Text Provenance, Never a New Field (R-006, R-007, R-008)

`checkpoint_count` MUST include every durably-observed checkpoint type `pipeline-state` records (tiering go-ahead; ambiguity clarifying question if fired) as a guaranteed floor within the single total. WHERE checkpoints occur during phases inception-pipeline does not durably observe, the record MUST NOT silently omit them from the total, and MUST disclose in `variance_vs_plan` free text which checkpoints were durably observed and which were reconstructed from the closure narrative. No structured sub-count field is added: no `checkpoint_count_durable`, no `checkpoint_count_supplemental`, no sibling field of any name — per the schema-wide no-new-property rule above. Any field description asserting it is the "real calendar-time driver" MUST match this single-field, prose-disclosed capacity, not overclaim precision the field alone cannot structurally guarantee. **Tradeoff, stated plainly**: the durable-vs-reconstructed split is therefore not machine-verifiable (free text, not a schema-enforced field); the compensating check is verify-phase review of the prose itemization for completeness and accuracy.

#### Scenario: Non-durable checkpoints disclosed in free text, durable vs. reconstructed itemized

- GIVEN a change whose delivery included judgment-day rounds and merge authorization (not durably observed by `pipeline-state`)
- WHEN its actuals record is closed
- THEN `checkpoint_count` includes them in the single total, AND `variance_vs_plan` itemizes in prose which checkpoints were durably observed (via `pipeline-state`) and which were reconstructed from the closure narrative

#### Scenario: No non-durable checkpoints — disclosure states so explicitly

- GIVEN no non-durable checkpoints occurred
- WHEN the record is closed
- THEN `variance_vs_plan` explicitly states that all counted checkpoints were durably observed, not silently omitting comment

### Requirement: Compute-Time Baseline Built From Three Phase Fields Only (R-009)

The CALIBRATION rule's per-phase agent-compute-time baseline MUST exclude the elapsed-calendar-time field, and MUST be built only from `implementation_hours`, `review_gate_hours`, and `post_review_fix_hours`.

#### Scenario: Baseline text is unambiguous

- GIVEN the CALIBRATION rule (`sdd-time-estimation/SKILL.md` line 27)
- WHEN read
- THEN its baseline inputs are exactly those three fields; `total_wall_clock_hours` is absent

### Requirement: Delivery Window From a Declared Formula, With a Fixed Uncalibrated Buffer Below n=3 (R-010)

The delivery-window output MUST derive from a declared formula (checkpoint count × round-trip latency + interruption allowance) with all inputs disclosed. Until calibration reaches n≥3 records, the interruption allowance MUST be a fixed, explicitly-disclosed buffer marked "uncalibrated," and MUST NOT scale with expected checkpoint count.

#### Scenario: Formula, inputs, and fixed buffer disclosed

- GIVEN a pre-start report with fewer than 3 calibration records
- WHEN it states a delivery window
- THEN it discloses checkpoint count, latency rate, and a fixed interruption buffer marked uncalibrated, independent of expected checkpoint count

### Requirement: Latency Rate Is a Formula Shape, Not a Shipped Calibrated Number (R-011)

This change MUST define the round-trip-latency-rate formula and begin populating its inputs; it MUST NOT ship the rate as a calibrated figure. WHEN n=0, a stated bootstrap default MUST be used with confidence marked Low. WHEN n≥1, the rate MUST cite its calibration sample and disclosed n.

#### Scenario: n=1 stays disclosed bootstrap, not calibrated

- GIVEN exactly one corrected actuals record
- WHEN a delivery-window estimate is produced
- THEN the rate discloses n=1 and is marked Low-confidence/bootstrap — never presented as a calibrated rate

### Requirement: Actuals Output and Roadmap Tracking Report Units Distinctly (R-012, R-013)

The Actuals and Calibration output section MUST report elapsed-calendar-time and checkpoint count under labels separate from agent-compute-time. `roadmap-maker` MUST source tracking-line figures from these corrected fields, never from the compute-time field.

#### Scenario: Report and tracking line both correctly labeled

- GIVEN a completed change's estimation report and its roadmap-maker tracking line
- WHEN both are read
- THEN calendar-time and checkpoint count appear under their own labels in the report, and the tracking line's figures match the corrected actuals fields

### Requirement: Historical Record Corrected With a Mandatory Provenance Disclaimer (R-014)

`sdd/sync-check-repo-behind-origin/actuals` MUST be corrected in place: `total_wall_clock_hours` set to a best-estimate approximate value (~36 hours, ~1.5 days) reconstructed from the closure narrative — not a conservative lower bound — accompanied by an explicit, unmissable statement that this figure is reconstructed, not measured. `checkpoint_count` MUST be added at 11, itemized 1+3+4+1+1+1 in `variance_vs_plan`, distinguishing the 1 durably-observed checkpoint (tiering go-ahead) from the 10 reconstructed from the closure narrative, per the R-007 disclosure rule above.

#### Scenario: Provenance disclaimer present (mandatory — fails if absent)

- GIVEN the corrected record
- WHEN `total_wall_clock_hours` is read as ~36
- THEN the record explicitly states this value is reconstructed from the closure narrative, not measured — a corrected record missing this statement FAILS this requirement

#### Scenario: Checkpoint count added and itemized to 11

- GIVEN the corrected record
- WHEN `checkpoint_count` is read
- THEN it reads 11, with `variance_vs_plan` itemizing 1+3+4+1+1+1 summing to 11, marking the tiering-go-ahead item as durably observed and the remaining ten as reconstructed from the closure narrative
