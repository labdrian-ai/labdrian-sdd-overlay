# Design: Union Retrieval Over Engram's Own Rows

Claim tags: **[M]** measured, **[C]** read from code (path:line), **[A]** assumed.
This document **formalises** `openspec/decisions/union-retrieval.md`; it does not
re-derive it. Every decision there survives. §"Where I complete the record"
names the two places I add to it and the one I would argue with.

> **Size note.** The `sdd-design` 800-word budget is deliberately exceeded. The
> orchestrator required five named mechanisms specified to test-name granularity
> plus a slicing decision; a procedurally unspecified mitigation is a hope.

## Technical Approach

Two retrievers over one corpus, merged so the result set **contains both arms'
top-5**. Recall@merged ≥ max(recall@5 of either arm) holds by set union, for
every query, unconditionally — it is not an experimental result. Rank 1 is the
only contested slot and is decided by a routing gate that **degrades softly**: a
wrong gate costs @1 and leaves @5 untouched.

Everything below exists to stop that structural guarantee being destroyed
downstream — by a byte ceiling, a stale index, a soft-deleted row, or a
tokenizer that differs from the one that ships.

## Architecture Decisions

Decisions carried unchanged from `openspec/decisions/union-retrieval.md`
(rationale there, not repeated): index at `<state-dir>/index/<project>/` §1;
fixed-stride blob + self-digesting JSON, no SQLite writer, no ANN §1; fingerprint
over the exact embedded bytes including `model‖dim‖input_limit` §2.1; explicit
`index --embeddings`, never lazy, never background §2.4; round-robin interleave
with per-source subsequence order and dedup by `engram_id` §4.2; R-006 amended
rather than `mergeResults` patched §4.1; vault retired from the default path, not
deleted §6.

### Decision: production's own tokenizer is exported, not re-implemented

**Choice**: extract `searchTokens` (`internal/engram/search.go:272`) **[C]** into
an exported `engram.SearchTokens`, and have the Go golden harness call it.

**Alternatives considered**: (a) port `evaluate.py`'s
`re.findall(r"[A-Za-z0-9_./-]+")` to Go and test it against `strings.Fields`;
(b) leave a comment telling the porter which to use.

**Rationale**: the hazard is drift between the harness and the shipped
retriever. A test detects drift; a shared function makes drift *impossible*, and
this repository prefers the design that removes a hazard over the one that
handles it (the frozen package's best decision was to stop tracking two derivable
files, not to merge them). **The tokenizer difference changes no number in any
class on the current query set — I re-checked this and it is [M], not inferred**,
so this is a forward-looking hazard, not a correction to the published table.

**RED test**: `TestSearchTokensSplitsOnFieldsNotPunctuation` in
`internal/engram/tokens_test.go`. Input `"search.go:181"`; want exactly one
token. Red because `engram.SearchTokens` does not exist. A second case
`"a/b c"` → `["a/b","c"]` pins the regex/Fields divergence point.
`TestUnionGoldenUsesProductionTokenizer` asserts the harness routes through it.

### Decision: the snippet budget is allocated per ROW, not per source

**Choice**: before any snippet is rendered, measure the encoded `Result` with all
rows present, all diagnostics attached, all `Standing` objects attached, and
**empty snippets**. That is `overhead` — measured, not estimated, the same
discipline `capResponse` already applies (`internal/query/query.go:313-316`)
**[C]**. Then:

```
available = ResponseByteCeiling - overhead
share     = clamp(available / n, MinSnippetBudget=120, SnippetBudget=480)
render every row at share
leftover  = available - sum(rendered)                      # short bodies
share2    = clamp(share + leftover/len(stillTruncated), …, 480)
re-render ONLY the still-truncated rows at share2           # exactly one pass
```

**Alternatives considered**: (a) divide the ceiling per *source* and hand a
source its own quota; (b) render at 480 and let `capResponse` drop rows (today);
(c) iterate the redistribution to convergence.

**Rationale**: (a) is what the orchestrator asked me to specify, and the right
answer is to **dissolve the question**. A per-source share makes "what happens
when a source's share is unused" load-bearing, and any rule for reclaiming it is
a comparison between sources performed at render time — adjacent to the
arithmetic D8 forbids. Per-row allocation makes an unused source share
structurally undefined: rows compete only with rows. (b) is the current defect —
the ceiling binds on row count, and row count is exactly what the union buys.
(c) is a loop whose termination depends on data; one pass is deterministic and
bounded, and a second pass recovers a shrinking remainder.

