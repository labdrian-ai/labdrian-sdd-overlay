# Delta for Actuals Instrumentation

## MODIFIED Requirements

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
