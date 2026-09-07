# Union retrieval: an embedding index over Engram's own rows

**Design only. No implementation in this pass.**

Every claim is tagged **[M]** measured (command given or re-runnable from the
existing harness), **[C]** read from code (file:line given), or **[A]** assumed.

Two claims in the brief that sent me here are corrected below, and one of them
is mine: the routing rule I intended to recommend does not survive contact with
realistic queries, and §4.2 replaces it with one that does. §9 states the
strongest argument against the whole design.

---

## 0. The shape of the answer

Two retrievers over one corpus — Engram's observations — merged so that the
result set **contains both retrievers' top-5**. Not a rerank, not a fusion, not
the vault.

The acceptance criterion ("match or beat the better arm in each class") is then
satisfied **by construction at @5**, not by experiment:

> if the merged set ⊇ arm_A.top5 ∪ arm_B.top5, then recall@merged ≥ max(recall@5
> of either arm), for every query and every class, unconditionally.

That is the load-bearing property of this design and it is worth being blunt
about why: it means the @5 half of the criterion cannot fail for retrieval
reasons. It can only fail because rows got dropped downstream under the response
budget — which is §5, and which is the part I am least comfortable with.

@1 is a different matter. Rank 1 is one slot and the two arms want it for
different query classes. §4.2 decides that with a routing gate, and the gate is
a heuristic, measured but overfittable. **The design degrades softly**: if the
gate is wrong, @1 falls back to one arm's @1 and @5 is untouched.

---

## 1. Where the index lives

**Decision: `<state-dir>/index/<project>/{index.json, vectors.bin}`, where
`<state-dir>` defaults to `~/.labdrian-overlay/longterm-mem` — the module's
existing state directory, already honoured by `--state-dir`.**

- R-002 makes Engram's database read-only by construction: `readOnlyDSN` is
  `mode=ro&_query_only=true` and `store.go`'s package doc says "no code path
  here can write to Engram's database" (`internal/engram/store.go:1-4,113-115`).
  **[C]** So the index is our state. Not negotiable, not a column in their
  schema.
- `defaultRegisterStateDir()` already resolves `~/.labdrian-overlay/longterm-mem`
  and already holds `install-state.json` (`cmd/longterm-mem/register_paths.go:41-47`).
  **[C]** Reusing it adds **zero new state locations** and inherits the existing
  `--state-dir` override that the tests already use.
- **Not inside the vault.** The vault root is today's per-project scope
  (`wiki/memory/`, `.raw/`, `.vault-meta/`) **[C]**, but the entire point of this
  design is that retrieval must work for a project with no vault configured.
  Putting the index under the vault would make the replacement depend on the
  thing it replaces.
- Project names are `vaults.json` keys **[C]** and are not guaranteed
  path-safe. The directory segment is the project name when it matches
  `^[A-Za-z0-9._-]+$`, otherwise `sha256(project)[:16]`, with the literal
  project string recorded inside `index.json` so the mapping is auditable.

### Not a database, and not a vector library

**Decision: a fixed-stride binary blob plus a self-digesting JSON manifest.
No new SQLite database, no ANN index, no new module dependency.**

The arithmetic decides this, and it is not close.

| quantity | value | source |
|---|---|---|
| live rows, this project | 584 | **[M]** brief |
| `nomic-embed-text` dimensions | 768 | **[M]** model |
| index size | 584 × 768 × 4 B = **1.79 MB** | **[M]** arithmetic |
| full-scan cosine, 584 rows | ~450k multiply-adds | **[M]** arithmetic |

A brute-force scan over 1.79 MB is microseconds in Go, against a query-embedding
round trip measured at **0.137 s warm / 0.44 s after five idle minutes**
(vault-value.md §1.1) **[M]**. An ANN index would optimise 0.01% of the latency
and add a dependency, a build step, and a second thing that can go stale.
Break-even, where a linear scan starts to cost more than ~10 ms and the argument
should be revisited: **~50,000 rows** (≈154 MB, ≈38M multiply-adds) **[A]**,
which is 85× the current corpus.

Rewriting the whole 1.79 MB blob on every index run is likewise irrelevant:
`durable.WriteFile` is atomic-rename + fsync (`internal/durable/write.go:75-119`)
**[C]** and a 1.79 MB atomic replace is single-digit milliseconds against a
0.62 s-per-row embedding cost. **Incremental means "do not re-embed", not "do
not rewrite the file"** — conflating the two is what would push this toward a
database it does not need.

