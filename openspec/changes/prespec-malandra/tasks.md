# Tasks: prespec-malandra

Generated: 2026-06-24  
Delivery strategy: `ask-on-risk`  
TDD mode: STRICT — tests written before implementation for every Go unit.

---

## Architecture note

The design is authoritative over the spec on two points:

1. **Package layout**: flat `engine/prespec/` (single package, not nested sub-packages).
   Files: `grid.go`, `readiness.go`, `lint.go`, `brief.go`, `prespec.go` + `*_test.go` + `testdata/`.
2. **Readiness formula**: `value = (Clear + 0.5 * Partial) / 10` (ADR-5).
   Partial contributes 0.5, not 0.0. This numerically encodes "Partial ≠ Clear" and is the
   authoritative formula. Spec R-009b (no partial credit) is superseded by the design's ADR-5.

---

## Dependency graph

```
T-01 (cell types)
  └─ T-02 (grid: rank, count, budget, stop)
       └─ T-03 (readiness: score, gate)
            └─ T-04 (lint: no-leading rules)
                 └─ T-05 (brief: schema, ULID, render, validate, TopicKey)
                      └─ T-06 (prespec.go orchestration + main.go dispatch + dispatch tests)
                           └─ T-07 (SKILL.md + coverage-taxonomy.md)
```

All tasks are **sequential** — each depends on the types and functions defined by the
previous task. No parallel execution.

---

## [x] T-01 — Cell types and state machine (test-first)

**Spec**: R-005a, R-015  
**Files**: `engine/prespec/grid.go` (CellState + Cell types only), `engine/prespec/grid_test.go`

### What to build

1. **Write tests first** (`engine/prespec/grid_test.go`):
   - Table test: `TestCellStateTransitions` — covers all valid and invalid transitions from
     R-015: Empty→Partial (valid), Empty→Clear (valid), Partial→Clear (valid),
     Clear→Partial (rejected, state unchanged, error returned), Clear→Empty (rejected).
   - Test: `TestCellDefinitions` — verifies exactly 10 cells are defined with the IDs,
     Impact values (1-5), and Uncertainty values (1-5) from R-005a table.

2. **Implement** `grid.go` (types only — no ranking yet):
   ```go
   type CellState int
   const ( Missing CellState = iota; Partial; Clear )

   type Cell struct {
       Key         string
       Impact      int
       Uncertainty int
       State       CellState
   }

   // SetState transitions the cell state; returns error on Clear→Partial or Clear→Empty.
   func (c *Cell) SetState(s CellState) error

   // DefaultCells returns the canonical 10-cell taxonomy in index order.
   func DefaultCells() []Cell
   ```

3. `DefaultCells()` returns cells in exact index order from R-005a:
   `jtbd-job` (I=5,U=5), `current-gap` (I=5,U=4), `why-now` (I=4,U=5),
   `user-segment` (I=4,U=3), `constraints` (I=3,U=4), `success-metric` (I=3,U=3),
   `alternatives` (I=3,U=3), `stakeholders` (I=2,U=3), `frequency` (I=2,U=2),
   `risk-unknowns` (I=2,U=4).

### Acceptance gate

`cd engine && go test ./prespec/ -run TestCellState -v` passes.  
`cd engine && go test ./prespec/ -run TestCellDefinitions -v` passes.

### Work-unit commit

```
feat(engine/prespec): cell type, state machine, and 10-cell taxonomy (test-first)
```

---

## [x] T-02 — Grid: rank, coverage count, budget, stop test (test-first)

**Spec**: R-005b, R-005c, R-006, R-016, R-019  
**Files**: `engine/prespec/grid.go` (Grid + functions), `engine/prespec/grid_test.go`

### What to build

