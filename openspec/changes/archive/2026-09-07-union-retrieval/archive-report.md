# Archive Report: union-retrieval

**Change**: `union-retrieval`
**Archived**: 2026-09-07
**Phase**: archive
**Store**: hybrid (openspec files + Engram)

## What shipped

Union retrieval over Engram's own corpus: an embedding arm alongside FTS, a
rank-1 routing gate deciding which arm answers first, and a hard server-side
response ceiling. Delivered as a chain of PRs #275–#283 rather than one change,
each carrying its own argument for its size.

## Capability promotion

| Capability | Promoted to | Status |
|---|---|---|
| `longterm-mem-query` | `openspec/specs/longterm-mem-query/spec.md` | promoted |
| `longterm-mem-ops` | `openspec/specs/longterm-mem-ops/spec.md` | promoted |
| `longterm-mem-embedding-index` | `openspec/specs/longterm-mem-embedding-index/spec.md` | promoted in the archiving commit |

The third was unpromoted until archive-reconcile named it. Its own message is
the record worth keeping: a stranded change is not merely a folder in the wrong
place — when a capability is unpromoted the canonical spec set is incomplete,
and nothing else in this repository notices.

## Cycle Timestamps (Verification & Content-Binding)

**Landing commit recorded explicitly by identity, never discovered by tree
matching** (R-016/R-017/R-018).

| Anchor | Value | Resolved as | Authority |
|--------|-------|---|---|
| **t0** (cycle start) | commit `216d399` `docs(decisions): measure whether the vault earns its cost, and design the union`, committer `2026-09-06T19:53:42+00:00` | first commit of this change's own work — the measurement that decided the union was worth building | committer timestamp from `git log --format=%cI` |
| **t1** (cycle end / merge) | commit `89aa0dd` (PR #282 merge), committer `2026-09-07T19:16:56+00:00` | **explicitly recorded**: PR #282 is the last PR carrying this change's own artifacts, and its merge is where they reached `main` | GitHub `mergeCommit.oid` = `89aa0dd5bdb0705c9dad3314014297597227561f`, `mergedAt` = `2026-09-07T19:16:56Z`, both read from the API rather than inferred |
| **Duration** | 23 h 23 m 14 s ≈ 23.39 hours ≈ 0.97 days | computed as t1 − t0, both in UTC | both endpoints carry explicit `+00:00` offsets, so no zone convention is assumed here |
| **Resolution path** | **self-asserted** | t0 from this change's earliest own commit; t1 recorded from the API. No independent tree exists to check either against | per R-016/R-017/R-018 |

**Why this anchor is self-asserted.** A first draft of this section claimed
the stronger outcome, and the archive-anchor gate refused it: *"with no
independent tree to check against there is nothing that could have failed, and
an unfailable check reported as verification is a fabricated assurance."* It
was right. The cross-check that draft offered -- `git show -s --format=%T` on
the same merge commit whose identity was being recorded -- compares a commit to
its own tree and cannot fail. Receipt-driven review is off for this repository,
so no `approved_tree` exists to check these anchors against. The timestamps are
read from real sources and recorded honestly; nothing independent corroborates
them, and this record now says which of the two it is.

**Why t1 needed no tree-collision handling**: the eight chain merges landed
within seconds of each other (`4ef3bee` 16:16:27 through `89aa0dd` 16:16:56,
local −03:00), but each carries a distinct tree because each PR changed files.
The identity was taken from the API regardless — recording it is the rule, and
the absence of a collision is not a licence to discover it.

## Judgment record

Two full Judgment Day rounds, ledgers preserved in this folder under
`judgment/`. Round 1 confirmed three severe findings; round 2 raised JD-5
against round 1's own correction. Every fix was proven load-bearing by
re-introducing the defect under `go test -count=1`.

Two defects were found by mutation that neither blind judge saw: a test named
for an ordering it could not detect, and a doc comment whose claim nothing
enforced. That is the durable finding of this cycle — a mutation that produces
no red is itself the result.

## Open follow-ups at archive time

- **PR #283** (`union/pr6-coverage-snapshot`) was still open when this change
  was archived. It replaces the two-read coverage split with one snapshot and
  deletes the clamp, the diagnostic and the untestable branch this change
  shipped. Recorded here rather than silently rolled in: the change is archived
  as it landed, not as it was later improved.
