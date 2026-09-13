# Delta for Actuals Instrumentation

## MODIFIED Requirements

### Requirement: Boundary Anchors: Named, Derived, Legible, or Omitted (R-016, R-017, R-018)

Traces to: R-009, R-011

Both anchors MUST come from typed, content-bound sources; neither may be parsed out of rendered display text.

**t0** MUST be read from the typed `created_at` field of the Engram observation `sdd/{change}/pipeline-state` (tiering-go-ahead), obtained through a structured export rather than a rendered footer. WHEN that observation does not exist — a change entered directly at SDD never runs inception-pipeline and so never has one — t0 MUST fall back to the earliest typed `created_at` among the change's own `sdd/{change}/*` observations, and the fallback MUST be named in `variance_vs_plan` so it is never mistaken for a tiering-go-ahead anchor. A `pipeline-state` observation carrying no `topic_key` is unresolvable by definition and MUST be treated as absent.

**t1** MUST be the committer timestamp of the commit that landed the change on the default branch, identified by `landing_commit` recorded in a **versioned** artifact at delivery. WHEN a persisted `openspec/changes/<change>/review-receipts/<lineage>.json` exists for that candidate (per the `review-receipt-capture` capability), the archive-report additionally records the `final_candidate_tree` it carries as `approved_tree`, and the anchor MUST be independently **verified**: the landing commit's own tree MUST equal the recorded `approved_tree`, and WHEN it does not the anchor MUST be **rejected** and t1 omitted. WHEN no persisted receipt exists for the lineage, `archive-anchor-gate` MUST block archive rather than resolve t1 from a self-asserted `approved_tree`, unless an explicit owner override is recorded; WHEN an owner override is recorded, t1 STILL resolves from `landing_commit` alone, and the anchor MUST be recorded as **self-asserted with a recorded override** — used, but with no independent authority to check it against. `variance_vs_plan` MUST name which of the three outcomes — verified, self-asserted with a recorded override, or rejected — applies, so no reader mistakes a self-asserted anchor for a verified one.
(Previously: WHEN no review ran, t1 resolved from `landing_commit` alone and was recorded as self-asserted with no archive-block path; a missing receipt never blocked archive.)

The anchor MUST NOT be read from unversioned local state. Review receipts under the repository's Git common directory are not versioned and are keyed by opaque lineage ids with no binding to a change name, so they cannot serve as the resolution source for anyone who did not perform the review. The persisted `review-receipts/<lineage>.json` copy is versioned and change-bound, and is the only source `approved_tree` may be read from.

Identification MUST NOT fall back to scanning which commits touched the change's own folder: a commit belonging to an unrelated change can touch that folder and would silently become the anchor. WHEN no versioned anchor is recorded — a change predating this convention — t1 MUST be omitted and the reason stated, never re-derived heuristically. A confidently wrong duration is worse for calibration than an absent one.

WHEN a change is delivered as chained slices, the recorded anchor MUST be the last slice to land. A tree hash MUST be used to **verify** a commit, never to **discover** one, because trees are not unique across commits — a content-preserving merge reproduces its parent's tree. WHEN a tree hash is all that is available and more than one commit on the default branch carries it, the anchor is ambiguous: t1 MUST be omitted and the ambiguity disclosed, never resolved by position.

closure-feedback (no new actor) MUST derive both anchors from Engram and git alone — no hooks, portable to Claude Code, OpenCode and Codex — and MUST record both in `variance_vs_plan` AND in an archive-report "Cycle timestamps" section (t0: observation id, topic key, `created_at`, and whether it is the primary or fallback source; t1: commit SHA, timestamp, and resolution path, including whether an owner override was recorded). WHEN neither t0 nor t1 resolves, `total_wall_clock_hours` MUST be omitted, never estimated.

#### Scenario: Anchors resolve and are legible in both stores

- GIVEN a change with a `pipeline-state` observation and a versioned archive-report recording `landing_commit`, and a persisted review receipt recording `final_candidate_tree`
- WHEN closure-feedback closes the cycle
- THEN t0 comes from the typed `created_at` field, t1 is the committer timestamp of `landing_commit` once its own tree is verified to equal the receipt's `final_candidate_tree`, and both appear with their sources named in the actuals record and the archive-report

#### Scenario: A missing receipt blocks archive unless an owner override is recorded

- GIVEN a versioned archive-report recording `landing_commit` but no persisted `review-receipts/<lineage>.json` for that lineage
- WHEN archive is attempted
- THEN `archive-anchor-gate` SHALL block archive with an explicit "no verified receipt" reason
- AND WHEN an explicit owner override is recorded instead, archive SHALL proceed, t1 SHALL resolve from `landing_commit`'s committer timestamp, and `variance_vs_plan` SHALL name the outcome as self-asserted with a recorded override, never as verified

#### Scenario: An unrelated change touching the folder does not become the anchor

- GIVEN a change whose folder was also touched by a commit belonging to a different change
- WHEN t1 is resolved
- THEN t1 is the recorded landing commit, and the unrelated commit is never selected

#### Scenario: A mis-recorded anchor is rejected, not trusted

- GIVEN a recorded landing commit whose own tree does not equal the receipt's recorded `final_candidate_tree`
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
