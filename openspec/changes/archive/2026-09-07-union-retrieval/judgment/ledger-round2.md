# Judgment Day — round 2 (scoped re-judgment of the correction)

Target: fix commit `3a681d3` on `union/pr4-arm`.
Frozen delta: `git show 3a681d3`, sha256 of the patch
`a05899e331ff47756db3d41ba0ec2417991e1f4f504ebcda4124587f764af0f6`.
Scope: the correction only. Round 1 is closed; JD-4 was excluded by
instruction as informational and latent.

Both judges ran blind and in parallel over the frozen round-1 ledger plus
that delta, with explicit permission to report defects caused by the fix
itself.

## Round-1 findings

| ID | Verdict | Both judges agree | Line that makes the defect unreachable |
|----|---------|-------------------|----------------------------------------|
| JD-1 | CLOSED | yes | `query.go:550` `sort.Strings(names)`, loop over the sorted slice |
| JD-2 | CLOSED | yes | `embedarm.go:161-163` clamp; `embedarm.go:79-82` names the failure |
| JD-3 | CLOSED | yes | `query.go:675-679` sets `SnippetTruncated` only when `hasFullBody` |

Orchestrator verification, independent of both judges: each fix was mutated
back out and the suite re-run with `-count=1`. Four mutations, four reds,
each red naming its own defect:

- remove `sort.Strings` -> `dropped vault 8 times and fts 42 times across identical calls`
- remove the negative guard -> `coverageUnindexed(0, 3, false) = -3, want 0` and `(2, 5, true) = -3`
- neutralise the diagnostic append -> `diagnostics = [], want one "live_count_unreadable"`
- restore the unconditional assignment -> `SnippetTruncated = true for a vault row (no Content)`

Every new test can fail, and fails for the reason it states.

## New findings, introduced by the correction

### JD-5 — WARNING — `internal/query/embedarm.go:157-165` — uncorroborated (judge A)

`coverageUnindexed` clamps to `0` when `live < indexed` while `liveKnown`
is true — a stale-count race the function's own doc comment names. That
path emits no diagnostic: `DiagnosticLiveCountUnreadable` is appended only
when `liveErr != nil`. Before the fix the same inputs produced a negative
number: wrong, but visibly wrong. After the fix they produce `0`, which is
byte-for-byte indistinguishable from full coverage.

This is the defect family JD-2 was raised to eliminate, surviving one call
deeper. `embedarm_test.go:247` (`live: 2, indexed: 5, liveKnown: true,
want: 0`) certifies the masking as correct rather than guarding against it.

Not fixed in this cycle: the protocol allows two rounds and both are spent.
Recorded as a follow-up. Smallest known fix: emit a diagnostic whenever the
clamp fires with `liveKnown == true`, and assert it in that table case.

### JD-6 — INFO — `internal/query/query.go:620-648` — uncorroborated (judge B), consequence disproved

Judge B observed that the JD-3 fix removes Content-less rows from
`stillTruncated`, so a vault row no longer competes for the leftover-byte
second pass. The mechanism is real. The consequence is not: `renderRowSnippet`
re-renders a Content-less row from `row.Snippet`, which the first pass has
already clipped, so a larger budget returns the same bytes.

Measured directly, 900-byte body, share 200 then share2 480:

    vault row:  original=900  afterShare=179  afterShare2=179
    engram row:               afterShare=179  afterShare2=459

The vault row gained nothing from the second pass before the fix either.
Downgraded from WARNING to INFO: no behaviour a caller can observe changed.

## Verdict

Three confirmed severe findings closed and independently proven load-bearing.
Zero severe findings in round 2. Two warnings raised, neither corroborated by
both judges; one measured to have no observable consequence, one recorded as
an open follow-up with its fix named.

JUDGMENT: APPROVED
