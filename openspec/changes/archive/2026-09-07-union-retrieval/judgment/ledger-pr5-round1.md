# Judgment Day — PR-5 (open findings), round 1

Target: branch `union/pr5-open-findings`, scope `union/pr4-arm..union/pr5-open-findings`.
Frozen delta sha256 `26dc0dcc1f08b3b5cc09b53e579eface741022ddce385a9bdbd51a47145bdf6b`.
Two blind judges, parallel, identical scope.

## What the branch claimed to close

| Item | Verdict | Both judges | Deciding line |
|------|---------|-------------|---------------|
| JD-4 — linked pair ignored the embedding arm | CLOSED | yes | `query.go:887-906`, the second loop over `embedRows` |
| doctor wrote the ledger it called read-only | CLOSED | yes | `project_resolve.go:178-180` `if policy == recordDerived`, wired at `cmd_doctor.go:52` |
| sync had no preview | CLOSED as scoped, INCOMPLETE | yes | `sync.go` `decidePromotion` shared by Plan and Sync — but see PD-1 |

## Findings

### PD-1 — CRITICAL — corroborated by both judges — `cmd/longterm-mem/cmd_sync.go:92-110`

`sync --dry-run` previews `promote.Plan` only. The real command runs TWO
passes: `promote.Sync` and then `promote.Propagate`, and prints
`promoted %d observation(s), patched %d page(s)`. Propagate calls
`PatchStatusFields` on already-promoted pages — a real write to existing
files — and the dry run neither runs it, counts it, nor mentions it, while
printing `nothing was written`.

Orchestrator-verified: `cmd_sync.go:115` prints both counts on a real run;
`propagate.go:70-71` writes the page. The preview describes half the command
it previews.

This is the same defect the commit that introduced it argued against, in
its own words: a preview that stops describing the run. It was built
against `Sync` while the command runs `Sync` and `Propagate`.

Neither `plan_test.go` nor `sync_dryrun_test.go` seeds an already-promoted
page with a pending status change, so the whole re-sync scenario — as
opposed to the first-sync scenario the change was written for — is
unguarded.

### PD-2 — WARNING — corroborated by both judges — `internal/query/query.go:908-924`

`MatchLinkedEngramRow` is left exported and unmodified, still consulting
only the FTS rows: it still carries the exact JD-4 defect. It now has zero
callers in the repository. Dead code is not the problem; dead EXPORTED code
still documented as the canonical matcher is a trap that silently
reintroduces JD-4 for whoever picks it up.

### PD-3 — WARNING — judge A, orchestrator-verified — `internal/query/query.go:745-747`

`mergeResults`'s doc comment still says "3b.8: MatchLinkedEngramRow is the
extracted matcher, reused unchanged by promote/MCP query later". It calls
`matchLinkedObservation`, and no other package calls the named function at
all. Verified by reading both.

### PD-4 — WARNING — judge A, orchestrator-verified BY MUTATION — `internal/query/jd4_test.go`

`TestMergeResults_EmbedOnlyLinkDoesNotStealAnFTSRow` is named for the
collapse's direction and cannot detect it. Its FTS row and embed row carry
the identical `Title`, and it asserts only `len(merged)` and `Sources[0]`.

Proven, not argued: swapping the two loops inside `matchLinkedObservation`
so the embed arm is searched first leaves `go test -count=1
./internal/query/` fully GREEN. A test that cannot fail for the thing it is
named after.

### PD-5 — WARNING — judge B, orchestrator-verified — `cmd/longterm-mem/project_resolve.go:165-168`

`adoptFromWorkingDirectory`'s comment states unconditionally that the ledger
is "consulted BEFORE resolving and written AFTER". Under the
`doNotRecordDerived` policy this same change introduced, it is never
written. The comment is false on the path the change added.

## Tally

Confirmed severe: 1 (PD-1). Suspect: 0. Contradictions: 0. WARNING: 4.
All five are `causal_disposition: introduced` — every one caused by this
delta, none inherited.
