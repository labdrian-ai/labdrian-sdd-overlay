# Specification: prespec-malandra

## Purpose

Delta specification for the `prespec-malandra` change. Defines WHAT must be true after
the change is applied, split across two implementation layers:

1. **`engine/prespec/` Go package** — deterministic mechanics verified by `go test`.
2. **`skills/prespec-malandra/SKILL.md`** — Socratic interview skill verified by scenario
   inspection and end-to-end review.

The proposal's assumption A-5 ("Markdown skill, no testable production code") is
OVERRIDDEN by the authoritative architecture in the spec phase. Deterministic behaviors
(grid scoring, ranking order, readiness gate, id generation, brief schema) MUST live in
the Go package so they are unit-testable, collision-resistant, and independent of LLM
non-determinism.

Requirements R-001 through R-013 are carried forward verbatim from the proposal and
assigned to the correct layer here. New sub-requirements (R-014..R-019) cover Go-package
specifics not explicit in the proposal.

---

## Scope Boundaries

### In scope (MVP / first slice)

- Go package `engine/prespec/` with the sub-packages listed below.
- Markdown skill `skills/prespec-malandra/SKILL.md` (delegate-only).
- Coverage taxonomy reference `skills/prespec-malandra/references/coverage-taxonomy.md`.
- Hardcoded `standard` mode — question budget = 5. No tier selection.
- Brief artifact `project/{project}/prespec/{discovery-id}` with ULID `discovery-id`.

### Out of scope (explicit non-goals for this slice)

- Tier-based mode selection (`quick` / `deep`).
- Security → `High` auto-promotion of coverage cells.
- Enforced `sdd-propose` crosswalk (7-of-10 pre-fill).
- `sdd-explore` auto-read of the brief.
- Public OSS packaging.
- Modification of any existing skill, engine phase, or topic-key owner.

---

## Layer 1 — Go Package `engine/prespec/`

### Package Structure

```
engine/prespec/
  cell/          cell IDs, impact and uncertainty weights, Clear/Partial/Empty states
  grid/          10-cell coverage grid: init, update, rank by Impact×Uncertainty
  score/         readiness score calculation and 0.6 gate
  lint/          no-leading question lint (rejection checklist)
  brief/         brief schema (sections 1-6 + transcript) and text renderer
  id/            ULID generation for discovery-id
```

Each sub-package is independently unit-testable. The skill calls into this package for
every deterministic step; the LLM handles only natural-language generation.

---

### R-001 — Cold-Start Probe (Skill Layer)

**Layer**: Skill  
**EARS**: WHEN `prespec-malandra` is invoked with a vague or empty idea, the engine SHALL
run Stage 0 and emit a Mom-Test past-behavior probe before any solution-shaped question.

This requirement governs the skill's conversational behavior. It is not testable as a Go
unit; coverage is by scenario inspection.

---

### R-002 — Refusal Floor (Skill Layer)

**Layer**: Skill  
**EARS**: IF Stage 0 cannot elicit any goal after the probe and the 5-archetype MCQ
fallback, THEN the engine SHALL return `needs-more-input` and SHALL NOT fabricate a goal.

Coverage by scenario inspection.

---

### R-003 — Job Statement + Anti-Solution Bounce (Skill Layer)

**Layer**: Skill  
**EARS**: The engine SHALL produce a `[verb] + [object] + [context]` job statement in
Stage 1 and SHALL redirect any premature solution input back to the problem.

Coverage by scenario inspection.

---

### R-004 — Bounded Readback (Skill Layer)

**Layer**: Skill  
**EARS**: The Stage 1 readback SHALL reframe the job statement ONLY and SHALL NOT invent
a data model, scenario, schema, or other strawman artifact.

Coverage by scenario inspection.

---

### R-005 — Grid Init + Impact×Uncertainty Ranking (Go Package)

**Layer**: `engine/prespec/grid/` + `engine/prespec/cell/`  
**EARS**: The engine SHALL initialize the 10-cell coverage grid and SHALL rank uncovered
cells by Impact × Uncertainty before selecting each Stage 3 question.

#### Sub-requirement R-005a — Cell Definitions

The `engine/prespec/cell` package SHALL define exactly 10 cells:

