# Design: prespec-malandra (hybrid Go engine + Socratic skill)

## Executive summary

prespec-malandra is built as a **hybrid**: a new zero-dependency Go package
`engine/prespec/` owns every deterministic mechanic (coverage grid, Impact×Uncertainty
ranking, no-leading lint, readiness scoring with the 0.6 gate, ULID generation, brief
schema and rendering), and the Markdown skill `skills/prespec-malandra/SKILL.md` drives
the probabilistic LLM Socratic interview (Stages 0-6) by **shelling out to a new
`engine prespec` subcommand** that speaks line-delimited JSON. The split exists so strict
TDD has real production code to test: the interview stays a portable prompt, the mechanics
become table-tested Go.

This document makes the hybrid decision (engram #1543) concrete. It does not re-open it.

## Quick path (how the pieces talk)

1. The skill conducts the interview turn by turn and accumulates **observed coverage** (which
   of the 10 cells the user has answered, and whether each answer is `Clear` or `Partial`).
2. To decide the next question, the skill calls `engine prespec rank` with the current grid
   state; the engine returns the ranked uncovered cells (Impact×Uncertainty) so the skill
   asks about the top cell.
3. Before asking, the skill calls `engine prespec lint` with the candidate question; the
   engine returns `accept` or a rejection reason. A rejected question is rewritten, never asked.
4. When the stop test may have fired, the skill calls `engine prespec readiness` with the
   grid; the engine returns the score and a pass/fail against the 0.6 gate.
5. On pass, the skill calls `engine prespec brief` with the full collected state; the engine
   mints a ULID, renders the brief (sections 1-6 + transcript), and returns it plus the
   target persistence path. The skill persists it to `project/{project}/prespec/{ULID}`.
6. On fail (or refusal floor), the engine/skill returns `needs-more-input` — never a brief,
   never a change-name.

The engine is **pure and stateless across calls**: the skill is the conversation memory and
passes the full state in on every invocation. This keeps each subcommand a deterministic
function of its input, which is exactly what golden/table tests need.

## Architecture decision

| Decision | Choice | Rationale | Rejected alternative |
|----------|--------|-----------|----------------------|
| Overall shape | Hybrid: Go mechanics + Markdown interview | Strict TDD needs testable code; interview is inherently LLM-driven (engram #1543) | Prompt-only (no `go test` target); Go-only (cannot run an LLM dialogue) |
| Invocation seam | New `engine prespec <verb>` subcommand, JSON over stdin/stdout | Matches existing subcommand-dispatch convention in `engine/cmd/main.go`; no new binary | Separate `prespec` binary (extra build/install surface); FFI/library import (skill cannot link Go) |
| Engine statefulness | Stateless; skill passes full grid state each call | Each call becomes a pure function → trivially table/golden testable | Engine holds session state (needs a store, breaks determinism, hard to test) |
| ULID | Generate in-package: 48-bit ms timestamp + 80-bit crypto-random, Crockford base32, lowercase | Keeps `engine/` zero-dep; format is unmistakably non-kebab (no hyphens, 26 chars) | Vendor `oklog/ulid` (adds a dep to a zero-dep module); timestamp-only slug (collision risk, looks date-like) |
| Failure contract | `prespec` fails LOUD (exit 1) on bad input | It is a CLI tool invoked deliberately, not a fail-safe hook like `gate-task` | Fail-safe (would silently emit garbage briefs) |
| Persistence | Skill writes brief to `project/{project}/prespec/{ULID}`, `capture_prompt:false` | Matches pre-sdd-contracts namespace rule; never under `sdd/` | Engine writes engram directly (engine has no engram access; skill owns memory) |

## Go package boundaries and file layout

All under `engine/prespec/`, package `prespec`, zero external deps (stdlib + `crypto/rand` only).

| File | Owns | Key exported surface |
|------|------|----------------------|
| `grid.go` | The 10-cell coverage model, cell states, Impact×Uncertainty ranking | `Cell`, `CellState`, `ImpactTier`, `Grid`, `Grid.RankUncovered() []Cell` |
| `readiness.go` | Readiness score + 0.6 gate; `Partial` never counts as `Clear` | `Readiness(Grid) Score`, `Score.Passes() bool`, gate const `ReadinessGate = 0.6` |
| `lint.go` | No-leading rejection rule set applied to a candidate question | `Lint(question string) LintResult`, `LintResult{Accepted bool, Reason string}` |
| `brief.go` | ULID id, brief schema (sections 1-6 + transcript), deterministic render | `Brief`, `NewID() string`, `RenderBrief(Brief) string`, `Brief.Path(project) string` |
| `prespec.go` | The thin orchestration each subcommand verb calls (rank/lint/readiness/brief) | `Rank(...)`, `CheckLint(...)`, `Evaluate(...)`, `BuildBrief(...)` |
| `*_test.go` | Tests for each of the above | table + golden |

The `engine prespec` dispatch lives in `engine/cmd/main.go` as `runPrespec(args)` →
`runPrespecCore(...)` with injected stdin/stdout/stderr and exit, mirroring the existing
`runPropagate`/`gateTaskCore` pattern. The core decodes the verb + JSON payload, calls the
matching `prespec` package function, and encodes the JSON response.

### Core types

```go
// CellState — a Partial answer must never be treated as Clear (R-009).
type CellState int
const (
    Missing CellState = iota // never asked / no answer
    Partial                  // answered but incomplete or hedged
    Clear                    // answered concretely
)

// ImpactTier — fixed per cell in the taxonomy; drives ranking weight.
type ImpactTier int
const ( Low ImpactTier = iota; Medium; High )

type Cell struct {
    Key         string     // stable cell id, e.g. "jtbd-job", "current-state-gap"
    Impact      ImpactTier // from taxonomy
    Uncertainty int        // 0..3, derived from state (Missing=3, Partial=2, Clear=0)
    State       CellState
}

type Grid struct { Cells []Cell } // exactly 10 cells, taxonomy order

// RankUncovered returns Missing+Partial cells sorted by Impact*Uncertainty desc,
// taxonomy order as the deterministic tiebreaker (no map iteration nondeterminism).
func (g Grid) RankUncovered() []Cell

type Score struct { Value float64; ClearCount, Total int }
func Readiness(g Grid) Score          // Clear-weighted; Partial contributes < Clear
func (s Score) Passes() bool          // s.Value >= ReadinessGate (0.6)

type Brief struct {
    ID         string   // ULID
    Job        string   // [verb]+[object]+[context]
    Sections   [6]string // sections 1-6, pinned by spec
    Assumptions []string // <= 3, each "[ASSUMPTION] ..."
    Transcript string    // full synthetic transcript
    CreatedAt  string    // RFC3339, derived from ULID timestamp for golden stability
}
```

## The invocation seam (CLI/JSON contract)

One subcommand, four verbs. Each reads a single JSON object from stdin and writes a single
JSON object to stdout. Errors go to stderr with exit 1 (fail loud).

```
engine prespec rank        < grid.json       > ranked.json
engine prespec lint        < question.json   > lintresult.json
engine prespec readiness   < grid.json       > score.json
engine prespec brief       < briefinput.json > brief.json
```

| Verb | Request shape | Response shape |
|------|---------------|----------------|
| `rank` | `{"cells":[{"key","impact","state"}...]}` | `{"ranked":[{"key","score"}...]}` |
| `lint` | `{"question":"..."}` | `{"accepted":bool,"reason":"..."}` |
| `readiness` | `{"cells":[...]}` | `{"value":0.0,"passes":bool,"clear":n,"total":10}` |
| `brief` | `{"job","sections":[...],"assumptions":[...],"transcript"}` | `{"id":"<ulid>","markdown":"...","path":"project/{project}/prespec/<ulid>"}` |

Why JSON over stdin (not flags): the grid and transcript are structured/multiline; stdin
matches how `gate-task` already takes structured input, and keeps the skill's call sites simple
(`echo '<json>' | engine prespec rank`). The skill owns project name and substitutes it into
the returned `path` template.

### no-leading lint rule set (`lint.go`)

A question is **rejected** when it matches any rule. Rules are deterministic string/shape checks
(the skill still does semantic judgement; the engine catches the mechanical smells):

1. **Smuggled answer** — contains a yes/no leading frame ("Don't you think...", "Isn't it...",
   "Wouldn't you rather...").
2. **Presupposed solution** — names an implementation noun before the problem is fixed
   (configurable solution-word list, e.g. "dashboard", "API", "button", "page") without a
   problem anchor.
3. **Bundled concerns** — contains a conjunction joining two question clauses ("and how / or
   what / and why" patterns), or more than one `?`.

`LintResult` returns the first failing rule's reason so the skill can rewrite precisely.

### readiness calc (`readiness.go`)

`value = (Clear + 0.5*Partial) / Total`, `Total = 10`. `Partial` weighted 0.5 guarantees a grid
full of Partial answers maxes at 0.5 and therefore **fails** the 0.6 gate — encoding "Partial ≠
Clear" numerically (R-009/R-010). Gate is `value >= 0.6`.

### ULID generation (`brief.go`)

```
ts   = uint48(time.Now().UnixMilli())          // 6 bytes, big-endian
rand = 10 bytes from crypto/rand               // 80 bits entropy
ulid = crockfordBase32(ts || rand)             // 26 chars, lowercased
```

Crockford base32 alphabet `0123456789abcdefghjkmnpqrstvwxyz`. The result is 26 lowercase
alphanumerics with **no hyphens** — structurally impossible to confuse with a kebab change-name
(R-012, AC-6). A small `NewIDFrom(ts, randReader)` seam lets tests inject a fixed clock + reader
for golden stability.

## Persistence

- The **engine never writes**. It returns the rendered markdown and the path template.
- The **skill** persists the brief to `project/{project}/prespec/{ULID}` via `mem_save` with
  `capture_prompt: false`, scope `project`, type appropriate to an automated artifact.
- Never under `sdd/` (red line). The ULID id and the `project/.../prespec/` prefix both enforce
  separation from change-name space.

## Testability (test seams)

| Unit | Pattern | Notes |
|------|---------|-------|
| `Grid.RankUncovered` | Table test | Cases: all-missing, mixed states, tie-break ordering, Clear cells excluded. Assert full ordered slice. |
| `Readiness` / `Passes` | Table test | Boundary cases around 0.6: all-Partial (0.5, fail), 6 Clear (0.6, pass), 5 Clear + 2 Partial (0.6, pass). |
| `Lint` | Table test | One case per rule (smuggle/presuppose/bundle) + clean-pass + first-failing-reason ordering. |
| `RenderBrief` | **Golden file** | Inject fixed ULID + CreatedAt; compare against `testdata/brief.golden`. Update only via `-update`. |
| `NewIDFrom` | Unit test | Fixed clock+reader → exact 26-char lowercase, no hyphen, decodes back to the timestamp. |
| `runPrespecCore` (dispatch) | Table test | Injected stdin/stdout/exit per verb; bad verb → exit 1 + stderr; malformed JSON → exit 1. |

Test command (per-module, strict TDD): `cd engine && go test ./...`. New tests live beside
their files as `engine/prespec/*_test.go`; golden data in `engine/prespec/testdata/`.

STRICT TDD is active for this change: each `prespec` function is written test-first. The skill
itself (Markdown) has no `go test` target and is exercised by manual interview runs, which is
the whole reason the deterministic logic was extracted into Go.

## Component / data flow

```
maintainer ──vague idea──> SKILL (Stages 0-6, interview, prompt-driven)
                              │  per-turn deterministic calls (stateless, JSON):
                              ├─ engine prespec rank      → next cell to probe
                              ├─ engine prespec lint       → accept/reject question
                              ├─ engine prespec readiness  → score + 0.6 gate
                              └─ engine prespec brief       → ULID + rendered markdown + path
                              │
                  pass ──────┴────── fail / refusal floor
                    │                       │
        mem_save project/{project}/prespec/{ULID}   return needs-more-input
        (capture_prompt:false)                       (no brief, no change-name)
```

Integration points: **none modified.** No engine phase, no orchestrator hook, no existing skill.
`gate-task`/`propagate` are untouched. The only new wiring is the `prespec` case in `main.go`'s
switch. The brief is consumed manually into `requirements-from-transcripts` in this slice.

## ADR-style decisions

- **ADR-1 — Subcommand over new binary.** Reuse `engine` dispatch. Keeps one build, one install
  path, consistent `*Core` test seams. Rejected: standalone binary (more install surface).
- **ADR-2 — Stateless engine, skill owns memory.** Every verb is a pure function of its JSON
  input. Rejected: engine session store (breaks determinism and TDD, needs persistence the
  engine has no business owning).
- **ADR-3 — Hand-rolled ULID, zero-dep.** `crypto/rand` + Crockford base32. Rejected: vendoring
  `oklog/ulid` (violates the module's zero-dep posture for ~40 lines of code we can test).
- **ADR-4 — Fail loud.** Unlike `gate-task`, `prespec` is deliberately invoked; a silent
  fallback would emit a malformed brief. Bad input → exit 1.
- **ADR-5 — Partial weighted 0.5 in readiness.** Numerically guarantees Partial ≠ Clear and that
  an all-Partial grid cannot pass the gate, satisfying R-009/R-010 without special-casing.

## Risks and alternatives

- **Skill/engine state drift** (skill mis-reports a cell as Clear). Mitigation: engine is the
  arbiter of ranking/readiness/lint, but it trusts the skill's reported states — garbage in,
  garbage out. Accepted for MVP; the readiness gate refuses low-coverage grids regardless.
- **Lint is mechanical, not semantic.** The Go lint catches shape smells; subtle leading
  questions still depend on the prompt. Accepted — the lint is a backstop, not the whole defense
  (consistent with the proposal's "probabilistic interview quality" risk).
- **ULID clock skew / non-monotonic.** Two briefs in the same millisecond rely on 80 random bits
  for uniqueness; collision probability is negligible. Not making ULIDs monotonic in this slice.
- **JSON-over-stdin ergonomics in the skill.** The skill must build valid JSON for each call.
  Mitigation: payloads are small and documented in the verb table; the engine fails loud on
  malformed JSON so mistakes surface immediately rather than corrupting a brief.
- **`requirements-from-transcripts` ingesting a synthetic transcript** (open question A-2 from the
  proposal). Unchanged by this design — the brief's transcript section is rendered to read like a
  real conversation; validation deferred to first real runs.

## Non-goals (deferred to v2, restated)

Tier mode selection (quick=3/deep=7 — MVP hardcodes budget 5); security→High cell auto-promotion;
enforced `sdd-propose` crosswalk; `sdd-explore` auto-read; OSS packaging; engine-side persistence;
monotonic ULIDs; modifying any existing skill or engine phase.

## Checklist (for sdd-tasks)

- [ ] `engine/prespec/grid.go` + table tests (ranking, tie-break, Clear exclusion)
- [ ] `engine/prespec/readiness.go` + boundary table tests around 0.6
- [ ] `engine/prespec/lint.go` + one table case per rejection rule
- [ ] `engine/prespec/brief.go` (ULID + render) + golden + ULID-format tests
- [ ] `engine/prespec/prespec.go` orchestration + `runPrespec` dispatch in `main.go` + dispatch tests
- [ ] `skills/prespec-malandra/SKILL.md` interview wired to the four verbs
- [ ] `skills/prespec-malandra/references/coverage-taxonomy.md` (10 cells, impact tiers, lint list)
- [ ] Persist path `project/{project}/prespec/{ULID}`, `capture_prompt:false`

## Next step

Proceed to `sdd-tasks` once the spec is also ready (tasks reads spec + design).
```

