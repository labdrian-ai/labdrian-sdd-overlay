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

### Requirement: Calendar Time Captured Independently, Merge-Anchored Shared Boundary (R-003, R-004, R-005)

Elapsed-calendar-time MUST be measured independently of the compute-time sum, corrected in place in `total_wall_clock_hours`, share the tiering-go-ahead-to-**merge** boundary with `checkpoint_count`, and include interruption gaps. The right edge is merge, not archive: archive lag is bookkeeping and MUST NOT count as development time. This change MUST NOT add any new property anywhere in the schema; `additionalProperties: false` and the property set are unchanged.
(Previously: right edge was tiering-go-ahead-to-archive.)

#### Scenario: Divergent calendar time, interruption included

- GIVEN a closed change with a multi-day interruption
- WHEN the actuals record is written
- THEN `total_wall_clock_hours` spans tiering-go-ahead through the merge-commit committer timestamp, matches `checkpoint_count`'s boundary, and includes the gap

#### Scenario: Post-merge bookkeeping replies do not count

- GIVEN archive authorization happens after the landing PR merges
- WHEN `checkpoint_count` is computed
- THEN the archive-authorization reply is excluded as post-boundary

#### Scenario: No schema shape change anywhere in the record

- GIVEN the schema
- WHEN `total_wall_clock_hours` and `checkpoint_count` are corrected
- THEN no property is added and `additionalProperties: false` is unchanged

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
- THEN `variance_vs_plan` explicitly states that all counted checkpoints were durably observed and that zero were reconstructed from the closure narrative, not silently omitting comment — the zero case names BOTH halves of the split, because the half-disclosure rule rejects prose that names only one

### Requirement: Compute-Time Baseline Built From Three Phase Fields Only (R-009)

The CALIBRATION rule's per-phase agent-compute-time baseline MUST exclude the elapsed-calendar-time field, and MUST be built only from `implementation_hours`, `review_gate_hours`, and `post_review_fix_hours`.

#### Scenario: Baseline text is unambiguous

- GIVEN the CALIBRATION rule (`sdd-time-estimation/SKILL.md` line 27)
- WHEN read
- THEN its baseline inputs are exactly those three fields; `total_wall_clock_hours` is absent

### Requirement: Delivery Window From a Declared Formula, With a Separate Fixed Uncalibrated Allowance (R-010)

The delivery-window output MUST derive from a declared formula (checkpoint count × round-trip latency + interruption allowance) with all inputs disclosed. Until explicit interruption evidence exists, the separate interruption allowance MUST be a fixed, explicitly-disclosed buffer marked "uncalibrated," and MUST NOT scale with expected checkpoint count.

#### Scenario: Formula, inputs, and fixed buffer disclosed

- GIVEN a pre-start report without explicit interruption evidence
- WHEN it states a delivery window
- THEN it discloses checkpoint count, latency rate, and a separate fixed interruption buffer marked uncalibrated, independent of expected checkpoint count

### Requirement: Latency Rate Is a Formula Shape, Not a Shipped Calibrated Number (R-011)

This change MUST define the round-trip-latency-rate formula and begin populating its inputs; it MUST NOT ship the rate as a calibrated figure. Calibration MUST use only interruption-clean residual samples with positive `checkpoint_count`; session/rate-limit/provider-interruption samples MUST be excluded, never adjusted by subtracting guessed interruption duration. WHEN no eligible clean sample exists, a disclosed bootstrap default MUST be used with confidence marked Low; otherwise the rate MUST cite its eligible clean sample and disclosed n.

#### Scenario: Interrupted n=1 leaves no eligible clean sample

- GIVEN exactly one corrected actuals record and it contains a provider interruption
- WHEN a delivery-window estimate is produced
- THEN the interruption-contaminated record is excluded, eligible clean-sample n=0 is disclosed, and the rate is marked Low-confidence/bootstrap — never adjusted or presented as calibrated

### Requirement: Actuals Output and Roadmap Tracking Report Units Distinctly (R-012, R-013)

