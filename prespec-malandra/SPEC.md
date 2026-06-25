# prespec-malandra — Specification

> **Version**: 1.0  
> **Status**: Draft  
> **License**: Apache-2.0

---

## Table of Contents

1. [Purpose and the Gap](#1-purpose-and-the-gap)
2. [Conformance Split](#2-conformance-split)
3. [Coverage Model](#3-coverage-model)
4. [Interview State Machine](#4-interview-state-machine)
5. [Determinism Contracts](#5-determinism-contracts)
6. [The Brief Artifact](#6-the-brief-artifact)
7. [Red Lines](#7-red-lines)
8. [Reference Verb Contract](#8-reference-verb-contract)
9. [Attribution](#9-attribution)
10. [Conformance Checklist](#10-conformance-checklist)

---

## 1. Purpose and the Gap

### The problem

Every structured discovery method assumes some prior artifact:

| Tool / Technique | What it requires as input |
|---|---|
| spec-kit `/clarify` | An existing `spec.md` with a change-name and scope |
| The Mom Test | A human facilitator to conduct the interview |
| Jobs-To-Be-Done | A human analyst to synthesize job statements |
| Impact Mapping | A stated goal, actor list, and deliverable candidates |

None of these addresses the zero-input case: **a person has a vague idea — no requirements, no transcript, no change-name — and needs to know what to build before they can spec it.**

### The thesis

prespec-malandra fills that gap with an **executable Socratic state machine** that:

- starts from zero input (a single vague idea sentence is sufficient);
- drives a structured discovery interview over a finite coverage grid;
- knows when it has enough information and stops;
- refuses to fabricate goals or requirements when evidence is absent;
- emits a machine-readable brief that downstream tools can consume.

The **novel contribution is the executable assembly**, not any individual piece. Past-behavior probing (Mom Test), job framing (JTBD), goal ordering (Impact Mapping), and taxonomy design (spec-kit) are each existing techniques. prespec-malandra is the first specification for composing them into an automated, convergence-aware, refusal-floor Socratic loop.

### What it is not

prespec-malandra is **not** a requirements tool. It produces a discovery brief — a structured transcript. Deriving change-names, writing EARS requirements, and producing a specification are downstream concerns (e.g., `requirements-from-transcripts`).

---

## 2. Conformance Split

Implementations MUST separate deterministic mechanics from LLM-driven judgment.

### Deterministic core (MUST be reproducible and testable)

The following behaviors MUST produce identical output given identical input, with no LLM call:

- The 10-cell taxonomy (keys, Impact values, Uncertainty values)
- Impact × Uncertainty ranking with tie-break
- No-leading lint (three rules, applied in order)
- Readiness formula and gate
- ID generation (ULID-compatible, structurally distinct from kebab slugs)
- Brief validation (field presence + readiness gate)
- Brief rendering (deterministic Markdown from a Brief struct)

An implementation MAY place the deterministic core in a library, a subprocess, or a remote service. It MUST behave identically regardless of placement.

### LLM-driven judgment (MAY vary across calls)

The following behaviors are driven by an LLM and are inherently non-deterministic:

- Selecting the next question text given a ranked cell key
- Assessing whether a user answer is substantive, partial, or non-responsive
- Synthesizing the job statement from a Stage 0 answer
- Writing the six brief sections from the conversation

The LLM layer MUST still obey all deterministic contracts (it MUST NOT ask questions that the lint rules would reject; it MUST NOT persist a brief when readiness fails).

---

## 3. Coverage Model

### The 10-cell grid

The grid is the shared state of one interview session. It contains exactly 10 cells, each with a fixed key, Impact score (1–5), and Uncertainty score (1–5).

| # | Key | Impact | Uncertainty | I×U | Covers |
|---|-----|:---:|:---:|:---:|--------|
| 0 | `jtbd-job` | 5 | 5 | 25 | Job statement: verb + object + context |
| 1 | `current-gap` | 5 | 4 | 20 | What fails in the current approach; evidence from real events |
| 2 | `why-now` | 4 | 5 | 20 | What changed recently that makes this worth solving now |
| 3 | `user-segment` | 4 | 3 | 12 | Who specifically faces this job; role, context, frequency |
| 4 | `constraints` | 3 | 4 | 12 | Real limits: time, tooling, budget, regulation, team size |
| 5 | `success-metric` | 3 | 3 | 9 | How the user would know the job is done; accept `no-metric-yet` |
| 6 | `alternatives` | 3 | 3 | 9 | What the user does today instead; workarounds, competing tools |
| 7 | `stakeholders` | 2 | 3 | 6 | Who else is affected, who decides, who must approve |
| 8 | `frequency` | 2 | 2 | 4 | How often the job arises |
| 9 | `risk-unknowns` | 2 | 4 | 8 | Risks, open unknowns, or dependencies that could invalidate the brief |

Implementations MUST NOT add, remove, or reorder cells. The cell index is load-bearing: it is the tie-break key in ranking.

### Cell states

Each cell carries exactly one of three states:

```
Missing  —  not yet probed
Partial  —  probed but not fully resolved
Clear    —  fully resolved
```

State transitions MUST obey a forward-only rule:

```
Missing → Partial → Clear    ✓  (allowed)
Missing → Clear              ✓  (allowed, skip)
Clear → Partial              ✗  (forbidden)
Clear → Missing              ✗  (forbidden)
Partial → Missing            ✗  (forbidden)
```

An attempt to regress a Clear cell MUST be rejected with an error. This enforces that evidence gathered during an interview is never undone.

### Ranking

To select the next cell to probe, implementations MUST rank all non-Clear cells (both Missing and Partial are eligible) by:

```
sort key  = Impact × Uncertainty  (descending)
tie-break = cell index            (ascending — lower index wins)
```

Example (all-Missing grid, full ranking):

```
Rank  Key              I×U
1     jtbd-job          25
2     current-gap       20
3     why-now           20   (same score; index 2 > index 1)
4     user-segment      12
5     constraints       12   (same score; index 4 > index 3)
6     success-metric     9
7     alternatives       9   (same score; index 6 > index 5)
8     risk-unknowns      8
9     stakeholders       6
10    frequency          4
```

When all cells are Clear, the ranking result is an empty list. This signals the interview SHOULD proceed to the convergence test immediately.

---

## 4. Interview State Machine

The interview is a 7-stage state machine (Stages 0–6). The deterministic core drives stage transitions and stop conditions; an LLM drives question generation and synthesis within each stage.

```
Stage 0: Cold Start
  │
  ├─ 0a: Mom-Test past-behavior probe
  ├─ 0b: Archetype MCQ fallback (if 0a yields no actionable answer)
  └─ 0c: Refusal floor (if 0b also yields nothing) → EXIT with needs-more-input
  │
Stage 1: Job Statement
  ├─ 1a: Anti-solution bounce (redirect if user leads with a solution)
  ├─ 1b: Synthesize [verb]+[object]+[context] job statement
  └─ 1c: Bounded readback (job framing only — no strawman)
  │
Stage 2: Initialize Grid
  (set jtbd-job cell state based on Stage 1 outcome; all others: Missing)
  │
Stage 3: Adaptive Question Loop  ←──────────────────────────┐
  ├─ 3a: rank(grid) → pick top uncovered cell               │
  ├─ 3b: draft one question for that cell                   │
  ├─ 3c: lint(question) → rewrite loop until accepted       │
  ├─ 3d: ask question; update cell state from answer        │
  └─ 3e: check stop conditions → if none: loop back ────────┘
          if stop fires: proceed to Stage 4
  │
Stage 4: Bounded Informed Guesses (at most 3 [ASSUMPTION]-marked items)
  │
Stage 5: Convergence Test
  ├─ readiness(grid) < 0.6 → EXIT with needs-more-input
  └─ readiness(grid) ≥ 0.6 → proceed to Stage 6
  │
Stage 6: Brief Emission
  ├─ 6a: synthesize 6 sections from conversation
  ├─ 6b: assemble transcript
  ├─ 6c: call brief(project, job, sections, transcript, cells) → {id, markdown, path}
  ├─ 6d: persist brief at path
  └─ 6e: non-leading readback to user
```

### Stage 0 — Cold Start

**Goal**: surface a past behavior or real friction without asking what the user wants to build.

**0a — Mom-Test probe**: The first message MUST be a single past-behavior question. Templates:
- "Tell me about the last time you ran into [relevant friction domain]. What happened?"
- "Walk me through how you handle [relevant task] today — step by step."
- "When did this problem last cost you time or money? What did you do?"

The implementation MUST NOT ask "What do you want to build?", "What feature would you like?", or any desire/plan/solution framing at this stage.

**0b — Archetype MCQ fallback**: If the user's answer is non-actionable (one-word, "I don't know", silence, or off-topic), the implementation MUST present a 5-archetype multiple-choice question:

```
I want to understand the kind of problem you're facing. Which of these is closest?

A. I have a manual/repetitive workflow that takes too long
B. I'm losing visibility into something I need to track
C. My team can't coordinate or hand off work reliably
D. I'm making decisions without the data I need
E. None of these — let me describe it differently
```

**0c — Refusal floor**: If, after both 0a and 0b, the user still provides no actionable goal (no job, no problem domain, no constraint), the implementation MUST return:

```
needs-more-input: I couldn't identify a clear problem or goal to explore.
No brief will be generated. When you have a real situation to discuss, start the interview again.
```

The implementation MUST stop at this point. It MUST NOT proceed to Stage 1, call any deterministic verb, or persist any artifact.

### Stage 1 — Job Statement

**1a — Anti-solution bounce**: If the user's Stage 0 answer is solution-first (names a technology, feature, or implementation artifact), the implementation MUST redirect to the underlying problem and return to a Stage 0 Mom-Test probe before synthesizing a job statement.

**1b — Job statement synthesis**: Synthesize a job statement in the form `[verb]+[object]+[context]`. Example: "track inventory levels across warehouse locations without manual spreadsheet updates".

**1c — Bounded readback**: Present the job statement to the user for confirmation. The readback MUST contain the job statement only. The implementation MUST NOT include data models, entity lists, architecture notes, hypothetical workflows, or any invented artifact. If confirmed without changes, set `jtbd-job` to `Clear`. If the user adjusts it, set `jtbd-job` to `Partial` until re-confirmed.

### Stage 2 — Grid Initialization

After the job statement is confirmed, initialize the 10-cell grid. `jtbd-job` reflects the Stage 1 outcome; all other cells start at `Missing`. No deterministic core call is needed at this stage — the cell table is a fixed constant.

### Stage 3 — Adaptive Question Loop

**Mode and budget**: The standard mode has a budget of 5 questions. Implementations MAY support additional modes (e.g., quick=3, deep=7) but MUST default to standard=5. The budget is a hard ceiling on Stage 3 questions asked.

**Per-iteration procedure**:

1. **Pick**: call `rank(grid)`. Select `ranked[0]` — the highest-priority uncovered cell. If ranked is empty (all Clear), exit the loop and proceed to Stage 4.
2. **Draft**: compose one question targeting the selected cell. The question MUST address past behavior or current reality (not future desires). It MUST focus on a single concern.
3. **Lint**: call `lint(question)`. If `accepted: false`, rewrite the question to avoid the flagged pattern and lint again. Repeat until `accepted: true`. The implementation MUST NOT present a question to the user that lint rejected.
4. **Ask and record**: present the linted question. After the user answers:
   - Substantive answer → set cell to `Partial` or `Clear`
   - "I don't know" / "not applicable" for `success-metric` → leave at `Missing`, record `no-metric-yet`, do not re-ask
   - "I don't know" for any other cell → set to `Partial` (partial information is still information)
   - Increment the `asked` counter.
5. **Stop check**: evaluate stop conditions (see [Section 5 — ShouldStop](#shouldstop)). If a stop condition fires, exit the loop and proceed to Stage 4.

### Stage 4 — Bounded Informed Guesses

After the loop exits, the implementation MAY make at most 3 informed guesses to fill critical gaps. Each guess MUST:

- Be marked with the literal token `[ASSUMPTION]`.
- Be interrogated against the job statement: "Does this assumption contradict or modify the job?"
- Be discarded if it contradicts the job statement.

If more than 3 guesses would be needed to fill gaps, the implementation MUST stop at 3 and let the readiness gate decide whether to emit a brief.

### Stage 5 — Convergence Test

Call `readiness(grid)` once more as the authoritative gate.

- If `value < 0.6`: return `needs-more-input` with the list of uncovered cell keys. Do NOT call `brief`. Do NOT persist any artifact.
- If `value ≥ 0.6`: proceed to Stage 6.

### Stage 6 — Brief Emission

**6a — Synthesize sections**: From the conversation, synthesize all 6 brief sections (see [Section 6](#6-the-brief-artifact) for section content mapping).

**6b — Assemble transcript**: Collect all questions asked and user answers, in order. This is the synthetic transcript that downstream tools consume.

**6c — Call brief**: Pass `{project, job, sections[6], transcript, cells}` to the deterministic core. The core validates the readiness gate a second time (defense in depth), assigns a ULID, renders Markdown, and returns `{id, markdown, path}`. If validation fails, report the error and stop.

**6d — Persist**: Save the brief to `project/{project}/prespec/{id}`. The deterministic core MUST NOT perform persistence; the driver (skill, service, CLI) is responsible.

**6e — Non-leading readback**: Present the rendered brief to the user. The readback MUST be informational. The implementation MUST NOT ask leading questions about what to build next.

---

## 5. Determinism Contracts

### Readiness formula

```
value = (Clear + 0.5 × Partial) / 10
passes = (value >= 0.6)   [boundary inclusive]
```

`Total` is always 10 (the fixed grid size). The 0.5 weight on Partial is a design invariant that encodes two guarantees:
- `Partial ≠ Clear` (a cell with partial evidence is worth half a clear cell)
- An all-Partial grid yields `value = 0.5`, which is strictly below the gate — an interview of all half-answers MUST NOT produce a brief.

### ShouldStop

Stop conditions MUST be evaluated in this exact priority order after each question in Stage 3:

| Priority | Condition | Stop Reason |
|:---:|---|---|
| 1 (highest) | `asked >= budget` | `budget-exhausted` |
| 2 | `value >= 0.6` | `coverage-threshold` |
| 3 | User explicitly signals done ("that's enough", "generate the brief", etc.) | `user-signal` |
| 4 | Last 2 consecutive answers were "I don't know" or "not applicable" | `diminishing-returns` |

Budget exhaustion MUST take priority over convergence. An implementation MUST NOT continue asking questions after the budget is exhausted even if readiness has not been reached. The convergence test in Stage 5 is authoritative; the Stage 3 stop check is advisory.

### No-leading lint

Three rules are applied in order. The first rule that matches causes rejection. Evaluation is case-insensitive.

**Rule 1 — `smuggles-answer`**

Intent: the question presupposes the user's desired outcome or embeds a leading frame.

Signals: "Would you say…", "Wouldn't you rather…", "Do you want…", "Don't you think…", "Wouldn't it be nice if…"

```
Rejected:  "Would you say that adding automation would fix this?"
Accepted:  "What did you try the last time this happened?"
```

A regex is one valid implementation. The rule must detect: word-boundary `would` (including `wouldn't`, `would not`) and the phrase `do you want`.

**Rule 2 — `presupposes-solution`**

Intent: the question names a specific implementation artifact or technology without anchoring to the problem.

Signals: `feature`, `dashboard`, `api`, `module`, `integration`, `plugin`, `service`, `microservice`, `algorithm`, `solution` — and implementation-specific nouns added by the implementor.

```
Rejected:  "Should we build a REST API or a webhook for this?"
Accepted:  "How do you currently get data from one system to another?"
```

The exact vocabulary list is at the implementor's discretion; the intent MUST be preserved.

**Rule 3 — `bundles-concerns`**

Intent: the question asks about two independent concerns in one turn.

Signals: "and also", "as well as", or "and" immediately followed by an interrogative word (what, how, why, when, where, who).

```
Rejected:  "Who experiences this problem and how often does it occur?"
Accepted:  "Who is most affected when this breaks?"
```

### ID generation

The discovery ID MUST satisfy all of the following:

- 26 characters, uppercase alphanumeric (no hyphens, no underscores, no lowercase)
- Lexicographically sortable by creation time (timestamp-prefixed)
- Generated from a cryptographically random source
- **Structurally impossible to confuse with a kebab-case change-name** (no hyphens by construction)

ULID (Universally Unique Lexicographically Sortable Identifier) is the reference-conforming format. The reference implementation uses: 48-bit millisecond timestamp ‖ 80-bit crypto-random entropy, encoded in Crockford base32 (uppercase, no `I`, `L`, `O`, `U`), 26 characters total. Implementations MAY use any format that satisfies all four constraints above.

---

## 6. The Brief Artifact

A brief is the structured output of one completed interview session. It is a **synthetic transcript**, not a specification. It is the artifact that downstream tools (e.g., `requirements-from-transcripts`) consume to derive requirements and a change-name.

### Structure

```markdown
# Discovery Brief

**ID**: <26-char ID>
**Project**: <project name>
**Created**: <RFC 3339 UTC timestamp>
**Job**: <job statement from Stage 1>

## 1. Job Statement

<job statement confirmed in Stage 1>

## 2. Current Gap & Problem Evidence

<current gap / problem evidence from Stage 0 + current-gap cell>

## 3. Why Now

<real limits from constraints cell and alternatives cell>

## 4. Users & Stakeholders

<why-now urgency + any bounded informed guesses from Stage 4>

## 5. Constraints & Alternatives

<user segment, stakeholders, frequency>

## 6. Assumptions

<success-metric if captured; [ASSUMPTION] if no-metric-yet>

## Interview Transcript

```
<full verbatim conversation: questions asked + user answers in order>
```
```

### Header fields

| Field | Requirement |
|---|---|
| `ID` | MUST be the ULID generated by the deterministic core |
| `Project` | MUST be the project name; MUST NOT start with `sdd/` |
| `Created` | MUST be the wall-clock time of brief generation in RFC 3339 UTC |
| `Job` | MUST be the confirmed job statement from Stage 1 |

### Persistence location rule

Briefs MUST be persisted at:

```
project/{project}/prespec/{id}
```

The deterministic core MUST NOT perform persistence. It MUST return `{id, markdown, path}` and leave storage to the driver. This separation allows the same core to be used in CLI tools, services, and agent frameworks without coupling to a storage backend.

The `sdd/` namespace is reserved for SDD change artifacts. A brief MUST NOT be saved under `sdd/` or any derivative path.

### Validation before emission

Before rendering and returning a brief, the deterministic core MUST verify:

- `project` is non-empty and does not start with `sdd/`
- `job` is non-empty
- `transcript` is non-empty
- `readiness(cells).passes == true`

Any validation failure MUST cause a hard error. The core MUST NOT silently emit an incomplete brief.

---

## 7. Red Lines

The following behaviors are unconditional prohibitions. An implementation that violates any of these does not conform to this specification.

| Red Line | Rule |
|---|---|
| **No change-name derivation** | The engine MUST NOT derive, suggest, or return a kebab-slug change-name. Change-name ownership belongs exclusively to downstream tools (e.g., `requirements-from-transcripts`). |
| **No invented strawman** | The Stage 1 readback MUST contain the job statement only. The implementation MUST NOT invent data models, entity lists, architecture notes, scenario diagrams, or any artifact beyond the job framing. |
| **No self-grading** | The implementation MUST NOT grade the coverage grid against a draft or artifact it wrote itself. Readiness is computed by the deterministic core against user-provided answers only. |
| **No metric pressure** | If the user cannot articulate a success metric, set `success-metric` to `Missing` and record `no-metric-yet`. Do NOT re-ask. Do NOT require a metric before proceeding. |
| **No fabrication** | If Stage 0 yields no actionable goal after both 0a and 0b, the implementation MUST return `needs-more-input` and stop. It MUST NOT invent a goal. |
| **No persistence without gate** | The implementation MUST NOT persist a brief when `readiness.passes == false`. |
| **Namespace guard** | Briefs MUST be saved under `project/{project}/prespec/{id}`. The `sdd/` namespace MUST NOT be used. |
| **No solution-anchored questions** | Every question asked during Stage 3 MUST pass the no-leading lint rules. Questions that fail lint MUST be rewritten before being presented. |

---

## 8. Reference Verb Contract

The following JSON interface is the reference implementation of the deterministic core. It is clearly marked as one conforming interface, not the only permissible interface. Implementations MAY expose the same logic as a library, gRPC service, or HTTP endpoint, provided the behavior contracts in Sections 3–6 are preserved.

In the reference implementation, the core is invoked as a subprocess: `engine prespec <verb>` reads JSON from stdin and writes JSON to stdout. Exit code 1 signals a hard error; the error description is written to stderr.

---

### `rank`

Rank all non-Clear cells by Impact × Uncertainty descending, stable-sorted by cell index.

**Input**
```json
{
  "cells": [
    { "key": "jtbd-job", "impact": 5, "uncertainty": 5, "state": "clear" },
    { "key": "current-gap", "impact": 5, "uncertainty": 4, "state": "missing" }
    // ... remaining 8 cells
  ]
}
```

**Output**
```json
{
  "ranked": [
    { "key": "current-gap", "impact": 5, "uncertainty": 4, "state": "missing" }
    // ... all non-Clear cells, sorted
  ]
}
```

Cell `state` in the output is `"missing"` or `"partial"`. `"clear"` cells are excluded.

---

### `lint`

Check whether a proposed interview question passes all three no-leading rules.

**Input**
```json
{ "question": "What happens when the current process breaks down?" }
```

**Output — accepted**
```json
{ "accepted": true, "rule": "", "reason": "" }
```

**Output — rejected**
```json
{
  "accepted": false,
  "rule": "smuggles-answer",
  "reason": "question steers toward a solution; ask what the user needs, not whether they want a specific thing"
}
```

`rule` is the name of the first failing rule. Only one rule is reported per call (first-match wins).

---

### `readiness`

Compute `(Clear + 0.5 × Partial) / 10` and evaluate against the 0.6 gate.

**Input**
```json
{ "cells": [ /* same shape as rank */ ] }
```

**Output**
```json
{ "value": 0.65, "passes": true, "clear": 6, "total": 10 }
```

`passes` is `true` when `value >= 0.6` (boundary inclusive).

---

### `brief`

Validate, assign ID, render Markdown, and return the path. Does not write to any storage.

**Input**
```json
{
  "project": "my-project",
  "job": "track inventory levels across warehouse locations without manual spreadsheet updates",
  "sections": [
    "Section 1 text — Job Statement",
    "Section 2 text — Current Gap & Problem Evidence",
    "Section 3 text — Why Now",
    "Section 4 text — Users & Stakeholders",
    "Section 5 text — Constraints & Alternatives",
    "Section 6 text — Assumptions or empty string"
  ],
  "transcript": "Q: Tell me about the last time... A: Last week...",
  "cells": [ /* same shape as rank */ ]
}
```

**Output**
```json
{
  "id": "01HXYZ1234567890ABCDEF",
  "markdown": "# Discovery Brief\n\n**ID**: 01HXYZ...",
  "path": "project/my-project/prespec/01HXYZ1234567890ABCDEF"
}
```

On validation failure (readiness not passing, empty project/job/transcript), the core exits with code 1 and writes a descriptive error to stderr.

---

## 9. Attribution

### Borrowed techniques

| Source | What is borrowed |
|---|---|
| **spec-kit `/clarify`** | Coverage taxonomy structure and the question-budget concept (standard=5 questions). The 10-cell grid is adapted from spec-kit's clarify taxonomy, re-weighted to front-load pre-goal cells (`jtbd-job`, `current-gap`, `why-now`). |
| **The Mom Test** (Rob Fitzpatrick) | Past-behavior probe pattern (Stage 0a): ask about what the user has done, not what they want. The prohibition on leading questions is The Mom Test's core teaching. |
| **Jobs-To-Be-Done** (Ulwick, Christensen) | The `[verb]+[object]+[context]` job statement form (Stage 1b). The framing that a user "hires" a product to do a job. |
| **Impact Mapping** (Gojko Adzic) | Goal-first, deliverable-last ordering in the coverage grid. The idea that identifying the goal and gap before naming features reduces waste. |

### What is new

The novel contribution is the **executable assembly**: an automated, convergence-aware, refusal-floor Socratic loop that starts from zero input. Specifically:

- **Cold-start from zero**: no prior transcript, no change-name, no existing spec required.
- **Adaptive question sequencing**: a deterministic ranking function drives question order dynamically based on coverage gaps, rather than following a fixed script.
- **Convergence criterion**: a quantitative readiness formula with a gated threshold that determines when enough information has been gathered, without relying on a human facilitator's judgment.
- **Honest refusal floor**: the system returns `needs-more-input` and halts rather than fabricating a goal when no actionable input is given.
- **Forward-only grid state**: a formal constraint that prevents LLM-driven regressions of established evidence.

---

## 10. Conformance Checklist

An implementation conforms to this specification if and only if all of the following hold:

- [ ] The 10-cell grid uses the exact keys, Impact values, and Uncertainty values defined in Section 3, in index order.
- [ ] Cell state transitions enforce the forward-only rule: a Clear cell cannot regress.
- [ ] `rank` sorts non-Clear cells by Impact × Uncertainty descending, with stable tie-break by cell index ascending.
- [ ] The no-leading lint applies exactly three rules in the specified order; first-match wins; evaluation is case-insensitive.
- [ ] Readiness is computed as `(Clear + 0.5 × Partial) / 10`; the gate is `>= 0.6` (boundary inclusive).
- [ ] Stop conditions are evaluated in the priority order: budget > coverage-threshold > user-signal > diminishing-returns.
- [ ] The discovery ID is 26 characters, uppercase alphanumeric, timestamp-prefixed, crypto-random, and contains no hyphens.
- [ ] The brief is never persisted when `readiness.passes == false`.
- [ ] The brief path follows the form `project/{project}/prespec/{id}`; `sdd/` is never used.
- [ ] The implementation returns `needs-more-input` without persisting any artifact when Stage 0 yields no actionable goal.
- [ ] No change-name is derived, suggested, or returned by the engine.
- [ ] The Stage 1 readback contains only the job statement; no data model or architecture is invented.
- [ ] The deterministic core never writes to storage; it returns `{id, markdown, path}` and delegates persistence to the driver.
- [ ] Questions presented to the user in Stage 3 have passed the lint check; lint-rejected questions are rewritten before asking.