| Index | ID | Impact (1-5) | Uncertainty (1-5) |
|---|---|---|---|
| 0 | `jtbd-job` | 5 | 5 |
| 1 | `current-gap` | 5 | 4 |
| 2 | `why-now` | 4 | 5 |
| 3 | `user-segment` | 4 | 3 |
| 4 | `constraints` | 3 | 4 |
| 5 | `success-metric` | 3 | 3 |
| 6 | `alternatives` | 3 | 3 |
| 7 | `stakeholders` | 2 | 3 |
| 8 | `frequency` | 2 | 2 |
| 9 | `risk-unknowns` | 2 | 4 |

Each cell carries a string `ID`, integer `Impact` (1-5), integer `Uncertainty` (1-5),
and a `State` of `Empty | Partial | Clear`.

#### Sub-requirement R-005b — Grid Initialization

WHEN the grid is initialized, each cell SHALL have `State = Empty`.

#### Sub-requirement R-005c — Impact×Uncertainty Ranking

WHEN the grid is ranked, the package SHALL return uncovered cells (those with
`State != Clear`) sorted descending by `Impact × Uncertainty`. Ties broken by cell index
ascending (stable, predictable).

#### Acceptance Scenarios (go test)

**Scenario: Fresh grid has all cells Empty**
- GIVEN `grid.New()` is called
- WHEN the caller reads each cell's state
- THEN all 10 cells have `State = Empty`
- AND the grid contains exactly 10 cells

**Scenario: Rank returns non-Clear cells in I×U descending order**
- GIVEN a grid where cells `jtbd-job` (I=5, U=5, score=25) and `why-now` (I=4, U=5,
  score=20) are Empty, and `current-gap` (I=5, U=4, score=20) is Partial
- WHEN `grid.Rank()` is called
- THEN the returned slice is [`jtbd-job`, `why-now`, `current-gap`] (score 25, 20, 20;
  tie on 20 broken by index: why-now index 2 before current-gap index 1 — NO: by ascending
  index, current-gap=1 comes before why-now=2, so order is `jtbd-job`, `current-gap`,
  `why-now`)
- AND `current-gap` appears before `why-now` due to ascending-index tie-break

**Scenario: Rank excludes Clear cells**
- GIVEN a grid where `jtbd-job` is `Clear`
- WHEN `grid.Rank()` is called
- THEN the returned slice does NOT contain `jtbd-job`

**Scenario: Rank on fully-cleared grid returns empty slice**
- GIVEN a grid where all 10 cells are `Clear`
- WHEN `grid.Rank()` is called
- THEN the returned slice has length 0

---

### R-006 — One Question Per Iteration, Budget = 5 (Go Package + Skill)

**Layer**: `engine/prespec/grid/` (budget tracking); Skill (question emission)  
**EARS**: The engine SHALL ask at most ONE question per Stage 3 iteration and SHALL stop
the loop at the question budget of 5 in `standard` mode.

#### Sub-requirement R-006a — Budget Tracking

The `engine/prespec/grid` package SHALL expose a `BudgetRemaining(asked int, mode string) int`
function returning `budget - asked` where `budget` for `"standard"` mode is 5.

#### Acceptance Scenarios (go test)

**Scenario: BudgetRemaining with standard mode**
- GIVEN mode = "standard", asked = 0
- WHEN `BudgetRemaining(0, "standard")` is called
- THEN result is 5

**Scenario: Budget exhausted**
- GIVEN mode = "standard", asked = 5
- WHEN `BudgetRemaining(5, "standard")` is called
- THEN result is 0

**Scenario: Unknown mode falls back to standard**
- GIVEN mode = "quick" (not yet defined in MVP)
- WHEN `BudgetRemaining(0, "quick")` is called
- THEN result is 5 (safe default, not a panic)

---

### R-007 — No-Leading Lint (Go Package)

**Layer**: `engine/prespec/lint/`  
**EARS**: Before asking any question, the engine SHALL apply the no-leading lint and SHALL
reject any question that smuggles the answer, presupposes a solution, or bundles concerns.

#### Sub-requirement R-007a — Lint Function Contract

`engine/prespec/lint` SHALL expose:

```go
type Violation struct {
    Rule    string // "smuggles-answer" | "presupposes-solution" | "bundles-concerns"
    Detail  string
}

func Check(question string) []Violation
```

`Check` returns nil (or empty slice) if the question passes all rules, or one or more
`Violation` entries if any rule fires.

#### Sub-requirement R-007b — Lint Rules

The lint SHALL implement at least these three rules:

| Rule ID | Trigger | Example pattern |
|---|---|---|
| `smuggles-answer` | Question contains a presupposed answer embedded in its phrasing (e.g., "Would you agree that X would fix this?") | "Would you say that a dashboard would help?" |
| `presupposes-solution` | Question names a specific technology, product, or implementation approach before the job is established | "Should we use React or Vue?" |
| `bundles-concerns` | Question contains two or more distinct information-seeking intents joined by "and", "or", or "also" | "Who uses this and how often?" |

#### Acceptance Scenarios (go test)

**Scenario: Clean question passes lint**
- GIVEN question = "What happens after the current process breaks down?"
- WHEN `lint.Check(question)` is called
- THEN result is empty (no violations)

**Scenario: Smuggled-answer question is rejected**
- GIVEN question = "Would you say that adding automation would fix this?"
- WHEN `lint.Check(question)` is called
- THEN result contains a violation with Rule = "smuggles-answer"

**Scenario: Solution-presupposing question is rejected**
- GIVEN question = "Should we build a REST API or a webhook?"
- WHEN `lint.Check(question)` is called
- THEN result contains a violation with Rule = "presupposes-solution"

**Scenario: Bundled question is rejected**
- GIVEN question = "Who experiences this problem and how often does it occur?"
- WHEN `lint.Check(question)` is called
- THEN result contains a violation with Rule = "bundles-concerns"

**Scenario: Multiple violations reported**
- GIVEN question = "Should we use Slack and would that fix the workflow and reduce errors?"
- WHEN `lint.Check(question)` is called
- THEN result contains violations for both "presupposes-solution" and "bundles-concerns"

---

### R-008 — Bounded Informed Guess (Skill Layer)

**Layer**: Skill  
**EARS**: WHERE a coverage cell remains unresolved at the budget, the engine MAY emit a
bounded informed guess marked `[ASSUMPTION]`, capped at 3 assumptions, each justified
against the job statement.

The cap of 3 is enforced by the skill instruction. Coverage by scenario inspection.

---

### R-009 — Readiness Score + Partial ≠ Clear (Go Package)

**Layer**: `engine/prespec/score/`  
**EARS**: The engine SHALL run a Stage 5 stop test and compute a readiness score; a
`Partial` cell SHALL NOT be counted as `Clear`.

#### Sub-requirement R-009a — Score Calculation

`engine/prespec/score` SHALL expose:

```go
func Compute(cells []cell.Cell) float64
```

The score is the fraction of cells with `State == Clear` over the total cell count (10).
`Partial` cells contribute 0.0 to the numerator. The denominator is always 10.

Formula: `score = count(Clear) / 10.0`

#### Sub-requirement R-009b — Partial Cell Non-Contribution

A cell with `State = Partial` MUST NOT be treated as contributing any fractional score.
The score computation has no partial-credit path.

#### Acceptance Scenarios (go test)

**Scenario: All cells Clear yields score 1.0**
- GIVEN all 10 cells have `State = Clear`
- WHEN `score.Compute(cells)` is called
- THEN result is 1.0

**Scenario: No cells Clear yields score 0.0**
- GIVEN all 10 cells have `State = Empty` or `State = Partial`
- WHEN `score.Compute(cells)` is called
- THEN result is 0.0

**Scenario: Partial cells do not count as Clear**
- GIVEN 5 cells are `Clear`, 3 cells are `Partial`, 2 cells are `Empty`
- WHEN `score.Compute(cells)` is called
- THEN result is 0.5 (5/10, NOT 0.8 or 0.5+fraction)

**Scenario: 6 of 10 Clear yields exactly 0.6**
- GIVEN 6 cells are `Clear`, 4 cells are `Empty`
- WHEN `score.Compute(cells)` is called
- THEN result is 0.6

**Scenario: Score is invariant to cell order**
- GIVEN 6 cells are `Clear` in any order within the slice
- WHEN `score.Compute(cells)` is called
- THEN result is 0.6 regardless of which 6 cells are `Clear`

---

### R-010 — Readiness Gate at 0.6 (Go Package)

**Layer**: `engine/prespec/score/`  
**EARS**: IF the readiness score is `< 0.6`, THEN the engine SHALL return
`needs-more-input` and SHALL NOT emit a brief.

#### Sub-requirement R-010a — Gate Function

`engine/prespec/score` SHALL expose:

```go
type GateResult string

const (
    GatePass GateResult = "pass"
    GateReject GateResult = "needs-more-input"
)

func Gate(score float64) GateResult
```

