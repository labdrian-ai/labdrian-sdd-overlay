# Does the vault earn its keep?

**No — not as a store. Yes — as a technique: stop querying the vault on every
call today, do not build `shared-project-vault`, and port its embedding rerank
onto Engram's own rows instead.**

Every claim below is tagged **[M]** measured, **[C]** read from code, or
**[A]** assumed. Commands are given so every number can be re-run.

---

## 1. Three premises in the brief are wrong

I was asked to challenge anything soft. Three of the starting facts did not
survive measurement, and two of them point the *opposite* way from the brief.

### 1.1 Retrieval does not cost 2.5–5s. It costs 0.14s. **[M]**

```bash
cd ~/labdrian-brain
python3 - <<'EOF'
import subprocess,time,random,string
for _ in range(6):
    q = "how do I keep my second brain from rotting over time " + \
        "".join(random.choices(string.ascii_lowercase, k=6))   # novel every run
    t0=time.monotonic()
    subprocess.run(["python3","scripts/retrieve.py",q,"--top","5"],capture_output=True)
    print(f"{time.monotonic()-t0:.2f}s")
EOF
```

Six queries with never-before-seen text: **0.14, 0.13, 0.13, 0.13, 0.13, 0.13s**.

Across all 78 vault calls made for this document: **median 0.137s, p90 0.725s,
max 2.814s, mean 0.268s**. **[M]**

The tail is not novel text — it is ollama loading `nomic-embed-text` into
memory. Forcing an unload and re-running gives **0.42–0.44s**, three times: **[M]**

```bash
curl -s -X POST http://127.0.0.1:11434/api/embeddings \
     -d '{"model":"nomic-embed-text","prompt":"unload","keep_alive":0}'
/usr/bin/time -f "%e s" python3 scripts/retrieve.py "cold start probe" --top 5
```

Ollama's `expires_at` keeps the model resident ~5 minutes (`/api/ps`). **[M]**
So the honest cost is **~0.14s warm, ~0.44s after 5 minutes idle**. I could
not reproduce 2.5–5s under any condition I could create; the most likely
explanation is a genuinely cold disk read of the model file, which happens at
most once per boot. **[A]**

**This changes the argument's shape completely.** The vault is not expensive
in seconds.

### 1.2 A read-path query does *not* touch `embed-cache.json`. **[M][C]**

```bash
stat -c %y ~/labdrian-brain/.vault-meta/embed-cache.json   # before
python3 scripts/retrieve.py "an entirely new phrasing xyzzy42" --top 5
stat -c %y ~/labdrian-brain/.vault-meta/embed-cache.json   # after — identical
```

`scripts/rerank.py` sets `cache_dirty` only when a *chunk* embedding is
missing (line 246); the cache holds exactly 131 entries for exactly 131
chunks. The query embedding is computed fresh every call and never cached
(line 220). **[C]** Nothing is written on the read path.

### 1.3 The indexed corpus is 49 pages, not 113. **[M]**

`fd -e md wiki | wc -l` → 49, matching 49 chunk directories in
`.vault-meta/chunks/`. The 113 figure counts the whole upstream repo
(README, CHANGELOG, docs); those are not indexed and not retrievable.

### What the brief got exactly right

`wiki/memory/` does not exist; the vault holds **zero promoted pages**; all 49
indexed pages are upstream `AgriciDaniel/claude-obsidian` documentation. **[M]**

---

## 2. The decisive experiment: is the vault complementary to trigram FTS?

### Method

A trigram-FTS5 baseline replicating `internal/engram/search.go` **exactly** —
stopword-filtered whitespace tokens, each double-quoted, `AND` first with an
`OR` retry only when `AND` returns nothing, `ORDER BY rank`,
`tokenize='trigram'` — indexed over the same 48 wiki pages the vault serves,
then compared against the real `scripts/retrieve.py --top 5`.

Two corpora, because one alone is arguable:

- **The vault's own 50-query benchmark** (`wiki/meta/retrieval-benchmark-v1.7.md`),
  written by the vault's authors with ground truth, to showcase their pipeline —
  i.e. deliberately biased *in the vault's favour*.
- **A 28-query class split** I built to answer the complementarity question
  directly.

**Methodology note that matters:** `wiki/meta/retrieval-benchmark-v1.7.md`
quotes all 50 queries verbatim and *is itself in the indexed corpus*. Left in,
trigram FTS returns that one page as the top hit for every query and scores
near zero. It is excluded from both retrievers. Any comparison that skips this
step is measuring contamination, not retrieval. **[M]**

