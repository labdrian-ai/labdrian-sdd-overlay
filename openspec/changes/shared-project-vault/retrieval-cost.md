# Retrieval cost: what a `query` call actually costs, and what to do about it

Question answered: *"¿cómo hacemos eficiente la búsqueda? porque ahora cuesta muchos
tokens el uso del vault."*

Measured on branch `sdd/shared-project-vault` at `33a4f37`, against the real
`~/.engram/engram.db` and the real vault at `~/labdrian-brain`.

Evidence tags used throughout:

- **[M]** measured — a command was run, the number is its output.
- **[C]** read from code — the behaviour is in a named file at a named line.
- **[A]** assumed — stated as a belief, not established here.

Token figures are **derived** from measured bytes at 4 bytes/token. No tokenizer was
available in this environment (`tiktoken` absent **[M]**), so every token number below
is an approximation of a measured byte count, and is labelled `~`. The byte counts are
exact.

---

## 1. Headline: the premise is wrong about which half is expensive

The complaint names the vault. The vault is **1.6%** of the payload.

One default `query` call over MCP — `query{project, query}`, no `top` — puts
**250,194 bytes** on the wire (~62,500 tokens) for the query text `"vault retrieval"`
**[M]**. That is roughly **31% of a 200k context window for a single search**.

Of the JSON payload, by source **[M]** (81 rows across 10 real queries):

| source | rows | total snippet bytes | share of snippet payload |
|---|---:|---:|---:|
| vault | 38 | 7,637 | **1.6%** |
| engram | 43 | 442,729 | **98.3%** |
| linked | 0 | 0 | 0% |

Vault snippets are uniformly capped: min 200, p50 201, max 201 **[M]**. The cap is the
vault's own — `chunk_snippet(chunk_data, max_chars=200)` at
`~/labdrian-brain/scripts/retrieve.py:98` **[C]**. The vault half is already efficient
and was designed to be.

Engram snippets are uncapped: p50 4,483, p90 33,991, max 42,757 **[M]**.

**The cost is Engram observation bodies, shipped whole, twice.** Naming it precisely:

1. `engram.Search` selects `o.content` — the full column, no truncation
   (`longterm-mem/internal/engram/search.go:29`) **[C]**.
2. `mergeResults` assigns it straight through: `Snippet: er.Content`
   (`longterm-mem/internal/query/query.go`, engram branch of `mergeResults`) **[C]**.
3. The MCP layer then emits the entire result **twice** (§3).

Snippet is **96.4%** of the JSON payload for the worst measured query **[M]**.

### The two facts in the brief, checked

- **Zero promoted pages** — confirmed **[M]**. `rg -l 'engram_id' ~/labdrian-brain/wiki/`
  returns 0 of 49 wiki pages.
- **`last sync: never`** — confirmed **[M]**, `longterm-mem status`.
- **`provisioned=false`** — reported, but **misleading**, see §2.

So promoted-page retrieval genuinely costs nothing today, exactly as the brief
predicted. But the vault is *not* silent: it is serving 4-5 rows per query from the
maintainer's personal wiki, cheaply. The expense is entirely on the Engram side, in
`longterm-mem`'s own Go code, and has nothing to do with the vault.

---

## 2. `status` and `query` disagree about provisioning — and `query` is right

`longterm-mem status --project labdrian-sdd-overlay` reports `vault: provisioned=false`
**[M]**, while the same binary's `query` returns `"vault_status": "ok"` with 4 vault
rows carrying real BM25 and rerank scores **[M]**.

Cause, read from code: `vault.Provisioned` (`internal/vault/index.go:52-64`) requires
three things — the sentinel `.vault-meta/.longterm-mem-provisioned`, `.vault-meta/bm25/index.json`,
and a non-empty `.vault-meta/chunks/` **[C]**. On disk, `bm25/` and `chunks/` (51 entries)
and `embed-cache.json` all exist; **the sentinel does not** **[M]**. The vault was
provisioned out of band by `setup-retrieve.sh`, which does not write longterm-mem's
sentinel. `retrieve.py` neither knows nor cares about the sentinel, so it exits 0 and
serves results.

