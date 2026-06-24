# Apply Progress: prespec-malandra

PR slice: **PR-1** (branch `prespec-malandra/pr-1-domain`)  
Last updated: 2026-06-24  
Batch: 1 of 3  

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

## Remaining

- [ ] **T-04** — Lint (PR-2)
- [ ] **T-05** — Brief: ULID, schema, render, validate, TopicKey (PR-2)
- [ ] **T-06** — Orchestration + dispatch + main.go wiring (PR-2)
- [ ] **T-07** — SKILL.md + coverage-taxonomy.md (PR-3)

---

## Test output (PR-1 final state)

```
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/cmd        0.015s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate       0.002s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/prespec    0.002s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator 0.002s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings   0.004s
```
