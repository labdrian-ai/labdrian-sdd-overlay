# Archive Report: gadu-portable-operator

**Archived by**: sdd-archive executor
**Branch**: `chore/archive-stranded-changes`
**Archive date**: 2026-08-11
**Artifact store**: hybrid (Engram + OpenSpec filesystem)
**Merge date (original implementation)**: 2026-06-27 (PR #54, `fe28245`)
**Target archive path** (move left to orchestrator — see "Folder Move" below):
`openspec/changes/archive/2026-06-27-gadu-portable-operator/`

## Gate Check — Native Review Receipt Gate (CRITICAL findings)

Read directly from the persisted verify-report, not taken on assertion:

- File: `openspec/changes/gadu-portable-operator/verify-report.md`
- Engram: `sdd/gadu-portable-operator/verify-report`, observation **#2810**
- `evidence_revision: sha256:64988cb0d3daec5c775c460b352c2d9b1a5fda6a6392a4c4f6288f7dd7343bb9`
- `verdict: fail`
- `blockers: 0`
- `critical_findings: 0`
- `requirements: 12/13`
- `scenarios: 30/31`

**Result: PASS.** `critical_findings: 0` and `blockers: 0`, confirmed by direct read of
both the OpenSpec file and observation #2810. The gate this phase enforces is on CRITICAL
findings, not on the verdict token. The `fail` verdict is a correct, deliberate artifact of
`gentle-ai sdd-verify-validate` refusing a `pass`/`pass_with_warnings` verdict against
incomplete evidence accounting (12/13 requirements, 30/31 scenarios) — driven entirely by
R-013, which this report documents below as permanently unsatisfiable and intentionally not
promoted. Verify pass 5's own text states this explicitly: "**0 CRITICAL findings and 0
blockers** — that is the gate the archive phase reads." Archive proceeds on that basis.

Separately, this executor has no shell access and could not independently query
`gentle-ai review status` (native review/RDD receipt state) for this change. No such gate
was flagged as applicable in the launch instructions, and none of the retrieved artifacts
reference an open native-review transaction for this SDD change. Recorded as a residual
unverified item, not a blocker — see Risks.

## Task Completion Gate

`openspec/changes/gadu-portable-operator/tasks.md` inspected directly: **17/17 tasks
checked** (`- [x]`), zero unchecked. Task IDs: A0, A1, A2, A3, A4, A5, A6, B0, B1, B2, B3,
B4, B5, B6, B7, B8, B9. This matches verify pass 5 §5 ("17/17 tasks checked, 0 unchecked")
and required no reconciliation.

## Artifacts Read (traceability)

| Artifact | Engram observation | OpenSpec file |
|---|---|---|
| proposal | not found in Engram (`mem_search` returned zero results) | `openspec/changes/gadu-portable-operator/proposal.md` |
| spec (delta specs) | not found in Engram (`mem_search` returned zero results) | `openspec/changes/gadu-portable-operator/specs/{gadu-canonical-source,gadu-generator,overlay-agent-route}/spec.md` |
| design | not found in Engram (`mem_search` returned zero results) | `openspec/changes/gadu-portable-operator/design.md` |
| tasks | not found in Engram (`mem_search` returned zero results) | `openspec/changes/gadu-portable-operator/tasks.md` |
| verify-report (pass 5) | **#2810** | `openspec/changes/gadu-portable-operator/verify-report.md` |
| apply-progress (units 1-4) | **#2813** | `openspec/changes/gadu-portable-operator/apply-progress.md` |

Proposal, spec, design, and tasks exist only as OpenSpec files for this change — no prior
`mem_save` call persisted them to Engram under the expected topic keys. This is recorded
here as a gap in cross-backend parity, not a blocker: the OpenSpec files are complete and
were read directly.

## Specs Synced (Promotion)

All three canonical spec files were absent before this archive (`openspec/specs/{domain}/spec.md`
did not exist for any of the three capabilities), so each promotion is a clean create with
zero merge/collision risk. Verify pass 5 §7 independently confirmed all three promotable
with no capability-name or requirement-ID collision.

| Domain | Action | Details |
|---|---|---|
| `gadu-canonical-source` | Created | 4 requirements (R-001, R-004, R-005, R-012), 9 scenarios — promoted verbatim |
| `gadu-generator` | Created | 3 requirements (R-002, R-003, R-010), 8 scenarios — promoted verbatim |
| `overlay-agent-route` | Created | **12 of 13** requirements promoted (R-006, R-007, R-008, R-009, R-011 + the 6 requirements shared with the other two files' numbering are counted once each); **R-013 excluded** — see below |

New files:
- `openspec/specs/gadu-canonical-source/spec.md`
- `openspec/specs/gadu-generator/spec.md`
- `openspec/specs/overlay-agent-route/spec.md`

## R-013 — Documented Historical Deviation (NOT Promoted)

`overlay-agent-route`'s delta spec contains 13 requirements. **12 were promoted. R-013
("E1 Experiment Documented Before Guard Is Sized") was deliberately excluded in its
entirety** — not promoted in any form, not silently dropped without explanation. This is a
maintainer decision, taken and recorded here so a future reader auditing 12/13 requirements
against the delta spec can find out why rather than assuming an oversight.

Two independent reasons, either of which would be sufficient alone:

**1. It is a change-time process gate, not a system property.** R-013's normative text
reads: "BEFORE any durability guard code is implemented for `agents/GADU.md`, the
implementor SHALL run experiment E1 ... and SHALL document the result in the overlay
implementation notes." This instructs the implementor of *this one change* about an
ordering constraint on *this change's own construction*. A canonical spec under
`openspec/specs/` describes the behavior of the running system for any future reader or
implementor — it is not a journal of how one historical change was built. A standing
system-level requirement to "run E1 before implementing guard code" is meaningless once
the guard code already exists and is shipped; there is no future implementor for whom this
instruction is actionable. This is the same category of malformation already corrected
in R-009 during this change's own apply phase, where "This change SHALL NOT ..." (change-scoped,
non-canonical phrasing) was rewritten to "The overlay SHALL NOT ..." (system-scoped,
canonical phrasing) before promotion — see apply-progress unit 1, described below.

**2. It is permanently unsatisfiable.** R-013's scenario demands: "the result (clobbers /
does not clobber) is written to the overlay implementation notes before any guard code is
sized, written, or merged." E1 was in fact run, but only half of it: the `gentle-ai sync`
half executed and its result is documented in `docs/e1-durability-probe.md`. The
`gentle-ai upgrade` half of E1 — the half that would produce the specific clobbers/does-not-clobber
verdict R-013's THEN clause demands — **never executed**, and per apply-progress unit 2,
was deliberately withheld because an unattended `gentle-ai upgrade` had already broken this
repository's review authority store once earlier in the same session, at a real cost of
recovery hours. R-013 also requires this to happen *before* any guard code is committed.
`git blame -L 730,736 bin/labdrian-overlay` places the deploy loop (the code that
constitutes the "guard") at commit `43fdcb1d`, dated **2026-06-25**. The BEFORE-guard-code
ordering window closed on that date. No action taken today, or at any future date, can
retroactively satisfy an ordering requirement whose window has already closed — even if
`gentle-ai upgrade` were run and documented right now, it would not be "before" guard code
that has existed since 2026-06-25. R-013 is therefore not merely incomplete; it is
structurally unsatisfiable regardless of future effort.