`MinSnippetBudget = 120` completes rather than contradicts the record's §5: below
~120 characters a snippet cannot carry the sentence a match sits in, so at large
`top` the ceiling correctly binds on row count again and `capResponse` fires.
480's own derivation (`search.go:29-39`) **[C]** is 2 sources × 5 rows at 4
bytes/token; 120 is that same arithmetic at 20 rows.

`Row.Content` is already selected (`search.go:177,195`) **[C]**, so re-rendering
costs no query. It needs the match offset: add `Row.MatchOffset int` (already
computed inside `extract`) and export `engram.SnippetAt(content, offset, budget)`.
**Embedding-arm rows have no lexical locus and take offset 0 — a head slice.**
That is honest: a cosine match has no position in the text, and inventing one
would be a claim the vector cannot support.

`capResponse` stays as the backstop and drops the lowest-ranked row of whichever
source holds the most slots, iteratively. Removing rows is not re-ranking;
survivors keep merged order and rank numbers, as today **[C]**.

**RED tests** (`internal/query/budget_test.go`):
`TestSnippetBudgetIsAllocatedBeforeRender` — 10 rows with 40 KB bodies; want
`len(Results) == 10` and **no** `response_capped` diagnostic. Red today: current
code renders at 480 then trims the tail.
`TestUnusedSnippetShareIsRedistributedExactlyOnce` — mixed short/long bodies;
want long rows above `share` and total under ceiling; a body just under `share2`
must not trigger a third render.
`TestSnippetShareIsPerRowNotPerSource` — 8 embed rows and 2 FTS rows; want all
ten shares equal.
`TestCapResponseDropsFromTheLargestSourceNotTheTail` — red today.

### Decision: coverage is a response field, not a diagnostic

**Choice**:

```go
type Coverage struct {
    Source    string `json:"source"`     // "engram-embed"
    Live      int    `json:"live"`       // M, one COUNT(*) … deleted_at IS NULL
    Indexed   int    `json:"indexed"`    // N_live
    Unindexed int    `json:"unindexed"`  // M - N_live, precomputed
    BuiltAt   string `json:"built_at"`   // RFC3339 or the literal "never"
}
// on Result — NOT omitempty:
Coverage []Coverage `json:"coverage"`
```

One entry per requested source that depends on an index **this module owns** —
today that is `engram-embed` only. FTS gets no entry: Engram maintains
`observations_fts` and we would be reporting an assumption **[A]** in the shape of
a measurement. When `engram-embed` is not requested there is no entry, and
`embedding_source_excluded` says why the results are lexical-only.

**Alternatives considered**: report coverage only in `doctor`; report it as a
diagnostic; omit it when complete.

**Rationale**: the caller acting on the answer is the one who needs it, and
`doctor` is not in that path — the same argument `DiagnosticSearchWidened` makes
about itself (`query.go:64-74`) **[C]**. It is a field rather than a diagnostic
because a diagnostic is a list a caller may not iterate, while an absent field and
"full coverage" must never look alike — hence **no `omitempty`**. `Unindexed` is
precomputed even though it is `Live - Indexed`: a caller who forgets the
subtraction gets a silent wrong answer, and a silent wrong answer is this
module's unforgivable failure. This condition was observed in the wild — the
corpus grew 584 → 586 mid-run and the index answered two rows stale **[M]**.

**What a caller does with it** (goes in the capability spec, RFC-2119):

| state | the caller MUST read it as |
|---|---|
| `Unindexed == 0` | absence from results is evidence of absence, to the degree FTS allows |
| `Unindexed > 0` | a thin or empty paraphrase result is **not** evidence of absence; the fix is `longterm-mem index --embeddings`, named verbatim in the detail |
| `BuiltAt == "never"` | paraphrase questions are unanswerable now; these results are lexical-only |

