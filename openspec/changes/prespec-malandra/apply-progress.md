# Apply Progress: prespec-malandra

PR slice: **PR-2** (branch `prespec-malandra/pr-2-lint-brief-dispatch`)  
Last updated: 2026-06-24  
Batch: 2 of 3  

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

## Remaining

- [ ] **T-07** — SKILL.md + coverage-taxonomy.md (PR-3)

---

## Test output (PR-2 final state)

```
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/cmd        0.027s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate       0.003s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/prespec    0.003s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator 0.002s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings   0.005s
```