**What was NOT done, stated explicitly:** R-013 is not implied satisfied anywhere in this
archive. It is not present in `openspec/specs/overlay-agent-route/spec.md` in any modified
or narrowed form (unlike R-009, which WAS narrowed and promoted). Verify pass 5 counts it
as incomplete (12/13, 30/31) and explicitly refused to inflate that count to buy a passing
verdict. This report preserves that same honesty at the promotion boundary.

## Trail — Five Verify Passes, Four Apply Units

The value of this cycle is largely in its correction trail; it is recorded here rather than
only in the (now-archived) intermediate snapshots, per the Final-State Authority principle
that the archive report is the terminal record.

- **Verify 1** — CRITICAL finding: R-009 forbade emitting native opencode agent-definition
  files while `main` deliberately emits and deploys `opencode/agents/GADU.md`. This was not
  a fault of this change at merge time: `git diff --name-only fe28245^1 fe28245` shows zero
  `opencode/agents` paths, so R-009 held true when this change merged (2026-06-27). PRs
  #62/#64 (commits `9408f02`, `fcdd84a`, 2026-07-02) later added the OpenCode agent artifact
  and narrowed an in-code known-limitations comment to describe the new scope — without ever
  writing that narrowing back to the delta spec, leaving the spec stale relative to the
  shipped system for five weeks.
