```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:353287b03ecee4ca1b63bd0974f61d43983745ed7ca08b3ff37d460cfae3522a
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 5/5
scenarios: 15/15
test_command: 'for m in engine tui tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do (cd "$m" && go test -count=1 ./...) || exit 1; done'
test_exit_code: 0
test_output_hash: sha256:45ada0f4a3e1c69cd0eb09c06ac81332247ac97649de15de88d8d8eedc680a9e
build_command: 'out="$(mktemp -d)"; for m in engine tui tools/deterministic-check-runner tools/entry-contract-validator tools/review-preflight; do (cd "$m" && go build -o "$out/" ./...) || exit 1; done'
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# Verification Report — sdd-cycle-timestamp-instrumentation (round 9, post-merge, gap-closing)

**Change**: `sdd-cycle-timestamp-instrumentation` | **Phase**: verify (round 9) | **Store**: hybrid | **Mode**: Strict TDD
**Date**: 2026-08-19 | **Branch**: `main` | **HEAD**: `04488df` | **State**: fully merged, working tree clean

## Scope of this certification — this round closes round 8's stated gap

Round 8 certified the change **only through Batch 9 (Phase 14)** and said so in a banner. Batch 10
(Phase 15) landed after it and edited `skills/sdd-time-estimation/SKILL.md` — a shipped, agent-executed
rule file, which under this change's own Phase 7B premise **is** implementation — plus added one contract
subtest. Task `15D.1` exists to close exactly that gap.

**Round 9 covers the Batch 10 delta and re-confirms the whole candidate on merged `main`.** Unlike round 8,
this round inspects committed, merged bytes rather than an uncommitted worktree: all four chained slices
landed (`321f54a`, `3ede60e`, `8c552a1`, `7343ff5` via PRs #143/#147/#145/#146). There is no longer any
uncertified batch. This report supersedes round 8's scope banner.

### Verdict field changed from `pass` to `pass_with_warnings`

Round 8's envelope said `verdict: pass` while its own prose verdict said "PASS WITH WARNINGS". Those
contradicted each other. `pass_with_warnings` is an accepted value of `gentle-ai.verify-result/v1`, so
round 9 uses the one that matches its prose. This is a truthfulness correction, not a downgrade:
`blockers` and `critical_findings` are both still 0, which is what gates archive.

### How `evidence_revision` was derived (re-derivable, unlike a bare assertion)

`evidence_revision` is the SHA-256 of a manifest containing, in this order: `head <HEAD sha>`, then one
`<sha256>  <path>` line for each of the eight inspected artifacts (schema, both shipped `SKILL.md`, both Go
test files, both testdata fixtures, the delta spec), then `test_output_hash` and `build_output_hash`.
Anyone can rebuild it. Round 8 deliberately did not re-stamp this field because its correction did not
re-run verification; round 9 did re-run everything, so it carries a new revision.

Corroboration worth stating: the schema hashes to `0684faa369cf91e9…`, **byte-identical to round 8's
recorded baseline**, and the ondisk-gate fixture hashes to `31226587098f12ce…`, matching obs #2896. The
instrument's contract surface did not drift while the change was being sliced and merged.

## Verdict

**PASS WITH WARNINGS** — 0 CRITICAL, 5 WARNING, 5 SUGGESTION. **Archive-ready: YES.**

**Task 15D.1 is honestly closeable.** The Batch 10 delta is real implementation, and it is now genuinely
verified rather than assumed: I reproduced the shipped rule at its primary source, mutation-probed its new
test in three directions (including one direction apply never ran), and re-ran the full suites and builds.

The two WARNINGs round 8 raised are **not both closed**. W1 is fully closed. W2 is **half closed** —
Phase 15 fixed the revert instruction but not the File Changes table, which was the other half of round 8's
own stated remedy. That is reported below as W1/W2 of this round, not absorbed.

---

## 1. Round 8 W1 — CLOSED, and now test-bound

**The gap.** `sdd-time-estimation`'s CALIBRATION rule built its baseline from `implementation_hours`,
`review_gate_hours`, `post_review_fix_hours` "ONLY", while R-019 (added by this very change) mandates those
three stay unpopulated. The skill had no branch for its own numerators being absent, and its `n>=3` branch
directed a reader to divide by a field that would no longer exist.

**The fix, read at the primary source** (`skills/sdd-time-estimation/SKILL.md:28`, inside the CALIBRATION
list item, immediately after the "ONLY" sentence):

> **WHEN one or more of these three fields is absent under R-019** (compute-time fields stay unpopulated
> until a durable source exists), the record supplies NO compute-time numerator for the affected phase —
> never substitute `total_wall_clock_hours` or any other elapsed-time figure in its place. State the
> consequence explicitly rather than dividing over an absent value: with the numerators absent, the
> per-unit compute-time rate cannot be computed, so confidence falls back to the disclosed bootstrap
> defaults / qualitative scaling regardless of calibration `n`.

Checked against round 8's stated remedy, clause by clause:

| Round 8 asked for | Present? |
|---|---|
| the three fields are absent from records written after this change | ✅ "WHEN one or more of these three fields is absent under R-019" |
| such records excluded from the per-phase baseline and rate denominators | ✅ "supplies NO compute-time numerator"; "the per-unit compute-time rate cannot be computed" |
| the `n>=3` divide-by-missing-field trap is overridden | ✅ "regardless of calibration `n`" — this is the clause that neutralises the `n>=3` branch |
| compute-time `n` stays at 2 until a durable source exists | ⚠️ expressed functionally ("falls back to bootstrap defaults / qualitative scaling") rather than as the literal number 2 |

The last row is a **deliberate improvement, not a shortfall**: hardcoding "n stays at 2" would go stale the
moment a third record lands, whereas the functional phrasing stays correct forever. I accept it as closed.

**R009's pre-existing pin is byte-unchanged.** The "`total_wall_clock_hours` is NEVER an input to the
agent-compute-time baseline" sentence still follows immediately, and
`R009_calibration_excludes_calendar_time` still passes.

### My own mutation probes — three directions, harness fails loudly on a non-landing mutation

Round 8 recorded a self-inflicted vacuity (an `sd` edit silently no-oped and four probes reported GREEN over
an unmutated file). I therefore used a Python harness with a hard `assert mutated_sha != ORIG_SHA` before
every run, and re-asserted byte-identical restore after every run.

| # | Direction | Result | Failure message |
|---|---|---|---|
| baseline | unmutated | **GREEN** | — |
| P1 | delete the new sentence | **RED** | `CALIBRATION rule must contain "WHEN one or more of these three fields is absent under R-019"` |
| P2 | invert it (substitute `total_wall_clock_hours` at full confidence) | **RED** | `must contain "the record supplies NO compute-time numerator for the affected phase"` |
| **P3** | **relocate the sentence verbatim out of the CALIBRATION item into another Hard Rules item** | **RED** | `must contain "WHEN one or more of these three fields is absent under R-019"` |
| restore | rewrite original bytes | **GREEN**, sha256 `2ed04f4e6499c0c0…` identical to baseline | — |

**P3 is my own addition and it is the decisive one.** Apply ran only delete and invert (15-P1/15-P2). P3
tests the subtest's *own stated claim* — its comment says "Scoped to the CALIBRATION list item itself, not
the whole file, so a substring hit elsewhere in Hard Rules cannot satisfy this gate." A whole-file substring
pin would have stayed GREEN under P3. It went RED, so the scoping is load-bearing and the claim is true.

I also confirmed the scoping helper is real rather than incidental: `sliceMarkdownListItem`
(`actuals_instrumentation_contract_test.go:1078`) finds the marker line, walks **back** to the enclosing
top-level list-item start and **forward** to the next list-item start or blank line, so the returned haystack
genuinely excludes the rest of the section.

**Non-vacuity of the subtest itself**: `go test -v` shows
`=== RUN TestActualsInstrumentationContract/R019_calibration_absent_numerators_fallback` followed by
`--- PASS`, not `[no tests to run]`. 16/16 contract subtests execute.

**Coverage effect.** This subtest strengthens R-019's scenario ("Deferral is stated, not silent"). Its pin
`never substitute total_wall_clock_hours or any other elapsed-time figure in its place` binds the scenario's
"none of the three fields holds a wall-clock-derived substitute" clause **at the consumer**, which no test
previously did.

## 2. Round 8 W2 — only half closed

**Closed half.** `design.md:165` now reads "Revert = delta spec + 5 file edits (…) + 2 file deletions
(`engine/skills/actuals_instrumentation_d2_anchor_test.go`,
`engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json`) — 7 files total", and states
why deletion is mandatory (the anchor test reads shipped `SKILL.md` prose at runtime, so a literal revert
would leave it pinning reverted prose and turn the suite RED). This is the half that carried the traced
consequence, and it is correct — I confirmed both files exist, are tracked, and that the anchor test does
read shipped prose at runtime.

**Open half.** Round 8's remedy was explicitly two-part: "add the two Create rows to the File Changes table
**and** restate the revert line". Only the revert line was done. See W1 and W2 below.

## 3. Independent consumer-enumeration sweep — 15A.3's claim holds

Round 8 caught Phase 14's sweep omitting a file, so I did not accept 15A.3's exhaustion claim. My own sweep
for `implementation_hours|review_gate_hours|post_review_fix_hours` across the repo (excluding `archive/**`
and this change's own folder) returns exactly seven files, and a separate sweep for *rate computation*
(`/ changed_lines`, `/ requirement_count`, `/ review_lens_count`) returns exactly one shipped consumer:

| File | Computes a rate? | Status |
|---|---|---|
| `skills/sdd-time-estimation/SKILL.md` | **YES** — the only one | ✅ fixed by 15A.1, bound by 15A.2 |
| `skills/_shared/actuals-record.schema.json` | no | ✅ already updated |
| `skills/inception-pipeline/SKILL.md` | no | ✅ already updated |
| `engine/skills/actuals_instrumentation_contract_test.go` | no (comments only) | ✅ updated |
| both `engine/skills/testdata/*.json` fixtures | no | ✅ historical records, correctly retain values |
| `openspec/specs/actuals-instrumentation/spec.md` (live base spec) | no | ➖ out of scope by construction — OpenSpec merges the delta at archive |

**15A.3's enumeration is exhaustive and its one out-of-scope call is correct.** I verified the live base spec
is still in its pre-change state (it still describes the archive-anchored boundary and the old
`checkpoint_count` of 12 for `sync-check`); that is the expected pre-archive condition, not a contradiction.

## 4. Whole-candidate re-confirmation on merged `main`

| Invariant | Expected | Observed | Verdict |
|---|---|---|---|
| `required` list | exactly 5 names | `approval_decision, change_name, project, scope_drift_notes, variance_vs_plan` | ✅ |
| property count | 13, none added/removed | 13 | ✅ |
| `additionalProperties` | `false` | `false` | ✅ |
| the four relaxed fields | present as properties, absent from `required` | all four confirmed both ways | ✅ |
| `total_wall_clock_hours` description | merge-anchored | "from the tiering go-ahead checkpoint to merge, including interruption gaps" | ✅ |
| schema bytes vs round 8 | unchanged | `0684faa369cf91e9…` identical | ✅ |

**The verification gate itself can actually fail — re-proven, not assumed.** Slice 4's review found that a
bare `for` loop reports only its last iteration's status, so a failure in any of the first four modules was
silently discarded. I re-demonstrated both forms against a genuinely failing first module:

```text
bare form:     for m in nonexistent-module engine; do (cd "$m" && go test ...); done          -> exit 0  (masks the failure)
recorded form: for m in nonexistent-module engine; do (cd "$m" && go test ...) || exit 1; done -> exit 1  (propagates)
```

The `|| exit 1` form is intact in both recorded commands. **It was not regressed.**

**`test_output_hash` is genuinely reproducible.** Normalising away per-package wall-clock durations (trailing
tab + `<n>.<n>s`, and trailing tab + `(cached)`) reproduces
`sha256:45ada0f4a3e1c69c…` — **the exact value round 8 recorded**, computed independently on a fresh run on a
different branch state. That is strong evidence the normalisation recipe is correct and portable, not a
one-off.

## 5. Deterministic quality gates

**`gofmt -l`**: clean across all five modules (no output). **`go vet ./...`**: exit 0 across all five
modules. Both are in the blocking set; neither fired, so no CRITICAL arises from them.

---

## WARNING W1 (new this round; round 8's W2, open half) — File Changes table still omits the two created files

`design.md`'s File Changes table (`:130-138`) lists 5 `Modify` rows, 1 `Create` row (the delta spec) and 1
Engram `Upsert` row. It still does **not** list either file this change creates:

- `engine/skills/actuals_instrumentation_d2_anchor_test.go` (432 lines, tracked, on `main`)
- `engine/skills/testdata/corrected-actuals-skills-validate-ondisk-gate.json` (tracked, on `main`)

**Why this is worth reporting rather than absorbing.** Round 8 noted the revert line "maps exactly onto
design.md's own File Changes table", i.e. the two were consistent while both were wrong. Phase 15 fixed one
and not the other, so `design.md` is now **internally contradictory**: `:165` says "7 files total" while the
table two sections earlier accounts for 5 files plus the spec plus an Engram upsert, and names neither
created file. A reader reconciling the two cannot tell which is authoritative.

**Severity: WARNING, not archive-blocking.** It falsifies an inventory note, not an acceptance criterion; no
requirement or scenario depends on it; the suite is green.

**Remedy (documentation only)**: add two `Create` rows to the File Changes table.

## WARNING W2 (new) — the File Changes row for the file Phase 15 edited is now stale

The table's row for `skills/sdd-time-estimation/SKILL.md` reads:

> Line 28: "(tiering-go-ahead to archive, …)" → merge; R-009 pins unaffected

That described the change accurately **before** Phase 15. Phase 15 then added an entire new R-019 fallback
rule to that same line 28 — the edit round 8 called the important one, and the one 15D.1 exists to verify.
The change's own inventory now under-describes its most consequential shipped prose edit.

This is the same meta-claim class as round 7's CRITICAL-1 and round 8's W2: a claim about the change's own
extent, contradicted by the artifact set. It is the smallest instance yet — one stale table cell.

**Severity: WARNING, not archive-blocking.** **Remedy**: extend that row to mention the R-019 absent-numerator
fallback sentence.

## WARNING W3 (carried, with a material status change) — task 6.3 is no longer structurally blocked

Rounds 6-8 recorded 6.3 as "not closable within this candidate" because t1 is this change's own merge commit.
**That blocker is now gone**: the change merged on 2026-08-19. 6.3 is now genuinely exercisable at
archive/closure, and this change's own closure becomes the instrument's first live self-measurement (n=3).

I resolved both anchors to show it is actionable:

| Anchor | Resolution | Evidence |
|---|---|---|
| **t0** | **FALLBACK branch** — no `sdd/sdd-cycle-timestamp-instrumentation/pipeline-state` observation exists (searched; none found). Earliest own-change observation is **#2859** `sdd/…/explore`, `created_at` **2026-08-13 13:37:34** | R-016 requires the fallback be *named* in `variance_vs_plan` so it is never mistaken for a tiering-go-ahead anchor |
| **t1** | **`a0374e33ffdf168e98ecb68b5170e46816803401`** (PR #146, last slice to land), committer **2026-08-19T15:42:14-03:00** | its tree `5ab14902aa0d0fd9…` equals slice 4's approved candidate `7343ff5`'s tree, lineage `review-8afca11eac0d548b` → outcome **verified**, not self-asserted |

This change will therefore exercise the *fallback t0* + *verified t1* combination — a real branch of its own
instrument, which is exactly the evidence 6.3 was written to collect.

## WARNING W4 (new) — nothing has recorded `landing_commit` yet, and the tree is a live 3-way collision

The shipped rule (`inception-pipeline/SKILL.md:147`) says the archive-report records `landing_commit` "at
delivery — when the merge has just happened and the values are trivially known". **Delivery has already
happened and no archive-report exists yet**, so nothing versioned records it. This is the designed sequence
(the archive-report is created by archive), but it is time-sensitive, and this repository now contains the
precise hazard the spec warns about:

**Three commits reachable from `main` carry the identical tree `5ab14902aa0d0fd9…`:**

| Commit | Committer time | What it actually is |
|---|---|---|
| `04488df` | 2026-08-19T19:03:38+00:00 | branch-sync merge of `origin/main` into a diverged local `main` — **not a slice of this change**, and the chronologically **last** merge |
| `a0374e3` | 2026-08-19T15:42:14-03:00 (18:42:14Z) | **PR #146 — the last slice to land. This is the correct `landing_commit`.** |
| `7343ff5` | — | slice 4 tip, the reviewed/approved candidate |

Two concrete failure modes follow, both forbidden by the spec:

1. **Resolving by position** — picking the chronologically last merge on `main` yields `04488df` and
   inflates t1 by **21 minutes**. The spec says an ambiguous anchor is "never resolved by position".
2. **Discovering by tree** — the tree has three carriers, so tree-based discovery is ambiguous by the spec's
   own rule ("a tree hash MUST be used to **verify** a commit, never to **discover** one"). Under that rule
   t1 would have to be omitted entirely.

Both are avoided **only** by recording `landing_commit = a0374e3` explicitly, after which the tree becomes a
successful cross-check rather than a search key. This is the spec working as designed — but it only works if
the value is written down.

**Severity: WARNING, not archive-blocking** (archive is precisely where it gets recorded). Flagged because a
wrong t1 here would corrupt the first datapoint of the instrument this change exists to build.

## WARNING W5 (carried, round 6) — sweep-guard heading recognition

The abolished-vocabulary span guard recognises only unindented ATX headings. Pre-existing recognition limit,
accepted, not a regression. Unchanged.

---

## SUGGESTIONS

- **S1 (carried)** — a substring pin cannot detect a semantic inversion that preserves the pinned bytes
  (Engram obs #2890). Accepted class limitation. My P2 probe inverted the *meaning* and was caught only
  because it also changed the bytes.
- **S2 (carried)** — `R013_roadmap_maker_no_compute_time_source_invariant` hand-maintains five forbidden
  field names. Still prospective-only; this change adds no property, so the list stays complete. Unchanged
  at SUGGESTION.
- **S3 (carried)** — `assertStringList`'s failure message says `frontmatter list length` even when asserting
  `schema.Required`. Cosmetic; both lists are printed.
- **S4 (carried, strengthened)** — a generic guard asserting that every exclusivity/count/extent claim has a
  binding test. **This class has now produced 3 CRITICALs (rounds 1-3, 7) and 4 WARNINGs (round 8 W1/W2,
  round 9 W1/W2).** Every individual instance keeps getting closed and the class keeps producing new ones,
  now migrating from specs into design bookkeeping. Worth solving structurally rather than per-instance.
- **S5 (new)** — `tasks.md` 15B.2 records the anchor test as "415 lines" with the runtime read "at line 197".
  It is now **432 lines** with the read at **line 210** (slice 3 added the vocabulary sweep). The substantive
  claim — that it reads shipped `SKILL.md` at runtime — is still true and I re-verified it. Stale line
  bookkeeping only.

---

## Spec compliance matrix — 5 requirements, 15 scenarios

Counted directly from `specs/actuals-instrumentation/spec.md`: 1 MODIFIED + 4 ADDED requirements;
3 + 7 + 2 + 1 + 2 = 15 scenarios. **Every covering test below was observed PASS at runtime in this run**
(25 subtests across three test functions, `-count=1`, on merged `main`).

| # | Requirement | Scenario | Covering test (runtime-verified) | Status |
|---|---|---|---|---|
| 1 | R-003/4/5 | Divergent calendar time, interruption included | `R004_R005_schema_temporal_boundary_and_interruption`, `R014_amendedR007_fixture_pins` | ✅ COMPLIANT |
| 2 | R-003/4/5 | Post-merge bookkeeping replies do not count | `R015_R006_R007_R008_checkpoint_count_description` | ✅ COMPLIANT |
| 3 | R-003/4/5 | No schema shape change anywhere in the record | `D2_D5_closed_schema_invariant` | ✅ COMPLIANT |
| 4 | R-016/17/18 | Anchors resolve and are legible in both stores | `recorded_anchor_with_matching_tree_resolves_verified` | ✅ COMPLIANT |
| 5 | R-016/17/18 | Anchor with no receipt is self-asserted, never verified | `recorded_anchor_with_no_review_receipt_is_self_asserted_never_verified` | ✅ COMPLIANT |
| 6 | R-016/17/18 | Unrelated change touching the folder is not the anchor | `no_recorded_anchor_yields_no_t1_and_attempts_no_folder_scan` | ✅ COMPLIANT |
| 7 | R-016/17/18 | A mis-recorded anchor is rejected, not trusted | `mismatched_tree_is_rejected_not_trusted` | ✅ COMPLIANT |
| 8 | R-016/17/18 | A change predating the convention omits rather than guesses | `no_recorded_anchor_yields_no_t1_and_attempts_no_folder_scan` | ✅ COMPLIANT |
| 9 | R-016/17/18 | A change that skipped inception-pipeline still measures | `R016_R017_R018_closure_feedback_anchor_prose` (t0-fallback pins) | ✅ COMPLIANT |
| 10 | R-016/17/18 | Neither anchor resolves | `R021_required_drops_wall_clock` write-anyway clause | ✅ COMPLIANT |
| 11 | R-021 | A cycle missing t0 still produces a usable record | `R021_required_drops_wall_clock` + schema validation | ✅ COMPLIANT |
| 12 | R-021 | Loosening required does not loosen the shape | `D2_D5_closed_schema_invariant` exact-list guard | ✅ COMPLIANT |
| 13 | R-019 | Deferral is stated, not silent | `R019_compute_time_deferral_rationale`, `R019_required_drops_compute_time_fields`, **`R019_calibration_absent_numerators_fallback` (new, Batch 10)** | ✅ COMPLIANT (strengthened) |
| 14 | R-020 | Reconstructed record stays annotated, not re-based | `R014_amendedR007_fixture_pins` + fixture provenance sentence (`total_wall_clock_hours = 36`, explicitly not re-based) | ✅ COMPLIANT |
| 15 | R-020 | Measured record re-based when both anchors resolve | `R020_skillsValidateOndiskGate_fixture_mirrors_measured_rebased_record` (`total_wall_clock_hours = 6.64`, verified anchor) | ✅ COMPLIANT |

**Scenarios: 15/15 compliant. Requirements: 5/5.**

Scenario 13 is the one the Batch 10 delta touches, and it is **strictly better covered than in round 8**: its
"none of the three fields holds a wall-clock-derived substitute" clause is now bound at the consumer skill,
not only at the schema and the deferral rationale.

## Correctness (static evidence)

| Requirement | Status | Notes |
|---|---|---|
| R-003/4/5 merge boundary | ✅ Implemented | schema descriptions + both shipped SKILL.md say merge |
| R-016/17/18 anchors | ✅ Implemented | 4-outcome rule shipped; verified/self-asserted/rejected/absent all present |
| R-019 compute-time deferral | ✅ Implemented | reason stated; consumer now handles absence (Batch 10) |
| R-020 historical annotations | ✅ Implemented | both fixtures carry correct provenance |
| R-021 relaxed `required` | ✅ Implemented | 5 names remain; 13 properties; `additionalProperties: false` |

## Coherence (design)

| Decision | Followed? | Notes |
|---|---|---|
| D2 rev 3 — tree verifies, never discovers | ✅ Yes | and now demonstrably necessary: a live 3-carrier tree collision exists on `main` (W4) |
| D5 — `required` drop sanctioned, two reasons | ✅ Yes | reconciled in round 8, unchanged since |
| D7 — Engram records byte-mirrored by fixtures | ✅ Yes | both fixtures mirror and carry provenance |
| Migration / Rollout — revert surface | ⚠️ Partial | revert line fixed; File Changes table not (W1, W2) |

---

## TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ | apply-progress Batch 10 records 15-P1/15-P2/15-P3 with hashes |
| All tasks have tests | ✅ | 15A.1 prose ↔ 15A.2 subtest; 15B is documentation-only, correctly unpinned |
| RED confirmed (test files exist) | ✅ | both test files present on `main`, compile, execute |
| GREEN confirmed (tests pass) | ✅ | 14/14 packages `ok`, exit 0, `-count=1` |
| Triangulation adequate | ✅ | 4 distinct substrings pinned; 3 independent mutation directions RED |
| Safety Net for modified files | ✅ | full suite green before and after every probe; restore byte-identical (`2ed04f4e…`) |
| Apply's probe claims independently reproduced | ✅ | 15-P1 and 15-P2 reproduced exactly; **P3 added by me and also RED** |

**TDD Compliance**: 7/7 checks passed.

Apply's claimed restore hash `sha256:2ed04f4e6499c0c0462e4d2611d47aeee8471f61398adeed264728dc50fdd4df`
matches my own independently computed baseline **exactly**. Apply's probe record is truthful.

## Test Layer Distribution

| Layer | What | Files | Tools |
|---|---|---|---|
| Unit | markdown slicers, schema classifiers, table-driven helpers | 1 | `go test` |
| Integration | contract pins against real shipped artifacts; `TestD2AnchorIsVerifiableAndUnambiguous` against real git history; vocabulary sweep | 2 | `go test`, read-only `git log` / `git show -s` |
| E2E | — | — | not applicable |
| **Total** | **14 packages ok / 25 relevant subtests** | | |

## Assertion Quality

✅ **All assertions verify real behaviour.** The new `R019_calibration_absent_numerators_fallback` pins four
distinct, semantically loaded substrings inside a *scoped* haystack; it has no tautology, no orphan empty
check, no ghost loop, no type-only assertion, and it calls the production artifact (reads the shipped file)
rather than a fixture copy. Its scoping is proven load-bearing by probe P3.

**Assertion quality**: 0 CRITICAL, 0 WARNING.

## Quality Metrics

**Linter (`gofmt -l`)**: ✅ clean, all 5 modules
**Vet (`go vet ./...`)**: ✅ exit 0, all 5 modules
**Build**: ✅ all 5 modules, exit 0, empty output
**Tests**: ✅ 14/14 packages ok, exit 0, `-count=1`
**Coverage**: ➖ not run — informational and non-blocking per the strict-TDD module

## Vacuous-pin trend

| Round | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | **9** |
|---|---|---|---|---|---|---|---|---|---|
| Vacuous pins | 1 | 3 | 5 | 0 | 0 | 0 | 0 | 0 | **0** |

Six consecutive clean rounds. Every pin probed this round had genuine mutation force in every direction
tried, including the scoping direction apply did not test.

---

## Task completion

**91 total: 88 checked, 3 unchecked** (after this round marks 15D.1 complete).

| Task | Status | Assessment |
|---|---|---|
| 15D.1 — re-run `sdd-verify` | **now `[x]`** | **Satisfied by this round.** The Batch 10 delta was inspected at its primary source, mutation-probed in 3 directions, and the full suites/builds re-run on merged `main`. This is the substantive gap review it asked for, not a formality. |
| 6.3 — own-closure live exercise (n=3) | unchecked | Structurally deferred to archive/closure. **No longer blocked** (W3): both anchors now resolve. |
| 8E.1 / 13E.1 — "full suites green; re-run `sdd-verify`" | unchecked | Self-referential verify handoffs from earlier remediation phases. Satisfied in substance by this run. Informational. |

**No unchecked task represents missing implementation. None is CRITICAL.** The three that remain are one
deferred closure exercise and two historical self-referential handoffs; blocking on a task whose content is
"run the phase that is currently running" would be a deadlock, which is why they are reported rather than
treated as incomplete work.

---

## What I did NOT find

Stated explicitly, because nine rounds is a lot and a manufactured finding would be worse than none:

- **No CRITICAL, no blocker, no regression.** The schema is byte-identical to round 8's baseline; the
  `|| exit 1` failure propagation was not reverted; `test_output_hash` re-derives to round 8's exact value.
- **No vacuous pin**, in the suite or in my own harness (which aborts on a non-landing mutation).
- **No consumer left starved.** My independent sweep found exactly one rate-computing consumer and it is
  fixed and test-bound.
- **No weakening of any requirement or scenario** to make Batch 10 pass. The delta only *added* coverage.
- **No new contradiction at requirement level.** Both new WARNINGs are design bookkeeping — a table with two
  missing rows and one stale cell — and neither falsifies a requirement, a scenario, or shipped behaviour.

## Archive-ready?

**YES.** Zero CRITICAL, zero blockers, 5/5 requirements, 15/15 scenarios, suites green, build clean, vet and
gofmt clean, on merged `main`.

**The certification gap round 8 declared is now closed.** Round 8's `verdict: pass` covered the candidate
only through Batch 9; this round covers Batch 10 and re-confirms everything else, so no batch of this change
remains uncertified. Task 15D.1 can be marked complete honestly.

The five WARNINGs are all documentation or process-sequencing items. W1/W2 are two-line edits to a table.
W3/W4 are instructions for whoever archives: record `landing_commit = a0374e3` in the archive-report and let
closure-feedback resolve t0 through the documented fallback. W5 is a pre-existing accepted limit. None
changes code, tests, schema, or any requirement, and none can regress the instrument.

**On convergence.** Round 8 said findings were "getting smaller and moving out of the contract and into its
documentation". Round 9 continues exactly that: the two new findings are a missing table row and a stale
table cell — the smallest instances the recurring meta-claim class has produced. The one genuinely new risk
(W4) is not a defect in the change at all; it is the change's own instrument correctly detecting an ambiguity
in the repository it will have to measure, which is the instrument working.
