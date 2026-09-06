# Delta for Longterm-Mem Query

## MODIFIED Requirements

### Requirement: Unified Query Fan-Out and Merge

ID: R-006
Traces to: longterm-mem R-006

WHEN `query` is called with a required `project` argument and a query
string, the longterm-mem query function SHALL merge results across every
requested source by deterministic round-robin, preserving each source's own
native rank order as a subsequence of the merged list, deduplicated by
`engram_id` keeping the first occurrence, with rank 1 routed by the
token-shape gate (R-059). The vault-first clause narrows to WHERE the vault
source is requested: vault rows still precede Engram rows within that
request only. No result field SHALL be derived by combining a score from one
source with a score from another; source-native scores stay per-source and
nullable — this is interleaving, not fusion, and D8's ban on cross-source
score arithmetic is unchanged.

(Previously: results were returned as vault matches, all of them, ahead of
all Engram matches, unconditionally.)

#### Scenario: Round-robin interleave preserves each source's native order

- GIVEN `sources: ["engram-fts", "engram-embed"]` and both return five
  ranked rows for a query
- WHEN `query` is invoked
- THEN the merged list interleaves the two sources, and within it every
  source's own rows keep their relative order from that source's output

#### Scenario: A row found by both sources appears once, at its earliest rank

- GIVEN a row present in both requested sources' top results
- WHEN `query` is invoked
- THEN it is emitted once, at the earlier of its two positions, with
  `source` naming both

#### Scenario: Vault-first applies only when the vault is requested

- GIVEN `sources` includes `vault` alongside the Engram sources
- WHEN `query` is invoked
- THEN vault rows precede Engram rows, exactly as before, for that request
  only

#### Scenario: No cross-source score arithmetic

- GIVEN a merged result containing rows from more than one source
- WHEN the result is inspected
- THEN no field is computed from two sources' scores together; each row's
  score is null or attributed to exactly one source

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

## ADDED Requirements

### Requirement: Union Recall Guarantee

ID: R-058
Traces to: longterm-mem R-058

WHEN two or more sources are requested, the merged result set SHALL contain
every row present in any requested source's own top-`top` results, so that
recall of the merged set is at least the recall of whichever requested
source alone would have scored best on that query. This is a set-membership
property and SHALL hold independent of which source is routed to rank 1.

#### Scenario: Merged set contains both sources' top rows regardless of routing

- GIVEN two requested sources whose top-`top` results are disjoint
- WHEN `query` is invoked
- THEN the merged result set contains every row from both sources' top-`top`
  lists

#### Scenario: An incorrect rank-1 routing does not shrink the guarantee

- GIVEN the rank-1 gate (R-059) routes the wrong source to rank 1 for a
  given query
- WHEN `query` is invoked
- THEN the merged set still contains both sources' top-`top` rows; only
  which row occupies rank 1 is affected

### Requirement: Rank-1 Routing By Token-Shape Gate

ID: R-059
Traces to: longterm-mem R-059

WHEN more than one source is requested, the longterm-mem query function
SHALL route rank 1 to the Engram FTS source WHERE any query token is
identifier-shaped or the FTS match mode is `MatchAny` (a widened search is
itself evidence the precise reading found nothing, which leans lexical
rather than paraphrase), and to the embedding source otherwise. This gate
is a heuristic over query shape; it SHALL NOT compute or compare a
relevance score, and a wrong routing decision SHALL only affect which
source occupies rank 1, never the set guaranteed by R-058.

Shipped (Branch A) at a measured routing accuracy of 89% (identifier
queries, n=18 decidable) and 86% (paraphrase queries, n=7) against blind,
third-party-adjudicated ground truth — both in the 80–89% band, so the
number is published here rather than only in a decision document (see
`openspec/changes/union-retrieval/validation/phase0.md` and `score.md` for
the full protocol and the surviving-n accounting). Under the design's exact
embedding input shape (title‖NUL‖content, re-measured rather than assumed
after a harness/shape mismatch was found), the same gate scores 94%/88%.

#### Scenario: Identifier-shaped token routes FTS to rank 1

- GIVEN a query containing a token with an interior CamelCase boundary, a
  `/`, `_`, `(`, `)`, or a dotted `word.word`
- WHEN `query` is invoked with both sources requested
- THEN the Engram FTS source's top row occupies rank 1

#### Scenario: A misrouted gate still yields both sources' top rows

- GIVEN a query the gate routes to the wrong source for rank 1
- WHEN `query` is invoked
- THEN rank 1 is filled from the routed source's top row, and the merged
  set still contains the other source's top-`top` rows per R-058

### Requirement: Sources Selection Parameter

ID: R-060
Traces to: longterm-mem R-060

The longterm-mem query function SHALL accept an optional `sources`
parameter naming which sources to query, defaulting to `["engram-fts",
"engram-embed"]` when omitted, with `vault` queried only when named
explicitly.

#### Scenario: Omitted `sources` queries both Engram arms, not the vault

- GIVEN `query` is called with no `sources` field
- WHEN it is invoked
- THEN it queries `engram-fts` and `engram-embed` only; the vault is not
  invoked and no vault subprocess is spawned

#### Scenario: Naming the vault invokes it

- GIVEN `sources` includes `vault`
- WHEN `query` is invoked
- THEN the vault is queried and its rows are merged per R-006

### Requirement: Per-Response Index Coverage

ID: R-061
Traces to: longterm-mem R-061

WHEN the embedding source is requested, every response SHALL carry the
embedding index's coverage for project P — live rows, indexed-and-live
rows, and the difference — as a response field, not only inside a
diagnostic.

#### Scenario: Coverage is present even when it is zero

- GIVEN the embedding index covers every live row for P
- WHEN `query` is invoked with the embedding source requested
- THEN the response carries a coverage field reporting zero missing rows

#### Scenario: Coverage names a gap the harness itself could not see otherwise

- GIVEN the corpus grew after the index was last built, so N rows are live
  but unindexed
- WHEN `query` is invoked with the embedding source requested
- THEN the response's coverage field reports exactly N missing rows

### Requirement: Budget-Before-Render Snippet Allocation

ID: R-062
Traces to: longterm-mem R-062

WHEN the response byte ceiling is applied, the longterm-mem query function
SHALL allocate the available budget across the rows selected by the merge
before rendering their snippets, rather than rendering full-length snippets
and dropping rows afterward.

#### Scenario: More rows yield shorter snippets rather than fewer rows

- GIVEN a merge producing more rows than fit at full snippet length under
  the byte ceiling
- WHEN the response is built
- THEN every merged row is rendered with a shortened snippet rather than
  some rows being dropped to preserve full-length snippets on the rest

#### Scenario: A shortened snippet is marked truncated

- GIVEN a row rendered with a budget-shortened snippet
- WHEN the response is inspected
- THEN that row is marked truncated and its full content remains fetchable

### Requirement: Unbiased Response Cap

ID: R-063
Traces to: longterm-mem R-063

IF the response still exceeds the byte ceiling after R-062's allocation,
THEN the cap SHALL drop the lowest-ranked row of whichever source currently
holds the most slots, iteratively, rather than trimming from the end of the
merged list.

#### Scenario: The cap shrinks both sources together, not just the tail source

- GIVEN a response still over the byte ceiling after snippet allocation,
  with one source holding more slots than the other
- WHEN the cap is applied
- THEN rows are dropped from the source holding the most slots first, and
  the surviving rows keep their merged order and rank numbers
