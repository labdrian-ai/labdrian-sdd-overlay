# Knowledge Ingestion Specification

## Purpose

Defines the ingestion convention by which external knowledge — a
documentation page, a specification, a pasted report, a local file — enters
`longterm-mem`'s long-term memory. This is the first work unit of
`longterm-mem-knowledge-ingestion`: it introduces no new Go code and no new
storage surface. It pins down the one sanctioned intake path (agent
fetch/read, then Engram `mem_save`, then `longterm-mem promote`), the field
contract an ingested observation MUST carry, the requirement that promotion
happens as part of ingestion rather than depending on a later automatic
sync, and how re-ingesting the same source is recognisable rather than
silently duplicated.

This capability governs the convention and its observable outcomes only. It
does not decide, and MUST NOT be read as deciding, where the convention
artifact lives structurally (OQ-1), the exact marker mechanism used to
distinguish an ingested observation (OQ-2), the ingestion topic-key
namespace (OQ-4), or the precise re-ingestion-on-change behavior (OQ-5).
Those are `sdd-design` decisions; this spec states the properties any of
those decisions MUST satisfy.

## Requirements

### Requirement: One Documented Ingestion Procedure

ID: R-001
Traces to: longterm-mem-knowledge-ingestion R-001

The system SHALL provide a durable, agent-discoverable repository artifact
that documents external-knowledge intake as exactly one procedure: agent
fetch or read of the source, followed by an Engram `mem_save` call on the
extracted content, followed by an explicit `longterm-mem promote` call (CLI
or MCP tool) on the resulting observation. The artifact SHALL state that no
other intake path exists for external knowledge. The artifact SHALL be
registered and propagated through the existing repository lifecycle for
durable procedural artifacts (e.g. the skill lifecycle and
`skills.registry.yaml`), so it survives a session boundary and is
discoverable by a future agent turn without depending on that turn's own
memory of this change.

#### Scenario: The procedure is discoverable in a fresh session

- GIVEN a new agent session with no memory of this change
- WHEN the agent is asked to ingest an external document into long-term
  memory
- THEN the agent can discover the documented procedure through the existing
  repository/skill lifecycle
- AND the discovered procedure names fetch/read, then `mem_save`, then
  `promote`, in that order, as the complete intake path

#### Scenario: The procedure names itself as the only intake path

- GIVEN the documented ingestion artifact
- WHEN it is read
- THEN it states that no ingestion path other than fetch/read → `mem_save`
  → `promote` exists for external knowledge
- AND it does not describe or imply any Go-side or network-side ingestion
  command

#### Scenario: Content that skips `mem_save` is not "ingested"

- GIVEN a piece of external content that an agent has fetched or read but
  for which no `mem_save` call was made
- WHEN the state of long-term memory is inspected for that content
- THEN no Engram observation exists for it
- AND no vault page exists for it
- AND that content is therefore not considered ingested by this capability,
  regardless of whether the agent's own conversational context contains it

### Requirement: Ingested-Observation Field Contract

ID: R-002
Traces to: longterm-mem-knowledge-ingestion R-002

Every observation produced by the documented ingestion procedure SHALL carry,
and SHALL make recoverable from the stored observation:

1. Its origin — a source URL, a local file path, or an explicit marker
   recording that the content was pasted by the operator with no other
   locatable source.
2. Its ingestion timestamp — when the ingestion `mem_save` call was made.
3. A marker that distinguishes the observation as ingested external
   knowledge, as opposed to an ordinary session observation an agent wrote
   about its own work. The concrete mechanism for this marker (an Engram
   `type` value, a `topic_key` convention, a content-body convention, or a
   combination) is a design decision (OQ-2); this requirement constrains
   only that the marker MUST exist and MUST be recoverable, not how it is
   implemented.

#### Scenario: Origin, timestamp, and ingested marker are all recoverable

- GIVEN an observation produced by the documented ingestion procedure
- WHEN the observation is read back (via `mem_get_observation` or
  equivalent)
- THEN its origin (source URL, local path, or explicit pasted-by-operator
  marker) is present and recoverable
- AND its ingestion timestamp is present and recoverable
- AND a marker distinguishing it from an ordinary session observation is
  present and recoverable

