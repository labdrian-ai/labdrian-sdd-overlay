```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:961d3a50e65c1bf2cadcd5a5455cdf6660ddba28770cbc7c4de415ee5b6c018c
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 9/9
scenarios: 13/13
test_command: cd engine && go test -count=1 ./... && cd ../tui && go test -count=1 ./...
test_exit_code: 0
test_output_hash: sha256:a2bc005cb89bdc2ccdbfa2961a1cc8d74805a86c5f5dd3b2546ca8ec5951d2e9
build_command: cd engine && go test -count=1 -run '^$' ./... && cd ../tui && go test -count=1 -run '^$' ./...
build_exit_code: 0
build_output_hash: sha256:53e0a4f4b2c51c1a22126e0b393365cab8b200c485dc897cac6253b1ebaae87b
```

## Verification Report

**Change**: actuals-calendar-checkpoint-instrumentation | **Mode**: Strict TDD | **Artifacts**: proposal, spec, design, tasks (full set)
**Candidate**: HEAD `508736b`, tree `c8cd081`, branch `fix/actuals-calendar-checkpoint-instrumentation`. Nothing staged, committed, or pushed.
**Worktree**: exactly the 2 declared modified tracked files (`engine/skills/actuals_instrumentation_contract_test.go` +38/-0, `specs/actuals-instrumentation/spec.md` +13/-5) plus the untracked report. Nothing unexpected. State digest `sha256:4029b64a…d997dd51` byte-identical before and after runtime execution.
**`evidence_revision` preimage**: 20 LF-joined `key=value` lines — schema, change, head, tree, worktree_state_digest, the four commands with exits and output hashes, live_obs_2096_digest, fixture_digest, requirements, scenarios, blockers, verdict.
**Completeness**: requirements 9 (compliant 9); scenarios 13 (compliant 13); tasks checked 16/16, substantively complete 16/16. The scenario total is 13, not the prior report's 12: the amended R-012/R-013 block split its single scenario into two.

### Build & tests — real uncached execution

| Command | Exit | Result | Output digest |
|---|---:|---|---|
| `cd engine && go test -count=1 ./... && cd ../tui && go test -count=1 ./...` | 0 | 10/10 engine + 1/1 tui packages ok | `sha256:a2bc005c…5951d2e9` |
| `cd engine && go test -count=1 ./skills/ -run TestActualsInstrumentationContract -v` | 0 | 1 test, **10/10** subtests PASS | `sha256:9dc4980c…ca26adb9` |
| `cd engine && go test -count=1 -run '^$' ./... && cd ../tui && …` (compile) | 0 | 11/11 packages compile | `sha256:53e0a4f4…ebaae87b` |
| `cd engine && go vet ./... && cd ../tui && go vet ./...` (+ `go build ./...` both modules) | 0 | no diagnostics, empty output | `sha256:e3b0c442…7852b855` |

Digests cover exact captured stdout+stderr; `go test` prints per-package timings, so they are run-specific and are not expected to match the prior report's. Coverage (informational): engine 81.0%, tui 79.3% — changed production files are markdown/JSON, which Go does not instrument.

### Blocker adjudication — the four prior CRITICALs

| # | Prior blocker | Verdict | Independent evidence |
|---|---|---|---|
| 1 | Live obs #2096 read 11/1/10, contradicting the 12/2/10 fixture | **RESOLVED** | Read #2096 content straight from `~/.engram/engram.db` and byte-compared to the committed fixture: both 3535 bytes, both `sha256:cd630b73a1ec3e900d907814df11a8cf7ed626b914ee5270516f937865a6c8a0`, `diff` clean — byte-identical, not merely substantive. `checkpoint_count: 12`, `total_wall_clock_hours: 36`, "Durable floor: 2 of 12", "Reconstructed from narrative: 10 of 12"; the false "the only checkpoint inception-pipeline itself durably records" claim is gone. |
| 2 | Apply-progress #2542 lacked the Strict-TDD Cycle Evidence table | **RESOLVED** | Revision 2 carries a 10-row table with an explicit RED-capable column; every row reproduced independently below, all 10 agreeing. Its provenance disclosure (executor killed by a provider 500; RED claims git-derived, not log-derived) is accurate, and I accept it as an honest evidence-class declaration rather than fabrication because each RED claim is reproducible from `git show` and both non-RED rows are labeled invariants in the table *and* in code. |
| 3 | R-001/R-002 had no covering assertion | **RESOLVED** | `R001_R002_three_units_never_blended` passes and is genuinely RED-capable: both markers count 0 at baseline, 1 at HEAD. |
| 4 | R-012/R-013 had no covering test | **RESOLVED; deferral judged honest** | R-012: `R012_actuals_output_separate_labels`, RED-capable (0→1). R-013: `R013_roadmap_maker_no_compute_time_source_invariant`, GREEN-by-construction and labeled NOT RED coverage in-code — RED is structurally impossible because D6 forbids editing the file and the obligation is negative. The spec's scope note names the exact file, section, and `**Tracking**:` line, states OUT OF SCOPE, cites D6, and explicitly disclaims delivering the positive obligation. Recorded deferral, not disguised completion. |