The Actuals and Calibration output section (`sdd-time-estimation/SKILL.md` Output item 14) MUST report elapsed-calendar-time and checkpoint count under labels separate from agent-compute-time, never blended. `skills/roadmap-maker/SKILL.md` MUST NOT reference any structured actuals field name — `total_wall_clock_hours`, `checkpoint_count`, `implementation_hours`, `review_gate_hours`, `post_review_fix_hours` — as a tracking-line data source. The enforcing gate is a whole-file forbid list over those five names, so the prohibition is absolute: none of them may appear anywhere in the file, including inside ownership-attribution prose. `roadmap-maker` refers to the actuals record by its topic (`sdd/{change}/actuals`) and describes the effort it consumes in prose instead of naming fields.

#### Scenario: Actuals output labels units distinctly

- GIVEN a completed change's Actuals and Calibration output section
- WHEN it is read
- THEN elapsed-calendar-time and checkpoint count appear under their own labels, separate from agent-compute-time, never summed into one figure

#### Scenario: roadmap-maker never sources tracking figures from compute-time

- GIVEN `skills/roadmap-maker/SKILL.md`
- WHEN scanned for structured actuals field names (`total_wall_clock_hours`, `checkpoint_count`, `implementation_hours`, `review_gate_hours`, `post_review_fix_hours`)
- THEN none of those five names appears anywhere in the file, and every actuals-related mention (nine prose lines, not one) refers to the actuals record by its topic (`sdd/{change}/actuals`) and attributes ownership to `inception-pipeline` closure-feedback without naming any structured field

**Scope note (superseded — the deferral has since been taken up elsewhere):** this note originally recorded that extending the tracking line with dedicated elapsed-calendar-time and checkpoint-count slots was OUT OF SCOPE, because design decision D6 chose not to edit `roadmap-maker` and "the template has no such slots today". The second half of that sentence is no longer true: `skills/roadmap-maker/assets/roadmap-template.md` was subsequently rewritten and now carries the full tracking table, including rows sourced from `total_wall_clock_hours` and `checkpoint_count`, each with its own source attribution and the `[PENDING]` / `[NOT MEASURED — reason]` sentinel pair.

The two statements are not in conflict once the OWNER is named, which the original note never did. R-013's negative obligation binds `skills/roadmap-maker/SKILL.md` — no structured actuals field name appears there, and the enforcing whole-file forbid list is unchanged. The positive slots live in the template ASSET, which is a different file and was never in R-013's scope. `SKILL.md` now renders the tracking block FROM that asset instead of restating it, so the duplicate that let the two drift apart is gone: the asset owns the rows, and the skill owns nothing to contradict them with. Nothing in this requirement asks `SKILL.md` to name a field, and nothing here weakens the forbid list.

### Requirement: Historical Record Corrected With a Mandatory Provenance Disclaimer (R-014)

`sdd/sync-check-repo-behind-origin/actuals` MUST be corrected in place: `total_wall_clock_hours` set to a best-estimate approximate value (~36 hours, ~1.5 days) reconstructed from the closure narrative — not a conservative lower bound — accompanied by an explicit, unmissable statement that this figure is reconstructed, not measured. `checkpoint_count` MUST be added at 12, itemized 1 tiering go-ahead + 1 AMB-001 ambiguity clarifying question + 3+4+1+1+1 = 12 in `variance_vs_plan`, distinguishing the 2 durably-observed checkpoints supported by `pipeline-state` from the 10 reconstructed from the closure narrative, per the R-007 disclosure rule above.

#### Scenario: Provenance disclaimer present (mandatory — fails if absent)

- GIVEN the corrected record
- WHEN `total_wall_clock_hours` is read as ~36
- THEN the record explicitly states this value is reconstructed from the closure narrative, not measured — a corrected record missing this statement FAILS this requirement

#### Scenario: Checkpoint count added and itemized to 12

- GIVEN the corrected record
- WHEN `checkpoint_count` is read
- THEN it reads 12, with `variance_vs_plan` itemizing 1 tiering go-ahead + 1 AMB-001 ambiguity clarifying question + 3+4+1+1+1 = 12, marking the first two as the durable floor supported by `pipeline-state` and the remaining ten as reconstructed from the closure narrative
## ADDED Requirements

### Requirement: Boundary Anchors: Named, Derived, Legible, or Omitted (R-016, R-017, R-018)

Both anchors MUST come from typed, content-bound sources; neither may be parsed out of rendered display text.