This is a false negative, not a cosmetic one, and §7 explains why R-051 makes it
urgent.

---

## 3. The single largest waste: the MCP tool ships every result twice, byte-identical

Measured on the wire, one `tools/call` for `query{project, "vault retrieval"}` **[M]**:

```
WIRE_BYTES: 250194
  content:           127273 bytes   (one text block)
  structuredContent: 125375 bytes
text parses as JSON, equal to structuredContent: True
```

The text block is a byte-identical JSON serialisation of the structured content **[M]**.

Cause, read from code: `queryHandler` returns `nil` for the `*mcp.CallToolResult`
(`internal/mcpserver/server.go:138-148`) **[C]**. The SDK then fills it in
(`go-sdk@v1.7.0/mcp/server.go:426-437`) **[C]**:

```go
res.StructuredContent = outJSON // avoid a second marshal over the wire
// If the Content field isn't being used, return the serialized JSON in a
// TextContent block, as the spec suggests:
if res.Content == nil {
    res.Content = []Content{&TextContent{Text: string(outJSON)}}
}
```

So the duplication is the SDK's documented fallback for pre-SEP-2106 clients, and it is
**opt-out**: returning a non-nil `CallToolResult` whose `Content` is already set
suppresses it. Because `outJSON` here is a JSON *object*, the `else if !isObjectJSON`
branch does not append either **[C]** — a short summary line fully replaces the
duplicate, and `structuredContent` still carries the complete result.

Whether a given MCP host forwards *both* fields into the model's context, or only one,
is **[A]** — not established here. The server emits both regardless, and the
conservative reading is that the agent pays for both.

---

## 4. What the CLI already gets right, and the MCP surface does not

`cmdQuery`'s human-readable output never prints `Snippet` at all — only rank, source,
address, title, and any standing (`cmd/longterm-mem/cmd_query.go`) **[C]**. Measured,
per query **[M]**:

| surface | bytes |
|---|---:|
| human CLI (`query`) | 552 – 795 |
| CLI `--json` | 10,961 – 125,434 |
| MCP wire (`tools/call`) | up to 250,194 |

The CLI is 150-300× cheaper than the MCP surface for the same call, and no one has
complained that the CLI hides something. That is a strong signal about how much of the
snippet anyone actually reads.

---

## 5. Upstream Engram already solved this, and longterm-mem regressed it

Engram's own `mem_search` handler **[C]** (`internal/mcp/mcp.go:1040-1043` in the
engram marketplace checkout):

```go
preview := truncate(r.Content, 300)
if len(r.Content) > 300 {
    anyTruncated = true
    preview += " [preview]"
}
```

Three things Engram does that longterm-mem's `query` does not:

1. Caps the preview at **300 characters** — same cap in the CLI,
   `cmd/engram/main.go:990` **[C]**, so CLI and MCP do **not** differ here. (They have
   differed before; this time they agree.)
2. **Marks** the truncation with a literal ` [preview]` suffix, so the caller can see
   that there is more.
3. Names the follow-up: *"use mem_get_observation for full untruncated content"*
   (`internal/setup/setup.go:169`) **[C]**. Its structured entries carry
   `id`/`title`/`type`/`scope` and **no content at all** **[C]**.

`longterm-mem query` reimplements Engram's search directly against the SQLite file and
drops all three properties. That is the whole regression.

---

## 6. Options, costed

All figures below are the **MCP wire cost** (structured + duplicate text where the
duplicate survives), averaged over the same 10 real queries, 81 rows **[M]**. Token
column is derived.

| # | option | avg wire bytes | ~tokens | saving |
|---|---|---:|---:|---:|
| A | today — uncapped snippet, duplicated payload | 95,397 | ~23,849 | — |
| B | suppress the duplicate text block only | 47,818 | ~11,954 | **49.9%** |
| C | cap snippet at 300 only (duplicate kept) | 7,083 | ~1,770 | **92.6%** |
| **D** | **cap 300 + suppress duplicate** | **3,661** | **~915** | **96.2%** |
| E | D, plus default `top` 5 → 3 | 2,570 | ~642 | 97.3% |
| F | address-only, no snippet at all | 1,412 | ~353 | 98.5% |

