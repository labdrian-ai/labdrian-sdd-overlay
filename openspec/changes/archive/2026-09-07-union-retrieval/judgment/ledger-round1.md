# Judgment Day — frozen ledger, round 1

target_identity: `165b6360332ee5ec34b9c3b29686284b2bfdd601a27ca773c8d6482755c9e4e1`
scope: `origin/main..union/pr4-arm` (base `f34d299`, tip `cd44cf8`)
judges: `jd-judge-a`, `jd-judge-b`, blind and parallel, identical scope

## CONFIRMED — both judges, severe

**JD-1 · `dropLargestSourceRow` drops a non-deterministic row on a tie** — status: fixed
`internal/query/query.go:522-534`

```go
var largest string
for name, n := range counts {
    if n > counts[largest] { largest = name }
}
```

Go map iteration order is unspecified, and the comparison is strict `>`, so
when two sources hold the same number of slots the one that loses a row is
whichever key the runtime happens to visit first. Identical calls against an
identical corpus can return different bodies.

Neither judge found a test covering it, and Judge B found something sharper:
`TestSnippetBudgetGate_WorstCaseFitsWithoutDroppingRows` builds an **exact
5-vault/5-FTS tie** and then asserts `dropped == 0`, so the fixture that most
resembles the defect is the one test guaranteed never to exercise it.

## SUSPECT — one judge each, both corroborated by the orchestrator

The protocol records a single-judge finding as suspect and forbids auto-fix.
Both were nonetheless verified directly against the code before being written
here, and neither rests on the reporting judge's word.

**JD-2 · A swallowed coverage error can serve a negative `unindexed`** — status: fixed
`internal/query/embedarm.go:73-97` — reported by Judge A

```go
if live, err := store.CountLiveObservations(project); err == nil {
    coverage.Live = live
}
...
coverage.Unindexed = coverage.Live - coverage.Indexed
```

The error is discarded with no diagnostic, `Live` stays zero, and `Unindexed`
becomes negative while `Indexed` is computed from a separately obtained map.
It ships as a caller-facing fact with no `omitempty`. Every other degradation
path in this same file attaches a diagnostic; this one does not.

What makes it worse than an ordinary bug is where it sits: coverage exists so
nobody receives a silently incomplete answer, and its own doc comment calls a
caller who forgets the subtraction "this module's unforgivable failure". The
field can emit exactly that.

**JD-3 · A vault row can be marked truncated with nothing to fetch** — status: fixed
`internal/query/query.go:631-641` — reported by Judge B

`renderRowSnippet` sets `row.Snippet, row.SnippetTruncated = snippet,
truncated` unconditionally. A vault row has no `Content`, so its already-cut
snippet is re-cut, and if the newly allocated share is smaller it is marked
`snippet_truncated: true` while `FullLength` stays zero.

The field's own doc comment, four lines above, says a vault row "leaves both
zero: there is no full body here to measure and no claim to make about one."
The code falsifies its own contract, and emits the truncation marker without
the machine-readable half this change insisted must ship with it or not at
all.

## INFO

**JD-4 · Linked-pair collapsing never consults the embedding arm** (WARNING,
inferential, latent) — `internal/query/query.go:713-729`. `MatchLinkedEngramRow`
is called only against FTS rows, so a caller asking for `vault` +
`engram-embed` would see one memory twice. Latent because production still
wires `NoLinkResolver`; it becomes visible when a real resolver lands.

## Contradictions

None. The judges agree on JD-1 and neither disputes the other's findings.