**t0** MUST be read from the typed `created_at` field of the Engram observation `sdd/{change}/pipeline-state` (tiering-go-ahead), obtained through a structured export rather than a rendered footer. WHEN that observation does not exist — a change entered directly at SDD never runs inception-pipeline and so never has one — t0 MUST fall back to the earliest typed `created_at` among the change's own `sdd/{change}/*` observations, and the fallback MUST be named in `variance_vs_plan` so it is never mistaken for a tiering-go-ahead anchor. A `pipeline-state` observation carrying no `topic_key` is unresolvable by definition and MUST be treated as absent.

**t1** MUST be the committer timestamp of the commit that landed the change on the default branch, identified by `landing_commit` recorded in a **versioned** artifact at delivery. WHEN a native review receipt exists for that candidate, the archive-report additionally records the approved candidate tree hash that authorizes it, and the anchor MUST be independently **verified**: the landing commit's own tree MUST equal the recorded tree hash, and WHEN it does not the anchor MUST be **rejected** and t1 omitted. WHEN no review ran, the approved-tree hash MUST NOT be synthesized from `landing_commit`'s own tree — a check comparing a value against itself can never fail, so it would not be verification. In that case t1 STILL resolves from `landing_commit` alone, and the anchor MUST be recorded as **self-asserted**: used, but with no independent authority to check it against. `variance_vs_plan` MUST name which of the three outcomes — verified, self-asserted, or rejected — applies, so no reader mistakes a self-asserted anchor for a verified one.

The anchor MUST NOT be read from unversioned local state. Review receipts under the repository's Git common directory are not versioned and are keyed by opaque lineage ids with no binding to a change name, so they cannot serve as the resolution source for anyone who did not perform the review.

Identification MUST NOT fall back to scanning which commits touched the change's own folder: a commit belonging to an unrelated change can touch that folder and would silently become the anchor. WHEN no versioned anchor is recorded — a change predating this convention — t1 MUST be omitted and the reason stated, never re-derived heuristically. A confidently wrong duration is worse for calibration than an absent one.

WHEN a change is delivered as chained slices, the recorded anchor MUST be the last slice to land. A tree hash MUST be used to **verify** a commit, never to **discover** one, because trees are not unique across commits — a content-preserving merge reproduces its parent's tree. WHEN a tree hash is all that is available and more than one commit on the default branch carries it, the anchor is ambiguous: t1 MUST be omitted and the ambiguity disclosed, never resolved by position.

closure-feedback (no new actor) MUST derive both anchors from Engram and git alone — no hooks, portable to Claude Code, OpenCode and Codex — and MUST record both in `variance_vs_plan` AND in an archive-report "Cycle timestamps" section (t0: observation id, topic key, `created_at`, and whether it is the primary or fallback source; t1: commit SHA, timestamp, and resolution path). WHEN neither t0 nor t1 resolves, `total_wall_clock_hours` MUST be omitted, never estimated.

#### Scenario: Anchors resolve and are legible in both stores

- GIVEN a change with a `pipeline-state` observation and a versioned archive-report recording `landing_commit` and `approved_tree` at delivery
- WHEN closure-feedback closes the cycle
- THEN t0 comes from the typed `created_at` field, t1 is the committer timestamp of `landing_commit` once its own tree is verified to equal the recorded `approved_tree`, and both appear with their sources named in the actuals record and the archive-report

#### Scenario: An anchor with no review receipt is used but recorded as self-asserted, never verified

- GIVEN a versioned archive-report recording `landing_commit` but no review receipt for that candidate, so `approved_tree` is not recorded
- WHEN t1 is resolved
- THEN t1 still resolves from `landing_commit`'s committer timestamp, `approved_tree` is not synthesized from `landing_commit`'s own tree, and `variance_vs_plan` names the outcome as self-asserted rather than verified

#### Scenario: An unrelated change touching the folder does not become the anchor

- GIVEN a change whose folder was also touched by a commit belonging to a different change
- WHEN t1 is resolved
- THEN t1 is the recorded landing commit, and the unrelated commit is never selected

#### Scenario: A mis-recorded anchor is rejected, not trusted

- GIVEN a recorded landing commit whose own tree does not equal the recorded approved tree
- WHEN t1 is resolved
- THEN the anchor is rejected, `total_wall_clock_hours` is omitted, and the mismatch is stated