Snippet-cap sensitivity in isolation, same corpus **[M]**: cap 2000 → 77.1% saved;
1000 → 85.1%; 600 → 88.7%; 400 → 90.5%; 200 → 92.4%; drop entirely → 96.3%. The curve
is nearly flat below ~600 characters, which is why 300 (Engram parity) costs almost
nothing extra over the most aggressive caps.

### Recommendation: **D**, and only D

B and C are each individually worth doing and are independent, but D is both of them
and they are two small, unrelated edits. D leaves the default `top` alone (see §8).

**What D costs in completeness, stated exactly.** With a 300-character cap, **43 of 43
Engram rows in the sample are truncated — 100%** **[M]**. Every Engram row in practice
loses body text. What the caller retains, verified across all 81 rows **[M]**:

- `engram_id` on **43/43** Engram rows — so `mem_get_observation` recovers the full
  content.
- `page_path` and `page_address` on **38/38** vault rows — so the page is readable
  from disk.
- `title` on **39/43** Engram rows.
- `score` (bm25, rerank) unchanged.
- `standing` unchanged (§9).

**Every row already carries a followable address.** Nothing becomes unreachable; it
becomes one call away. That is the property that makes truncation safe here, and it is
measured, not assumed.

**How the caller knows something is missing** — this is non-negotiable and must ship
with the cap, not after it. A retrieval that silently returns less than the operator
expects is worse than one that costs more. Concretely, the truncation must be visible
in the data, not only in documentation:

- a literal marker on the truncated string (Engram's ` [preview]` is the precedent, and
  matching it exactly is free), **and**
- a machine-readable flag on the row (e.g. `snippet_truncated: true`) plus the full
  length, so a caller can decide without string-matching.

Without both, D should not ship. The saving is not worth a silent narrowing.

**Break-even.** D saves ~22,934 tokens per query. The average observation in this
project is 4,341 characters **[M]** ≈ ~1,085 tokens to re-fetch. An agent can issue
**21 full-content follow-ups per query** before capping costs more than it saves
**[M-derived]**. Realistically an agent follows up on one or two rows. The margin is
not close.

### What I would **not** change

- **The vault's 200-char snippet.** It is 1.6% of the payload and already the correct
  design **[M]**. Touching it is optimising the wrong half, and it lives in the vault's
  `retrieve.py`, outside this component.
- **`DefaultTopN = 5`.** See §8 — the evidence does not support lowering it, and E's
  extra 1.1% is not worth a completeness change made on a hunch.
- **Ranking / the never-re-rank merge rule.** No measured cost. D8's "vault first, then
  Engram, never fused" is a correctness property, not a cost one. Changing it to save
  tokens would be trading a designed guarantee for nothing.
- **`Standing`.** See §9.
- **Dropping snippets entirely (option F).** It saves 2.3 points over D and removes the
  ability to judge relevance without a follow-up on *every* row. That inverts the
  break-even: F makes follow-ups the norm rather than the exception.

---

## 7. What this implies for `shared-project-vault` — mostly nothing, with one exception

Asked plainly: does retrieval efficiency constrain **what gets written into a page** —
its size, its frontmatter, whether the body belongs in the page at all?

**No, and the measurement says so directly.** All 38 vault rows returned snippets of
200-201 bytes regardless of the underlying page **[M]**, because `retrieve.py` caps
chunk snippets before longterm-mem ever sees them **[C]**. Page body size does not
propagate into query cost. A page can be as long as it needs to be. There is no
retrieval argument for shortening page bodies, moving bodies out of pages, or
constraining what promotion writes.

I am not going to manufacture a coupling. R-050–R-057 and D1–D11 can proceed on their
own merits; the token problem is in `internal/query` and `internal/mcpserver` and is
independent of them.