- **Apply 1** (unit 1) — Reconciled R-009 from change-scoped ("This change SHALL NOT ...")
  to system-scoped ("The overlay SHALL NOT ..."), narrowed to Codex-only (matching the
  shipped code), with an explicit attribution note naming PRs #62/#64 and the two commits
  that introduced the divergence. Requirement ID unchanged; scenario narrowed from
  "No GADU files in opencode or Codex agent directories" to "No GADU files in Codex agent
  directory."
- **Verify 2** — 0 CRITICAL findings. Counts at 11/13 requirements, 27/31 scenarios. The
  verifier declined to round these up to a passing verdict; four scenarios (two under R-005,
  two under R-008) lacked complete evidence at that point.
- **Apply 2** (unit 2) — Corrected a stale factual claim in `docs/e1-durability-probe.md`:
  `gentle-ai upgrade` had been recorded as unavailable; verified directly on 2026-08-11 that
  it now exists (installed `gentle-ai 2.3.0`). Annotated four scenarios (two under R-005,
  two under R-008) as MANUAL acceptance criteria.
- **Verify 3** — Accepted R-005's MANUAL annotation as legitimate (corroborated by passing
  format/placement tests; the last mile — loading a skill in a live third-party runtime —
  is genuinely not automatable in CI). **Rejected R-008's MANUAL annotation**: the
  verifier determined `gentle-ai upgrade` is a real, runnable, automatable command that was
  withheld for *safety* (an unattended run had already caused damage earlier in the session),
  which is a categorically different fact from "not automatable in CI." The false label was
  identified and flagged for correction.
- **Apply 3** (unit 3) — Rewrote R-008's normative clause away from the mutually-exclusive
  branch-selector text conditioned on an unrun `gentle-ai upgrade` probe, narrowing it to the
  guarantee actually engineered: GADU artifacts registered as overlay-managed, re-deployable
  after any `gentle-ai sync`/`upgrade`, drift detected by `cmd_sync_check`, restored by
  `cmd_apply`. Replaced the two branch-selector scenarios with two test-backed scenarios
  covering deployment and drift-detection. Removed the false MANUAL annotation Apply 2 had
  added.
- **Verify 4** — CRITICAL finding: `docs/e1-durability-probe.md` claimed the AC-5 restoration
  regression test was "exercised in `engine/installer/route_test.go`." This was checked
  directly and found false — zero matches for `AC-5|reapply|simulated sync` across `engine/`,
  and none of that file's 19 test functions performed a re-application cycle. The test did
  not exist.
- **Apply 4** (unit 4) — Wrote the missing test: extended `TestSyncCheck_DetectsMissingAgentFile`
  in `engine/installer/route_test.go` with a restoration phase (reapply, `os.Stat` the
  restored agent file, `sync-check` confirming `IN_SYNC`). Labelled explicitly as a
  **characterization test** — the restoration behavior (unconditional copy-on-missing-or-diff
  in `cmd_apply`'s deploy loop) predated the test and the test passed on first run — with no
  RED phase fabricated to make it look like a red-green cycle it was not.
- **Verify 5** (this pass) — **0 CRITICAL findings.** Verify 4's CRITICAL confirmed closed on
  three independent checks: (a) the test now exists at the cited location; (b) the document
  now accurately describes it (with one residual scope overstatement graded WARNING, not
  CRITICAL — see below); (c) the test was proven **non-vacuous** by neutralization —
  `engine/` and `bin/` were copied to a scratch directory *outside* the repository, the
  scratch copy's deploy loop was mutated to simulate an "already applied" ledger bug, and the
  restoration assertions failed under mutation while the first-deploy assertions still
  passed, confirming the test genuinely depends on the restoration behavior it claims to pin.
  The characterization label was also verified rather than trusted: `git blame` confirmed the
  deploy loop dates to commit `43fdcb1d`, 2026-06-25, and `git status --short` confirmed no
  production code was touched by this unit.

## Open Follow-Ups (Recorded, Neither Blocking Archive)

- **R-006 route-domain gap.** R-006 enumerates the route domain as `{skill, agent}`, but the
  real domain now includes a third value, `opencode-agent` (`overlay.manifest:80`;
  `bin/labdrian-overlay` branches on it at lines 318, 359, 385, 405). Every normative clause
  in R-006 holds and all of its scenarios pass — this is an incomplete *description*, not a
  contradicted *prohibition* (R-006 does not say "SHALL only accept {skill, agent}"). Follow-up:
  widen the enumeration by one word at a future edit.
- **WARNING-1 (scope overstatement).** `docs/e1-durability-probe.md`'s AC-5 correction section
  restates AC-5 as "both artifacts present + managed," but the restoration test added in Apply
  4 covers exactly one artifact (the Claude agent file). The integration fixture manifest at
  `route_test.go:669-675` has no `gadu-operator/SKILL.md` row, so the skill artifact is never
  removed or restored by any test. Suggested remediation (from verify pass 5): add the missing
  fixture row so the restoration half covers both GADU artifacts directly.
- **WARNING-4 (near-vacuous assertion).** The `strings.Contains(outRestored, "IN_SYNC")`
  assertion at `route_test.go:901-902` is near-vacuous in isolation: `sync-check` prints
  `IN_SYNC` for unrelated manifest rows, so this assertion stays satisfied even when
  `GADU.md` is absent — demonstrated empirically by the verify pass 5 neutralization
  experiment, where this specific assertion did not fire under the simulated bug while its
  two sibling assertions did. Not a defect in the test's overall proof (the sibling
  assertions carry it), but this specific line should not be cited as independent evidence.