`Gate` returns `GatePass` when `score >= 0.6`, `GateReject` when `score < 0.6`.

#### Acceptance Scenarios (go test)

**Scenario: Score exactly 0.6 passes the gate**
- GIVEN score = 0.6
- WHEN `score.Gate(0.6)` is called
- THEN result is `GatePass`

**Scenario: Score below 0.6 is rejected**
- GIVEN score = 0.5
- WHEN `score.Gate(0.5)` is called
- THEN result is `GateReject`

**Scenario: Score of 0.0 is rejected**
- GIVEN score = 0.0
- WHEN `score.Gate(0.0)` is called
- THEN result is `GateReject`

**Scenario: Score of 1.0 passes**
- GIVEN score = 1.0
- WHEN `score.Gate(1.0)` is called
- THEN result is `GatePass`

**Scenario: Score 0.59 is rejected (boundary)**
- GIVEN score = 0.59
- WHEN `score.Gate(0.59)` is called
- THEN result is `GateReject`

---

### R-011 — Brief Artifact Schema + Persistence (Go Package + Skill)

**Layer**: `engine/prespec/brief/` (schema + render); Skill (invokes mem_save)  
**EARS**: WHEN the readiness gate passes, the engine SHALL persist the brief (sections 1-6
+ transcript) to `project/{project}/prespec/{discovery-id}` using a ULID identifier and
`capture_prompt: false`.

#### Sub-requirement R-011a — Brief Schema

`engine/prespec/brief` SHALL define a `Brief` struct with at minimum these fields:

| Field | Type | Description |
|---|---|---|
| `DiscoveryID` | string | ULID identifier for this prespec run |
| `Project` | string | Project name passed by the skill |
| `JobStatement` | string | `[verb] + [object] + [context]` from Stage 1 |
| `Section1` | string | Summary / context |
| `Section2` | string | Job statement and current-state gap |
| `Section3` | string | User segment and stakeholders |
| `Section4` | string | Success criteria (or `no-metric-yet` flag) |
| `Section5` | string | Constraints and alternatives considered |
| `Section6` | string | Open questions and `[ASSUMPTION]` list |
| `Transcript` | string | Full synthetic transcript of the interview |
| `Assumptions` | []string | Up to 3 `[ASSUMPTION]`-marked items |
| `ReadinessScore` | float64 | Computed score from `score.Compute` |

#### Sub-requirement R-011b — Brief Renderer

`engine/prespec/brief` SHALL expose:

```go
func Render(b Brief) string
```

`Render` returns a Markdown string containing sections 1-6 labelled by heading, followed
by the full transcript. The output is deterministic given the same `Brief` input.

#### Sub-requirement R-011c — Persistence Contract

The brief SHALL be saved via `mem_save` with:
- `topic_key`: `project/{project}/prespec/{discovery-id}`
- `capture_prompt`: `false`
- `type`: `"architecture"` (or a project-defined artifact type)
- The `discovery-id` MUST be the ULID from the `Brief.DiscoveryID` field.

The brief SHALL NOT be saved under any key beginning with `sdd/`.

#### Acceptance Scenarios (go test)

**Scenario: Render includes all six section headings**
- GIVEN a `Brief` with non-empty Section1..Section6 and Transcript
- WHEN `brief.Render(b)` is called
- THEN the returned string contains headings for all six sections and a transcript section

**Scenario: Render is deterministic**
- GIVEN the same `Brief` value is passed twice
- WHEN `brief.Render(b)` is called both times
- THEN both return values are byte-identical

**Scenario: Assumptions capped at 3**
- GIVEN a `Brief` with 4 items in Assumptions
- WHEN the brief is validated
- THEN validation returns an error (more than 3 assumptions violates R-008)

---

### R-012 — No Change-Name Derivation (Skill + Go Package — Red Line)

**Layer**: Skill + `engine/prespec/brief/`  
**EARS**: The engine SHALL NOT derive a change-name; change-name derivation remains the
exclusive responsibility of `requirements-from-transcripts`.

The `Brief` struct SHALL NOT contain a `ChangeName` field. The id field SHALL be named
`DiscoveryID` and its type SHALL be a ULID string — a format that is structurally
incompatible with a kebab-case change-name and cannot be confused for one.

#### Acceptance Scenario (go test)

**Scenario: Brief struct has no ChangeName field**
- GIVEN the `brief.Brief` type definition
- WHEN a Go compiler processes the type
- THEN there is no exported or unexported field named `ChangeName`, `ChangeSlug`, or any
  variant that could be confused with a requirements-from-transcripts change-name