**The one real interaction is §2's false negative, and it is R-051's problem.**
R-051 says the component *"SHALL provision it into that clone's `.longterm-mem/` root
before serving a query or a promotion"*. Today `query` does not consult
`vault.Provisioned` at all — it just runs `retrieve.py` and reads the exit code **[C]**.
Once R-051 gates serving on a provisioning check, whichever predicate that check uses
inherits §2: a vault that is fully indexed but lacks longterm-mem's sentinel reads as
unprovisioned and gets re-provisioned. `~/labdrian-brain` is in exactly that state right
now **[M]**, so this fires on the maintainer's own machine on the first R-051 run — not
in some hypothetical clone.

**Second, smaller interaction: vault rows carry no title — 0 of 38** **[M]**. A
title-plus-address mode (the cheapest useful result shape) cannot be built on the vault
side today. It does **not** require a page-format change: promotion already writes
`title` into frontmatter (`internal/promote/frontmatter.go:145,178`) **[C]**, and D6
already regenerates `index.md`/`log.md` *from page frontmatter — address, title,
created, updated* **[C]**. The title is already there and already travels. Surfacing it
in a query row is a retrieval-side change, and under R-051 the retrieval tooling is
provisioned per clone, so its output contract is cheaper to settle before many clones
provision than after.

---

## 8. Top-N: the default is not the problem

Measured, query `"vault retrieval"`, CLI `--json` bytes **[M]**:

| `--top` | bytes | ~tokens |
|---:|---:|---:|
| 1 | 2,429 | ~607 |
| 3 | 43,540 | ~10,885 |
| 5 (default) | 125,434 | ~31,358 |
| 10 | 176,990 | ~44,247 |
| 20 | 177,978 | ~44,494 |
| 50 | 177,978 | ~44,494 |

Two things to read here. The curve **plateaus at ~10** because the corpus simply does
not hold more matches for that query — the `--top` bound (`maxTopN = 50`,
`cmd_query.go`) **[C]** is not what is being hit. And the growth from 1 to 5 is driven
by *snippet size*, not row count: one 42,757-character row costs more than nine capped
ones.

`top` is applied **per source**, so `top=5` returns up to 10 rows (5 vault + 5 Engram),
and 9 were returned **[M]**. That is worth documenting — the parameter is not
"5 results" — but it is a documentation defect, not a cost one.

Once D lands, top=5 costs ~915 tokens. **Lowering the default would then be optimising
something that is already free, at a direct cost in recall.** I have no measurement
showing nobody reads rows 4 and 5, and I am not going to assume it. Leave it at 5;
revisit only if someone measures which ranks get acted on.

---

## 9. `Standing` is cheap and it is not noise — keep it

Measured across 81 rows **[M]**: `standing` is present on **2 rows (2.5%)**, totalling
**268 bytes — 0.06% of the row payload**. It is free.

It is also not decorative. The two live annotations are:

- `#3244` → `superseded_by #3246` — *"shared-project-vault: addressing scoped by vault
  kind, R-021 gets a third allowlist entry"*, a supersession inside this very change.
- `#3110` → `unjudged` against `#3239` — an undecided conflict.

Cutting a 268-byte field that flags a superseded decision inside the change in flight,
in order to save 0.06%, would be a bad trade at any price.

**One correction to the record, though.** The comment justifying `Standing` in
`internal/query/query.go` says Engram's own search does not carry standing, and cites
being *"verified on a copy of a real database"*. Observation **#3239** — the one
`#3110` is flagged as unjudged against — records that those copy-based probes measured
nothing, because `ENGRAM_DATABASE_URL` is not honoured and every "copy" probe read and
wrote the real database. It further records that `mem_search` **does** annotate
`superseded_by`/`supersedes`, which I confirmed in the handler **[C]**
(`internal/mcp/mcp.go`, relation-annotation block, batch-loaded via
`GetRelationsForObservations`).