1. **Write tests first** (extend `grid_test.go`):
   - `TestGridNew` — fresh grid has all 10 cells with State=Missing; grid has exactly 10 cells (R-005b).
   - `TestRankUncovered` — table test:
     - All-missing: returns all 10 cells sorted by Impact×Uncertainty desc, index asc as tie-break.
     - Mixed: jtbd-job (25) first, then current-gap (20, idx=1) before why-now (20, idx=2) (spec R-005c tie-break scenario).
     - Clear cells excluded from result (R-005c).
     - Fully-cleared grid returns empty slice.
   - `TestCoverageCount` — table test: fresh grid (0,0,10); mixed 3/2/5 grid (3,2,5) (R-016).
   - `TestBudgetRemaining` — table test: standard+asked=0→5, standard+asked=5→0,
     unknown mode→5 safe default (R-006).
   - `TestShouldStop` — table test:
     - budget fires first (asked=5, clear=3, signal=false) → (true, "budget-exhausted").
     - convergence fires (asked=3, clear=6, signal=false) → (true, "coverage-threshold").
     - user signal fires (asked=2, clear=4, signal=true) → (true, "user-signal").
     - no condition (asked=2, clear=4, signal=false) → (false, "").
     Priority: budget > convergence > user signal (R-019).

2. **Implement** in `grid.go`:
   ```go
   type Grid struct { Cells []Cell }

   func New() Grid                                            // 10 Missing cells (R-005b)
   func (g Grid) RankUncovered() []Cell                      // Impact×Uncertainty desc, index asc tie-break (R-005c)
   func CoverageCount(g Grid) (clear, partial, empty int)    // R-016
   func BudgetRemaining(asked int, mode string) int          // R-006a; unknown mode→5
   type StopReason string
   const ( StopBudget StopReason = "budget-exhausted"; StopConverged = "coverage-threshold"; StopUserSignal = "user-signal" )
   func ShouldStop(asked int, mode string, clear int, userSignal bool) (bool, StopReason)  // R-019
   ```

   RankUncovered must filter out `State == Clear` cells, compute `Impact * Uncertainty`
   as sort key, break ties by ascending cell index (slice index, NOT key string sort).
   Use `sort.SliceStable` for deterministic ordering.

### Acceptance gate

`cd engine && go test ./prespec/ -run TestGrid -v` passes.  
`cd engine && go test ./prespec/ -run TestRankUncovered -v` passes.  
`cd engine && go test ./prespec/ -run TestCoverage -v` passes.  
`cd engine && go test ./prespec/ -run TestBudget -v` passes.  
`cd engine && go test ./prespec/ -run TestShouldStop -v` passes.

### Work-unit commit

```
feat(engine/prespec): grid rank, coverage count, budget, and stop test (test-first)
```

---

## [x] T-03 — Readiness: score computation and 0.6 gate (test-first)

**Spec**: R-009, R-010  
**Files**: `engine/prespec/readiness.go`, `engine/prespec/readiness_test.go`

### What to build

1. **Write tests first** (`readiness_test.go`):
   - `TestReadinessCompute` — table test:
     - All 10 Clear → value=1.0, passes=true.
     - All 10 Missing → value=0.0, passes=false.
     - 5 Clear, 3 Partial, 2 Missing → value = (5 + 0.5×3)/10 = 0.65, passes=true (ADR-5 formula).
     - 10 Partial → value = (0 + 0.5×10)/10 = 0.5, passes=false (Partial-filled grid cannot pass gate).
     - 6 Clear, 4 Missing → value=0.6, passes=true (boundary inclusive).
     - 5 Clear, 4 Missing, 1 Partial → value = (5 + 0.5×1)/10 = 0.55, passes=false.
   - `TestReadinessGate` — table test (boundary tests for Score.Passes()):
     - value=0.6 → passes=true.
     - value=0.59 → passes=false.
     - value=0.0 → passes=false.
     - value=1.0 → passes=true.
     - value=0.5 → passes=false.
   - `TestReadinessGateConstantExported` — asserts ReadinessGate == 0.6 (spec OQ-4: must be exported).

2. **Implement** `readiness.go`:
   ```go
   const ReadinessGate = 0.6  // exported constant (R-010, OQ-4)

   type Score struct {
       Value      float64
       ClearCount int
       Total      int
   }

   // Readiness computes (Clear + 0.5*Partial) / Total. Total always 10.
   func Readiness(cells []Cell) Score

   // Passes returns true when Value >= ReadinessGate.
   func (s Score) Passes() bool
   ```

   Note: `Partial` weighted at 0.5 per ADR-5; `Missing` contributes 0.0.

### Acceptance gate

`cd engine && go test ./prespec/ -run TestReadiness -v` passes.

### Work-unit commit

```
feat(engine/prespec): readiness score and 0.6 gate with Partial=0.5 weight (test-first)
```

