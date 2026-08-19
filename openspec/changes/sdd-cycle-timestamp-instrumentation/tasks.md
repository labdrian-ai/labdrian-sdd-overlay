# Tasks: SDD Cycle Timestamp Instrumentation

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~280-380 |
| 400-line budget risk | Medium |
| Chained PRs recommended | No |
| Suggested split | Single PR, 6 internal work units for rollback clarity |
| Delivery strategy | auto-chain |
| Chain strategy | pending (not required — estimate stays under budget) |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Test command | Harness | Rollback |
|------|------|--------------|---------|----------|
| 1 | Schema+test co-edit (behavior) | `go test -run TestActualsInstrumentationContract ./skills/...` | N/A, content-assertion test | Revert schema.json + test together |
| 2 | D2 anchor verification proof (behavior) | `go test -run TestD2AnchorIsVerifiableAndUnambiguous ./skills/...` | read-only `git show -s --format=%T`, this repo | Delete new test file |
| 3 | closure-feedback SKILL.md (behavior/doc) | contract test above | N/A, prose pinned by test | Revert SKILL.md hunk |
| 4 | sdd-time-estimation boundary (doc only) | contract test, regression check | N/A | Revert one line |
| 5 | Historical records (behavior+data) | `R014_amendedR007_fixture_pins` | Engram upsert #2096/#2789 | Fixture revert; upsert reversible via topic_key |
| 6 | Closure verification | full module suites | `engram export` + `jq` on live obs | N/A, read-only proof |

## Phase 1: Schema + contract test — one atomic unit, do not land partially

- [x] 1.1 RED `actuals_instrumentation_contract_test.go:45`: pin phrase → `"...checkpoint to merge"`; assert `checkpoint_count` desc also merge-anchored.
- [x] 1.2 RED: new subtest `R021_required_drops_wall_clock` — `"total_wall_clock_hours"` absent from `schema.Required`.
- [x] 1.3 GREEN `actuals-record.schema.json`: both descriptions → merge boundary; drop `total_wall_clock_hours` from `required` (13-property list, `additionalProperties:false` untouched). `go test -run TestActualsInstrumentationContract` → green.

## Phase 2: D2 retro-execution proof (behavior, standalone — not RED/GREEN paired)

**SUPERSEDED by Phase 7A.** The algorithm this phase proved (receipt-bound-then-path-scan,
two-source-union fallback) was removed from the design in D2 revision 3 after verify found
it unexecutable (no receipt-discovery rule) and wrong on a third real case (`79995ea`
instead of `be2c3ca`). `actuals_instrumentation_d2_retro_test.go` was deleted in 7A.1 and
replaced by `actuals_instrumentation_d2_anchor_test.go` / `TestD2AnchorIsVerifiableAndUnambiguous`.
The `[x]` marks below are left as an honest historical record of what this batch actually
did at the time — they are NOT current behavior. See Phase 7A and apply-progress.md.

- [x] 2.1 New `actuals_instrumentation_d2_retro_test.go`: `TestD2AnchorRetroExecution` — read-only `git log` (`--full-history --no-merges` union `--first-parent`) scoped per archived change, D2 skip rule applied; assert SHA prefix `0094afd` (actuals-calendar-checkpoint-instrumentation) and `fe28245` (gadu-portable-operator).
- [x] 2.2 Ran it; both matched on first correct algorithm (two-source union, latest-wins after archive-skip) — see Deviations for why a naive single first-parent scan does not suffice.

## Phase 3: closure-feedback SKILL.md (behavior, pinned-string verified)

**SUPERSEDED by Phase 7B.** The D2 prose this phase wrote (`receipt-bound`/`path-scan`,
first-parent scanning) was rewritten to design revision 3 in 7B.2 — no receipt-store
lookup, no folder scanning anywhere, anchor verified via `landing_commit` + `approved_tree`
recorded at delivery. The `[x]` marks below are left as an honest historical record of what
this batch actually did at the time — they are NOT current behavior. See Phase 7B and
apply-progress.md.

- [x] 3.1 RED: subtests pin new `inception-pipeline/SKILL.md` markers — `"through merge"`, `"merge-commit committer timestamp"`, `"receipt-bound"`, `"path-scan"`, `"Cycle Timestamps"`, append-only carve-out phrase.
- [x] 3.2 GREEN, `SKILL.md:132-157`: Plan bullet (t0 via `engram export` typed `created_at`, topic_key-absent guard); line 142 → merge + bookkeeping-exclusion sentence; new D2 command paragraph + omit/disclose rule; line 144 checkpoint boundary → merge, preserve `"one unit per distinct human round-trip reply"` / `"explicitly zero"`; Execute item 3 = archive-report append, STOP rule covers all 3 writes; Gotchas names the carve-out. Full contract test → green.

## Phase 4: sdd-time-estimation boundary (documentation-only)

- [x] 4.1 `sdd-time-estimation/SKILL.md:28` — `"...to archive"` → merge. Regression run only, R-009 pins unaffected.

## Phase 5: Historical records D7 (behavior+data)

- [x] 5.1 `testdata/corrected-actuals-sync-check-repo-behind-origin.json`: append "not re-based" sentence to `variance_vs_plan`, preserve all 6 R-014 pinned substrings. Also backported the live record's pre-existing Revision-4 `requirement_count`/`changed_lines` backfill into the fixture (see Deviations — needed for a real byte-identical match in 5.2). `R014_amendedR007_fixture_pins` stays green.
- [x] 5.2 Upserted Engram `sdd/sync-check-repo-behind-origin/actuals` (obs #2096, now revision 5) with `--topic` first-class via the `engram` CLI (MCP tool binding unavailable this session, CLI binary was). Verified: same obs id (no duplicate), embedded JSON block byte-identical to the fixture.
- [x] 5.3 Resolved `skills-validate-ondisk-gate` anchors: t0 = obs #2772 `created_at 2026-08-05 14:12:55` (confirmed live via `engram export` + `jq`, also satisfies 6.2). t1 resolved TWO ways in agreement: (a) receipt-bound — `.git/gentle-ai/review-transactions/v2/review-578b97aed1cb4065/review-receipt.json` `final_candidate_tree=b654d2d0…` equals `git show -s --format=%T 79995ea`; (b) path-scan fallback (Phase 2 algorithm) independently lands on the same commit `79995ea`, committer time `2026-08-05 17:51:17-03:00` = `2026-08-05 20:51:17` in this host's local zone (UTC). Re-based `total_wall_clock_hours`: 6.4 → 6.64 (6:38:22 = 6.6394h). Upserted obs #2789 (revision bumped) with the recomputed value and a provenance sentence naming both anchors and the `receipt-bound` resolution path.

  **CORRECTED by Phase 9 (see below).** Both resolution paths named above (`receipt-bound`, `path-scan`) are D2 revision 3's abolished mechanisms (`spec.md:38`/`:40` forbid them outright). The **numeric value (6.64h) was independently re-confirmed correct** by Phase 9 through a *third*, ratified path this entry did not use: the change's own archive-report already names the landing commit and candidate tree in versioned prose (`openspec/changes/archive/2026-08-05-skills-validate-ondisk-gate/archive-report.md`: "Merged to main: YES — commit `79995ea`" / "Candidate tree: `b654d2d0…`"), which is the exact "historical-reconstruction" case D2 revision 3 documents. Obs #2789 (revision 3) was upserted with the resolution-path sentence rewritten to name the archive-report instead of the abolished paths. This entry is left as an honest historical record of what Phase 5 actually did and disclosed at the time — see Phase 9 and apply-progress.md batch 4.

## Phase 6: Closure verification (proof, non-code)

- [x] 6.1 `engine`, `tui`, each `tools/*` module test suite — all green (`engine` incl. `installer`, `cmd`, `gadu`, `gate`, `prespec`, `propagator`, `runtime`, `settings`, `skills`; `tui`; `tools/deterministic-check-runner`, `tools/entry-contract-validator`, `tools/review-preflight`).
- [x] 6.2 `engram export` + `jq` on `sdd/skills-validate-ondisk-gate/pipeline-state` → `created_at: "2026-08-05 14:12:55"` — confirmed live, byte-for-byte match to design.md's claim. Proves D1 typed extraction live; no new cycle needed.
- [ ] 6.3 This change's own closure (n=3) exercises the path live — evidence, not a gate. Deferred to the archive/closure phase (out of apply's scope; this session's own `sdd/sdd-cycle-timestamp-instrumentation/pipeline-state` existence is uncertain per design's Open Questions).

