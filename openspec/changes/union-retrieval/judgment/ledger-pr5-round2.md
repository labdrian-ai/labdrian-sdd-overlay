# Judgment Day — PR-5, round 2 (scoped re-judgment)

Frozen fix delta sha256 `0ef5707a0a42ccce8ccb8dd6e6cde4a231de7e039d2b8e8673489610cfdc0dc0`.
Both judges saw only the round-1 ledger plus that delta, with explicit
permission to report defects caused by the correction itself.

## Round-1 findings

| ID | Verdict | Both judges | Deciding line |
|----|---------|-------------|----------------|
| PD-1 | CLOSED | yes | `cmd_sync.go:102` + `sync.go:188-194`; differentially tested against Propagate's own output |
| PD-2 | CLOSED | yes | `MatchLinkedEngramRow` deleted; zero Go references remain |
| PD-3 | CLOSED | yes | `query.go:745-747` now names `matchLinkedObservation` |
| PD-4 | CLOSED | yes | `jd4_test.go:74-76` asserts which arm's title survives |
| PD-5 | CLOSED | yes | `project_resolve.go:165-169` conditions the write on the policy |

Orchestrator verification, independent of both judges: every round-1 fix
was mutated back out under `-count=1` and produced its own named red,
including the one that matters most — swapping `matchLinkedObservation`'s
two loops, which left the whole package GREEN before PD-4 and is red now.

## New findings, introduced by the correction — both corroborated

### PD-6 — duplicated walk — `internal/promote/propagate.go`

Closing PD-1 gave the preview its own copy of Propagate's loop and shared
only the decision inside it. That is the defect the preview exists to
prevent, one level out: the copies agreed the day they were written and
nothing structural kept them agreeing.

Fixed. `eachPatchTarget` is the one walk; Propagate visits it to patch,
Plan visits it to count.

### PD-7 — one failure counted twice — `internal/promote/sync.go`

Plan previews two passes and both call `findPromotedPage` on the same
observation, so a page with unparseable frontmatter arrived in
`plan.Failed` twice with an identical message. Verified by probe before
being believed: one broken observation, two entries, byte-identical.

Fixed by `mergeFailures`, which drops only an EXACT repeat — two different
failures for one observation are two real problems, and collapsing those
would be the same lie pointing the other way.

## The one that was found by mutation, not by a judge

`mergeFailures`'s doc comment claimed exact-repeat-only. Collapsing the key
to the bare observation ID left the entire suite green: the claim was
unenforced, and `TestPlan_OneBrokenObservationIsReportedOnce` passes just
as well under the wrong rule. `TestMergeFailures_DropsOnlyExactRepeats` now
pins it, and that mutation is red.

This is the fifth distinct instance in this change of one defect class: a
record that no longer describes what it records. Found this time in a
comment written to close the fourth.

## Verdict

Round budget: two rounds, both spent. Severe findings open: none. Both
round-2 findings corrected and mutation-proven. `go test -count=1 ./...`
green across all three modules.

JUDGMENT: APPROVED
