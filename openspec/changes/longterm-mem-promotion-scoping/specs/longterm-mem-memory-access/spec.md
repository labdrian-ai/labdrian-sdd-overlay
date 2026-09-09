# Delta for longterm-mem-memory-access

## ADDED Requirements

### Requirement: Observation topic_key Exposure

Traces to: longterm-mem-promotion R-007 (Promotion Eligibility Predicate)

WHEN the longterm-mem component loads an Engram observation row through its
read-only Engram connection, it SHALL populate that observation's `topic_key`
field from the SQLite `observations.topic_key` column, mapping a SQL `NULL`
value to an empty string, so that eligibility evaluation (per
longterm-mem-promotion R-007) always receives a determinate value rather
than an unread or nil field.

#### Scenario: A row with a topic_key populates the loaded struct

- GIVEN an `observations` row whose `topic_key` column holds
  `longterm-mem/promotion-eligibility-policy`
- WHEN the row is loaded through the read-only Engram connection
- THEN the loaded observation's `topic_key` field carries that exact value

#### Scenario: A NULL topic_key column maps to an empty string

- GIVEN an `observations` row whose `topic_key` column is SQL `NULL`
- WHEN the row is loaded through the read-only Engram connection
- THEN the loaded observation's `topic_key` field is an empty string, not a
  nil or unset value

#### Scenario: Every observation-loading query path carries topic_key

- GIVEN any query path that loads observation rows for a project (mid-term
  query scoping per R-020, or a promotion/eligibility scan)
- WHEN observations are loaded
- THEN every returned observation struct carries its `topic_key` field
  populated per this requirement, with no query path bypassing it