## Phase 7: Remediation — the three blockers verify raised (design revision 3)

Verify returned **fail**: 3 blockers / 3 CRITICAL. Phases 1, 4, 5 and 6.1–6.2 survive unchanged and verified. Phases 2 and 3 are superseded because the algorithm they encoded is removed from the design.

### 7A — Replace the fitted path-scan proof (was C2)
The original Phase 2 test was pinned to two SHAs the algorithm happened to fit. Swept over all 19 archived changes it selects the merge of an unrelated PR for `deterministic-verification-evidence` (`79995ea`, which landed a *different* change, instead of `be2c3ca`) and returns nil for 9 of 19. Path scanning is removed from the design, so its proof goes with it.

- [x] 7A.1 DELETE `engine/skills/actuals_instrumentation_d2_retro_test.go` — it asserts coverage of an algorithm that no longer ships, which is worse than no test.
- [x] 7A.2 RED, replacing it: `TestD2AnchorIsVerifiableAndUnambiguous` in `engine/skills/`, asserting (a) an anchor whose `landing_commit` tree equals its recorded `approved_tree` resolves; (b) one whose tree does NOT match is **rejected** and yields no t1; (c) a tree carried by more than one commit is **ambiguous** — t1 is omitted and the ambiguity disclosed, never resolved by position — pin this against a tree that genuinely repeats on `main` (`main` has 434 commits sharing only 84 distinct duplicated tree values, so 205 of 434 commits sit in a colliding tree — a real duplicate exists to use as a fixture); (d) a change with no recorded anchor yields no t1 and attempts no folder scan.
- [x] 7A.3 GREEN. Every assertion must read real bytes or execute real git — no string-only pins. All git execution stays inside the `_test.go` file (ADR-15 zero-fetch import allowlist).

### 7B — Make the shipped prose the algorithm (was C1)
closure-feedback is agent-executed prose, so `skills/inception-pipeline/SKILL.md` **is** the implementation. It currently still describes the removed first-parent path scan, which yields zero candidates for the very case that motivated the change — a green test guaranteeing an algorithm nobody ships.

- [x] 7B.1 RED: pin the revision-3 markers and FORBID the removed ones. Required present: `approved_tree`, `landing_commit`, the verify-never-discover rule, the ambiguity-disclosure rule, the rejection rule. Required absent: `path-scan`, `first-parent`, `receipt-bound`, the positional/earliest-carrier resolution phrasing ("carrying that tree MUST be chosen").
- [x] 7B.2 GREEN: rewrite the SKILL.md D2 paragraph to design revision 3 — anchor recorded in the versioned archive-report at delivery (`landing_commit` + `approved_tree`), verify the commit's tree equals the recorded tree or reject, earliest-carrier disambiguation, omit-and-disclose when no anchor is recorded, and no folder scanning anywhere.
- [x] 7B.3 Execute the shipped prose verbatim against `deterministic-verification-evidence` and confirm it now yields `be2c3ca` (its archive-report records "merged as `be2c3ca`") or omits — never `79995ea`.

### 7C — Implement R-019, which was specified but never tasked (was C3)
Its three required rationale strings exist only in this change's own proposal and spec — they appear in no shipped artifact, so the positive clause has no implementation.

- [x] 7C.1 RED: assert `skills/inception-pipeline/SKILL.md` states the deferral reason for the three compute-time fields, naming session-transient telemetry, the `sdd-attempt` ledger's lack of timestamp fields, and the absence of structured subagent durations in transcripts.
- [x] 7C.2 GREEN: add that rationale to closure-feedback, adjacent to where the three fields are written.

### 7D — Re-verify
- [x] 7D.1 Full suites green (confirmed by apply — see apply-progress). `sdd-verify` was re-run and returned `fail / 2 blockers` (C1 round 2, C2) — see Phase 8.

## Phase 8: Remediation — propagate the round-2 inversion, repair the self-contradicting scenario, close three non-covering pins (design revision 3, round 2)

Verify's re-run (evidence `sha256:0aa54d5f…`) found the ratified "verify, never discover" tree-resolution rule applied to exactly one of five artifacts (`skills/inception-pipeline/SKILL.md`, already correct); `spec.md`, `design.md`, `tasks.md`, and the D2 behavioural test still mandated the abolished earliest-carrier rule (C1, round 2). `spec.md`'s first R-016/17/18 scenario still described the superseded receipt-sourced tree discovery, contradicting its own requirement body twelve lines earlier (C2). Three prose pins were also proven non-covering by mutation.

### 8A — Propagate the inversion (C1, round 2)
- [x] 8A.1 `spec.md:42` — normative MUST rewritten from "the **earliest** commit … MUST be chosen" to the ratified ambiguous-then-omit rule, matching SKILL.md's wording.
- [x] 8A.2 `design.md:57` (D2 revision 3, "Disambiguation rule") — rewritten to the same rule, citing the real three-way collision (`be2c3ca`/`aa81361`/`459b48d` sharing tree `2ad8e42e…`) as the proof earliest-by-time is wrong.
- [x] 8A.3 `tasks.md:85` (7A.2) and `tasks.md:91` (7B.1) — reworded from "earliest carrier" language to the ambiguity rule.
- [x] 8A.4 `engine/skills/actuals_instrumentation_d2_anchor_test.go` — deleted `tree_resolution_selects_the_earliest_carrier` and `resolveEarliestCommitForTree` (both encoded the abolished rule); replaced with `tree_carried_by_more_than_one_commit_is_ambiguous_never_resolved_by_position` and `resolveTreeToCommitOrAmbiguous`, proven against the real 3-way collision. Mutation-proven RED on reintroducing positional selection.

### 8B — Repair the self-contradicting scenario (C2)
- [x] 8B.1 `spec.md:46-50` ("Anchors resolve and are legible in both stores") rewritten from receipt-sourced tree discovery to the `landing_commit`/`approved_tree` verified-anchor mechanism, matching the requirement body it belongs to.

### 8C — Close the three non-covering pins
- [x] 8C.1 `"bookkeeping"` (occurred at 2 sites) → repinned to `"excluded from this count as post-boundary bookkeeping"`, the operative checkpoint_count clause the R-003/4/5 scenario actually covers. Mutation-proven: deleting the clause → RED; restored → GREEN.
- [x] 8C.2 `"Cycle Timestamps"` (occurred at 2 sites) → repinned to `"Append a `## Cycle Timestamps` section to the archived"`, the operative Execute item 3 clause. Mutation-proven: deleting Execute item 3 → RED; restored → GREEN.
- [x] 8C.3 D1b t0 fallback (previously unpinned) → new pin `"t0 falls back to the earliest `created_at` among the change"`. Mutation-proven: deleting the fallback clause → RED; restored → GREEN.

