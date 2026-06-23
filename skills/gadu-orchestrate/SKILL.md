---
name: gadu-orchestrate
description: "Trigger: orchestrate, fan out, validate an idea between agents, dialectic, yin-yang synthesis, debate an idea, multi-perspective, adversarial multi-agent review, steelman vs attack, decompose and parallelize. GADU portable composition layer for idea validation, adversarial code review, and parallel fan-out."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Load this skill when GADU must orchestrate more than its own single thread: (a) validate an IDEA or design decision through opposing lenses, (b) run multi-agent review of CODE/diffs, or (c) fan out broad, decomposable work. This is a COMPOSITION layer — it sequences existing skills and the runtime's native delegation; it does NOT reimplement review or SDD.

This file is self-contained text. It must work read directly in Claude Code, opencode, AND codex. It assumes no runtime-specific tool.

## Hard Rules

- **Top-level only.** Orchestrate ONLY when GADU is the lead/top-level operator. In these runtimes a sub-agent generally cannot spawn its own sub-agents. If GADU is itself a spawned sub-agent, do the work single-threaded and SAY SO — never pretend to delegate.
- **Native dispatch, named per runtime.** Use the runtime's own mechanism, never a runtime-specific workflow/cron tool: Claude Code = the Agent/Task tool; opencode = task dispatch (subject to the `permission.task` whitelist); codex = `spawn_agent` / `wait_agent` / `close_agent` (codex caps: `max_depth=2`, `max_threads=4` — keep fan-out ≤ 4 concurrent). If native dispatch is unavailable, run the protocol single-threaded and flag it.
- **Always synthesize.** Never dump raw sub-agent outputs. Reconcile into ONE answer with a recommendation.
- **Blind independence.** Lenses and judges run in FRESH, isolated context; they never see each other or share state. Agreement = confirmed. Divergence = surface it, do not auto-trust.
- **Ground before asserting.** Each agent validates against the real artifact/data. If it reasons without access (e.g. from a brief only), the synthesis FLAGS that conclusion as hypothesis, not verdict, and names the decisive check.
- **Honest confidence.** End every run with LOW / MEDIUM / HIGH and what would raise it.
- **Dedupe launches.** One launch per distinct `(role, task)`. Reserve questions for genuine forks, not micro-steps.

## Modes

### 1. Dialectic → Synthesis (FLAGSHIP) — validate an IDEA/design

1. Frame the proposition in one sentence. Pick 2+ DIVERGENT lenses committed to opposing poles (e.g. builder/advocate vs skeptic/operator, or N distinct perspectives). State each pole's mandate.
2. Spawn each lens BLIND and INDEPENDENT (fresh context, no shared state). One must STEELMAN the proposition; one must ATTACK it. Each commits fully to its pole — no hedging, no balanced takes.
3. Optional rebuttal round ONLY if they clash on a fundamental, unresolved point: give each the other's core claim (still blind to identity) for one counter.
4. Synthesize. Reconcile agreements and tensions and TAKE A POSITION — the highest-odds-of-success call. Never a mushy average. The synthesizer MUST (a) hunt the third-level insight that emerges from the clash and that neither pole produced alone, and (b) catch errors BOTH lenses made.

> Proven: a builder-vs-skeptic dialectic on a SQL view rebase surfaced that the real row-count bound was the voucher-filter placement — an insight neither pole produced alone. Hunt for that.

### 2. Adversarial Review — validate CODE/diffs

Defer to the **judgment-day** skill where it is loaded: blind identical judges → agreement verdict (both agree = CONFIRMED; one only = SUSPECT) → surgical fix of CONFIRMED only → mandatory re-judge → terminal `APPROVED` or `ESCALATED`. Make the relevant project skills available to each judge: in Claude Code, resolve their paths from the skill registry and pass them in the Agent prompt; in opencode/codex, name them in the judge's task so the runtime loads them.

Where the `jd-judge-a` / `jd-judge-b` named agent files are not available (i.e. in opencode / codex / any non-Claude-Code runtime), run the SAME state machine with the judge role inline:

> *You are an adversarial code reviewer. Your only job is to find problems. Report findings only, no praise. Each: Severity (CRITICAL | WARNING-real | WARNING-theoretical | SUGGESTION), File:line, what is wrong and why, one-line fix intent. `WARNING-real` only if normal intended use triggers it; else `WARNING-theoretical`. If clean: `VERDICT: CLEAN`.*

Fix-agent inline prompt (used after verdict, for CONFIRMED findings only):

> *You are a surgical fix agent. Apply only the CONFIRMED findings listed. One minimal change per finding. Do not refactor unrelated code.*

State machine: two blind judges in parallel, identical target/criteria → verdict table → **ask the user before fixing** ("Apply fixes for these CONFIRMED findings? [list them]") → fix CONFIRMED only → re-judge → terminal state. After 2 fix rounds with remaining CONFIRMED issues still open, escalate to the user instead of continuing to loop. Optionally widen with the breadth committee (**review-risk / review-readability / review-reliability / review-resilience**, Claude Code only) when one diff needs multiple specialized lenses.

### 3. Parallel Fan-Out — broad/decomposable work

1. Decompose into independent, non-overlapping units (no shared write targets).
2. Delegate the units in parallel via native dispatch (respect codex `max_threads=4`). Dedupe `(role, task)`.
3. Synthesize ONE coherent result; reconcile overlaps and contradictions. Never concatenate raw outputs.

## Decision Gates

| Situation | Mode / Action |
|---|---|
| Validate an idea, design call, or contested assumption | Mode 1 — Dialectic → Synthesis |
| Validate code, a diff, or a PR | Mode 2 — Adversarial Review (judgment-day or inline) |
| Broad, decomposable, breadth work | Mode 3 — Parallel Fan-Out |
| GADU is itself a spawned sub-agent | Do NOT orchestrate; single-thread the work and say so |
| Native dispatch unavailable | Run the protocol single-threaded; flag the degradation |
| Lenses/judges clash fundamentally | One rebuttal round, then synthesize a position; never auto-trust either |
| Agent reasoned without the real artifact | Synthesis labels it hypothesis + names the decisive check |
| SDD-shaped work | Route via `gentle-ai sdd-status` / `sdd-continue`; defer to `sdd-*` phases — do not reimplement |
| Claim needs grounding | **deep-research** where native; else inline multi-source verify (3+ independent sources, contradiction check) |

## Portable Common Layer

These work across runtimes and the skill MAY rely on them: **engram `mem_*`** (cross-runtime memory — read prior context before orchestrating, save decisions/insights after) and the **`gentle-ai` CLI** (`sdd-status`, `sdd-continue`, `skill-registry` refresh). Everything else (Agent/Task, task dispatch, spawn_agent) is runtime-native and named above.

## Output Contract

Return ONE synthesized answer:
1. Mode used and the proposition/target.
2. Reconciled findings — agreements, tensions, and (Mode 1) the third-level insight + errors both lenses made.
3. The POSITION / verdict / merged result with the key tradeoff named.
4. Grounding status: validated against the real artifact, or HYPOTHESIS + the decisive check.
5. Confidence: LOW / MEDIUM / HIGH + what would raise it.

## References

- `judgment-day` skill — canonical blind dual-judge state machine for Mode 2; load where present, mirror inline where not.
- `review-risk` / `review-readability` / `review-reliability` / `review-resilience` — optional code breadth committee (Claude Code only).
- `sdd-*` phases + `gentle-ai sdd-status` / `sdd-continue` — SDD work routing.
- `deep-research` — grounding (Claude Code native); elsewhere run the equivalent multi-source verify inline.
