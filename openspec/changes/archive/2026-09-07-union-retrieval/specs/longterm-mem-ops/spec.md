# Delta for Longterm-Mem Ops

## ADDED Requirements

### Requirement: Embedding Index Diagnostic Checks

ID: R-064
Traces to: longterm-mem R-064

WHEN doctor is requested for project P, the longterm-mem component SHALL
additionally run three read-only checks over the embedding index —
`embedding-index-present`, `embedding-index-fresh`, and
`embedding-backend-reachable` — and report each independently, alongside
R-011's five existing checks.

#### Scenario: Missing index is named, not silently skipped

- GIVEN no embedding index has ever been built for P
- WHEN doctor runs
- THEN `embedding-index-present` reports failed, naming the missing index

#### Scenario: A stale index is named with its coverage gap

- GIVEN the embedding index is missing N live rows for P
- WHEN doctor runs
- THEN `embedding-index-fresh` reports failed, naming N

#### Scenario: An unreachable backend is distinguished from a missing model

- GIVEN the embedding backend does not answer
- WHEN doctor runs
- THEN `embedding-backend-reachable` reports failed by that name, distinct
  from a reachable backend that lacks the required model

### Requirement: Embedding Index Status Field

ID: R-065
Traces to: longterm-mem R-065

WHEN `status` is requested for project P, the longterm-mem component SHALL
additionally report the embedding index's last build time as
`embedding_index_built_at`, or the literal `never` when no index has been
built for P, alongside R-010's existing fields.

#### Scenario: A built index reports its build time

- GIVEN an embedding index built for P
- WHEN status is requested
- THEN `embedding_index_built_at` reports that build's timestamp

#### Scenario: Never-built reports never, not a fabricated timestamp

- GIVEN no embedding index has ever been built for P
- WHEN status is requested
- THEN `embedding_index_built_at` reports `never`
