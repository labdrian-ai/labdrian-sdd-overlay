# SDD Roadmap — {Project Name}

**Date**: {current date}
**Version**: {1 if first, n+1 if update}
**Based on**:
- Manifest context: `project/{project}/manifest/context`
- Manifest rules: `project/{project}/manifest/rules`
- Architecture: `project/{project}/architect/final`
- Existing SDD history: {list archived/completed changes, or "none"}

---

## Sequence Summary

| Order | SDD-id | Status | Depends on | Objective |
|-------|--------|--------|-----------|----------|
| 0 | {SDD-id} | completed | — | {foundational, already built} |
| 1 | {SDD-id} | planned | {SDD-id} | {goal} |

---

## Detail per SDD

### {SDD-id} — {short goal}

- **Status**: completed | in-progress | blocked | deferred | superseded | planned
- **Objective**: {one sentence describing what this change delivers}
- **Derived from**: {>=1 citation from manifest/architecture/existing SDD — if none, write `[PENDING DECISION]`}
- **Dependencies**: {SDD-ids that must land first, or "none"}
- **Acceptance evidence**: {observable signal that proves this SDD is done}
- **Risk if done too early**: {concrete risk of premature execution}
- **SDD entry command**: `/sdd-new {change-id}`

#### Tracking (update across the cycle)

Two sentinels, and they mean different things. `[PENDING]` means the value is not
recorded yet and a later pass can fill it. `[NOT MEASURED — reason]` means the value
will never be filled under the current contract, and the reason says why. Never
substitute one for the other: a roadmap that renders "waiting on a record" and "no
such measurement exists" identically is lying about which one it is.

| Field | Value | Source |
|-------|-------|--------|
| Estimate (before start) | {… or `[PENDING]`} | `sdd/{change}/estimate` |
| Expected human checkpoints (planned) | {… or `[PENDING]`} | `sdd/{change}/estimate` — `expected_checkpoints` |
| Planned review slices | {… or `[PENDING]`} | `sdd/{change}/entry` — number of `review_slices` entries |
| **Elapsed calendar time, tiering go-ahead → merge** | {… or `[PENDING]`} | `sdd/{change}/actuals` — `total_wall_clock_hours`. Includes interruption gaps. Right edge is the merge-commit committer timestamp, NOT archive: post-merge archive-authorization lag is bookkeeping and does not count as development time. Mark the value **reconstructed** rather than measured when the record says so. |
| **Human checkpoints realized (same boundary)** | {… or `[PENDING]`} | `sdd/{change}/actuals` — `checkpoint_count`. One unit per distinct human round-trip reply. Note the durable-vs-reconstructed split the record discloses in prose. |
| Realized review slices | {… or `[PENDING]`} | `sdd/{change}/actuals` — `realized_slices` figure inside `variance_vs_plan`; there is no structured field for it |
| Implementation effort (agent compute) | `[NOT MEASURED — R-019: no durable per-phase duration source]` | Left unpopulated at source by `inception-pipeline` closure-feedback. Orchestrator telemetry is session-transient, the `sdd-attempt` ledger has no timestamp fields, and transcripts carry no structured subagent durations. |
| Verification effort (agent compute) | `[NOT MEASURED — R-019, same reason]` | Same as above. |
| Post-review fix effort (agent compute) | `[NOT MEASURED — R-019, same reason]` | Same as above. |
| Human review duration | `[NOT MEASURED — no field exists]` | The actuals record has never carried a human-review-duration field. This is a different fact from the three rows above: those are deferred, this one was never specified. |
| Review findings | {… or `[PENDING]`} | Prose only, from the change's `verify-report.md` / review receipt — no structured field |
| Approval decision | {… or `[PENDING]`} | `sdd/{change}/actuals` — `approval_decision` |
| Deviations from the original plan (and why) | {… or "none"} | `sdd/{change}/actuals` — scope-drift and variance prose |
| Sequencing impact (reordering/scope) | {… or "none"} | This roadmap's own analysis |

> The four agent-compute and human-review rows above are kept, not deleted, on purpose.
> Deleting them would erase the distinction between "this project never tracked effort"
> and "this project deliberately declines to fabricate an effort number it cannot source",
> and it would leave a reader unable to explain why two pre-R-019 records on this project
> DO carry compute-hour figures while nothing written since does. The row is the disclosure.

---

{Repeat the detail block per SDD. Foundational/completed items first, then the planned sequence in dependency order.}

---

## Legacy sentinel note

Roadmaps written before this template used the Spanish sentinels `[PENDIENTE]` and
`[PENDIENTE DE DECISIÓN]`. They remain valid in those files and MUST NOT be
retro-translated: the Language Rule in `../../_shared/pre-sdd-contracts.md` applies to
new writes only and explicitly tolerates legacy artifacts until their next full
regeneration. Use `[PENDING]` / `[PENDING DECISION]` in new roadmaps and in any block
being fully regenerated; leave existing Spanish sentinels alone otherwise.
