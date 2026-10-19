---
name: memory-plan
description: "Loads the bound workflow's memory plan as instructions. Trigger: work in a repo bound to a labdrian workflow, before memory reads, or after a denied one."
license: MIT
metadata:
  author: labdrian
  version: "1.0"
---

# Memory plan

The memory plan says what memory a workflow may read. The engine computes it from the workflow
log; this skill only loads it. In Claude Code the overlay's hooks already put the same
instructions in the session and in every subagent prompt; in Codex and Pi this skill is the only
way they arrive.

## Load the plan

Run:

```
labdrian memory plan --instructions
```

It reads the workflow this repository is bound to and prints the instructions block. Follow every
line of it for the rest of the session. To load another workflow's plan, name it:

```
labdrian memory plan --instructions --project <id> --workflow <id>
```

- If it prints that no workflow is bound, there is no plan: nothing here limits memory.
- If it exits with an error, say so in one line and do not guess a plan.

Load it again after a compaction, and whenever the workflow is narrowed (`labdrian workflow
narrow`): the plan can only shrink, and the old block may name a source that is now denied.

## What the instructions mean

- **Sources.** Read memory only from the sources the block names. In Claude Code a read from any
  other source is denied by the PreToolUse gate; elsewhere nothing enforces it, so follow it.
- **Project.** Engram reads name the Engram project the block gives; longterm-mem `query` and `get`
  name the project id it gives.
- **Goal.** Under a goal, pass that `goal` to every longterm-mem `query`, and give every Engram
  record you create for the goal a `topic_key` that begins with `goal:<goal id>/`, for example
  `goal:g-42/architecture/auth`. A record created without the prefix is visible to every goal.
- **Writes.** Memory writes (`mem_save`, `promote`) are not limited by the plan.

## Procedural candidates (T1/T2)

The trigger contract is section 5 of `skills/_shared/procedural-candidate-detection.md`:

- **T1**, right after a proactive `mem_save` of type `bugfix`, `pattern`, `discovery` or
  `decision`: check whether the saved occurrence makes a procedural candidate.
- **T2**, at session close, beside `mem_session_summary`: sweep the session for candidates T1
  missed.

Follow that section for what counts as a candidate and how it is recorded.