Harness: `fts_baseline.py`, `parse_bench.py`, `run_vault.py`, `score.py` (session
scratchpad; each is ~40 lines and re-creatable from this description).

### Result A — the vault's home-turf benchmark (n=50)

| category | n | vault@1 | vault@5 | trigram@1 | trigram@5 |
|---|---|---|---|---|---|
| cross-page | 10 | 30% | 80% | 50% | 80% |
| derived | 25 | 72% | 88% | 48% | 76% |
| partial-recall | 5 | 60% | 100% | 40% | 80% |
| synonym | 5 | 60% | 100% | 40% | 100% |
| negative | 5 | 40% | 0% | 20% | 0% |
| **ALL** | **50** | **58%** | **80%** | **44%** | **72%** |

**[M]** The vault wins: +14 points at top-1, +8 at top-5. 7 queries hit only in
the vault, 3 only in FTS — complementarity in both directions.

The `negative` row scoring 0%/0% is an artifact worth naming: a negative query
succeeds by returning *nothing*, and **neither retriever can abstain** — both
always return 5 rows. Symmetric, so it does not favour either, but it means
neither system can say "I don't know". **[M]**

### Result B — the class split (n=28), the question actually asked

| class | n | vault@1 | vault@5 | trigram@1 | trigram@5 |
|---|---|---|---|---|---|
| exact-identifier | 14 | 43% | 71% | **86%** | **100%** |
| paraphrase | 14 | **57%** | **79%** | 14% | 64% |

**[M]** This is the mirror image the hypothesis predicted, and it is sharp:

- On **exact identifiers**, trigram FTS is perfect (100%@5) and the vault
  misses 4 of 14 entirely.
- On **paraphrase**, the vault is **4× better at top-1** (57% vs 14%).

The 14 identifier queries were derived **mechanically**, not cherry-picked: all
tokens matching a filename/path/versioned-name shape that occur on exactly one
page (275 candidates; `random.seed(11)`, sample 14). Examples: `bin/setup-mode.sh`,
`F43F5E`, `v1.7.2`, `main.js`, `wiki/meta/claude-obsidian-cover.gif`.

The 14 paraphrase queries I wrote by hand, so they are listed in full at §6 for
you to attack. Ground truth is one page each.

Every paraphrase query drove FTS into `OR` mode — `AND` found nothing, all 14
times. **[M]** That is Engram's real fallback behaviour, and it is why its
top-1 collapses to 14%: an OR-join over a natural-language question ranks
noise first.

### Result C — the ablation, and the number that decides everything

Is the vault's paraphrase win from the **embeddings**, or merely from BM25's
word tokenizer beating trigram? Re-run with `--no-rerank`:

```bash
python3 scripts/retrieve.py "<query>" --top 5 --no-rerank
```

| paraphrase (n=14) | @1 | @5 | median latency |
|---|---|---|---|
| trigram FTS | 14% | 64% | ~0.007s |
| vault, BM25 only | 36% | 64% | 0.063s |
| vault, BM25 + embedding rerank | **57%** | **79%** | 0.137s |

**[M]** The embedding rerank is genuinely load-bearing: **+21 points at top-1,
+15 at top-5** over BM25 alone. And its marginal cost is
**0.137 − 0.063 = ~0.074 seconds.**

Note that BM25-only and trigram tie at 64%@5. At top-5 the tokenizer is not the
differentiator — **the embedding rerank is the entire advantage.**

### The architectural ceiling on all of this **[C]**

`scripts/retrieve.py` runs `bm25.query(...)` for the top-20 candidates and
passes *only those* to `rerank.rerank(...)`. The embeddings **reorder BM25's
candidate list; they never widen it.** Anything BM25's word index misses in its
top-20 is unreachable, however semantically close. The vault is lexical recall
with a semantic re-sort — not semantic retrieval. That caps how much
complementarity is available at all, and explains why the paraphrase gain shows
up at top-1 (+21) far more than top-5 (+15).

---

## 3. What promotion actually buys — read from `EmitPage`

`internal/promote/page.go`. **[C]**

```go
func renderBody(obs engram.Observation) string {
	content := strings.TrimRight(obs.Content, "\n")
	// ... prepend "# <title>" if absent ...
	b.WriteString(content)          // verbatim
	b.WriteString(promotionFooter(obs.ID, obs.RevisionCount))
}
```

**There is no distillation.** `EmitPage` writes `obs.Content` byte-for-byte,
prepends an H1 if the content lacks one, and appends a provenance footer.
Frontmatter carries title, address, aliases, dates, tags, status, wikilinks and
flat `engram_*` fields. Nothing summarizes, compresses, rewrites, or merges.