### RED-capability adjudication — derived from `git show`, working tree untouched

Baseline merge-base `86c3539`; all four files under test have identical blobs at merge-base and `main`, so the baseline choice changes no verdict. Counts are exact substring occurrences.

| Subtest | Markers at baseline | At HEAD | Class |
|---|---|---|---|
| `R004_R005_schema_temporal_boundary_and_interruption` | REQUIRE 2 = 0,0; FORBID "from first apply to archive" = 1 | 1,1; 0 | **RED** (doubly) |
| `R015_R006_R007_R008_checkpoint_count_description` | REQUIRE "round-trip" = 0; FORBID "LOWER BOUND" = 1 | 1; 0 | **RED** (doubly) |
| `R009_calibration_excludes_calendar_time` | REQUIRE 5 = 0 each; FORBID blend phrase = 1 | 1,1,2,1,2; 0 | **RED** (doubly) |
| `R010_R011_delivery_window_formula_shape` | REQUIRE 4 = 0 each | 1,1,2,1 | **RED** |
| `R006_R007_R015_inception_round_trip_and_explicit_zero` | REQUIRE 2 = 0,0; FORBID lower-bound sentence = 1 | 1,1; 0 | **RED** (doubly) |
| `R014_amendedR007_fixture_pins` | fixture absent from `main` (`git show` → fatal: path not in 'main') | exists, all pins pass | **RED** (whole group) |
| `R001_R002_three_units_never_blended` | REQUIRE 2 = 0,0 | 1,1 | **RED** |
| `R012_actuals_output_separate_labels` | REQUIRE 1 = 0 | 1 | **RED** |
| `R013_roadmap_maker_no_compute_time_source_invariant` | 5 FORBID names = 0 each | 0 each; file byte-identical to `main` | **INVARIANT** — not RED coverage |
| `D2_D5_closed_schema_invariant` | property set identical to HEAD | identical | **INVARIANT** — not RED coverage |

**8 genuine RED groups, 2 invariants.** Both invariants are labeled as invariants in code and in #2542, and neither is counted as RED coverage — honest. No invariant is presented as TDD evidence.

### Spec compliance matrix

| # | Req | Scenario | Covering evidence (passed at runtime) | Result |
|---:|---|---|---|---|
| 1 | R-001/2 | Units listed and kept separate | `R001_R002_…` (RED) | COMPLIANT |
| 2 | R-003/4/5 | Divergent calendar time, shared boundary, interruption | `R004_R005_…` (RED) + `R014_…` pins 36 ≠ compute sum 1.36 | COMPLIANT |
| 3 | R-003/4/5 | No schema shape change anywhere | `D2_D5_…` pins the exact 13-key set; independent parse: `additionalProperties: false`, `required` (9 keys), `$id` `…/actuals-record/2.0.0` all unchanged vs baseline | COMPLIANT (W-1) |
| 4 | R-015 | Batching vs. repetition counted correctly | `R006_R007_R015_…` (RED) + `R015_…_checkpoint_count_description` (RED) | COMPLIANT (S-1) |
| 5 | R-006/7/8 | Non-durable disclosed, durable vs reconstructed itemized | `R014_…` (RED) + live #2096 byte-identical + pipeline-state #2073 corroborates AMB-001 | COMPLIANT |
| 6 | R-006/7/8 | No non-durable → disclosure states so explicitly | `R006_R007_R015_…` marker "explicitly zero" (RED) | COMPLIANT |
| 7 | R-009 | Baseline text is unambiguous | `R009_…` (RED); HEAD baseline list is exactly the three phase fields, `total_wall_clock_hours` absent from it | COMPLIANT |
| 8 | R-010 | Formula, inputs, fixed buffer disclosed | `R010_R011_…` (RED), incl. "does not scale with checkpoint count" and the "uncalibrated" buffer | COMPLIANT |
| 9 | R-011 | Interrupted n=1 leaves no eligible clean sample | `R010_R011_…` + `R009_…` exclusion / never-subtract markers (RED) | COMPLIANT |
| 10 | R-012/13 | Actuals output labels units distinctly | `R012_…` (RED) | COMPLIANT |
| 11 | R-012/13 | roadmap-maker never sources tracking figures from compute-time | `R013_…_invariant` (all 5 field names = 0) + read of the line-85 ownership prose | COMPLIANT (W-2, S-3) |
| 12 | R-014 | Provenance disclaimer present | `R014_…` pins "RECONSTRUCTED FROM THE CLOSURE NARRATIVE, NOT MEASURED" (RED); present verbatim in live #2096 | COMPLIANT |
| 13 | R-014 | Checkpoint count added and itemized to 12 | `R014_…` pins `== 12`, "= 12", "Durable floor: 2 of 12", "AMB-001" (RED) + live identical + arithmetic below | COMPLIANT |

