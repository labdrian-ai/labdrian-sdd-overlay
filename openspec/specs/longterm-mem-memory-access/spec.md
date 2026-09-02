# Longterm-Mem Memory Access Specification

## Purpose

Defines the read-only foundation of the `longterm-mem` component: its
standalone module boundary, direct read access to Engram's mid-term memory,
and per-project resolution and invocation of the long-term vault. Covers the
no-CLI-shelling and query-scoping constraints on the Engram read path,
vault-registry resolution (explicit override, per-project default, and
rejection when unconfigured), and the vault query invoke/parse contract
including its not-provisioned failure mode.

## Requirements

### Requirement: Standalone Module Outside engine/

ID: R-001
Traces to: longterm-mem R-001

The longterm-mem component SHALL exist as a standalone module with its own
dependency manifest, located outside the `engine/` zero-dependency boundary.

#### Scenario: Component builds as an independent module

- GIVEN the labdrian-sdd-overlay repository
- WHEN longterm-mem is built
- THEN it compiles as an independent module under `longterm-mem/` capable of
  declaring third-party dependencies

#### Scenario: engine/'s zero-dependency gate stays green

- GIVEN longterm-mem's module manifest adds third-party dependencies
- WHEN `engine/skills`'s zero-fetch import-allowlist test runs
- THEN it still passes unmodified, and `engine/`'s own dependency manifest
  carries no third-party requirement

### Requirement: Read-Only Engram Connection

ID: R-002
Traces to: longterm-mem R-002

The longterm-mem component SHALL read Engram observations only via a
read-only connection to the Engram database, defaulting to Engram's standard
database location and overridable via configuration for tests.

#### Scenario: Default connection is read-only

- GIVEN no override is configured
- WHEN longterm-mem opens its Engram connection
- THEN it connects read-only to Engram's default database location and any
  write attempt on that connection fails

#### Scenario: Overridden connection stays read-only

- GIVEN a test fixture overrides the database path
- WHEN longterm-mem opens its Engram connection
- THEN it connects read-only to the overridden path instead

### Requirement: Mid-Term Query Scoping

ID: R-020
Traces to: longterm-mem R-020

WHEN mid-term observations are queried for project P, the longterm-mem
component SHALL return only observations where the record is not
soft-deleted and belongs to project P.

#### Scenario: Soft-deleted and other-project observations are excluded

- GIVEN a fixture set of active, soft-deleted, and other-project observations
- WHEN observations are queried for project P
- THEN only active observations belonging to P are returned

### Requirement: No CLI Shelling to Engram

ID: R-021
Traces to: longterm-mem R-021

The longterm-mem component SHALL NOT invoke Engram's `search` or `save` CLI
commands.

#### Scenario: No subprocess call to Engram's CLI

- GIVEN any longterm-mem read or write path that touches Engram
- WHEN it executes
- THEN no subprocess invocation of Engram's `search` or `save` command occurs

### Requirement: Vault Resolution From Configuration

ID: R-003
Traces to: longterm-mem R-003

WHEN a vault is resolved for project P and the vault-registry configuration
names a vault path for P, the longterm-mem component SHALL use that
configured path.

#### Scenario: Configured override is used

- GIVEN project `some-other-project` has a vault-registry entry pointing at
  a vault path
- WHEN its vault is resolved
- THEN longterm-mem uses that configured path

### Requirement: Default Vault for labdrian-sdd-overlay

ID: R-022
Traces to: longterm-mem R-022

WHERE project P is `labdrian-sdd-overlay` and no explicit override is
configured for P, the longterm-mem component SHALL resolve P's vault via a
pre-seeded default entry in the vault-registry configuration.

#### Scenario: Default resolves without a code-level constant

- GIVEN a vault-registry configuration with no override for
  `labdrian-sdd-overlay`
- WHEN its vault is resolved
- THEN longterm-mem resolves it via the pre-seeded default entry, and that
  entry is readable and editable like any other configuration row

### Requirement: Vault-Not-Configured Rejection

ID: R-023
Traces to: longterm-mem R-023

IF no vault is configured for project P and no default entry applies to P,
THEN the longterm-mem component SHALL reject the resolution with a
vault-not-configured error.

#### Scenario: Unconfigured, non-default project is rejected, not guessed

- GIVEN project `some-new-project` has no vault-registry entry and is not
  `labdrian-sdd-overlay`
- WHEN its vault is resolved
- THEN longterm-mem rejects the resolution with a vault-not-configured error
  rather than guessing a path

### Requirement: Vault Query Invoke and Parse

ID: R-004
Traces to: longterm-mem R-004

WHEN a long-term query is issued for project P, the longterm-mem component
SHALL invoke the vault's retrieval entrypoint inside P's resolved vault with
a top-N result count (overridable via configuration, default 5) and parse
the page address, absolute path, BM25 score, rerank score, and snippet from
its output.

#### Scenario: Default top-N and full field parse

- GIVEN project `labdrian-sdd-overlay` resolves to its provisioned vault
- WHEN a query is issued with no explicit top-N
- THEN longterm-mem invokes the vault's retrieval entrypoint with the
  default top-N and parses all five named fields from its output

#### Scenario: Explicit top-N override

- GIVEN an explicit top-N override
- WHEN a query is issued
- THEN longterm-mem passes that value instead of the default

### Requirement: Vault Query Not-Provisioned Handling

ID: R-024
Traces to: longterm-mem R-024

IF the vault's retrieval entrypoint exits with the not-provisioned status
code, THEN the longterm-mem component SHALL return a `not_provisioned`
status instead of treating it as an error.

#### Scenario: Never-indexed vault maps to not_provisioned, not a generic error

- GIVEN a resolved vault that has never been indexed
- WHEN it is queried
- THEN longterm-mem maps the resulting not-provisioned exit status to a
  `not_provisioned` result rather than a generic failure