So "distilled prose versus session exhaust" is **false**. A promoted page is
the same bytes as the observation, in a file, with a header. Whatever an
Engram observation is, its page is that exact thing.

**One point in promotion's favour, and it is real:** promotion *curates* by
selection even though it does not distill by rewriting. `internal/promote/eligible.go`
admits an observation only if it is pinned, or typed
`decision`/`architecture`/`pattern`, or has `RevisionCount >= 3`. **[C]** That
is a meaningful quality filter.

But it is a filter, not a datastore. `Search()` already accepts `excludeTypes`
and applies it as a SQL predicate beside the project and soft-delete filters
(`search.go:181`). **[C]** Expressing "pinned OR type IN (...) OR revision_count
>= 3" is one `WHERE` clause against the table Engram already has. It does not
require a second store, a page format, a sync protocol, or a promotion writer.

---

## 4. The real cost of the vault today — and it is not latency

`internal/query/query.go`, `mergeResults` (line 395). **[C]**

```go
for _, c := range vaultRows {  ...  merged = append(merged, ...) }   // vault FIRST
for _, er := range engramRows { ... merged = append(merged, ...) }   // engram AFTER
```

**Every vault row is emitted ahead of every Engram row, unconditionally.** The
MCP `top` parameter is *per source*, so with the default `top=5` the vault
permanently owns **ranks 1–5 of every single query response** — and with zero
promoted pages, ranks 1–5 are always upstream Obsidian-plugin documentation.

`capResponse` then trims **from the end** to fit `ResponseByteCeiling` — and
says so explicitly in its own comment, because merge order is a correctness
guarantee it may not re-rank. **[C]** The consequence is structural: under
budget pressure the rows dropped are **Engram rows**. The vault cannot lose a
slot; Engram cannot win one.

Live demonstration, this session, via the real MCP path: **[M]**

```
query(project="labdrian-sdd-overlay",
      query="how do we decide which observations get promoted to the vault", top=5)
```

returned **four rows, all `source: "vault"`, all upstream documentation** —
`methodology-modes.md`, `boundary-frontier-2026-04-24.md`, `hot.md`,
`getting-started.md` — and **zero Engram rows**, for a question about his own
promotion logic.

A second query, `"trigram tokenizer snippet"` with `top=3`, returned ranks 1–2
as irrelevant upstream release-session notes, and at **rank 3** the Engram
observation that actually answers it — a full, structured, genuinely distilled
memory about FTS5 trigram behaviour. **[M]**

That is the cost, stated precisely: **not seconds — result slots and reading
order.** The brief's framing ("2.5–5 seconds spent searching somebody else's
documentation") understates the problem by blaming the wrong resource. It is
~0.14s spent searching somebody else's documentation, and then that
documentation is placed *above* the answer.

---

## 5. Options, costed

| # | Option | Cost to build | What it does to today's defect |
|---|---|---|---|
| 1 | Keep as-is | 0 | Ranks 1–5 stay upstream docs on every call. Unacceptable. |
| 2 | **Opt-in per call** — add a `sources` arg to `query`, default engram-only | **~50–100 lines** | Removes the displacement immediately and reversibly. |
| 3 | Build `shared-project-vault` | **2200–3200 lines** | Ships pages between collaborators. Does not fix ordering. Buys travel for text that is a verbatim copy of Engram rows. |
| 4 | Drop the vault, invest in Engram retrieval | ~200–400 lines | Real gains available (§7), but trigram is a hard ceiling on paraphrase: 14%@1. |
| 5 | **Port the rerank onto Engram** *(not in your list)* | **~300–500 lines** | Keeps the measured +21pt paraphrase win. No pages, no promote, no sync, no second copy of the text. |

### Recommendation

**Do option 2 now, then option 5. Do not build `shared-project-vault`.**

Option 2 is the immediate fix and costs almost nothing: the vault currently
displaces his own memory from the top of every response, and one default flip
stops that today without deleting anything.

Option 5 is the destination. The measurement says the load-bearing component is
the **embedding rerank** — worth +21 points of paraphrase top-1 for ~74ms — and
that component has **no dependency whatsoever on pages, files, frontmatter,
addresses, promotion, or sync**. It is: embed the query, embed the candidate
rows, sort by cosine. Applied to Engram's own FTS candidates it needs no second
store, no duplicated text, and no `shared-project-vault`.

The key tradeoff I am accepting: option 5 **inherits Engram's candidate
generator**, and trigram FTS in `OR` mode is a worse candidate list than word
BM25. A rerank over bad candidates cannot fix bad candidates (§2, ceiling). So
option 5 should be scoped as *rerank + a word-tokenized FTS column*, not rerank
alone — which is why I costed it at 300–500 lines rather than 100.