## The Original Stranding

Implementation merged 2026-06-27 via PR #54 (`fe28245`), all 17 tasks checked, with no
`apply-progress` record persisted in either backend at the time (that per-task RED/GREEN
history is unrecoverable; the commit chain `fe28245^1..fe28245^2` is the only surviving
evidence of the original apply phase). The change was never archived at that time. It sat
in `openspec/changes/gadu-portable-operator/` (active, unarchived) for roughly 46 days before
being found — not by a human noticing, but by an archive-completeness guard. For the entire
46-day window, none of this change's three capabilities existed anywhere under
`openspec/specs/`: `gadu-canonical-source`, `gadu-generator`, and `overlay-agent-route` were
shipped and running in production with zero canonical-spec representation.

## Folder Move — Left to Orchestrator

This executor's toolset has no shell (no Bash, no `git mv`, no delete capability). Per
explicit instruction, **no folder move and no file copy was attempted**. The archive report
and the three promoted spec files above have been written in place. The change folder
`openspec/changes/gadu-portable-operator/` (containing `proposal.md`, `design.md`,
`tasks.md`, `specs/`, `apply-progress.md`, `verify-report.md`, and this `archive-report.md`)
remains in the active `openspec/changes/` directory. **The orchestrator must perform:**

```
git mv openspec/changes/gadu-portable-operator openspec/changes/archive/2026-06-27-gadu-portable-operator
```

using the merge-date prefix (2026-06-27, per PR #54's merge date) per the dated-name
convention for `openspec/changes/archive/`.

## Boundaries Honored

- `openspec/changes/archive/` was not read from or written to by this executor beyond the
  target path named above for the orchestrator's pending move.
- `openspec/changes/archive/2026-07-08-anti-generic-design-runtime-wiring/` was not touched.
- `docs/`, `engine/`, `tui/`, `bin/`, and all code/test files were not touched.
- No commit, stage, or push was performed.

## Risks

- **Native review receipt gate unverified.** This executor had no shell access to query
  `gentle-ai review status` for this change. No open native-review transaction was
  referenced by any retrieved artifact, and the launch instructions did not flag one as
  applicable, so this is recorded as an unverified residual item rather than acted on as
  a blocker.
- **Cross-backend artifact parity gap.** proposal/spec/design/tasks exist only in OpenSpec
  files, not in Engram, for this change. Not a blocker for this archive (OpenSpec is the
  authoritative source in hybrid mode and was read directly), but noted for anyone relying
  on Engram-only retrieval of this change's history.
- **Folder move outstanding.** The change folder has not yet been physically relocated to
  `openspec/changes/archive/`. Until the orchestrator performs the `git mv` above, the
  change technically still appears "active" on the filesystem even though its specs are
  now promoted and this report is written.

## Source of Truth Updated

The following specs now reflect the shipped behavior of `gadu-portable-operator`:

- `openspec/specs/gadu-canonical-source/spec.md`
- `openspec/specs/gadu-generator/spec.md`
- `openspec/specs/overlay-agent-route/spec.md` (12 of 13 requirements; R-013 excluded per
  documented historical deviation above)

## SDD Cycle Status

Planned, implemented, verified (5 passes, 0 CRITICAL findings as of pass 5), and specs
promoted. **Folder relocation to archive is the one remaining mechanical step**, explicitly
deferred to the orchestrator due to this executor's tooling limits.
