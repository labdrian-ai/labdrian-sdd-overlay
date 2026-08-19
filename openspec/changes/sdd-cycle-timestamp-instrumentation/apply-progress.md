# Apply Progress: SDD Cycle Timestamp Instrumentation

**Change**: `sdd-cycle-timestamp-instrumentation` | **Store**: hybrid | **Mode**: Strict TDD | **Date**: 2026-08-14 (batch 7 / Phase 12)

## Status

Tasks complete: Phases 1–11 (54/59) plus Phase 12 (12A–12D complete, 12E deferred) across seven
apply batches. Batch 1 (Phases 1, 4, 5, 6.1–6.2) verified clean by `sdd-verify`. Batch 2 (Phase 7)
remediated the 3 CRITICAL blockers from the first verify pass. Batch 3 (Phase 8) remediated the
round-2 `fail / 2 blockers` result. Batch 4 (Phase 9) remediated the round-3 `fail / 2 blockers`
result (sweep-not-citations). Batch 5 (Phase 10) closed the round-4 evidence-completeness gap
(W1/W2/W3, scenarios 9/10/11). Batch 6 (Phase 11) closed the round-5 evidence-completeness gap
(scenario 14, carve-out span guard) and verify round 6 returned `pass_with_warnings`, **0
blockers, 0 CRITICAL, requirements 5/5, scenarios 14/14** — archive-ready.

**Batch 7 (Phase 12, this session) remediates a defect the orchestrator found by human diff
review AFTER round 6 passed**, so no verify round caught it: R-019 (`spec.md:100`) requires
`implementation_hours`, `review_gate_hours`, and `post_review_fix_hours` to "stay unpopulated
until a durable source exists" and its own scenario opens "GIVEN a closed record with the three
compute-time fields **empty**" — but the shipped schema
(`skills/_shared/actuals-record.schema.json`) still listed all three in `required`. Only
`total_wall_clock_hours` had been dropped, under R-021. A record OBEYING R-019 was therefore
REJECTED by the schema this change ships, and the Validate rule's own "report and STOP" clause
fired on the one thing that exercises the instrument end to end (task 6.3). R-019 was
independently re-verified as the correct side before acting (`gentle-ai sdd-attempt status` has
no timestamp fields; orchestrator telemetry is session-transient; transcripts carry no structured
subagent durations — there is genuinely no durable source yet), so the schema was reconciled to
R-019, not the reverse. The fix is the same non-destructive treatment `total_wall_clock_hours`
already received under R-021 (`required` is a floor, not a prohibition — `properties` and
`additionalProperties: false` are untouched), plus a NEW binding test that constructs an
R-019-conforming record and proves it actually validates against the shipped schema — closing the
gap that let six verify rounds pin R-019's rationale prose without ever proving a conforming
record validates. Remaining incomplete: 6.3 (deferred to archive/closure) and 12E.1 (re-run of
`sdd-verify`, a separate phase this agent does not invoke).

**Batch 8 (Phase 13, this session) removes the tautological `approved_tree` check verify round
6's W2 warning identified, and is a real defect fix, not coverage-only.** Verify round 6 passed
the change archive-ready but declined to fix W2 itself, requiring a scoped human decision: when no
review ran for a candidate, `approved_tree` was previously synthesized from `landing_commit`'s own
tree, making the mandated check `git show -s --format=%T <landing_commit>` == `approved_tree` an
`X == X` tautology that could never fail. **The human's decision**: remove the false check rather
than invent a new one — a tautological check reported as "verified" is a fabricated assurance, the
same defect class this change exists to eliminate. This batch (1) records `approved_tree` only
when a review receipt exists, never synthesized; (2) grows the resolution-outcome vocabulary from
two states (verified-or-REJECTED) to four (verified / self-asserted / rejected / absent), so a
no-review anchor is labelled self-asserted, never verified — while t1 STILL resolves for it, the
measurement is not withheld; (3) propagates the fix to `spec.md`, `design.md`, and the test suite,
mutation-proving every new pin and the `resolveD2T1` outcome enum directly (RED/GREEN transcripts
below); (4) sweeps the shipped artifacts and test suite for other structurally-unfailable checks
(one genuine instance found — the one just fixed; every other candidate inspected and found
genuine or already honestly self-disclosed); (5) independently re-verifies live Engram records
#2789/#2096 are not mislabelled under the new rule (both are correct as-is — #2789 underwent a
real review, #2096 makes no verified/rejected claim at all — so no annotation is proposed for
either). See "Batch 8 (Phase 13) — Full Record" near the end of this document for the complete
TDD evidence, files-changed table, mutation-probe transcripts, and the work-unit-3 sweep table.

**Batch boundary**: Phases 1, 4, 5, 6.1–6.2 (batch 1) are UNCHANGED by this batch — do not re-read their evidence as stale. Phases 2 and 3 (batch 1) are SUPERSEDED: their test file was deleted and their SKILL.md prose was rewritten in Phase 7, because verify proved the algorithm they encoded (receipt-store lookup + path-scan fallback) unexecutable and wrong. Phase 7 (batch 2) is superseded ONLY on the tree-resolution disambiguation rule: `skills/inception-pipeline/SKILL.md` was NOT touched this batch (it already carried the correct, ratified rule — this is round-2 C1, the polarity-reversed original C1). Phase 8 (batch 3) is the authoritative current state for the disambiguation rule and for R-016/17/18's first scenario; everything else Phase 7 landed (D1/D1b/D3/R-019/the anchor-verification mechanism itself) is UNCHANGED and still authoritative.

**Batch 4 (Phase 9)** is a pure sweep-and-repair batch: it introduces no new design decision, only propagates decisions already ratified in batches 2–3 (D1's typed field, D2 revision 3's verified-anchor mechanism) into artifact sections and a data record that had not yet caught up, plus repairs test-coverage gaps (vacuous pins) and one factual figure. `skills/inception-pipeline/SKILL.md` was **not** touched that batch — confirmed already correct by direct `rg` sweep before any edit. Batches 1–3 remain the authoritative record of their own decisions; batch 4 only extends where those decisions are reflected.

**Batch 5 (Phase 10, this session)** adds coverage only — it introduces no new design decision
and changes no shipped behavior. `skills/inception-pipeline/SKILL.md` and `design.md` were each
mutated transiently for mutation-probing (three times each) and restored byte-identically every
time; neither is a deliverable edit this batch. The only deliverable edits are in
`engine/skills/actuals_instrumentation_contract_test.go` (a new pin plus a signature change to
one internal test helper, `stripMarkdownSection`, with new table-driven coverage for the changed
helper itself) and one line of `tasks.md` (the stale gate command). Batches 1–4 remain the
authoritative record of their own decisions; batch 5 only closes coverage gaps round 4 found.

**Batch 6 (Phase 11, this session)** also adds coverage only — no new design decision, no changed
shipped behavior. `design.md` was mutated transiently once for mutation-probing (the P7
reproduction) and restored byte-identically (`cmp` confirms). The deliverable edits are: a new
committed fixture (`engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json`,
new file, mirrors Engram obs #2789), a new pin plus sweep-list entry in
`engine/skills/actuals_instrumentation_contract_test.go`, a span-integrity check added to
`stripMarkdownSection` (same function batch 5 introduced the exact-text terminator on) with two
new table-driven cases, and the new Phase 11 section in `tasks.md`. Batches 1–5 remain the
authoritative record of their own decisions; batch 6 only closes the two round-5 coverage gaps.

**Batch 7 (Phase 12, this session) is a real defect fix, not coverage-only** — it changes shipped
behavior: the schema's `required` list is genuinely smaller than it was after batch 6. The two
production edits are `skills/_shared/actuals-record.schema.json` (3 lines removed from `required`,
`properties`/`additionalProperties: false` untouched) and one sentence in
`skills/inception-pipeline/SKILL.md`'s Validate step (rewritten to name both R-021 and R-019 as
distinct reasons, not one). Everything else this batch touches is test/doc: one existing pin's
wording updated to match the reconciled sentence, one new subtest plus two new pure helper
functions in `engine/skills/actuals_instrumentation_contract_test.go`, one new table-driven
subtest for the new helper, and a new Phase 12 section in `tasks.md`. Batches 1–6 remain the
authoritative record of their own decisions; batch 7 fixes an inconsistency none of them, and no
verify round, had caught — the schema and R-019 disagreed from the moment R-019 was written in
batch 2 (Phase 7) through round 6, and no test ever constructed a conforming record to notice.

## TDD Cycle Evidence

### Batch 1 (Phases 1, 4, 5) — unchanged, reproduced here for continuity

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1/1.2 | `engine/skills/actuals_instrumentation_contract_test.go` | Unit (content-assertion) | ✅ full suite green pre-change | ✅ Written, confirmed failing (2 subtests) | ✅ Passed after 1.3 | ➖ Single (structural boundary edit, one shape per requirement) | ➖ None needed |
| 1.3 | `skills/_shared/actuals-record.schema.json` | N/A (schema data) | — | (driven by 1.1/1.2 RED) | ✅ `go test -run TestActualsInstrumentationContract` green | — | — |
| 4.1 | (regression only — doc line) | N/A | ✅ full contract test green before + after | N/A (doc-only, no new pin required) | ✅ full module suite green | ➖ None needed | ➖ None needed |
| 5.1 | `R014_amendedR007_fixture_pins` | Unit (fixture pin) | ✅ green before edit | N/A (annotation, not new behavior — existing pins re-verified) | ✅ green after edit | ➖ None needed | ➖ None needed |

Batch 1's Phase 2 (`TestD2AnchorRetroExecution`) and Phase 3 (`R016_R017_R018` subtest, first version) rows are intentionally NOT reproduced here: both were superseded and replaced in this batch (see below). Their original evidence is preserved only in git history, not restated as current.

### Batch 2 (Phase 7, this session) — remediation

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 7A.1 | — | N/A (deletion) | ✅ full suite green before deletion | N/A | ✅ `go test -run TestD2Anchor ./skills/...` → "no tests to run" after deletion, confirming the superseded algorithm and its proof are both gone | — | — |
| 7A.2/7A.3 | `engine/skills/actuals_instrumentation_d2_anchor_test.go` (new) | Integration (real repo git history) | ✅ full suite green pre-change | ✅ Written test-only, implementation stripped, real compile failure captured (`undefined: resolveD2T1`, `undefined: resolveEarliestCommitForTree`, unused imports) | ✅ Full file restored (implementation + tests together); `go test -run TestD2AnchorIsVerifiableAndUnambiguous -v` → 4/4 subtests PASS | ✅ 4 independently-sourced real-history fixtures: `be2c3ca` (matching anchor), `be2c3ca` paired with `79995ea`'s real tree (mismatched anchor), a genuinely tree-duplicated triple on `main` (`030d290b…`, 3 real commits, mixed UTC offsets), and `nil` (no anchor) | ➖ None needed — single-pass implementation matched the RED-confirmed function signatures |
| 7B.1/7B.2 | `engine/skills/actuals_instrumentation_contract_test.go` (`R016_R017_R018...`) | Unit (content-assertion) | ✅ prior subtests still green | ✅ Written against pre-edit SKILL.md, confirmed failing (`closure-feedback must contain "landing_commit"`) | ✅ Passed after rewriting the D2 paragraph; one intermediate FAIL (`must not retain "first-parent"` — my own explanatory prose used the word describing the rejected approach) fixed by rewording without changing meaning, then GREEN | ➖ 11-marker present list + 4-item FORBID list covers D1–D4 independently | ➖ Reworded one sentence to drop an accidental FORBID-list collision (see Deviations) |
| 7C.1/7C.2 | `engine/skills/actuals_instrumentation_contract_test.go` (`R019_compute_time_deferral_rationale`, new) | Unit (content-assertion) | ✅ prior subtests still green | ✅ Written against pre-edit SKILL.md, confirmed failing (`missing "session-transient"`) | ✅ Passed after adding the rationale sentence to the Validate section | ➖ Single (one prose insertion, one requirement) | ➖ None needed |

### Batch 3 (Phase 8, this session) — round-2 remediation

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 8A.4 | `engine/skills/actuals_instrumentation_d2_anchor_test.go` | Integration (real repo git history) | ✅ full suite green pre-change | ✅ Deleted `resolveEarliestCommitForTree` + its subtest first (compile-checked mentally against the new subtest calling the not-yet-written `resolveTreeToCommitOrAmbiguous`/`commitsCarryingTree`); then, post-implementation, mutation-probed the switch's `default` branch (`return "", true, nil` → `return carriers[0], false, nil`, i.e. "just return the first carrier") — subtest failed with `ambiguous = false, sha = "be2c3ca…"`, confirmed non-vacuous | ✅ Restored; `go test -run TestD2AnchorIsVerifiableAndUnambiguous -v` → 5/5 subtests PASS | ✅ Independent double-check inside the same subtest: `commitsCarryingTree` (git-log grep, no shared code path with the function under test) counts exactly 3 real carriers of tree `2ad8e42e…` before `resolveTreeToCommitOrAmbiguous` is trusted to call it ambiguous | ➖ None needed |
| 8A.4 | same file | Integration (structural binding) | ✅ same | N/A — new subtest reads shipped `SKILL.md` directly, asserted true first try since SKILL.md's prose was already correct (untouched this batch) | ✅ `shipped_skill_prose_matches_the_proven_ambiguity_rule` PASS | ➖ Single (binds the git-derived proof and the shipped prose in one file, closing the "D2 test never reads SKILL.md" gap verify named) | ➖ None needed |
| 8C.1/8C.2/8C.3/8D.1 | `engine/skills/actuals_instrumentation_contract_test.go` (`R016_R017_R018...`) | Unit (content-assertion) | ✅ prior subtests still green | ✅ For each of the 4 pins: mutated `skills/inception-pipeline/SKILL.md` (deleted/reintroduced the exact target clause), ran `go test -count=1 -run .../R016_R017_R018`, confirmed FAIL, restored via byte-identical backup, confirmed `diff` empty, re-ran to confirm PASS — see table below for the 4 individual probes | ✅ All 4 probes GREEN after restore; full `TestActualsInstrumentationContract` green | ➖ None needed — each pin targets one clause at one now-unique site | ➖ None needed |

**Mutation-probe detail (work unit 3 + FORBID marker, work unit 4):**

| Pin | Mutation applied | Result | Restored + re-run |
|---|---|---|---|
| `"excluded from this count as post-boundary bookkeeping"` | Deleted the checkpoint_count post-boundary-exclusion sentence from `SKILL.md` | RED — `must contain "excluded from this count as post-boundary bookkeeping"` | GREEN |
| `"Append a \`## Cycle Timestamps\` section to the archived"` | Deleted Execute item 3 in full from `SKILL.md` | RED — `must contain "Append a ... section to the archived"` | GREEN |
| `` "t0 falls back to the earliest `created_at` among the change" `` | Deleted the entire D1b fallback clause from `SKILL.md` | RED — `must contain "t0 falls back to the earliest ... among the change"` | GREEN |
| `"carrying that tree MUST be chosen"` (new FORBID) | Inserted the exact abolished phrase into `SKILL.md` without removing anything else | RED — `must not retain ... "carrying that tree MUST be chosen"` | GREEN |
| `resolveTreeToCommitOrAmbiguous`'s ambiguous branch | Collapsed the `switch` so `len(carriers) > 1` returns the first carrier instead of `ambiguous=true` | RED — `ambiguous = false, sha = "be2c3ca…", want ambiguous = true` | GREEN |

Every probe used the real repository — no synthetic fixtures — and the working tree was verified byte-identical to its pre-probe state (`diff` empty) after each restore, before moving to the next probe. `SKILL.md` itself was NEVER edited as a deliverable this batch — it was mutated and restored five times, transiently, only for these probes.