**RED tests** (`internal/query/coverage_test.go`):
`TestResponseCarriesEmbeddingCoverageWhenSourceRequested`;
`TestCoverageIsPresentEvenWhenIndexIsComplete` (guards an `omitempty`
regression — this is the test that would catch someone "tidying" the struct);
`TestIncompleteCoverageDetailNamesTheRebuildCommand`.

### Decision: the egress guard refuses hostnames, not just non-loopback addresses

**Choice**: `longterm-mem/net_allowlist_test.go`, `package guard`, a direct
sibling of `exec_allowlist_test.go`. Generalise its walk to
`findImporters(root, importPath)`; keep `findOSExecImporters` as a thin wrapper so
`TestOSExecImportAllowlistCatchesTestdataPackage` is untouched **[C]**.

```go
// Adding a second entry is a change to the egress boundary, not a
// convenience — the same seriousness as R-021's exec allowlist.
var allowedNetImporters = map[string]bool{"internal/embed/client.go": true}
var guardedImports = []string{"net/http", "net"}
```

`net` is guarded alongside `net/http` because raw `net.Dial` is the same boundary
and is precisely the workaround that defeats an `http`-only guard. `net/url` is
deliberately **not** guarded: it parses and cannot egress.

**`internal/embed/client.go` — may and may not:**

| MAY | MAY NOT |
|---|---|
| construct exactly one `*http.Client` with an explicit timeout | be constructed without a timeout |
| POST to the configured embedding endpoint | follow **any** redirect (`CheckRedirect` returns an error) |
| parse the embedding response | reach a non-loopback endpoint absent `--allow-remote-embedder` |
| distinguish unreachable from model-missing by typed error | persist that flag to `install-state.json` — it is per-invocation |
| | export the `*http.Client`, or accept one from a caller |

**The loopback rule refuses hostnames.** The endpoint host must be a literal IP
for which `netip.Addr.IsLoopback()` is true, or the exact string `localhost`.
Any other hostname is refused **even if it would resolve to loopback**, because
resolution can change between the check and the dial. This chooses impossibility
over careful handling. `--allow-remote-embedder` lifts it; that flag is the whole
point of the flag. Default `http://127.0.0.1:11434`, matching `rerank.py:63`
**[C]**, and refusal is at construction, **not a warning**.

**Alternatives considered**: resolve the hostname and check the resulting
addresses (loses to DNS rebinding); warn on non-loopback (the module has just
shipped a fix for results that look fine and are not).

**RED tests** (`internal/embed/client_test.go`):
`TestNewClientRefusesNonLoopbackEndpoint`;
`TestNewClientRefusesHostnameEvenWhenItResolvesToLoopback`;
`TestClientRefusesRedirectAndNeverRequestsTheTarget` (httptest 302 → assert the
target handler was never invoked);
`TestClientTimeoutIsExplicit`;
`TestUnreachableBackendAndMissingModelAreDistinctErrors`.
Guard: `TestNetImportAllowlist`, `TestNetImportAllowlistStillRefusesOthers`
(asserts `len(allowedNetImporters) == 1` and refuses
`internal/vecindex/build.go`, `internal/query/query.go`,
`cmd/longterm-mem/main.go`).

### Decision: the embedding arm may not reuse `ObservationByID`

**Choice**: new `Store.LiveObservationsByID(project string, ids []int64)`
carrying `project = ? AND deleted_at IS NULL`.

**Rationale**: `ObservationByID` **deliberately returns soft-deleted rows**
(`store.go:247-265`) **[C]**. Reusing the one obvious method would let an
observation deleted after the index was built surface from a months-old index —
R-020 defeated by convenience. R-020 is enforced at read time, not index time, so
the index is allowed to be stale about deletions and the query is not.

**RED test**: `TestSoftDeletedObservationNeverSurfacesFromTheIndex`
(`internal/query/embedarm_test.go`) — index a row, soft-delete it, query; want it
absent and the coverage `Unindexed` count unchanged by the deletion.

## Data Flow

```
query ──► SearchTokens ──┬─► FTS arm (bm25, MatchMode) ──┐
                         │                                ├─► routeRank1(shape OR MatchAll)
                         └─► embed.Client ──► vecindex ────┤        │
                                (cosine, ids only)         │        ▼
                                                           │  round-robin interleave
                                                           │  dedup by engram_id
                                                           ▼        │
                              LiveObservationsByID (project, ¬deleted)
                                       │
                              fingerprint verify ──► mismatch ⇒ DROP row
                                       │
                              allocate byte budget ──► render snippets ──► redistribute once
                                       │
                              capResponse (backstop, largest-source drop)
                                       │
                              Result{ Results, Coverage[], Diagnostics[] }
```