The *conclusion* survives on other grounds: `engram.Search`'s SQL genuinely does not
join `memory_relations` **[C]**, so `longterm-mem query` really would omit standing
without `attachStandings`. But the stated justification is now known to rest on an
invalid experiment, and the comment should say why it holds rather than citing a probe
that measured nothing. That is a comment fix, not a behaviour change.

---

## 10. Summary of recommended changes

Ranked by saving per unit of risk.

| # | change | saving | risk | file |
|---|---|---:|---|---|
| 1 | Return a non-nil `CallToolResult` with a one-line summary `Content`, suppressing the duplicate text block | ~50% | very low — `structuredContent` is unchanged and complete | `internal/mcpserver/server.go` |
| 2 | Cap the Engram snippet at 300 chars, with a ` [preview]` marker **and** a `snippet_truncated` flag + full length | ~93% on top of #1 | low, **conditional on the marker shipping in the same change** | `internal/query/query.go` |
| 3 | Document that `top` is per-source (5 → up to 10 rows) | none | none | `cmd_query.go` usage string, tool description |
| 4 | Fix the `provisioned` false negative before R-051 gates serving on it | none directly | see §7 — blocks a correct R-051 | `internal/vault/index.go` |
| 5 | Correct the `Standing` justification comment (§9) | none | none | `internal/query/query.go` |

Combined effect of #1 + #2: **~96% reduction**, from ~23,849 to ~915 tokens on the
average query, and from ~62,500 to roughly ~2,400 on the worst measured one.

Neither change touches the vault, promotion, page format, or frontmatter. Both are
confined to `internal/query` and `internal/mcpserver`, and neither collides with
`shared-project-vault`.

---

## Appendix: how to reproduce

```bash
cd longterm-mem && go build -o /tmp/ltmbin ./cmd/longterm-mem

# §1, §8 — payload size and top-N curve
/tmp/ltmbin query --project labdrian-sdd-overlay --json "vault retrieval" | wc -c
for n in 1 3 5 10 20 50; do
  /tmp/ltmbin query --project labdrian-sdd-overlay --top $n --json "vault retrieval" | wc -c
done

# §2 — the disagreement
/tmp/ltmbin status --project labdrian-sdd-overlay          # provisioned=false
ls -la ~/labdrian-brain/.vault-meta/.longterm-mem-provisioned   # absent
ls ~/labdrian-brain/.vault-meta/bm25 ~/labdrian-brain/.vault-meta/chunks | head

# §1 — corpus shape
sqlite3 ~/.engram/engram.db "select count(*), avg(length(content)), max(length(content))
  from observations where project='labdrian-sdd-overlay' and deleted_at is null;"
# → 580 | 4341.28 | 49151

# §1 — zero promoted pages
rg -l 'engram_id' ~/labdrian-brain/wiki/ | wc -l   # → 0

# §3 — the duplication, over real MCP stdio
# initialize, notifications/initialized, then tools/call name=query;
# compare len(result.content[0].text) against len(json(result.structuredContent))
```

---

# Addendum — what this analysis missed, and what shipped

Written after implementing the fixes on `fix/retrieval-cost`. The audit above is a
spend audit, and one of its conclusions was wrong for a structural reason worth
naming.

## A1. The failure a spend audit cannot see

`ftsMatchQuery` AND-joined every whitespace token, stopwords included. Measured on
the live corpus of 577 rows for `labdrian-sdd-overlay` **[M]**:

| query | AND (shipped) | OR |
|---|---:|---:|
| `canonical identity` | 13 | 114 |
| `what conventions apply when editing the register writer` | **0** | 556 |
| the same eight tokens, stopwords stripped, still AND-joined | **0** | 349 |

**A silently empty result costs zero tokens, so §1's methodology could never find
it.** Every number in this document is a byte count of something that came back;
nothing here can see a question that came back with nothing. Capping a snippet
nobody receives changes nothing, which is why this outranked the payload work
despite the payload work being correctly measured.

The third row matters on its own: stopword stripping **alone does not fix it**. The
AND→OR fallback is the load-bearing change; stripping improves both the precise
attempt and the widened one, but shipped by itself it would have left the question
returning nothing.

