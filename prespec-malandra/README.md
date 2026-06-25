# prespec-malandra

Zero-input Socratic prespec engine — drives a structured discovery interview from a vague idea to a machine-readable brief, with no prior spec required.

---

## The gap

Every structured discovery technique assumes an existing artifact:

| Technique | What it requires |
|-----------|-----------------|
| spec-kit `/clarify` | An existing `spec.md` with a change-name and scope |
| The Mom Test | A human facilitator to conduct the interview |
| Jobs-To-Be-Done | A human analyst to synthesize job statements |
| Impact Mapping | A stated goal, actor list, and deliverable candidates |

None of them handles the **zero-input case**: a person has a vague idea — no requirements, no transcript, no change-name — and needs to know *what to build* before they can spec it.

prespec-malandra fills that gap with an **executable Socratic state machine** that starts from zero, drives discovery over a finite coverage grid, knows when it has enough information, and refuses to fabricate goals when evidence is absent. The novel contribution is the executable assembly, not any individual piece.

---

## How it works

**Deterministic core (`engine prespec <verb>`).**
A pure Go binary that exposes four JSON-in / JSON-out verbs: `rank` (order uncovered coverage cells by impact × uncertainty), `lint` (validate a question against non-leading rules), `readiness` (compute a convergence score against the gate), and `brief` (validate, assign a ULID, render Markdown, and return the persistence path). No LLM, no state — each verb is a pure function over its input. See [SPEC.md](./SPEC.md) for the full contract.

**LLM-driven interview skill (`skills/prespec-malandra/SKILL.md`).**
A Claude Code skill that orchestrates the interview. It calls the engine verbs to pick the next question, check question quality, and decide when to stop. The LLM contributes language and judgment; the engine enforces deterministic sequencing, question-budget, and the readiness gate.

---

## Quickstart

### 1. Build the engine

```bash
cd engine && go build -o /tmp/engine ./cmd
```

### 2. Try the verbs

**`lint` — check whether a question is admissible**

Reject a leading question:
```bash
echo '{"question": "Would you say the biggest problem is slow deploys?"}' \
  | /tmp/engine prespec lint
```
```json
{"accepted":false,"reason":"question steers toward a solution ('would you', 'do you want', 'wouldn't it be nice'); ask what the user needs, not whether they want a specific thing","rule":"smuggles-answer"}
```

Accept a past-behavior question:
```bash
echo '{"question": "Tell me about the last time you tried to track inventory levels manually — what happened?"}' \
  | /tmp/engine prespec lint
```
```json
{"accepted":true,"reason":"","rule":""}
```

**`rank` — order uncovered cells by priority**

```bash
echo '{
  "cells": [
    {"key": "current-gap",   "impact": 5, "uncertainty": 5, "state": "missing"},
    {"key": "why-now",       "impact": 4, "uncertainty": 3, "state": "partial"},
    {"key": "job-statement", "impact": 5, "uncertainty": 1, "state": "clear"}
  ]
}' | /tmp/engine prespec rank
```
```json
{"ranked":[{"key":"current-gap","impact":5,"uncertainty":5,"state":"missing"},{"key":"why-now","impact":4,"uncertainty":3,"state":"partial"}]}
```

`rank` returns only uncovered cells (missing or partial), sorted highest priority first. Clear cells are excluded.

### 3. Run the interview via the skill

Load `skills/prespec-malandra/SKILL.md` in any Claude Code session (or have it injected via the registry), then describe your idea. The skill drives the full interview and calls `engine prespec` verbs internally.

---

## Conformance

The full behavioral contract is in [SPEC.md](./SPEC.md). Any language or agent implementation that satisfies the spec's verb contracts and state machine rules is conformant.

---

## What's novel vs spec-kit

| Capability | spec-kit `/clarify` | prespec-malandra |
|------------|-------------------|-----------------|
| Cold-start from zero | Requires existing `spec.md` | Starts from a single vague sentence |
| Executable question sequencing | No — human-driven | Yes — rank verb + coverage grid |
| Sufficiency / convergence criterion | No | Yes — readiness gate (score ≥ 0.6) |
| Honest refusal floor | No | Yes — exits rather than fabricating |
| Machine-readable brief | No | Yes — brief verb emits JSON + Markdown |

---

## Attribution

This project borrows and credits the following techniques. The new contribution is their executable assembly.

| Source | What is borrowed |
|--------|-----------------|
| spec-kit `/clarify` | Coverage taxonomy and question-budget concept |
| The Mom Test (Rob Fitzpatrick) | Past-behavior probing, non-leading question rules |
| Jobs-To-Be-Done (Christensen) | Job statement framing |
| Impact Mapping (Gojko Adzic) | Goal → deliverable ordering |

---

## Status

**MVP — v1.0 (standard mode, budget 5).**

Deferred to v2:

- Tier-based mode selection (standard / deep / express)
- Security cells → automatic High promotion
- Enforced `sdd-propose` crosswalk after brief generation
- `sdd-explore` auto-read on session start

---

## License

MIT — see [LICENSE](./LICENSE).