The vault does not appear: it runs only when a caller names it in `sources`.

## File Changes

| File | Action | Description |
|---|---|---|
| `openspec/specs/longterm-mem-query/spec.md` | Modify | R-006 ordering amendment (spec phase owns this) |
| `internal/engram/tokens.go` | Create | exported `SearchTokens` moved from `search.go` |
| `internal/engram/search.go` | Modify | `Row.MatchOffset`, exported `SnippetAt` |
| `internal/engram/store.go` | Modify | `LiveObservationsByID`, `CountLiveObservations` |
| `internal/query/query.go` | Modify | `sources`, merge, budget, cap, `Coverage`, 6 diagnostics |
| `internal/query/gate.go` | Create | `routeRank1` (PR-4, conditional — see validation) |
| `internal/mcpserver/server.go` | Modify | `sources` field, mirroring `ExcludeTypes` **[C]** |
| `internal/embed/client.go` | Create | the only file permitted `net/http`/`net` |
| `internal/vecindex/{index,fingerprint,build}.go` | Create | manifest, fingerprints, incremental build |
| `internal/ops/{doctor,status}.go` | Modify | 3 `ops.Check` rows, `embedding_index_built_at` |
| `cmd/longterm-mem/` | Modify | `index --embeddings`, `--allow-remote-embedder` |
| `longterm-mem/net_allowlist_test.go` | Create | egress guard, sibling of the exec guard |
| `internal/query/testdata/union/*.gz` | Create | golden fixture (generated, gzipped) |

## Interfaces / Contracts

```go
// sources: exactly these three names. An unknown name is REFUSED, never
// ignored — silently narrowing a corpus is the failure this change exists
// to remove. Test: TestUnknownSourceIsRefusedNotIgnored.
const (SourceEngramFTS = "engram-fts"; SourceEngramEmbed = "engram-embed"; SourceVault = "vault")

// ResultRow.Source becomes a LIST: a row found by both arms names both.
// Precedent: the "linked" two-source row already exists (query.go:32-36) [C].
Sources []string `json:"sources"`
```

## Testing Strategy

| Layer | What to test | Approach |
|---|---|---|
| Unit | tokenizer, gate shapes, fingerprint, budget allocation, coverage arithmetic | table-driven, `t.Run` per scenario |
| Static | `net/http`+`net` import allowlist | AST walk, sibling of the exec guard |
| Property | subsequence invariant per source; merged ⊇ each arm's top-5 | generated inputs, not examples |
| Golden | arm D reproduces `93/100 · 88/94 · 40/70` **[M]** | offline fixture, temp SQLite, `-update` path |
| Integration | ollama round trip, incremental rebuild timing | `testing.Short()` skip |