## A2. `content[:200]` was the wrong fix, and so was `snippet()`

The §5 proposal to cap at 200 characters is superseded by its own standard. These
bodies are structured markdown opening with frontmatter and a repeated title, so
their first 200 bytes are the *least* informative bytes they have.

FTS5's `snippet()` was the intended replacement and does not work against this
index. The live `observations_fts` is `tokenize='trigram'` **[M]** — a fact this
audit never checked. `snippet()`'s window is counted in tokens, SQLite clamps that
count to 64, and a trigram token is one three-character window of the document.
Measured on five live rows, at both 64 and 300 requested tokens: a **69-70 byte**
extract **[M]**. Not a snippet — a fragment of a line.

What shipped uses `highlight()` to get a tokenizer-accurate match offset and centres
a 480-character budget on it in Go. It behaves identically whatever tokenizer the
index was built with, which `snippet()` does not.

The test fixture had drifted here too: it declared `observations_fts` with FTS5's
default tokenizer and two columns against the live six and `trigram`. One existing
guard was passing for the wrong reason as a result.

## A3. Measured before/after, same binary shape, real database

CLI (`query --project labdrian-sdd-overlay --json`), bytes of encoded result **[M]**:

| query | before | after | change |
|---|---:|---:|---|
| `what conventions apply when editing the register writer` | 2,135 (**0** engram rows) | 6,131 (**5** engram rows) | the question now has answers |
| `canonical identity` | 33,877 | 5,797 | **−82.9%** |
| `vault retrieval` | 90,880 | 6,042 | **−93.4%** |

MCP `tools/call` over real stdio, full JSON-RPC response line **[M]**:

| query | before | after | change |
|---|---:|---:|---|
| `vault retrieval` | 180,734 | 9,868 | **−94.5%** |
| `canonical identity` | 67,047 | 9,478 | **−85.9%** |

§3's duplication is confirmed on the wire and fixed: `content` 89,741 B beside
`structuredContent` 90,689 B, the first parsing to exactly the second, before;
`duplicate=False` after. The cause is in the go-sdk (v1.7.0 `mcp/server.go`: `if
res.Content == nil`), not in the handler's own code — which is why reading the
handler alone suggests, wrongly, that the payload is emitted once.

The duplication was **replaced, not suppressed**. That SDK fallback exists so a
pre-SEP-2106 client can recover the payload from the text block; deleting it would
hand those clients an empty response. Each handler now supplies a compact human
rendering instead: smaller, readable, and — the explicit trade — no longer
re-parseable back into the structured shape by such a client.

## A4. Where the type filter landed, and why not as a default

`session_summary` is 71 of 581 live observations and carries the 15k-43k byte bodies
**[M]**. Both arguments for excluding it by default fail on measurement:

- The payload argument is spent — those bodies now cost one 480-character snippet
  like every other row.
- The relevance argument does not survive checking: across eight real queries,
  `session_summary` took **5 of 36** top-5 slots (14%) against its 12% share of the
  corpus **[M]**. bm25 already ranks it at about its weight.

It shipped as an opt-in filter (`exclude_types` / `--exclude-types`), default
excluding nothing, and always reported when applied.

---

# Hazard: a first `sync` would create ~271 pages in one shot

**Recorded here to survive the session. Not acted on, not part of this change.**

`promote` has never run: `wiki/memory/` does not exist and there is no sync state
**[M]**. Eligibility (`longterm-mem/internal/promote/eligible.go:27` — pinned, or
type in `decision`/`architecture`/`pattern`, or `revision_count >= 3`) matches
**271 of 581** live observations, **46%** **[M]**.

A first `longterm-mem sync` would therefore write roughly 271 pages in a single
run, into a vault whose `index.md` and `log.md` are hand-authored and carry no
marker distinguishing generated content from written content.

**Eligibility wants tightening before the first sync**, and the vault's hand-authored
files want a boundary that a generator can see. Whoever picks up the vault work
should treat this as a precondition, not a follow-up.