---

### R-013 — No-Metric-Yet Escape (Skill Layer)

**Layer**: Skill  
**EARS**: WHERE a metric or success measure cannot be elicited, the engine SHALL offer a
`no-metric-yet` escape and SHALL NOT force the user to invent a metric.

Coverage by scenario inspection.

---

### R-014 — ULID Discovery-ID (Go Package — New)

**Layer**: `engine/prespec/id/`

The `engine/prespec/id` package SHALL expose:

```go
func New() string
```

`New()` returns a valid ULID string (26-character base32-encoded, Crockford alphabet,
monotonically increasing within a millisecond). The returned value SHALL NOT be
kebab-case, SHALL NOT match the pattern `[a-z][a-z0-9-]*[a-z0-9]` (kebab slug), and
SHALL NOT begin with `sdd/`.

#### Acceptance Scenarios (go test)

**Scenario: Generated ID is 26 characters**
- GIVEN `id.New()` is called
- WHEN the return value is inspected
- THEN it has exactly 26 characters

**Scenario: Generated ID uses Crockford base32 alphabet only**
- GIVEN `id.New()` is called
- WHEN the return value is matched against `^[0-9A-HJKMNP-TV-Z]{26}$`
- THEN the match succeeds

**Scenario: Two sequential calls produce distinct IDs**
- GIVEN `id.New()` is called twice in succession
- WHEN both return values are compared
- THEN they are not equal

**Scenario: ULID is not a kebab-case slug**
- GIVEN `id.New()` is called
- WHEN the return value is checked against the kebab pattern `^[a-z][a-z0-9-]+$`
- THEN the match FAILS (ULIDs contain uppercase letters and no hyphens)

---

### R-015 — Cell State Transitions (Go Package — New)

**Layer**: `engine/prespec/cell/`

Cells SHALL support three states: `Empty`, `Partial`, `Clear`. State transitions SHALL
follow this contract:

- `Empty` → `Partial` is valid.
- `Empty` → `Clear` is valid.
- `Partial` → `Clear` is valid.
- `Clear` → `Partial` SHALL be rejected (a cleared cell cannot degrade).
- `Clear` → `Empty` SHALL be rejected.

#### Acceptance Scenarios (go test)

**Scenario: Empty to Partial is valid**
- GIVEN a cell with State = Empty
- WHEN `cell.SetState(Partial)` is called
- THEN State = Partial and no error is returned

**Scenario: Partial to Clear is valid**
- GIVEN a cell with State = Partial
- WHEN `cell.SetState(Clear)` is called
- THEN State = Clear and no error is returned

**Scenario: Clear to Partial is rejected**
- GIVEN a cell with State = Clear
- WHEN `cell.SetState(Partial)` is called
- THEN an error is returned and State remains Clear

---

### R-016 — Grid Coverage Count (Go Package — New)

**Layer**: `engine/prespec/grid/`

`engine/prespec/grid` SHALL expose:

```go
func CoverageCount(g Grid) (clear int, partial int, empty int)
```

The function returns counts by state. These counts feed the Stage 5 stop test.

#### Acceptance Scenarios (go test)

**Scenario: Fresh grid returns 0 clear, 0 partial, 10 empty**
- GIVEN a newly initialized grid
- WHEN `grid.CoverageCount(g)` is called
- THEN (clear=0, partial=0, empty=10)

**Scenario: Mixed state grid is counted correctly**
- GIVEN a grid with 3 Clear, 2 Partial, 5 Empty cells
- WHEN `grid.CoverageCount(g)` is called
- THEN (clear=3, partial=2, empty=5)

---

### R-017 — Namespace Guard (Go Package — New)

**Layer**: `engine/prespec/brief/`

The `engine/prespec/brief` package SHALL expose:

```go
func TopicKey(project, discoveryID string) string
```

`TopicKey` returns `"project/" + project + "/prespec/" + discoveryID`.

The function SHALL panic (or return an error if the signature is changed to
`(string, error)`) if `discoveryID` begins with `sdd/` or if `project` is empty.

#### Acceptance Scenarios (go test)

**Scenario: Valid inputs produce correct key**
- GIVEN project = "myproject", discoveryID = "01JXKM5F0G3PQRST4UVWXYZ00"
- WHEN `brief.TopicKey("myproject", "01JXKM5F0G3PQRST4UVWXYZ00")` is called
- THEN result is `"project/myproject/prespec/01JXKM5F0G3PQRST4UVWXYZ00"`