### 8D — Structural guard against silent re-drift
- [x] 8D.1 Added FORBID marker `"carrying that tree MUST be chosen"` to the R016/17/18 FORBID list — the abolished positional-resolution phrase, previously unguarded. Mutation-proven: reintroducing the phrase → RED.
- [x] 8D.2 `actuals_instrumentation_d2_anchor_test.go` now reads `skills/inception-pipeline/SKILL.md` directly (`shipped_skill_prose_matches_the_proven_ambiguity_rule`), binding the real-git-derived ambiguity proof and the shipped prose in one test file so they cannot silently drift apart again.

### 8E — Re-verify
- [ ] 8E.1 Full suites green (confirmed by apply — see apply-progress); re-running `sdd-verify` itself is a separate phase, out of apply's scope, and is the recommended next step.

## Phase 9: Remediation — sweep, not chase citations (round 3 blockers, design revision 3)

Verify's round-3 re-run (evidence `sha256:8493b257…`) found the ratified rule fixed at exactly the
line each prior report cited — `design.md:57` — and never swept from the rest of the file (C1:
lines 110, 118, 137, 144 still mandated or described the abolished path-scan), and found that
residue had produced a real spec violation in shipped data (C2: live obs #2789 disclosed the
forbidden `receipt-bound`/`path-scan`/`first-parent` resolution paths). Engram obs #2888
("Citation-chasing remediation reproduces the same defect one scope-level down") records the
three-round pattern this phase exists to break: fix the sample, not the extent.

