# Verify Report: prespec-malandra — PR-1

**Scope**: T-01, T-02, T-03 only (branch `prespec-malandra/pr-1-domain`)  
**Verified**: 2026-06-24  
**TDD Mode**: STRICT  
**Verdict**: GO — 0 CRITICAL, 2 WARNING, 2 SUGGESTION

---

## Test Suite Output

```
go clean -testcache && cd engine && go test ./... -v
```

All 5 packages pass. Fresh run (cache cleared):

```
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/cmd        0.016s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate       0.003s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/prespec    0.002s  coverage: 97.7%
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator 0.001s
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings   0.004s
```

**engine/prespec tests (14 total, 0 failures)**:

- TestCellStateTransitions (5 subtests)
- TestCellDefinitions
- TestGridNew
- TestRankUncovered (3 subtests)
- TestCoverageCount (2 subtests)
- TestBudgetRemaining (3 subtests)
- TestShouldStop (4 subtests)
- TestReadinessCompute (6 subtests)
- TestReadinessGate (5 subtests)
- TestReadinessGateConstantExported

**Coverage**: 97.7% of statements. Only uncovered branch: `rem < 0` guard in `BudgetRemaining`.

---

## Requirement Compliance Grid

| Req | Description | Status | Covering Test(s) |
|-----|-------------|--------|-----------------|
| R-005a | 10-cell taxonomy (ID, Impact, Uncertainty, initial state) | PASS | TestCellDefinitions |
| R-005b | Grid.New() → all 10 cells in Missing state | PASS | TestGridNew |
| R-005c | I×U descending rank; ascending-index tie-break | PASS | TestRankUncovered/all-missing, TestRankUncovered/clear_cells_excluded, TestRankUncovered/fully-cleared |
| R-006 / R-006a | BudgetRemaining: standard=5, unknown mode→5 safe default | PASS | TestBudgetRemaining (3 cases) |
| R-009 | Readiness formula (ADR-5 authoritative: Partial=0.5) | PASS | TestReadinessCompute (6 cases including boundary) |
| R-010 | Gate >= 0.6 inclusive; ReadinessGate exported constant | PASS | TestReadinessGate, TestReadinessGateConstantExported |
| R-015 | Cell state transitions: forward valid, Clear regressions rejected, state unchanged on rejection | PASS | TestCellStateTransitions (5 cases) |
| R-016 | CoverageCount returns (clear, partial, empty) tuple | PASS | TestCoverageCount (fresh grid + mixed grid) |
| R-019 | ShouldStop: budget > convergence > user-signal priority | PASS | TestShouldStop (4 cases, all priority branches) |

**Out-of-scope requirements (deferred — DO NOT flag as failures)**:

- R-001..R-004, R-008, R-013: Skill layer (T-07, PR-3)
- R-007: Lint (T-04, PR-2)
- R-011, R-012, R-014, R-017, R-018: Brief/ULID (T-05, PR-2)
- S-R-001..S-R-006: Skill layer (T-07, PR-3)

---

## Issues

### WARNING-1 — BudgetRemaining defensive branch not tested

**File**: `engine/prespec/grid.go:111`  
**Detail**: The `if rem < 0 { return 0 }` guard handles `asked > budget` (e.g., `asked=6, mode="standard"`). This branch is reachable in integration scenarios (skill over-asks). 85.7% branch coverage on this function. Not a spec violation — the spec only defines `budget - asked` for valid inputs — but the guard is semantically meaningful and should be exercised.  
**Action**: Add `{"asked=6 over budget", 6, "standard", 0}` case to `TestBudgetRemaining` in PR-2 or as a follow-up commit on this branch.

### WARNING-2 — Spec R-009b formula divergence requires annotation

**Files**: `openspec/changes/prespec-malandra/specs/prespec-malandra/spec.md:289` vs `engine/prespec/readiness.go:35`  
**Detail**: Spec R-009b states `score = count(Clear)/10.0` (no partial credit). The implementation uses `(Clear + 0.5*Partial)/10` per ADR-5 from the design. This is a documented, intentional decision (tasks.md records: "Spec R-009b (no partial credit) is superseded by the design's ADR-5"). No code defect. However, a reader of spec.md alone will see a contradiction: R-009b says no partial credit, yet spec scenario "5 Clear 3 Partial → 0.65" implies Partial=0.5. The spec has an internal inconsistency that should be annotated or corrected in the next spec revision.  
**Action**: Add a spec errata note to R-009b in PR-3 (or when spec.md is next touched): "Superseded by ADR-5; see design.md."

### SUGGESTION-1 — CellState naming: Missing vs Empty

**Detail**: Spec prose (R-005, R-015 text) uses "Empty" throughout. Implementation exports `Missing` (matching tasks.md). No defect — the tasks artifact is the authoritative naming source. Consider a one-line comment in grid.go: `// Missing corresponds to "Empty" in spec prose — name chosen to be explicit.`

### SUGGESTION-2 — TestRankUncovered lacks mixed Empty+Partial subtest

**Detail**: R-005c defines a scenario where `current-gap` is `Partial` (not just `Missing`) and must still appear before `why-now` on a tie. The test's tie-break verification uses all-Missing cells, which does cover the sort logic correctly. An explicit `Partial-included-in-rank` subtest would improve spec traceability and guard against a future regression where someone incorrectly excludes Partial from ranking.

---

## Design Conformance

| ADR / Design Decision | Expected | Actual | Status |
|-----------------------|----------|--------|--------|
| Flat package (not sub-packages) | `engine/prespec/*.go` | `engine/prespec/grid.go`, `readiness.go` — no nested dirs | PASS |
| Stateless pure functions | No I/O, no persistence in Go | Confirmed — stdlib only, no side effects | PASS |
| sort.SliceStable for tie-break | Deterministic on equal scores | `sort.SliceStable` at grid.go:78 | PASS |
| ADR-5 Partial=0.5 formula | `(Clear + 0.5*Partial)/10` | readiness.go:35: exact formula | PASS |
| ReadinessGate exported | `const ReadinessGate = 0.6` | readiness.go:6 | PASS |
| Fail-loud on bad input | ADR-4 | Not applicable to PR-1 (dispatch in T-06, PR-2) | DEFERRED |

---

## Tasks Checklist State

- [x] T-01 — complete, committed, tested
- [x] T-02 — complete, committed, tested
- [x] T-03 — complete, committed, tested
- [ ] T-04 — deferred (PR-2)
- [ ] T-05 — deferred (PR-2)
- [ ] T-06 — deferred (PR-2)
- [ ] T-07 — deferred (PR-3)

Task state matches apply-progress.md and openspec tasks.md checkboxes.

---

## Verdict

**GO for PR-1.**

0 CRITICAL issues. 2 WARNINGs (both addressable in PR-2 without reopening this PR). 2 SUGGESTIONS (stylistic/traceability). The domain logic is correct, the tests are meaningful and cover all spec boundary conditions, coverage is 97.7%, and no regressions were introduced in existing packages.
