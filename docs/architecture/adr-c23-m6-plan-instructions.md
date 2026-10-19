# ADR: the memory plan reaches the agent as instructions (C23, M6)

- **Status:** Accepted, 2026-10-19.
- **Decided by:** the owner, three questions asked one at a time on 2026-10-19. What the instructions
  must say was decided in M1 (`adr-c23-m1-memory-sources.md`, M6) and M5
  (`adr-c23-m5-goal-filter.md`, M6).
- **Ledger:** `odd/tasks/consolidation-phase9.md`, task C23, unit M6 (gaps G1 and G2, Engram #4133).

## Context

The memory plan reaches Claude Code on one route only: the projection family's `UserPromptSubmit`
hook (`engine/settings/projection.go`), whose context carries one line rendered by
`memoryPlanLines` (`engine/projection/render.go`):

```
memory plan (read-only: it executes no query and grants no memory write): scope=goal sources=engram,longterm-mem project_id=p goal_id=g write=...
```

The line describes the plan; it tells the agent to do nothing. Nothing says that a read outside the
plan is denied (M4), that a `longterm-mem query` under scope goal must pass `goal` (M5), or that a
record created under a goal takes the topic prefix `goal:<id>/` (M5). No skill or agent mentions the
plan (G1).

Subagents never see it (G2). The overlay installs no `SessionStart` hook. Its one route into a
subagent is the PreToolUse `Agent` hook (`gate-task`, `engine/gate/gate.go`), which appends contract
paths to the subagent's prompt by `subagent_type`. Codex and Pi have no hook at all
(`engine/capability/declarations.go`), so for them only a skill can carry the plan.

The T1/T2 contract is section 5 of `skills/_shared/procedural-candidate-detection.md`: T1 runs right
after a proactive `mem_save` of type `bugfix`, `pattern`, `discovery` or `decision`; T2 is the sweep
at session close, beside `mem_session_summary`.

## Decision

1. **The main session: SessionStart, and the line stays (the owner, 2026-10-19).** A new
   `SessionStart` hook (sources `startup`, `resume`, `clear`, `compact`) injects the full
   instructions block once, while a workflow is bound. Each `UserPromptSubmit` keeps today's plan
   line, so a workflow bound or narrowed in the middle of a session is still seen.
2. **Subagents: the full block in the prompt (the owner, 2026-10-19).** While a workflow is bound,
   the PreToolUse `Agent` hook appends the same instructions block to every subagent's prompt. The
   owner weighed a contract path, which the subagent would have to read with a tool call costing as
   much as the block, and injection into memory roles only, whose undeclared roles would learn the
   plan from the gate's denials.
3. **One invocable skill, `memory-plan` (the owner, 2026-10-19).** An overlay skill,
   `skills/memory-plan/SKILL.md`, tells the agent to run `labdrian memory plan --instructions` and
   follow what it prints, and states the T1/T2 contract. It works the same in Claude Code, Codex and
   Pi. Like any overlay skill it carries the owner's approval record.

### The instructions block

One renderer in the engine writes the block, from the plan `resolvePlan` computes, so the hooks and
the skill say the same thing. Under scope goal it reads:

```
labdrian memory instructions (workflow <wf>, scope goal):
- Read memory only from: engram, longterm-mem. A read from any other source is denied.
- Engram reads (mem_search, mem_context) name project "<engram name>"; longterm-mem query and get name project "<project id>".
- Under goal "<goal id>": pass goal "<goal id>" to every longterm-mem query, and give every Engram record you create for this goal a topic_key that begins with "goal:<goal id>/", for example "goal:<goal id>/architecture/auth".
- Memory writes (mem_save, promote) are not limited by the plan.
- Procedural candidates: T1 right after a proactive mem_save of type bugfix, pattern, discovery or decision; T2 at session close, beside mem_session_summary (skills/_shared/procedural-candidate-detection.md, section 5).
```

The goal line appears only under scope goal. Under scope none the second line says to read no
memory, because the plan permits no source. A plan that cannot be computed gives one line saying
so, as `memoryPlanLines` does today. Every value from the workflow is sanitized to one line, as the
projection already does.

### Alternatives considered

| Decision | Rejected | What choosing it would have meant |
|---|---|---|
| 1 | The full block on every `UserPromptSubmit` | Always current, but about six lines repeated every turn. |
| 1 | `SessionStart` only, the prompt line removed | A workflow bound or narrowed mid-session would go unseen until the next resume or compaction. |
| 2 | A contract path and the plan line | The subagent reads a file to learn six lines; if it does not, it knows no rule. |
| 2 | Only the memory roles | A role nobody declared learns the plan from denials and retries. |
| 3 | A reference doc in `skills/_shared/` | Codex and Pi never load it, so G1 stays open outside Claude Code. |
| 3 | Extend `procedural-candidate-detection.md` | Mixes the read plan with candidate detection, and still reaches neither Codex nor Pi. |

## Consequences

- **One rewrite per `Agent` call.** The plan block joins the contract paths in the same `gate-task`
  rewrite, which reads the repository's binding as the projection does. A second PreToolUse hook
  rewriting the same call would race the first: Claude Code runs matching hooks in parallel and
  names no winner between two rewritten inputs. `install-hooks` already runs two `gate-task`
  entries on every `Agent` call, so the block rides the `--contract-file` (minimalism) entry alone,
  and the `--embedded-contract` entry answers bound as it answers unbound.
- **A new hook event.** `install-hooks` installs a `SessionStart` entry in the projection family, and
  `projection hook` accepts the event. Removing the family removes it, as for its other entries.
- **`memory plan --instructions`.** Without `--project` and `--workflow` it reads the repository's
  binding; with them, that workflow. With no binding it prints that no workflow is bound and exits 0.
- **Nothing is injected without a binding.** No workflow, no block: the session and the subagents
  stay as they are today.
- **Not enforced here.** The instructions add no rule. M4 and M5 enforce reads and the query goal; a
  record created without the goal prefix is still accepted, as M5 accepted.
- **Codex and Pi** get the plan only when the skill is loaded; their hooks are M12.