**13/13 scenarios, 9/9 requirements.** All 15 IDs R-001..R-015 trace into the spec.

### Independent checkpoint re-derivation

1 tiering go-ahead + 1 AMB-001 ambiguity question + 3 proposal-decision confirmations + 4 judgment-day rounds + 1 chained-PR split + 1 merge authorization + 1 ACTION-hint fix scope = **12**. Durable 2 + reconstructed 10 = **12**. Stored `checkpoint_count` = **12**. All three agree and match R-014's mandated itemization. The durable floor of 2 is corroborated at its source: `sdd/sync-check-repo-behind-origin/pipeline-state` (obs **#2073**) records the Tier-1 outcome and, verbatim, "One blocking ambiguity (AMB-001) surfaced during requirements capture … Resolved by asking the user". The "durably observed via pipeline-state" claim is verified, not asserted.

### Design coherence D1–D8

| D | Followed | Evidence |
|---|---|---|
| D1 | Yes | `total_wall_clock_hours` corrected in place; description now states tiering-go-ahead→archive plus interruption gaps; no field added or renamed. |
| D2 | Yes | `checkpoint_count` remains one field; zero new properties; durable/reconstructed split lives in `variance_vs_plan` prose. |
| D3 | Yes | Committed gate `engine/skills/actuals_instrumentation_contract_test.go` (232 lines) runs inside `go test ./...`; reuses `readRepoFile`, which `t.Fatalf`s on a missing file. |
| D4 | Yes | Fixture committed; live obs byte-identical to it (single shared digest above). |
| D5 | Yes | `$id` = `https://labdrian.ai/schemas/actuals-record/2.0.0`, unchanged. |
| D6 | Yes | `skills/roadmap-maker/SKILL.md` byte-identical to `main` (blob compare). |
| D7 | Yes | Formula shape only; interruption-clean gating, no guessed subtraction, fixed non-scaling uncalibrated allowance — all present and pinned. |
| D8 | Yes | Estimator fixes `d51a042`/`66bb084`/`508736b` all precede the #2096 upsert (`updated_at` 2026-07-30 02:45:18). Task 6.2 guard holds: zero `sdd/*/estimate` topic-keyed records exist for this project, and the only pre-start estimate observation (#2528) predates the upsert by ~10 h. |

### TDD compliance

| Check | Result | Detail |
|---|---|---|
| Cycle Evidence table reported | Yes | #2542 revision 2 — 10 rows plus summary. |
| All scenarios have a passing covering assertion | Yes | 13/13. |
| RED confirmed | Yes, git-derived | 8/10 groups RED-capable, each reproduced independently. Evidence class is marker diffing against baseline, not a captured failing transcript — disclosed, and valid for a pure content gate whose only input is repository bytes. |
| GREEN confirmed | Yes | 10/10 subtests and both suites pass now. |
| Triangulation | Adequate | 10 groups, 22 distinct marker assertions across 5 files; 5 groups pair FORBID with REQUIRE. |
| Safety net for modified files | Yes | Full suite green; the modified test file added groups without weakening any existing assertion (`git diff` = 38 additions, 0 deletions). |
| Invariants not miscounted as RED | Yes | Both excluded from RED coverage in code, in #2542, and here. |

### Schema closure & assertion quality

**Schema closure** — baseline vs HEAD, parsed: property key set identical at 13 keys, no addition anywhere, no `checkpoint_count_durable`, no `_supplemental`, no sibling of any name; `additionalProperties: false` unchanged; `required` unchanged (9 keys); `$id` unchanged. Only two `description` strings differ. Confirmed by both the shipped invariant subtest and an independent parse.

**Assertion quality** — no tautology, no ghost loop (every loop iterates a non-empty literal slice, the 10-key fixture, or the 9-item `required` list), no type-only-alone assertion, no smoke-only assertion, no mock. Helpers are real: `readRepoFile` `t.Fatalf`s on read error; `assertStringList` compares sorted length **and** elements. Layer: 1 file, 10 content-contract subtests over repository bytes; no unit/integration/E2E production layer applies to a markdown/JSON-schema change. **Tally: 0 CRITICAL, 3 WARNING (W-1, W-2, W-6).**

