# Blind validation — scored

Query set frozen at `a7a6f9c` **before** this scoring ran. 36 scoreable of 40
authored.

## Results

| class | n | arm | hit@1 | hit@5 |
|---|---:|---|---:|---:|
| identifier | 20 | trigram FTS | 90% | 95% |
| | | embeddings | 70% | 75% |
| | | **union** | 80% | **95%** |
| paraphrase | 16 | trigram FTS | 6% | 12% |
| | | embeddings | 38% | 50% |
| | | **union** | 38% | **56%** |

The union's @5 matches or beats both arms in both classes, as the structural
guarantee requires.

## Routing accuracy, and a metric that was wrong first

The gate's job is choosing which arm owns rank 1. Measured as union hit@1 it
reads 80% / 38% — and that would have selected **Branch B**, the wrong branch,
because it conflates the gate's choice with the chosen arm's own ceiling.
Paraphrase union@1 equals embeddings@1 exactly, which is the signal that the
gate is routing correctly and 38% is simply what embeddings reach.

Measured as the gate's actual job — of the queries where at least one arm
holds the truth at its own rank 1, how often did the gate route to an arm
that holds it:

| class | decidable | gate right | accuracy |
|---|---:|---:|---:|
| identifier | 18 | 16 | **89%** |
| paraphrase | 7 | 6 | **86%** |

Both clear the 80% threshold, and both land in the 80–89% band, so
**Branch A ships with the routing accuracy published** rather than recorded
only in a decision document.

## What this does not establish

**The paraphrase figure rests on 7 decidable queries.** One different outcome
takes it to 71% and flips the branch. It is above the threshold on the
evidence available and it is thin, and nothing about the 80% rule was written
with an n of 7 in mind.

**The earlier curated set was unrepresentative in both directions.** It
measured embeddings at 0%/7% on identifier queries; the blind set measures
70%/75%. The difference is that the curated identifier queries were bare
symbols, and the blind author wrote the descriptive kind a person actually
types. The complementarity is real but less absolute than the first table
implied, and the first table should not be cited on its own.

Trigram FTS also reads 90% here against 93–100% there, and paraphrase 6%
against 10% — same direction, and consistently softer once the queries were
not written by someone who had watched the retriever behave.

## PR-4: the predicted failure happened, within hours

The line above — "one different outcome takes it to 71% and flips the
branch" — was not hypothetical. PR-4 shipped `routeRank1` with a real
defect: an early return on `matchMode == MatchAny` that pre-empted the
token-shape rule instead of being ORed with it, forcing every widened
query (100% of blind paraphrase queries widen) to the FTS arm regardless
of shape. `TestGateRoutingAccuracyOnBlindSet`, built to pin exactly this
class of regression, measured it at **22%** paraphrase routing accuracy
against this same frozen `queries.json` set — a severe, unambiguous
failure, confirmed by a mutation proof (reverting the fix reproduces it).

Once fixed (`shape OR matchMode==MatchAll`, the decision record's own
validated rule, §4.3), two independently honest re-measurements of the
corrected gate against this exact frozen set **disagreed with each other**
about whether it clears 80%, purely because of embedding-index freshness
at measurement time — a difference of exactly one query's decidability:

| measurement | decidable | right | accuracy |
|---|---:|---:|---:|
| this apply batch (592/592 rows embedded) | 9 | 7 | 77.8% |
| maintainer re-run (591 rows embedded) | 8 | 7 | 87.5% |

One query is worth 12.5 percentage points at this n. Both measurements are
correct; both are honestly obtained; they land on opposite sides of the
80% line. **The threshold is not measurable at paraphrase's decidable
sample size (n≈8), and no amount of re-measuring the same 16-query class
will fix that** — the fix is a larger blind paraphrase set, which does not
exist yet.

**Maintainer's ruling**: accept the corrected gate (its defect-vs-fixed
separation — 22% vs 78–88% — is unambiguous at this n, even though the
80% line itself is not), and declare the 80% threshold unmeasurable at
this sample size rather than resolve it by picking whichever
re-measurement is more convenient. `TestGateRoutingAccuracyOnBlindSet`
now asserts a floor (50%) chosen to separate the defect from the fix, not
the design's own 80% acceptance line, with the reasoning in the test's own
comment.

**Open debt, recorded here so it is found before the 80% rule is relied on
again**: the published routing-accuracy numbers this change carries (89%
identifier / 86% paraphrase in this file above; 89%/86% and 94%/88% in
`longterm-mem-query`'s R-059 and `longterm-mem-embedding-index`'s spec)
are published WITHOUT their n. Publishing "86%" without publishing "n=7"
is the same overclaim this change exists to catch elsewhere — a number
without the sample size behind it looks like more evidence than it is.
Before the 80% threshold is used again to decide a branch or gate a
release, the blind paraphrase set needs enough queries (a rough rule of
thumb: n≥30 before a single query is worth less than ~3 points) for an
80% line to mean something.