**The golden fixture must not call ollama.** It carries the ground-truth rows plus
every row in either arm's top-10 for any query (~250–350 rows **[A]**), with
**untruncated** content (truncating to `input_limit` would change FTS behaviour
and move arm A's number) and their float32 vectors, gzipped. Per §E of the phase
contract, a generated golden is excluded from the authored review-line count —
which is what makes a ~2 MB fixture affordable.

**One fidelity trap the port must not fall into.** Arm A's numbers depend on the
live `observations_fts` **trigram** tokenizer. A temp DB created with the default
tokenizer will not reproduce them. The fixture generator records
`sqlite_master.sql` for `observations_fts` verbatim and the test recreates it.
**RED test**: `TestUnionGoldenFixtureUsesLiveFTSSchema`.

## The blind-authored gate validation, specified as a procedure

A mitigation that is not procedurally specified is a hope. This is the protocol;
it is a **merge prerequisite for PR-4** and its result is published either way in
`openspec/decisions/union-retrieval-gate-validation.md`.

1. **Who writes.** A fresh agent session that has read neither §4.2/§4.3 of the
   decision record nor `evaluate.py`'s gate code, and has seen no routing result.
   It receives the corpus (titles + first 500 chars) and the class definitions
   only. It writes 20 paraphrase and 20 **multi-token** identifier queries into
   `split/blind/{paraphrase,identifier}_queries.json`.
   Not the design's author. *This is the one thing nobody did for the
   measurements this design is built on.*
2. **Ground truth is fixed before any result exists.** The author records the
   expected `engram_id`(s) in the same file. That file is **committed, and its
   sha recorded in the decision doc, before `evaluate.py` is run against it.**
   The results doc records both shas; a scoring commit that does not descend from
   the query commit is void.
3. **Adjudication by a third party.** An agent (or human) who authored neither
   the queries nor the design reviews all 40 ground-truth judgements *before*
   scoring. Disagreements are **dropped, not resolved by the author**. Report the
   surviving `n`.
4. **Metric.** Per query, run both arms. The "right" arm is the one whose own
   top-1 is a ground-truth hit. If both or neither, the query is unscoreable and
   is excluded and counted. Routing accuracy = gate-matched / scoreable, **per
   class**.
5. **Thresholds — and the fail-closed rule.**

| outcome | what ships |
|---|---|
| ≥ 90% in both classes | gate on; number published |
| 80–89% in either class | gate on; number published in the capability spec, not only in a decision doc |
| **< 80% in either class** | **fixed FTS-first order. The gate code is not written — not written and flag-disabled, because a dead flag is a hazard.** Publish the paraphrase@1 loss (40% → ~10% **[M]**) in the capability spec |
| **fewer than 34 of 40 scoreable** | **inconclusive ⇒ treat as < 80%.** A validation that cannot see is not a validation that passed |

The `@5` guarantee is untouched by every row of that table. That is the whole
reason a gate is allowed to be a heuristic at all.

## Slicing — four PRs, and why

`ask-on-risk`, 800-line budget. Slice 2 as proposed (~500–700 lines, two new
packages, a harness port) would land at or past it, and the proposal already
anticipated a split.

| PR | content | authored lines **[A]** | start / finish | rollback |
|---|---|---|---|---|
| 1 | merge + `sources` + budget-before-render + quota-aware cap + R-006 delta | 250–350 | finishes when Engram rows can reach rank 1 | revert; `sources` is additive+`omitempty` |
| 2 | `internal/embed` + egress guard + allowlist test | ~420 | finishes when a loopback embedding can be fetched and a non-loopback one refused | revert; nothing reads it |
| 3 | `internal/vecindex` + `index --embeddings` + doctor/status | ~450 | finishes when an index builds, rebuilds incrementally, and `doctor` reports it | delete the index dir; revert |
| 4 | embedding arm in `query` + coverage + golden harness port | ~400 + fixture | finishes when arm D reproduces the table | flip default `sources` back to `["engram-fts"]` |

**Why this boundary and not the proposal's two-way split.** The natural fault
line is *producing* the index versus *reading* it, and the two have unrelated
failure modes: build/staleness/egress versus routing/merge/budget. Splitting
2-from-3 also isolates the only file in the repository allowed to touch the
network into a PR a reviewer can read end-to-end. PR-4 must be its own unit
because **it ships in one of two mutually exclusive shapes** depending on the
validation above; `tasks.md` must carry both branches.

**PR-1 has a user-visible consequence worth stating loudly**: its default
`sources` is `["engram-fts"]` (embed does not exist yet), so **the vault stops
running by default at PR-1** and `os/exec` goes cold on the default query path.
A caller who wants it back passes `sources: ["vault","engram-fts"]`.

Guard lines: `Decision needed before apply: Yes` · `Chained PRs recommended: Yes`
· `400-line budget risk: High`.

## Threat Matrix

The design changes source *routing* and opens a network egress boundary; the
reference matrix's rows are exec/VCS-shaped and are mostly N/A. Recorded honestly
rather than padded:

| Boundary | Applicability | Design response | RED tests |
|---|---|---|---|
| Documentation-like paths | **N/A** — no file is classified or executed | — | — |
| Git repository selection | **N/A** — no git invocation added | — | — |
| Commit state | **N/A** | — | — |
| Push state | **N/A** | — | — |
| PR commands | **N/A** | — | — |
| **Network egress (added)** | **Applicable** — first `net/http` in the module **[C]** | one-file allowlist; literal-loopback-only; redirects refused; explicit timeout; opt-in is per-invocation | the five `internal/embed` tests + two allowlist tests above |
| **Source routing (added)** | **Applicable** — `routeRank1` decides rank 1 | reads query string + one enum; emits an order; computes no score (D8 intact); soft failure | `TestGateRoutesIdentifierShapesToLexicalArm`, `TestGateDoesNotFireOnHyphenatedEnglish`, blind protocol above |

R-021 is untouched: no `os/exec` entry is added, and PR-1 makes the existing one
cold on the default path.

## Migration / Rollout

No data migration. The index is derived state: deleting
`<state-dir>/index/<project>/` is safe at any moment — it holds ids,
fingerprints and floats, never observation text. Rebuild ≈ 6 min for 586 rows
**[M]**. Rollout is the four PRs above; each is independently revertable.

## Where I complete the record, and where I would argue with it

**Completed (not changed):** (i) §5 says the ceiling binds on snippet length
"instead of" row count; with `MinSnippetBudget` it binds on row count again at
large `top`, and that is correct rather than a regression — a 40-character
snippet is worse than a dropped row. (ii) §2.4 does not say what the arm may
fetch with; `ObservationByID` is a live R-020 trap and the design forbids it.

**Where I would argue:** §5's `capResponse` rule ("drop from whichever source
holds the most slots") re-introduces a *comparison between sources* at the point
of loss. It is not score arithmetic, so D8 survives, and I keep it — but it is
the one place where a rule justified as neutral does look at both sources at
once, and it should be named as such rather than presented as obviously free.

## What this validation does NOT establish

- The published table is **one project's corpus**, n = 14/16/10, paraphrase
  questions written by the agent that ran the experiment, relevance judgements
  never independently reviewed **[M, and its limits are measured too]**.
- Embeddings covered the first 2,000 characters: long observations were judged on
  their openings. A row edited only past character 2,000 keeps a valid
  fingerprint and a valid vector while its snippet changes. That is correct under
  §2.1 and it is not obvious. **RED test**:
  `TestFingerprintIgnoresContentBeyondInputLimit`.
- The blind protocol validates **routing**, not relevance. It cannot tell us
  whether the ground truth is right, only whether an independent author agrees
  with it.
- 0.62 s/row was measured once **[M]**. PR-3 re-measures the incremental path.
- Nothing here is measured against a second corpus, a second human, or live
  traffic.

## The single strongest argument against this design

Not the gate — the decision record already owns that (§9), and I agree with it.

**Against my own additions: PR-3 merges a package nothing reads.** A new
package, a CLI flag, three doctor checks and a six-minute build step ship one PR
before their only consumer, and that consumer might be sent back to FTS-first by
a validation that has not run yet. Worse, if the Go port of arm D fails to
reproduce `93/100 · 88/94 · 40/70`, PR-3 is dead code already on main.

I do not think this dissolves, and I will not pretend it does. It is mitigated,
not removed: §8 check 1 (the Python arm-D simulation) is a **merge prerequisite
for PR-3, not PR-4** — the cheapest hour in the plan buys the right to merge the
expensive package — and PR-3's rollback is a directory deletion. If check 1 lands
between the arms, nothing after PR-1 is written at all.

The runner-up remains §5: the guarantee is computed correctly in the merge and
can still be deleted by a byte budget that has no way to know what it is
destroying. Budget-before-render is reasoned, and the numbers behind it were
measured on estimated `Result` values **[M, but estimated inputs]**. Check 3 —
real encoded responses with the full diagnostic set and `Standing` objects
present — must run before PR-1 fixes `MinSnippetBudget` at 120.

## Open Questions

- [ ] Fixture size for the golden port is **[A]** ~250–350 rows; check 3's run
      settles it. If it exceeds ~3 MB gzipped, the port drops to the ground-truth
      rows plus top-5 (weaker: it can no longer show a *near miss*).
- [ ] Does `--allow-remote-embedder` belong on `index` only, or on `query` too?
      Design leans `index` only — a query should never be the thing that egresses.
- [ ] Blind validation has not run. PR-4's shape is undetermined until it does.
