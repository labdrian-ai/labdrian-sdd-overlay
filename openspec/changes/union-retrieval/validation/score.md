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
