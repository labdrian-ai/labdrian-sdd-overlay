# Apply Progress: prespec-malandra

PR slice: **PR-3** (branch `prespec-malandra/pr-3-skill`)  
Last updated: 2026-06-24  
Batch: 3 of 3 — IMPLEMENTATION COMPLETE  

---

## Completed (PR-1)

- [x] **T-01** — Cell types and state machine  
  Files: `engine/prespec/grid.go`, `engine/prespec/grid_test.go`  
  Commit: `feat(engine/prespec): cell type, state machine, and 10-cell taxonomy (test-first)`  
  All transitions tested: Missing→Partial, Missing→Clear, Partial→Clear (valid);  
  Clear→Partial, Clear→Missing (rejected, state unchanged, error returned).  
  10-cell taxonomy verified (key, Impact, Uncertainty, initial Missing state).

- [x] **T-02** — Grid functions (rank, coverage count, budget, stop)  
  Files: `engine/prespec/grid.go`, `engine/prespec/grid_test.go` (same commit as T-01)  
  Grid.New() → 10 Missing cells.  
  RankUncovered() → Impact×Uncertainty desc, index asc tie-break; Clear cells excluded.  
  CoverageCount(), BudgetRemaining() (standard=5, unknown→5 safe default).  
  ShouldStop() priority: budget > convergence (6 cells) > user signal.

- [x] **T-03** — Readiness score and 0.6 gate  
  Files: `engine/prespec/readiness.go`, `engine/prespec/readiness_test.go`  
  Commit: `feat(engine/prespec): readiness score and 0.6 gate with Partial=0.5 weight (test-first)`  
  Formula: (Clear + 0.5×Partial) / 10 (ADR-5).  
  ReadinessGate=0.6 exported constant. Score.Passes() boundary-inclusive.  
  Boundary cases: 6 Clear=0.6 passes; 10 Partial=0.5 fails; 5 Clear+1 Partial=0.55 fails.

---

## Completed (PR-2)

- [x] **T-04** — Lint: no-leading question rules  
  Files: `engine/prespec/lint.go`, `engine/prespec/lint_test.go`  
  Commit: `feat(engine/prespec): no-leading lint rules with deterministic regex (test-first)`  
  Three compiled regex rules in order: smuggles-answer (would/wouldn't/do you want),
  presupposes-solution (feature/dashboard/api/module/integration/plugin/service/
  microservice/algorithm/solution), bundles-concerns (and).  
  First-match wins. LintResult carries Rule and Reason fields. Accepted=true when no rules fire.  
  Case-insensitive, stdlib regexp only (zero new deps).

- [x] **T-05** — Brief: ULID, schema, render, validate, TopicKey  
  Files: `engine/prespec/brief.go`, `engine/prespec/brief_test.go`,
         `engine/prespec/testdata/brief.golden`  
  Commit: `feat(engine/prespec): Brief schema, ULID hand-rolled zero-dep, render, validate, TopicKey (test-first)`  
  ULID: hand-rolled using crypto/rand + time.UnixMilli(), Crockford base32 uppercase 26 chars,
  matches `^[0-9A-HJKMNP-TV-Z]{26}$`. NewIDFrom(ts, reader) seam for deterministic tests.  
  Brief struct: DiscoveryID (not ChangeName — R-012), Project, CreatedAt, Job, Sections[6], Transcript.  
  TopicKey() panics on Project starting with "sdd/" (R-017).  
  Validate() checks readiness gate + non-empty Job + non-empty Transcript.  
  RenderBrief() golden file test with fixed time=2024-01-15T10:30:00Z, zeroBits reader.

- [x] **T-06** — Dispatch: prespec subcommand wired to engine main  
  Files: `engine/prespec/prespec.go`, `engine/prespec/prespec_test.go`,
         `engine/cmd/main.go`, `engine/cmd/main_test.go`  
  Commit: `feat(engine/prespec): prespec subcommand dispatch wired into engine main (test-first)`  
  PrespecCore(verb, stdin, stdout, stderr, exit) in prespec package — 4 verbs: rank/lint/readiness/brief.  
  Each verb: JSON decode from stdin → pure function call → JSON encode to stdout.  
  Fail loud (exit 1) on unknown verb or malformed JSON (ADR-4).  
  main.go: added `"prespec"` case to switch, runPrespec(args), runPrespecCore with injected I/O,
  verbFromArgs() helper. usage() updated. Package doc updated.  
  Engine is STATELESS — no persistence in Go; skill owns mem_save (ADR-2).

---

## Post-verify fixes (PR-2, conditional GO remediation)

- [x] **W-3 fix** — TopicKey + Validate empty Project guard  
  Files: `engine/prespec/brief.go`, `engine/prespec/brief_test.go`  
  Commit: `fix(engine/prespec): guard TopicKey and Validate against empty Project (W-3)`  
  TopicKey now panics on empty Project (before the sdd/ check) — prevents silent malformed key
  `project//prespec/<ULID>`. Validate now rejects empty Project as well.  
  New tests: TestTopicKeyEmptyProjectPanic, TestValidateRequiresProject.

- [x] **W-1 fix** — Narrow bundles-concerns lint rule  
  Files: `engine/prespec/lint.go`, `engine/prespec/lint_test.go`, `engine/prespec/prespec_test.go`  
  Commit: `fix(engine/prespec): narrow bundles-concerns to compound-ask signals only (W-1)`  
  Replaced bare `\band\b` with specific signals: `\band also\b`, `as well as`,
  `\band\s+(what|how|why|when|where|who)\b`. Incidental "and" in noun phrases and
  conditional clauses no longer fires. Removed redundant local `min()` (Go 1.21 built-in).
  Added LintResult comment (supersedes R-007a naming); added Brief spec field mapping comment.  
  New tests: TestLintBundlesConcernsRejects (8 cases), TestLintBundlesConcernsPasses (3 cases).

---

## Completed (PR-3)

- [x] **T-07** — SKILL.md + coverage taxonomy reference  
  Files: `skills/prespec-malandra/SKILL.md`,
         `skills/prespec-malandra/references/coverage-taxonomy.md`  
  Commit: `feat(skills): prespec-malandra Socratic interview skill and coverage taxonomy reference`  
  7-stage interview (0-6): Mom-Test cold start, 5-archetype MCQ fallback, refusal floor
  (needs-more-input on no actionable goal), anti-solution bounce, bounded readback (job only),
  adaptive loop with engine rank/lint/readiness calls, Stage 4 bounded assumptions (cap 3,
  each marked [ASSUMPTION] and interrogated against job), Stage 5 convergence gate (0.6),
  Stage 6 brief emission + mem_save to project/{project}/prespec/{ULID}, final non-leading readback.  
  Red lines encoded: no change-name derivation, no invented strawman, no self-grading,
  no-metric-yet escape for success-metric cell, namespace guard (never sdd/ prefix),
  no mem_save before gate passes. Hand-off contract to requirements-from-transcripts stated.  
  Coverage taxonomy reference: 10-cell table with I×U scores, ranking formula + tie-break,
  lint rejection checklist (3 rules + examples), stop conditions in priority order,
  no-metric-yet escape documented.  
  skill-registry updated with prespec-malandra entry.

## Remaining

None — all tasks complete.

---

## Test output (PR-2 final state after W-1/W-3 fixes)

```
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/cmd        0.019s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate       (cached)
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/prespec    0.004s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator (cached)
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings   (cached)
```