**Scenario: sdd/ prefixed discoveryID is rejected**
- GIVEN discoveryID = "sdd/some-change/foo"
- WHEN `brief.TopicKey("myproject", "sdd/some-change/foo")` is called
- THEN the call panics (or returns a non-nil error)

**Scenario: Empty project is rejected**
- GIVEN project = ""
- WHEN `brief.TopicKey("", "01JXKM5F0G3PQRST4UVWXYZ00")` is called
- THEN the call panics (or returns a non-nil error)

---

### R-018 — Brief Validation (Go Package — New)

**Layer**: `engine/prespec/brief/`

`engine/prespec/brief` SHALL expose:

```go
func Validate(b Brief) error
```

`Validate` returns an error if any of these conditions are true:
- `DiscoveryID` is empty or is not a valid ULID (26 chars, Crockford alphabet).
- `Project` is empty.
- `JobStatement` is empty.
- `Transcript` is empty.
- `len(Assumptions) > 3`.
- `ReadinessScore < 0.6` (a brief that failed the gate must not be persisted).

#### Acceptance Scenarios (go test)

**Scenario: Valid brief passes validation**
- GIVEN a Brief with valid ULID, non-empty Project, JobStatement, Transcript,
  ReadinessScore = 0.7, and 0 Assumptions
- WHEN `brief.Validate(b)` is called
- THEN error is nil

**Scenario: Score below gate is invalid**
- GIVEN a Brief with ReadinessScore = 0.5 and otherwise valid fields
- WHEN `brief.Validate(b)` is called
- THEN error is non-nil

**Scenario: More than 3 assumptions is invalid**
- GIVEN a Brief with 4 items in Assumptions
- WHEN `brief.Validate(b)` is called
- THEN error is non-nil

**Scenario: Empty transcript is invalid**
- GIVEN a Brief with Transcript = ""
- WHEN `brief.Validate(b)` is called
- THEN error is non-nil

---

### R-019 — Grid Stop Test (Go Package — New)

**Layer**: `engine/prespec/grid/`

`engine/prespec/grid` SHALL expose:

```go
type StopReason string

const (
    StopBudget      StopReason = "budget-exhausted"
    StopConverged   StopReason = "coverage-threshold"
    StopUserSignal  StopReason = "user-signal"
    StopDiminishing StopReason = "diminishing-returns"
)

func ShouldStop(asked int, mode string, clear int, userSignal bool) (bool, StopReason)
```

`ShouldStop` returns `(true, reason)` when any stop condition is met:
- Budget: `asked >= budget(mode)` → `StopBudget`
- Convergence: `clear >= 6` (60% of 10 cells) → `StopConverged`
- User signal: `userSignal == true` → `StopUserSignal`

Priority: budget check runs first, then convergence, then user signal. Diminishing-returns
(`StopDiminishing`) is emitted by the skill layer, not the Go package (deferred to v2
automated detection).

#### Acceptance Scenarios (go test)

**Scenario: Budget exhausted fires first**
- GIVEN asked = 5, mode = "standard", clear = 3, userSignal = false
- WHEN `grid.ShouldStop(5, "standard", 3, false)` is called
- THEN result is (true, "budget-exhausted")

**Scenario: Convergence fires when coverage threshold reached**
- GIVEN asked = 3, mode = "standard", clear = 6, userSignal = false
- WHEN `grid.ShouldStop(3, "standard", 6, false)` is called
- THEN result is (true, "coverage-threshold")

**Scenario: User signal fires when budget not exhausted and threshold not reached**
- GIVEN asked = 2, mode = "standard", clear = 4, userSignal = true
- WHEN `grid.ShouldStop(2, "standard", 4, true)` is called
- THEN result is (true, "user-signal")

**Scenario: No stop condition returns false**
- GIVEN asked = 2, mode = "standard", clear = 4, userSignal = false
- WHEN `grid.ShouldStop(2, "standard", 4, false)` is called
- THEN result is (false, "")

---

## Layer 2 — Markdown Skill `skills/prespec-malandra/SKILL.md`

The skill is a delegate-only Socratic interview conductor. It calls into `engine/prespec/`
for deterministic steps (grid ranking, lint check, score computation, brief rendering,
topic-key generation, ULID id generation). It handles natural-language stages and the
conversation flow.

### Skill-Level Requirements