**On `shared-project-vault` specifically:** the 8 phases and 40 tasks exist to
make pages travel between collaborators. Given §3, those pages are Engram
observations copied verbatim into files. The change therefore builds
2200–3200 lines of transport for content whose canonical copy already lives in
a database that could be replicated directly. The planning being finished is
not evidence that the thing should be built. **Do not build it.** If sharing
between collaborators is the real requirement, that is a *sync-Engram-rows*
problem and deserves its own proposal, not this one.

---

## 6. The strongest argument against my own recommendation

**The complementarity I measured may not transfer to his actual corpus, and if
it doesn't, option 5 buys nothing and option 2 is the whole answer.**

Everything in §2 was measured on 48 pages of upstream *prose* — conceptual
essays about knowledge management, written in flowing English. That is the
text type embeddings are best at and trigram is worst at, so the paraphrase
result may be close to a best case for the vault's technique.

Engram observations are a different genre. The one surfaced in §4 is dense
structured technical markdown: `observations_fts`, `fts5(title, content, ...)`,
`tokenize='trigram'`, `search.go`, `highlight()`, `69-70 byte extract`. That is
**exact-identifier text** — precisely the class where trigram FTS scored
**100%@5 and 86%@1** and the vault's pipeline scored 71%/43%. If his real
memory skews that way, porting the rerank could be *neutral or negative*, and
the honest recommendation collapses to "option 2, stop there".

I did not test that, and I am not going to assert it either way. It is the one
thing that would change the plan, so it should be measured before a line of
option 5 is written.

**What would settle it:** run this exact class-split experiment against the live
577-row Engram corpus — mechanically-derived identifier queries and hand-written
paraphrase queries over his own observations, scored the same way. If the
paraphrase top-1 gap survives at anything like +21 points, build option 5. If it
lands under ~+8, keep option 2 and invest the effort in §7 instead. That
experiment is perhaps a day, against 2200–3200 lines it may cancel.

### What else would change my mind

- **If `EmitPage` gained real distillation** — an actual summarization step, not
  a copy — §3 reverses and pages become a genuinely different artifact from
  observations, worth their own store.
- **If the vault's candidate generator stopped being BM25-only** (true vector
  recall, not a top-20 re-sort), the ceiling in §2 lifts and the complementarity
  argument gets much stronger.
- **If collaborator sharing is a hard, near-term requirement** and replicating
  Engram rows turns out to be materially harder than shipping markdown files,
  `shared-project-vault` earns a second look — on transport grounds, never on
  retrieval-quality grounds.
- **If measured latency regressed to seconds** on his real hardware in normal
  use, option 2 becomes urgent rather than merely correct.

---

## 7. Cheaper wins available in Engram itself

Falling out of the measurements, independent of which option is chosen: **[M]**

1. **Paraphrase top-1 is 14%.** Every one of the 14 paraphrase queries fell to
   `OR` mode. The AND→OR fallback rescues *recall* (64%@5) but destroys
   *precision at rank 1*. A word-tokenized FTS column beside the trigram one
   would take that to ~36% at BM25-only quality, before any embeddings.
2. **Neither retriever can abstain.** Both always return 5 rows; the negative
   queries scored 0%/0%. A relevance floor would let `query` return nothing
   instead of five confident wrong answers.
3. **Trigram is excellent at what it is for** — 100%@5, 86%@1 on identifiers.
   Do not replace it. Add beside it.

## 8. The 14 hand-written paraphrase queries, verbatim

Ground truth in parentheses; `v` = vault top-1 hit, `f` = trigram FTS top-1 hit.

| id | query | correct page | v@1 | f@1 |
|---|---|---|---|---|
| B1 | why does a shared notebook get more valuable the more you write in it | Compounding Knowledge | ✗ | ✓ |
| B2 | picking up a conversation where the last one stopped | Hot Cache | ✓ | ✗ |
| B3 | grouping search phrases into families without paying a subscription | Semantic Topic Clustering | ✓ | ✓ |
| B4 | spotting when a webpage quietly loses the signals that made it rank | SEO Drift Monitoring | ✗ | ✗ |
| B5 | which typefaces and colours should illustrations use | SVG Diagram Style Guide | ✓ | ✗ |
| B6 | who came up with this way of keeping notes | Andrej Karpathy | ✗ | ✗ |
| B7 | keeping the original material apart from what the assistant writes | Source-First Synthesis | ✓ | ✗ |
| B8 | how to reach my notes when the add-on is not installed | transport-fallback | ✓ | ✗ |
| B9 | a contest with cash prizes for building extensions | Pro Hub Challenge | ✓ | ✗ |
| B10 | choosing a folder arrangement style for my notes | methodology-modes | ✓ | ✗ |
| B11 | fetching passages only once somebody asks something | Query-Time Retrieval | ✓ | ✗ |
| B12 | shrinking many old diary entries into one shorter record | DragonScale Memory | ✗ | ✗ |
| B13 | the toolkit written by the person who created the note taking app itself | kepano-obsidian-skills | ✗ | ✗ |
| B14 | the curated write-up that sits between source documents and future answers | Persistent Wiki Artifact | ✗ | ✗ |