`modernc.org/sqlite` is already a direct dependency **[C]**, so a writable DB
was available and was still the wrong choice: it would make this module a
sqlite *writer* for the first time (grep confirms the only `sql.Open` in the
module is the read-only Engram one) **[C]**, for a workload with no
transactions, no concurrent writers, and no queries beyond a full scan.

`index.json` follows two existing shapes rather than inventing one:
`promote.PrecedenceStore` — a per-key map of sha256 fingerprints written through
`durable.WriteFile` (`internal/promote/store.go:20-43,73`) — and
`identityledger`'s self-digesting record with `ErrCorrupt` when
`Revision != digest(Entries)` (`internal/identityledger/ledger.go:73-78,188-195`).
**[C]** Both patterns are already in the module and already tested.

```
index.json
  schema        "longterm-mem.embedding-index/v1"
  project       "labdrian-sdd-overlay"     # literal, even when the dir is hashed
  model         "nomic-embed-text"
  dim           768
  input_limit   2000                       # characters embedded, see §2
  revision      "sha256:…"                 # over entries, per identityledger
  built_at      RFC3339
  entries       [ { engram_id, fingerprint, slot } … ]

vectors.bin
  slot i occupies bytes [i*dim*4, (i+1)*dim*4) — little-endian float32
```

---

## 2. Staleness

This module's recurring defect is an index that no longer describes the rows it
indexed. The existing machinery says so out loud in five places
(`stale`, `sync`, the precedence sidecar, `doctor`, `status`) and — this is the
important part — **none of it hashes an Engram row's content.** Engram-vs-page
drift is decided by `promoted.Revision >= obs.RevisionCount`
(`internal/promote/sync.go:92`); repo-vs-memory drift by `updated_at` against a
git commit date (`internal/staleness/staleness.go:157,174-187`); content sha256
is used only page-vs-sidecar (`internal/promote/store.go:62-68`). **[C]**

`revision_count` and `updated_at` are *claims about* a row. A fingerprint must
be *derived from* the bytes that were embedded, because the failure being
guarded is precisely "the bytes changed and the metadata did not".

### 2.1 The fingerprint

```
fingerprint = sha256( model ‖ "\n" ‖ dim ‖ "\n" ‖ input_limit ‖ "\n"
                      ‖ title ‖ "\x00" ‖ content[:input_limit] )
```

Hex-encoded, via the same `hashText` shape already used at
`internal/promote/update.go:475-478`. **[C]**

Three properties earn their place:

- **`model`, `dim` and `input_limit` are inside the hash.** Changing the model,
  or the 2,000-character truncation the measurement used **[M]**, invalidates
  every entry mechanically. A vector produced under one embedding contract and
  compared under another is a silent wrong answer, and there is no external
  bookkeeping that reliably catches it.
- **It hashes exactly the bytes sent to the embedder**, not the whole row. The
  claim being verified is "this vector describes this text", and only the text
  that was actually embedded can support it.
- **`sync_id` and `revision_count` are absent.** They are metadata; the hash
  already subsumes any change that matters.

### 2.2 Two drift checks, because one cannot catch the other

**Per-returned-row verification (exact).** The embedding arm resolves ids, and
must fetch those observations from Engram to render snippets anyway. At that
point it has the content in hand: recompute the fingerprint and compare. A
mismatch **drops the row** and increments a counter. This makes a confidently
wrong *returned* row structurally impossible.

Two things fall out of doing the fetch this way rather than caching text in the
index:

- **R-020 is enforced at read time, not index time.** The fetch carries
  `deleted_at IS NULL AND project = ?` like every other read
  (`internal/engram/store.go:164-167`) **[C]**, so an observation soft-deleted
  after the index was built can never surface, even from a months-old index.
  The index is allowed to be stale about deletions; the query is not.
- The index stores **no observation text at all** — only ids, fingerprints, and
  floats. There is no second copy of the human's memory on disk.

**Corpus coverage (approximate, and the one that matters).** Per-row
verification structurally cannot see a row that was *never indexed* — you
cannot verify a row you never returned, and an unindexed row is exactly the
silent-absence failure. So every response carrying embedding results also
carries its coverage, computed from one cheap `COUNT(*)`:

```
live rows for project P   (COUNT(*) … deleted_at IS NULL)   = M
entries in index.json                                        = N_total
entries whose engram_id is still live                        = N_live
```