### 9A — Sweep `design.md` completely (C1)
- [x] 9A.1 Swept every line of `design.md` for `path-scan`, `first-parent`, `receipt-bound`,
  positional/earliest-carrier phrasing, and the pre-D1 footer-parsing description, not only the
  four lines the report cited. Fixed: line 7 (Gating Question, clarified as evidence-only, not
  the shipped mechanism), line 11 (Technical Approach — now names the typed field and the
  versioned anchor, matching D1/D2 revision 3), line 110 (D7 #2789 entry — see 9C), line 118
  (Data Flow diagram — now shows the typed field and the versioned anchor, not a first-parent
  scan), line 133 (File Changes — the closure-feedback edit list literally said "harvest t0 via
  the footer", contradicting D1; fixed to name the typed field), line 137 (Interfaces/Contracts —
  the anchor sentence mandated naming a `first-parent scan` resolution path, a FORBIDDEN string
  in the shipped contract test; rewritten to the verified-anchor shape), line 144 (Testing
  Strategy — described `TestD2AnchorRetroExecution`, deleted in 7A.1; rewritten to describe the
  test that actually ships), line 153 (Threat Matrix — claimed a `git -C <project-root>` pin and
  RED coverage that do not exist; corrected to the real command and the real covering tests).
  Lines 40, 74, 83, 86 and the `### D2 (revision 2 — SUPERSEDED...)` block (65–88) were left
  unchanged: they are explanatory prose describing why the abolished mechanism was rejected, not
  the shipped rule, and 65–88 is already explicitly marked SUPERSEDED.
- [x] 9A.2 Swept `tasks.md`, `apply-progress.md`, the schema, both test files,
  `skills/inception-pipeline/SKILL.md`, `skills/sdd-time-estimation/SKILL.md`, and
  `engine/skills/testdata/` for the same vocabulary. Found and fixed: `tasks.md:85` (tree-collision
  figure, see 9E) and the doc comment in
  `actuals_instrumentation_d2_anchor_test.go:18-19` (same figure). `skills/inception-pipeline/SKILL.md`,
  `skills/sdd-time-estimation/SKILL.md`, the schema, and the testdata fixture were already clean —
  confirmed by direct `rg` sweep, not assumed from a prior report. `apply-progress.md`'s
  batch-1–3 BEFORE/AFTER quotes and `verify-report.md` are historical records of exact prior
  diffs/verdicts and are intentionally left byte-unchanged; new findings are appended as new
  entries instead (see apply-progress.md batch 4).

### 9B — Make the sweep enforceable (structural guard, not a one-time cleanup)
- [x] 9B.1 RED: new test `TestAbolishedVocabularySweptFromCurrentSections` in
  `engine/skills/actuals_instrumentation_contract_test.go` — greps every shipped artifact this
  change touches for `path-scan`, `first-parent`, `receipt-bound`, and the positional
  earliest-carrier phrase, skipping only text inside an explicitly-marked `SUPERSEDED` block
  (`design.md`'s revision-2 section, delimited by its own heading and the next `---`/`##`
  boundary). Failed before this batch's `design.md` edits landed (found the four residues at
  9A.1's lines).
- [x] 9B.2 GREEN after 9A's edits. Mutation-proven: reintroducing `"first-parent"` into a current
  (non-SUPERSEDED) section of `design.md` → RED; reintroducing the same string inside the
  SUPERSEDED block → stays GREEN (the carve-out itself is exercised, not merely assumed).

### 9C — Correct obs #2789's provenance (C2)
- [x] 9C.1 Verified independently, against the real repository, whether the verifier's own
  claim ("archive-report.md names no merge SHA") was accurate: it was **not** —
  `openspec/changes/archive/2026-08-05-skills-validate-ondisk-gate/archive-report.md` records
  both "Merged to main: YES — commit `79995ea`" and "Candidate tree: `b654d2d0…`" in versioned
  prose, and `git show -s --format=%T 79995ea` = `b654d2d0…` (match). This is exactly the
  "historical-reconstruction" case `skills/inception-pipeline/SKILL.md`'s D2 paragraph already
  documents ("WHEN reconstructing a historical record whose archive-report names a landing SHA
  in versioned prose... use that SHA directly"). t1 legitimately resolves; `total_wall_clock_hours
  = 6.64` was already correct.
- [x] 9C.2 Upserted obs #2789 (revision 3): value unchanged (6.64h), resolution-path sentence
  rewritten from `receipt-bound`/`path-scan` to name the archive-report as the resolution source.
  `topic_key` passed as a top-level `mem_save` argument; confirmed `revision_count` incremented
  (not a duplicate). Checked obs #2096 and `engine/skills/testdata/` for the same class of
  violation — both already clean (no forbidden vocabulary in either).

### 9D — Repair the five vacuous pins the round-3 report found, and probe the full pin set
- [x] 9D.1 W1 (`REJECTED`, 2 sites) → repinned to the operative rejection clause. W2
  (`append-only carve-out`, 2 sites) → repinned to the operative Execute-item-3 clause. W3
  (`git show -s --format=%T` command, unpinned) → new pin added. W4 (task 4.1's boundary edit,
  unpinned) → new pin `"tiering-go-ahead to merge, including interruption gaps"` +
  FORBID `"tiering-go-ahead to archive"`. W5 (R-020 "not re-based" annotation, unpinned) → new
  fixture pin `"was NOT re-based to the merge-anchored boundary"`. All five mutation-proven
  RED/GREEN — see apply-progress.md batch 4 for the full table.
- [x] 9D.2 Occurrence-count swept every remaining string pin in both test files (the rule from
  Engram obs #2884: a pin is non-covering whenever its exact text occurs at more than one site
  in its target file) and mutation-probed every multi-occurrence pin directly. Full inventory,
  probe count, and results are in apply-progress.md batch 4 — not a subset.

### 9E — Correct the tree-collision figure (W7)
- [x] 9E.1 `design.md:57`, `tasks.md:85`, and `actuals_instrumentation_d2_anchor_test.go:18-19`
  all corrected from "84 of 434 commits… (~19%)" to "205 of 434 commits (~47%) sit in a colliding
  tree; 84 is the number of duplicated tree values, not commits" — verified by hand with
  `git rev-list`, reproduced in apply-progress.md batch 4.

## Phase 10: Remediation — close the three absent-coverage gaps (round 4 warnings, no blockers)

Verify's round-4 re-run (evidence `sha256:42c2a3ce…`) found **0 blockers, 0 CRITICAL** — the first
clean round — but scored scenarios 12/14: an *absent* coverage class occurrence-counting cannot
see by construction (Engram obs #2890). Three gaps: W1 (R-021's shipped write-anyway clause had
no assertion at all), W2 (the sweep guard's SUPERSEDED-carve-out terminator was itself unpinned,
so demoting it silently widened the carve-out — V5 in the round-4 report), W3 (`tasks.md:24`
published a gate command naming a test deleted in 7A.1, vacuously green). All three closed by
adding coverage, not by re-litigating any settled finding.

### 10A — Pin R-021's shipped write-anyway clause (W1)
- [x] 10A.1 `R021_required_drops_wall_clock` now also reads `skills/inception-pipeline/SKILL.md`
  and pins the operative Validate-step clause at its unique site: "`total_wall_clock_hours` is
  the one field allowed to be validly absent (R-021)" and "every other resolved field is still
  required" (both occur exactly once in the file). Closes spec.md scenarios "A cycle missing t0
  still produces a usable record" and "Neither anchor resolves" (R-016/17/18 + R-021), previously
  PARTIAL.
- [x] 10A.2 Mutation-proven directly against the real file: deleting the clause → RED; inverting
  it to "Every field is required; if any field cannot be resolved, discard the whole record" (the
  exact all-or-nothing rule R-021 exists to abolish) → RED; restored byte-identical → GREEN. See
  apply-progress.md Phase 10 for the transcript.

### 10B — Pin the sweep guard's carve-out terminator (W2)
- [x] 10B.1 `stripMarkdownSection`'s signature changed from level-based termination
  (`stripMarkdownSection(doc, heading)`, which stops at "the next heading of level ≤ heading's
  own level") to an explicit, exact-text terminator
  (`stripMarkdownSection(doc, heading, terminatorHeading)`). A level-based terminator cannot tell
  a demoted heading apart from a genuine nested subsection of the carve-out; requiring the
  terminator's exact text (including its "#" count) pins its level, so a demotion is no longer
  found and the strip errors instead of silently widening scope.
- [x] 10B.2 `design_md_current_sections` now calls `stripMarkdownSection` with the exact
  terminator `"### D3 — Where anchors are written"`. New table-driven coverage
  (`TestActualsInstrumentationGateHelpers/section_stripping`, 7 cases) exercises the terminator
  pin directly, including the specific demotion shape (`### D3` → `#### D3`).
- [x] 10B.3 Mutation-proven against the real `design.md`, reproducing round 4's V5 exactly:
  demoted `### D3 — Where anchors are written` to `#### D3 — Where anchors are written` **and**
  injected `path-scan` into D3's newly-swallowed body → RED (`design_md_current_sections`:
  "terminator heading may have been renamed, deleted, or demoted"). Restored byte-identical →
  GREEN. See apply-progress.md Phase 10 for the transcript.

### 10C — Fix the stale published gate and sweep for others (W3)
- [x] 10C.1 `tasks.md`'s Suggested Work Units table, row 2: `go test -run TestD2AnchorRetroExecution ./skills/...`
  (deleted in 7A.1, 0 hits in any `.go` file, confirmed vacuously green: `ok … [no tests to run]`,
  exit 0) → `go test -run TestD2AnchorIsVerifiableAndUnambiguous ./skills/...` (confirmed: 5/5
  subtests PASS). Harness column corrected from `git log --first-parent` (abolished mechanism) to
  `git show -s --format=%T` (the actual shipped command).
- [x] 10C.2 Swept every other artifact this change ships for the same defect class (a published
  gate naming a test/symbol that no longer exists): `apply-progress.md`'s current (non-historical)
  references, `design.md`'s Testing Strategy table (`TestD2AnchorIsVerifiableAndUnambiguous`,
  already correct — fixed in 9A.1), `spec.md`, `proposal.md` (no gate commands present). Found
  exactly one live stale gate (10C.1); every other `-run Test...` citation in this change's own
  artifacts either already names a current symbol or is explicitly framed as a historical record
  of something already deleted (e.g. `verify-report.md`'s own citation of the defect it found,
  and `tasks.md`'s Phase 2/Phase 9 sections, both already marked as honest historical record, not
  current behavior). Gate commands in OTHER, unrelated changes'
  (`skill-lifecycle`, `skill-manifest-gen`, archived changes) task files were found but are out of
  this change's scope — a citation is a sample, not the extent, but the extent is bounded by what
  this change ships (Engram obs #2888).

### 10D — Re-verify
- [x] 10D.1 Full suites green (confirmed by apply — see apply-progress.md Phase 10); re-running
  `sdd-verify` was the recommended next step and has since run as round 5 (evidence
  `sha256:242912ab…`, `verify-report.md`) — 0 blockers, 0 CRITICAL, scenarios 13/14. See Phase 11
  below for the remediation this apply session performs against that round's two findings.

## Phase 11: Remediation — close the two bounded gaps round 5 found (0 blockers, 0 CRITICAL)

Verify's round-5 re-run (evidence `sha256:242912ab…`) found **0 blockers, 0 CRITICAL** — a second
consecutive clean round — but scored scenarios 13/14: scenario 14 (R-020, "Measured record
re-based when both anchors resolve") had no committed guard, and its stated exemption reason
("live Engram data, not shipped repository content") is falsified by the change's own sibling
scenario 13, which is ALSO live Engram data (obs #2096) and IS guarded through a committed
fixture mirror. A second, independent finding (W-A) showed the sweep guard's carve-out span was
still bounded on only one end: an exact-text terminator defeats heading demotion (round 4's V5),
but a brand-new CURRENT section inserted *before* the terminator was still silently swallowed
into the removed span (probe P7, with its after-terminator control P8 isolating position as the
cause). Both gaps closed by adding coverage; no prior finding re-litigated.

### 11A — Close scenario 14 with a committed fixture mirror (W-B)
- [x] 11A.1 Read Engram obs #2789 (`sdd/skills-validate-ondisk-gate/actuals`, revision 3) via
  BOTH `mem_get_observation` (MCP) and `engram export` + `jq` (CLI), extracted the embedded JSON
  block via the typed export path, and wrote it verbatim to
  `engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json` — the same
  committed-mirror mechanism `corrected-actuals-sync-check-repo-behind-origin.json` already
  applies to obs #2096.
- [x] 11A.2 Proved fidelity: `cmp` against the `jq`-extracted live JSON block reports
  BYTE-IDENTICAL (see apply-progress.md for the transcript).
- [x] 11A.3 New pin `R020_skillsValidateOndiskGate_fixture_mirrors_measured_rebased_record` in
  `engine/skills/actuals_instrumentation_contract_test.go`: asserts `total_wall_clock_hours ==
  6.64` (the re-based value, not the retired 6.4) and three provenance substrings — the
  re-basing sentence naming both the retired and re-based values through the merge boundary, t0's
  anchor (obs #2772, typed `created_at`, primary source), and t1's anchor (commit `79995ea`,
  resolution path: the archive-report). RED confirmed by temporarily removing the fixture file
  (genuine RED-before-GREEN, not a mutation probe against already-shipped content); GREEN after
  restoring. Mutation-proven in both directions on the two most semantically load-bearing
  assertions: numeric value inverted to the retired `6.4` → RED; the re-basing sentence inverted
  to the R-020 else-branch ("was NOT re-based… anchors did not both resolve") → RED; plus
  deletion probes on the numeric field and both anchor-naming sentences, all → RED. All six
  probes restored byte-identical and re-confirmed GREEN.
- [x] 11A.4 Added the new fixture path to `TestAbolishedVocabularySweptFromCurrentSections`'s
  `other_shipped_artifacts_have_no_carve_out` sweep list, consistent with the sibling fixture.

### 11B — Bound the sweep guard's carve-out span on both ends (W-A)
- [x] 11B.1 `stripMarkdownSection` now rejects any heading at or above the carve-out's own level
  encountered before the exact-text terminator, other than the terminator itself — reported as
  `errMarkdownSectionNotFound` (from the caller's perspective, a widened span and a
  renamed/demoted terminator mean the same thing: this document no longer has the single
  contiguous carve-out the function assumes). A heading strictly deeper than the carve-out's own
  level (a genuine nested subsection) does not trip the guard.
- [x] 11B.2 New table cases in `TestActualsInstrumentationGateHelpers/section_stripping`: (a) an
  inserted same-level section before the terminator now errors instead of being swallowed —
  confirmed RED against the OLD (pre-11B.1) implementation before the fix landed, GREEN after;
  (b) the complement control — a genuinely nested, deeper-level subsection does not trip the
  guard, unaffected by the fix.
- [x] 11B.3 Mutation-proven against the REAL `design.md`, reproducing round 5's P7 exactly:
  inserted a new `### D2c` current section with injected `path-scan` vocabulary immediately
  before the real `### D3` terminator → RED (`design_md_current_sections`, "a section may have
  been inserted, widening the removed span"). Restored byte-identical (`cmp` confirms) → GREEN.
  The real, un-mutated `design.md` was independently confirmed to have no nested heading at or
  above the carve-out's own level inside its SUPERSEDED block, so the fix introduces no false
  positive against shipped content.

### 11C — Re-verify
- [x] 11C.1 Full suites green (confirmed by apply — see apply-progress.md Phase 11); re-running
  `sdd-verify` itself is a separate phase, out of apply's scope, and is the recommended next
  step.

## Phase 12: Remediation — reconcile R-019 with the schema `required` list (orchestrator finding, post-round-6)

Verify round 6 passed the change as archive-ready (`pass_with_warnings`, 0 blockers, 0 CRITICAL,
14/14 scenarios), but a human diff review the orchestrator performed AFTER that round found a
defect no round had caught: R-019 (`spec.md:100`) says `implementation_hours`, `review_gate_hours`,
and `post_review_fix_hours` "MUST stay unpopulated until a durable source exists", and its own
scenario opens "GIVEN a closed record with the three compute-time fields **empty**" — but the
shipped schema (`skills/_shared/actuals-record.schema.json`) still listed all three in `required`.
Only `total_wall_clock_hours` had been dropped, under R-021. A record that OBEYS R-019 was
therefore REJECTED by the schema this change ships, and the Validate rule's own "report and STOP"
clause fired on the one thing that exercises the instrument end to end (task 6.3). Empirically
confirmed before any fix: constructing the exact required-fields-minus-the-three record and
validating it against the shipped schema failed. R-019 was independently verified as the correct
side (`gentle-ai sdd-attempt status` has no timestamp fields; orchestrator telemetry is
session-transient; transcripts carry no structured subagent durations — there is genuinely no
durable source yet), so the schema was reconciled to it, the same non-destructive treatment
`total_wall_clock_hours` already received under R-021 (`required` is a floor, not a prohibition;
`properties` and `additionalProperties: false` are untouched).

### 12A — Reconcile the schema with R-019 (work unit 1)
- [x] 12A.1 Removed `implementation_hours`, `review_gate_hours`, and `post_review_fix_hours` from
  `skills/_shared/actuals-record.schema.json`'s `required` list — 3 lines deleted, 0 added. The
  `properties` block, property description text, and `additionalProperties: false` are untouched.
  `D2_D5_closed_schema_invariant` (`engine/skills/actuals_instrumentation_contract_test.go`)
  re-checked: it iterates `schema.Required` and asserts each name is present in the committed
  fixture, so shrinking `required` only narrows which names that loop checks — it does not go
  vacuous (the fixture already carries all three fields with narrative values) and does not need
  editing to stay coherent.
- [x] 12A.2 Updated `skills/inception-pipeline/SKILL.md`'s Validate sentence: it named
  `total_wall_clock_hours` as "the one field allowed to be validly absent (R-021)" — no longer
  true once three more fields joined it. Rewritten to name all four fields and their two distinct
  reasons (R-021: anchors did not resolve; R-019: no durable source exists yet) without collapsing
  either into the other.

### 12B — Bind the rule to the artifact by test (work unit 2 — the real fix)
- [x] 12B.1 New subtest `R019_required_drops_compute_time_fields` in
  `TestActualsInstrumentationContract`: negative pins that the three field names are absent from
  `schema.Required` (mirroring `R021_required_drops_wall_clock`'s own pattern) plus positive pins
  that the three are still declared in `schema.Properties`.
- [x] 12B.2 New pure helper `validateAgainstActualsSchema(schema, record)` — a minimal
  closed-schema validator (required-presence + additionalProperties:false closure) — and
  `r019ConformingRecordFixture()`, which builds the exact record R-019's own scenario describes
  (every other required field present, the three compute-time fields absent). The new subtest
  calls `validateAgainstActualsSchema` against the REAL shipped schema with that fixture and
  requires a nil error, so a future re-add of any of the three names to `required` fails this
  assertion, not only the negative pins in 12B.1.
- [x] 12B.3 Mutation-proven against the real `skills/_shared/actuals-record.schema.json`, one
  field at a time: re-inserted `implementation_hours` into `required` → RED; restored,
  byte-identical (`diff` empty) → GREEN; repeated for `review_gate_hours` → RED → GREEN; repeated
  for `post_review_fix_hours` → RED → GREEN. `validateAgainstActualsSchema` itself also gained
  direct table-driven coverage (`TestActualsInstrumentationGateHelpers/validate_against_actuals_schema`,
  4 cases: valid record, valid record with an optional field present, missing required field
  rejected, undeclared key rejected under `additionalProperties: false`).
- [x] 12B.4 Updated `R021_required_drops_wall_clock`'s own pin to match the reconciled Validate
  sentence (`is the one field` → `is validly absent when its anchors do not resolve`); confirmed
  genuinely RED first (the production edit in 12A.2 landed before this pin update, so the OLD pin
  text was briefly absent from the file and the suite failed for real), then GREEN after the pin
  was corrected.

### 12C — Sweep the class (work unit 3)
- [x] 12C.1 Cross-checked every rule stating a field MAY be absent/omitted/optional/deferred
  against the schema's `required` list, and the converse (every required field, and whether any
  shipped rule allows it to be missing) — full table in apply-progress.md. Finding: R-019 was the
  only disagreement; every other field's rule and schema state already agreed (R-021/
  `total_wall_clock_hours` was already correctly excluded; `requirement_count`, `changed_lines`,
  `review_lens_count`, `checkpoint_count` were already correctly excluded and their descriptions
  already say "Omit if not available"/"Omit if the spec could not be read"; `change_name`,
  `project`, `approval_decision`, `scope_drift_notes`, `variance_vs_plan` remain required with no
  rule claiming otherwise).
- [x] 12C.2 Swept `design.md`, `proposal.md`, `tasks.md` for stale claims about the required list
  content: no artifact claims `implementation_hours`/`review_gate_hours`/`post_review_fix_hours`
  are or should be required or optional — the only pre-existing `required`-list discussion in
  those files concerns `total_wall_clock_hours`/R-021 (already correctly resolved in Phase 1), so
  no further edit was needed there.

### 12D — Explicitly out of scope this batch
- Did NOT withdraw or rewrite the compute-field values in live Engram records #2789/#2096 or their
  committed fixture mirrors — R-019 is forward-looking, and R-020 already establishes
  annotate-never-rebase for historical records. No annotation was judged warranted beyond what
  R-020 already requires (see apply-progress.md).
- Did NOT touch the `approved_tree` tautology (`skills/inception-pipeline/SKILL.md:147`) — awaiting
  a separate scoped human decision, unchanged since Phase 7.
- Did NOT touch cumulative size / chained PRs (W3) or task 6.3 (structurally deferred to archive).

### 12E — Re-verify
- [x] 12E.1 Full suites green (confirmed by apply — see apply-progress.md Phase 12); re-running
  `sdd-verify` is a separate phase this agent does not invoke — verify round 6 has since run
  (`pass_with_warnings`, 0 blockers, 0 CRITICAL, 5/5 requirements, 14/14 scenarios) and its own
  warning W2 is the defect Phase 13 below fixes.

## Phase 13: Remediation — remove the tautological `approved_tree` check; three-state resolution outcome (verify round 6 W2, scoped human decision)

Verify round 6 passed the change archive-ready but carried one WARNING it deliberately declined to
fix itself: `skills/inception-pipeline/SKILL.md:147` defined `approved_tree` as the review
receipt's `final_candidate_tree` when review ran, **otherwise `landing_commit`'s own tree**. In
that second branch the mandated check `git show -s --format=%T <landing_commit>` MUST equal the
recorded `approved_tree` is `X == X` by construction — it can never fail, precisely where there is
no independent authority to back it. Round 6 rated this WARNING (not blocking) and required a
scoped human decision rather than a silent fix, because closing it changes what "verified" means
in a live archive-report convention. **The decision, made by the human**: remove the false check
rather than invent a new one — a tautological check reported as "verified" is a fabricated
assurance, the same defect class this entire change exists to eliminate from the numbers it
measures.

### 13A — Three-state rule: `approved_tree` recorded only when a receipt exists (work unit 1)
- [x] 13A.1 `skills/inception-pipeline/SKILL.md`: `approved_tree` is now recorded ONLY when a
  native review receipt exists for the candidate — the "otherwise `landing_commit`'s own tree"
  synthesis branch is removed outright, not demoted. The resolution-outcome vocabulary grows from
  two states (verified-or-REJECTED) to four: **verified** (an independent receipt tree matched),
  **self-asserted** (no review ran, so no independent authority exists — the anchor is recorded
  and t1 STILL resolves, but is explicitly NOT verified), **rejected** (a recorded `approved_tree`
  did not match — t1 omitted, mismatch disclosed), and **absent** (no `landing_commit` was ever
  recorded — a change predating the convention, unchanged from before). `variance_vs_plan` MUST
  name which outcome applies so no reader mistakes a self-asserted anchor for a verified one.
  Also updated: the review-receipt bullet (line 138, "`approved_tree` is never recorded when no
  review ran"), and the archive-report Execute-item-3 table description (line 160).
- [x] 13A.2 Propagated to `specs/actuals-instrumentation/spec.md` (R-016/17/18 body rewritten;
  new scenario "An anchor with no review receipt is used but recorded as self-asserted, never
  verified") and `design.md` (D2 revision 3's field table, a new "Correction (Phase 13)" paragraph
  documenting the fix without reproducing the abolished phrase verbatim, the verification
  paragraph, the Data Flow diagram, and the Interfaces/Contracts anchor sentence template). Swept
  every artifact this change touches (`rg` for `approved_tree|REJECTED|verifiable rather than
  merely asserted|landing_commit`) — full before/after in apply-progress.md. **t1 still resolves
  in the self-asserted case** — this is a labelling fix, not a new omit path; the measurement is
  never withheld for a no-review anchor.

### 13B — Bind by test (work unit 2)
- [x] 13B.1 New pins in `R016_R017_R018_closure_feedback_anchor_prose`
  (`engine/skills/actuals_instrumentation_contract_test.go`): the operative "`approved_tree` MUST
  NOT be recorded at all" clause, the self-asserted outcome clause, and the four-way outcome
  vocabulary (`"verified, self-asserted, rejected, or absent"`). New FORBID entry for the exact
  abolished phrase, `"otherwise \`landing_commit\`'s own tree"`. Mutation-probed: reintroduced the
  phrase into `SKILL.md` → RED; restored, byte-identical → GREEN.
- [x] 13B.2 Added the same phrase to `TestAbolishedVocabularySweptFromCurrentSections`'s
  `abolished` list, so the sweep now covers this defect class across every shipped artifact, not
  only `SKILL.md` (Engram obs #2888: "a citation is a sample, never the extent"). Required
  rewording `design.md`'s own "Correction (Phase 13)" paragraph to describe the abolished rule
  without quoting it verbatim, since a literal citation in a CURRENT (non-SUPERSEDED) section
  would itself trip this sweep. Mutation-probed directly against `-count=1` (a stale `go test`
  cache initially masked a false GREEN on the first attempt — re-ran with `-count=1` and confirmed
  genuine RED, then restored byte-identical and confirmed GREEN, `-count=1` again).
- [x] 13B.3 `resolveD2T1` in `actuals_instrumentation_d2_anchor_test.go` gained a fourth outcome:
  the binary `rejected bool` return became a `d2AnchorOutcome` enum (`absent` / `self-asserted` /
  `verified` / `rejected`). New subtest
  `recorded_anchor_with_no_review_receipt_is_self_asserted_never_verified` constructs an anchor
  with a real `landingCommit` (`be2c3ca`) and `approvedTree: ""` (the real shape an absent-receipt
  archive-report entry takes), and asserts t1 STILL resolves while `outcome != d2AnchorVerified`.
  **Mutation-proven**: temporarily replaced the self-asserted branch with the exact abolished
  behavior (`anchor.approvedTree = actualTree` before the existing comparison, i.e. synthesizing
  the tree from itself) → RED (`outcome = "verified", want "self-asserted"`); restored,
  byte-identical (`diff` empty) → GREEN. The three pre-existing subtests (verified / rejected /
  absent) were updated to the new enum and re-confirmed GREEN; all 6 subtests pass.

### 13C — Sweep the class: other structurally-unfailable checks (work unit 3)
- [x] 13C.1 Swept this change's shipped artifacts and test suite for other checks whose success
  condition is definitionally true. Full table in apply-progress.md; summary: one genuine instance
  found and fixed (13A/13B above); every other candidate inspected — `D2_D5_closed_schema_invariant`,
  the R-019 binding test, the `commitsCarryingTree`/`resolveTreeToCommitOrAmbiguous` cross-check,
  `sliceMarkdownSection`'s not-found-vs-empty-string distinction, and the D6 roadmap-maker
  FORBID-list guard — either compares two independently-sourced values or is already honestly
  self-disclosed as "GREEN by construction, not RED coverage" (D6, pre-existing, round 6's own
  audit table). The historical `go test -run TestD2AnchorRetroExecution` vacuity (fixed Phase 10)
  was re-confirmed still fixed (0 `rg` hits repo-wide; the live gate command in the Suggested Work
  Units table cites the current test).

### 13D — Explicitly out of scope this batch
- Did NOT withdraw, re-base, or rewrite live Engram records #2789/#2096 or their committed fixture
  mirrors. Independently re-verified obs #2789's own source record before concluding anything:
  its archive-report (`openspec/changes/archive/2026-08-05-skills-validate-ondisk-gate/archive-report.md`)
  names a real review lineage (`Lineage: review-578b97aed1cb4065`, `Gate result: allow`, terminal
  state `APPROVED`) — a review genuinely ran for that candidate, so its existing `verified` outcome
  label is CORRECT under the new three-state rule, not a self-asserted claim wearing a verified
  label. No annotation is proposed for either record (see apply-progress.md for the full
  independent check on both #2789 and #2096).
- Did NOT touch cumulative size / chained PRs (a delivery question, not a code defect) or task 6.3
  (structurally deferred to archive — this change's own closure exercises the instrument, and it
  cannot happen before archive by definition).
- Did NOT re-open R-019/schema reconciliation (closed in batch 7/Phase 12).

### 13E — Re-verify
- [ ] 13E.1 Full suites green (confirmed by apply — see apply-progress.md Phase 13); re-running
  `sdd-verify` itself is a separate phase, out of apply's scope, and is the recommended next step.

## Phase 14: Remediation — reconcile R-021/D5 with the four-name `required` drop, pin `required` positively (round 7 CRITICAL-1)

Verify round 7 (evidence `sha256:ea5f683d…`) found **1 CRITICAL, 0 other findings**: Phase 12
correctly removed three compute-time fields from the schema's `required` list under R-019, but
never re-checked the sibling artifacts that constrain that schema. `spec.md:90`/`:102` (R-021's
requirement body and acceptance scenario) and `design.md:104` (D5) still claimed only
`total_wall_clock_hours` was removed from `required` — false against the shipped four-name diff.
The suite stayed green throughout because `schema.Required` was pinned only by NEGATIVE membership
checks (`R021_required_drops_wall_clock`, `R019_required_drops_compute_time_fields`), which cannot
detect a name leaving the list other than the one each negatively pins.

### 14A — Reconcile the artifact text with the schema (work unit 1)
- [x] 14A.1 `spec.md:90` (R-021 body): rewritten to name the R-019 drop explicitly and scope the
  "every other required field is unchanged" claim to exclude the four now-named fields, instead of
  falsely claiming universality.
- [x] 14A.2 `spec.md:102` (R-021's "Loosening required does not loosen the shape" scenario): THEN
  clause rewritten from "`total_wall_clock_hours` is the only name removed" to naming both R-021's
  and R-019's drops explicitly ("no other name changes"), preserving the scenario's real invariants
  (no property added/removed, `additionalProperties: false` unchanged) unweakened.
- [x] 14A.3 `design.md` D5: extended with a second, independent decision paragraph sanctioning the
  R-019 `required` drop alongside the R-021 one (previously zero design coverage — `rg
  'R-019|implementation_hours' design.md` returned 0 hits before this batch), and corrected D5's
  stale parenthetical ("the contract test does not pin the required list — verified") to reflect
  that two subtests plus the new exact-list assertion (14B) now pin it.
- [x] 14A.4 `design.md`'s File Changes table row for the schema file: extended to name both drops
  (previously named only the R-021 one, silently stale since Phase 12).
- [x] 14A.5 Swept every shipped artifact this change touches (`spec.md`, `design.md`, `proposal.md`,
  `skills/inception-pipeline/SKILL.md`, `skills/_shared/actuals-record.schema.json`) for further
  "only"/"every other .. unchanged"/"required list" exclusivity claims via `rg`. Found and fixed
  exactly the three sites round 7 cited (14A.1–14A.3); `SKILL.md:153` already carried the correct
  four-field/two-reason wording (verify round 7's Source E) and needed no edit; the schema's own
  field-scoped `required` mention (`total_wall_clock_hours`'s own description) makes no exclusivity
  claim about the whole list and needed no edit. Exhaustion proven by `rg` returning zero further
  hits after the edits (see apply-progress.md).

### 14B — Pin `schema.Required` positively, closing the structural class (work unit 2)
- [x] 14B.1 Added a positive exact-list assertion on `schema.Required` inside
  `D2_D5_closed_schema_invariant` (`engine/skills/actuals_instrumentation_contract_test.go`),
  mirroring the pre-existing `assertStringList` guard already used for `schema.Properties`:
  `wantRequired := []string{"change_name", "project", "approval_decision", "scope_drift_notes",
  "variance_vs_plan"}`. Any name entering or leaving `required`, named by an existing negative pin
  or not, now fails this assertion independently.
- [x] 14B.2 Mutation-proven against the real shipped schema, all four directions, every run with
  `go test -count=1` (per Engram obs #2903):
  (a) removed `change_name` (a name still expected in `required`) → RED (`frontmatter list length
  = 4 ... want 5`); (b) re-inserted `total_wall_clock_hours` (one of the four dropped names) →
  RED, on both the pre-existing negative pin AND the new exact-list guard; (c) added
  `requirement_count` — a brand-new name never named by any negative pin — → RED **only** on the
  new exact-list guard (`R021_required_drops_wall_clock` stayed green), reproducing round 7's exact
  CRITICAL-1 mechanism and proving the fix closes it; (d) restored byte-identical after each probe
  (`sha256:0684faa3…`, matching round 7's own restore hash) → GREEN.

### 14C — Sweep the class: other lists pinned only negatively (work unit 3)
- [x] 14C.1 Swept this change's two owned test files
  (`actuals_instrumentation_contract_test.go`, `actuals_instrumentation_d2_anchor_test.go`) for
  every `slices.Contains`/`strings.Contains`/"must not contain" list guard. Full table in
  apply-progress.md. Finding: `schema.Required` (fixed, 14B) was the only list of this shape —
  a finite enumerable JSON array with a real "exact membership" invariant — pinned only
  negatively. Every other negative-only list found (temporal-boundary FORBID phrases, D2
  abolished-vocabulary FORBID lists, the roadmap-maker no-compute-time-source FORBID list) guards
  against RETIRED prose reappearing, not against a live enumerable set's membership, so there is no
  positive complement to assert; left negative, with rationale recorded. One adjacent risk noted
  but not converted (scope/budget): the roadmap-maker FORBID list is hand-maintained against the
  schema's actuals-only field names and would not automatically flag a brand-new actuals-only field
  added to the schema in the future — a real but lower-severity "new content escapes an existing
  guard" risk, distinct from CRITICAL-1's "existing content silently leaves a pinned list" shape.
  `schema.Properties` was re-confirmed already positively pinned (pre-existing `assertStringList`
  use, the same idiom this batch reused for `required`).

### 14D — Explicitly out of scope this batch
- Did NOT re-open R-019, the `approved_tree` four-state rule, or any other finding round 7
  confirmed clean.
- Did NOT withdraw or annotate live Engram records #2789/#2096 or their fixture mirrors — round 7
  independently re-confirmed both source claims correct.
- Did NOT touch cumulative size / chained PRs (delivery question) or task 6.3 (structurally
  deferred).
- Did NOT convert the roadmap-maker FORBID list to a derived/positive form (see 14C.1) — a
  separate, lower-severity finding outside this batch's surgical remediation scope.

### 14E — Re-verify
- [x] 14E.1 Full suites green (confirmed by apply — see apply-progress.md Phase 14); re-verify round
  8 returned PASS — 0 CRITICAL, 0 blockers, requirements 5/5, scenarios 15/15, archive-ready. Carried
  two documentation-grade WARNINGs into Phase 15 (below).

## Phase 15: Remediation — close 2 documentation-grade WARNINGs (round 8), no new blockers

Verify round 8 returned **PASS** (0 CRITICAL, 0 blockers) but carried two WARNINGs forward:

### 15A — Teach `sdd-time-estimation` what to do when the three numerators are absent (W1)
- [x] 15A.1 `skills/sdd-time-estimation/SKILL.md`'s CALIBRATION rule (Hard Rules): inserted a new
  sentence immediately after "ONLY, build a per-phase agent-compute-time baseline." stating the
  R-019 consequence explicitly — WHEN `implementation_hours`/`review_gate_hours`/
  `post_review_fix_hours` is absent, the record supplies NO compute-time numerator for the affected
  phase, `total_wall_clock_hours` (or any other elapsed-time figure) MUST NOT be substituted, and
  confidence falls back to the disclosed bootstrap defaults / qualitative scaling regardless of
  calibration `n`. The pre-existing "NEVER an input to the agent-compute-time baseline" sentence
  (R009's pin) is left byte-unchanged immediately after it.
- [x] 15A.2 New subtest `R019_calibration_absent_numerators_fallback` added to
  `engine/skills/actuals_instrumentation_contract_test.go`, scoped to the CALIBRATION list item via
  the existing `markdownListItemContaining` helper (same idiom as `R001_R002_three_units_never_blended`).
  Mutation-proven, every run `go test -count=1`: (a) deleted the new sentence → RED
  (`CALIBRATION rule must contain "WHEN one or more of these three fields is absent under R-019"`);
  (b) inverted the sentence to instruct substituting `total_wall_clock_hours` for the missing
  numerator → RED (`must contain "the record supplies NO compute-time numerator..."`); (c) restored
  byte-identical (`sha256:2ed04f4e…`, `cmp`-verified) → GREEN. Full contract test suite green
  throughout at `-count=1`.
- [x] 15A.3 Consumer-enumeration sweep (governing rule: an objective that changes what a value MEANS
  must enumerate its consumers). Every file in the repo (excluding `openspec/changes/archive/**` and
  this change's own `openspec/changes/sdd-cycle-timestamp-instrumentation/**`) referencing
  `implementation_hours`, `review_gate_hours`, `post_review_fix_hours`, `checkpoint_count`, or
  `total_wall_clock_hours`, or the `sdd/{change}/actuals` / `pipeline-state` / `closure-feedback`
  topics, was read and classified. Full table in apply-progress.md Phase 15. Result: the three files
  this change already edits (schema, `inception-pipeline/SKILL.md`, `sdd-time-estimation/SKILL.md`)
  were the only genuine consumers of the *changed* meaning, and `sdd-time-estimation/SKILL.md` was
  the one left unupdated — now fixed by 15A.1/15A.2. `roadmap-maker/SKILL.md` reads actuals generically
  ("copy relevant fields") without hard-coding field names or a numerator computation (already
  guarded by pre-existing `R013_roadmap_maker_no_compute_time_source_invariant`) — genuinely
  unaffected. `entry-contract-validator/main.go`, `project-inception/SKILL.md`,
  `pre-sdd-contracts.md`, `tier-selection.md` reference only the `pipeline-state`/`actuals` topic-key
  strings or ownership, never the field semantics R-019/R-021 changed — genuinely unaffected.
  `openspec/specs/actuals-instrumentation/spec.md` (the live/merged base spec) is untouched by
  design; it is updated by OpenSpec's own archive-time merge of this change's delta spec, not by
  apply — out of this phase's scope by construction.

### 15B — Fix the revert instructions (W2)
- [x] 15B.1 `design.md`'s Migration / Rollout section: "Revert = delta spec + 5 file edits" (which
  omitted the two files this change CREATES) rewritten to name all seven files explicitly, with the
  5 edits and 2 new-file deletions distinguished, and a sentence stating why the deletion is
  mandatory — `actuals_instrumentation_d2_anchor_test.go` reads shipped `SKILL.md` prose at runtime
  (confirmed: `readRepoFile(t, repoRoot, inceptionPipelineSkillRelPath)` at line 197), so leaving it
  in place after reverting the other 5 edits pins reverted prose against a test still expecting
  shipped wording — suite RED after a "complete" revert.
- [x] 15B.2 Verified the claim before writing it: `wc -l` confirms
  `actuals_instrumentation_d2_anchor_test.go` is 415 lines and does read
  `inception-pipeline/SKILL.md` at runtime; confirmed no test pins design.md's Migration / Rollout
  prose itself (`TestAbolishedVocabularySweptFromCurrentSections`'s FORBID list does not contain any
  string introduced by this edit), so no additional pin was required for this documentation-only fix.

### 15C — Full suites, build, and scope check
- [x] 15C.1 `for m in engine tui tools/deterministic-check-runner tools/entry-contract-validator
  tools/review-preflight; do (cd "$m" && go test -count=1 ./...) || exit 1; done` — all green. `go build`
  all 5 modules, same `|| exit 1` form — exit 0. `git status --untracked-files=all` — same
  tracked/untracked file set as post-Phase-14, no stray files.
  **Corrected during review of this candidate**: this line previously recorded the loop WITHOUT
  `|| exit 1`. A bare `for` loop exits with the status of its last iteration only, so the recorded
  exit 0 attested to `tools/review-preflight` alone and silently discarded a failure in any of the
  other four modules — a gate that could not fail, reported as a pass. Both commands were re-executed
  in the corrected form and the recorded exit codes and output hash in `verify-report.md` re-derived
  from that run; see its "Provenance of the recorded commands" section for the two-line demonstration.
  That provenance is prose in the report BODY, deliberately not extra envelope keys:
  `gentle-ai.verify-result/v1` has a closed field set, so adding keys makes the whole report fail
  admission under `gentle-ai sdd-verify-validate`.
- Did NOT re-open R-019, R-021, the four-state anchor rule, the sweep guard, or anything else rounds
  7/8 confirmed clean.
- Did NOT withdraw, re-base, or rewrite live Engram records #2789/#2096 or their fixture mirrors.
- Did NOT touch the roadmap-maker FORBID list (round 8 rated it SUGGESTION, prospective-only).
- Did NOT touch cumulative size / chained PRs, or task 6.3 (structurally deferred to archive).

### 15D — Re-verify (the item Phase 15 was missing)
- [ ] 15D.1 Re-run `sdd-verify`. **This is the gap review of this candidate found, not a formality.**
  Phase 15 edited `skills/sdd-time-estimation/SKILL.md` — a shipped, agent-executed rule file — and
  added the `R019_calibration_absent_numerators_fallback` subtest, both *after* round 8 returned PASS.
  Phase 7B establishes this change's own premise that agent-executed SKILL.md prose **is** the
  implementation, so that edit changed shipped behaviour with no verification round covering it. Every
  other remediation phase carried this item (7D.1, 8E.1, 10D.1, 11C.1, 12E.1, 13E.1, 14E.1); Phase 15
  ended at 15C without one, so the loop that caught real defects after rounds 6 and 7 was not scheduled
  to run again. Full suites being green (15C.1) is NOT a substitute: rounds 6, 7 and 8 each found real
  defects against a green suite. Re-running `sdd-verify` is a separate phase outside apply's scope.
  `verify-report.md`'s `verdict: pass` certifies only through Batch 9 (Phase 14) — see the SCOPE
  banner under its title. Whoever archives decides whether 15D.1 must run first; this item does not
  claim to block on its own. Note `6.3`, `8E.1` and `13E.1` are also unchecked and coexisted with that
  same `Archive-ready: YES`, so an unchecked re-verify item has demonstrably not blocked this change
  before.