### Issues

**CRITICAL: none.**

**WARNING**

- **W-1** `D2_D5_closed_schema_invariant` never asserts `additionalProperties: false` — it unmarshals only `properties` and `required`. Scenario 3 requires that flag unchanged. I verified it independently (false at baseline and at HEAD), so the scenario holds, but the durable guard would not catch its removal. Add the assertion.
- **W-2** Content assertions are file-scoped, not section-scoped. Groups 1, 2, 3, 4, 5, 7, 8 grep the whole file while the design's gate table names specific targets (the `checkpoint_count` description, the CALIBRATION rule, Output items 6 and 14, the closure-feedback block). A marker relocated into the wrong section would still pass. Group 1's `"interruption"` is a single common word — the weakest pin in the suite.
- **W-3** `tasks.md` retains superseded acceptance criteria: lines 40 and 46 prescribe `checkpoint_count == 11` / "= 11" / "1 durable, 10 reconstructed"; line 57 prescribes the abandoned "below n=3" allowance gate; lines 37 and 56 describe a shortened proxy formula. These are **checked** tasks whose recorded criteria contradict spec R-014 (12/2/10) and the shipped test. Re-assessed on the merits and **kept WARNING, not escalated**: all 16 tasks are substantively complete, no scenario fails, and the contradiction resolves in the spec's favor (the spec has said 12 since `10da613`). But archive freezes these artifacts as the historical record, so normalize them through their owning workflow before archive rather than carrying a self-contradicting record forward. Not edited here, per scope.
- **W-4** Contract asymmetry on the durable ambiguity checkpoint. `inception-pipeline/SKILL.md:144` scopes it to "if rule 4 fired" (tier ambiguity), but for `sync-check-repo-behind-origin` rule **1** fired, yet pipeline-state #2073 durably recorded a *requirements-capture* ambiguity. The schema (line 70) and spec R-006 both say "if fired", with no rule-4 restriction. The record's durable-floor-2 claim is correct; the writer contract simply under-describes pipeline-state's real durable capacity. Reconcile in a future change.
- **W-5** Design gate group 6 declared the fixture pin should carry the full itemization string "1 tiering go-ahead + 1 AMB-001 … + 3 + 4 + 1 + 1 + 1 = 12"; the shipped test pins only "= 12", "Durable floor: 2 of 12", and "AMB-001". Weaker drift guard than designed — the fixture could drift its per-category counts and still pass. Not a spec violation: the itemization is present and re-derives to 12.
- **W-6** `D2_D5_closed_schema_invariant` (test lines 194-198) silently `return`s when the fixture is unreadable, self-disabling half that group. The fixture now exists permanently and group 6's `readRepoFile` fails loudly on absence, so absence is still caught — but this dead RED-era branch should be deleted.

**SUGGESTION**

- **S-1** R-015's batching half ("uniformly regardless of how many decisions a single reply resolved") has no pinned marker; only "one unit per distinct human round-trip reply" is asserted.
- **S-2** R-003's independence clause ("Measured independently of the agent-compute-time phase fields … never blended into their sum") has no pinned marker in either the schema or `inception-pipeline`.
- **S-3** Scenario 11's clause "its **only** actuals-related mention is prose correctly attributing ownership" is factually over-narrow: `roadmap-maker/SKILL.md` carries nine actuals-related prose mentions (lines 40, 42, 85, 100, 141, 142, 151, 152, 163), all correct ownership / read-only-consumer statements. Substantively satisfied; tighten the wording.
- **S-4** The branch is 2 commits behind `main` (`9999df5` merging upstream sync `1391563`). Those commits touch only `skills/sdd-verify/*` and none of the four files under test — verified by blob compare, so no verdict here depends on it. Rebase before delivery.

### Verdict

**PASS WITH WARNINGS**

All four previously-failing blockers are independently resolved against live sources rather than phase self-reports: obs #2096 is byte-identical to the committed fixture, #2542 carries a Cycle Evidence table whose every RED claim I reproduced from `git show`, R-001/R-002 and R-012 now have RED-capable assertions, and R-013's structurally-GREEN invariant is honestly labeled. 9/9 requirements and 13/13 scenarios have a passing covering assertion; the full runner, compile, and vet are exit 0; the schema stayed closed at 13 properties; the checkpoint arithmetic re-derives to 12/2/10 from an independent reading of pipeline-state #2073. Six WARNINGs remain and none invalidates a requirement; the strongest is W-3, stale `tasks.md` acceptance criteria to normalize through their owning workflow before archive. No CRITICAL finding blocks archive.
