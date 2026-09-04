# Longterm-Mem Query Specification

## Purpose

Defines the index lifecycle CLI that owns vault re-indexing — including
first-time provisioning of a never-indexed vault — and the unified,
library-level `query` function that fans out to both memory tiers, merges
results by source, and degrades gracefully when the long-term tier is
unavailable.

## Requirements

### Requirement: Index Rebuild With First-Provision

ID: R-005
Traces to: longterm-mem R-005

WHEN an index rebuild is requested for project P, the longterm-mem component
SHALL rebuild P's resolved vault index — provisioning it first if the vault
has never been indexed — before reporting success.

#### Scenario: Already-provisioned vault refresh

- GIVEN a resolved vault with new or changed pages that is already
  provisioned
- WHEN an index rebuild is requested for it
- THEN the vault's index state reflects those pages after the command exits
  successfully

#### Scenario: Never-indexed vault is provisioned first

- GIVEN a resolved vault that has never been indexed (a query against it
  would report not-provisioned)
- WHEN an index rebuild is requested for it
- THEN the vault is provisioned before the refresh runs, and the vault is
  queryable (no longer not-provisioned) afterward

### Requirement: Index Rebuild Subprocess Failure Handling

ID: R-025
Traces to: longterm-mem R-025

IF the index-rebuild step fails during a requested rebuild, THEN the rebuild
SHALL fail without reporting the index as rebuilt.

#### Scenario: Failing rebuild step reports failure, not false success

- GIVEN a fixture where the index-rebuild step is forced to fail
- WHEN an index rebuild is requested
- THEN the command reports failure and its output does not claim success

### Requirement: Unified Query Fan-Out and Merge

ID: R-006
Traces to: longterm-mem R-006

WHEN `query` is called with a required `project` argument and a query
string, the longterm-mem query function SHALL return one result list grouped
by source — vault matches first in the vault's own native rank order, then
Engram matches in Engram's own native rank order — with any result pair
sharing a promotion link emitted once, carrying both references.

#### Scenario: Results are grouped by source in native rank order

- GIVEN a query string and a `project` value
- WHEN `query` is invoked
- THEN results appear as vault matches (in vault rank order) followed by
  Engram matches (in Engram rank order), each tagged with its source

#### Scenario: Linked pair is emitted once

- GIVEN a vault result and an Engram result connected by an existing
  promotion link
- WHEN `query` is invoked
- THEN they are emitted as a single linked result carrying both references,
  not as two separate entries

#### Scenario: Missing project argument is rejected

- GIVEN `query` is called without a `project` argument
- WHEN it is invoked
- THEN the call is rejected as invalid rather than falling back to any
  inferred project

### Requirement: Query Not-Provisioned Degrade Path

ID: R-026
Traces to: longterm-mem R-026

IF the resolved vault is not provisioned for `project`, THEN the
longterm-mem query function SHALL return Engram-only results tagged
`vault_status: not_provisioned` instead of failing the call.

#### Scenario: Unprovisioned vault degrades instead of failing the whole call

- GIVEN a project whose vault has never been indexed
- WHEN `query` is invoked
- THEN it returns Engram-only results plus `vault_status: not_provisioned`,
  with no error raised
