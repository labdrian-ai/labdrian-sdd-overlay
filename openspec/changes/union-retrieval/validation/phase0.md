# Phase 0 — the gates, resolved

All three ran before any PR-1 code was written. Phase 0 exists because
`shared-project-vault` was planned at 3,000 lines before its evidence.

## 0.1–0.3 Blind validation → **Branch A**

Forty queries by an author kept blind: it read the corpus but never ran a
retrieval, never saw the routing rule, never read the design. Four dropped —
three whose ground truth the author itself flagged as disputed, one caught by
a mechanical check at 68% content-word overlap with its target. 36 scoreable
against a floor of 34. Frozen at `a7a6f9c`, scored in `69fb301`.

Routing accuracy 89% (identifier, n=18 decidable) and 86% (paraphrase, n=7).
Both inside the 80–89% band, so Branch A ships with the number published.

**The first metric was wrong and selected Branch B.** Measuring the gate as
union hit@1 conflates its choice with the chosen arm's ceiling; paraphrase
union@1 equalled embeddings@1 exactly, which is the signal the gate was
routing correctly. The gate's job is which arm owns rank 1.

## 0.4 Snippet budget → **RED, with the number**

Worst case — 10 rows, every diagnostic, a `Standing` on every Engram row —
encodes to **9,415 bytes against an 8,000 ceiling** at the shipped 480-char
budget, and `capResponse` recovers by dropping **2 rows**. About **338**
characters per row fits.

The design reasoned 480 from an estimated `Result`. Measured on a real one it
is optimistic, and the recovery deletes exactly what the union exists to
deliver, in the layer forbidden to re-rank and so unable to choose which loss
hurts least. `internal/query/budget_gate_test.go` stays red until PR-1.

## 0.5 Arm-D reproduction under the design's shape → **passes, and improves**

The design specifies the embedding input as `title‖NUL‖content[:limit]`; the
harness had used `title‖newline‖content`. A different separator is a different
embedding, so the ranking had to be re-measured rather than assumed.

| shape | ident routing | para routing | para B@1 | union ident@1 |
|---|---:|---:|---:|---:|
| harness (`\n`) | 89% | 86% | 38% | 80% |
| design (`NUL`) | **94%** | **88%** | **44%** | **85%** |

Branch A under both. PR-3 is unblocked; had this failed, `internal/vecindex`
would not have been written at all.

## PR-4: Branch A shipped

`routeRank1` (`internal/query/gate.go`, unwired since PR-1's 1.7/1.8) is now
wired live into `mergeResults` (`internal/query/query.go`): when both
`engram-fts` and `engram-embed` are requested, the gate decides which
source's rows are offered first to `interleaveEngramSources`, deciding rank
1 only — `interleaveEngramSources` itself never consults the gate, so the
`@5` union guarantee (R-058) is provably unaffected by the gate's decision
either way (`TestAnIncorrectRank1RoutingDoesNotShrinkTheGuarantee`,
`TestMergedSetContainsEachRequestedSourceRow`).

The routing accuracy this shipped on is the 80–89%-band number from 0.1–0.3
above (89% identifier / 86% paraphrase), also published in
`openspec/specs/longterm-mem-embedding-index/spec.md` per the 80–89% row of
design's threshold table. Branch B (fixed FTS-first order, `gate.go` deleted
or left unwired) was not taken.

The corpus was 584 rows when first embedded, 586 at the blind scoring, and
**591** at gate 0.5 — memory this session kept saving. Every measurement here
is a snapshot of a moving corpus, and the index answered each time slightly
behind it. That is the condition the design's per-response coverage field
exists to surface, observed three times while building the thing that
surfaces it.