#### Scenario: An observation missing the origin field does not satisfy the contract

- GIVEN an observation saved through `mem_save` with no origin recorded
- WHEN it is checked against the ingested-observation field contract
- THEN it fails the contract
- AND it MUST NOT be treated as a conforming ingestion by this capability,
  even if it was later promoted

#### Scenario: Pasted content with no locatable source still satisfies the contract

- GIVEN content pasted directly by the operator with no URL or file path of
  its own
- WHEN it is ingested through the documented procedure
- THEN its origin field carries the explicit "pasted by the operator" marker
  rather than being left empty or omitted

### Requirement: Promotion Is Part Of Ingestion

ID: R-003
Traces to: longterm-mem-knowledge-ingestion R-003

The documented ingestion procedure SHALL include an explicit `longterm-mem
promote` call (CLI `promote --id N` or the `promote` MCP tool) for every
observation it saves, so that ingested knowledge reaches the vault
deterministically as part of the same ingestion turn, rather than depending
on whether a later automatic `sync` finds the observation eligible. When
ingestion produces more than one observation (e.g. a multi-chunk document),
the procedure SHALL promote each resulting observation individually, since
`promote` accepts exactly one observation id per call.

#### Scenario: A single-observation ingestion is promoted in the same turn

- GIVEN an agent ingests a source that produces exactly one observation
- WHEN the ingestion procedure completes
- THEN an explicit `promote` call has been made for that observation's id
- AND a corresponding vault page exists, reachable without waiting for a
  later `sync`

#### Scenario: A multi-observation ingestion promotes every observation

- GIVEN an agent ingests a source that produces N observations (N > 1)
- WHEN the ingestion procedure completes
- THEN N separate `promote` calls have been made, one per observation id
- AND every one of the N observations has a corresponding vault page

#### Scenario: Reaching the vault does not depend on automatic sync eligibility

- GIVEN an ingested observation whose `topic_key` (per the design's OQ-4
  resolution) would not, on its own, make it automatically eligible under
  `longterm-mem-promotion` R-007's curated-prefix rule
- WHEN the documented ingestion procedure is followed
- THEN the observation still reaches the vault, because the procedure's own
  explicit `promote` call supplies eligibility regardless of the automatic
  `topic_key` rule

### Requirement: Re-Ingestion Of The Same Source Is Recognisable

ID: R-004
Traces to: longterm-mem-knowledge-ingestion R-004

Ingesting the same source a second time SHALL be detectable from the stored
observations, using a stable identity derived from the source's origin
(R-002's origin field). The system SHALL rely on Engram's existing
`topic_key` upsert behavior and the existing `(project, engram_id)` page
dedup already used by promotion; this capability SHALL NOT introduce a new
dedup store, index, or identity scheme. The exact resolved behavior on
re-ingestion of changed content — update-in-place versus write-new-and
-supersede — is a design decision (OQ-5); this requirement constrains only
that the second ingestion is recognisable and reconciled through existing
Engram/vault mechanisms, never silently duplicated as an unrelated second
memory.

#### Scenario: Re-ingesting an unchanged source does not create an unrelated second memory

- GIVEN a source was already ingested once, producing an observation with a
  stable origin-derived identity
- WHEN the same source is ingested again with unchanged content
- THEN the second ingestion resolves to the same stable identity as the
  first
- AND no independent, unrelated second observation is created for that
  source

#### Scenario: Re-ingesting a changed source is reconciled, not silently duplicated

- GIVEN a source was already ingested once
- WHEN the same source is ingested again with changed content
- THEN the resolved behavior (update-in-place or write-new-and-supersede,
  per the design's OQ-5 decision) is applied
- AND the stored observations make it possible to tell that the second
  ingestion is a re-ingestion of the same source, rather than an unrelated
  new memory

#### Scenario: No second durability store is introduced for re-ingestion identity

- GIVEN the re-ingestion identity mechanism as implemented
- WHEN its storage is inspected
- THEN the only identity/dedup mechanisms in use are Engram's `topic_key`
  upsert and the existing `(project, engram_id)` vault-page dedup
- AND no new SQLite database, file-based index, or other store outside
  those two existing mechanisms has been introduced