`M - N_live` unindexed rows is reported **whenever it is non-zero**, not only
when someone runs `doctor`. A stale index that answers confidently is worse than
one that refuses; an index that answers while silently missing 200 rows is the
same failure wearing a result set.

### 2.3 Where it is reported

- **In every query response** as diagnostics (§3) — because the caller acting on
  the answer is the one who needs it, and `doctor` is not in that path. This is
  the same argument `DiagnosticSearchWidened` makes about itself
  (`internal/query/query.go:64-74`). **[C]**
- **In `doctor`**, as three new `ops.Check` rows in the existing shape
  (`internal/ops/doctor.go:55-59,21-26`) **[C]**:
  `embedding-index-present`, `embedding-index-fresh`, `embedding-backend-reachable`.
- **In `status`**, as `embedding_index_built_at` alongside
  `last_sync_completed_at`, using the same never-fabricate rule — the literal
  `"never"` when absent (`internal/ops/status.go:26,78-98`). **[C]**

### 2.4 Building it

**Decision: an explicit `longterm-mem index --embeddings` pass. Never lazily at
query time, and never from a background goroutine.**

- Initial build is ~6 min for 584 rows = **0.62 s/row** **[M]**. A query that
  silently costs six minutes is a worse failure than a query that says its index
  is incomplete.
- Incremental: read `index.json`, diff live rows against stored fingerprints,
  embed only the rows whose fingerprint is missing or changed, drop entries whose
  id is gone. A session that saves 10 observations costs ~6 s on the next run.
- **No background indexing.** A writer racing the query path is the exact bug
  class this repository keeps finding — roadmap item 12 is literally
  `fix-propagate-foreign-writer-race` **[C]** — and the module has *no file
  locking anywhere* (no flock, no `O_EXCL`, no mutex; `identityledger.Record` is
  an unlocked read-modify-write) **[C]**. Adding a concurrent writer to a module
  with no locking discipline is how this design would earn its own postmortem.

---

## 3. Degradation

`nomic-embed-text` runs behind local ollama. It can be absent, unreachable, or
present-without-the-model, and the vault's own `rerank.py` already distinguishes
those two cases (`noop-no-ollama` vs `noop-no-model`, rerank.py:198-215). **[C]**
So must we, by name.

New diagnostic codes, in the existing `Diagnostic{Code, Detail}` shape
(`internal/query/query.go:218-221`):

| code | condition |
|---|---|
| `embedding_index_absent` | no index has ever been built for this project |
| `embedding_index_incomplete` | `M - N_live > 0` live rows are not in the index |
| `embedding_index_stale` | K returned rows failed fingerprint verification and were dropped |
| `embedding_backend_unreachable` | the loopback embedding endpoint did not answer |
| `embedding_model_missing` | the backend answered; the model is not pulled |
| `embedding_source_excluded` | the caller did not request this source |

**Every `Detail` must name the consequence in query terms, not in system terms.**
"embeddings unavailable" is a status; "paraphrased questions cannot be answered
right now — only exact-identifier matches are below" is the sentence a caller can
act on. The existing details already meet this bar and set the standard
(`query.go:256, 274, 342`). **[C]**

**Degrade, never fail.** An unreachable backend returns FTS-only results plus the
diagnostic, exactly as an unprovisioned vault degrades under R-026 **[C]**. What
must never happen is FTS-only results that *look like* a working union — which is
today's exact defect in a new costume, since the vault currently reports
`vault_status: ok` while `status` reports `provisioned=false`
(`shared-project-vault/retrieval-cost.md:66-80`). **[M]**

### 3.1 The egress boundary — where the brief is right, and incomplete