#### S-R-001 — Delegate-Only Invocation

The skill SHALL be invocable exclusively as a delegate (sub-agent). It SHALL NOT be
callable inline by the orchestrator's main context. Its first instruction SHALL be:
"You are a sub-agent. Do not delegate further."

#### S-R-002 — Stage Sequence

The skill SHALL execute stages in this order: Stage 0 → Stage 1 → Stage 3 → Stage 5 →
Brief emission. The skill SHALL NOT skip Stage 0, Stage 1, or Stage 5.

#### S-R-003 — No Strawman Artifacts

The skill SHALL NOT produce, generate, or suggest: data models, database schemas, UI
wireframes, API contracts, or scenario narratives. The Stage 1 readback is the ONLY
reframing artifact and it is bounded to the job statement.

#### S-R-004 — No sdd/ Namespace

The skill SHALL NOT save any artifact under a topic key beginning with `sdd/`. All
prespec artifacts go to `project/{project}/prespec/{discovery-id}`.

#### S-R-005 — No Change-Name Derivation

The skill SHALL NOT produce, suggest, or emit a kebab-case change-name. If the user asks
for a change-name, the skill SHALL redirect: "Change-name derivation is the responsibility
of requirements-from-transcripts. Run it with this brief as input."

#### S-R-006 — No Self-Grading Against Invented Draft

The skill SHALL NOT fabricate a partial spec, strawman PRD, or any synthetic artifact for
the purpose of grading the coverage grid against it. Coverage is measured against elicited
user responses only.

### Skill-Level Acceptance Scenarios (scenario inspection)

#### Scenario: Stage 0 cold-start probe

- GIVEN prespec-malandra is invoked with input "I want to improve onboarding"
- WHEN the skill begins Stage 0
- THEN the first utterance is a Mom-Test past-behavior probe (e.g., "Tell me about the
  last time onboarding frustrated someone on your team")
- AND it is NOT a solution-shaped question (e.g., "Should we build a wizard?")

#### Scenario: Stage 0 fallback to archetype MCQ

- GIVEN the user cannot answer the past-behavior probe freely (e.g., "I don't know")
- WHEN the skill applies the Stage 0 fallback
- THEN it presents a 5-archetype pain MCQ (e.g., "Which best describes what's frustrating
  you: A) too slow, B) too many errors, C) too hard to learn, D) too many steps,
  E) other")
- AND it does NOT fabricate a goal from the non-answer

#### Scenario: Stage 0 refusal floor

- GIVEN the user provides no actionable goal after the probe and MCQ
- WHEN Stage 0 concludes
- THEN the skill returns `needs-more-input` with an explanation
- AND it does NOT emit a job statement, readback, or brief

#### Scenario: Anti-solution bounce

- GIVEN the user says "I want to add a Slack integration" during Stage 1
- WHEN the skill processes this input
- THEN the skill redirects: asks what problem the Slack integration would solve
- AND it does NOT accept the solution framing as the job statement

#### Scenario: Bounded readback is job-only

- GIVEN Stage 1 has produced a job statement "[verb: streamline] + [object: weekly
  reporting] + [context: for project managers]"
- WHEN the skill emits the Stage 1 readback
- THEN the readback contains only a restatement of that job in plain language
- AND it does NOT contain a data model, entity list, schema, or scenario narrative

#### Scenario: Stage 3 respects no-leading lint

- GIVEN the grid rank places `constraints` as the top uncovered cell
- WHEN the skill formulates a question about constraints
- THEN it calls the lint check before asking
- AND if the candidate question fails lint, the skill reformulates before asking

#### Scenario: Stage 3 budget enforcement

- GIVEN the skill has asked 5 questions (budget = 5 in standard mode)
- WHEN Stage 3 evaluates whether to continue
- THEN the loop stops and Stage 5 is entered, regardless of remaining uncovered cells

#### Scenario: No-metric-yet escape

- GIVEN the skill reaches the success-metric cell and the user says "I have no idea how
  to measure this"
- WHEN the skill handles this response
- THEN it records the cell as Empty with a no-metric-yet note
- AND it does NOT press the user to invent a metric or suggest one as a leading option

#### Scenario: Gate failure yields no brief

- GIVEN Stage 5 computes a readiness score of 0.5 (5 Clear cells out of 10)
- WHEN the gate is applied
- THEN the skill returns `needs-more-input` with the score and which cells remain uncovered
- AND no brief is written, no `mem_save` is called, no topic key is created

