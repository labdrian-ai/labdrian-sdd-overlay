# Longterm-Mem Ingest Extraction Specification

## Purpose

Defines the local, non-network extraction and chunking helper inside
`longterm-mem` — work unit 2 of `longterm-mem-knowledge-ingestion`. This
capability accepts content an agent has already fetched or already has
locally (raw text, or a local file path) and returns structured, right-sized
ingestion units, each satisfying the `knowledge-ingestion` R-002 field
contract. It performs no network access of any kind: it depends on and
extends the read-only-Engram, zero-egress posture already enforced
elsewhere in `longterm-mem` (`net_allowlist_test.go`, R-071), and it
introduces no new dependency on `net` or `net/http`.

This capability governs the extraction/chunking entrypoint's input surface,
its zero-egress guarantee, its chunk boundary and chunk metadata, and the
fact that its output satisfies the ingested-observation contract. It does
not decide the concrete chunk-size threshold (OQ-3) — that is a
`sdd-design` decision that MUST be argued against the token/byte evidence
named in this spec's R-006, not an arbitrary number.

## Requirements

### Requirement: Local, Non-Network Extraction Entrypoint

ID: R-005
Traces to: longterm-mem-knowledge-ingestion R-005

The system SHALL provide a Go entrypoint inside `longterm-mem` that accepts
either already-fetched raw text or a local file path, and returns
structured ingestion units, without performing any network access. The
entrypoint's implementation SHALL NOT import `net` or `net/http` in any
non-test `.go` file, so that `longterm-mem/net_allowlist_test.go`'s static
AST-based allowlist guard (R-071) continues to hold at exactly one
allowlisted entry (`internal/embed/client.go`) after this capability ships.
The existing tests `TestNetImportAllowlist` and
`TestNetImportAllowlistStillRefusesOthers` SHALL continue to pass unmodified.

#### Scenario: The entrypoint accepts raw text

- GIVEN already-fetched text handed in-process (no file path)
- WHEN the extraction entrypoint is called with that text
- THEN it returns one or more structured ingestion units derived from the
  text
- AND no network call was made during the call

#### Scenario: The entrypoint accepts a local file path

- GIVEN a path to a local text file
- WHEN the extraction entrypoint is called with that path
- THEN it reads the file from local disk and returns one or more structured
  ingestion units derived from its content
- AND no network call was made during the call

#### Scenario: The zero-egress guarantee is enforced by the existing static guard

- GIVEN the full set of non-test `.go` files added or modified by this
  capability
- WHEN `longterm-mem/net_allowlist_test.go`'s AST-based import guard runs
- THEN no file introduced or modified by this capability imports `net` or
  `net/http`
- AND `TestNetImportAllowlist` and `TestNetImportAllowlistStillRefusesOthers`
  both pass without modification to their own assertions
- AND the allowlist recognised by the guard still holds exactly one entry
  (`internal/embed/client.go`)

#### Scenario: `go vet` and `go test` pass for the new package

- GIVEN the new extraction package and its tests
- WHEN `cd longterm-mem && go vet ./... && go test ./...` is run
- THEN both commands complete successfully

### Requirement: Deterministic Chunking With A Stated, Evidenced Boundary

ID: R-006
Traces to: longterm-mem-knowledge-ingestion R-006

When the input content exceeds the size of one ingestion unit, the
extraction entrypoint SHALL split it into multiple chunks at a defined,
tested boundary rule. Each chunk SHALL carry its position within the
originating document (e.g. an index or offset) and a shared source identity
common to every chunk of that document, so the full set of chunks belonging
to one document is recoverable as one document from the chunks alone. The
concrete size/boundary threshold used by the boundary rule SHALL be
evidenced against `internal/query/query.go`'s existing query response
budget — `ResponseTokenCeiling = 2000` tokens (`bytesPerToken = 4`, ~8000
bytes total), `MinSnippetBudget = 120` bytes per row, and the default top-N
of 5 results — rather than chosen as an arbitrary number. A chunk boundary
whose chosen threshold cannot be traced to that evidence, or to an
equivalent evidenced constraint recorded at design time, does not satisfy
this requirement.

#### Scenario: Content at or under one unit is not split

- GIVEN input content whose size is at or under the one-unit threshold
- WHEN the extraction entrypoint processes it
- THEN exactly one ingestion unit is returned
- AND that unit's position is the first (and only) position for its source

#### Scenario: Content just over the threshold produces multiple chunks

- GIVEN input content whose size is just over the one-unit threshold
- WHEN the extraction entrypoint processes it
- THEN more than one ingestion unit is returned
- AND each returned unit is at or under the one-unit threshold

#### Scenario: Chunks of one document carry a shared source identity and position

- GIVEN a document large enough to be split into N chunks (N > 1)
- WHEN the extraction entrypoint processes it
- THEN all N chunks carry the same shared source identity value
- AND each chunk carries a distinct position value
- AND the N chunks, ordered by position, reconstruct the document's
  contiguous coverage with no gap or overlap left unaccounted for

#### Scenario: The chunk-size threshold is traceable to the query budget evidence

- GIVEN the concrete chunk-size threshold chosen at design time
- WHEN that threshold is inspected against `internal/query/query.go`'s
  `ResponseTokenCeiling`, `MinSnippetBudget`, and default top-N constants
- THEN the threshold's justification references those constants (or an
  equivalent recorded, evidenced constraint), not an unexplained fixed
  number
- AND a chunk sized at the threshold does not, by itself, already exceed
  `ResponseTokenCeiling` when converted to bytes at `bytesPerToken = 4`

### Requirement: Extraction Output Satisfies The Ingested-Observation Contract

ID: R-007
Traces to: longterm-mem-knowledge-ingestion R-007

Every ingestion unit emitted by the extraction entrypoint — whether the
document produced one unit or many — SHALL carry the full
`knowledge-ingestion` R-002 field set (origin, ingestion timestamp, and the
ingested-vs-session marker), so that an agent can pass an emitted unit
directly to `mem_save` without separately inventing or back-filling any of
that metadata. For a multi-chunk document, every chunk SHALL carry the same
origin and ingested-vs-session marker as every other chunk of that document,
differing only in whatever fields distinguish the chunk itself (position,
shared source identity, chunk content).

#### Scenario: A single-unit extraction output is `mem_save`-ready

- GIVEN input content that produces exactly one ingestion unit
- WHEN that unit is inspected
- THEN it carries an origin value, an ingestion timestamp, and an
  ingested-vs-session marker
- AND an agent can call `mem_save` with that unit's fields without adding
  any missing R-002 field itself

#### Scenario: Every chunk of a multi-chunk extraction carries the full field set

- GIVEN input content that produces N chunks (N > 1)
- WHEN each of the N chunks is inspected
- THEN each chunk independently carries an origin value, an ingestion
  timestamp, and an ingested-vs-session marker
- AND the origin value and ingested-vs-session marker are identical across
  all N chunks
- AND no chunk requires an agent to supply a missing R-002 field before
  calling `mem_save`

#### Scenario: A unit missing any R-002 field fails this requirement

- GIVEN an ingestion unit emitted by the extraction entrypoint
- WHEN any one of origin, ingestion timestamp, or ingested-vs-session
  marker is absent from that unit
- THEN the unit does not satisfy this requirement, regardless of whether its
  chunk position and source identity are otherwise correct