B6, B12, B13 and B14 are the hardest and both retrievers miss them; B13 in
particular requires knowing Kepano created Obsidian, which is an inference, not
a retrieval. If you judge any of these unfair, drop them and the paraphrase gap
narrows but does not close: on the 10 remaining, vault top-1 is 8/10 against
FTS 2/10.

---

# The confirming split, run against the live Engram corpus

The recommendation above rested on measurements taken over the vault's 49
upstream pages — somebody else's prose. Its own strongest counter-argument
was that this human's real observations are identifier-dense markdown, the
class where a trigram index wins. That experiment has now been run against
the live corpus.

## Method

584 live `labdrian-sdd-overlay` observations, embedded once with
`nomic-embed-text` (first 2000 chars of title + content). Three arms over
the same corpus:

- **A — trigram FTS**, exactly as shipped: the 67 stopwords extracted from
  `internal/engram/search.go`, AND-joined, falling back to OR when AND
  yields nothing, ranked by bm25.
- **B — embeddings only**: cosine over all 584, no FTS.
- **C — FTS recall + embedding rerank**: FTS's top 50, reordered by cosine.
  This is the arm that matters, because it is what "port the vault's rerank
  onto Engram" would actually mean — nobody embeds 584 rows per query.

**Identifier queries were derived mechanically**, not chosen: Go-shaped
symbols and file paths extracted from observation content, keeping only
those appearing in 1–3 observations so the ground truth is unambiguous;
14 non-overlapping ones taken.

**Paraphrase queries were hand-written**, which is unavoidable, and then
verified: a script measured what fraction of each question's content words
appear in its target's text. All ten scored between 0% and 36%, so each is
genuinely lexically distant from the document it is supposed to find. A
question that reuses its target's vocabulary would prove nothing.

## Result

| arm | identifier@1 | identifier@5 | paraphrase@1 | paraphrase@5 |
|---|---:|---:|---:|---:|
| A trigram FTS (shipped) | **100%** | **100%** | 10% | 10% |
| B embeddings only | 0% | 7% | **40%** | **70%** |
| C FTS recall + rerank | 93% | 100% | 30% | 30% |

n = 14 identifier, 10 paraphrase.

## What this changes

**Complementarity is confirmed, and it is starker than on the vault corpus.**
Trigram FTS is perfect on identifiers and near-useless on paraphrase;
embeddings are the exact inverse. Neither is sufficient alone.

**But reranking an FTS pool does not capture the embedding win.** Arm C
reaches 30% on paraphrase where arm B reaches 70%, because the bottleneck is
FTS *recall*, not ordering: the answer is frequently not in the pool at all,
and no reranker recovers a document its candidate set never contained. The
recommendation to "port the rerank onto Engram" therefore buys 10% → 30%,
not 10% → 70%. That is a real gain and it is less than half of what the
framing implied.

**Blind reranking also costs something on the case FTS already nails**:
identifier@1 falls from 100% to 93% under arm C.

**What the numbers actually argue for is a union, not a rerank** — both
retrievers run and their results merged, which is structurally what
`query` already does. The defect is not the architecture; it is that the
embedding half is pointed at the wrong corpus. It indexes 49 pages of
upstream documentation instead of this human's 584 observations.

And since `EmitPage`'s `renderBody` copies `obs.Content` byte for byte,
indexing promoted pages is indexing the observations with extra steps. The
cheap form of this is an embedding index over Engram rows directly: no
promotion, no pages, and none of `shared-project-vault`.

## Limits of this measurement

n is small — 14 and 10. The identifier set is mechanical and trustworthy;
the paraphrase set is ten questions written by the same agent that ran the
experiment, and lexical distance was verified while relevance judgement was
not independently reviewed. The corpus is one project's memory, not a
general claim about retrieval. Embeddings were computed over the first 2000
characters, so long observations were judged on their opening.