#### Scenario: Successful brief emission

- GIVEN Stage 5 computes a readiness score of 0.7 (7 Clear cells out of 10)
- WHEN the gate passes
- THEN the skill constructs a Brief, calls `brief.Validate`, renders via `brief.Render`
- AND calls `mem_save` with topic_key = `project/{project}/prespec/{ULID}` and
  `capture_prompt: false`
- AND the ULID identifier is returned to the user

#### Scenario: Assumption cap enforcement

- GIVEN 3 cells remain uncovered at budget exhaustion
- WHEN the skill emits informed guesses
- THEN at most 3 `[ASSUMPTION]`-marked items appear in Section6
- AND each assumption is justified against the job statement, not invented from context

---

## Coverage Taxonomy Reference

The `skills/prespec-malandra/references/coverage-taxonomy.md` file SHALL define:

- The 10-cell grid with cell IDs, Impact weights, Uncertainty weights, and a description
  of what each cell covers.
- The Impact × Uncertainty ranking formula and tie-break rule.
- The no-leading lint rejection checklist (three rules: smuggles-answer,
  presupposes-solution, bundles-concerns) with an example for each.
- The Stage 5 stop test conditions in plain prose.

---

## File Structure (Post-Apply)

```
engine/prespec/
  cell/
    cell.go          Cell type, State enum, SetState
    cell_test.go     State transition scenarios
  grid/
    grid.go          Grid type, New, Rank, CoverageCount, BudgetRemaining, ShouldStop
    grid_test.go     Rank, count, budget, stop scenarios
  score/
    score.go         Compute, Gate, GateResult
    score_test.go    Score and gate scenarios
  lint/
    lint.go          Violation type, Check function
    lint_test.go     Lint rule scenarios
  brief/
    brief.go         Brief struct, Render, Validate, TopicKey
    brief_test.go    Schema, render, validate, namespace guard scenarios
  id/
    id.go            New() string — ULID generation
    id_test.go       ULID format, uniqueness scenarios

skills/prespec-malandra/
  SKILL.md
  references/
    coverage-taxonomy.md
```

---

## Red Lines (Encoded as Hard Requirements)

All five red lines from the proposal are encoded as TESTABLE or INSPECTABLE requirements:

| Red Line | Encoding |
|---|---|
| Never derive change-name | R-012: `Brief` struct has no `ChangeName` field; ULID is structurally non-kebab |
| Never invent strawman | R-004, S-R-003: job-only readback; no data models, schemas, scenarios |
| Never self-grade against invented draft | S-R-006: coverage measured against elicited responses only |
| Never force a metric | R-013, S-R-0xx scenario: no-metric-yet escape required |
| Never squat sdd/ namespace | R-011c, R-017, S-R-004: TopicKey enforces prefix; namespace guard panics on violation |

---

## Non-Goals Reaffirmed

Requirements and scenarios in this spec are scoped to the MVP slice. The following are
NOT requirements for this change and will not be verified:

- Tier selection (quick / deep modes).
- Security-domain auto-promotion of coverage cells.
- Enforced sdd-propose crosswalk.
- sdd-explore auto-read.
- OSS packaging.
- Modification of requirements-from-transcripts, sdd-explore, or any engine phase.

---

## Open Questions (Spec-Level Assumptions)

**OQ-1 (from A-2)**: Does `requirements-from-transcripts` ingest the brief's synthetic
transcript without a brief-aware adapter? Spec assumes YES (AC-2 coverage). If no,
design must add a brief-aware hint field to the Brief schema or a thin adapter skill.
This is the highest-risk open question.

**OQ-2 (from A-1)**: ULID generation is assigned to `engine/prespec/id/`. Design must
confirm the Go ULID library choice (e.g., `github.com/oklog/ulid/v2`) and whether it
meets the "collision-resistant, non-kebab, monotonic within ms" contract.

**OQ-3 (R-007, lint rules)**: The three lint rules are pattern-based. Design must decide
whether the lint uses regex patterns, heuristic scoring, or an LLM sub-call. The spec
requires deterministic `go test`-verifiable behavior; design must choose an approach
compatible with that constraint. Regex or keyword patterns are the preferred path.

**OQ-4 (R-009, score threshold 0.6)**: The 0.6 threshold is unvalidated by empirical
data. It is encoded as a constant in `engine/prespec/score/` and must be revisited after
the first handful of real runs. The constant SHOULD be exported so it is easy to adjust.