---

## [x] T-04 — Lint: no-leading question rules (test-first)

**Spec**: R-007  
**Files**: `engine/prespec/lint.go`, `engine/prespec/lint_test.go`

### What to build

1. **Write tests first** (`lint_test.go`):
   - `TestLintCleanPass` — "What happens after the current process breaks down?" → accepted=true.
   - `TestLintSmuggledAnswer` — "Would you say that adding automation would fix this?" →
     accepted=false, rule="smuggles-answer".
   - `TestLintPresupposesSolution` — "Should we build a REST API or a webhook?" →
     accepted=false, rule="presupposes-solution".
   - `TestLintBundledConcerns` — "Who experiences this problem and how often does it occur?" →
     accepted=false, rule="bundles-concerns".
   - `TestLintMultipleViolations` — question that trips both presupposes-solution and bundles-concerns;
     result returns first failing rule reason (LintResult carries first match per design).
   - Test cases for each rule's additional trigger patterns from the design:
     - Smuggled: "Don't you think...", "Isn't it true that...", "Wouldn't you rather..."
     - Presupposes: "dashboard", "API", "button", "page", "React", "Vue" without problem anchor.
     - Bundled: two `?` in one string, "and how / or what / and why" clause patterns.

2. **Implement** `lint.go` using deterministic string/regex checks (OQ-3 resolved: regex/keyword):
   ```go
   type LintResult struct {
       Accepted bool
       Reason   string  // empty when Accepted==true; first-match rule name otherwise
   }

   // Lint checks question against the three no-leading rules.
   // Returns first failing rule; Accepted=true if all pass.
   func Lint(question string) LintResult
   ```

   Rules implemented as compiled `regexp.MustCompile` patterns (package-level vars, zero alloc on call):
   - Rule 1 `smuggles-answer`: matches leading-frame phrases (case-insensitive).
   - Rule 2 `presupposes-solution`: matches solution-noun list without a prior problem anchor.
   - Rule 3 `bundles-concerns`: matches `?.*?` (two question marks) or conjunction-clause patterns.

   // minimal: rung-2 stdlib regexp — no fuzzy scoring needed; spec requires deterministic go test.

### Acceptance gate

`cd engine && go test ./prespec/ -run TestLint -v` passes.

### Work-unit commit

```
feat(engine/prespec): no-leading lint rules with regex patterns (test-first)
```

---

## [x] T-05 — Brief: ULID generation, schema, render, validate, TopicKey (test-first)

**Spec**: R-011, R-012, R-014, R-017, R-018  
**Files**: `engine/prespec/brief.go`, `engine/prespec/brief_test.go`,
           `engine/prespec/testdata/brief.golden`

### What to build

1. **Write tests first** (`brief_test.go`):

   **ULID tests** (R-014):
   - `TestNewID_Length` — `NewID()` returns exactly 26 characters.
   - `TestNewID_Alphabet` — return value matches `^[0-9A-HJKMNP-TV-Z]{26}$` (Crockford base32 uppercase).
   - `TestNewID_Uniqueness` — two sequential calls return distinct values.
   - `TestNewID_NotKebab` — return value does NOT match `^[a-z][a-z0-9-]+$`.
   - `TestNewIDFrom_Deterministic` — `NewIDFrom(fixedTime, fixedReader)` returns same value on two calls.

   **Brief schema tests** (R-012):
   - `TestBriefNoChangeName` — compile-time guard: use `reflect.TypeOf(Brief{})` to assert no field named
     "ChangeName", "ChangeSlug", or any variant. Field must be `DiscoveryID string`.

   **TopicKey tests** (R-017):
   - `TestTopicKey_ValidInputs` — `TopicKey("myproject", "01JXKM5F0G3PQRST4UVWXYZ00")` →
     `"project/myproject/prespec/01JXKM5F0G3PQRST4UVWXYZ00"`.
   - `TestTopicKey_SddPrefix_Panics` — `discoveryID = "sdd/..."` → panics.
   - `TestTopicKey_EmptyProject_Panics` — `project = ""` → panics.

   **Validate tests** (R-018):
   - `TestValidate_ValidBrief` — valid Brief passes (nil error).
   - `TestValidate_ScoreBelow06` — ReadinessScore=0.5 → error.
   - `TestValidate_TooManyAssumptions` — len(Assumptions)=4 → error.
   - `TestValidate_EmptyTranscript` — Transcript="" → error.
   - `TestValidate_EmptyDiscoveryID` — DiscoveryID="" → error.
   - `TestValidate_EmptyProject` — Project="" → error.
   - `TestValidate_EmptyJobStatement` — JobStatement="" → error.

   **Render golden test** (R-011b):
   - `TestRenderBrief_Golden` — inject fixed ULID + CreatedAt via `NewIDFrom`; compare
     `RenderBrief(b)` against `testdata/brief.golden`. Run with `-update` flag to regenerate.
   - `TestRenderBrief_Deterministic` — same Brief → byte-identical output on two calls.
   - `TestRenderBrief_AllSections` — rendered string contains headings for sections 1-6 and a
     transcript section.
   - `TestRenderBrief_AssumptionsCapped3` — `Validate(b)` with 4 Assumptions → error (R-011a).

