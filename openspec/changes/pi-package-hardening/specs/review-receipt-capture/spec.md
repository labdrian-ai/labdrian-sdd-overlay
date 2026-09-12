# Review Receipt Capture Specification

## Purpose

Persist a review transaction's receipt before `acknowledge-approved` burns
it, so `approved_tree` is always sourced from a verified receipt — never
self-asserted — closing the bug that archived three prior changes with
unverified anchors.

## Requirements

### Requirement: Receipt Persisted Before Acknowledge Burns It

Traces to: R-008

WHEN a review transaction reaches the `approved` state and before the
`acknowledge-approved` invocation runs, the orchestrator workflow SHALL
persist that transaction's approved artifact to
`openspec/changes/<change>/review-receipts/`. Two on-disk shapes are
recognized: the legacy `gentle-ai.review-receipt/v2` `review-receipt.json`
(gentle-ai versions before 2.7.0), persisted as `<lineage>.json`, and the
gentle-ai 2.7.0+ lifecycle `review-state.json` -- under which an
approved-but-unacknowledged lineage directory contains ONLY this file --
persisted as `<lineage>.review-state.json`. Both are captured when both are
present for the same lineage.

#### Scenario: Receipt file exists before acknowledge is invoked

- GIVEN a review transaction reaches `approved`
- WHEN the orchestrator proceeds to acknowledge it
- THEN `openspec/changes/<change>/review-receipts/<lineage>.json` (or
  `<lineage>.review-state.json` on gentle-ai 2.7.0+) SHALL exist and SHALL
  resolve, via `reviewreceipt.ApprovedSummary`, to a `final_candidate_tree`,
  `selected_lenses`, and approved state before `acknowledge-approved` is
  invoked

### Requirement: approved_tree Sourced Only From the Persisted Receipt

Traces to: R-009

WHEN `tools/archive-anchor-gate` computes `approved_tree` for a change, it
SHALL set `approved_tree` equal to the `final_candidate_tree` field read
from that change's persisted `openspec/changes/<change>/review-receipts/<lineage>.json`.

#### Scenario: approved_tree equals the receipt's final_candidate_tree

- GIVEN a persisted receipt with `final_candidate_tree: "deadbeef"`
- WHEN `archive-anchor-gate` runs
- THEN `approved_tree` SHALL equal `"deadbeef"`

### Requirement: Closure-Feedback Reads the Persisted Receipt, Not Live State

Traces to: R-010

WHEN closure-feedback computes a change's `approved_tree` for the actuals
record, closure-feedback SHALL read it from
`openspec/changes/<change>/review-receipts/<lineage>.json` rather than from
the live review-state store.

#### Scenario: Post-acknowledge closure still produces a verified value

- GIVEN a change whose review transaction has already been acknowledged
  (post-burn, only `review-state.json` remains live)
- WHEN closure-feedback runs
- THEN it SHALL still produce a verified `approved_tree` sourced from the
  persisted receipt file

### Requirement: Archive Blocks Without a Receipt Unless Overridden

Traces to: R-011

IF `tools/archive-anchor-gate` finds no persisted
`openspec/changes/<change>/review-receipts/<lineage>.json` for the change's
review lineage, THEN `archive-anchor-gate` SHALL reject any self-asserted
`approved_tree` value and SHALL block archive until either a receipt is
captured or an explicit owner override is recorded.

#### Scenario: Missing receipt blocks archive

- GIVEN a change with no persisted receipt file and an agent-supplied
  `approved_tree`
- WHEN `archive-anchor-gate` runs
- THEN it SHALL block archive with an explicit "no verified receipt" reason

#### Scenario: Recorded owner override permits archive

- GIVEN the same change with an explicit, recorded owner override
- WHEN `archive-anchor-gate` runs
- THEN it SHALL permit archive and SHALL record the override in the
  archive report