#### Scenario: A change predating the convention omits rather than guesses

- GIVEN an archived change with no recorded landing anchor
- WHEN the actuals record is closed
- THEN t1 is absent, no folder-scan re-derivation is attempted, and the reason is stated

#### Scenario: A change that skipped inception-pipeline still measures

- GIVEN a change with no `pipeline-state` observation
- WHEN the actuals record is closed
- THEN t0 is the earliest typed `created_at` among that change's own SDD observations, `total_wall_clock_hours` is present, and `variance_vs_plan` names the fallback

#### Scenario: Neither anchor resolves

- GIVEN a change with no resolvable t0 and no resolvable t1
- WHEN the actuals record is closed
- THEN `total_wall_clock_hours` is absent, every other resolved field is still written, and `variance_vs_plan` states which resolution was attempted

### Requirement: An Unresolvable Anchor Costs One Field, Not the Whole Record (R-021)

`total_wall_clock_hours` MUST NOT appear in the schema's `required` list, so that omitting it under R-016 is valid rather than fatal. `implementation_hours`, `review_gate_hours`, and `post_review_fix_hours` are also absent from `required`, for the separate reason R-019 states; aside from these four names, every other required field is unchanged, and the property set itself is untouched in all cases — this relaxes which fields are mandatory, not which fields exist. A record whose t0 cannot be resolved MUST still be written with every field that did resolve; the all-or-nothing closure rule MUST NOT discard a record for this omission alone. Rationale: three of five cycles produced no record at all because one unavailable field voided the whole write, which is the direct cause of the calibration sample stalling at n=2.

#### Scenario: A cycle missing t0 still produces a usable record

- GIVEN a closed change whose `pipeline-state` observation cannot be found
- WHEN closure-feedback writes the actuals record
- THEN the record validates and persists with `total_wall_clock_hours` absent, every other resolved field present, and `variance_vs_plan` stating why t0 was unresolvable

#### Scenario: Loosening required does not loosen the shape

- GIVEN the schema after this change
- WHEN it is compared to its prior version
- THEN `total_wall_clock_hours` is removed from `required` under this requirement and `implementation_hours`, `review_gate_hours`, and `post_review_fix_hours` are separately removed under R-019 — no other name changes, no property is added or removed, and `additionalProperties: false` is unchanged

### Requirement: Compute-Time Fields Stay Unpopulated With Stated Technical Reason (R-019)

`implementation_hours`, `review_gate_hours`, and `post_review_fix_hours` MUST stay unpopulated until a durable source exists and MUST NOT be filled with elapsed-calendar-time or any proxy. State why in the record or its closure-feedback documentation: orchestrator telemetry is session-transient, the `sdd-attempt` ledger has no timestamp fields, and transcripts carry no structured subagent durations.

#### Scenario: Deferral is stated, not silent

- GIVEN a closed record with the three compute-time fields empty
- WHEN it or its closure-feedback documentation is read
- THEN the technical reason is stated and none of the three fields holds a wall-clock-derived substitute

### Requirement: Historical n=2 Records Carry Boundary-Provenance Annotations (R-020)

`sdd/sync-check-repo-behind-origin/actuals` MUST carry a `variance_vs_plan` annotation stating it was NOT re-based to merge — its `total_wall_clock_hours` is a hand reconstruction with no measured anchors, and re-basing it would manufacture false precision. `sdd/skills-validate-ondisk-gate/actuals` MUST be re-based to the merge-anchored value WHEN both t0 and t1 resolve; WHEN either anchor is missing, it MUST instead carry a `variance_vs_plan` annotation stating why.

#### Scenario: Reconstructed record stays annotated, not re-based

- GIVEN `sync-check-repo-behind-origin`'s reconstructed ~36h value
- WHEN its record is reviewed
- THEN `variance_vs_plan` states it was not re-based, because the value is hand-reconstructed, not measured

#### Scenario: Measured record re-based when both anchors resolve, else annotated

- GIVEN `skills-validate-ondisk-gate`'s record
- WHEN both t0 and t1 resolve
- THEN `total_wall_clock_hours` is re-based to the merge-anchored value; WHEN either anchor is missing, it stays unchanged and `variance_vs_plan` states which anchor failed
