# Prespec Coverage Taxonomy

Reference for `prespec-malandra` skill. Read this alongside SKILL.md; it does not replace the stage instructions.

---

## 10-Cell Grid

Full taxonomy in canonical index order. The engine's `rank` verb respects this index order for tie-breaking.

| # | Key | Impact (1-5) | Uncertainty (1-5) | I×U | Covers |
|---|-----|:---:|:---:|:---:|--------|
| 0 | `jtbd-job` | 5 | 5 | 25 | The job statement: verb+object+context. The foundational cell — everything else is weighed against it. |
| 1 | `current-gap` | 5 | 4 | 20 | What breaks down or fails in the current approach. Evidence from real past events. |
| 2 | `why-now` | 4 | 5 | 20 | What changed recently that makes this worth solving now. Urgency trigger. |
| 3 | `user-segment` | 4 | 3 | 12 | Who specifically faces this job. Role, context, frequency of exposure. |
| 4 | `constraints` | 3 | 4 | 12 | Real limits: time, tooling, budget, regulation, team size, integration surface. |
| 5 | `success-metric` | 3 | 3 | 9 | How the user would know the job is done. Measurable outcome. Accept `no-metric-yet` if unknown. |
| 6 | `alternatives` | 3 | 3 | 9 | What the user does today instead. Workarounds, competing tools, manual steps. |
| 7 | `stakeholders` | 2 | 3 | 6 | Who else is affected, who decides, who must approve. |
| 8 | `frequency` | 2 | 2 | 4 | How often the job arises. Daily? Weekly? Per-project? |
| 9 | `risk-unknowns` | 2 | 4 | 8 | Risks, open unknowns, or dependencies that could invalidate the brief. |

---

## Ranking Formula

The engine sorts uncovered cells by:

```
sort key = Impact × Uncertainty  (descending)
tie-break = cell index           (ascending — lower index wins)
```

`Clear` cells are excluded from ranking. `Partial` cells are still ranked (not yet fully covered).

Example with all-missing grid:

```
jtbd-job    (5×5=25)  → ranked #1
current-gap (5×4=20)  → ranked #2
why-now     (4×5=20)  → ranked #3  (same score as current-gap; index 2 > index 1, so current-gap wins)
user-segment(4×3=12)  → ranked #4
constraints (3×4=12)  → ranked #5  (same score; index 4 > index 3, so user-segment wins)
risk-unknowns(2×4=8)  → ranked #6
success-metric(3×3=9) → ranked... wait — score 9 > 8, so success-metric before risk-unknowns
```

Correct full order (all-missing):

| Rank | Key | I×U |
|------|-----|-----|
| 1 | jtbd-job | 25 |
| 2 | current-gap | 20 |
| 3 | why-now | 20 |
| 4 | user-segment | 12 |
| 5 | constraints | 12 |
| 6 | success-metric | 9 |
| 7 | alternatives | 9 |
| 8 | risk-unknowns | 8 |
| 9 | stakeholders | 6 |
| 10 | frequency | 4 |

---

## No-Leading Lint Rejection Checklist

The engine `lint` verb applies these three rules in order. First match wins; the question is rejected.

### Rule 1 — `smuggles-answer`

The question embeds a leading frame or presupposes the user's feeling.

Signals: "Would you say...", "Don't you think...", "Isn't it true that...", "Wouldn't you rather...", "Do you feel that..."

Example rejected: "Would you say that adding automation would fix this?"
Example accepted: "What did you try the last time this happened?"

### Rule 2 — `presupposes-solution`

The question names a specific technology, feature, or solution noun without anchoring to the problem first.

Signals: dashboard, API, webhook, button, page, module, integration, plugin, service, microservice, algorithm, solution, feature, React, Vue, Next, Slack, Stripe, ...

Example rejected: "Should we build a REST API or a webhook for this?"
Example accepted: "How do you currently get data from one system to another?"

### Rule 3 — `bundles-concerns`

The question asks about two separate things at once.

Signals:
- Two question marks in one sentence: "Who does this affect? And how often?"
- Conjunction-clause patterns: "and also", "as well as", "and what / and how / and why / and when / and where / and who"

Example rejected: "Who experiences this problem and how often does it occur?"
Example accepted: "Who is most affected when this breaks?"

---

## Readiness Formula

```
value = (Clear + 0.5 × Partial) / 10
```

- `Clear` cells count as 1.0 each.
- `Partial` cells count as 0.5 each.
- `Missing` cells count as 0.0.
- Total always 10.

Gate: `passes = (value >= 0.6)` (boundary inclusive).

Note: An all-`Partial` grid gives `value = 0.5` — below the gate. `Partial` is explicitly not equal to `Clear`.

---

## Stage 5 Stop Conditions

The skill checks these after every question in Stage 3, in priority order:

| Priority | Condition | Stop Reason |
|---|---|---|
| 1 (highest) | `asked >= budget (5)` | `budget-exhausted` |
| 2 | `readiness value >= 0.6` | `coverage-threshold` |
| 3 | User says "that's enough" / "let's proceed" / "generate the brief" | `user-signal` |
| 4 | Last 2 answers were "I don't know" or "not applicable" | `diminishing-returns` |

When a stop condition fires, proceed to Stage 5 (convergence test). The engine's readiness call in Stage 5 is the authoritative gate — the loop's early check is advisory.

---

## `no-metric-yet` Escape

For the `success-metric` cell specifically:

- If the user says "I don't know", "not yet", "hard to say", or similar → leave the cell at `"missing"`.
- Do NOT re-ask. Do NOT pressure the user.
- The assumption "no metric defined yet" is valid and should appear in Section 6 as `[ASSUMPTION] Success metric not yet defined.`
- The cell at `"missing"` with score 0.0 is correctly factored into readiness. Other cells must compensate.