2. **Create golden file skeleton** `engine/prespec/testdata/brief.golden` (placeholder; will be
   regenerated by `-update` run after implementation).

3. **Implement** `brief.go`:
   ```go
   type Brief struct {
       DiscoveryID     string     // ULID (never ChangeName — R-012)
       Project         string
       JobStatement    string
       Sections        [6]string  // sections 1-6
       Assumptions     []string   // <= 3, each "[ASSUMPTION] ..."
       Transcript      string
       ReadinessScore  float64
   }

   // NewID mints a fresh ULID using time.Now() and crypto/rand.
   func NewID() string

   // NewIDFrom mints a ULID from an injected timestamp (ms) and random reader.
   // Seam for golden tests.
   func NewIDFrom(tsMs int64, randReader io.Reader) string

   // TopicKey returns "project/{project}/prespec/{discoveryID}".
   // Panics if discoveryID begins with "sdd/" or project is empty.
   func TopicKey(project, discoveryID string) string

   // Validate returns an error if any Brief invariant is violated (R-018).
   func Validate(b Brief) error

   // RenderBrief returns deterministic Markdown from a Brief (R-011b).
   func RenderBrief(b Brief) string
   ```

   ULID generation (ADR-3 — hand-rolled, zero dep):
   - 6-byte big-endian timestamp (unix ms).
   - 10 bytes from `randReader` (or `crypto/rand.Read`).
   - Encode 16 bytes as 26 Crockford base32 characters (uppercase, design spec: lowercase in
     description but Crockford base32 spec and R-014 alphabet test `^[0-9A-HJKMNP-TV-Z]{26}$`
     use uppercase — implement uppercase to match the spec's regex acceptance scenario).
   // minimal: forced — ADR-3 zero-dep constraint; ~40 lines preferred over vendoring oklog/ulid.

### Acceptance gate

`cd engine && go test ./prespec/ -run TestNewID -v` passes.  
`cd engine && go test ./prespec/ -run TestBrief -v` passes.  
`cd engine && go test ./prespec/ -run TestTopicKey -v` passes.  
`cd engine && go test ./prespec/ -run TestValidate -v` passes.  
`cd engine && go test ./prespec/ -run TestRenderBrief -v` passes.

### Work-unit commit

```
feat(engine/prespec): Brief schema, ULID generation, render, validate, and TopicKey (test-first)
```

---

## [x] T-06 — Orchestration layer + subcommand dispatch + dispatch tests (test-first)

**Spec**: R-005 through R-019 (invocation seam), design §invocation seam  
**Files**: `engine/prespec/prespec.go`, `engine/prespec/prespec_test.go`,
           `engine/cmd/main.go` (add `prespec` case)

### What to build

1. **Write tests first** (`prespec_test.go`):
   - `TestDispatch_Rank` — injected stdin `{"cells":[...]}`, verb `rank` → stdout contains
     valid JSON with `ranked` array in Impact×Uncertainty desc order.
   - `TestDispatch_Lint` — injected stdin `{"question":"..."}`, verb `lint` → stdout contains
     `{"accepted":true}` for a clean question and `{"accepted":false,"reason":"..."}` for a
     flagged one.
   - `TestDispatch_Readiness` — injected stdin with 6-Clear grid → stdout `{"value":0.6,"passes":true,...}`.
   - `TestDispatch_Brief` — injected stdin with valid brief input →
     stdout contains `id` (26 chars), `markdown`, `path` (starts with `"project/"`).
   - `TestDispatch_UnknownVerb` — unknown verb → exit 1 + stderr message (fail loud).
   - `TestDispatch_MalformedJSON` — malformed stdin → exit 1 + stderr message.

   Tests use the `runPrespecCore(verb, stdin, stdout, stderr, exit)` injected-I/O pattern
   mirroring `gateTaskCore` / `runPropagateCore` in the existing codebase.

2. **Implement** `prespec.go` (thin orchestration layer):
   ```go
   // runPrespecCore is the testable dispatch for all prespec verbs.
   // verb: "rank" | "lint" | "readiness" | "brief"
   // Fails LOUD (exit 1) on bad verb or malformed JSON (ADR-4).
   func runPrespecCore(verb string, stdin io.Reader, stdout io.Writer, stderr io.Writer, exit func(int))
   ```

   Each verb decodes JSON from stdin, calls the matching package function, encodes response to stdout.
   JSON shapes per design §invocation seam:
   - `rank`:      `{"cells":[...]}` → `{"ranked":[{"key","score"}...]}`
   - `lint`:      `{"question":"..."}` → `{"accepted":bool,"reason":"..."}`
   - `readiness`: `{"cells":[...]}` → `{"value":0.0,"passes":bool,"clear":n,"total":10}`
   - `brief`:     `{"job","sections":[6],"assumptions":[...],"transcript"}` → `{"id","markdown","path"}`

   For the `brief` verb: mint ULID via `NewID()`, build `Brief`, call `Validate`, then `RenderBrief`.
   Return path as `"project/" + project + "/prespec/" + id` (project comes from the request payload).

3. **Wire dispatch in `engine/cmd/main.go`**:
   - Add `"prespec"` case to the `switch os.Args[1]` block.
   - `runPrespec(args)` → `runPrespecCore(args[0], os.Stdin, os.Stdout, os.Stderr, os.Exit)`
     (or parse verb from args[0]).
   - Update `usage()` to include `engine prespec <verb>`.
   - Update the package doc comment at the top of `main.go`.

### Acceptance gate

`cd engine && go test ./prespec/ -run TestDispatch -v` passes.  
`cd engine && go test ./... -v` passes (no regressions in existing subcommands).

### Work-unit commit

```
feat(engine): prespec subcommand dispatch wired to main.go (test-first)
```

---

## T-07 — SKILL.md authoring and coverage taxonomy reference

**Spec**: S-R-001 through S-R-006, R-001 through R-004, R-008, R-013  
**Files**: `skills/prespec-malandra/SKILL.md`,
           `skills/prespec-malandra/references/coverage-taxonomy.md`

### What to build

This task has no `go test` target. Verification is by scenario inspection against the
skill-level acceptance scenarios in the spec (Layer 2 section).

1. **`skills/prespec-malandra/SKILL.md`** — Socratic interview conductor skill:

   **Opening contract** (S-R-001):
   - First line: "You are a sub-agent. Do not delegate further."

   **Invocation sequence** (S-R-002):
   - Stage 0: Mom-Test past-behavior probe → if no response: 5-archetype MCQ fallback.
     Refusal floor: return `needs-more-input` if no actionable goal after both (R-002).
   - Stage 1: derive `[verb]+[object]+[context]` job statement; anti-solution bounce on
     premature solution input; bounded readback (job statement only, no strawman — S-R-003, R-004).
   - Stage 3: question loop (budget=5 standard mode):
     - Call `engine prespec rank < grid.json` → pick top uncovered cell.
     - Draft a question, call `engine prespec lint < question.json`.
     - If rejected: rewrite and re-lint (never ask a rejected question).
     - Record cell as Partial/Clear after user answers.
     - No-metric-yet escape for success-metric cell (R-013).
     - Call `engine prespec readiness < grid.json` after each answer to check stop test.
     - Call `engine prespec` `grid.ShouldStop` equivalent via readiness check; also check budget.
   - Stage 5: stop test:
     - Call `engine prespec readiness` for final score.
     - If score < 0.6: return `needs-more-input` with score and uncovered cells (no brief, no mem_save).
     - If score >= 0.6: proceed to brief emission.
   - Brief emission:
     - Synthesize sections 1-6 + transcript from conversation.
     - Emit at most 3 `[ASSUMPTION]`-marked items (cap 3 — R-008).
     - Call `engine prespec brief < briefinput.json`.
     - Call `mem_save` with topic_key from returned `path`, `capture_prompt: false`.
     - Never save under any `sdd/` key (S-R-004).
     - Never produce a change-name; redirect to requirements-from-transcripts if asked (S-R-005).
     - Never self-grade against an invented draft (S-R-006).

   **JSON call patterns** (document inline in SKILL.md for each stage):
   ```
   echo '{"cells":[...]}' | engine prespec rank
   echo '{"question":"..."}' | engine prespec lint
   echo '{"cells":[...]}' | engine prespec readiness
   echo '{"job":"...","sections":[...],"assumptions":[...],"transcript":"...","project":"..."}' | engine prespec brief
   ```

2. **`skills/prespec-malandra/references/coverage-taxonomy.md`** — reference document:
   - Full 10-cell table: ID, Impact (1-5), Uncertainty (1-5), description of what each cell covers.
   - Impact × Uncertainty ranking formula and tie-break rule (ascending index).
   - No-leading lint rejection checklist: three rules with one example each
     (smuggles-answer, presupposes-solution, bundles-concerns).
   - Stage 5 stop conditions in plain prose.

### Scenario inspection checklist (manual, before PR)

- [ ] Stage 0 cold-start probe scenario: first utterance is a Mom-Test probe, not a solution question.
- [ ] Stage 0 archetype MCQ fallback fires on non-answer.
- [ ] Stage 0 refusal floor: no job statement, no readback, no brief on total non-answer.
- [ ] Anti-solution bounce: "I want to add a Slack integration" → redirected to problem.
- [ ] Bounded readback: job statement only, no data model or entity list.
- [ ] Stage 3 lint check: candidate question goes through lint before being asked.
- [ ] Stage 3 budget enforcement: loop exits after 5 questions regardless of remaining cells.
- [ ] No-metric-yet escape: user "I don't know" on success-metric → cell stays Empty, no pressure.
- [ ] Gate failure: score=0.5 → needs-more-input, no mem_save, no brief, no change-name.
- [ ] Successful brief: score=0.7 → mem_save with `project/{project}/prespec/{ULID}`, `capture_prompt:false`.
- [ ] Assumption cap: at most 3 `[ASSUMPTION]` items in Section6.
- [ ] No sdd/ namespace usage anywhere in the skill.
- [ ] No change-name derived or suggested.

### Work-unit commit

```
feat(skills): prespec-malandra Socratic interview skill and coverage taxonomy reference
```

---

## Review Workload Forecast

```
Estimated changed lines: ~430
400-line budget risk: High
Chained PRs recommended: Yes
Decision needed before apply: Yes
```

**Budget breakdown (approximate)**:

| Task | Files | Estimated lines |
|------|-------|----------------|
| T-01 | grid.go (types) + grid_test.go (transitions + definitions) | ~80 |
| T-02 | grid.go (functions) + grid_test.go (rank/count/budget/stop) | ~110 |
| T-03 | readiness.go + readiness_test.go | ~60 |
| T-04 | lint.go + lint_test.go | ~80 |
| T-05 | brief.go + brief_test.go + testdata/brief.golden | ~120 |
| T-06 | prespec.go + prespec_test.go + main.go delta | ~90 |
| T-07 | SKILL.md + coverage-taxonomy.md | ~100 |
| **Total** | | **~640** |

At ~640 lines, this change exceeds the 400-line threshold. Per delivery_strategy `ask-on-risk`,
the orchestrator MUST stop before apply and ask whether to split into chained PRs. Suggested
natural seams for chaining:

- **PR-1**: T-01 + T-02 + T-03 (cell/grid/readiness — pure domain logic + tests, ~250 lines)
- **PR-2**: T-04 + T-05 + T-06 (lint/brief/dispatch + wiring, ~290 lines)
- **PR-3**: T-07 (SKILL.md + taxonomy reference, ~100 lines)

If user approves chained PRs, chain_strategy must be resolved before `sdd-apply`.
