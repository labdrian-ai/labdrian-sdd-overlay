# Apply Progress: gadu-portable-operator

branch: chore/archive-stranded-changes
last_updated: 2026-08-11
unit: spec-reconciliation (post-verify), NOT original implementation. This file now
records FOUR units of work, in order: (1) the R-009 reconciliation described below,
(2) the evidence-closing unit ("Unit 2"), (3) the R-008 MANUAL-mislabel correction
("Unit 3"), and (4) the AC-5 restoration test ("Unit 4").

---

## Provenance notice

This is the **only** apply-progress record that exists for `gadu-portable-operator` in either
backend. The original implementation (PR #54, merge `fe28245`, 2026-06-27, 17/17 tasks) shipped
without one — `mem_search` for `sdd/gadu-portable-operator/apply-progress` returned zero
memories, and no `apply-progress.md` file existed in this change folder before this record.
That per-task history (RED/GREEN evidence, commit-by-commit notes) is **unrecoverable**; it is
not reconstructed here. The commit chain (`git log fe28245^1..fe28245^2`) remains the only
surviving evidence of the original apply phase and is documented in the verify report
(Engram `sdd/gadu-portable-operator/verify-report`, obs #2810).

## This unit of work

The verify phase (obs #2810) found one CRITICAL blocker: `R-009` in
`openspec/changes/gadu-portable-operator/specs/overlay-agent-route/spec.md` was contradicted by
`main` — it forbade emitting native opencode/Codex agent-definition files, but
`opencode/agents/GADU.md` is deliberately generated, committed (7663 B), and registered at
`overlay.manifest:80`. This was verified directly, not assumed:

- `engine/gadu/gadu.go:84,90-91` writes `opencode/agents/GADU.md`.
- `overlay.manifest:80` registers `opencode/agents/GADU.md custom opencode-agent`.
- `git diff --name-only fe28245^1 fe28245` contains zero `opencode/agents` paths — R-009 was
  true at merge time (2026-06-27).
- Commit `9408f02` "feat(gadu): add OpenCode agent artifact" (PR #62/#64, 2026-07-02) introduced
  the artifact five days later and narrowed the in-code known-limitations comment in
  `bin/labdrian-overlay` from "No native opencode/Codex agent-definition files" to "No native
  Codex agent-definition files are produced (R-009)" — without ever writing that narrowing back
  to the delta spec.

### Task

- [x] Reconcile `R-009` in `openspec/changes/gadu-portable-operator/specs/overlay-agent-route/spec.md`
  from change-scoped ("this change SHALL NOT ... opencode/Codex ...") to system-scoped and
  narrowed to Codex-only, matching the shipped code and the already-narrowed in-code comment.
  Added an explicit note recording that `opencode/agents/GADU.md` is emitted deliberately, added
  by PRs #62/#64 (commits `9408f02`, `fcdd84a`) after this change merged, and registered at
  `overlay.manifest:80`. Requirement ID `R-009` unchanged; no renumbering. Scenario "No GADU
  files in opencode or Codex agent directories" narrowed to "No GADU files in Codex agent
  directory" (opencode half removed, since that half is no longer forbidden); the SDD
  auto-spawn scenario is unchanged since it remains true.
  - File: `openspec/changes/gadu-portable-operator/specs/overlay-agent-route/spec.md`
  - No code changed. No test added — this is a documentation-only spec edit with no executable
    behavior change, so Strict TDD's RED-before-GREEN cycle does not apply (per explicit scope
    instruction for this unit of work).

### Not in scope for this unit (left for a future apply pass, if ever undertaken)

- WARNING-1 (R-008 `gentle-ai upgrade` half unproven) — not touched, out of instructed scope.
- WARNING-2 (R-006 route domain stale, missing `opencode-agent`) — not touched, out of
  instructed scope.
- `gadu-canonical-source` and `gadu-generator` delta specs — verify phase found both ready to
  promote verbatim; not touched here per instruction.

## TDD Cycle Evidence

Not applicable. This unit is a spec-text reconciliation with zero executable behavior change.
No RED phase exists to report; none was fabricated.

---

## Unit 2 (2026-08-11) — Evidence-closing for the `fail` (incomplete-evidence) verdict

Scope: documentation and spec annotations only. No code, no tests, no mutating commands.

The re-verify (Engram `sdd/gadu-portable-operator/verify-report`; also mirrored at
`openspec/changes/gadu-portable-operator/verify-report.md`) returned **0 CRITICAL** and
confirmed all three capabilities are promotable verbatim. The `fail` verdict was driven
solely by incomplete evidence accounting (`gentle-ai sdd-verify-validate` refuses a passing
verdict when evidence is incomplete): honest counts were 11/13 requirements and 27/31
scenarios, gated by exactly four scenarios lacking complete evidence:

1. R-005 "Skill is loadable in opencode" — inherently manual, not automatable in CI.
2. R-005 "Skill is loadable in Codex" — inherently manual, not automatable in CI.
3. R-008 "E1 confirms no clobbering" — the `gentle-ai upgrade` precondition was never probed.
4. R-008 "E1 confirms clobbering" — NOT APPLICABLE (the other E1 branch was taken), also
   requires actually running `gentle-ai upgrade` to determine which branch applies.

### Task 1 — corrected the stale E1 durability document

- [x] Corrected `docs/e1-durability-probe.md:24` (evidence table). Verified directly on
  2026-08-11 that `gentle-ai upgrade` now exists — `gentle-ai upgrade --help` prints an
  `Upgrade` help section and describes it as upgrading managed tool binaries. Installed
  version: `gentle-ai 2.3.0`.
  - The original "Not available" finding was **not deleted** — it is preserved with a
    strikethrough and a "SUPERSEDED, see 2026-08-11 correction below" label so a reader
    can see the finding changed and when.
  - A new row records the corrected 2026-08-11 availability finding.
  - A new row explicitly records that the durability probe against `upgrade` itself was
    **not executed**, deliberately: `gentle-ai upgrade` mutates installed tool binaries,
    and an unattended `gentle-ai upgrade` already broke this repository's review authority
    store earlier in this same session, costing hours of recovery. Only the `sync` half of
    the probe was executed and passed (unchanged from the original document).
  - Added a "## 2026-08-11 Correction" prose section explaining the same facts and
    reaffirming that the D4 guard-branch selection remains based on the `sync` evidence
    only, not on any observation of `gentle-ai upgrade` behavior.
  - File: `docs/e1-durability-probe.md`. No `gentle-ai upgrade` command was run — the
    safety constraint was honored throughout.

### Task 2 — marked genuinely non-automatable scenarios MANUAL

- [x] `openspec/changes/gadu-portable-operator/specs/gadu-canonical-source/spec.md` —
  R-005. Inspected the file: two runtime-load scenarios are present ("Skill is loadable
  in opencode" and "Skill is loadable in Codex"; no other runtime-load scenario exists in
  R-005). Added one MANUAL acceptance-criteria comment immediately after both scenarios,
  matching the exact formatting/indentation/voice convention already used after R-004's
  scenarios (`<!-- MANUAL acceptance criteria (not automatable in CI): ... -->`).
  Requirement ID unchanged, no renumbering, no scenario deleted or weakened.
- [x] `openspec/changes/gadu-portable-operator/specs/overlay-agent-route/spec.md` —
  R-008. Both scenarios ("E1 confirms clobbering" and "E1 confirms no clobbering") have a
  GIVEN clause that depends on actually running `gentle-ai upgrade` to determine which
  branch applies; per Task 1, that probe was deliberately not executed because it mutates
  installed tooling. Added one MANUAL acceptance-criterion comment immediately after both
  R-008 scenarios, before the R-009 heading, using the same convention. Requirement ID
  unchanged, no renumbering, no scenario deleted or weakened.

### Not in scope for this unit

- `openspec/specs/` promotion (archive phase's job, not apply's).
- `openspec/changes/archive/` (immutable, not touched).
- WARNING-2 (R-006 route domain stale, missing `opencode-agent`) — not touched, out of
  instructed scope for this unit.
- No commit, stage, or push performed.

## TDD Cycle Evidence — Unit 2

Not applicable. This unit is documentation/spec-annotation only with zero executable
behavior change (no `.go` file, test, `overlay.manifest`, `bin/labdrian-overlay`, or
anything under `engine/`, `tui/`, or `opencode/` was touched). No RED phase exists to
report; none was fabricated, per explicit scope instruction for this unit of work.

## Work Unit Evidence — Unit 2

| Evidence | Value |
|---|---|
| Focused test command and exact result | N/A — no executable behavior changed; no test command applies to a documentation/spec-comment-only edit |
| Runtime harness command/scenario and exact result | N/A — no runtime boundary exists for this unit; explicitly not exercised, and `gentle-ai upgrade` was explicitly NOT run per the safety constraint in the task instructions |
| Rollback boundary | Three files: `docs/e1-durability-probe.md`, `openspec/changes/gadu-portable-operator/specs/gadu-canonical-source/spec.md`, `openspec/changes/gadu-portable-operator/specs/overlay-agent-route/spec.md`. Each edit is additive (superseding annotations / MANUAL comments); reverting any one file fully removes that file's change without touching the other two or any unrelated content. |

### Status — Unit 2

3/3 tasks complete (E1 doc correction, R-005 MANUAL annotation, R-008 MANUAL annotation).
Ready for the next `sdd-verify` pass to re-count requirements/scenarios (expected 13/13,
31/31 if the MANUAL convention is accepted as complete evidence by the validator).

---

## Unit 3 (2026-08-11) — Corrected the R-008 MANUAL mislabel and narrowed the claim to the engineered guarantee

Scope: one requirement (`R-008`), one file. No code, no tests, no mutating commands.
`gentle-ai upgrade`, `gentle-ai sync`, and any command that mutates installed tooling or
the home directory were NOT run for this unit.

### Why this unit exists

Unit 2 (above) added a MANUAL acceptance-criterion annotation to R-008's two scenarios,
asserting they were "not automatable in CI." **That label was incorrect, and the mistake
was Unit 2's** (i.e., mine, in the previous pass): it conflated two different facts.
`gentle-ai upgrade` is a real, runnable, automatable command — it was withheld because
running it is *unsafe* (it mutates installed binaries, and an unattended run already
broke this repository's review authority store once earlier in this session). "Deliberately
not run for safety" is not "not automatable in CI"; labelling it that way would have
silently laundered an evidence gap into the canonical spec had it been promoted.

The deeper structural problem the MANUAL label papered over: R-008's two scenarios were
mutually exclusive branch selectors ("E1 confirms clobbering" vs. "E1 confirms no
clobbering"). Only one branch could ever apply, `E1` never probed `upgrade` (per
`docs/e1-durability-probe.md`), and no amount of annotation could select a branch or
close that indeterminacy.

### Task 1 — rewrote R-008's normative clause to state the guarantee actually engineered

- [x] Replaced the branch-conditional normative text ("SHALL survive a `gentle-ai sync`
  / `gentle-ai upgrade` without being clobbered ... sized after experiment E1 ... is run
  and its result is documented" — with the outcome left open pending an unrun experiment)
  with a direct statement of the guarantee the guard code actually implements: the GADU
  artifacts SHALL be registered as overlay-managed and SHALL be re-deployable by the
  overlay after any `gentle-ai sync` or `gentle-ai upgrade`, with drift detected by
  `cmd_sync_check` and restored by `cmd_apply`. This was the robust design-time answer
  already chosen (per Locked Decision 3): drift is detected and artifacts are restored
  whether or not anything ever clobbers them, so the branch question no longer needs
  resolving in the spec text.
  - The `(Resolves F1, Locked Decision 3)` parenthetical decision reference was preserved
    verbatim, matching the file's existing convention for carrying decision provenance.
  - The E1/R-013 relationship was preserved: the sizing clause ("the guard implementation
    SHALL be the minimum necessary to achieve re-deployability, sized after experiment E1
    (see R-013) is run and its result is documented") is kept, since R-013 (E1 was run and
    documented before guard code) remains fully satisfied and the surrounding text still
    depends on it for coherence.
  - Requirement ID `R-008` unchanged. No renumbering. No other requirement touched.

### Task 2 — replaced the two branch-selector scenarios with test-backed scenarios

- [x] Read `TestApply_AgentsLandInNativeAgentDirs` and `TestSyncCheck_DetectsMissingAgentFile`
  in `engine/installer/route_test.go` (lines 757-829 and 831-871 respectively) before
  writing any scenario text, to name the behaviour the tests actually assert rather than
  writing scenarios first and hoping a test would match.
  - `TestApply_AgentsLandInNativeAgentDirs` asserts: after `cmd_apply` runs,
    `agents/GADU.md` lands at `~/.claude/agents/GADU.md` (not a skills destination) and
    `skills/gadu-operator/SKILL.md`-equivalent skill rows land in all three skills
    destinations. This became the new scenario "Agent artifacts are deployed to their
    overlay-managed destinations."
  - `TestSyncCheck_DetectsMissingAgentFile` asserts: after deploy, `cmd_sync_check
    --target claude` reports `IN_SYNC` for the deployed agent file (pinning the L511 fix
    against a false `OVERLAY_NOT_DEPLOYED`); after the file is manually removed from
    `~/.claude/agents`, the same command reports `OVERLAY_NOT_DEPLOYED`. This became the
    new scenario "Sync-check detects drift in the deployed agent file."
  - Both replacement scenarios are unconditional (no branch selector, no dependency on an
    unrun `gentle-ai upgrade` probe) and are fully covered by existing, already-passing
    automated tests — no test was written or modified for this unit.
  - The two removed scenarios ("E1 confirms clobbering" / "E1 confirms no clobbering")
    are gone; they asserted outcomes conditioned on `gentle-ai upgrade` behavior that was
    never observed, and are superseded by the guarantee-based normative clause in Task 1.

### Task 3 — removed the false MANUAL annotation

- [x] Deleted the `<!-- MANUAL acceptance criterion (not automatable in CI): ... -->`
  HTML comment block that Unit 2 attached immediately after R-008's two scenarios (it
  mislabelled a deliberate safety withholding as a CI-automatability limitation; see
  "Why this unit exists" above). R-008 now carries no MANUAL annotation, matching the
  fact that both of its current scenarios ARE automatable in CI and already are, via the
  two named tests.
  - R-005's MANUAL annotation in
    `openspec/changes/gadu-portable-operator/specs/gadu-canonical-source/spec.md` was
    left untouched, exactly as instructed: the verifier examined it against the
    repository's R-004 convention and accepted it as legitimate, because its last mile
    (loading a skill file in a live third-party runtime) is genuinely not automatable in
    CI and is corroborated by passing format/placement tests. R-008's was not — R-008's
    scenarios test overlay deployment and sync-check behavior, both of which run headless
    in CI today.
  - File: `openspec/changes/gadu-portable-operator/specs/overlay-agent-route/spec.md`.
    Only R-008 was touched. R-006, R-007, R-009, R-011, R-013 in the same file were not
    modified (verified by re-reading the full file after the edit).

### Not in scope for this unit

- `openspec/specs/` promotion (archive phase's job, not apply's).
- `openspec/changes/archive/` (immutable, not touched).
- Any `.go` file, test, `bin/labdrian-overlay`, `overlay.manifest`, or anything under
  `engine/`, `tui/`, or `opencode/` — none were touched; nothing is broken in the code,
  this was purely a spec-annotation correction.
- No commit, stage, or push performed.
- `gentle-ai upgrade`, `gentle-ai sync`, or any command mutating installed tooling or the
  home directory — none were run, per the absolute constraint for this unit.

## TDD Cycle Evidence — Unit 3

Not applicable. This unit changes zero executable behavior (only requirement prose and
scenario text in one Markdown spec file). No RED phase exists to report and none was
fabricated, per explicit scope instruction for this unit of work (Strict TDD Mode is
active project-wide, but does not apply to a spec-only edit with no code path).

## Work Unit Evidence — Unit 3

| Evidence | Value |
|---|---|
| Focused test command and exact result | N/A — no executable behavior changed; the two tests referenced (`TestApply_AgentsLandInNativeAgentDirs`, `TestSyncCheck_DetectsMissingAgentFile`) were read for evidence to write accurate scenario text, not run, since no code they cover was modified |
| Runtime harness command/scenario and exact result | N/A — no runtime boundary exists for this unit; `gentle-ai upgrade` and `gentle-ai sync` were explicitly NOT run per the absolute safety constraint in the task instructions |
| Rollback boundary | One file: `openspec/changes/gadu-portable-operator/specs/overlay-agent-route/spec.md`, one requirement block (`R-008`, lines ~80-104). Reverting this edit restores Unit 2's MANUAL-annotated branch-selector text without touching any other requirement in the file or any other file. |

### Status — Unit 3

1/1 task-group complete (R-008 normative clause rewritten, scenarios replaced with
test-backed ones, false MANUAL annotation removed). Requirement ID unchanged, no
renumbering, no other requirement touched, `openspec/specs/` and
`openspec/changes/archive/` untouched, no mutating command run.

---

## Unit 4 (2026-08-11) — Wrote the missing AC-5 restoration test

Scope: one test function extended, one file, plus its documenting correction in
`docs/e1-durability-probe.md`. No spec files touched. No `gentle-ai upgrade`,
`gentle-ai sync`, or any command mutating installed tooling or the home directory
was run.

### Why this unit exists

`docs/e1-durability-probe.md` asserted: "AC-5 (simulated sync → reapply → both
artifacts present + managed) is still the regression test for the agent route,
exercised in `engine/installer/route_test.go`." That was checked directly and found
false: `rg 'AC-5|reapply|simulated sync' engine/` returned zero matches, and none of
`route_test.go`'s 19 test functions performed a re-application cycle. Units 1–3 on
this change closed evidence gaps by narrowing claims; this unit closes one by writing
the test the document had been promising, so the AC-5 claim becomes actually true.

### Task 1 — extended `TestSyncCheck_DetectsMissingAgentFile` with the restoration half

- [x] Read the existing test first (`engine/installer/route_test.go`, then at lines
  834-871) before writing anything, per instruction. Matched its existing fixture
  setup (`overlayScript`, `setupSandboxOverlay`, `runOverlay`), assertion style
  (`strings.Contains` on CLI output, `os.Stat` on deployed paths), and naming
  convention — no new idiom introduced.
  - After the existing `OVERLAY_NOT_DEPLOYED` assertion (post-removal), added: (a)
    a reapply (`overlay apply --target all`), (b) an `os.Stat` assertion that
    `~/.claude/agents/GADU.md` is present again, and (c) a `sync-check --target
    claude` assertion that the output now reports `IN_SYNC` and no longer contains
    `OVERLAY_NOT_DEPLOYED: `.
  - Updated the function's doc comment to name the AC-5 connection explicitly (the
    restoration half is the regression test `docs/e1-durability-probe.md` references)
    and to state plainly that this is a **characterization test**: the restoration
    behavior already exists in `bin/labdrian-overlay`'s `cmd_apply` deploy loop
    (unconditional `cp` whenever the destination is absent or `diff` differs from the
    source; verified directly by reading `bin/labdrian-overlay:730-736` before
    writing the test — no "already applied" ledger is consulted), so the test was
    expected to, and did, PASS on first run with no RED phase.
  - File: `engine/installer/route_test.go`.

### Task 2 — corrected the AC-5 sentence in the E1 document

- [x] `docs/e1-durability-probe.md`: struck through the original AC-5 sentence
  (preserved, not deleted, per the file's existing 2026-08-11 correction style —
  same convention as the `gentle-ai upgrade` availability correction) and added a
  new "## 2026-08-11 Correction — AC-5 regression test now exists" section naming
  `TestSyncCheck_DetectsMissingAgentFile` in `engine/installer/route_test.go` as the
  test that now exercises AC-5, with the exact command run and its result. The
  struck-through 2026-06-26 finding and the earlier `gentle-ai upgrade` correction
  section were left untouched, per instruction.
  - File: `docs/e1-durability-probe.md`.

### Strict TDD classification — characterization test, not RED→GREEN

STRICT TDD MODE IS ACTIVE for this project. This test is explicitly a
**characterization test**, not a red-green cycle: the behavior it asserts
(`cmd_apply` restores a missing/drifted agent file, `cmd_sync_check` reports
`IN_SYNC` afterward) already existed in `bin/labdrian-overlay` before this test was
written. No RED phase was fabricated — the implementation was not temporarily broken,
no deliberately-wrong assertion was written first, and no RED-then-fixed cycle is
claimed anywhere in this record. The test passed on its first and only run.

## TDD Cycle Evidence — Unit 4

| Task | RED | GREEN | REFACTOR |
|---|---|---|---|
| AC-5 restoration assertions in `TestSyncCheck_DetectsMissingAgentFile` | N/A — characterization test; behavior pre-existed, no RED phase exists or is claimed | PASS on first run (see command/output below) | None needed — assertions match the file's existing style, no cleanup required |

## Work Unit Evidence — Unit 4

| Evidence | Value |
|---|---|
| Focused test command and exact result | `cd engine && go test -count=1 -v -run TestSyncCheck_DetectsMissingAgentFile ./installer/...` → `=== RUN   TestSyncCheck_DetectsMissingAgentFile` / `--- PASS: TestSyncCheck_DetectsMissingAgentFile (0.87s)` / `PASS` / `ok  	github.com/labdrian-ai/labdrian-sdd-overlay/engine/installer	0.868s`. Confirmed PASS, not SKIP — `testing.Short()` guard present in this test but `-short` was not passed. |
| Runtime harness command/scenario and exact result | Full module suites, both green: `cd engine && go test -count=1 ./...` → all packages `ok` (installer package: `ok ... 6.108s`); `cd tui && go test -count=1 ./...` → `ok ... 0.078s`. Both run from their own module roots, never from the repository root, per instruction. |
| Rollback boundary | Two files: `engine/installer/route_test.go` (one test function extended, additive only — reverting removes only the restoration assertions and doc-comment update, leaving the original removal-detection assertions intact) and `docs/e1-durability-probe.md` (one struck-through sentence plus one new correction section, additive per the file's existing convention). No spec file, no production code (`bin/labdrian-overlay`, `engine/installer/route.go`, or any other `.go` file outside the test) was touched. |

### Status — Unit 4

1/1 task-group complete (AC-5 restoration test written and passing, `docs/e1-durability-probe.md`
corrected to name the test). `go test -count=1 ./...` green in both `engine/` and `tui/` module
roots. No spec file touched. No mutating command run.

### R-013 status — explicitly not addressed and permanently unsatisfiable

R-013 required experiment E1 to be run and documented *before* any D4 guard code was
committed. That ordering window closed in June 2026 (E1 was documented 2026-06-26; guard
code shipped in the original implementation, PR #54, merged 2026-06-27 — but the guard's
actual code-authorship-vs-documentation ordering within that window cannot be re-verified
retroactively, and even if it could, no unit of work performed after 2026-06-27 can change
when E1 was run relative to when guard code was committed). This unit adds a regression
test and a documentation correction; it does not and cannot run E1 earlier than it already
ran, and does not claim to close R-013. R-013 remains **permanently unsatisfiable** as
originally worded. This is stated explicitly here so no later reader infers otherwise from
the AC-5 test now existing — AC-5 (the regression test) and R-013 (the temporal ordering
requirement) are different claims, and only the former is closed by this unit.