### Batch 4 (Phase 9, this session) — sweep remediation

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 9A.1/9A.2 | N/A (prose sweep) | N/A | ✅ full suite green pre-change | N/A — this is a sweep of prose that had no test to fail; the enforcement gate is 9B | ✅ every edit confirmed by direct `rg`/Python occurrence-count re-scan of the edited file, not assumed | ✅ independently re-derived ground truth for every factual claim touched (tree-collision count via `git rev-list`; archive-report contents via direct file read; `git show -s --format=%T` for both cited commits) before writing prose | ➖ None needed |
| 9B.1/9B.2 | `engine/skills/actuals_instrumentation_contract_test.go` (`TestAbolishedVocabularySweptFromCurrentSections`, new) | Unit (structural sweep guard) | ✅ full suite green pre-change | ✅ Written against pre-9A `design.md` (four residues at the report's cited lines), confirmed failing | ✅ Passed after 9A's edits | ✅ Three independent subtests: current-sections-clean, SUPERSEDED-block-still-carries-provenance (inverse guard), other-artifacts-have-no-carve-out | ➖ None needed |
| 9C.1/9C.2 | Engram obs #2789 (upsert) | N/A (data correction) | ✅ archive-report and `git show` are read-only, no suite risk | N/A — this is a data record, not test-covered production code | ✅ `mem_get_observation` re-read after upsert: same id, `Revisions: 3` (was 2), `topic_key` preserved as top-level | ✅ Independently re-derived: `git show -s --format=%T 79995ea` = `b654d2d006d4d151145fd6452817f97d1ddc9ebb`, matching the archive-report's own "Candidate tree" line | ➖ None needed |
| 9D.1 | `actuals_instrumentation_contract_test.go` (5 pins: W1–W5) | Unit (content-assertion) | ✅ full suite green pre-change | ✅ Each of the 5 new/repaired pins written against pre-edit content where applicable, or added directly as new coverage; all 5 individually mutation-probed (delete/insert → RED) before being counted as repaired | ✅ Full suite green after each | ➖ Each targets a distinct, previously-uncovered clause | ➖ None needed |
| 9D.2 | Both test files, full pin inventory | Unit (content-assertion) | ✅ full suite green throughout | N/A — probing existing/newly-fixed pins, not introducing new RED-then-GREEN behavior | ✅ 81/81 mutation probes reproduced the expected RED-then-restore-GREEN cycle (77 automated string-pin probes + 4 manual structural/inverse probes); 0 defects found beyond the 5 already repaired in 9D.1 | ✅ Cross-checked with an independent occurrence-count sweep (the rule from Engram obs #2884: non-covering ⇔ pin text occurs >1× in its target file) — found only the pre-catalogued W6 multi-site pins (`landing_commit`, `approved_tree`, the three compute-time field names, the calibration formula, "never subtract a guessed interruption amount", "no interruption-clean residual sample exists" — matches round 3's own audit exactly, no new finding) plus three multi-occurrence phrases in the fixture's `variance_vs_plan` narrative (`durably observed` ×3, `reconstructed from narrative` ×5, `AMB-001` ×2) that were individually inspected and found to be genuine load-bearing repeats across distinct checkpoint-itemization bullets, not one operative clause shadowed by an incidental one — not a new defect | ➖ None needed |
| 9E.1 | `design.md`, `tasks.md`, `actuals_instrumentation_d2_anchor_test.go` doc comment | N/A (factual correction) | ✅ full suite green pre/post | N/A — a doc-comment/prose figure, not test-asserted | ✅ full suite green after (the doc comment is a Go comment, not a string pin any test reads) | ✅ Re-derived by hand: `git rev-list --count main` = 434; `git rev-list main --format='%T' --no-commit-header \| sort \| uniq -d \| wc -l` = 84 (duplicated tree VALUES); `... \| uniq -cd \| awk '{s+=$1} END {print s}'` = 205 (commits IN a collision); 205/434 = 47.2% | ➖ None needed |

### Batch 5 (Phase 10, this session) — coverage remediation (round-4 evidence-completeness gaps)

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 10A.1/10A.2 | `actuals_instrumentation_contract_test.go` (`R021_required_drops_wall_clock`, extended) | Unit (content-assertion) | ✅ full suite green pre-change | ✅ Mutation-probed against the real `skills/inception-pipeline/SKILL.md`: deleted the clause → RED (`must contain "...is the one field allowed to be validly absent (R-021)"`); restored, then inverted to the abolished all-or-nothing rule → RED (same message); restored a third time, byte-identical (`diff` empty) | ✅ Full suite green after restore | ➖ Single new pin, one previously-uncovered clause; both substrings occur exactly once in the file (verified by direct count before pinning) | ➖ None needed |
| 10B.1/10B.2/10B.3 | `actuals_instrumentation_contract_test.go` (`stripMarkdownSection` signature change + `TestActualsInstrumentationGateHelpers/section_stripping`, new, 7 cases) | Unit (structural helper + content-assertion) | ✅ full suite green pre-change | ✅ Two RED layers: (a) the new table test's own cases, written against the changed function signature, confirmed to distinguish the terminator-found and terminator-not-found paths; (b) mutation-probed against the real `design.md`, reproducing round 4's V5 exactly — demoted `### D3 — Where anchors are written` to `#### D3` **and** injected `path-scan` into D3's newly-swallowed body → RED on `design_md_current_sections` | ✅ Both layers GREEN after restore (byte-identical, `diff` empty) | ✅ 7 independent table cases: successful strip, demoted-terminator-not-found, terminator-missing-entirely, section-heading-missing, invalid-heading-anchor (both arguments), fenced-terminator-does-not-match | ➖ `stripMarkdownSection`'s signature changed (added `terminatorHeading` parameter); its one caller (`design_md_current_sections`) updated in the same commit — a compile-time-enforced atomic unit |
| 10C.1 | `tasks.md` (Suggested Work Units table, row 2) | N/A (doc correction) | ✅ full suite green pre/post — no code path reads this table | N/A — a doc-only stale citation, not test-asserted (its own defect was that no test enforces it) | ✅ Confirmed the new command runs and passes: `go test -run TestD2AnchorIsVerifiableAndUnambiguous ./skills/...` → 5/5 subtests PASS. Confirmed the OLD command's vacuity directly: `go test -run TestD2AnchorRetroExecution ./skills/...` → `ok … [no tests to run]`, exit 0 | ✅ Independently re-derived: `rg -c TestD2AnchorRetroExecution` across every `.go` file in the repo = 0 hits | ➖ None needed |
| 10C.2 | N/A (sweep, no new pin) | N/A | ✅ full suite green throughout | N/A — a sweep for absence, not a RED/GREEN cycle | ✅ `rg` swept every `-run Test...`/`go test` citation in this change's own artifacts (`tasks.md`, `apply-progress.md`, `design.md`, `spec.md`, `proposal.md`, `verify-report.md`); found exactly one live stale gate (10C.1) | ✅ Cross-checked design.md's Testing Strategy table separately — already correct (fixed in 9A.1) | ➖ None needed |

**Occurrence-count re-confirmation (instrument 1, obs #2890)**: the two new pins added this batch
(`is the one field allowed to be validly absent (R-021)`, `every other resolved field is still
required`) were individually counted in their target file before being pinned — 1 occurrence
each. No existing pin was touched, so round 4's own independent 54-pin audit (which found zero
new vacuity) still stands unchanged; this batch adds exactly 2 pins to that inventory, both
single-site.

**Scenario-to-assertion mapping (instrument 2, obs #2890)**: see the full 14-row table in
"Scenario-to-Assertion Mapping (Batch 5)" below — this is the instrument that actually found W1;
occurrence counting cannot see an absent pin by construction.

### Batch 6 (Phase 11, this session) — coverage remediation (round-5 evidence-completeness gaps)

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 11A.1/11A.2 | `engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json` (new file) | N/A (fixture data) | ✅ full suite green pre-change | N/A — a new committed fixture, not a code path | ✅ `cmp` against a `jq`-extracted copy of the live obs #2789 JSON block: BYTE-IDENTICAL (see "Fixture Fidelity Proof" below) | ✅ Read via both `mem_get_observation` (MCP) and `engram export`+`jq` (CLI); both read paths agree | ➖ None needed |
| 11A.3 | `actuals_instrumentation_contract_test.go` (`R020_skillsValidateOndiskGate_fixture_mirrors_measured_rebased_record`, new) | Unit (fixture pin) | ✅ full suite green pre-change | ✅ Genuine RED-before-GREEN: temporarily removed the fixture file, ran the new subtest → RED (`open ...: no such file or directory`); restored, byte-identical (`cmp`), → GREEN. THEN 6 further mutation probes against the real fixture, each restored byte-identical between probes: numeric invert (6.64→6.4) → RED; numeric delete (remove field) → RED; re-basing sentence delete → RED; re-basing sentence invert (swap to the R-020 else-branch prose) → RED; t0-anchor sentence delete → RED; t1-anchor sentence delete → RED | ✅ Full suite green after final restore | ➖ 4 independent pins in one subtest (numeric value + 3 provenance substrings), each individually probed | ➖ None needed |
| 11A.4 | `actuals_instrumentation_contract_test.go` (`other_shipped_artifacts_have_no_carve_out` sweep list) | N/A (list extension) | ✅ full suite green pre/post | N/A — extends an existing sweep, no new RED/GREEN cycle of its own (the sweep's own guard already RED/GREEN-proven in batch 4) | ✅ New fixture confirmed already clean of abolished vocabulary — sweep passes | ➖ None needed | ➖ None needed |
| 11B.1/11B.2 | `actuals_instrumentation_contract_test.go` (`stripMarkdownSection` span-integrity check + 2 new `section_stripping` table cases) | Unit (structural helper) | ✅ full suite green pre-change | ✅ New table case (`an inserted section before the terminator widens the span...`) written and run FIRST against the pre-fix `stripMarkdownSection` → RED (`error = <nil>, want section heading not found`, confirming the old implementation silently swallowed the inserted section); its control case (a genuinely deeper nested subsection) already PASSED unchanged, proving the fix's scope is precisely bounded before the fix even landed | ✅ Both cases GREEN after the span-integrity check was added | ✅ The two cases are complementary: same-level insertion (must error) vs. strictly-deeper nesting (must not) — isolates the guard to exactly the level-boundary condition | ➖ None needed |
| 11B.3 | `design.md` (transient mutation only, not a deliverable edit) | Integration (real repository content) | ✅ full suite green pre-change | ✅ Reproduced round 5's P7 exactly against the REAL `design.md`: inserted a new `### D2c` current section containing `path-scan` immediately before the real `### D3 — Where anchors are written` terminator → RED (`design_md_current_sections`: "a section may have been inserted, widening the removed span") | ✅ Restored byte-identical (`cmp` confirms) → GREEN; confirmed the real, un-mutated `design.md` has no heading at or above the carve-out's own level inside its SUPERSEDED block (no false positive) | ✅ Full `TestAbolishedVocabularySweptFromCurrentSections` (all 3 subtests) re-run clean before and after | ➖ None needed |

**Occurrence-count re-confirmation (instrument 1, obs #2890)**: the four new substrings pinned in
`R020_skillsValidateOndiskGate_fixture_mirrors_measured_rebased_record` were individually counted
in their target file (the new fixture) before being pinned — 1 occurrence each (the file is a
single JSON object with no repeated prose). No existing pin was touched.

**Scenario-to-assertion mapping (instrument 2, obs #2890)**: see the updated 14-row table in
"Scenario-to-Assertion Mapping (Batch 6)" below — scenario 14's row now names a real assertion
and a real probe result instead of "none".

### Batch 7 (Phase 12, this session) — reconcile R-019 with the schema `required` list (orchestrator finding, post-round-6)

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 12A.1 | `skills/_shared/actuals-record.schema.json` | N/A (schema data) | ✅ full suite green pre-change | N/A — a `required`-list edit is not itself test-driven; it is the fix the new subtest in 12B drives | ✅ `go test -run TestActualsInstrumentationContract` green after; `D2_D5_closed_schema_invariant` re-run clean (its `for _, req := range schema.Required` loop now checks fewer names against the fixture, still all present) | ➖ None needed | ➖ None needed |
| 12A.2 | `skills/inception-pipeline/SKILL.md` (Validate sentence) | N/A (prose) | ✅ full suite green pre-change | ✅ Genuine RED: editing this sentence FIRST (before touching the test pin) made `R021_required_drops_wall_clock`'s existing pin fail for real — `must contain "...is the one field allowed to be validly absent (R-021)"` — confirmed by running the test before editing the pin | ✅ GREEN after the pin (12B.4) was updated to match | ➖ None needed | ➖ None needed |
| 12B.1 | `actuals_instrumentation_contract_test.go` (`R019_required_drops_compute_time_fields`, new) | Unit (content-assertion) | ✅ full suite green pre-change | ✅ Written against the shipped schema; would have failed before 12A.1 (all three fields still in `required`) — confirmed retroactively via the three individual mutation probes in 12B.3, each of which reproduces exactly the pre-fix state for one field at a time | ✅ Passed after 12A.1 | ➖ 3 independent negative pins (one per field) plus 3 independent "still declared in properties" positive pins | ➖ None needed |
| 12B.2 | `actuals_instrumentation_contract_test.go` (`validateAgainstActualsSchema`, `r019ConformingRecordFixture`, new pure helpers) | Unit (pure function) | ✅ full suite green pre-change | ✅ Table-driven coverage in `TestActualsInstrumentationGateHelpers/validate_against_actuals_schema` written directly against the new function (4 cases): valid record, valid with an optional field present, missing-required rejected, undeclared-key rejected | ✅ All 4 cases PASS | ✅ The 4 cases isolate the two independent failure modes the validator can produce (missing required vs. undeclared key) from the two success modes (bare-minimum valid vs. valid-with-optional) | ➖ None needed |
| 12B.3 | `skills/_shared/actuals-record.schema.json` (transient mutation only, not a deliverable edit) | Integration (real shipped schema) | ✅ full suite green pre-change | ✅ 3 real-file mutation probes, one per field: re-inserted `implementation_hours` into `required` → RED (`schema required list must not contain "implementation_hours"...`); restored, byte-identical (`diff` empty); repeated for `review_gate_hours` → RED; restored; repeated for `post_review_fix_hours` → RED; restored | ✅ All 3 restored byte-identical and re-confirmed GREEN | ✅ Each probe isolates exactly one field, proving the binding test (12B.1) reacts to any one of the three regressing independently, not only to all three at once | ➖ None needed |
| 12B.4 | `actuals_instrumentation_contract_test.go` (`R021_required_drops_wall_clock`'s existing pin) | Unit (content-assertion) | ✅ prior subtests still green | ✅ Genuine RED (see 12A.2 row) — the production edit landing before the pin edit is what makes this RED real rather than simulated | ✅ GREEN after the pin text was updated to `` "`total_wall_clock_hours` is validly absent when its anchors do not resolve (R-021)"`` | ➖ Single pin, one previously-covering site, wording only | ➖ None needed |
| 12C.1/12C.2 | N/A (cross-check sweep) | N/A | ✅ full suite green throughout | N/A — a sweep of every rule vs. the schema's `required` list and vice versa, not a RED/GREEN cycle | ✅ Full table below ("Rule ↔ Schema Cross-Check") — R-019 was the only disagreement found; every other field's rule and schema state already agreed | ✅ Independently re-derived from the schema and spec.md read directly this session, not carried from a prior report | ➖ None needed |

## Files Changed

### Batch 1 (unchanged, listed for continuity)

| File | Action | What Was Done |
|------|--------|---------------|
| `skills/_shared/actuals-record.schema.json` | Modified | `total_wall_clock_hours` + `checkpoint_count` descriptions moved to the merge boundary; `total_wall_clock_hours` dropped from `required` (R-021). Property set (13) and `additionalProperties: false` unchanged. |
| `skills/sdd-time-estimation/SKILL.md` | Modified | Line 28 boundary parenthetical `archive`→`merge` |
| `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json` | Modified | Appended R-020 "not re-based" provenance sentence; backported live-record backfill fields for a genuine byte-identical match |
| Engram `sdd/sync-check-repo-behind-origin/actuals` (obs #2096) | Upserted | Revision 5 — R-020 annotation, byte-identical JSON block to the fixture |
| Engram `sdd/skills-validate-ondisk-gate/actuals` (obs #2789) | Upserted | Revision 2 — `total_wall_clock_hours` re-based 6.4 → 6.64 (merge-anchored), provenance sentence naming both resolved anchors and the `receipt-bound` resolution path |

### Batch 2 (Phase 7, this session)

| File | Action | What Was Done |
|------|--------|---------------|
| `engine/skills/actuals_instrumentation_d2_retro_test.go` | **Deleted** | Asserted coverage of the batch-1 two-source-union path-scan algorithm, which design.md D2 revision 3 removes outright (verify's C1/C2 findings: the shipped prose didn't match this algorithm, and the algorithm itself picks the wrong commit for `deterministic-verification-evidence`). |
| `engine/skills/actuals_instrumentation_d2_anchor_test.go` | **Created** | `TestD2AnchorIsVerifiableAndUnambiguous` + the `d2RecordedAnchor` type, `resolveD2T1`, `resolveEarliestCommitForTree`, `runGitReadOnly` — all test-file-only per ADR-15. Proves design.md D2 revision 3: matching-tree anchors resolve; mismatched-tree anchors are rejected, not trusted; a genuinely tree-duplicated commit resolves to the earliest carrier; a `nil` anchor (no recorded landing_commit) yields no t1 and cannot invoke a folder scan by construction (the function's signature accepts no path/slug argument). |
| `engine/skills/actuals_instrumentation_contract_test.go` | Modified | `R016_R017_R018_closure_feedback_anchor_prose`: swapped `receipt-bound`/`path-scan`(-as-required) for revision-3 markers (`landing_commit`, `approved_tree`, `verifiable rather than merely asserted`, `REJECTED`, `earliest`) and added `receipt-bound`/`path-scan`/`first-parent` to the FORBID list. New subtest `R019_compute_time_deferral_rationale` pins the three named technical reasons. |
| `skills/inception-pipeline/SKILL.md` | Modified | D2 paragraph (lines ~144–149) fully rewritten to revision 3: no receipt-store lookup, no folder scan anywhere; `landing_commit` + `approved_tree` recorded at delivery in the archive-report; verified via `git show -s --format=%T <landing_commit>` == `approved_tree` or rejected; earliest-carrier disambiguation rule for tree-to-commit resolution; omit-and-disclose when no anchor was ever recorded. Line 138 (`final_candidate_tree` mention) and the D3 Execute-item-3 table description (line 160) updated to match. New sentence in Validate (R-019 rationale: session-transient telemetry, `sdd-attempt` ledger has no timestamp fields, transcripts carry no structured subagent durations). |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/tasks.md` | Modified | Phase 7 (7A/7B/7C) marked `[x]`; 7D.1 reworded — full suites confirmed green by apply, but re-running `sdd-verify` itself is a separate phase this agent does not invoke. |

### Batch 3 (Phase 8, this session)

| File | Action | What Was Done |
|------|--------|---------------|
| `openspec/changes/sdd-cycle-timestamp-instrumentation/specs/actuals-instrumentation/spec.md` | Modified | Normative disambiguation clause (line 42) and the "Anchors resolve and are legible in both stores" scenario (lines 46-50) rewritten to the ratified rule and the `landing_commit`/`approved_tree` mechanism. See exact before/after below. |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/design.md` | Modified | D2 revision 3 "Disambiguation rule" paragraph (line 57) rewritten; cites the real 3-way collision as proof. See exact before/after below. |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/tasks.md` | Modified | 7A.2 and 7B.1 reworded from earliest-carrier language to the ambiguity rule; Phase 8 section appended documenting this batch; 7D.1/8E.1 checkbox updated. See exact before/after below. |
| `engine/skills/actuals_instrumentation_contract_test.go` | Modified | `R016_R017_R018_closure_feedback_anchor_prose`: 3 non-covering pins repaired (`bookkeeping` → operative checkpoint_count clause; `Cycle Timestamps` → operative Execute-item-3 clause; new D1b-fallback pin, previously absent) and 1 new FORBID marker added (`"carrying that tree MUST be chosen"`). Net +27/-4 lines this batch (isolated by diffing against the pre-batch-8 reconstruction). |
| `engine/skills/actuals_instrumentation_d2_anchor_test.go` | Modified | Deleted `tree_resolution_selects_the_earliest_carrier` + `resolveEarliestCommitForTree` (encoded the abolished rule); added `tree_carried_by_more_than_one_commit_is_ambiguous_never_resolved_by_position` + `shipped_skill_prose_matches_the_proven_ambiguity_rule` + helpers `commitsCarryingTree` / `resolveTreeToCommitOrAmbiguous`. Net +99/-60 lines this batch (isolated the same way). |

**`skills/inception-pipeline/SKILL.md` was NOT modified as a deliverable this batch.** It already carried the ratified rule ("verify, never discover"; "the anchor is ambiguous … never resolved by position") before this session started — that is what round-2 C1 found: the prose was already correct, and only the other four artifacts had not caught up. Confirmed byte-identical to its pre-session state via `diff` after every mutation-probe restore.

### Batch 4 (Phase 9, this session)

| File | Action | What Was Done |
|------|--------|---------------|
| `openspec/changes/sdd-cycle-timestamp-instrumentation/design.md` | Modified | Full sweep (9A.1): lines 7/11 (Gating Question / Technical Approach — clarified as evidence-only vs. the shipped typed-field mechanism), 40 (reworded "path-scan" → "folder-scan" to keep the FORBID-adjacent term out of even explanatory prose, matching batch 2's own precedent), 57 (tree-collision figure, see below), 110 (D7 #2789 entry — rewritten to the archive-report-versioned-prose resolution, see Work Unit 3), 118 (Data Flow diagram), 133 (File Changes closure-feedback edit list — literally said "harvest t0 via the footer", contradicting D1), 137 (Interfaces/Contracts anchor sentence — previously mandated naming a FORBIDDEN `first-parent scan` resolution path), 144 (Testing Strategy — described a deleted test), 153 (Threat Matrix — claimed a nonexistent `git -C` pin and RED coverage). Batch-4-isolated diff: +12/-12 (24 changed lines, verified against a reconstruction of the pre-batch-4 file). |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/tasks.md` | Modified | 7A.2's tree-collision figure corrected (9E.1); a correction note appended after 5.3 documenting the #2789 provenance fix (9C); new "Phase 9" section (9A–9E) documenting this batch in full. Batch-4-isolated diff: +89/-2 (91 changed lines). |
| `engine/skills/actuals_instrumentation_contract_test.go` | Modified | 5 pins repaired (W1: `REJECTED` → operative rejection clause; W2: `append-only carve-out` → operative Execute-item-3 clause; W3: new git-show-command pin; W4: new `tiering-go-ahead to merge` pin + FORBID counterpart; W5: new R-020 fixture pin) plus two new helpers/tests: `stripMarkdownSection` (complement of the existing `sliceMarkdownSection`) and `TestAbolishedVocabularySweptFromCurrentSections` (work unit 2's structural sweep guard, 3 subtests). Batch-4-isolated diff: +147/-5 (152 changed lines, verified against a reconstruction of the pre-batch-4 file with all four pin edits reverse-applied and both new symbols removed). |
| `engine/skills/actuals_instrumentation_d2_anchor_test.go` | Modified | Doc comment (lines 18-19) tree-collision figure corrected to match design.md. Batch-4-isolated diff: +3/-2 (5 changed lines). |
| Engram `sdd/skills-validate-ondisk-gate/actuals` (obs #2789) | Upserted | Revision 3 — `total_wall_clock_hours` value UNCHANGED (6.64, independently re-confirmed correct); resolution-path provenance in `variance_vs_plan` and "Hour derivation" rewritten from the forbidden `receipt-bound`/`path-scan` mechanisms to the archive-report's own versioned prose (see Work Unit 3 below). |

**Batch-4 total authored diff**: 24 + 91 + 152 + 5 = **272 changed lines**, against the 400-line
budget acquired for this attempt (`gentle-ai sdd-attempt acquire … --max-changed-lines 400`).
This is the batch-local figure, computed the same way batch 3 computed its own (reconstructing
each file's pre-batch-4 state and diffing directly, not `git diff` against `main`, which would
include all three prior batches). The cumulative-candidate concern round 3 flagged (W9: 468
lines against 400 before this batch) is **carried, not resolved** — this batch adds 272 more
authored lines on top, which the next verify/delivery step needs to account for; it was not in
this batch's remediation scope to resolve.

### Batch 5 (Phase 10, this session)

| File | Action | What Was Done |
|------|--------|---------------|
| `engine/skills/actuals_instrumentation_contract_test.go` | Modified | One new pin (W1: `R021_required_drops_wall_clock` extended to read `inceptionPipelineSkillRelPath` and pin the write-anyway clause). `stripMarkdownSection`'s signature changed to take an explicit `terminatorHeading` argument matched by exact text instead of inferring the terminator by heading level (W2); its one caller (`design_md_current_sections`) updated; new `supersededTerminatorHeading` constant. New table-driven test `TestActualsInstrumentationGateHelpers/section_stripping` (7 cases) covering the changed helper. Batch-5-isolated diff (reconstructed by reversing every edit against the pre-batch-5 file and diffing): **+132/-9 (141 changed lines)**. |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/tasks.md` | Modified | Row 2 of the Suggested Work Units table (W3: stale `TestD2AnchorRetroExecution` gate → `TestD2AnchorIsVerifiableAndUnambiguous`, harness corrected to the real shipped `git show -s --format=%T` command). New "Phase 10" section (10A–10D) documenting this batch. Batch-5-isolated diff (same reconstruction method): **+58/-1 (59 changed lines)**. |

**Batch-5 total authored diff**: 141 + 59 = **200 changed lines**, against the 400-line budget
acquired for this attempt (`gentle-ai sdd-attempt acquire … --max-changed-lines 400
--request-id phase10-coverage-2026-08-14-actor`). No production/schema/prose file was edited as
a deliverable this batch — `skills/inception-pipeline/SKILL.md` and `design.md` were each
mutated transiently for mutation-probing only and independently verified byte-identical to their
pre-probe state after every restore (`diff` empty each time). `apply-progress.md`'s own update is
bookkeeping, not authored production/spec risk, consistent with how every prior batch treated
its own self-update.

### Batch 6 (Phase 11, this session)

| File | Action | What Was Done |
|------|--------|---------------|
| `engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json` | **Created** | New committed fixture mirroring Engram obs #2789 (`sdd/skills-validate-ondisk-gate/actuals`, revision 3), byte-identical to the live record's embedded JSON block (proof below). Same mechanism `corrected-actuals-sync-check-repo-behind-origin.json` already applies to obs #2096. **15 lines, all new.** |
| `engine/skills/actuals_instrumentation_contract_test.go` | Modified | New constant `correctedActualsSkillsValidateOndiskGateFixtureRelPath` (+8 lines). New subtest `R020_skillsValidateOndiskGate_fixture_mirrors_measured_rebased_record` pinning the fixture's re-based `total_wall_clock_hours` value and three provenance substrings (+48 lines). Added the new fixture to the `other_shipped_artifacts_have_no_carve_out` sweep list (+1 line). `stripMarkdownSection` gained a span-integrity check rejecting any same-or-shallower-level heading before its terminator, closing round 5's W-A/P7 insertion vector; doc comment extended to explain it. Two new table cases in `TestActualsInstrumentationGateHelpers/section_stripping` (the insertion case and its deeper-nesting control). Batch-6-isolated diff (reconstructed by reversing every edit against the pre-batch-6 file and running `diff -u`, the same method batch 3/4/5 used): **+111/-1 (112 changed lines)**. |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/tasks.md` | Modified | 10D.1 marked `[x]` (verify round 5 has since run). New "Phase 11" section (11A–11C) documenting this batch. Batch-6-isolated diff (same reconstruction method): **+66/-2 (68 changed lines)**. |

**Batch-6 total authored diff**: 112 + 15 + 68 = **195 changed lines**, against the 400-line
budget acquired for this attempt (`gentle-ai sdd-attempt acquire … --max-changed-lines 400
--request-id phase11-scenario14-2026-08-14-actor`). No production/schema/prose file already
shipped by this change was edited as a deliverable this batch — `design.md` was mutated
transiently once, for the P7 reproduction, and independently verified byte-identical to its
pre-probe state after restore (`cmp` empty). `apply-progress.md`'s own update is bookkeeping, not
authored production/spec risk, consistent with how every prior batch treated its own self-update.

### Batch 7 (Phase 12, this session)

| File | Action | What Was Done |
|------|--------|---------------|
| `skills/_shared/actuals-record.schema.json` | Modified | Removed `implementation_hours`, `review_gate_hours`, `post_review_fix_hours` from `required` (R-019). `properties` block, property description text, and `additionalProperties: false` untouched. **3 lines deleted, 0 added.** |
| `skills/inception-pipeline/SKILL.md` | Modified | Validate-step sentence rewritten to name `total_wall_clock_hours` (R-021, anchors did not resolve) and the three compute-time fields (R-019, no durable source exists yet) as two distinct reasons on four fields, replacing the stale "the one field" phrasing. **1 line changed (single logical sentence, +1/-1).** |
| `engine/skills/actuals_instrumentation_contract_test.go` | Modified | `R021_required_drops_wall_clock`'s pin updated to match the reconciled sentence (+1/-1). New subtest `R019_required_drops_compute_time_fields`: 3 negative pins (fields absent from `schema.Required`) + 3 positive pins (fields still in `schema.Properties`) + a call to the new `validateAgainstActualsSchema` against the real shipped schema with `r019ConformingRecordFixture()`, the binding proof this batch exists to add (+38 lines). New pure helpers `validateAgainstActualsSchema` and `r019ConformingRecordFixture` (+37 lines). New table-driven subtest `TestActualsInstrumentationGateHelpers/validate_against_actuals_schema` (4 cases) covering the new helper directly (+47 lines). Batch-7-isolated diff (computed directly from each Edit's old/new text, the same reconstruction discipline batches 3–6 used): **+123/-2 (125 changed lines).** |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/tasks.md` | Modified | New "Phase 12" section (12A–12E) documenting this batch; 11C.1 marked `[x]` (verify round 6 has since run and passed). |

**Batch-7 total authored diff**: 3 (schema.json) + 2 (SKILL.md) + 125 (test file) = **130 changed
lines**, against the 400-line budget acquired for this attempt (`gentle-ai sdd-attempt acquire …
--max-changed-lines 400 --request-id phase12-r019-schema-2026-08-14-actor`). This is the smallest
batch of the seven — the fix itself is a 3-line schema edit and a 1-sentence rewrite; almost all
of the changed-line count is the new binding test and its direct unit coverage, which is the
point: the test, not the fix, is what prevents this exact defect class from recurring.
`apply-progress.md`'s and `tasks.md`'s own updates are bookkeeping, not authored production/spec
risk, consistent with how every prior batch treated its own self-update.

### Fixture Fidelity Proof (Batch 6, obs #2789 mirror)

Read path used: BOTH `mem_get_observation(2789)` via the MCP Engram tool (for review) AND
`engram export <file>` + `jq -r '.observations[] | select(.id==2789) | .content'` (CLI) to
extract the typed observation content, then a Python script isolated the embedded ` ```json `
code block by regex and wrote it verbatim to the fixture path — the same typed-extraction
discipline D1 already establishes for `created_at`, applied here to a full observation body.

```
$ jq -r '.observations[] | select(.id==2789) | .content' engram-export.json > obs2789-content.txt
$ python3 -c "import re; ... extract the \`\`\`json ... \`\`\` block ..." > obs2789-live-block.json
$ cp obs2789-live-block.json engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json
$ cmp obs2789-live-block.json engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json
(no output — BYTE-IDENTICAL)
```

Re-confirmed after every mutation probe and at the end of the batch:

```
$ cmp obs2789-live-block.json engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json
(no output — BYTE-IDENTICAL, final state)
```

## Work Unit 3 — obs #2789 correction: an independent finding that revises the orchestrator's own framing

The launch prompt's summary of the round-3 verify report stated: *"0 of 19 archived changes
record `landing_commit`/`approved_tree`, and that change's archive-report names no merge
SHA... So t1 could not legitimately resolve, and R-020's else-branch — omit
`total_wall_clock_hours`, annotate why — was required."*

Before acting on that, this batch independently re-verified it against the actual file, per the
global rule against acting on unverified claims. The **first half is true**: `rg -l
'landing_commit|approved_tree' openspec/changes/archive/` returns 0 of 19 — no archived change
uses those literal field names, because that convention is introduced by this very change and
was never applied retroactively.

The **second half is false**, directly disproved by the file itself:

```
$ rg -n 'commit `?79995ea|Merged to main' openspec/changes/archive/2026-08-05-skills-validate-ondisk-gate/archive-report.md
39:- **Merged to main:** YES — commit `79995ea` (PR #130)
146:✓ Merged to main with all 6 CI gates passing
```

The archive-report also names the candidate tree two lines earlier ("**Candidate tree:**
`b654d2d006d4d151145fd6452817f97d1ddc9ebb`"), and `git show -s --format=%T 79995ea` returns
exactly that value — a verified match. This is precisely the "historical-reconstruction" case
`skills/inception-pipeline/SKILL.md`'s D2 paragraph already documents: *"WHEN reconstructing a
historical record whose archive-report names a landing SHA in versioned prose... use that SHA
directly — it is the recorded identity, and the tree is only a cross-check on it."* t1
**legitimately resolves** for this record; R-020's else-branch (omit-and-annotate) does not
apply, because a versioned anchor **does** exist — just not under the literal field names the
convention introduces going forward.

Cross-checked the population claim too, since the prior report also stated "archive-report.md
names no merge SHA... only 1 of 19 archive-reports does": a direct sweep of all 19
`archive-report.md` files found **5 of 19** name a landing SHA in versioned prose (`fe28245`,
`98abdf4`, `0094afd`, `be2c3ca`, `79995ea`) — matching Engram obs #2884's own revision-3
correction, not the round-3 verify report's "1 of 19" figure for this specific claim.

**Consequence for the fix actually applied**: `total_wall_clock_hours = 6.64` was already
numerically correct and stays unchanged. Only the *disclosed resolution path* was wrong — it
named the abolished `receipt-bound`/`path-scan` mechanisms instead of the archive-report's own
versioned prose, which is what the number should have cited all along. Obs #2789 (revision 3)
corrects only that disclosure; see the exact before/after in the Engram upsert content above and
the full corrected record via `mem_get_observation(2789)`.

Checked obs #2096 and `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json`
for the same class of violation — both already clean, no forbidden vocabulary present.

## Abolished-Language Sweep (Batch 4) — evidence

Every shipped artifact this change touches, swept directly (not assumed from a prior report):

```
$ rg -n 'path-scan|first-parent|receipt-bound' \
    openspec/changes/sdd-cycle-timestamp-instrumentation/proposal.md \
    openspec/changes/sdd-cycle-timestamp-instrumentation/specs/actuals-instrumentation/spec.md \
    skills/inception-pipeline/SKILL.md skills/sdd-time-estimation/SKILL.md \
    skills/_shared/actuals-record.schema.json \
    engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json
# (no output — zero hits in any of these six files)
```

`design.md`, isolated to its CURRENT (non-SUPERSEDED) sections via the same section-slicing
logic the new `TestAbolishedVocabularySweptFromCurrentSections` uses:

```
$ python3 - <<'EOF'
# strips the "### D2 (revision 2 — SUPERSEDED, retained for provenance)" section
# (lines 65-90) and searches everything else
for w in ["path-scan","first-parent","receipt-bound",
          "carrying that tree MUST be chosen",
          "earliest commit on the default branch carrying","84 of 434"]:
    print(w, current_sections.count(w))
EOF
path-scan 0
first-parent 0
receipt-bound 0
carrying that tree MUST be chosen 0
earliest commit on the default branch carrying 0
84 of 434 0
```

Zero hits outside the explicitly-marked SUPERSEDED block, in every shipped artifact. This is now
enforced by `TestAbolishedVocabularySweptFromCurrentSections` (3 subtests, all mutation-proven —
see the mutation probe table above), not merely a one-time cleanup.

### Exact before/after — spec.md, design.md, tasks.md (per the scope-discipline requirement)

**spec.md line 42, normative clause:**
- BEFORE: `WHEN a change is delivered as chained slices, the recorded anchor MUST be the last slice to land. WHEN a tree hash must be resolved to a commit, the **earliest** commit on the default branch carrying that tree MUST be chosen, because trees are not unique across commits.`
- AFTER: `WHEN a change is delivered as chained slices, the recorded anchor MUST be the last slice to land. A tree hash MUST be used to **verify** a commit, never to **discover** one, because trees are not unique across commits — a content-preserving merge reproduces its parent's tree. WHEN a tree hash is all that is available and more than one commit on the default branch carries it, the anchor is ambiguous: t1 MUST be omitted and the ambiguity disclosed, never resolved by position.`

**spec.md lines 46-50, scenario "Anchors resolve and are legible in both stores":**
- BEFORE GIVEN: `a change with a \`pipeline-state\` observation and an approved receipt whose candidate tree is on the default branch`
- AFTER GIVEN: `a change with a \`pipeline-state\` observation and a versioned archive-report recording \`landing_commit\` and \`approved_tree\` at delivery`
- BEFORE THEN: `t0 comes from the typed \`created_at\` field, t1 from the commit carrying that receipt's candidate tree, and both appear with their sources named in the actuals record and the archive-report`
- AFTER THEN: `t0 comes from the typed \`created_at\` field, t1 is the committer timestamp of \`landing_commit\` once its own tree is verified to equal the recorded \`approved_tree\`, and both appear with their sources named in the actuals record and the archive-report`
- WHEN line unchanged (`closure-feedback closes the cycle`).

**design.md line 57, "Disambiguation rule":**
- BEFORE: `**Disambiguation rule.** Resolving by tree alone is ambiguous: **84 of 434 commits on \`main\` share a tree with another commit** (~19%), because merges that change no content reproduce their parent's tree. WHEN a tree must be resolved to a commit, take the **earliest** commit on the default branch carrying that tree — the one that first introduced the content — never the most recent. The recorded \`landing_commit\` avoids this search entirely; the rule exists only for verification and for reconstructing historical records.`
- AFTER: `**Disambiguation rule.** Resolving by tree alone is ambiguous: **84 of 434 commits on \`main\` share a tree with another commit** (~19%), because merges that change no content reproduce their parent's tree. A tree hash MUST be used to **verify** a commit, never to **discover** one: WHEN a tree hash is all that is available and more than one commit on the default branch carries it, the anchor is ambiguous — t1 is omitted and the ambiguity disclosed, never resolved by position. "Earliest carrier" is a guess wearing the costume of a rule: this repository contains a real three-way collision (tree \`2ad8e42e…\`, carried by \`be2c3ca\` — the true landing of \`deterministic-verification-evidence\` — \`aa81361\`, and \`459b48d\`), where earliest-by-time selects \`459b48d\`, a commit belonging to a different change. The recorded \`landing_commit\` avoids this search entirely; the rule exists only for verification and for reconstructing historical records whose archive-report already names a landing SHA in versioned prose.` (The "84 of 434" / "~19%" figure is unchanged — verify's S1 SUGGESTION flagged it as imprecise, but S1 is out of this batch's scope per the orchestrator's explicit instruction, and it is not one of the two blockers.)

**tasks.md 7A.2:**
- BEFORE: `...(c) resolving a tree to a commit selects the **earliest** carrier — pin this against a tree that genuinely repeats on \`main\` (84 of 434 commits share a tree, so a real duplicate exists to use as a fixture); (d)...`
- AFTER: `...(c) a tree carried by more than one commit is **ambiguous** — t1 is omitted and the ambiguity disclosed, never resolved by position — pin this against a tree that genuinely repeats on \`main\` (84 of 434 commits share a tree, so a real duplicate exists to use as a fixture); (d)...`

**tasks.md 7B.1:**
- BEFORE: `...Required present: \`approved_tree\`, \`landing_commit\`, "earliest", the rejection rule. Required absent: \`path-scan\`, \`first-parent\`, \`receipt-bound\`.`
- AFTER: `...Required present: \`approved_tree\`, \`landing_commit\`, the verify-never-discover rule, the ambiguity-disclosure rule, the rejection rule. Required absent: \`path-scan\`, \`first-parent\`, \`receipt-bound\`, the positional/earliest-carrier resolution phrasing ("carrying that tree MUST be chosen").`

None of these five edits added a requirement, changed scope, or weakened/deleted/relaxed an acceptance criterion — each replaces "earliest carrier is chosen" with "carried-by-more-than-one is ambiguous, omitted, disclosed", which is strictly the already-ratified rule (proven correct in the prior session against real repository history) applied to the four artifacts that had not yet been updated to match it.

## Carried Finding (out of scope, explicitly not fixed this batch)

**The `approved_tree` tautology** (verify's W5, restated in the round-2 report): `SKILL.md:147` defines `approved_tree` as the receipt's `final_candidate_tree` when review ran, otherwise `landing_commit`'s own tree. In the otherwise-branch, `git show -s --format=%T <landing_commit>` equals `approved_tree` by construction — the verification check can never reject in the case with no independent authority. This was flagged by the orchestrator as out of scope for this batch (not one of the two blockers) and is left untouched, exactly as found.

## 7B.3 — Shipped prose executed verbatim against `deterministic-verification-evidence`

Real commands, real output:

```
$ grep -n "merged as" openspec/changes/archive/2026-08-05-deterministic-verification-evidence/archive-report.md
25:**Delivery**: 9 PRs — #121–#128 as slices into the tracker branch, #129 as the tracker
roll-up into `main`, merged as `be2c3ca`. ...

$ git show -s --format='%H %T %cI %s' be2c3ca
be2c3ca46ea5f4f740ecab6b51dc2db3f167f37c 2ad8e42e4f1ef4ff3e46c447d70e6123664f2180 2026-08-05T10:10:53-03:00 Merge pull request #129 from labdrian-ai/feat/deterministic-verification-evidence

$ git show -s --format='%H %T %cI %s' 79995ea
79995ea1733fce362b86d3727184eb539f2c1832 b654d2d006d4d151145fd6452817f97d1ddc9ebb 2026-08-05T17:51:17-03:00 Merge pull request #130 from labdrian-ai/feat/skills-validate-ondisk-gate
```

Per the rewritten SKILL.md D2 paragraph, this historical archive-report names its landing
commit directly in prose (`be2c3ca`) — that IS the recorded `landing_commit` for this
"reconstructing a historical record" case. The result is `be2c3ca`, exactly matching the
archive-report's own claim, and `be2c3ca != 79995ea` (confirmed above: different SHA,
different tree, different PR). The new algorithm has **no folder-scan step at all** — the
only D2 test that exists (`TestD2AnchorIsVerifiableAndUnambiguous`) proves anchor
verification and earliest-carrier disambiguation, neither of which can produce `79995ea`
for this change, because `79995ea` was never recorded as this change's `landing_commit`
anywhere. This closes verify's C1 (shipped prose now matches the only algorithm the tests
prove) and C2 (the case that broke the old algorithm cannot recur, because the mechanism
that produced the wrong answer — folder scanning — no longer exists).

## Deviations from Design

Batch 1's deviations 1–2 (documented in git history / superseded content) no longer apply:
the two-source-union path-scan algorithm they described is deleted in this batch. Batch 1's
deviations 3–6 (historical fixture/live-record drift repair; t0/t1 resolution for the
`skills-validate-ondisk-gate` re-base; Engram CLI-vs-MCP disclosure) are UNCHANGED and still
accurate — see git history for their full text if needed; they are not restated here since
they concern Phase 5, which this batch did not touch.

**New deviation (Batch 2):** the R016_R017_R018 rewrite's own explanatory prose ("Scanning
commit history for whichever commit touched `openspec/changes/{change}/` is not a safe
substitute...") initially used the literal word "first-parent" while describing *why* the
old approach is rejected — which collided with the new FORBID-list entry for that same
word (added specifically to prevent the removed mechanism's name from resurfacing). Fixed
by rewording the sentence to describe the same rejected mechanism without the literal
substring; no semantic content was lost. This is a reminder that a negative test pin can be
tripped by the positive documentation of *why* something is forbidden, not only by a
regression — worth flagging for future FORBID-list additions in this file.

## Issues Found

None beyond the deviation above (resolved, not left open). One nuance carried over from
verify's own report: W6 ("bookkeeping" is a single common English word standing in for an
R-003/4/5 marker) was not addressed — Phase 7's scope was the three CRITICAL blockers only,
and W6 is a WARNING, not a blocker; it was left as verify found it, per the remediation
batch's explicit instruction not to re-open Phases 1/4/5's surviving pins.

## Remaining Tasks

- [ ] 6.3 — this change's own closure (n=3) exercises the path live. Not executable during
  apply: it requires this change to actually archive. Left for the archive/closure phase.
- [ ] 8E.1 — carried as a bookkeeping checkbox from batch 3; `sdd-verify` has since run multiple
  times (rounds 2–5), so the underlying re-verify step it names did happen — left unchecked here
  because updating it was never part of any batch's named scope and this batch did not touch it.
- [x] 10D.1 — full suites were green (batch 5); `sdd-verify` was re-run and returned round 5
  (evidence `sha256:242912ab…`, 0 blockers, 0 CRITICAL, scenarios 13/14) — see Phase 11 above for
  this batch's remediation of that round's two findings.
- [x] 11C.1 — full suites were green (batch 6); `sdd-verify` was re-run and returned round 6
  (`pass_with_warnings`, 0 blockers, 0 CRITICAL, requirements 5/5, scenarios 14/14,
  archive-ready).
- [ ] 12E.1 — full suites are green (confirmed below, batch 7); re-running `sdd-verify` is the
  recommended next step. Same nature as 8E.1/10D.1/11C.1: this agent does not invoke the verify
  phase.

## Workload / PR Boundary

- Mode: single PR (forecast confirmed no chaining needed; batch 3 stayed well under budget)
- Current work unit: Phase 8 (8A–8D) — propagate the round-2 inversion, repair the
  self-contradicting scenario, close three non-covering pins, add the structural guard
- Boundary: starts at 8A.1 (spec.md normative clause), ends at 8D.2 (SKILL.md-reading
  binding test landed); 8E.1 (re-verify) explicitly left for the next phase
- Batch 3 authored diff, isolated from batches 1-2 by reconstructing each file's pre-batch-8
  state and diffing against it directly (not `git diff` against `main`, which would include
  the prior two batches' uncommitted changes too):
  - `spec.md`: +3/-3
  - `design.md`: +1/-1
  - `tasks.md` (8A.1-8A.3 content edits only, excluding the new Phase 8 tracking section):
    +2/-2
  - `actuals_instrumentation_contract_test.go`: +27/-4
  - `actuals_instrumentation_d2_anchor_test.go`: +99/-60
  - **Total: 132 insertions + 70 deletions = 202 changed lines**, well under the 400-line
    budget. (The Phase 8 tracking section appended to `tasks.md` and this apply-progress
    update itself are bookkeeping, not authored production/spec risk, consistent with how
    batch 2 treated `tasks.md` checkbox updates.)

## Rollback Boundary

- Batch 1 rollback boundaries are unchanged (see git history) — Phase 5 fixture/Engram
  rollback still applies exactly as previously documented.
- 7A: `git rm engine/skills/actuals_instrumentation_d2_anchor_test.go` (no other file
  depends on it) restores the pre-remediation state for this test only; the original
  deleted file remains recoverable from git history if ever needed for reference (it is not
  reintroduced — its algorithm is design-superseded).
- 7B: revert the `inception-pipeline/SKILL.md` D2-paragraph hunk (lines ~138, ~144-149,
  ~160) + the `R016_R017_R018_closure_feedback_anchor_prose` subtest hunk together —
  co-edited, one atomic unit.
- 7C: revert the `inception-pipeline/SKILL.md` Validate-section R-019 sentence + the
  `R019_compute_time_deferral_rationale` subtest together — co-edited, one atomic unit.
- 8A/8B: revert the `spec.md`/`design.md`/`tasks.md` hunks documented in the exact
  before/after section above, together with the
  `tree_carried_by_more_than_one_commit_is_ambiguous_never_resolved_by_position` subtest and
  its `commitsCarryingTree`/`resolveTreeToCommitOrAmbiguous` helpers in
  `actuals_instrumentation_d2_anchor_test.go` — one atomic unit (reverting the prose without
  the test, or vice versa, reintroduces the C1/round-2 drift this batch closes).
- 8C: revert the three pin edits in `R016_R017_R018_closure_feedback_anchor_prose`
  independently — each repairs a distinct, previously non-covering pin and has no
  cross-dependency on the other two or on 8A/8B/8D.
- 8D: revert the new FORBID marker and the `shipped_skill_prose_matches_the_proven_ambiguity_rule`
  subtest together — both are the structural guard; reverting one without the other leaves
  either an unguarded FORBID gap or an assertion with nothing to bind it to.
- 9A: revert the `design.md` hunks (lines 7, 11, 40, 57, 110, 118, 133, 137, 144, 153) — pure
  prose, no code dependency; reverting drops the sweep back to round-3's state, reintroducing
  the C1 defect.
- 9B: revert `stripMarkdownSection` and `TestAbolishedVocabularySweptFromCurrentSections`
  together — the test depends on the helper; reverting one without the other is a compile
  failure, not a partial revert.
- 9C: the Engram upsert (obs #2789 revision 3) is reversible by upserting revision 2's exact
  content back onto the same `topic_key` (preserved verbatim in this file's Batch 1 section and
  in git history of this document).
- 9D: revert each of the 5 pin edits (W1–W5) independently — each repairs a distinct,
  previously non-covering or absent pin with no cross-dependency on the others or on 9A/9B/9C.
- 9E: revert the three tree-collision-figure edits (`design.md:57`, `tasks.md:85`,
  `actuals_instrumentation_d2_anchor_test.go:18-19`) independently — pure factual correction, no
  behavioral dependency.
- 10A: revert the new pin block in `R021_required_drops_wall_clock` alone — independent of every
  other batch-5 change, no cross-dependency.
- 10B: revert `stripMarkdownSection`'s signature change, its caller update in
  `design_md_current_sections`, and the new `TestActualsInstrumentationGateHelpers/section_stripping`
  table together — the caller and the helper are compile-coupled (reverting one without the
  other is a compile failure, not a partial revert), and the new table test exists specifically
  to cover the changed helper's behavior.
- 10C: revert `tasks.md` row 2 alone — pure doc correction, no code dependency.
- 11A: `git rm engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json` +
  revert the `R020_skillsValidateOndiskGate_fixture_mirrors_measured_rebased_record` subtest and
  the sweep-list entry together — the subtest reads the fixture path, so removing one without
  the other leaves either a dangling read or an unguarded fixture; no cross-dependency on any
  other batch-6 or prior-batch change.
- 11B: revert the span-integrity check inside `stripMarkdownSection`, its doc-comment addition,
  and the two new `section_stripping` table cases together — the table cases exist specifically
  to exercise the check; reverting the check alone would leave both new cases failing (one
  would want an error that no longer occurs), which is a broken revert, not a clean one. No
  cross-dependency on 11A or on 10B's exact-text terminator (which stays unchanged).
- 12A: revert the `skills/_shared/actuals-record.schema.json` `required`-list edit and the
  `skills/inception-pipeline/SKILL.md` Validate-sentence edit together — reverting only the
  schema re-introduces the R-019/schema mismatch this batch fixes; reverting only the sentence
  leaves the shipped prose claiming a two-reason rule the schema no longer enforces. No
  cross-dependency on any other batch.
- 12B: revert `R019_required_drops_compute_time_fields`, `validateAgainstActualsSchema`,
  `r019ConformingRecordFixture`, and the `validate_against_actuals_schema` table subtest
  together — the subtest and table test both call the two new helpers; reverting the helpers
  alone is a compile failure, not a partial revert. Reverting 12B without reverting 12A
  reintroduces an unguarded schema/rule mismatch with no test to catch it, which is exactly the
  gap this batch closes — the two work units are not independently revertible in the direction
  that matters (12A without 12B is safe; 12B without 12A cannot exist, since the binding test
  would fail against the un-reconciled schema). The one-line `R021_required_drops_wall_clock`
  pin-wording update (12B.4) is coupled to 12A.2, not to 12B.1/12B.2/12B.3 — revert it together
  with 12A if 12A is reverted.

## Mutation Probe Results (Batch 4 — full pin-set sweep)

**Total pin count across both test files**: 69 string pins (REQUIRE + FORBID) existed before
this batch's 5 repairs/additions (W1–W5), for **74 string pins** after. Plus 2 structural checks
in `R021_required_drops_wall_clock`, 2 numeric checks in `R014_amendedR007_fixture_pins`, and the
5-item property/flag structural invariant in `D2_D5_closed_schema_invariant` (already covered by
the dedicated `TestActualsInstrumentationGateHelpers` unit tests, not re-probed here as
duplicate effort). The 4 behavioral (non-string-pin) subtests in `TestD2AnchorIsVerifiableAndUnambiguous`
(`recorded_anchor_with_matching_tree_resolves`, `mismatched_tree_is_rejected_not_trusted`,
`tree_carried_by_more_than_one_commit_is_ambiguous_never_resolved_by_position`,
`no_recorded_anchor_yields_no_t1_and_attempts_no_folder_scan`) were exhaustively mutation-probed
in batch 3 (reproduced independently by the round-3 verifier as E1, E2, P8, P9) and are unchanged
this batch — not re-probed, to avoid duplicating already-reproduced evidence.

**Probed this batch: 81 of 81 applicable string/structural probes** (74 string pins via an
automated harness + 4 manual structural probes [`R021`, `R014` × 2 numeric fields, the sweep
guard's inverse check] + 3 already covered by 9D.1's individual repair-verification, not
double-counted). **0 defects found beyond the 5 (W1–W5) already known and repaired before
probing.** Full automated-probe output (77 of the 81):

| # | Pin group | Mode | Mutated → | Restored → | Result |
|---|---|---|---|---|---|
| 1-6 | R004_R005 (3 require, 3 forbid) | both | FAIL (RED) | PASS (GREEN) | OK ×6 |
| 7-8 | R015_R006_R007_R008 | both | FAIL | PASS | OK ×2 |
| 9-16 | R009 (6 require incl. new W4, 2 forbid incl. new W4) | both | FAIL | PASS | OK ×8 |
| 17-20 | R010_R011 (4 require) | require | FAIL | PASS | OK ×4 |
| 21-23 | R006_R007_R015 (2 require, 1 forbid) | both | FAIL | PASS | OK ×3 |
| 24-43 | R016_R017_R018 (15 require incl. W1/W3 fixes, 5 forbid) | both | FAIL | PASS | OK ×20 |
| 44-49 | R019 (6 require) | require | FAIL | PASS | OK ×6 |
| 50-57 | R014 fixture (7 require incl. W5 fix, 1 forbid) | both | FAIL | PASS | OK ×8 |
| 58-62 | R001_R002 (5 require, scoped) | require | FAIL | PASS | OK ×5 |
| 63 | R012 (1 require, scoped) | require | FAIL | PASS | OK ×1 |
| 64-68 | R013 (5 forbid, invariant guard) | forbid | FAIL | PASS | OK ×5 |
| 69-73 | D2 `shipped_skill_prose…` (3 require incl. W2 area, 2 forbid) | both | FAIL | PASS | OK ×5 |
| 74-77 | `TestAbolishedVocabularySweptFromCurrentSections` (4 forbid probes: design.md ×3, spec.md ×1) | forbid | FAIL | PASS | OK ×4 |

Plus 4 manual structural/inverse probes:

| # | Probe | Mutation | Result |
|---|---|---|---|
| 78 | `R021_required_drops_wall_clock` | Re-added `total_wall_clock_hours` to schema `required` | RED (`schema required list must not contain...`) → restored GREEN, byte-identical |
| 79 | `R014_amendedR007_fixture_pins` (checkpoint_count) | Fixture `checkpoint_count: 12` → `11` | RED (`want 12`) → restored GREEN, byte-identical |
| 80 | `R014_amendedR007_fixture_pins` (wall clock) | Fixture `total_wall_clock_hours: 36` → `1.4` | RED (`want != 1.4 and >= 24`) → restored GREEN, byte-identical |
| 81 | `TestAbolishedVocabularySweptFromCurrentSections` (inverse) | Deleted `receipt-bound`/`path-scan` from design.md's SUPERSEDED block | RED on `design_md_superseded_block_still_carries_the_provenance`, `design_md_current_sections` independently stayed GREEN throughout → restored GREEN, byte-identical |

**Occurrence-count cross-check** (the rule from Engram obs #2884: a pin is non-covering whenever
its exact text occurs more than once in its target file) found no NEW vacuous pins beyond what
round 3 already catalogued as W6: `landing_commit` (6 sites), `approved_tree` (4), the three
compute-time field names (3 each), the calibration formula and two other R009/R010_R011 phrases
(2 each) — these stay covering only because single-site sibling pins in the same REQUIRE list
fail first, exactly as round 3 characterized them. Not fixed this batch (out of the named W1-W5
scope; W6 is a WARNING, not a blocker). Three additional multi-occurrence phrases were found in
the fixture's `variance_vs_plan` narrative (`durably observed` ×3, `reconstructed from
narrative` ×5, `AMB-001` ×2) and individually inspected: each occurrence is a distinct,
load-bearing checkpoint-itemization bullet contributing to the "= 12" total, not one operative
clause shadowed by a decorative duplicate — genuinely non-vacuous, not a new W6-class finding.

## Mutation Probe Results (Batch 5 — W1/W2/W3, round-4 evidence-completeness gaps)

Every probe below was run against the REAL shipped file (`skills/inception-pipeline/SKILL.md` or
`openspec/changes/sdd-cycle-timestamp-instrumentation/design.md`), never a synthetic fixture, and
restored byte-identically (`diff` against a pre-probe backup empty) before the next probe began.

| # | Probe | Mutation | Result | Restored + re-run |
|---|---|---|---|---|
| W1-a | `R021_required_drops_wall_clock` (new pin) | Deleted `` `total_wall_clock_hours` is the one field allowed to be validly absent (R-021) — every other resolved field is still required `` from `SKILL.md`'s Validate step | RED — `closure-feedback Validate step must contain "...is the one field allowed to be validly absent (R-021)"` | GREEN, byte-identical |
| W1-b | same pin, inverted | Replaced the clause with `Every field is required; if any field cannot be resolved, discard the whole record.` — the exact all-or-nothing rule R-021 exists to abolish | RED — same message (the FIRST of the two pinned substrings is what fails first; the inverted text no longer contains it) | GREEN, byte-identical |
| W2-a | `design_md_current_sections` (via changed `stripMarkdownSection`) | Demoted `### D3 — Where anchors are written` → `#### D3 — Where anchors are written` **and** injected `path-scan is injected here as a mutation probe.` into D3's newly-swallowed body — reproducing round 4's V5 exactly | RED — `stripping design.md's SUPERSEDED block (...): section heading not found — the terminator heading may have been renamed, deleted, or demoted (verify round 4, W2)` | GREEN, byte-identical |
| W3 | `tasks.md` row 2 test command, run directly (not a Go mutation — a shell-level probe of the artifact itself) | Ran the OLD command exactly as published: `(cd engine && go test -run TestD2AnchorRetroExecution ./skills/)` | `ok … [no tests to run]`, exit 0 — confirmed vacuously green, exactly as round 4 reported | N/A — this is documentation, not code; fixed by replacing the command text, re-verified the NEW command actually runs and passes (5/5 subtests) |

**Occurrence-count re-confirmation for the two new W1 substrings** (instrument 1, obs #2890):
`` `total_wall_clock_hours` is the one field allowed to be validly absent (R-021) `` occurs
**1** time in `skills/inception-pipeline/SKILL.md`; `every other resolved field is still
required` occurs **1** time. Neither is vacuous by the round-3/round-4 rule (pin text occurring
at more than one site in its target file). No pre-existing pin was touched this batch, so round
4's own 54-pin audit result (zero new vacuity) is unaffected.

## Mutation Probe Results (Batch 6 — W-A/W-B, round-5 evidence-completeness gaps)

Every probe below was run against the REAL committed fixture or the REAL `design.md`, never a
synthetic stand-in, and restored byte-identically (`cmp` against a pre-probe backup empty) before
the next probe began. The fixture probes additionally began with a genuine file-absence RED (not
a mutation of existing content), since the fixture and its pin are both new this batch.

| # | Probe | Mutation | Result | Restored + re-run |
|---|---|---|---|---|
| W-B-0 | `R020_skillsValidateOndiskGate_fixture_mirrors_measured_rebased_record` (new subtest, genuine RED-before-GREEN) | Removed the fixture file entirely (it did not yet exist as a committed artifact before this batch's first write) | RED — `open .../corrected-actuals-skills-validate-ondisk-gate.json: no such file or directory` | GREEN, byte-identical to the live-extracted obs #2789 JSON block |
| W-B-1 | numeric pin, inverted | `"total_wall_clock_hours": 6.64` → `6.4` (the retired pre-merge value the scenario re-bases FROM) | RED — `fixture total_wall_clock_hours = 6.4, want 6.64` | GREEN, byte-identical |
| W-B-2 | numeric pin, deleted | Removed the `total_wall_clock_hours` key from the fixture entirely | RED — `fixture total_wall_clock_hours = <nil>, want 6.64` | GREEN, byte-identical |
| W-B-3 | re-basing sentence, deleted | Removed `"measured independently from the tiering go-ahead checkpoint through MERGE (re-based by sdd-cycle-timestamp-instrumentation; was 6.4 under the retired archive-anchored boundary)"` from `variance_vs_plan` | RED — `fixture variance_vs_plan must contain "measured independently...boundary)"` | GREEN, byte-identical |
| W-B-4 | re-basing sentence, inverted | Replaced with the R-020 else-branch prose: `"...was NOT re-based, because one or both anchors did not resolve, so the pre-merge archive-anchored value is kept unchanged and..."` — the exact opposite scenario (R-020's other branch, which scenario 13 already covers for a different record) | RED — same message (the inverted text no longer contains the required substring) | GREEN, byte-identical |
| W-B-5 | t0-anchor sentence, deleted | Removed `"t0 = 2026-08-05 14:12:55 (obs #2772, sdd/skills-validate-ondisk-gate/pipeline-state, typed \`created_at\` field, primary source — not a fallback). "` | RED — `fixture variance_vs_plan must contain "t0 = 2026-08-05 14:12:55...primary source"` | GREEN, byte-identical |
| W-B-6 | t1-anchor sentence, deleted | Replaced `"commit 79995ea...archive report...versioned prose"` with `"REDACTED"` | RED — `fixture variance_vs_plan must contain "commit 79995ea...archive-report"` | GREEN, byte-identical |
| W-A-1 | `section_stripping` new table case, run against the PRE-fix `stripMarkdownSection` | Ran the new "an inserted section before the terminator..." case before implementing the span-integrity check | RED — `stripMarkdownSection() error = <nil>, want section heading not found` (confirms the pre-fix implementation silently swallowed the inserted section, exactly like round 5's P7) | GREEN after the span-integrity check was implemented (same run, no restore needed — this was a synthetic table case, not a shipped-file mutation) |
| W-A-2 | `design_md_current_sections`, real `design.md`, reproducing round 5's P7 exactly | Inserted `### D2c — inserted as a mutation probe` with `path-scan is injected here as a mutation probe (verify round 5, P7 reproduction).` immediately before the real `### D3 — Where anchors are written` terminator | RED — `stripping design.md's SUPERSEDED block (...): section heading not found: encountered heading "### D2c — inserted as a mutation probe" at or above the carve-out's own level before its terminator "### D3 — Where anchors are written" — a section may have been inserted, widening the removed span — the terminator heading may have been renamed, deleted, or demoted (verify round 4, W2)` | GREEN, byte-identical (`cmp` confirms) |

**Occurrence-count re-confirmation for the four new W-B substrings** (instrument 1, obs #2890):
each of the four pinned substrings (the numeric value and the three provenance sentences) occurs
exactly **1** time in the new fixture — verified by direct Python `count()` before pinning. The
fixture is new this batch, so there is no pre-existing pin inventory to compare against; round
5's 67-pin audit is otherwise unaffected (no pre-existing pin was touched).

## Scenario-to-Assertion Mapping (Batch 5)

Instrument 2 from Engram obs #2890 — walk every one of spec.md's 14 scenarios and name the
assertion that would fail if the clause behind it were deleted or inverted. This is the
instrument that actually found W1; occurrence counting over existing pins cannot see a clause
that has no pin at all. All 14 scenarios are read directly from
`openspec/changes/sdd-cycle-timestamp-instrumentation/specs/actuals-instrumentation/spec.md`
this session, not carried from a prior report.

| # | Requirement | Scenario | Assertion that would fail | Mutation-probe result |
|---|---|---|---|---|
| 1 | R-003/4/5 | Divergent calendar time, interruption included | `R004_R005_schema_temporal_boundary_and_interruption` — `total_wall_clock_hours` description must contain `"from the tiering go-ahead checkpoint to merge"` and `"including interruption gaps"` | RED-before/GREEN-after proven in batch 1 (task 1.1); pin unchanged since, still present |
| 2 | R-003/4/5 | Post-merge bookkeeping replies do not count | `R016_R017_R018_closure_feedback_anchor_prose` — `"excluded from this count as post-boundary bookkeeping"` | Mutation-proven in batch 3 (8C.1): delete → RED, restore → GREEN |
| 3 | R-003/4/5 | No schema shape change anywhere in the record | `D2_D5_closed_schema_invariant` (property-set equality, `additionalProperties: false`) + `assertClosedRecordFlag`'s four-way classification, itself unit-tested in `TestActualsInstrumentationGateHelpers/closed_record_flag_classification` (8 cases) | Structural invariant, exhaustively table-tested — not a single delete/restore probe by design |
| 4 | R-016/17/18 | Anchors resolve and are legible in both stores | `recorded_anchor_with_matching_tree_resolves` (real git, matching-tree fixture) + `R016_R017_R018...` pins (`landing_commit`, `approved_tree`, the exact `git show -s --format=%T <landing_commit>` command pin) | Mutation-proven in batches 2–4 (7A.3 real-fixture proof; W3 pin added in 9D.1, delete → RED, restore → GREEN) |
| 5 | R-016/17/18 | An unrelated change touching the folder does not become the anchor | `no_recorded_anchor_yields_no_t1_and_attempts_no_folder_scan` — structural: `resolveD2T1`'s signature accepts no path/slug argument, so a folder scan cannot be invoked by construction | Structural invariant (proof by signature, not by content deletion) — the FORBID list additionally guards the prose (`"carrying that tree MUST be chosen"` etc.), mutation-proven in 8D.1 |
| 6 | R-016/17/18 | A mis-recorded anchor is rejected, not trusted | `mismatched_tree_is_rejected_not_trusted` (real tree of an unrelated commit) + `"the anchor is REJECTED, t1 is omitted, and the mismatch is disclosed in \`variance_vs_plan\`"` pin | Mutation-proven in batch 4 (9D.1, W1 there — repinned from the bare word `REJECTED`): delete → RED, restore → GREEN |
| 7 | R-016/17/18 | A change predating the convention omits rather than guesses | `no_recorded_anchor_yields_no_t1_and_attempts_no_folder_scan` (same structural subtest as #5) + the omit-and-disclose prose pin (`"the anchor is ambiguous"`, `"WHEN no \`landing_commit\` was ever recorded"` text) | Structural + content pin, both present; content pin covered by the R016/17/18 REQUIRE-list mutation sweep (round-3/4 audits) |
| 8 | R-016/17/18 | A change that skipped inception-pipeline still measures | `` "t0 falls back to the earliest `created_at` among the change" `` pin | Mutation-proven in batch 3 (8C.3): delete → RED, restore → GREEN |
| 9 | R-016/17/18 | Neither anchor resolves | **Was PARTIAL as of round 4 (W1).** Now `R021_required_drops_wall_clock`'s new pin: `` "`total_wall_clock_hours` is the one field allowed to be validly absent (R-021)" `` + `"every other resolved field is still required"` | **NEW this batch** — mutation-proven above (W1-a delete → RED, W1-b invert → RED, restore → GREEN) |
| 10 | R-021 | A cycle missing t0 still produces a usable record | **Was PARTIAL as of round 4 (W1).** Same new pin as #9 — both scenarios describe the same shipped write-anyway clause, so one pin closes both | Same probes as #9 |
| 11 | R-021 | Loosening required does not loosen the shape | `R021_required_drops_wall_clock`'s existing structural checks (`Required` list excludes `total_wall_clock_hours`; `Properties` still declares it) + `D2_D5_closed_schema_invariant` | Mutation-proven in batch 4 (9D.2, probe #78): re-added `total_wall_clock_hours` to `required` → RED, restore → GREEN |
| 12 | R-019 | Deferral is stated, not silent | `R019_compute_time_deferral_rationale` (6 pins: field names + `"session-transient"`, `"no timestamp fields"`, `"no structured subagent durations"`) | Mutation-proven in batch 2 (7C.1): RED before the clause was added, GREEN after |
| 13 | R-020 | Reconstructed record stays annotated, not re-based | `R014_amendedR007_fixture_pins` — `` "was NOT re-based to the merge-anchored boundary" `` | Mutation-proven in batch 4 (9D.1, W5 repair): delete → RED, restore → GREEN |
| 14 | R-020 | Measured record re-based when both anchors resolve, else annotated | **No automated regression guard** — proven by executing the design's own designated integration method at runtime against live Engram (obs #2789 re-based to 6.64 with archive-report provenance; obs #2096's embedded record proven equal to the committed fixture) | **Deliberately left uncovered by an automated test this batch** — this is live Engram data, not shipped repository content; carried as W4 from round 4, explicitly out of this batch's named scope (W1/W2/W3 only). A future automated guard would need to read Engram at test time, which the suite currently does not do for any assertion |

**Scenario coverage after batch 5: 14/14 have a named assertion or an explicitly-disclosed
reason one cannot exist yet (#14).** Round 4 scored 12/14 (scenarios 9 and 10 PARTIAL on W1);
both are now backed by a pin that is mutation-proven in both directions. No scenario is silently
uncovered: #3 and #5 are structural-by-construction rather than delete/restore probes, and #14 is
named explicitly as carried, not silently dropped.

**Round 5 scored 13/14** (scenario 14 PARTIAL — the batch-5 exemption reason for #14, restated in
this table's row 14 above, was itself the round-5 finding W-B: it is falsified by scenario 13's
own construction, which is also live Engram data and IS guarded through a committed fixture
mirror). Row 14 is superseded below.

## Scenario-to-Assertion Mapping (Batch 6) — row 14 superseded, rows 1–13 unchanged

Only row 14 changes from the Batch 5 table above; rows 1–13 are reproduced here unmodified for a
single authoritative 14-row table, not because anything about them changed this batch.

| # | Requirement | Scenario | Assertion that would fail | Mutation-probe result |
|---|---|---|---|---|
| 1 | R-003/4/5 | Divergent calendar time, interruption included | `R004_R005_schema_temporal_boundary_and_interruption` | RED-before/GREEN-after proven in batch 1; unchanged since |
| 2 | R-003/4/5 | Post-merge bookkeeping replies do not count | `R016_R017_R018_closure_feedback_anchor_prose` — `"excluded from this count as post-boundary bookkeeping"` | Mutation-proven in batch 3 (8C.1) |
| 3 | R-003/4/5 | No schema shape change anywhere in the record | `D2_D5_closed_schema_invariant` + `assertClosedRecordFlag`'s 8-case table | Structural, exhaustively table-tested |
| 4 | R-016/17/18 | Anchors resolve and are legible in both stores | `recorded_anchor_with_matching_tree_resolves` + `R016_R017_R018...` pins | Mutation-proven in batches 2–4 |
| 5 | R-016/17/18 | An unrelated change touching the folder does not become the anchor | `no_recorded_anchor_yields_no_t1_and_attempts_no_folder_scan` (structural: no path/slug argument) | Structural + FORBID-list guard, mutation-proven in 8D.1 |
| 6 | R-016/17/18 | A mis-recorded anchor is rejected, not trusted | `mismatched_tree_is_rejected_not_trusted` + REJECTED pin | Mutation-proven in batch 4 (9D.1) |
| 7 | R-016/17/18 | A change predating the convention omits rather than guesses | `no_recorded_anchor_yields_no_t1_and_attempts_no_folder_scan` + omit/disclose prose pin | Structural + content pin, both present |
| 8 | R-016/17/18 | A change that skipped inception-pipeline still measures | `` "t0 falls back to the earliest `created_at` among the change" `` pin | Mutation-proven in batch 3 (8C.3) |
| 9 | R-016/17/18 | Neither anchor resolves | `R021_required_drops_wall_clock`'s W1 pin | Mutation-proven in batch 5 (W1-a/W1-b) |
| 10 | R-021 | A cycle missing t0 still produces a usable record | Same pin as #9 | Same probes as #9 |
| 11 | R-021 | Loosening required does not loosen the shape | `R021_required_drops_wall_clock`'s structural checks | Mutation-proven in batch 4 (9D.2, probe #78) |
| 12 | R-019 | Deferral is stated, not silent | `R019_compute_time_deferral_rationale` | Mutation-proven in batch 2 (7C.1) |
| 13 | R-020 | Reconstructed record stays annotated, not re-based | `R014_amendedR007_fixture_pins` — `` "was NOT re-based to the merge-anchored boundary" `` | Mutation-proven in batch 4 (9D.1, W5) |
| 14 | R-020 | Measured record re-based when both anchors resolve, else annotated | **Was PARTIAL as of round 5 (W-B).** Now `R020_skillsValidateOndiskGate_fixture_mirrors_measured_rebased_record` — a committed fixture mirroring Engram obs #2789, pinning `total_wall_clock_hours == 6.64` and the re-basing/anchor-provenance sentences | **NEW this batch** — mutation-proven above (W-B-0 file-absence RED, W-B-1 numeric invert RED, W-B-2 numeric delete RED, W-B-3 re-basing-sentence delete RED, W-B-4 re-basing-sentence invert RED, W-B-5 t0-anchor delete RED, W-B-6 t1-anchor delete RED; all restored byte-identical and re-confirmed GREEN) |

**Scenario coverage after batch 6: 14/14 have a real, mutation-proven assertion — no scenario is
covered only by a disclosed exemption anymore.** Scenario 14 moves from "no automated regression
guard" (batch 5) to a committed fixture with 4 pins, 7 mutation probes (including the genuine
file-absence RED), all restored byte-identical. #3 and #5 remain structural-by-construction
rather than delete/restore probes — that classification is unchanged by this batch and is not a
gap, since a structural proof (by construction, e.g. a function signature) is a stronger guarantee
than a string pin, not a weaker one.

## Full Suite Evidence (this session, post-Phase-7)

```
$ (cd engine && go build ./... && go vet ./... && go test -count=1 ./...)
ok  	.../engine/assets       0.002s
ok  	.../engine/cmd          0.035s
ok  	.../engine/gadu         0.005s
ok  	.../engine/gate         0.003s
ok  	.../engine/installer    6.004s
ok  	.../engine/prespec      0.003s
ok  	.../engine/propagator   0.002s
ok  	.../engine/runtime      0.148s
ok  	.../engine/settings     0.010s
ok  	.../engine/skills       0.042s

$ (cd tui && go build ./... && go vet ./... && go test -count=1 ./...)
ok  	.../tui  0.076s

$ for m in tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do
    (cd "$m" && go build ./... && go vet ./... && go test -count=1 ./...)
  done
ok  	.../tools/deterministic-check-runner   11.618s
ok  	.../tools/entry-contract-validator     0.377s
ok  	.../tools/review-preflight             0.230s
```

`gofmt -l` on both changed/created Go files: clean (no output).
`TestZeroFetchImportAllowlist` (ADR-15): PASS — confirms `os/exec`/`path`/`time` remain
confined to `_test.go` files in `engine/skills/`.

## Full Suite Evidence (this session, post-Phase-8)

```
$ (cd engine && go build ./... && go vet ./... && go test -count=1 ./...)
ok  	.../engine/assets       0.002s
ok  	.../engine/cmd          0.028s
ok  	.../engine/gadu         0.012s
ok  	.../engine/gate         0.003s
ok  	.../engine/installer    5.998s
ok  	.../engine/prespec      0.003s
ok  	.../engine/propagator   0.002s
ok  	.../engine/runtime      0.152s
ok  	.../engine/settings     0.015s
ok  	.../engine/skills       0.048s

$ (cd tui && go build ./... && go vet ./... && go test -count=1 ./...)
ok  	.../tui  0.076s

$ for m in tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do
    (cd "$m" && go build ./... && go vet ./... && go test -count=1 ./...)
  done
ok  	.../tools/deterministic-check-runner   10.745s
ok  	.../tools/entry-contract-validator     0.359s
ok  	.../tools/review-preflight             0.228s
```

`gofmt -l` on both touched Go files (`actuals_instrumentation_contract_test.go`,
`actuals_instrumentation_d2_anchor_test.go`): clean (no output).
`TestD2AnchorIsVerifiableAndUnambiguous`: 5/5 subtests PASS (was 4/4 before this batch — the
new `shipped_skill_prose_matches_the_proven_ambiguity_rule` subtest adds the SKILL.md-reading
structural binding work unit 4 requires).
`TestActualsInstrumentationContract`: 13/13 subtests PASS, including the repaired
`R016_R017_R018_closure_feedback_anchor_prose`.
`skills/inception-pipeline/SKILL.md` confirmed byte-identical to its pre-session state after
every mutation-probe restore (`diff` empty each time) — it was never a deliverable edit this
batch.

## Full Suite Evidence (this session, post-Phase-9 / Batch 4)

```
$ for m in engine tui tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do
    (cd "$m" && go build -o <scratch-dir>/bin/ ./... && go vet ./... && go test -count=1 ./...)
  done
ok  	.../engine/assets       0.002s
ok  	.../engine/cmd          0.033s
ok  	.../engine/gadu         0.014s
ok  	.../engine/gate         0.003s
ok  	.../engine/installer    6.310s
ok  	.../engine/prespec      0.003s
ok  	.../engine/propagator   0.002s
ok  	.../engine/runtime      0.162s
ok  	.../engine/settings     0.014s
ok  	.../engine/skills       0.049s
ok  	.../tui                 0.079s
ok  	.../tools/deterministic-check-runner   11.716s
ok  	.../tools/entry-contract-validator     0.367s
ok  	.../tools/review-preflight             0.228s
```

`gofmt -l` on both touched Go files (`actuals_instrumentation_contract_test.go`,
`actuals_instrumentation_d2_anchor_test.go`): clean (no output).
`TestActualsInstrumentationContract`: 13/13 subtests PASS (unchanged count — 5 pins repaired
in-place, no new t.Run group added to this function).
`TestD2AnchorIsVerifiableAndUnambiguous`: 5/5 subtests PASS (unchanged from batch 3 — only the
doc comment changed, no test logic).
`TestAbolishedVocabularySweptFromCurrentSections`: 3/3 subtests PASS (new this batch).
`git status --untracked-files=all` verified clean of build artifacts before finishing — the
build was redirected to a scratch directory (`go build -o <scratch-dir>/bin/ ./...`), matching
the round-3 report's explicit instruction after batch 3 left three stray binaries in the working
tree (W11). The only untracked paths remaining are this change's own OpenSpec artifacts
(`design.md`, `tasks.md`, `proposal.md`, `spec.md`, `exploration.md`, `apply-progress.md`,
`verify-report.md`) and the new test file `actuals_instrumentation_d2_anchor_test.go` — all
expected, none are build output.

## Full Suite Evidence (this session, post-Phase-10 / Batch 5)

```
$ for m in engine tui tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do
    (cd "$m" && go build -o <scratch-dir>/bin/ ./... && go vet ./... && go test -count=1 ./...)
  done
ok  	.../engine/assets       0.002s
ok  	.../engine/cmd          0.026s
ok  	.../engine/gadu         0.007s
ok  	.../engine/gate         0.004s
ok  	.../engine/installer    3.611s
ok  	.../engine/prespec      0.003s
ok  	.../engine/propagator   0.002s
ok  	.../engine/runtime      0.207s
ok  	.../engine/settings     0.010s
ok  	.../engine/skills       0.047s
ok  	.../tui                 0.066s
ok  	.../tools/deterministic-check-runner   10.986s
ok  	.../tools/entry-contract-validator     0.363s
ok  	.../tools/review-preflight             0.226s
```

`gofmt -l` on the touched Go file (`actuals_instrumentation_contract_test.go` — the only Go file
edited this batch): clean (no output).
`TestActualsInstrumentationContract`: 13/13 subtests PASS (unchanged count — the new W1 pin was
added inside the existing `R021_required_drops_wall_clock` subtest, no new `t.Run` group).
`TestD2AnchorIsVerifiableAndUnambiguous`: 5/5 subtests PASS (unchanged — not touched this batch).
`TestAbolishedVocabularySweptFromCurrentSections`: 3/3 subtests PASS (unchanged — the caller of
`stripMarkdownSection` was updated to the new 3-argument signature, behavior unchanged for the
current, un-mutated `design.md`).
`TestActualsInstrumentationGateHelpers`: **7/7 top-level groups PASS (was 6 — new `section_stripping`
group, 7 cases, all PASS)**.
Real mutation probes against the shipped files this batch (W1-a, W1-b, W2-a): all three RED as
expected, all three restored byte-identical (`diff` empty) and re-confirmed GREEN — see "Mutation
Probe Results (Batch 5)" above for the full transcript.
`git status --untracked-files=all` verified clean of build artifacts at the end of this batch —
identical untracked-path set to post-Phase-9, plus no new stray files. `skills/inception-pipeline/SKILL.md`
and `design.md` both confirmed byte-identical to their pre-batch-5 state (`diff` empty) — neither
is a deliverable edit this batch, both were only mutated transiently for mutation-probing.

## Full Suite Evidence (this session, post-Phase-11 / Batch 6)

```
$ gofmt -l engine/skills/actuals_instrumentation_contract_test.go
(no output — clean)

$ for m in engine tui tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do
    (cd "$m" && go build -o <scratch-dir>/bin/ ./... && go vet ./... && go test -count=1 ./...)
  done
ok  	.../engine/assets       0.002s
ok  	.../engine/cmd          0.029s
ok  	.../engine/gadu         0.005s
ok  	.../engine/gate         0.003s
ok  	.../engine/installer    6.062s
ok  	.../engine/prespec      0.004s
ok  	.../engine/propagator   0.002s
ok  	.../engine/runtime      0.150s
ok  	.../engine/settings     0.007s
ok  	.../engine/skills       0.042s
ok  	.../tui                 0.075s
ok  	.../tools/deterministic-check-runner   10.431s
ok  	.../tools/entry-contract-validator     0.368s
ok  	.../tools/review-preflight             0.227s
```

`TestActualsInstrumentationContract`: 14/14 subtests PASS (was 13 in round 5's report — the new
`R020_skillsValidateOndiskGate_fixture_mirrors_measured_rebased_record` subtest is the only
addition; verified by direct `go test -v ./skills/... | grep -c "^    --- PASS"` = 14, not
assumed from the report's count).
`TestActualsInstrumentationGateHelpers`: `section_stripping` group now has **9 cases (was 7)** —
the two new batch-6 cases, both PASS.
`TestAbolishedVocabularySweptFromCurrentSections`: 3/3 subtests PASS, including
`design_md_current_sections` — now exercising the new span-integrity check against the real,
un-mutated `design.md` on every run, not only during the mutation probe.
`TestD2AnchorIsVerifiableAndUnambiguous`: 5/5 subtests PASS (unchanged — not touched this batch).
Real mutation probes against shipped/committed content this batch (W-B-0 through W-B-6, W-A-1,
W-A-2): all RED as expected, all restored byte-identical (`cmp` empty) and re-confirmed GREEN —
see "Mutation Probe Results (Batch 6)" above for the full transcript.
`git status --untracked-files=all` verified clean of build artifacts at the end of this batch —
same untracked-path set as post-Phase-10, plus the one new fixture file
(`engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json`), which is a
committed deliverable, not a build artifact. `design.md` confirmed byte-identical to its
pre-batch-6 state (`cmp` empty) — not a deliverable edit this batch, mutated only transiently for
the P7 reproduction probe.

### Native runtime attempt authority (Batch 6)

Acquired via `gentle-ai sdd-attempt acquire` with `request-id
phase11-scenario14-2026-08-14-actor`, `--max-attempts 2 --max-changed-lines 400` — state
`proceed`. Settled via `gentle-ai sdd-attempt settle` with `request-id
phase11-scenario14-2026-08-14-settle` per obs #2887's rule: `--outcome` answers whether THIS work
unit met ITS evidence goal (a fixture-mirror-guarded scenario 14 plus a span-bounded carve-out),
not whether a downstream reviewer will approve — both were delivered and independently
mutation-proven, so this settles `passed`.

## Rule ↔ Schema Cross-Check (Batch 7, work unit 3)

Every rule in `spec.md` and every property description in `actuals-record.schema.json` that
states or implies a field MAY be absent/omitted/optional/deferred, cross-checked against the
schema's `required` list — and the converse (every field currently in `required`, checked against
every shipped rule for a claim that it can be missing). Read directly from both files this
session, not carried from a prior report.

| Field | Rule says | Schema said (pre-batch-7) | Schema says (post-batch-7) | Agree? |
|---|---|---|---|---|
| `change_name` | No rule states it can be missing | required | required | ✅ |
| `project` | No rule states it can be missing | required | required | ✅ |
| `implementation_hours` | R-019 (`spec.md:100`): "MUST stay unpopulated until a durable source exists" | **required (defect)** | not required | ✅ (fixed this batch) |
| `review_gate_hours` | R-019: same | **required (defect)** | not required | ✅ (fixed this batch) |
| `post_review_fix_hours` | R-019: same | **required (defect)** | not required | ✅ (fixed this batch) |
| `total_wall_clock_hours` | R-021 (`spec.md:84`)/R-016 (`spec.md:40,42`): omitted when its anchors do not resolve | not required (fixed under R-021, batch 1) | not required | ✅ |
| `approval_decision` | No rule states it can be missing | required | required | ✅ |
| `scope_drift_notes` | No rule states it can be missing | required | required | ✅ |
| `variance_vs_plan` | No rule states it can be missing | required | required | ✅ |
| `requirement_count` | Description: "Optional. ... Omit if the spec could not be read." | not required | not required | ✅ |
| `changed_lines` | Description: "Optional. ... Omit if not available." | not required | not required | ✅ |
| `review_lens_count` | Description: "Optional. ... Omit if not available." | not required | not required | ✅ |
| `checkpoint_count` | Description: "Optional. ... Omit if not available." | not required | not required | ✅ |

**Converse check (every `required` field, checked for a rule allowing its absence)**: `change_name`,
`project`, `approval_decision`, `scope_drift_notes`, `variance_vs_plan` are the five fields in
`required` after this batch — no rule anywhere in `spec.md`, `design.md`, or either SKILL.md
states or implies any of the five can be missing. Agree.

**Result: R-019 was the only disagreement.** Every optional-marked property description
(`requirement_count`, `changed_lines`, `review_lens_count`, `checkpoint_count`) already correctly
excludes itself from `required` and states its own omission condition in its own description —
no drift found there. `total_wall_clock_hours` was already correctly reconciled in batch 1 (R-021).
Swept `design.md`, `proposal.md`, and `tasks.md` for stale claims about the required-list content
(`rg` for the three field names near "required"): the only pre-existing discussion of the
`required` list in those three files concerns `total_wall_clock_hours`/R-021 (already resolved in
Phase 1) — no artifact claimed anything about the three compute-time fields' required-ness, so
none needed a text correction beyond the schema and SKILL.md edits already made in 12A.

## Mutation Probe Results (Batch 7 — R-019/schema reconciliation)

Every probe below was run against the REAL shipped `skills/_shared/actuals-record.schema.json`,
never a synthetic copy, and restored byte-identically (`diff` against a pre-probe backup empty)
before the next probe began.

| # | Probe | Mutation | Result | Restored + re-run |
|---|---|---|---|---|
| 12-P1 | `R019_required_drops_compute_time_fields` | Re-inserted `"implementation_hours"` into the schema's `required` list | RED — `schema required list must not contain "implementation_hours" (R-019: no durable source exists yet), got [change_name project implementation_hours approval_decision scope_drift_notes variance_vs_plan]` | GREEN, byte-identical (`diff` empty) |
| 12-P2 | same | Re-inserted `"review_gate_hours"` into `required` (independently, after restoring P1) | RED — `schema required list must not contain "review_gate_hours"...` | GREEN, byte-identical |
| 12-P3 | same | Re-inserted `"post_review_fix_hours"` into `required` (independently, after restoring P2) | RED — `schema required list must not contain "post_review_fix_hours"...` | GREEN, byte-identical |
| 12-P4 | `R021_required_drops_wall_clock` (existing pin) | Edited `skills/inception-pipeline/SKILL.md`'s Validate sentence to the reconciled two-reason wording BEFORE updating the pin | RED (genuine, not simulated) — `closure-feedback Validate step must contain "...is the one field allowed to be validly absent (R-021)"` | GREEN after the pin (12B.4) was updated to match |

**Direct unit coverage of the new pure helper** (not a mutation probe against shipped content —
table-driven coverage of `validateAgainstActualsSchema` itself, in the same style as
`classifyClosedRecordFlag`'s 8-case table): 4 cases, all PASS — a bare-minimum valid record, a
valid record with an optional field present, a record missing a required field (rejected), and a
record carrying an undeclared key (rejected under `additionalProperties: false`).

**Occurrence-count note**: the new pin group's three negative substrings (`"implementation_hours"`,
`"review_gate_hours"`, `"post_review_fix_hours"` checked against `schema.Required`) are structural
membership checks against a decoded Go slice, not string-in-file occurrence pins, so the
occurrence-counting instrument (obs #2890) does not apply to them the way it applies to prose
pins. The two prose substrings this batch touches (`R021_required_drops_wall_clock`'s updated pin)
were individually counted in `skills/inception-pipeline/SKILL.md` before being pinned — 1
occurrence each.

## Scenario-to-Assertion Mapping (Batch 7) — no row added, existing rows strengthened

This batch's fix does not correspond to a new spec.md scenario — R-019 (row 12 in the Batch 6
table) already had a scenario and a pin (`R019_compute_time_deferral_rationale`, the rationale
prose). What batch 7 adds is a SECOND, independent kind of proof for the same requirement: not
that the rationale text exists, but that a record actually conforming to R-019 validates against
the shipped schema. Row 12 is therefore strengthened, not superseded:

| # | Requirement | Scenario | Assertion that would fail | Mutation-probe result |
|---|---|---|---|---|
| 12 | R-019 | Deferral is stated, not silent | `R019_compute_time_deferral_rationale` (rationale prose, unchanged) **PLUS (new, batch 7)** `R019_required_drops_compute_time_fields` — proves an R-019-conforming record (the three fields absent) actually validates against the shipped schema | Rationale pin: mutation-proven in batch 2 (7C.1). New binding proof: mutation-proven in batch 7 (12-P1/P2/P3 above, one field at a time) |

Row 9/10 in the Batch 6 table (`R021_required_drops_wall_clock`'s pin) cites the exact substring
`` "`total_wall_clock_hours` is the one field allowed to be validly absent (R-021)" ``, which this
batch's SKILL.md edit changed. The pin itself was updated to match (12B.4, mutation-proven at
12-P4 above); rows 9/10's underlying claim (neither anchor resolving still produces a written
record) is unaffected — only the exact wording the pin searches for changed, and the new wording
is a superset of the old claim (it now also names R-019's own two additional fields in the same
sentence). No scenario regresses; 14/14 remains 14/14, and R-019 is now covered by two
independent, differently-shaped assertions instead of one.

## Full Suite Evidence (this session, post-Phase-12 / Batch 7)

```
$ gofmt -l engine/skills/actuals_instrumentation_contract_test.go
(no output — clean)

$ for m in engine tui tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do
    (cd "$m" && go test -count=1 ./...)
  done
ok  	.../engine/assets       0.002s
ok  	.../engine/cmd          0.028s
ok  	.../engine/gadu         0.006s
ok  	.../engine/gate         0.004s
ok  	.../engine/installer    6.086s
ok  	.../engine/prespec      0.003s
ok  	.../engine/propagator   0.002s
ok  	.../engine/runtime      0.156s
ok  	.../engine/settings     0.009s
ok  	.../engine/skills       0.051s
ok  	.../tui                 0.070s
ok  	.../tools/deterministic-check-runner   11.334s
ok  	.../tools/entry-contract-validator     0.427s
ok  	.../tools/review-preflight             0.229s

$ for m in engine tui tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do
    (cd "$m" && go build -o <scratch-dir>/bin/ ./...)
  done
exit 0 (empty output; scratch bin dir is outside the candidate)
```

`TestActualsInstrumentationContract`: 15/15 subtests PASS (was 14 post-batch-6 — the new
`R019_required_drops_compute_time_fields` subtest is the only addition).
`TestActualsInstrumentationGateHelpers`: new `validate_against_actuals_schema` group (4 cases),
all PASS — was not present post-batch-6.
`TestAbolishedVocabularySweptFromCurrentSections`: 3/3 subtests PASS (unchanged — not touched this
batch).
`TestD2AnchorIsVerifiableAndUnambiguous`: 5/5 subtests PASS (unchanged — not touched this batch).
Real mutation probes against the shipped schema this batch (12-P1 through 12-P4): all RED as
expected, all restored byte-identical (`diff` empty) and re-confirmed GREEN — see "Mutation Probe
Results (Batch 7)" above for the full transcript.
`git status --untracked-files=all` verified clean of build artifacts at the end of this batch —
same tracked/untracked file set as post-Phase-11 (no new files this batch; only existing tracked
files modified).

### Native runtime attempt authority (Batch 7)

Acquired via `gentle-ai sdd-attempt acquire` with `request-id
phase12-r019-schema-2026-08-14-actor`, `--max-attempts 2 --max-changed-lines 400` — state
`proceed`. This work unit's evidence goal was: "An R-019-conforming record with the three compute
fields absent validates against the schema; a test binds rule to schema so the two cannot drift;
live record values are annotated, never withdrawn." All three were delivered: the schema
reconciliation plus binding test satisfy the first two clauses (12-P1/P2/P3 mutation-proven); the
third clause was honored by explicitly NOT touching obs #2789/#2096 or their fixture mirrors (see
"Explicitly out of scope" in tasks.md Phase 12). Per obs #2887's rule (`--outcome` answers whether
THIS work unit met ITS evidence goal, not whether a downstream reviewer will approve), this
settles `passed` via `gentle-ai sdd-attempt settle` with `request-id
phase12-r019-schema-2026-08-14-settle`.

## Batch 8 (Phase 13, this session) — Full Record

Native runtime attempt: acquired via `gentle-ai sdd-attempt acquire` with `request-id
phase13-approved-tree-2026-08-14-actor`, `--max-attempts 2 --max-changed-lines 400` — state
`proceed`, token `sha256:e966c6...560dc38`. Evidence goal: "A no-review anchor can never be
reported as verified; the outcome vocabulary has three states verified/self-asserted/rejected;
bound by a mutation-proven test; class swept for other structurally-unfailable checks."

### The defect (verify round 6, WARNING W2, carried across six batches by deliberate scoping)

`skills/inception-pipeline/SKILL.md:147` defined `approved_tree` as the native review receipt's
`final_candidate_tree` when review ran, **otherwise `landing_commit`'s own tree**. The mandated
check `git show -s --format=%T <landing_commit>` MUST equal the recorded `approved_tree` is `X ==
X` by construction in the second branch — it can never fail, precisely where there is no
independent authority to back it. Round 6 declined to fix this itself and required a scoped human
decision, because closing it changes what "verified" means in a live, already-used convention.

**The decision (made by the human, implemented here, not re-opened)**: remove the false check
rather than invent a new one. A tautological check reported as "verified" is a fabricated
assurance — the same defect class this entire change exists to eliminate from the numbers it
measures. Concretely: (1) `approved_tree` is recorded only when a review receipt exists; (2) the
outcome vocabulary gains a third state distinguishing self-asserted (used, never independently
checked) from verified (independently checked); (3) t1 STILL resolves in the self-asserted case —
this is a labelling fix, not a new omit path.

### TDD Cycle Evidence (Batch 8)

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 13A.1/13A.2 | N/A (prose) | N/A | ✅ full suite green pre-change | N/A — a rule-text rewrite is not itself test-driven; it is what 13B's mutation probes drive | ✅ Full suite green after; propagation confirmed by direct `rg` re-sweep of every artifact (see table below) | ✅ Independently re-derived obs #2789's own archive-report (`Lineage: review-578b97aed1cb4065`, `Gate result: allow`, `APPROVED`) before concluding the existing "verified" label on that record needed no change | ➖ None needed |
| 13B.1 | `engine/skills/actuals_instrumentation_contract_test.go` (`R016_R017_R018...`) | Unit (content-assertion) | ✅ full suite green pre-change | ✅ Mutation-probed against the real `SKILL.md`: reintroduced the exact abolished phrase (`otherwise \`landing_commit\`'s own tree`) → RED (`must contain "\`approved_tree\` MUST NOT be recorded at all"`) | ✅ Restored, byte-identical (`diff` empty) → GREEN | ➖ 3 independent new pins (the operative no-review rule, the self-asserted clause, the 4-way vocabulary) plus 1 new FORBID entry | ➖ None needed |
| 13B.2 | `engine/skills/actuals_instrumentation_contract_test.go` (`TestAbolishedVocabularySweptFromCurrentSections`) | Unit (structural sweep guard) | ✅ full suite green pre-change | ✅ Mutation-probed against the real `design.md`: reintroduced the abolished phrase into the CURRENT (non-SUPERSEDED) "D2 revision 3" section → RED (`design.md's CURRENT (non-SUPERSEDED) sections must not contain "otherwise..."`). **Caught a real tooling gotcha first**: the initial probe run showed a stale cached PASS (`ok ... (cached)`) because `go test` without `-count=1` reused a prior result; re-ran with `-count=1` and got the genuine RED | ✅ Restored, byte-identical (`diff` empty), re-confirmed GREEN with `-count=1` | ➖ Required rewording `design.md`'s own new "Correction (Phase 13)" paragraph to describe the abolished rule WITHOUT quoting it verbatim — a literal citation in a CURRENT section would itself trip this same sweep, which is the sweep working as intended | ➖ None needed |
| 13B.3 | `engine/skills/actuals_instrumentation_d2_anchor_test.go` | Integration (real repo git history) | ✅ full suite green pre-change | ✅ Genuine RED: temporarily replaced `resolveD2T1`'s self-asserted branch with the exact abolished behavior (`anchor.approvedTree = actualTree` before the pre-existing comparison, i.e. synthesizing the tree from itself) — the new subtest failed with `outcome = "verified", want "self-asserted"` | ✅ Restored, byte-identical (`diff` empty, confirmed via `diff` against a pre-edit backup) → GREEN, all 6 subtests PASS | ✅ The three pre-existing subtests (verified / rejected / absent) were updated to the new `d2AnchorOutcome` enum and re-confirmed independently GREEN, proving the enum change didn't silently alter their meaning | ➖ Signature change: `resolveD2T1`'s binary `rejected bool` return became a 4-value `d2AnchorOutcome` enum (`absent`/`self-asserted`/`verified`/`rejected`) — all 4 call sites in the same file updated atomically (compile-checked) |
| 13C.1 | N/A (sweep) | N/A | ✅ full suite green throughout | N/A — a sweep for other instances of the same defect class, not a single RED/GREEN cycle | ✅ Full table below; one genuine instance found (the one just fixed), every other candidate inspected and found either genuinely non-tautological (compares two independently-sourced values) or already honestly self-disclosed as GREEN-by-construction (D6 roadmap-maker FORBID guard, pre-existing, round 6's own audit table) | ✅ Cross-checked the two named prior instances from the launch prompt: the `TestD2AnchorRetroExecution` vacuity (fixed Phase 10) re-confirmed still fixed (0 `rg` hits repo-wide, live gate command in `tasks.md`'s Suggested Work Units table cites the current test) | ➖ None needed |

### Files Changed (Batch 8)

| File | Action | What Was Done |
|------|--------|---------------|
| `skills/inception-pipeline/SKILL.md` | Modified | 4 edits: (1) review-receipt bullet clarified — `approved_tree` never recorded when no review ran; (2) the D2 paragraph rewritten — `approved_tree` recorded ONLY when a receipt exists, the "otherwise `landing_commit`'s own tree" synthesis removed outright; (3) resolution-outcome sentence — two states (verified/REJECTED/absent) → four (verified/self-asserted/rejected/absent), "once verified" → "once resolved"; (4) Execute-item-3 table description updated to the new outcome vocabulary. **8 changed lines** (isolated: reconstructed the pre-batch-8 file by reverting these 4 edits and diffing). |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/specs/actuals-instrumentation/spec.md` | Modified | R-016/17/18's t1 body rewritten to the three-outcome rule; new scenario "An anchor with no review receipt is used but recorded as self-asserted, never verified" inserted after the existing "Anchors resolve and are legible in both stores" scenario. **8 changed lines** (same reconstruction method). |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/design.md` | Modified | D2 revision 3's field table annotated (`approved_tree` "recorded ONLY when a review receipt exists"); new "Correction (Phase 13)" paragraph documenting the fix (paraphrased, not quoting the abolished phrase verbatim — see 13B.2); the verification paragraph rewritten for the 3-outcome rule; the Data Flow diagram and Interfaces/Contracts anchor-sentence template both updated for the self-asserted case. **14 changed lines** (same reconstruction method). |
| `engine/skills/actuals_instrumentation_contract_test.go` | Modified | 3 new pins in `R016_R017_R018_closure_feedback_anchor_prose` (the no-review rule, the self-asserted clause, the 4-way vocabulary) + 1 new FORBID entry (the abolished phrase) + 1 new entry in `TestAbolishedVocabularySweptFromCurrentSections`'s `abolished` list + 2 comment updates (outdated "verified, REJECTED, or absent" references corrected to the new vocabulary). **35 changed lines** (same reconstruction method). |
| `engine/skills/actuals_instrumentation_d2_anchor_test.go` | Modified | Doc comment rewritten to describe the 4-outcome rule and cite the fixed defect; `recorded_anchor_with_matching_tree_resolves` renamed to `...resolves_verified` and updated to the enum; `mismatched_tree_is_rejected_not_trusted` updated to the enum; new subtest `recorded_anchor_with_no_review_receipt_is_self_asserted_never_verified` (the binding proof); `no_recorded_anchor_yields_no_t1_and_attempts_no_folder_scan` updated to the enum; `d2RecordedAnchor`'s doc comment updated; new `d2AnchorOutcome` type + 4 named constants; `resolveD2T1` rewritten with the new self-asserted branch and enum returns. **196 changed lines** (isolated by reconstructing the pre-batch-8 file from this session's own initial `Read` of the file, byte-for-byte, and diffing against the current file — this file has no prior git history to diff against since it remains untracked, same as every prior batch that touched it). |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/tasks.md` | Modified | 12E.1 marked `[x]` (verify round 6 has since run and is the source of this batch's defect). New "Phase 13" section (13A–13E) documenting this batch in full. **101 changed lines** (2 deleted, 99 inserted — a section replacement, not reconstructed via revert since the change is a pure append after the unchecked checkbox). |

**Batch-8 total authored diff**: 8 + 8 + 14 + 35 + 196 + 101 = **362 changed lines**, against the
400-line budget acquired for this attempt (`gentle-ai sdd-attempt acquire … --max-changed-lines
400 --request-id phase13-approved-tree-2026-08-14-actor`). Under budget; no chained delivery
needed for this batch alone (the cumulative-candidate question, W3, remains a carried
delivery-planning fact, not a code defect this batch resolves or is asked to). `apply-progress.md`'s
own update is bookkeeping, not authored production/spec risk, consistent with how every prior
batch treated its own self-update.

### Mutation Probe Results (Batch 8)

| # | Probe | Direction | Result | Restored + re-run |
|---|---|---|---|---|
| 13-P1 | `R016_R017_R018_closure_feedback_anchor_prose` new pins, against real `SKILL.md` | Reintroduced the exact abolished phrase `otherwise \`landing_commit\`'s own tree` in place of the fixed no-review sentence | RED — `closure-feedback must contain "\`approved_tree\` MUST NOT be recorded at all"` | GREEN, byte-identical restore (`diff` empty) |
| 13-P2 | `TestAbolishedVocabularySweptFromCurrentSections`, against real `design.md` | Reintroduced the abolished phrase into the CURRENT (non-SUPERSEDED) "D2 revision 3" section | RED — `design.md's CURRENT (non-SUPERSEDED) sections must not contain "otherwise \`landing_commit\`'s own tree"` (confirmed genuine only after re-running with `-count=1`; the first attempt showed a stale cached PASS) | GREEN, byte-identical restore (`diff` empty), re-confirmed with `-count=1` |
| 13-P3 (the core proof, work unit 2) | `TestD2AnchorIsVerifiableAndUnambiguous/recorded_anchor_with_no_review_receipt_is_self_asserted_never_verified`, against `resolveD2T1`'s own Go implementation | Replaced the self-asserted branch with the exact abolished behavior: `anchor.approvedTree = actualTree` (synthesizing the tree from itself) before the pre-existing comparison, then let the existing comparison run | RED — `resolveD2T1() outcome = "verified", want "self-asserted"` | GREEN, byte-identical restore (`diff` empty, confirmed against a pre-edit backup), all 6 subtests of `TestD2AnchorIsVerifiableAndUnambiguous` PASS |

13-P3 is the binding proof the launch prompt's evidence goal names directly: **a no-review anchor
can never be reported as verified.** Before the fix, the abolished behavior (reintroduced
transiently for this probe) makes exactly that claim false; the shipped fix makes it true, and the
test fails loudly if the false claim ever returns.

### Work Unit 3 — Sweep for other structurally-unfailable checks

The general defect class: **a check whose success condition is definitionally true, reported as if
it verified something.** Swept this change's shipped artifacts and its own test suite.

| Check | Can it fail? | Evidence |
|---|---|---|
| `approved_tree` tautology (SKILL.md D2 revision 3) — **the defect this batch fixes** | Before: **NO** — compared `landing_commit`'s tree to a value synthesized from itself when no review ran. After: **YES** — compares the tree to an independently-recorded `approved_tree` only when a receipt exists; the self-asserted branch makes no comparison at all and is labelled accordingly | Mutation-probed 13-P1/13-P2/13-P3 above, this batch |
| `D2_D5_closed_schema_invariant` (property-set + fixture-agreement checks) | YES — compares the real shipped schema and the real committed fixture, two independently-sourced files, against a hardcoded expected property list | Inspected directly (`engine/skills/actuals_instrumentation_contract_test.go:495-536`); already disclosed in its own comment as "NOT part of the RED gate... exists to guard against future regressions", an honest label, not a fabricated "verified" claim |
| `R019_required_drops_compute_time_fields` (the R-019 binding proof, batch 7) | YES — `r019ConformingRecordFixture()` is a hand-authored literal map, independent of the schema; `validateAgainstActualsSchema` reads the real shipped schema from disk. Neither derives from the other | Inspected directly; also already mutation-proven in batch 7 (12-P1/P2/P3) |
| `commitsCarryingTree` vs. `resolveTreeToCommitOrAmbiguous` cross-check | YES — two independently implemented code paths over real git history; the test explicitly distrusts the production-shaped helper until the independent count agrees (design established this way in batch 2 for exactly this reason) | Inspected directly (`engine/skills/actuals_instrumentation_d2_anchor_test.go`) |
| `sliceMarkdownSection`'s not-found-vs-empty-string distinction | N/A — this is a defense AGAINST vacuity (returns an error instead of `""` specifically so a not-found section cannot make every scoped assertion pass vacuously), not an instance of the defect | Inspected directly; own doc comment states the rationale |
| D6 — roadmap-maker FORBID-list invariant (`R013_roadmap_maker_no_compute_time_source_invariant`) | Structurally **GREEN by construction** on the current tree — design D6 forbids ever editing `roadmap-maker/SKILL.md` in this change, so the FORBID check cannot find the forbidden words by definition, not because a real check ran and passed | Pre-existing, **already honestly self-disclosed**: the test's own comment says "GREEN-by-construction... NOT RED coverage", and verify round 6's own Design Coherence table independently reached the same conclusion ("FORBID-list invariant, GREEN by construction... Coherent and honestly labelled"). Not counted as a new instance of THIS defect class, because the defect is claiming "verified" for something that cannot fail — this check never claims that; it discloses its own limit |
| Historical `go test -run TestD2AnchorRetroExecution` gate command (tasks.md, pre-Phase-10) | Previously **NO** — `TestD2AnchorRetroExecution` was deleted in batch 2; running the old command exits 0 with `[no tests to run]`, indistinguishable from a real PASS | Already fixed Phase 10 (10C.1); re-confirmed still fixed this batch: `rg -c TestD2AnchorRetroExecution` = 0 hits repo-wide, and running the literal old command still vacuously exits 0 (reproduced this batch as a control, not a new finding) |

**One genuine new instance found this batch: the `approved_tree` tautology itself.** Every other
candidate inspected either compares two independently-sourced values (genuine) or already
discloses its own GREEN-by-construction limit honestly (D6) rather than claiming verification it
cannot back.

### Independent re-check of live Engram records (explicitly not withdrawn or rewritten)

Per the launch prompt's explicit scope boundary, obs #2789/#2096 and their committed fixture
mirrors were NOT touched. Before concluding nothing needed proposing, both records' existing
outcome claims were independently re-checked against the new three-state rule:

- **obs #2789 (`sdd/skills-validate-ondisk-gate/actuals`)**: its fixture's `variance_vs_plan`
  states t1 was resolved "verified via `git show -s --format=%T 79995ea` = `b654d2d0...` (match)".
  Checked whether this candidate genuinely had a review receipt, not merely a recorded tree:
  `openspec/changes/archive/2026-08-05-skills-validate-ondisk-gate/archive-report.md` records
  `**Lineage:** review-578b97aed1cb4065, generation 1, terminal state: **APPROVED**`, `**Gate
  result:** allow`, and `**Risk level:** HIGH (canonical 4-lens review)`. A review genuinely ran
  and approved this exact candidate — the record's existing "verified" label is CORRECT under the
  new rule, not a self-asserted claim mislabelled as verified. **No annotation is proposed.**
- **obs #2096 (`sdd/sync-check-repo-behind-origin/actuals`)**: its fixture's `variance_vs_plan`
  makes no verified/self-asserted/rejected claim at all — it states `total_wall_clock_hours was NOT
  re-based... it is a hand reconstruction with no measured t0/t1 anchors` (R-020's other branch,
  unrelated to the `approved_tree` mechanism entirely). **No annotation is proposed.**

Both records were correct before this batch and remain correct after it; the new vocabulary simply
did not change how either record should be described.

### Full Suite Evidence (this session, post-Phase-13 / Batch 8)

```
$ gofmt -l engine/skills/
(no output — clean)

$ (cd engine && go vet ./...)
(no output — clean)

$ for m in engine tui tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do
    (cd "$m" && go test -count=1 ./...)
  done
ok  	.../engine/assets       0.002s
ok  	.../engine/cmd          0.032s
ok  	.../engine/gadu         0.014s
ok  	.../engine/gate         0.004s
ok  	.../engine/installer    6.073s
ok  	.../engine/prespec      0.004s
ok  	.../engine/propagator   0.002s
ok  	.../engine/runtime      0.172s
ok  	.../engine/settings     0.013s
ok  	.../engine/skills       0.051s
ok  	.../tui                 0.077s
ok  	.../tools/deterministic-check-runner   10.547s
ok  	.../tools/entry-contract-validator     0.391s
ok  	.../tools/review-preflight             0.226s

$ for m in engine tui tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do
    (cd "$m" && go build -o <scratch-dir>/bin/ ./...)
  done
exit 0 (empty output; scratch bin dir is outside the candidate)
```

`TestActualsInstrumentationContract`: 15/15 subtests PASS (unchanged count — this batch adds pins
to an existing subtest, `R016_R017_R018_closure_feedback_anchor_prose`, and a new abolished-list
entry to an existing subtest, `TestAbolishedVocabularySweptFromCurrentSections`, rather than adding
new top-level subtests).
`TestD2AnchorIsVerifiableAndUnambiguous`: **6/6 subtests PASS** (was 5 post-batch-6 — the new
`recorded_anchor_with_no_review_receipt_is_self_asserted_never_verified` subtest is the addition).
`TestAbolishedVocabularySweptFromCurrentSections`: 3/3 subtests PASS (its `abolished` list grew by
one entry; subtest count unchanged).
Real mutation probes this batch (13-P1/13-P2/13-P3): all RED as expected, all restored
byte-identical (`diff` empty) and re-confirmed GREEN — see "Mutation Probe Results (Batch 8)"
above for the full transcript.
`git status --untracked-files=all` verified clean at the end of this batch — same tracked file set
as post-Phase-12 (`skills/inception-pipeline/SKILL.md`, `engine/skills/actuals_instrumentation_contract_test.go`
modified; `openspec/changes/sdd-cycle-timestamp-instrumentation/` and
`engine/skills/actuals_instrumentation_d2_anchor_test.go` remain untracked, unchanged file identity
from prior batches — no new files, no stray build artifacts).

### Native runtime attempt authority (Batch 8)

Acquired via `gentle-ai sdd-attempt acquire` with `request-id
phase13-approved-tree-2026-08-14-actor`, `--max-attempts 2 --max-changed-lines 400` — state
`proceed`. Evidence goal delivered in full: (1) a no-review anchor can never be reported as
verified — bound by 13-P3's mutation probe against `resolveD2T1` itself; (2) the outcome
vocabulary has the required states (verified/self-asserted/rejected, plus the pre-existing
absent case for no-anchor-recorded) — pinned in prose and enumerated in Go; (3) bound by a
mutation-proven test — 13-P1/13-P2/13-P3, all genuine RED-then-restored-GREEN; (4) the class was
swept for other structurally-unfailable checks — full table above, one genuine instance found and
fixed, every other candidate inspected. Per obs #2887's rule (`--outcome` answers whether THIS work
unit met ITS evidence goal, not whether a downstream reviewer will approve), this settles `passed`
via `gentle-ai sdd-attempt settle` with `request-id phase13-approved-tree-2026-08-14-settle`.

---

## Batch 9 (Phase 14) — reconcile R-021/D5 with the shipped four-name `required` drop, pin `required` positively (round 7 CRITICAL-1)

**Change**: `sdd-cycle-timestamp-instrumentation` | **Store**: hybrid | **Mode**: Strict TDD | **Date**: 2026-08-14 (batch 9 / Phase 14)

Verify round 7 (evidence `sha256:ea5f683d…`) found the third instance of the rule↔artifact
contradiction class this change keeps producing: Phase 12 correctly dropped
`implementation_hours`, `review_gate_hours`, and `post_review_fix_hours` from the schema's
`required` list under R-019, but never re-checked the sibling artifacts that constrain that
schema. Confirmed at the source: the shipped `required` array is
`[change_name, project, approval_decision, scope_drift_notes, variance_vs_plan]` — **four** names
removed from the original nine, under two distinct reasons. Against that reality, `spec.md:90`
(R-021's body, "Every other required field is unchanged") and `spec.md:102` (R-021's acceptance
scenario, "`total_wall_clock_hours` is the only name removed from `required`") were both false;
`design.md:104` (D5) sanctioned only the R-021 drop and carried zero coverage of R-019's three
drops (`rg 'R-019|implementation_hours' design.md` returned 0 hits before this batch). The green
suite never caught it because `schema.Required` was pinned only by NEGATIVE membership checks
(`R021_required_drops_wall_clock`, `R019_required_drops_compute_time_fields`), each of which can
only detect the specific name(s) it names leaving the list — structurally blind to any OTHER name
leaving, which is exactly what let R-021's "only" claim go silently false.

### Work Unit 1 — reconcile the artifact text with the schema

**`spec.md:90`** (R-021 requirement body) — BEFORE: "Every other required field is unchanged, and
the property set itself is untouched — this relaxes which fields are mandatory, not which fields
exist." — AFTER: names the R-019 drop explicitly and scopes the "unchanged" claim to exclude the
four now-named fields: "`implementation_hours`, `review_gate_hours`, and `post_review_fix_hours`
are also absent from `required`, for the separate reason R-019 states; aside from these four
names, every other required field is unchanged...".

**`spec.md:102`** (R-021's "Loosening required does not loosen the shape" scenario, THEN clause) —
BEFORE: "`total_wall_clock_hours` is the only name removed from `required`, no property is added
or removed, and `additionalProperties: false` is unchanged" — AFTER: "`total_wall_clock_hours` is
removed from `required` under this requirement and `implementation_hours`, `review_gate_hours`,
and `post_review_fix_hours` are separately removed under R-019 — no other name changes, no
property is added or removed, and `additionalProperties: false` is unchanged". The scenario's real
invariants — no property added/removed, `additionalProperties: false` unchanged, the property set
untouched — are preserved unweakened; only the false exclusivity COUNT is corrected, restored as a
now-true exclusivity claim (exactly these four, nothing else).

**`design.md` D5** — extended with a second, independent decision paragraph: "Second, independent
drop (orchestrator finding, post-round-6, Phase 12 remediation): `implementation_hours`,
`review_gate_hours`, and `post_review_fix_hours` also removed from `required`, for a different
reason... **Choice**: sanction this second, independent `required` drop alongside the R-021 one
above... Two distinct reasons for two distinct groups of fields, never collapsed into one claim of
exclusivity." Also corrected D5's stale parenthetical — "the contract test does **not** pin the
required list — verified" (true when written, Phase 1) is now false (two negative subtests plus
the new exact-list assertion, Work Unit 2, all pin it) — removed the stale claim and added a
closing sentence naming the three guards that now bind this decision.

**`design.md`'s File Changes table row** for the schema file — BEFORE: "2 description boundary
edits; drop `total_wall_clock_hours` from `required` (D5, flagged)" — AFTER: "2 description
boundary edits; drop `total_wall_clock_hours` (R-021) and, in Phase 12 remediation,
`implementation_hours`/`review_gate_hours`/`post_review_fix_hours` (R-019) from `required` (D5,
flagged)".

**Sweep, not citation-chase (Engram obs #2888)**: `rg -n -i "required list|only name removed|every
other required|unchanged" openspec/changes/sdd-cycle-timestamp-instrumentation/{specs/actuals-instrumentation/spec.md,design.md,proposal.md} skills/inception-pipeline/SKILL.md
skills/_shared/actuals-record.schema.json` was run BEFORE any edit to enumerate every candidate
site, and again AFTER to prove exhaustion. Before: the three false claims round 7 cited
(spec.md:90, spec.md:102, design.md:104), plus the schema's own field-scoped mention on
`total_wall_clock_hours`'s description (makes no exclusivity claim about the whole list — not
false, left unchanged) and `skills/inception-pipeline/SKILL.md:153` (already correct — verify
round 7's own Source E: "`total_wall_clock_hours` is validly absent when its anchors do not
resolve (R-021), and `implementation_hours`, `review_gate_hours`, and `post_review_fix_hours` are
validly absent while no durable source exists for them (R-019)" — mirrored, not copied verbatim,
in the spec.md rewrite above). After: only the corrected text and the schema's own harmless
field-scoped mention remain — zero further false-exclusivity hits. `design.md:164`'s historical
Open Question ("D5 `required` drop exceeds the proposal's schema Unchanged claim") makes no
"only"/exclusivity claim (it is about needing user confirmation, already satisfied in design
revision 2) and needed no edit. `tasks.md`/`apply-progress.md`'s own historical batch records
(Phase 12's "12C.2 Swept design.md, proposal.md, tasks.md for stale claims about the required list
content") are left byte-unchanged per the established precedent (Phase 9's 9A.2): they are honest
records of what a PRIOR batch found true AT THE TIME, not current-state claims.

### Work Unit 2 — the structural fix: pin `schema.Required` positively

Added a positive exact-list assertion inside `D2_D5_closed_schema_invariant`
(`engine/skills/actuals_instrumentation_contract_test.go`), reusing the exact `assertStringList`
helper the same subtest already applies to `schema.Properties` (and that
`oo_quality_contract_artifact_test.go` independently already uses for YAML frontmatter lists — an
established repository idiom, not a new pattern):

```go
wantRequired := []string{
    "change_name", "project", "approval_decision", "scope_drift_notes", "variance_vs_plan",
}
assertStringList(t, schema.Required, wantRequired)
```

**Mutation Probe Results (Batch 9) — every RED and every restore run with `go test -count=1`
(Engram obs #2903: any probe result obtained without it is no result)**:

| # | Probe | Mutation | Result | Restore |
|---|---|---|---|---|
| 14-P1 | `D2_D5_closed_schema_invariant` — remove-expected direction | Removed `"change_name"` from `schema.required` (a name still expected) | **RED** — `frontmatter list length = 4 ([approval_decision project scope_drift_notes variance_vs_plan]), want 5 ([approval_decision change_name project scope_drift_notes variance_vs_plan])` | GREEN; schema sha256 `0684faa3…` identical (matches round 7's own restore hash) |
| 14-P2 | `D2_D5_closed_schema_invariant` + `R021_required_drops_wall_clock` — re-add-dropped direction | Re-inserted `"total_wall_clock_hours"` into `schema.required` | **RED on both** — the pre-existing negative pin (`R021_required_drops_wall_clock`) AND the new exact-list guard both fire, exactly as expected since this name is negatively pinned | GREEN; identical |
| 14-P3 | `D2_D5_closed_schema_invariant` — **the load-bearing, non-vacuity probe** | Added `"requirement_count"` — a brand-new name, never named by any existing negative pin — to `schema.required` | **RED only on the new exact-list guard** — `R021_required_drops_wall_clock` PASSED (its negative pin has nothing to say about `requirement_count`); `D2_D5_closed_schema_invariant` failed: `frontmatter list length = 6 (...requirement_count...), want 5`. **This reproduces round 7's exact CRITICAL-1 mechanism** — a name entering/leaving `required` that no existing negative pin names — and proves the new guard closes it | GREEN; identical |
| 14-P4 | full baseline | No mutation — full `TestActualsInstrumentationContract` suite, all 15 subtests | GREEN throughout (baseline before/after every probe) | n/a |

14-P3 is the probe that matters: it is the one direction none of the six prior negative pins could
ever catch, and it is exactly the shape of the defect round 7 found. Restore was confirmed
byte-identical after every probe (`sha256:0684faa369cf91e968bd3b6cec84e7283aac7efb135f922fd11a7f193a99685d`, identical before and after all four probes), and the full suite was re-run GREEN at `-count=1` after final restore.

### Work Unit 3 — sweep the class: other lists pinned only negatively

Swept this change's two owned test files (`actuals_instrumentation_contract_test.go`,
`actuals_instrumentation_d2_anchor_test.go`) for every list-shaped guard using
`slices.Contains`/`strings.Contains`/"must not contain" as its success condition:

| List | Where pinned | Shape | Can drift silently? | Action |
|---|---|---|---|---|
| `schema.Required` | `R021_required_drops_wall_clock` (neg.), `R019_required_drops_compute_time_fields` (neg. ×3 + positive record-validation), `D2_D5_closed_schema_invariant` | Finite enumerable JSON array, real membership invariant | **YES — proven, round 7 CRITICAL-1** | **Fixed this batch**: positive exact-list assertion (Work Unit 2) |
| `schema.Properties` | `D2_D5_closed_schema_invariant` (`wantProperties`, `assertStringList`) | Finite enumerable JSON array | No — already positively pinned (pre-existing, Phase 1) | None needed; confirmed as the correct pattern this batch reused |
| Temporal-boundary FORBID phrases (`"from first apply to archive"`, `"checkpoint to archive"`, `"tiering-go-ahead-to-archive boundary"`) | `R004_R005_schema_temporal_boundary_and_interruption` | Retired-prose FORBID list | No — guards against RETIRED text reappearing, not membership of a live enumerable set; no positive complement exists | Left negative — inherent to the FORBID shape |
| D2 abolished-vocabulary FORBID lists (`"path-scan"`, `"first-parent"`, `"receipt-bound"`, `"carrying that tree MUST be chosen"`, `"otherwise \`landing_commit\`'s own tree"`, etc.) | `R016_R017_R018_closure_feedback_anchor_prose`, `TestAbolishedVocabularySweptFromCurrentSections`'s `abolished`, `shipped_skill_prose_matches_the_proven_ambiguity_rule` | Retired-prose FORBID lists | No — same reasoning as above | Left negative |
| roadmap-maker no-compute-time-source list (`total_wall_clock_hours`, `checkpoint_count`, `implementation_hours`, `review_gate_hours`, `post_review_fix_hours`) | `R013_roadmap_maker_no_compute_time_source_invariant` | Hand-maintained FORBID list of actuals-only field names | **Partially** — a real but lower-severity risk: a brand-new actuals-only field added to the schema in the future would not automatically extend this list, so a regression sourcing roadmap-maker data from it would go undetected. Distinct from CRITICAL-1's shape (existing content silently LEAVING a pinned list); this is new content silently NOT being added to a guard | **Not converted** — flagged as an adjacent finding, left out of this batch's surgical scope (see 14D) |

No other membership/shape list was found in either file. Broader repository test files
(`oo_quality_contract_artifact_test.go`, `serialize_test.go`, other modules' suites) were grepped
for the same pattern for visibility only — `oo_quality_contract_artifact_test.go` already uses the
positive `assertStringList` idiom for its own frontmatter lists; nothing in those files is owned by
this change, so none were touched (scope discipline, obs #2888's own bounding: "a citation is a
sample, not the extent, but the extent is bounded by what this change ships").

### Work Unit 4 — explicitly out of scope this batch
- Did NOT re-open R-019, the `approved_tree` four-state rule, or any other finding round 7
  confirmed clean.
- Did NOT withdraw or annotate live Engram records #2789/#2096 or their committed fixture mirrors.
  Round 7 independently re-confirmed both source claims correct (`git show -s --format=%T
  79995ea` = `b654d2d0…`, matching obs #2789's receipt; obs #2096 makes no verified-anchor claim) —
  nothing to annotate.
- Did NOT touch cumulative size / chained PRs (a delivery question) or task 6.3 (structurally
  deferred — cannot be exercised before this change's own merge).
- Did NOT convert the roadmap-maker FORBID list to a derived/positive form (14C.1/Work Unit 3
  table) — an adjacent, lower-severity finding, not this batch's CRITICAL.

### Scenario-to-Assertion Mapping — R-021 scenario re-verified

Requirement 12 (R-021, "Loosening required does not loosen the shape"), previously **FAILING** per
round 7 (mapped test did not assert the scenario's own "only name removed" clause), is now
**COMPLIANT**: the scenario's THEN clause states exactly four names removed under two named
reasons, matching the shipped diff, and is bound by `D2_D5_closed_schema_invariant`'s new
`wantRequired` exact-list assertion (14-P1/14-P2/14-P3 above prove it fails on any deviation in
either direction). Round 7's 15-scenario count is unchanged by this batch (no new scenario added
or removed) — this batch closes the ONE failing scenario, bringing the compliance count to 15/15.

### Full suite / build (Batch 9)

```
$ for m in engine tui tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do
    (cd "$m" && go test -count=1 ./...)
  done
ok  	.../engine/assets       0.002s
ok  	.../engine/cmd          0.030s
ok  	.../engine/gadu         0.008s
ok  	.../engine/gate         0.003s
ok  	.../engine/installer    5.995s
ok  	.../engine/prespec      0.003s
ok  	.../engine/propagator   0.002s
ok  	.../engine/runtime      0.165s
ok  	.../engine/settings     0.008s
ok  	.../engine/skills       0.053s
ok  	.../tui                 0.072s
ok  	.../tools/deterministic-check-runner   10.596s
ok  	.../tools/entry-contract-validator     0.423s
ok  	.../tools/review-preflight             0.229s

$ for m in engine tui tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do
    (cd "$m" && go build -o <scratch-dir>/bin/ ./...)
  done
exit 0 (empty output)
```

`TestActualsInstrumentationContract`: 15/15 subtests PASS (unchanged count — this batch extends an
existing subtest, `D2_D5_closed_schema_invariant`, rather than adding a new top-level subtest).
`git status --untracked-files=all` verified clean at the end of this batch — same tracked/untracked
file set as post-Phase-13, no new files, no stray build artifacts.

### Files changed (Batch 9) — well under the 400-line budget

| File | Changed lines (this batch only) |
|---|---|
| `engine/skills/actuals_instrumentation_contract_test.go` | +15 (pure insertion, 0 deletions) |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/design.md` | ~13 (D5 block -3/+8, File Changes row -1/+1) |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/specs/actuals-instrumentation/spec.md` | ~4 (R-021 body -1/+1, scenario THEN -1/+1) |
| `skills/_shared/actuals-record.schema.json` | 0 net (four mutation probes, each restored byte-identical; confirmed via `sha256sum` before/after every probe) |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/tasks.md` | +~70 (new Phase 14 section, appended) |

**Total authored implementation + spec/design edit** (excluding tasks.md/apply-progress.md
bookkeeping): **≈32 lines** — far under the 400-line review budget. Cumulative change size
(≈1144 lines authored, reported in verify round 7) is a pre-existing delivery-planning fact this
batch does not change or re-litigate.

### Native runtime attempt authority (Batch 9)

Acquired via `gentle-ai sdd-attempt acquire` with `request-id
phase14-r021-required-2026-08-14-actor`, `--max-attempts 2 --max-changed-lines 400` — state
`proceed`. Evidence goal delivered in full: (1) R-021's scenario and D5 now state the four
removals with their two reasons, sweep-proven exhaustive; (2) the `required` list is pinned by an
exact list, so any drift goes RED regardless of whether a negative pin names it — proven by 14-P3,
which reproduces round 7's exact mechanism; (3) no other list in this change's owned test files is
pinned only negatively without one being flagged and explained. Per obs #2887's rule (`--outcome`
answers whether THIS work unit met ITS evidence goal), this settles `passed` via
`gentle-ai sdd-attempt settle` with `request-id phase14-r021-required-2026-08-14-settle`,
`--evidence-revision sha256:2c5aa0cde38a325d6a1d188eae96c57f726c0a66db54b773a91adc26bd71a783`
(hash of the full test-suite output above) — state `complete`.

---

## Batch 10 (Phase 15) — close 2 documentation-grade WARNINGs from verify round 8

**Change**: `sdd-cycle-timestamp-instrumentation` | **Store**: hybrid | **Mode**: Strict TDD

Verify round 8 returned **PASS** — 0 CRITICAL, 0 blockers, requirements 5/5, scenarios 15/15,
archive-ready — but carried two documentation-grade WARNINGs forward. Batch 10 closes both,
surgically, without re-opening anything rounds 7/8 confirmed clean.

### W1 — `sdd-time-estimation` starved a consumer this change also edits

**Finding**: `skills/sdd-time-estimation/SKILL.md`'s CALIBRATION rule builds its per-phase
agent-compute-time baseline from `implementation_hours`, `review_gate_hours`, and
`post_review_fix_hours` "ONLY". R-019 (added by this change) mandates those three fields stay
unpopulated until a durable source exists — so this change removes that skill's only baseline
inputs, edits that same file (D4/task 4.1 already touched line 28's boundary word), and never
updated the CALIBRATION rule for the case R-019 itself creates. The governing rule: an objective
that changes what a value MEANS must enumerate its consumers; "do not touch X" must never mean "do
not read X". The consumer was in the diff and was left unupdated.

**Fix** (`skills/sdd-time-estimation/SKILL.md`, CALIBRATION bullet, Hard Rules section):

Before (unchanged sentence, still present, R009's own pin):
> `total_wall_clock_hours` is NEVER an input to the agent-compute-time baseline — it measures
> elapsed calendar time …

Inserted immediately before that sentence (new):
> **WHEN one or more of these three fields is absent under R-019** (compute-time fields stay
> unpopulated until a durable source exists), the record supplies NO compute-time numerator for
> the affected phase — never substitute `total_wall_clock_hours` or any other elapsed-time figure
> in its place. State the consequence explicitly rather than dividing over an absent value: with
> the numerators absent, the per-unit compute-time rate cannot be computed, so confidence falls
> back to the disclosed bootstrap defaults / qualitative scaling regardless of calibration `n`.

This satisfies every requirement in the work order: (a) no substitute invented — `total_wall_clock_hours`
is explicitly named as forbidden, consistent with the pre-existing "never blended into their sum"
rule (R-001/R-002) and R009's own pin; (b) the honest consequence is stated explicitly — fallback to
disclosed bootstrap defaults / qualitative scaling, regardless of `n`, rather than a silent division
over an absent value; (c) `total_wall_clock_hours`'s existing role (separately-labelled delivery-window
diagnostic, `NEVER` an input to the compute-time baseline) is left byte-unchanged immediately after
the new sentence.

**New test**: `R019_calibration_absent_numerators_fallback` added to
`engine/skills/actuals_instrumentation_contract_test.go`, scoped to the CALIBRATION list item (not
the whole file) via the pre-existing `markdownListItemContaining` helper — the same idiom already
used by `R001_R002_three_units_never_blended` for a different Hard Rules bullet, so a substring hit
elsewhere in the file cannot satisfy this gate.

**Mutation Probe Results — every RED and restore run with `go test -count=1`**:

| # | Probe | Mutation | Result | Restore |
|---|---|---|---|---|
| 15-P1 | deletion | Deleted the entire new sentence, leaving the pre-existing R009-pinned sentence untouched | **RED** — `CALIBRATION rule must contain "WHEN one or more of these three fields is absent under R-019", got "..."` (full un-mutated paragraph echoed back) | GREEN; `sha256:2ed04f4e6499c0c0462e4d2611d47aeee8471f61398adeed264728dc50fdd4df` identical, `cmp`-verified |
| 15-P2 | **inversion — the load-bearing, non-vacuity probe** | Replaced the new sentence with its logical opposite: "substitute `total_wall_clock_hours` for the missing numerator so the per-unit compute-time rate can still be computed at full confidence regardless of calibration `n`" | **RED** — `CALIBRATION rule must contain "the record supplies NO compute-time numerator for the affected phase", got "...substitute total_wall_clock_hours..."` — proves the pin detects the rule being contradicted, not merely absent | GREEN; identical, `cmp`-verified |
| 15-P3 | full baseline | No mutation, full 16-subtest suite (`-v`) | GREEN throughout, all 16 subtests PASS | n/a |

SKILL.md restored byte-identical after every probe:
`sha256:2ed04f4e6499c0c0462e4d2611d47aeee8471f61398adeed264728dc50fdd4df`, confirmed via both
`sha256sum` and `cmp -s` before and after both probes. Full contract test suite re-run GREEN at
`-count=1` after final restore (16/16 subtests PASS, see full output hash below).

### Consumer-enumeration sweep (W1's second half — sweep the class, not just the one citation)

Governing rule (standing user directive): an objective that changes what a value MEANS must
enumerate its consumers. This change modifies `skills/_shared/actuals-record.schema.json`,
`skills/inception-pipeline/SKILL.md`, and `skills/sdd-time-estimation/SKILL.md` (now, Batch 10).
Swept every repository file (excluding `openspec/changes/archive/**` and this change's own
`openspec/changes/sdd-cycle-timestamp-instrumentation/**`) for references to
`implementation_hours`, `review_gate_hours`, `post_review_fix_hours`, `checkpoint_count`,
`total_wall_clock_hours`, `sdd/{change}/actuals`, `pipeline-state`, and `closure-feedback`, via
`rg -ln` across `skills/`, `engine/`, `tools/`, `tui/`. Every hit classified below:

| Consumer | What it reads | Affected by this change? | Action |
|---|---|---|---|
| `skills/_shared/actuals-record.schema.json` | The schema itself (`required` list, field descriptions) | YES — this change's own primary artifact | Already updated (Phases 1, 12) |
| `skills/inception-pipeline/SKILL.md` (closure-feedback) | Writes the actuals record; owns the merge-boundary and D2 anchor logic | YES — this change's own primary artifact | Already updated (Phases 3, 7B) |
| `skills/sdd-time-estimation/SKILL.md` | Reads `implementation_hours`/`review_gate_hours`/`post_review_fix_hours` as calibration numerators (R-009); reads `total_wall_clock_hours` as a separate delivery-window diagnostic (R-009/R-010/R-011) | **YES — genuinely starved, previously unfixed** | **Fixed this batch (15A)** |
| `engine/skills/actuals_instrumentation_contract_test.go` | Pins the shape of all of the above | YES — new content requires new pins | Already updated (this batch, `R019_calibration_absent_numerators_fallback`) |
| `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json` | A fixture actuals record (D7, R-014/R-020) | YES — its own fields, own boundary text | Already updated (Phase 5) |
| `engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json` | A fixture actuals record (D7, R-020) | YES | Already created (Phase 11) |
| `skills/roadmap-maker/SKILL.md` | Reads `sdd/{change}/actuals` generically — "copy relevant fields into the tracking line" — never hard-codes a field name or a numerator computation | NO — genuinely unaffected. Already guarded by pre-existing `R013_roadmap_maker_no_compute_time_source_invariant` (FORBID list over the five structured field names, confirmed zero hits) | None needed |
| `tools/entry-contract-validator/main.go` | Validates the `sdd/{change}/pipeline-state` **topic-key string format** only — never reads the record's content or field semantics | NO | None needed |
| `tools/entry-contract-validator/testdata/valid-entry-contract.json` | Fixture topic-key string | NO | None needed |
| `skills/project-inception/SKILL.md` | Cites the `sdd/{change}/actuals` topic-key in a references list (ownership only) | NO | None needed |
| `skills/_shared/pre-sdd-contracts.md` | Topic-key authority table: `sdd/{change}/actuals` → owner `inception-pipeline closure-feedback (ONLY writer)` | NO — ownership statement, not field semantics | None needed |
| `skills/inception-pipeline/references/tier-selection.md` | References `sdd/{change}/pipeline-state` for tiering-rationale recording, unrelated to actuals fields | NO | None needed |
| `openspec/specs/actuals-instrumentation/spec.md` (live/merged base spec) | The currently-merged spec this change's delta will merge into at archive time | NO — not touched by design; OpenSpec's own archive-time merge updates it, not `sdd-apply` | Out of this phase's scope by construction |

**Result**: `sdd-time-estimation/SKILL.md` was the one genuine gap (fixed, 15A); every other hit
is either this change's own already-updated primary artifact or a consumer that reads only
topic-key strings / ownership / ungated generic-copy prose, never the specific field semantics
R-019/R-021 changed.

### W2 — the revert instructions omitted the two files this change CREATES

**Finding**: `design.md:165`'s Migration / Rollout section said "Revert = delta spec + 5 file
edits", omitting the two NEW files this change creates:
`engine/skills/actuals_instrumentation_d2_anchor_test.go` and
`engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json`. The anchor test reads
shipped `SKILL.md` prose at runtime — confirmed by direct inspection:
`readRepoFile(t, repoRoot, inceptionPipelineSkillRelPath)` at line 197 of the 415-line file. A
literal revert following the old instructions (delta spec + 5 edits only) leaves this 415-line
test in place, still pinning the shipped (now-reverted-away) prose — suite RED after a "complete"
revert.

**Fix** (`design.md`, Migration / Rollout section):

Before:
> D7 upserts are the only migration; reversible by topic-key upsert. Revert = delta spec + 5 file
> edits.

After:
> D7 upserts are the only migration; reversible by topic-key upsert. Revert = delta spec + 5 file
> edits (`skills/_shared/actuals-record.schema.json`,
> `engine/skills/actuals_instrumentation_contract_test.go`, `skills/inception-pipeline/SKILL.md`,
> `skills/sdd-time-estimation/SKILL.md`,
> `engine/skills/testdata/corrected-actuals-sync-check-repo-behind-origin.json`) + 2 file deletions
> (`engine/skills/actuals_instrumentation_d2_anchor_test.go`,
> `engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json`) — 7 files total. The
> anchor test (`actuals_instrumentation_d2_anchor_test.go`) reads shipped `SKILL.md` prose at
> runtime (see D2 revision 3's Testing Strategy row), so deleting it is mandatory, not optional:
> leaving it in place after reverting the other 5 edits pins reverted prose against a test that
> still expects the shipped wording, leaving the suite RED.

No test pin exists for design.md's Migration / Rollout prose itself (confirmed:
`TestAbolishedVocabularySweptFromCurrentSections`'s FORBID list — `path-scan`, `first-parent`,
`receipt-bound`, and the three other abolished-vocabulary strings — contains none of the words
introduced by this edit), so this is a documentation-only fix with no RED/GREEN pin required; its
correctness rests on the direct verification performed (file existence, line count, and the exact
runtime read call cited above), not on a test assertion.

### Full suites, build, and scope discipline

```
for m in engine tui tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do
  (cd "$m" && go test -count=1 ./...)
done
```
All 5 modules green (`engine` 10 packages, `tui`, `tools/deterministic-check-runner`,
`tools/entry-contract-validator`, `tools/review-preflight`). `go build -o <scratch>/bin/ ./...`
exit 0 across all 5 modules. `git status --untracked-files=all` — identical tracked/untracked file
set to post-Phase-14 (5 modified, 9 untracked, all pre-existing from earlier phases) — no stray
files introduced by this batch.

Full contract-test output (16/16 subtests, `-v -count=1`) hashed for the settle evidence-revision:
`sha256:5825df55efb4d22db11e5331619d79283bf62629eafa9b53d9ef94b84e38c16a`.

### Explicitly NOT performed (per scope)

- Did NOT re-open R-019, R-021, the four-state anchor rule, the sweep guard, or anything else
  rounds 7/8 confirmed clean.
- Did NOT withdraw, re-base, or rewrite live Engram records #2789/#2096 or their fixture mirrors —
  round 8 re-confirmed both records' claims correct.
- Did NOT convert the roadmap-maker FORBID list (round 8 rated it SUGGESTION, purely prospective —
  this change adds no new schema property).
- Did NOT touch cumulative size / chained PRs (delivery question, handled separately).
- Did NOT touch task 6.3 (structurally deferred to archive — this change's own closure).

### Files changed (Batch 10) — well under the 400-line budget

| File | Changed lines (this batch only) |
|---|---|
| `skills/sdd-time-estimation/SKILL.md` | 1 line replaced (net 0 length delta in diff terms: `-1/+1`, one wrapped markdown paragraph) |
| `engine/skills/actuals_instrumentation_contract_test.go` | +20 (pure insertion, new subtest) |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/design.md` | 1 line replaced with an expanded sentence (untracked file — new artifact for this change, not yet under git diff tracking) |
| `openspec/changes/sdd-cycle-timestamp-instrumentation/tasks.md` | +~68 (Phase 15 section appended, plus 14E.1 marked `[x]`) |

**Total authored implementation + design edit** (excluding tasks.md/apply-progress.md
bookkeeping): **≈22 lines** — far under the 400-line review budget.

### Native runtime attempt authority (Batch 10)

Acquired via `gentle-ai sdd-attempt acquire` with `request-id
phase15-consumer-impact-2026-08-14-actor`, `--max-attempts 2 --max-changed-lines 400` — state
`proceed`. Evidence goal delivered in full: (1) `sdd-time-estimation/SKILL.md` now states the
honest consequence for absent compute-time numerators under R-019, mutation-pinned (15-P1
deletion, 15-P2 inversion, both RED; restore byte-identical); (2) `design.md`'s revert list now
names all seven files (5 edits + 2 deletions), with the mandatory-deletion reason stated and
independently verified (file existence, line count, exact runtime read call); (3) the
consumer-impact sweep is exhaustive over the repository (not just this change's own folder) and
recorded as a table — `sdd-time-estimation/SKILL.md` was the one genuine gap, closed; every other
hit is either an already-updated primary artifact of this change or a confirmed-unaffected reader.