The brief's claim is **correct**: ollama is an HTTP API, so the embedding arm
needs no Python, no subprocess, and adds no entry to `allowedExecImporters`
(`exec_allowlist_test.go:32-36`) **[C]**. That test's own comment — "adding a
third entry is a change to R-021, not a convenience" — is untouched, and R-021's
spec text (which forbids shelling to Engram's CLI) is untouched. **[C]** The
guard test passes unchanged.

**It is also incomplete, and the gap is worth closing in the same change.**
The module today imports `net/http` **nowhere** — zero occurrences across every
non-test file **[C]**. This design opens that door for the first time. R-021
governs the exec boundary; there is no equivalent governing the network one, and
"we did not widen the exec allowlist" is a true statement that would be doing
concealment work if it stood alone.

The precedent for what is owed already exists in this module's own decisions:
D12 says bodies stay on-machine and forces `--no-llm` on every vault index
**[C]**, and the vault's `rerank.py` requires an explicit `--allow-remote-ollama`
because page bodies would otherwise egress (rerank.py:23) **[C]**. Observation
content is strictly more sensitive than a wiki page.

So, three obligations, mirroring the exec guard:

1. **One file may import `net/http`** (`internal/embed/client.go`), enforced by
   an allowlist test written as a direct sibling of `exec_allowlist_test.go`.
2. **Loopback by default, and non-loopback refused** unless explicitly opted in
   per invocation. `rerank.py` defaults to `http://127.0.0.1:11434`
   (rerank.py:63) **[C]**; we default to the same and *refuse* anything else
   without the flag, rather than warning about it.
3. **`http.Client` with an explicit timeout and `CheckRedirect` refusing every
   redirect**, so a compromised or misconfigured local endpoint cannot bounce
   observation content elsewhere. Stdlib default is no timeout and follow-up-to-10
   redirects; both are wrong here.

Net dependency change: **none**. `net/http` is stdlib; no vector library, no
HTTP library, no model runtime is vendored.

---

## 4. The merge, and the bug

### 4.1 What is actually broken, and it is broken in the spec

The brief describes the symptom correctly. The cause is one level deeper than
"a bug in `mergeResults`":

- `mergeResults` emits every vault row before any Engram row
  (`internal/query/query.go:398-421`) **[C]**;
- `top` is per source — the MCP schema says so in words: `"results per source
  (default 5 when omitted or 0)"` (`internal/mcpserver/server.go:96-104`) **[C]**;
- `capResponse` trims from the end, and says in its own comment that it may only
  keep a prefix because merge order is a correctness guarantee
  (`query.go:302-316`) **[C]**.

Composed: the vault owns ranks 1–5 permanently, and every row lost to budget
pressure is Engram's.

**But `mergeResults` is not disobeying anything — it is implementing R-006
faithfully.** R-006 requires, verbatim:

> "results appear as vault matches (in vault rank order) followed by Engram
> matches (in Engram rank order), each tagged with its source"

and D8 records the same: *"vault rows then Engram FTS5 rows, native orders"*.
**[C]**

So the defect is **in the requirement**, and fixing it is a spec amendment, not
a code fix. That single fact is most of the sizing answer in §7, and it is the
correction I would most want challenged if I am wrong about it.

### 4.2 Interleave, not quotas — and the order is decided per query

The brief asks which of interleaving or per-source quotas I choose. **Neither is
the interesting axis**, because `top` is *already* per-source **[C]**: each
source is already quota'd at `top`. The union returns up to `n_sources × top`
rows, which is what the documented contract already says it does. **No change to
the `top` contract is required** — only to the ordering R-006 mandates.

The live question is therefore **order**, and specifically **who gets rank 1**.

**Decision: deterministic round-robin across sources, each source's own rank
order preserved as a subsequence, dedup by `engram_id` keeping the first
occurrence — with the source order for each query chosen by a shape gate.**

Why not a fixed order: rank 1 is one slot, and the two arms are near-perfect
inverses. Fixing FTS first buys identifier@1 = 100% and caps paraphrase@1 at
FTS's 10%. Fixing embeddings first does the reverse. Either fixed choice fails
the acceptance criterion in one class **[M]**, which is exactly the "landing
between the arms" outcome the brief rules out.

**Why this is legal under D8.** D8 forbids fusing scores across sources. This
merge never compares, scales, normalises, or combines a cosine with a bm25 —
no cross-source arithmetic exists anywhere in it. Two properties, both
mechanically testable, express that:

- **Subsequence invariant**: for any source *s* and any two rows from *s*, their
  relative order in the merged list equals their relative order in *s*'s native
  output. A property test over generated inputs, not an example test.
- **No cross-source arithmetic**: `Score` stays per-source and nullable exactly
  as today (`query.go:156-160`) **[C]**; no field is derived from two sources.

Dedup keeps the first occurrence, so a row found by both arms naturally lands at
the earlier of its two positions — a consequence of set union, not a comparison
anyone performs. Its `source` becomes a list rather than a scalar (the `linked`
precedent for a two-source row already exists, `query.go:32-36`) **[C]**.

### 4.3 The gate — where my own first answer was wrong

I intended to recommend a gate this codebase already computes for free:
`SearchResult.MatchMode` (`internal/engram/search.go:50-57,116-141`) **[C]**.
FTS widening from AND to OR is FTS declaring it could not match the whole query;
so, put the embedding arm first when it widened. Elegant, zero new state, and it
uses the arm's own admission of weakness rather than a guess about the query.

**On the curated class split it is perfect. On realistic queries it is not.**
Measured now, against the live corpus, with the existing harness **[M]**:

```
gate: match-mode
  ident-curated    14/14      para-curated     10/10
  ident-realistic   6/ 9      para-realistic    4/ 4
    misrouted: "ClaudeAdapter CombinedOutput"
               "where is CaptureBackend defined"
               "why did we choose highlight() over snippet()"
```

All three misroutes are identifier questions that widen to OR because their
tokens never co-occur, and all three would surrender rank 1 to an arm measured
at **0%** identifier@1. The curated identifier set hid this completely: all 14 of
its queries are single-token, and a single-token query is `MatchAll` by
definition (`search.go:127-132`) **[C]**. The gate was validated on a degenerate
case.

A token-shape gate does better, and the conjunction is clean on everything I
have **[M]**:

```
gate: shape             ident-curated 14/14  para-curated 10/10
                        ident-realistic 8/ 9  para-realistic 4/ 4
                          misrouted: "R-021 exec allowlist"

gate: shape OR match-mode
                        ident-curated 14/14  para-curated 10/10
                        ident-realistic 9/ 9  para-realistic 4/ 4
```

**Decision — the gate is `shape OR match-mode`:**

> Rank 1 goes to the lexical arm if **any query token is identifier-shaped**
> (interior CamelCase boundary, or contains `/`, `_`, `(`, `)`, or a dotted
> `word.word`) **or** FTS matched in `MatchAll` mode. Otherwise rank 1 goes to
> the embedding arm. Beyond rank 1 the round-robin is fixed.

Deliberately excluded from "shaped": the hyphen, which would fire on ordinary
English (`off-the-shelf`) and misroute paraphrase to the lexical arm.

This is source *routing*, not scoring: it reads the query string and one enum
the search already returns, and it emits an ordering of sources. No score is
computed, compared, or combined. D8 is untouched.

**Read §9 before trusting this gate.** 37 queries is not a validation set, and
I authored 13 of them after seeing how the first 24 behaved.

### 4.4 What the acceptance criterion becomes

| class | criterion | how it is met |
|---|---|---|
| identifier@5 | 100% | union ⊇ FTS.top5; FTS@5 = 100% **[M]** → guaranteed |
| paraphrase@5 | ~70% | union ⊇ embed.top5; embed@5 = 70% **[M]** → guaranteed |
| identifier@1 | 100% | gate routes FTS first; FTS@1 = 100% **[M]** → gate-dependent |
| paraphrase@1 | 40% | gate routes embed first; embed@1 = 40% **[M]** → gate-dependent |

Both @5 rows are **structural** and hold whatever the gate does. Both @1 rows are
**heuristic**. Arm C (the rerank the previous recommendation proposed) is beaten
in three of four cells and tied in the fourth: 100 vs 93, 100 vs 100, 40 vs 30,
70 vs 30. **[M]**

---

## 5. The response budget — the constraint that actually binds

Doubling the sources doubles the rows, and `ResponseTokenCeiling` is 2,000 tokens
= 8,000 bytes, with `SnippetBudget` at 480 characters
(`query.go:95-114`, `search.go:39`). **[C]** Measured, by encoding realistic
`Result` values built from 200 live observations **[M]**:

| rows | snippet budget | encoded bytes | vs 8,000 ceiling |
|---:|---:|---:|---|
| 5 | 480 | 3,765 | 47% — today |
| **10** | **480** | **7,060** | **88% — the union at defaults** |
| 10 | 300 | 5,194 | 65% |
| 10 | 240 | 4,568 | 57% |

So the union at default `top=5` fits — **at 88% of the ceiling, with two
diagnostics and no `standing` objects**. Add a third diagnostic (this design
introduces six new codes) or a couple of `Standing` annotations
(`query.go:189-214`) **[C]** and it goes over. At which point `capResponse` trims
from the end, and the design's one structural guarantee — that the merged set
contains both arms' top-5 — is destroyed *after* the merge computed it correctly,
silently, in the layer that is forbidden from re-ranking and so cannot choose
wisely which row to lose.

**Decision: allocate the byte budget across rows before rendering snippets,
rather than rendering full snippets and then dropping rows.**

The snippet budget becomes a function of how many rows are being returned, so the
ceiling binds on *snippet length* instead of on *row count*. "Ten answers in
shorter previews" strictly dominates "five answers in long previews" when the
entire purpose of the change is recall — and the machinery to make that honest
already exists and is already argued for: `SnippetTruncated` and `FullLength`
tell a program the preview is a fragment, the `…` marker tells a person, and the
`get` tool fetches the whole row (`query.go:171-186`) **[C]**.

`capResponse` stays as the backstop for pathological shapes. When it does fire,
it must drop **the lowest-ranked row of whichever source currently holds the most
slots**, iteratively, so both sources shrink together. Today's tail-trim is
structurally biased against whichever source the merge put last; under a union
that bias would silently delete the entire reason the union exists. Removing rows
is not re-ranking — the surviving rows keep their merged order and their rank
numbers, exactly as `capResponse` already guarantees (`query.go:302-316`). **[C]**

---

## 6. What happens to the vault half

**It is subsumed for retrieval, retired from the default path, kept reachable,
and not deleted in this change.**

The retrieval case for the vault is finished, and it is finished on facts rather
than preference:

- Its embedding half indexes `<vault>/wiki/**/*.md` — 49 pages of upstream
  `claude-obsidian` documentation — because `contextual-prefix.py:79` hardcodes
  `WIKI_DIR = VAULT_ROOT / "wiki"`. **[C]**
- **Pointing it at Engram is not an available fix.** A probe that placed an
  Engram export inside a vault and re-indexed found "BM25 doc_count stayed 131,
  zero `engram/` chunks … `export_notes_retrievable = false`"
  (`longterm-mem/docs/bridge-probe.md:36-41`). **[M]** This kills the cheapest
  alternative to the present design, and it kills it with a measurement rather
  than an argument.
- And a promoted page is its observation's bytes: `renderBody` writes
  `obs.Content` verbatim behind an H1 and a provenance footer
  (`internal/promote/page.go:98-122`). **[C]** Indexing pages is indexing
  observations with extra steps.

**Mechanism.** Add `sources []string` to `QueryIn`, defaulting to
`["engram-fts", "engram-embed"]`. The precedent is exact and already shipped:
`ExcludeTypes` is an optional, additive, `omitempty` field threaded verbatim into
`query.Request` (`server.go:96-104`, `query.go:140`). **[C]** The vault runs only
when a caller names it.

Three consequences worth stating plainly:

1. R-006's vault-first clause narrows to "*when the vault source is requested*"
   rather than being deleted, so `vault_status` and R-026's degrade path survive
   unchanged for callers who ask.
2. **The `os/exec` surface goes cold on the default path.** Today every query
   spawns `python3 scripts/retrieve.py` under `internal/vault/runner.go` **[C]**;
   after this, the default query spawns no process at all. That is a security
   improvement the retrieval argument gets for free, and it should be named as
   one.
3. Deletion is a separate decision, taken *after* the union is measured in
   production, not bundled into the change that replaces it. Retiring a path and
   deleting its code in one step removes the ability to compare them.

---

## 7. Sizing: one SDD cycle, two slices — and why that is not a repeat of the last one

**This is not a small change.** Three independent reasons, none of which is line
count:

1. **It amends a shipped requirement.** R-006 mandates the block ordering that
   produces the defect (§4.1) **[C]**. A spec delta is not a bug fix.
2. **It changes a contract every MCP caller sees**: result ordering, a new
   `sources` parameter, six new diagnostic codes, and a snippet length that now
   varies with row count.
3. **It opens a new external dependency and a new egress boundary** in a module
   that currently has neither (§3.1) **[C]**, and it introduces the module's
   first self-owned mutable index — the artifact class whose staleness this
   codebase keeps finding.

Two delivery slices, one cycle:

| slice | content | spec impact | rough size |
|---|---|---|---|
| **1 — merge** | interleave + gate scaffold + `sources` opt-in + budget-aware snippets + quota-aware cap | amend R-006; restate D8 as "no cross-source score arithmetic" | ~250–350 lines + tests |
| **2 — index** | `internal/embed` (HTTP client, egress guard), `internal/vecindex` (fingerprints, incremental build), `index`/`doctor`/`status` wiring, embedding arm | new capability spec | ~500–700 lines + tests |

Slice 1 ships alone and is valuable alone: it stops Engram's own memory being
displaced from ranks 1–5 of every response **[M]**, and it settles the merge
contract *before* a third source arrives — which is the brief's own point that
adding a source without settling this makes it worse.

### What it displaces

**Nothing live.** It occupies the slot `shared-project-vault` vacated under
roadmap item 25 `longterm-mem` (status `planned`) **[C]**. It competes for
attention with item 22 `gadu-portable-operator` (`in-progress`) and item 19
`skills-validate-ondisk-gate` (`planned`) **[C]** — those are the real costs, and
they are attention costs, not dependency conflicts.

### The safeguard against repeating the frozen package

`shared-project-vault` froze because 2,200–3,200 lines of planning were written
before the evidence that cancelled them. The inversion here is that the evidence
came first — but that is not automatically protective, because §9 says the
evidence is thin in a specific place.

**So: write no spec, design, or tasks until §8's four checks pass.** The
pre-implementation cost is roughly a day; the planning package it gates is the
thing that got frozen last time.

---

## 8. Verifying the criterion *before* implementation

The harness that produced the brief's table is real, is at
`split/{evaluate.py, corpus.json, identifier_queries.json, paraphrase_queries.json,
stopwords.txt}`, and reads the live Engram database read-only **[C]**. Four
checks, in order, all runnable before a line of Go:

1. **Simulate the union as arm D.** ~30 lines added to `evaluate.py`: merge
   `shipped()`'s top-5 and `pure_embed()`'s top-5 by the §4.2 gate, dedup, score
   @1/@5 over both classes. Pass = `100 / 100 / 40 / 70`. Fail — anything landing
   between the arms — kills the design before it costs anything. This is the
   single highest-value hour in the whole plan.

2. **Attack the gate with queries it has never seen.** A *different* agent, shown
   the corpus but not the gate, writes 20 paraphrase and 20 realistic multi-token
   identifier queries; score the gate's routing on those. This is the direct test
   of §9. Pass = ≥90% routing accuracy per class. Below ~80%, ship the union with
   a fixed FTS-first order, take the paraphrase@1 loss, and say so — the @5
   guarantee survives either way.

3. **Measure the budget with real encoded responses**, not the estimates in §5:
   run all 24+ queries through the union, build actual `Result` values with real
   snippets, `Standing` objects and the full diagnostic set, and count how often
   `capResponse` would fire and what it would drop. This sizes the §5 snippet
   allocation instead of guessing it.

4. **Re-run the corpus embed and time it honestly** — 584 rows at 0.62 s/row was
   measured once **[M]**; confirm the incremental path (embed only changed
   fingerprints) on a corpus mutated by ~10 rows.

Then, in implementation, **port the harness to a Go golden test** so the criterion
becomes a regression gate rather than a one-off — the same discipline as
`exec_allowlist_test.go`, which turned an argued boundary into a test that fails
when someone widens it.

### One fidelity gap in the harness, worth fixing while porting

`evaluate.py:18` tokenises with `re.findall(r"[A-Za-z0-9_./-]+", query)`; the
shipped code uses `strings.Fields` (`search.go:273`) **[C]**. On
`search.go:181` the harness yields two tokens where production yields one. It
does not change the published table (identifier queries are single symbols), but
a Go port must use `strings.Fields` or it will measure a retriever that does not
ship.

---

## 9. The strongest argument against this design

**The routing gate is fitted to 37 queries, and I wrote 13 of them after
watching the first 24 behave.**

The union itself rests on the brief's own honest limits: n = 14 and 10, the
paraphrase questions hand-written by the agent that ran the experiment, lexical
distance verified but relevance judgements never independently reviewed, one
project's corpus, embeddings over the first 2,000 characters so long observations
were judged on their openings. The gate then adds a *second* layer of the same
methodological weakness on top: I built §4.2's rule against those 24 queries plus
13 probes of my own invention, discovered that my first rule failed on my own
probes, and adopted a second rule that passes all 37. **A rule that survives the
examples it was designed against has not been tested.** The shape heuristic in
particular — CamelCase, slashes, underscores, parens — is a claim about how *this
human* writes queries, inferred from queries *no human wrote*.

If the gate is overfit, paraphrase@1 lands nearer FTS's 10% than embeddings' 40%
on real traffic, and the honest headline shrinks from "matches the better arm in
every class" to "matches it at @5, sometimes at @1".

Two things keep this from being fatal, and neither excuses it:

- **The @5 guarantee is gate-independent.** It is a set-union property, not an
  empirical result. A bad gate costs rank-1 quality and nothing else.
- **Check 2 in §8 is designed specifically to catch it**, by having someone other
  than me write the queries — which is precisely what nobody did for the
  measurements this design is built on.

### Runner-up, and it is close

**§5.** The union sits at 88% of the response ceiling at defaults **[M]**, and
the ceiling binds on the exact thing the design exists to deliver: the second
arm's rows. The guarantee is computed correctly in the merge and can be deleted
downstream by a byte budget that has no way to know what it is destroying. §5's
budget-before-render answer is right, but it is an answer I reasoned to rather
than measured, and check 3 exists because of that.

### What would change my mind

- **Check 1 lands between the arms.** Then the union does not compose the way set
  union says it should — most likely because embedding@5 = 70% does not survive
  contact with `top`-bounded candidate handling — and the design is wrong at its
  root, not at its edges.
- **Check 2 scores the gate below ~80%.** Ship the union, drop the gate, take
  FTS-first and the paraphrase@1 loss, and say so in the response.
- **The corpus grows past ~50k rows.** The full-scan argument in §1 expires and
  an ANN index stops being over-engineering.
- **Ollama stops being reliably local.** The entire egress posture in §3.1 rests
  on a loopback endpoint; a hosted embedder makes this a data-egress decision
  about the human's private memory, which is a different decision with a
  different owner.

---

## 10. Enabled, not scoped in

The embedding arm produces a bounded, comparable relevance signal (cosine ∈
[-1, 1]) where bm25 produces an unbounded one. That makes **abstention** — the
gap named in vault-value.md §7.2, where both retrievers scored 0%/0% on negative
queries because neither can say "I don't know" **[M]** — implementable for the
first time, as a per-source floor.

It is deliberately **not** in this design. A floor is a threshold, and a
threshold chosen on 24 queries is the §9 mistake repeated in a place where its
failure mode is silent absence rather than a bad rank. It wants its own
measurement and its own decision.

---

# The union simulated as a fourth arm, before any spec or tasks

Run against the live corpus with the harness from `vault-value.md`, extended
to close the gap this design's own testing exposed: **every identifier query
in the original set was a single token**, and a single token is `MatchAll` by
definition, so that set could not see a routing gate fail. Sixteen
multi-token identifier queries were generated mechanically — pairs of
co-occurring rare identifiers, and the shape that actually breaks routing, an
identifier wrapped in prose (`where is X defined and how is it used`).

Arm D is the design as written: FTS and embeddings both run; the token-shape
rule decides which owns rank 1; round-robin interleave; dedup.

| class | arm | hit@1 | hit@5 |
|---|---|---:|---:|
| identifier, single token (n=14) | A trigram FTS | 93% | 100% |
| | B embeddings | 0% | 7% |
| | C FTS + rerank | 93% | 100% |
| | **D union** | **93%** | **100%** |
| identifier, multi-token (n=16) | A trigram FTS | 88% | 94% |
| | B embeddings | 0% | 6% |
| | C FTS + rerank | **50%** | **56%** |
| | **D union** | **88%** | **94%** |
| paraphrase (n=10) | A trigram FTS | 10% | 10% |
| | B embeddings | 40% | 70% |
| | C FTS + rerank | 30% | 30% |
| | **D union** | **40%** | **70%** |

**The acceptance criterion is met: the union matches the better arm in every
class**, rather than landing between them. It costs nothing where FTS wins
and delivers the whole embedding gain where FTS loses.

**And the multi-token class settles the rerank question for good.** Arm C
falls from 88% to 50%@1 there — reranking actively destroys the case the
trigram index nails, on the query shape a human is most likely to type. The
single-token set had hidden it; the earlier recommendation to "port the
rerank" was measured on a harness that could not see its own failure mode.

## Two staleness artifacts, recorded rather than smoothed over

**The ground truth aged during the experiment.** The corpus grew from 584 to
586 rows mid-run — memory saved by the same session — and 4 of the 14
original identifier queries lost their unambiguous ground truth, because the
new observations mention those identifiers. That, not a retrieval change, is
why arm A reads 93% here and 100% in the first run. The relative comparison
is unaffected: all four arms ran against the same corpus in the same
invocation.

**The embedding index was already stale by two rows** when it answered — 584
embedded against 586 present. Arm B and arm D could not return the two newest
observations at all. This is the exact condition the design's coverage
reporting exists to surface, observed in the wild within one session of
building the harness, and it is the strongest single argument for making
coverage a field in every response rather than a diagnostic nobody reads.

A measuring instrument that silently goes out of date is the same defect this
module keeps finding in the things it measures.
