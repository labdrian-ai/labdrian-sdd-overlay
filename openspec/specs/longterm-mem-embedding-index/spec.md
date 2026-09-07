# Longterm-Mem Embedding Index Specification

## Purpose

Defines the embedding index over Engram's own observations for project P:
where it lives and its format, fingerprint-based staleness detection,
per-response coverage reporting, its explicit build lifecycle, named
degradation when the embedding backend is unavailable, and the network
egress boundary its HTTP dependency introduces.

## Measured Rank-1 Routing Accuracy

The rank-1 routing gate this index feeds (`longterm-mem-query`'s R-059) is
shipped live (Branch A), not fixed FTS-first order, on blind,
third-party-adjudicated validation: **89%** routing accuracy on identifier
queries (n=18 decidable of 20 authored) and **86%** on paraphrase queries
(n=7 decidable of 20 authored) — both in the 80–89% band the design's own
threshold table requires publishing here rather than only in a decision
document. Re-measured under the design's exact embedding input shape
(`title‖NUL‖content[:limit]`), the same gate scores **94%**/**88%**.

Both numbers describe the gate's own job — of the queries where at least
one arm holds the ground truth at its own rank 1, how often the gate routed
to that arm — not the union's `@1`/`@5` hit rates, which is a different
question the gate does not answer and R-058's set-membership guarantee does
not depend on. Full protocol, adjudication accounting, and the metric this
supersedes are in `openspec/changes/union-retrieval/validation/phase0.md`
and `score.md`.

**The paraphrase n (7-9 decidable, depending on measurement) is too small
for the design's own 80% acceptance threshold to be reliably resolved by a
single re-measurement.** One query is worth 12-14 percentage points at
this sample size; two independently honest measurements of the shipped
gate against the same frozen blind set landed at 77.8% and 87.5%,
straddling 80%, differing only in embedding-index freshness at
measurement time (`validation/score.md`, "PR-4: the predicted failure
happened"). The 86%/88% figures are real, not estimated — but they should
be read as "well clear of a defective gate measured at ~22% on this same
set," not as a value that would reproduce to the point. Widening the
blind paraphrase set is open debt before this threshold is relied on
again.

## Requirements

### Requirement: Embedding Index Location and Format

ID: R-066
Traces to: longterm-mem R-066

The longterm-mem component SHALL store project P's embedding index under
`<state-dir>/index/<project-dir>/` as a fixed-stride vector blob plus a
self-digesting JSON manifest recording model, dimension, input character
limit, per-row fingerprints, and build time. The index is this module's own
state, never a modification to Engram's database or the vault.

#### Scenario: Index files live under the state directory, not the vault

- GIVEN project P's embedding index is built
- WHEN its files are located
- THEN they are found under `<state-dir>/index/<project-dir>/`, never
  inside any vault path

#### Scenario: A corrupted manifest is reported, not silently trusted

- GIVEN the manifest's recorded revision does not match a digest of its
  entries
- WHEN the index is loaded
- THEN loading fails with a corruption error rather than serving stale or
  invalid vectors

### Requirement: Engram's Database Stays Read-Only

ID: R-067
Traces to: longterm-mem R-067

Building or querying the embedding index SHALL NOT write to Engram's
database. All persistent state the index requires SHALL live in the
index's own files under the state directory.

#### Scenario: An index build performs no write against Engram

- GIVEN an embedding index build for project P
- WHEN it runs to completion
- THEN Engram's read-only connection contract (R-002) is unchanged and no
  write is attempted against it

### Requirement: Fingerprint-Based Row Staleness And Coverage

ID: R-068
Traces to: longterm-mem R-068

WHEN a row is returned by the embedding source, the longterm-mem component
SHALL recompute its fingerprint from the observation's current model,
dimension, input limit, title, and truncated content, and SHALL drop the
row from the response IF that fingerprint does not match the indexed one.
Separately, every response using the embedding source SHALL report corpus
coverage: the count of live rows for P not present in the index under a
matching fingerprint.

#### Scenario: A changed observation is dropped, not returned stale

- GIVEN an indexed row whose observation content changed since indexing
- WHEN it would be returned by the embedding source
- THEN it is dropped from the response and a stale-row count is reported

#### Scenario: An unindexed row is visible as a coverage gap, not silence

- GIVEN the corpus holds rows saved after the index was last built
- WHEN a query using the embedding source runs
- THEN the response reports the number of live, unindexed rows as coverage,
  even though those rows cannot be returned

#### Scenario: A soft-deleted row never surfaces even from a stale index

- GIVEN an indexed row whose observation was later soft-deleted
- WHEN it would be returned by the embedding source
- THEN it is excluded, because R-020's read-time scoping applies to the
  fetch that renders it regardless of the index's own staleness

### Requirement: Explicit Build Lifecycle

ID: R-069
Traces to: longterm-mem R-069

The longterm-mem component SHALL build or update the embedding index only
via an explicit `index --embeddings` invocation. It SHALL NOT build the
index lazily during a query and SHALL NOT build it from a background
process. An incremental build SHALL re-embed only rows whose fingerprint
is missing or changed, and SHALL remove entries for rows no longer live.

#### Scenario: A query never triggers a build

- GIVEN no embedding index exists for P
- WHEN a query using the embedding source runs
- THEN no index build is triggered; the query degrades per R-070 instead

#### Scenario: Incremental build re-embeds only changed rows

- GIVEN an existing index and a corpus with a small number of new or
  changed rows since the last build
- WHEN `index --embeddings` runs
- THEN only the new or changed rows are embedded, and rows no longer live
  are removed from the index

### Requirement: Named Embedding Backend Degradation

ID: R-070
Traces to: longterm-mem R-070

IF the embedding backend is unreachable or reachable without the required
model pulled, THEN the longterm-mem component SHALL return Engram-FTS-only
results plus a diagnostic naming which of the two conditions applies, in
query-consequence terms. It SHALL NOT return FTS-only results without that
diagnostic present.

#### Scenario: An unreachable backend is named, not silently downgraded

- GIVEN the embedding backend does not answer
- WHEN a query requesting the embedding source runs
- THEN it returns FTS-only results plus an `embedding_backend_unreachable`
  diagnostic naming that paraphrased queries cannot be answered right now

#### Scenario: A reachable backend without the model is named distinctly

- GIVEN the backend answers but the required model is not pulled
- WHEN a query requesting the embedding source runs
- THEN it returns FTS-only results plus an `embedding_model_missing`
  diagnostic, distinct from the unreachable case

#### Scenario: FTS-only results never look like a working union

- GIVEN either degradation condition
- WHEN the response is inspected
- THEN it carries the naming diagnostic; a caller cannot mistake it for a
  complete union response

### Requirement: Network Egress Allowlist For The Embedding Client

ID: R-071
Traces to: longterm-mem R-071

Exactly one file SHALL import `net/http` in the longterm-mem module,
enforced by an allowlist test sibling to the exec allowlist. The embedding
client SHALL default to a loopback endpoint, SHALL refuse any non-loopback
endpoint unless explicitly opted in per invocation, and SHALL refuse every
HTTP redirect.

#### Scenario: A second file importing net/http fails the allowlist check

- GIVEN a production file other than the embedding client imports
  `net/http`
- WHEN the allowlist test runs
- THEN it fails, naming the unauthorized file

#### Scenario: A non-loopback endpoint is refused, not warned about

- GIVEN no explicit opt-in and a configured non-loopback embedding endpoint
- WHEN the embedding client is invoked
- THEN the call is refused before any request is sent, and no warning-only
  path exists

#### Scenario: A redirect is refused

- GIVEN the embedding endpoint responds with a redirect
- WHEN the embedding client follows up
- THEN the redirect is refused and the call fails rather than following it
